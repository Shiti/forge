package api

import (
	"fmt"
	"os"
	"strings"

	"github.com/rustic-ai/forge/forge-go/filesystem"
	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"gopkg.in/yaml.v3"
)

func (s *Server) resolveFilesystemDependency(guildID string) (string, filesystem.DependencyConfig, int, error) {
	guildModel, err := s.store.GetGuild(guildID)
	if err != nil {
		return "", filesystem.DependencyConfig{}, 404, fmt.Errorf("Guild not found")
	}

	spec := store.ToGuildSpec(guildModel)
	merged := map[string]protocol.DependencySpec{}

	defaultDeps, err := loadDependencyConfigSpecs(dependencyConfigPath())
	if err != nil {
		return "", filesystem.DependencyConfig{}, 500, err
	}
	for k, v := range defaultDeps {
		merged[k] = v
	}
	for k, v := range spec.DependencyMap {
		merged[k] = v
	}

	depSpec, ok := merged["filesystem"]
	if !ok {
		return "", filesystem.DependencyConfig{}, 404, fmt.Errorf("Dependency for filesystem not configured for guild %s", guildID)
	}

	cfg := filesystem.DependencyConfig{
		ClassName:      depSpec.ClassName,
		Protocol:       "file",
		StorageOptions: map[string]any{},
	}
	if depSpec.Properties != nil {
		if base, ok := depSpec.Properties["path_base"].(string); ok {
			cfg.PathBase = strings.TrimSpace(base)
		}
		if protocolName, ok := depSpec.Properties["protocol"].(string); ok && strings.TrimSpace(protocolName) != "" {
			cfg.Protocol = strings.ToLower(strings.TrimSpace(protocolName))
		}
		if options, ok := depSpec.Properties["storage_options"].(map[string]any); ok && options != nil {
			cfg.StorageOptions = options
		}
	}
	if cfg.StorageOptions == nil {
		cfg.StorageOptions = map[string]any{}
	}

	orgID := strings.TrimSpace(guildModel.OrganizationID)
	if orgID == "" {
		orgID = guildID
	}

	return orgID, cfg, 200, nil
}

func loadDependencyConfigSpecs(configPath string) (map[string]protocol.DependencySpec, error) {
	fileData, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]protocol.DependencySpec{}, nil
		}
		return nil, fmt.Errorf("read dependency config: %w", err)
	}

	var fileDeps map[string]protocol.DependencySpec
	if err := yaml.Unmarshal(fileData, &fileDeps); err != nil {
		return nil, fmt.Errorf("parse dependency config: %w", err)
	}
	if fileDeps == nil {
		return map[string]protocol.DependencySpec{}, nil
	}
	return fileDeps, nil
}
