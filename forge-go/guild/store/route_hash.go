package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

// RoutingRuleHash mirrors rustic-ai RoutingRule.hashid:
// - model dump with exclude_none
// - excludes route_times, process_status, reason
// - deterministic JSON + sha256 hex digest
func RoutingRuleHash(rule *protocol.RoutingRule) (string, error) {
	if rule == nil {
		return "", fmt.Errorf("routing rule is required")
	}

	serialized, err := json.Marshal(rule)
	if err != nil {
		return "", fmt.Errorf("marshal routing rule: %w", err)
	}

	var canonical map[string]interface{}
	if err := json.Unmarshal(serialized, &canonical); err != nil {
		return "", fmt.Errorf("unmarshal routing rule: %w", err)
	}

	delete(canonical, "route_times")
	delete(canonical, "process_status")
	delete(canonical, "reason")

	pruneNilRecursive(canonical)

	canonicalBytes, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal canonical routing rule: %w", err)
	}

	sum := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(sum[:]), nil
}

func pruneNilRecursive(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, item := range t {
			pruned := pruneNilRecursive(item)
			if pruned == nil {
				delete(t, k)
				continue
			}
			t[k] = pruned
		}
		return t
	case []interface{}:
		out := make([]interface{}, 0, len(t))
		for _, item := range t {
			pruned := pruneNilRecursive(item)
			if pruned != nil {
				out = append(out, pruned)
			}
		}
		return out
	default:
		return v
	}
}
