package leader

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisElector struct {
	client     *redis.Client
	nodeID     string
	key        string
	ttl        time.Duration
	isLeader   atomic.Bool
	watchChan  chan bool
	cancelFunc context.CancelFunc // To stop the renewal loop
}

var _ LeaderElector = (*RedisElector)(nil)

func NewRedisElector(client *redis.Client, nodeID string, key string, ttl time.Duration) *RedisElector {
	return &RedisElector{
		client:    client,
		nodeID:    nodeID,
		key:       key,
		ttl:       ttl,
		watchChan: make(chan bool, 1),
	}
}

func (r *RedisElector) Acquire(ctx context.Context) error {
	// Immediate first attempt
	ok, err := r.client.SetNX(ctx, r.key, r.nodeID, r.ttl).Result()
	if err == nil && ok {
		r.startHeartbeat()
		r.setLeader(true)
		return nil
	}

	ticker := time.NewTicker(r.ttl / 2) // Attempt to acquire frequently
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := r.client.SetNX(ctx, r.key, r.nodeID, r.ttl).Result()
			if err != nil {
				slog.Error("RedisElector: fail to try lock", "err", err)
				continue
			}

			if ok {
				// We got the lock!
				r.startHeartbeat()
				r.setLeader(true)
				return nil
			}
		}
	}
}

func (r *RedisElector) startHeartbeat() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancelFunc = cancel

	go func() {
		ticker := time.NewTicker(r.ttl / 3) // Renew more frequently than expiration
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Extend the lock using an eval script to ensure we only extend our own lock
				script := `
					if redis.call("get", KEYS[1]) == ARGV[1] then
						return redis.call("pexpire", KEYS[1], ARGV[2])
					else
						return 0
					end
				`
				res, err := r.client.Eval(ctx, script, []string{r.key}, r.nodeID, r.ttl.Milliseconds()).Result()
				if err != nil {
					slog.Error("RedisElector: heartbeat error", "err", err)
					// We might lose leadership if network partition
					r.setLeader(false)
					return
				}

				extended, ok := res.(int64)
				if !ok || extended == 0 {
					slog.Warn("RedisElector: failed to extend lock, another node may have it")
					r.setLeader(false)
					return
				}
			}
		}
	}()
}

func (r *RedisElector) IsLeader() bool {
	return r.isLeader.Load()
}

func (r *RedisElector) Resign(ctx context.Context) error {
	if !r.IsLeader() {
		return errors.New("not the leader")
	}

	// Stop heartbeat
	if r.cancelFunc != nil {
		r.cancelFunc()
	}

	// Delete lock cleanly
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	_, err := r.client.Eval(ctx, script, []string{r.key}, r.nodeID).Result()

	r.setLeader(false)
	return err
}

func (r *RedisElector) setLeader(state bool) {
	if r.isLeader.CompareAndSwap(!state, state) {
		select {
		case r.watchChan <- state:
		default:
			// Overwrite if full
			<-r.watchChan
			r.watchChan <- state
		}
	}
}

func (r *RedisElector) Watch() <-chan bool {
	return r.watchChan
}
