// Package keychain provides a secrets.SecretProvider backed by the OS keychain
// (macOS Keychain, Windows Credential Manager, Linux Secret Service).
//
// This provider is value-aware: it inspects each stored value and extracts the
// meaningful secret based on the detected type (OAuth access token, plain
// string, …). A single keychain service can therefore hold heterogeneous
// secrets without the caller needing to know how each one was stored.
//
// The key passed to Resolve is used directly as the keychain account name.
// For OAuth tokens stored by the OAuth token store, the key must include the
// userID prefix: "userID|providerID" (e.g. "alice|github").
//
package keychain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"github.com/rustic-ai/forge/forge-go/oauth"
	"github.com/rustic-ai/forge/forge-go/secrets"
	"github.com/zalando/go-keyring"
)

// oauthMgr is the OAuth manager used to refresh expired tokens. Set via SetOAuthManager.
var oauthMgr *oauth.Manager

// SetOAuthManager registers the OAuth manager for token refresh. Call once at server startup.
func SetOAuthManager(m *oauth.Manager) {
	oauthMgr = m
}

// SecretProvider resolves secrets from the OS keychain. The key is used
// directly as the keychain account name. Stored values are inspected to
// extract the canonical secret string:
//   - OAuth token JSON  → access_token field
//   - Plain string      → returned as-is
//   - (Future: PEM cert → public key or fingerprint, etc.)
type SecretProvider struct {
	service string
}

func NewSecretProvider() *SecretProvider {
	return &SecretProvider{service: forgepath.AppNamespace()}
}

func NewSecretProviderWithService(service string) *SecretProvider {
	return &SecretProvider{service: service}
}

// parseOAuthKey parses an "oauth:userID|providerID" key. Returns ok=false for non-OAuth keys.
func parseOAuthKey(key string) (userID, providerID string, ok bool) {
	rest, found := strings.CutPrefix(key, "oauth:")
	if !found {
		return "", "", false
	}
	idx := strings.Index(rest, "|")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// parseOAuthEntry parses raw as an OAuth token JSON. Returns the access token
// and whether it needs refresh (expiring within 60s with a refresh token present).
// Returns ("", false) if raw is not OAuth token JSON.
func parseOAuthEntry(raw string) (accessToken string, needsRefresh bool) {
	var entry struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		Expiry       time.Time `json:"expiry"`
	}
	if err := json.Unmarshal([]byte(raw), &entry); err != nil || entry.AccessToken == "" {
		return "", false
	}
	expired := entry.RefreshToken != "" && !entry.Expiry.IsZero() && time.Until(entry.Expiry) <= 60*time.Second
	return entry.AccessToken, expired
}

func (p *SecretProvider) Resolve(ctx context.Context, key string) (string, error) {
	raw, err := keyring.Get(p.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", secrets.ErrSecretNotFound
		}
		return "", err
	}

	if userID, providerID, ok := parseOAuthKey(key); ok {
		if token, needsRefresh := parseOAuthEntry(raw); token != "" {
			if needsRefresh && oauthMgr != nil {
				return oauthMgr.GetAccessToken(ctx, userID, providerID)
			}
			return token, nil
		}
	}

	return raw, nil
}

func init() {
	secrets.RegisterProvider("keychain", func() secrets.SecretProvider {
		return NewSecretProvider()
	})
}
