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

// Client encapsulates the Redis connection and configuration for the Gateway messaging operations.
type Client struct {
	rdb    *redis.Client
	config Config
}

// NewClient creates a new messaging Client using the provided Redis connection.
// It automatically loads configuration from DefaultConfig().
func NewClient(rdb *redis.Client) *Client {
	return &Client{
		rdb:    rdb,
		config: DefaultConfig(),
	}
}

// NewClientWithConfig creates a new messaging Client with explicit configuration.
func NewClientWithConfig(rdb *redis.Client, config Config) *Client {
	return &Client{
		rdb:    rdb,
		config: config,
	}
}
