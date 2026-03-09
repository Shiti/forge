package secrets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// FileSecretProvider resolves secrets by reading files from a directory.
// The file name should match the secret key.
type FileSecretProvider struct {
	BaseDir string
}

func NewFileSecretProvider(baseDir string) *FileSecretProvider {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			baseDir = filepath.Join(home, ".forge", "secrets")
		}
	}
	return &FileSecretProvider{
		BaseDir: baseDir,
	}
}

func (p *FileSecretProvider) Resolve(ctx context.Context, key string) (string, error) {
	if p.BaseDir == "" {
		return "", ErrSecretNotFound
	}
	cleanKey := filepath.Base(key)
	if cleanKey == "" || cleanKey == "." || cleanKey == "/" {
		return "", ErrSecretNotFound
	}

	fullPath := filepath.Join(p.BaseDir, cleanKey)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrSecretNotFound
		}
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}
