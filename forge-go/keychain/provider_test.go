package keychain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rustic-ai/forge/forge-go/forgepath"
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

	entry := map[string]any{
		"access_token":  "ghp_test456",
		"token_type":    "Bearer",
		"refresh_token": "refresh_xyz",
		"expiry":        time.Now().Add(time.Hour),
	}
	data, _ := json.Marshal(entry)
	if err := keyring.Set(forgepath.AppNamespace(), "oauth:user1|github", string(data)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := NewSecretProvider()
	val, err := p.Resolve(context.Background(), "oauth:user1|github")
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

	for _, tc := range []struct{ user, token string }{
		{"alice", "token-alice"},
		{"bob", "token-bob"},
	} {
		entry := map[string]any{"access_token": tc.token}
		data, _ := json.Marshal(entry)
		if err := keyring.Set(forgepath.AppNamespace(), "oauth:"+tc.user+"|github", string(data)); err != nil {
			t.Fatalf("seed %s: %v", tc.user, err)
		}
	}

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
