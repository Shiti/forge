package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	extProvidersMu sync.RWMutex
	extProviders   = map[string]func() SecretProvider{}
)

// RegisterProvider registers an external provider type by name so it can be
// activated via FORGE_SECRET_PROVIDERS. Call from an init() function.
func RegisterProvider(name string, fn func() SecretProvider) {
	extProvidersMu.Lock()
	defer extProvidersMu.Unlock()
	extProviders[name] = fn
}

func newProvider(name string) (SecretProvider, error) {
	switch name {
	case "env":
		return NewEnvSecretProvider(), nil
	case "dotenv":
		return NewDotEnvSecretProvider(""), nil
	case "file":
		return NewFileSecretProvider(""), nil
	default:
		extProvidersMu.RLock()
		fn, ok := extProviders[name]
		extProvidersMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown secret provider type %q", name)
		}
		return fn(), nil
	}
}

type ChainSecretProvider struct {
	providers []SecretProvider
}

func NewChainSecretProvider(providers ...SecretProvider) *ChainSecretProvider {
	return &ChainSecretProvider{providers: providers}
}

func (p *ChainSecretProvider) Resolve(ctx context.Context, key string) (string, error) {
	for _, provider := range p.providers {
		val, err := provider.Resolve(ctx, key)
		if err == nil {
			return val, nil
		}
	}
	return "", ErrSecretNotFound
}

// DefaultProvider builds the secret resolution chain from FORGE_SECRET_PROVIDERS
// (comma-separated, e.g. "env,dotenv,file,keychain"). Falls back to
// "env,dotenv,file". External types (e.g. keychain) require a side-effect import
// of their package to register via RegisterProvider.
func DefaultProvider() SecretProvider {
	order := os.Getenv("FORGE_SECRET_PROVIDERS")
	if order == "" {
		order = "env,dotenv,file"
	}
	var providers []SecretProvider
	for _, name := range strings.Split(order, ",") {
		p, err := newProvider(strings.TrimSpace(name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "secrets: %v (check FORGE_SECRET_PROVIDERS and side-effect imports)\n", err)
			continue
		}
		providers = append(providers, p)
	}
	if len(providers) == 0 {
		return NewChainSecretProvider(
			NewEnvSecretProvider(),
			NewDotEnvSecretProvider(""),
			NewFileSecretProvider(""),
		)
	}
	return NewChainSecretProvider(providers...)
}
