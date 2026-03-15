package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rustic-ai/forge/forge-go/messaging"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

func RetrieveHistory(ctx context.Context, msgClient messaging.Backend, guildID, userID string, sinceID uint64) ([]json.RawMessage, error) {
	userHist, err := msgClient.GetMessagesSince(ctx, guildID, userNotificationsTopic(userID), sinceID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching user history: %w", err)
	}

	broadcastHist, err := msgClient.GetMessagesSince(ctx, guildID, userMessageBroadcast, sinceID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching broadcast history: %w", err)
	}

	seen := make(map[uint64]bool)
	var merged []protocol.Message

	for _, m := range append(userHist, broadcastHist...) {
		if !seen[m.ID] {
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		mI, errI := protocol.ParseGemstoneID(merged[i].ID)
		mJ, errJ := protocol.ParseGemstoneID(merged[j].ID)
		if errI != nil || errJ != nil {
			return merged[i].ID < merged[j].ID
		}
		return protocol.Compare(mI, mJ) < 0
	})

	var final []json.RawMessage
	for _, m := range merged {
		b, _ := json.Marshal(m)
		final = append(final, json.RawMessage(b))
	}

	return final, nil
}
