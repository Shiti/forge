package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisControlTransport implements ControlPlane using Redis lists.
type RedisControlTransport struct {
	rdb *redis.Client
}

// NewRedisControlTransport creates a new Redis-backed control transport.
func NewRedisControlTransport(rdb *redis.Client) *RedisControlTransport {
	return &RedisControlTransport{rdb: rdb}
}

func (t *RedisControlTransport) Push(ctx context.Context, queueKey string, payload []byte) error {
	return t.rdb.LPush(ctx, queueKey, payload).Err()
}

func (t *RedisControlTransport) Pop(ctx context.Context, queueKey string, timeout time.Duration) ([]byte, error) {
	res, err := t.rdb.BRPop(ctx, timeout, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	if len(res) < 2 {
		return nil, nil
	}
	return []byte(res[1]), nil
}

func (t *RedisControlTransport) QueueDepth(ctx context.Context, queueKey string) (int64, error) {
	return t.rdb.LLen(ctx, queueKey).Result()
}

func (t *RedisControlTransport) PushResponse(ctx context.Context, requestID string, payload []byte, ttl time.Duration) error {
	key := fmt.Sprintf("forge:control:response:%s", requestID)
	pipe := t.rdb.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (t *RedisControlTransport) WaitResponse(ctx context.Context, requestID string, timeout time.Duration) ([]byte, error) {
	key := fmt.Sprintf("forge:control:response:%s", requestID)
	res, err := t.rdb.BRPop(ctx, timeout, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	if len(res) < 2 {
		return nil, nil
	}
	return []byte(res[1]), nil
}
