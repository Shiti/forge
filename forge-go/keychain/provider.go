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
package keychain

import (
	"context"
	"errors"

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

func (p *SecretProvider) Resolve(ctx context.Context, key string) (string, error) {
	// use OAuthMgr directly so that it refreshes tokens if needed
	if userID, providerID, ok := oauth.ParseOAuthKey(key); ok && oauthMgr != nil {
		token, err := oauthMgr.GetAccessToken(ctx, userID, providerID)
		if errors.Is(err, oauth.ErrNotConnected) {
			return "", secrets.ErrSecretNotFound
		}
		return token, err
	}

	raw, err := keyring.Get(p.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", secrets.ErrSecretNotFound
		}
		return "", err
	}
	return raw, nil
}

func init() {
	secrets.RegisterProvider("keychain", func() secrets.SecretProvider {
		return NewSecretProvider()
	})
}
