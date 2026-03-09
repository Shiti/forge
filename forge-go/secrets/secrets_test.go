package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvSecretProvider(t *testing.T) {
	p := NewEnvSecretProvider()
	ctx := context.Background()

	os.Setenv("TEST_SECRET_ENV", "env_value")
	defer os.Unsetenv("TEST_SECRET_ENV")

	val, err := p.Resolve(ctx, "TEST_SECRET_ENV")
	if err != nil {
		t.Fatalf("Expected to find env secret but got error: %v", err)
	}
	if val != "env_value" {
		t.Errorf("Expected env_value, got %s", val)
	}

	_, err = p.Resolve(ctx, "NONEXISTENT_ENV")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}

func TestFileSecretProvider(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewFileSecretProvider(tmpDir)
	ctx := context.Background()

	secretPath := filepath.Join(tmpDir, "API_KEY")
	if err := os.WriteFile(secretPath, []byte("  file_value_with_spaces  \n"), 0600); err != nil {
		t.Fatalf("Failed to write mock secret file: %v", err)
	}

	val, err := p.Resolve(ctx, "API_KEY")
	if err != nil {
		t.Fatalf("Expected to find file secret but got error: %v", err)
	}
	if val != "file_value_with_spaces" {
		t.Errorf("Expected trimmed 'file_value_with_spaces', got '%s'", val)
	}

	_, err = p.Resolve(ctx, "UNKNOWN_FILE")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}

func TestChainSecretProvider(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file secret
	secretPath := filepath.Join(tmpDir, "SHARED_KEY")
	os.WriteFile(secretPath, []byte("file_wins"), 0600)

	envP := NewEnvSecretProvider()
	fileP := NewFileSecretProvider(tmpDir)

	// Chain 1: Env -> File
	chain1 := NewChainSecretProvider(envP, fileP)

	ctx := context.Background()

	val, err := chain1.Resolve(ctx, "SHARED_KEY")
	if err != nil {
		t.Fatalf("chain1 failed to resolve SHARED_KEY: %v", err)
	}
	if val != "file_wins" {
		t.Errorf("Expected file_wins, got %s", val)
	}

	// Now set ENV var, which should take precedence
	os.Setenv("SHARED_KEY", "env_wins")
	defer os.Unsetenv("SHARED_KEY")

	val2, err := chain1.Resolve(ctx, "SHARED_KEY")
	if err != nil {
		t.Fatalf("chain1 failed to resolve SHARED_KEY again: %v", err)
	}
	if val2 != "env_wins" {
		t.Errorf("Expected env_wins, got %s", val2)
	}

	// Unknown key
	_, err = chain1.Resolve(ctx, "NONEXISTENT")
	if err != ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got %v", err)
	}
}
