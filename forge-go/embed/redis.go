package embed

import (
	"fmt"
	"strings"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// EmbeddedRedis wraps miniredis to provide a local Redis server.
type EmbeddedRedis struct {
	mr *miniredis.Miniredis
}

// StartEmbeddedRedis spins up a new miniredis instance on an ephemeral port.
func StartEmbeddedRedis() (*EmbeddedRedis, error) {
	return StartEmbeddedRedisAt("")
}

// StartEmbeddedRedisAt spins up a new miniredis instance on a specific address.
// If addr is empty, an ephemeral port is used.
func StartEmbeddedRedisAt(addr string) (*EmbeddedRedis, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		mr, err := miniredis.Run()
		if err != nil {
			return nil, fmt.Errorf("failed to start embedded redis: %w", err)
		}
		return &EmbeddedRedis{mr: mr}, nil
	}

	mr := miniredis.NewMiniRedis()
	if err := mr.StartAddr(addr); err != nil {
		return nil, fmt.Errorf("failed to start embedded redis at %s: %w", addr, err)
	}
	return &EmbeddedRedis{mr: mr}, nil
}

// Host returns the bound hostname.
func (e *EmbeddedRedis) Host() string {
	return e.mr.Host()
}

// Port returns the bound port as a string.
func (e *EmbeddedRedis) Port() string {
	return e.mr.Port()
}

// Addr returns host:port.
func (e *EmbeddedRedis) Addr() string {
	return e.mr.Addr()
}

// Client returns a go-redis client connected to this instance.
func (e *EmbeddedRedis) Client() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: e.mr.Addr(),
	})
}

// Close terminates the embedded instance.
func (e *EmbeddedRedis) Close() {
	if e.mr != nil {
		e.mr.Close()
	}
}
