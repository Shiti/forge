package messaging

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultMessageTTL is the default time-to-live for cached messages in Redis.
const DefaultMessageTTL = 3600 * time.Second

// Config holds the configuration options for the messaging client.
type Config struct {
	// MessageTTL defines how long raw messages are retained in the direct-lookup cache.
	MessageTTL time.Duration
}

// DefaultConfig loads configuration from environment variables, establishing the
// same defaults as the Python RUSTIC_AI_REDIS_MSG_TTL environment lookup.
func DefaultConfig() Config {
	ttl := DefaultMessageTTL

	if val := os.Getenv("RUSTIC_AI_REDIS_MSG_TTL"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			ttl = time.Duration(parsed) * time.Second
		} else {
			slog.Warn("Invalid RUSTIC_AI_REDIS_MSG_TTL, falling back to default", "val", val, "default", DefaultMessageTTL.Seconds())
		}
	}

	return Config{
		MessageTTL: ttl,
	}
}

// RedisBackend encapsulates the Redis connection and configuration for messaging operations.
type RedisBackend struct {
	rdb    *redis.Client
	config Config
}

// Compile-time check that RedisBackend satisfies the Backend interface.
var _ Backend = (*RedisBackend)(nil)

// NewRedisBackend creates a new RedisBackend using the provided Redis connection.
func NewRedisBackend(rdb *redis.Client) *RedisBackend {
	return &RedisBackend{
		rdb:    rdb,
		config: DefaultConfig(),
	}
}

// NewRedisBackendWithConfig creates a new RedisBackend with explicit configuration.
func NewRedisBackendWithConfig(rdb *redis.Client, config Config) *RedisBackend {
	return &RedisBackend{
		rdb:    rdb,
		config: config,
	}
}

// Close is a no-op for Redis — the connection is externally managed.
func (r *RedisBackend) Close() error {
	return nil
}

// Backward-compat aliases so existing callers of messaging.NewClient keep compiling.

// Client is an alias for RedisBackend.
type Client = RedisBackend

// NewClient creates a new RedisBackend (alias kept for backward compatibility).
func NewClient(rdb *redis.Client) *RedisBackend {
	return NewRedisBackend(rdb)
}

// NewClientWithConfig creates a new RedisBackend with explicit config (alias kept for backward compatibility).
func NewClientWithConfig(rdb *redis.Client, config Config) *RedisBackend {
	return NewRedisBackendWithConfig(rdb, config)
}
