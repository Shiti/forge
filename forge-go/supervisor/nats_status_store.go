package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const agentStatusKVBucket = "agent-status"

// NATSAgentStatusStore implements AgentStatusStore using a NATS JetStream KV bucket.
// A single bucket "agent-status" with TTL 300s (max needed) is used for all guilds/agents.
// Running agents (30s desired) rely on the 10s refresh cycle to keep keys alive.
type NATSAgentStatusStore struct {
	js nats.JetStreamContext
	kv nats.KeyValue
}

var _ AgentStatusStore = (*NATSAgentStatusStore)(nil)

// NewNATSAgentStatusStore creates a NATSAgentStatusStore, ensuring the KV bucket exists.
func NewNATSAgentStatusStore(nc *nats.Conn) (*NATSAgentStatusStore, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("supervisor: failed to get JetStream context: %w", err)
	}
	const maxTTL = 300 * time.Second
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: agentStatusKVBucket,
		TTL:    maxTTL,
	})
	if err != nil {
		// Bucket may already exist — try to bind.
		kv, err = js.KeyValue(agentStatusKVBucket)
		if err != nil {
			return nil, fmt.Errorf("supervisor: failed to ensure KV bucket %q: %w", agentStatusKVBucket, err)
		}
	}
	return &NATSAgentStatusStore{js: js, kv: kv}, nil
}

// statusKVKey builds the KV key for guildID+agentID, sanitizing each component.
func statusKVKey(guildID, agentID string) string {
	return kvSanitize(guildID) + "." + kvSanitize(agentID)
}

// kvSanitize replaces any character not valid in a NATS KV key component with underscore.
// NATS KV keys allow only alphanumeric, '-', '_', '.'.
func kvSanitize(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, name)
}

// WriteStatus serialises status to JSON and stores it in the KV bucket.
// The ttl parameter is accepted for interface compatibility; actual TTL is governed by bucket MaxAge.
func (s *NATSAgentStatusStore) WriteStatus(ctx context.Context, guildID, agentID string, status *AgentStatusJSON, ttl time.Duration) error {
	b, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("supervisor: failed to marshal status: %w", err)
	}
	_, err = s.kv.Put(statusKVKey(guildID, agentID), b)
	return err
}

// RefreshStatus re-puts the existing value to reset the KV entry age.
func (s *NATSAgentStatusStore) RefreshStatus(ctx context.Context, guildID, agentID string, ttl time.Duration) error {
	entry, err := s.kv.Get(statusKVKey(guildID, agentID))
	if err == nats.ErrKeyNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.kv.Put(statusKVKey(guildID, agentID), entry.Value())
	return err
}

// GetStatus returns the parsed AgentStatusJSON for an agent, or (nil, nil) if not found.
func (s *NATSAgentStatusStore) GetStatus(ctx context.Context, guildID, agentID string) (*AgentStatusJSON, error) {
	entry, err := s.kv.Get(statusKVKey(guildID, agentID))
	if err == nats.ErrKeyNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var status AgentStatusJSON
	if err := json.Unmarshal(entry.Value(), &status); err != nil {
		return nil, fmt.Errorf("supervisor: failed to unmarshal status: %w", err)
	}
	return &status, nil
}

// DeleteStatus removes the status entry for an agent.
func (s *NATSAgentStatusStore) DeleteStatus(ctx context.Context, guildID, agentID string) error {
	err := s.kv.Delete(statusKVKey(guildID, agentID))
	if err == nats.ErrKeyNotFound {
		return nil
	}
	return err
}
