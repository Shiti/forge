package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisAgentStatusStore implements AgentStatusStore using Redis SET/GET/EXPIRE/DEL.
type RedisAgentStatusStore struct {
	rdb *redis.Client
}

// NewRedisAgentStatusStore creates a new Redis-backed agent status store.
func NewRedisAgentStatusStore(rdb *redis.Client) *RedisAgentStatusStore {
	return &RedisAgentStatusStore{rdb: rdb}
}

func (s *RedisAgentStatusStore) WriteStatus(ctx context.Context, guildID, agentID string, status *AgentStatusJSON, ttl time.Duration) error {
	key := statusKey(guildID, agentID)
	b, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal agent status: %w", err)
	}
	return s.rdb.Set(ctx, key, b, ttl).Err()
}

func (s *RedisAgentStatusStore) RefreshStatus(ctx context.Context, guildID, agentID string, ttl time.Duration) error {
	key := statusKey(guildID, agentID)
	return s.rdb.Expire(ctx, key, ttl).Err()
}

func (s *RedisAgentStatusStore) GetStatus(ctx context.Context, guildID, agentID string) (*AgentStatusJSON, error) {
	key := statusKey(guildID, agentID)
	raw, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var status AgentStatusJSON
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}
	return &status, nil
}

func (s *RedisAgentStatusStore) DeleteStatus(ctx context.Context, guildID, agentID string) error {
	key := statusKey(guildID, agentID)
	return s.rdb.Del(ctx, key).Err()
}

func statusKey(guildID, agentID string) string {
	return fmt.Sprintf("forge:agent:status:%s:%s", guildID, agentID)
}
