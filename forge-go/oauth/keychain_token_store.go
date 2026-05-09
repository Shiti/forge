package oauth

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// storedEntry is the JSON-serializable form of tokenEntry for keychain persistence.
type storedEntry struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	AuthURL      string    `json:"auth_url"`
	TokenURL     string    `json:"token_url"`
	AuthStyle    int       `json:"auth_style"`
	Scopes       []string  `json:"scopes"`
}

func toStoredEntry(e *tokenEntry) *storedEntry {
	return &storedEntry{
		AccessToken:  e.token.AccessToken,
		TokenType:    e.token.TokenType,
		RefreshToken: e.token.RefreshToken,
		Expiry:       e.token.Expiry,
		ClientID:     e.clientID,
		ClientSecret: e.clientSecret,
		AuthURL:      e.endpoint.AuthURL,
		TokenURL:     e.endpoint.TokenURL,
		AuthStyle:    int(e.endpoint.AuthStyle),
		Scopes:       e.scopes,
	}
}

func fromStoredEntry(s *storedEntry) *tokenEntry {
	return &tokenEntry{
		token: &oauth2.Token{
			AccessToken:  s.AccessToken,
			TokenType:    s.TokenType,
			RefreshToken: s.RefreshToken,
			Expiry:       s.Expiry,
		},
		clientID:     s.ClientID,
		clientSecret: s.ClientSecret,
		endpoint: oauth2.Endpoint{
			AuthURL:   s.AuthURL,
			TokenURL:  s.TokenURL,
			AuthStyle: oauth2.AuthStyle(s.AuthStyle),
		},
		scopes: s.Scopes,
	}
}

func keychainIndexAccount(orgID string) string {
	return orgID + "|__index__"
}

// KeychainTokenStore persists OAuth tokens in the OS keychain (macOS Keychain,
// Windows Credential Manager, Linux Secret Service via libsecret).
//
// Set FORGE_OAUTH_TOKEN_STORE=keychain to activate this backend.
type KeychainTokenStore struct {
	service string
}

func NewKeychainTokenStore() *KeychainTokenStore {
	return NewKeychainTokenStoreWithService(forgepath.AppNamespace())
}

func NewKeychainTokenStoreWithService(service string) *KeychainTokenStore {
	return &KeychainTokenStore{service: service}
}

func (s *KeychainTokenStore) Save(orgID, providerID string, entry *tokenEntry) error {
	data, err := json.Marshal(toStoredEntry(entry))
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}
	if err := keyring.Set(s.service, StoreKey(orgID, providerID), string(data)); err != nil {
		return fmt.Errorf("saving to keychain: %w", err)
	}
	return s.addToIndex(orgID, providerID)
}

func (s *KeychainTokenStore) Load(orgID, providerID string) (*tokenEntry, bool) {
	data, err := keyring.Get(s.service, StoreKey(orgID, providerID))
	if err != nil {
		return nil, false
	}
	var se storedEntry
	if err := json.Unmarshal([]byte(data), &se); err != nil {
		return nil, false
	}
	return fromStoredEntry(&se), true
}

func (s *KeychainTokenStore) Delete(orgID, providerID string) bool {
	if err := keyring.Delete(s.service, StoreKey(orgID, providerID)); err != nil {
		return false
	}
	_ = s.removeFromIndex(orgID, providerID)
	return true
}

func (s *KeychainTokenStore) LoadAllForOrg(orgID string) map[string]*tokenEntry {
	providers := s.readIndex(orgID)
	out := make(map[string]*tokenEntry, len(providers))
	for _, pid := range providers {
		if e, ok := s.Load(orgID, pid); ok {
			out[pid] = e
		}
	}
	return out
}

func (s *KeychainTokenStore) readIndex(orgID string) []string {
	data, err := keyring.Get(s.service, keychainIndexAccount(orgID))
	if err != nil || data == "" {
		return nil
	}
	return strings.Split(data, "\n")
}

func (s *KeychainTokenStore) writeIndex(orgID string, providers []string) error {
	return keyring.Set(s.service, keychainIndexAccount(orgID), strings.Join(providers, "\n"))
}

func (s *KeychainTokenStore) addToIndex(orgID, providerID string) error {
	existing := s.readIndex(orgID)
	for _, p := range existing {
		if p == providerID {
			return nil
		}
	}
	return s.writeIndex(orgID, append(existing, providerID))
}

func (s *KeychainTokenStore) removeFromIndex(orgID, providerID string) error {
	existing := s.readIndex(orgID)
	updated := existing[:0]
	for _, p := range existing {
		if p != providerID {
			updated = append(updated, p)
		}
	}
	return s.writeIndex(orgID, updated)
}
