package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	baseDir, err := os.MkdirTemp("", "forge-testutil-e2e-env-*")
	if err == nil {
		homeDir := filepath.Join(baseDir, "home")
		xdgCache := filepath.Join(baseDir, "xdg-cache")
		xdgData := filepath.Join(baseDir, "xdg-data")
		uvCache := filepath.Join(baseDir, "uv-cache")
		tmpDir := filepath.Join(baseDir, "tmp")
		_ = os.MkdirAll(homeDir, 0o755)
		_ = os.MkdirAll(xdgCache, 0o755)
		_ = os.MkdirAll(xdgData, 0o755)
		_ = os.MkdirAll(uvCache, 0o755)
		_ = os.MkdirAll(tmpDir, 0o755)

		_ = os.Setenv("HOME", homeDir)
		_ = os.Setenv("XDG_CACHE_HOME", xdgCache)
		_ = os.Setenv("XDG_DATA_HOME", xdgData)
		_ = os.Setenv("TMPDIR", tmpDir)
		_ = os.Setenv("FORGE_UV_CACHE_DIR", uvCache)
		_ = os.Setenv("UV_CACHE_DIR", uvCache)
	}
	os.Exit(m.Run())
}
