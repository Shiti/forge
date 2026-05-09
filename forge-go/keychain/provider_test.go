package keychain

import (
	"context"
	"errors"
	"testing"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"github.com/rustic-ai/forge/forge-go/oauth"
	"github.com/rustic-ai/forge/forge-go/secrets"
	"github.com/zalando/go-keyring"
)

func TestSecretProvider_PlainSecret(t *testing.T) {
	keyring.MockInit()

	if err := keyring.Set(forgepath.AppNamespace(), "MY_API_KEY", "sk-abc123"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := NewSecretProvider()
	val, err := p.Resolve(context.Background(), "MY_API_KEY")
	if err != nil {
		t.Fatalf("expected plain secret, got error: %v", err)
	}
	if val != "sk-abc123" {
		t.Errorf("expected sk-abc123, got %s", val)
	}
}

func TestSecretProvider_OAuthToken(t *testing.T) {
	keyring.MockInit()

	mgr := oauth.NewManager(&oauth.ProvidersConfig{})
	mgr.SeedToken("org1", "github", "ghp_test456")
	SetOAuthManager(mgr)
	t.Cleanup(func() { SetOAuthManager(nil) })

	p := NewSecretProvider()
	val, err := p.Resolve(context.Background(), "oauth:org1|github")
	if err != nil {
		t.Fatalf("expected OAuth token, got error: %v", err)
	}
	if val != "ghp_test456" {
		t.Errorf("expected ghp_test456, got %s", val)
	}
}

func TestSecretProvider_NotFound(t *testing.T) {
	keyring.MockInit()

	p := NewSecretProvider()
	_, err := p.Resolve(context.Background(), "NONEXISTENT")
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestSecretProvider_UserIsolation(t *testing.T) {
	keyring.MockInit()

	mgr := oauth.NewManager(&oauth.ProvidersConfig{})
	mgr.SeedToken("alice", "github", "token-alice")
	mgr.SeedToken("bob", "github", "token-bob")
	SetOAuthManager(mgr)
	t.Cleanup(func() { SetOAuthManager(nil) })

	p := NewSecretProvider()
	ctx := context.Background()

	for _, tc := range []struct{ key, want string }{
		{"oauth:alice|github", "token-alice"},
		{"oauth:bob|github", "token-bob"},
	} {
		val, err := p.Resolve(ctx, tc.key)
		if err != nil || val != tc.want {
			t.Errorf("key %s: expected %s, got %q err=%v", tc.key, tc.want, val, err)
		}
	}
}

func TestSecretProvider_NonOAuthJSONReturnedRaw(t *testing.T) {
	keyring.MockInit()

	raw := `{"some_field":"some_value"}`
	if err := keyring.Set(forgepath.AppNamespace(), "JSON_SECRET", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := NewSecretProvider()
	val, err := p.Resolve(context.Background(), "JSON_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != raw {
		t.Errorf("expected raw JSON, got %s", val)
	}
}

func TestSecretProvider_InDefaultProviderChain(t *testing.T) {
	keyring.MockInit()
	t.Setenv("FORGE_SECRET_PROVIDERS", "keychain")

	if err := keyring.Set(forgepath.AppNamespace(), "CHAIN_KEY", "chain_value"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	provider := secrets.DefaultProvider()
	val, err := provider.Resolve(context.Background(), "CHAIN_KEY")
	if err != nil {
		t.Fatalf("expected DefaultProvider to resolve from keychain, got: %v", err)
	}
	if val != "chain_value" {
		t.Errorf("expected chain_value, got %s", val)
	}
}
