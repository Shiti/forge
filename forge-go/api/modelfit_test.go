package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rustic-ai/forge/forge-go/modelfit"
	"github.com/stretchr/testify/require"
)

type staticProfiler struct {
	profile modelfit.SystemProfile
	err     error
}

func (p staticProfiler) Profile(context.Context) (modelfit.SystemProfile, error) {
	return p.profile, p.err
}

func TestRusticModelFitRouteReturnsRankedLocalModels(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	dir := t.TempDir()
	dependencyConfigPath := filepath.Join(dir, "agent-dependencies.yaml")
	catalogPath := filepath.Join(dir, "local-model-catalog.yaml")

	require.NoError(t, os.WriteFile(dependencyConfigPath, []byte(`
llm_local_small:
  class_name: rustic_ai.litellm.agent_ext.llm.LiteLLMResolver
  provided_type: rustic_ai.core.llm.LLM
  properties:
    model: openai/rustic/small
    base_url: http://localhost:55262/v1
llm_local_large:
  class_name: rustic_ai.litellm.agent_ext.llm.LiteLLMResolver
  provided_type: rustic_ai.core.llm.LLM
  properties:
    model: openai/rustic/large
    base_url: http://localhost:55262/v1
`), 0o644))
	require.NoError(t, os.WriteFile(catalogPath, []byte(`
models:
  - id: small
    display_name: Small
    dependency_key: llm_local_small
    model_name: openai/rustic/small
    parameter_count_b: 2
    quantization: Q4_K_M
    context_length: 8192
    min_ram_bytes: 2147483648
    preferred_vram_bytes: 2147483648
    estimated_memory_bytes: 2147483648
    use_case_tags: [coding]
    quality_rank: 2
    token_speed_hint: 40
  - id: large
    display_name: Large
    dependency_key: llm_local_large
    model_name: openai/rustic/large
    parameter_count_b: 7
    quantization: Q4_K_M
    context_length: 16384
    min_ram_bytes: 12884901888
    preferred_vram_bytes: 8589934592
    estimated_memory_bytes: 12884901888
    use_case_tags: [coding]
    quality_rank: 1
    preferred_discrete_gpu: true
`), 0o644))

	s := NewServer(nil, nil, nil, nil, nil, ":0").WithModelFit(
		catalogPath,
		dependencyConfigPath,
		staticProfiler{profile: modelfit.SystemProfile{
			TotalRAMBytes:     16 * 1024 * 1024 * 1024,
			AvailableRAMBytes: 8 * 1024 * 1024 * 1024,
			CPUCores:          8,
		}},
	)
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/modelfit/local-models?use_case=coding&runnable_only=true&limit=1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var results []modelfit.FitResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &results))
	require.Len(t, results, 1)
	require.Equal(t, "llm_local_small", results[0].DependencyKey)
	require.True(t, results[0].Runnable)
	require.Equal(t, modelfit.FitPerfect, results[0].FitLevel)
}

func TestRusticModelFitRouteRejectsInvalidLimit(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	s := NewServer(nil, nil, nil, nil, nil, ":0")
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/modelfit/local-models?limit=abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "invalid limit")
}

func TestRusticModelFitCapabilitiesRouteReturnsSystemProfile(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	s := NewServer(nil, nil, nil, nil, nil, ":0").WithModelFit(
		"",
		"",
		staticProfiler{profile: modelfit.SystemProfile{
			TotalRAMBytes:             32 * 1024 * 1024 * 1024,
			AvailableRAMBytes:         24 * 1024 * 1024 * 1024,
			CPUCores:                  12,
			Backend:                   modelfit.BackendCUDA,
			RuntimeUsableAcceleration: true,
			SelectedAcceleratorID:     "nvidia-0",
			Confidence:                modelfit.DetectionConfidenceProbe,
			ReasonCodes:               []modelfit.DiagnosticReason{modelfit.ReasonRuntimeDeviceDetected},
			Runtime: modelfit.RuntimeCapabilityProfile{
				RuntimeAvailable: true,
				SelectedBackend:  modelfit.BackendCUDA,
				Confidence:       modelfit.DetectionConfidenceProbe,
				UsableAccelerators: []modelfit.UsableAccelerator{
					{
						ID:               "nvidia-0",
						Vendor:           "nvidia",
						Name:             "RTX 4090",
						Backend:          modelfit.BackendCUDA,
						Discrete:         true,
						TotalMemoryBytes: 24 * 1024 * 1024 * 1024,
					},
				},
			},
		}},
	)
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/modelfit/capabilities", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var system modelfit.SystemProfile
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &system))
	require.True(t, system.RuntimeUsableAcceleration)
	require.Equal(t, modelfit.BackendCUDA, system.Backend)
	require.Equal(t, "nvidia-0", system.SelectedAcceleratorID)
	require.Contains(t, system.ReasonCodes, modelfit.ReasonRuntimeDeviceDetected)
	require.Len(t, system.Runtime.UsableAccelerators, 1)
}
