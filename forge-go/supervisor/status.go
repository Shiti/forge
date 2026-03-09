package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// AgentStatusJSON defines the JSON payload written to the Redis status key
type AgentStatusJSON struct {
	State     string    `json:"state"`
	NodeID    string    `json:"node_id,omitempty"`
	PID       int       `json:"pid,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// WriteStatusKey writes the initial "running" status for an agent with a 30s TTL
func WriteStatusKey(ctx context.Context, rdb *redis.Client, guildID, agentID, nodeID string, pid int) error {
	status := AgentStatusJSON{
		State:     "running",
		NodeID:    nodeID,
		PID:       pid,
		Timestamp: time.Now(),
	}

	return setStatus(ctx, rdb, guildID, agentID, &status, 30*time.Second)
}

// RefreshStatusKey resets the TTL on an existing status key to 30s.
func RefreshStatusKey(ctx context.Context, rdb *redis.Client, guildID, agentID string) error {
	key := fmt.Sprintf("forge:agent:status:%s:%s", guildID, agentID)
	return rdb.Expire(ctx, key, 30*time.Second).Err()
}

// SetRestartingStatus explicitly sets the status payload to "restarting" with a 30s TTL
func SetRestartingStatus(ctx context.Context, rdb *redis.Client, guildID, agentID string) error {
	status := AgentStatusJSON{
		State:     "restarting",
		Timestamp: time.Now(),
	}
	return setStatus(ctx, rdb, guildID, agentID, &status, 30*time.Second)
}

// SetFailedStatus explicitly sets the status payload to "failed" with a 300s TTL for diagnostics
func SetFailedStatus(ctx context.Context, rdb *redis.Client, guildID, agentID string) error {
	status := AgentStatusJSON{
		State:     "failed",
		Timestamp: time.Now(),
	}
	return setStatus(ctx, rdb, guildID, agentID, &status, 300*time.Second)
}

// DeleteStatusKey removes the agent's status key from Redis immediately upon graceful Stop
func DeleteStatusKey(ctx context.Context, rdb *redis.Client, guildID, agentID string) error {
	key := fmt.Sprintf("forge:agent:status:%s:%s", guildID, agentID)
	return rdb.Del(ctx, key).Err()
}

func setStatus(ctx context.Context, rdb *redis.Client, guildID, agentID string, status *AgentStatusJSON, ttl time.Duration) error {
	key := fmt.Sprintf("forge:agent:status:%s:%s", guildID, agentID)

	b, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal agent status: %w", err)
	}

	return rdb.Set(ctx, key, b, ttl).Err()
}
