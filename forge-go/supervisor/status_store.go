package supervisor

import (
	"context"
	"time"
)

// AgentStatusStore defines the interface for persisting and querying agent run-time status.
type AgentStatusStore interface {
	// WriteStatus writes (or overwrites) the agent's status with the given TTL.
	WriteStatus(ctx context.Context, guildID, agentID string, status *AgentStatusJSON, ttl time.Duration) error
	// RefreshStatus resets the TTL on an existing status entry without changing its value.
	RefreshStatus(ctx context.Context, guildID, agentID string, ttl time.Duration) error
	// GetStatus retrieves the current status for an agent. Returns (nil, nil) if not found.
	GetStatus(ctx context.Context, guildID, agentID string) (*AgentStatusJSON, error)
	// DeleteStatus removes the agent's status entry immediately.
	DeleteStatus(ctx context.Context, guildID, agentID string) error
}
