package secrets

import (
	"context"
	"os"
)

// EnvSecretProvider resolves secrets from environment variables
type EnvSecretProvider struct{}

func NewEnvSecretProvider() *EnvSecretProvider {
	return &EnvSecretProvider{}
}

func (p *EnvSecretProvider) Resolve(ctx context.Context, key string) (string, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}
