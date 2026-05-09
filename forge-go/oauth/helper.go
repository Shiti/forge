package oauth

import (
	"strings"
)

const prefix = "oauth:"

func StoreKey(orgID, providerID string) string {
	return prefix + orgID + "|" + providerID
}

// ParseOAuthKey parses an "oauth:orgID|providerID" key. Returns ok=false for non-OAuth keys.
func ParseOAuthKey(key string) (orgID, providerID string, ok bool) {
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
