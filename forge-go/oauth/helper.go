package oauth

import (
	"strings"
)

const prefix = "oauth:"

func StoreKey(userID, providerID string) string {
	return prefix + userID + "|" + providerID
}

// ParseOAuthKey parses an "oauth:userID|providerID" key. Returns ok=false for non-OAuth keys.
func ParseOAuthKey(key string) (userID, providerID string, ok bool) {
	rest, found := strings.CutPrefix(key, prefix)
	if !found {
		return "", "", false
	}
	idx := strings.Index(rest, "|")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}
