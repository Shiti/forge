package modelfit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"gopkg.in/yaml.v3"
)

type Backend string

const (
	BackendCPU     Backend = "cpu"
	BackendCUDA    Backend = "cuda"
	BackendMetal   Backend = "metal"
	BackendROCM    Backend = "rocm"
	BackendUnknown Backend = "unknown"
)

type FitLevel string

const (
	FitPerfect  FitLevel = "perfect"
	FitGood     FitLevel = "good"
	FitMarginal FitLevel = "marginal"
	FitTooTight FitLevel = "too_tight"
)

type SystemProfile struct {
	TotalRAMBytes     uint64  `json:"total_ram_bytes"`
	AvailableRAMBytes uint64  `json:"available_ram_bytes"`
	CPUCores          int     `json:"cpu_cores"`
	HasGPU            bool    `json:"has_gpu"`
	GPUCount          int     `json:"gpu_count"`
	TotalVRAMBytes    uint64  `json:"total_vram_bytes"`
	Backend           Backend `json:"backend"`
	UnifiedMemory     bool    `json:"unified_memory"`
	CPUName           string  `json:"cpu_name,omitempty"`
	GPUName           string  `json:"gpu_name,omitempty"`
}

type ModelProfile struct {
	ID                   string   `json:"id" yaml:"id"`
	DisplayName          string   `json:"display_name" yaml:"display_name"`
	DependencyKey        string   `json:"dependency_key" yaml:"dependency_key"`
	ResolverClassName    string   `json:"resolver_class_name" yaml:"-"`
	ProvidedType         string   `json:"provided_type" yaml:"-"`
	ModelName            string   `json:"model_name" yaml:"model_name"`
	BaseURL              string   `json:"base_url" yaml:"-"`
	ParameterCountB      float64  `json:"parameter_count_b" yaml:"parameter_count_b"`
	Quantization         string   `json:"quantization" yaml:"quantization"`
	ContextLength        int      `json:"context_length" yaml:"context_length"`
	MinRAMBytes          uint64   `json:"min_ram_bytes" yaml:"min_ram_bytes"`
	PreferredVRAMBytes   uint64   `json:"preferred_vram_bytes" yaml:"preferred_vram_bytes"`
	EstimatedMemoryBytes uint64   `json:"estimated_memory_bytes" yaml:"estimated_memory_bytes"`
	EmbeddingOnly        bool     `json:"embedding_only" yaml:"embedding_only"`
	UseCaseTags          []string `json:"use_case_tags" yaml:"use_case_tags"`
	QualityRank          int      `json:"quality_rank" yaml:"quality_rank"`
	TokenSpeedHint       float64  `json:"token_speed_hint,omitempty" yaml:"token_speed_hint,omitempty"`
	Multimodal           bool     `json:"multimodal,omitempty" yaml:"multimodal,omitempty"`
	PreferredDiscreteGPU bool     `json:"preferred_discrete_gpu,omitempty" yaml:"preferred_discrete_gpu,omitempty"`
}

type FitResult struct {
	ModelID                  string   `json:"model_id"`
	DependencyKey            string   `json:"dependency_key"`
	DisplayName              string   `json:"display_name"`
	ModelName                string   `json:"model_name"`
	UseCaseTags              []string `json:"use_case_tags"`
	FitLevel                 FitLevel `json:"fit_level"`
	Runnable                 bool     `json:"runnable"`
	EstimatedMemoryBytes     uint64   `json:"estimated_memory_bytes"`
	AvailableMemoryBytes     uint64   `json:"available_memory_bytes"`
	UtilizationPct           float64  `json:"utilization_pct"`
	EstimatedTokensPerSecond *float64 `json:"estimated_tokens_per_second,omitempty"`
	Score                    float64  `json:"score"`
	Explanations             []string `json:"explanations"`
}

type QueryOptions struct {
	UseCase      string
	Limit        int
	RunnableOnly bool
}

type Profiler interface {
	Profile(context.Context) (SystemProfile, error)
}

type DefaultProfiler struct{}

func (DefaultProfiler) Profile(context.Context) (SystemProfile, error) {
	return DetectSystemProfile()
}

type catalogFile struct {
	Models []ModelProfile `yaml:"models"`
}

var (
	virtualMemoryFn   = mem.VirtualMemory
	cpuInfoFn         = cpu.Info
	nvidiaSMIFunc     = defaultNvidiaSMI
	macGPUProfileFunc = defaultMacGPUProfile
)

func LoadProfiles(catalogPath, dependencyConfigPath string) ([]ModelProfile, error) {
	fileData, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read local model catalog: %w", err)
	}

	var catalog catalogFile
	if err := yaml.Unmarshal(fileData, &catalog); err != nil {
		return nil, fmt.Errorf("parse local model catalog: %w", err)
	}

	deps, err := loadDependencySpecs(dependencyConfigPath)
	if err != nil {
		return nil, err
	}

	out := make([]ModelProfile, 0, len(catalog.Models))
	seen := map[string]struct{}{}
	for _, model := range catalog.Models {
		if model.DependencyKey == "" {
			return nil, errors.New("local model catalog entry missing dependency_key")
		}
		if _, ok := seen[model.DependencyKey]; ok {
			return nil, fmt.Errorf("duplicate local model dependency key %q", model.DependencyKey)
		}
		seen[model.DependencyKey] = struct{}{}
		if model.ID == "" || model.DisplayName == "" || model.ModelName == "" {
			return nil, fmt.Errorf("local model catalog entry %q missing required identity fields", model.DependencyKey)
		}
		if model.ParameterCountB <= 0 || model.ContextLength <= 0 || model.EstimatedMemoryBytes == 0 {
			return nil, fmt.Errorf("local model catalog entry %q missing fit metadata", model.DependencyKey)
		}
		spec, ok := deps[model.DependencyKey]
		if !ok {
			return nil, fmt.Errorf("local model dependency key %q not found in dependency config", model.DependencyKey)
		}
		if spec.ClassName == "" {
			return nil, fmt.Errorf("dependency config entry %q missing class_name", model.DependencyKey)
		}
		modelName, _ := spec.Properties["model"].(string)
		if modelName != model.ModelName {
			return nil, fmt.Errorf("local model dependency key %q model mismatch: catalog=%q dependency=%q", model.DependencyKey, model.ModelName, modelName)
		}
		baseURL, _ := spec.Properties["base_url"].(string)
		model.ResolverClassName = spec.ClassName
		model.ProvidedType = spec.ProvidedType
		model.BaseURL = baseURL
		model.UseCaseTags = normalizeStrings(model.UseCaseTags)
		out = append(out, model)
	}

	slices.SortFunc(out, func(a, b ModelProfile) int {
		return strings.Compare(a.DependencyKey, b.DependencyKey)
	})
	return out, nil
}

func Recommend(profiles []ModelProfile, system SystemProfile, opts QueryOptions) []FitResult {
	useCase := strings.TrimSpace(strings.ToLower(opts.UseCase))
	results := make([]FitResult, 0, len(profiles))
	for _, profile := range profiles {
		if useCase != "" && !containsStringFold(profile.UseCaseTags, useCase) {
			continue
		}
		result := evaluate(profile, system)
		if opts.RunnableOnly && !result.Runnable {
			continue
		}
		results = append(results, result)
	}

	slices.SortFunc(results, compareResults)
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results
}

func DetectSystemProfile() (SystemProfile, error) {
	vm, err := virtualMemoryFn()
	if err != nil {
		return SystemProfile{}, fmt.Errorf("detect memory: %w", err)
	}
	profile := SystemProfile{
		TotalRAMBytes:     vm.Total,
		AvailableRAMBytes: vm.Available,
		CPUCores:          runtime.NumCPU(),
		Backend:           BackendCPU,
	}
	if info, err := cpuInfoFn(); err == nil && len(info) > 0 {
		profile.CPUName = strings.TrimSpace(info[0].ModelName)
	}

	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			profile.HasGPU = true
			profile.GPUCount = 1
			profile.Backend = BackendMetal
			profile.UnifiedMemory = true
		}
		if name, count, err := macGPUProfileFunc(); err == nil {
			if strings.TrimSpace(name) != "" {
				profile.GPUName = name
			}
			if count > 0 {
				profile.GPUCount = count
				profile.HasGPU = true
			}
		}
	default:
		if gpuName, count, vramBytes, err := nvidiaSMIFunc(); err == nil && count > 0 {
			profile.HasGPU = true
			profile.GPUCount = count
			profile.TotalVRAMBytes = vramBytes
			profile.GPUName = gpuName
			profile.Backend = BackendCUDA
		}
	}

	return profile, nil
}

func evaluate(profile ModelProfile, system SystemProfile) FitResult {
	var required uint64
	var available uint64
	reasons := []string{}

	if profile.PreferredDiscreteGPU && system.HasGPU && !system.UnifiedMemory && system.TotalVRAMBytes > 0 {
		required = maxUint64(profile.PreferredVRAMBytes, profile.EstimatedMemoryBytes)
		available = system.TotalVRAMBytes
		reasons = append(reasons, "Using discrete GPU memory pool")
	} else if system.UnifiedMemory {
		required = maxUint64(profile.MinRAMBytes, profile.EstimatedMemoryBytes)
		available = system.AvailableRAMBytes
		reasons = append(reasons, "Using unified memory pool")
	} else {
		required = maxUint64(profile.MinRAMBytes, profile.EstimatedMemoryBytes)
		available = system.AvailableRAMBytes
		reasons = append(reasons, "Using system RAM")
	}

	utilization := 0.0
	if available > 0 {
		utilization = (float64(required) / float64(available)) * 100
	}
	fit := classify(utilization, available > 0)
	if profile.EmbeddingOnly {
		reasons = append(reasons, "Embedding-only model")
	}
	switch fit {
	case FitPerfect:
		reasons = append(reasons, "Fits comfortably on this machine")
	case FitGood:
		reasons = append(reasons, "Runnable with reasonable headroom")
	case FitMarginal:
		reasons = append(reasons, "Runnable but memory is tight")
	case FitTooTight:
		reasons = append(reasons, "Exceeds available memory")
	}
	if !system.HasGPU && profile.PreferredDiscreteGPU {
		reasons = append(reasons, "Discrete GPU preferred, but no GPU detected")
	}

	var tps *float64
	if estimate := estimateTPS(profile, system); estimate > 0 {
		tps = &estimate
	}

	return FitResult{
		ModelID:                  profile.ID,
		DependencyKey:            profile.DependencyKey,
		DisplayName:              profile.DisplayName,
		ModelName:                profile.ModelName,
		UseCaseTags:              append([]string(nil), profile.UseCaseTags...),
		FitLevel:                 fit,
		Runnable:                 fit != FitTooTight,
		EstimatedMemoryBytes:     required,
		AvailableMemoryBytes:     available,
		UtilizationPct:           round2(utilization),
		EstimatedTokensPerSecond: tps,
		Score:                    score(profile, fit, utilization, tps),
		Explanations:             reasons,
	}
}

func classify(utilization float64, available bool) FitLevel {
	if !available {
		return FitTooTight
	}
	switch {
	case utilization <= 70:
		return FitPerfect
	case utilization <= 85:
		return FitGood
	case utilization <= 100:
		return FitMarginal
	default:
		return FitTooTight
	}
}

func estimateTPS(profile ModelProfile, system SystemProfile) float64 {
	if profile.TokenSpeedHint > 0 {
		switch {
		case system.HasGPU && !system.UnifiedMemory:
			return round2(profile.TokenSpeedHint)
		case system.UnifiedMemory:
			return round2(profile.TokenSpeedHint * 0.7)
		default:
			return round2(profile.TokenSpeedHint * 0.2)
		}
	}
	base := 0.0
	switch {
	case system.HasGPU && !system.UnifiedMemory:
		base = 120
	case system.UnifiedMemory:
		base = 75
	default:
		base = 18
	}
	if profile.ParameterCountB <= 0 {
		return 0
	}
	return round2(base / math.Max(profile.ParameterCountB, 0.5))
}

func score(profile ModelProfile, fit FitLevel, utilization float64, tps *float64) float64 {
	fitBase := map[FitLevel]float64{
		FitPerfect:  400,
		FitGood:     300,
		FitMarginal: 200,
		FitTooTight: 100,
	}[fit]
	quality := math.Max(0, 100-float64(profile.QualityRank*10))
	speed := 0.0
	if tps != nil {
		speed = math.Min(*tps, 100)
	}
	return round2(fitBase + quality + speed - math.Min(utilization, 150))
}

func compareResults(a, b FitResult) int {
	if a.Runnable != b.Runnable {
		if a.Runnable {
			return -1
		}
		return 1
	}
	if fa, fb := fitWeight(a.FitLevel), fitWeight(b.FitLevel); fa != fb {
		return cmpInt(fb, fa)
	}
	if a.Score != b.Score {
		if a.Score > b.Score {
			return -1
		}
		return 1
	}
	if a.EstimatedMemoryBytes != b.EstimatedMemoryBytes {
		if a.EstimatedMemoryBytes < b.EstimatedMemoryBytes {
			return -1
		}
		return 1
	}
	return strings.Compare(a.DependencyKey, b.DependencyKey)
}

func fitWeight(level FitLevel) int {
	switch level {
	case FitPerfect:
		return 4
	case FitGood:
		return 3
	case FitMarginal:
		return 2
	default:
		return 1
	}
}

func loadDependencySpecs(path string) (map[string]protocol.DependencySpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dependency config: %w", err)
	}
	var specs map[string]protocol.DependencySpec
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, fmt.Errorf("parse dependency config: %w", err)
	}
	for key, spec := range specs {
		spec.Normalize()
		specs[key] = spec
	}
	return specs, nil
}

func defaultNvidiaSMI() (string, int, uint64, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return "", 0, 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", 0, 0, errors.New("no nvidia gpus found")
	}
	var total uint64
	var name string
	count := 0
	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		count++
		if name == "" {
			name = strings.TrimSpace(parts[0])
		}
		memMB, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			continue
		}
		total += memMB * 1024 * 1024
	}
	if count == 0 {
		return "", 0, 0, errors.New("no parseable nvidia gpu rows")
	}
	return name, count, total, nil
}

func defaultMacGPUProfile() (string, int, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return "", 0, err
	}
	var payload struct {
		Displays []map[string]any `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", 0, err
	}
	if len(payload.Displays) == 0 {
		return "", 0, errors.New("no mac gpu devices")
	}
	name, _ := payload.Displays[0]["sppci_model"].(string)
	return strings.TrimSpace(name), len(payload.Displays), nil
}

func normalizeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(strings.ToLower(item))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	slices.Sort(out)
	return out
}

func containsStringFold(items []string, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
