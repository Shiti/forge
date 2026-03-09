package supervisor

import "strings"

const unknownGuildKey = "unknown-guild"

func normalizeGuildID(guildID string) string {
	if strings.TrimSpace(guildID) == "" {
		return unknownGuildKey
	}
	return guildID
}

func scopedAgentKey(guildID, agentID string) string {
	return normalizeGuildID(guildID) + "::" + agentID
}
