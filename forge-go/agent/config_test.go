package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("defaults and positional argument", func(t *testing.T) {
		cfg, err := LoadConfig(map[string]string{}, []string{"spec.yaml"})
		require.NoError(t, err)
		assert.Equal(t, "spec.yaml", cfg.SpecFile)
		assert.Empty(t, cfg.RedisAddr)
		assert.Empty(t, cfg.RegistryPath)
	})

	t.Run("environment variables override defaults", func(t *testing.T) {
		os.Setenv("FORGE_REDIS", "redis.local:6379")
		os.Setenv("FORGE_REGISTRY", "/tmp/registry.yaml")
		defer os.Unsetenv("FORGE_REDIS")
		defer os.Unsetenv("FORGE_REGISTRY")

		cfg, err := LoadConfig(map[string]string{}, []string{"spec.yaml"})
		require.NoError(t, err)
		assert.Equal(t, "redis.local:6379", cfg.RedisAddr)
		assert.Equal(t, "/tmp/registry.yaml", cfg.RegistryPath)
	})

	t.Run("explicit map overrides env", func(t *testing.T) {
		os.Setenv("FORGE_REDIS", "redis.local:6379")
		defer os.Unsetenv("FORGE_REDIS")

		flags := map[string]string{
			"redis": "cli-redis:9000",
		}

		cfg, err := LoadConfig(flags, []string{"spec.yaml"})
		require.NoError(t, err)
		assert.Equal(t, "cli-redis:9000", cfg.RedisAddr)
	})

	t.Run("missing positional arg", func(t *testing.T) {
		_, err := LoadConfig(map[string]string{}, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec file required")
	})
}
