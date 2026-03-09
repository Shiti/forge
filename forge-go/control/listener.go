package control

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	// ControlQueueRequestKey is the Redis list that holds incoming control requests
	ControlQueueRequestKey = "forge:control:requests"
)

// ControlMessageWrapper represents the raw JSON packet pushed to the queue
type ControlMessageWrapper struct {
	Command string          `json:"command"`
	Payload json.RawMessage `json:"payload"`
}

// ControlQueueListener listens for and dispatches incoming control requests from Redis.
type ControlQueueListener struct {
	rdb             *redis.Client
	requestQueueKey string
	stopCh          chan struct{}
	OnSpawn         func(ctx context.Context, req *protocol.SpawnRequest)
	OnStop          func(ctx context.Context, req *protocol.StopRequest)
}

func NewControlQueueListener(rdb *redis.Client) *ControlQueueListener {
	return NewControlQueueListenerWithQueue(rdb, ControlQueueRequestKey)
}

func NewControlQueueListenerWithQueue(rdb *redis.Client, queueKey string) *ControlQueueListener {
	if queueKey == "" {
		queueKey = ControlQueueRequestKey
	}
	return &ControlQueueListener{
		rdb:             rdb,
		requestQueueKey: queueKey,
		stopCh:          make(chan struct{}),
	}
}

func (l *ControlQueueListener) Start(ctx context.Context) {
	for {
		select {
		case <-l.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			if depth, err := l.rdb.LLen(ctx, l.requestQueueKey).Result(); err == nil {
				telemetry.QueueDepth.WithLabelValues(l.requestQueueKey).Set(float64(depth))
			}

			res, err := l.rdb.BRPop(ctx, time.Second, l.requestQueueKey).Result()
			if err != nil {
				if err == redis.Nil {
					continue
				}
				if err == redis.ErrClosed {
					return
				}
				slog.Error("ControlQueueListener error reading queue", "err", err)
				continue
			}

			if len(res) < 2 {
				continue
			}

			var wrapper ControlMessageWrapper
			if err := json.Unmarshal([]byte(res[1]), &wrapper); err != nil {
				slog.Error("ControlQueueListener failed to parse wrapper", "err", err)
				continue
			}

			switch wrapper.Command {
			case "spawn":
				if l.OnSpawn != nil {
					var req protocol.SpawnRequest
					if err := json.Unmarshal(wrapper.Payload, &req); err == nil {
						propagator := otel.GetTextMapPropagator()
						spanCtx := propagator.Extract(ctx, propagation.MapCarrier(req.TraceContext))
						spanCtx, span := otel.Tracer("forge.control").Start(spanCtx, "redis.consume")
						span.End()

						telemetry.QueueConsumeTotal.WithLabelValues(l.requestQueueKey, "spawn").Inc()
						go l.OnSpawn(spanCtx, &req)
					} else {
						telemetry.QueueProcessingErrorsTotal.WithLabelValues(l.requestQueueKey, "spawn", "json_unmarshal").Inc()
						slog.Error("ControlQueueListener failed to parse SpawnRequest payload", "err", err)
					}
				}
			case "stop":
				if l.OnStop != nil {
					var req protocol.StopRequest
					if err := json.Unmarshal(wrapper.Payload, &req); err == nil {
						telemetry.QueueConsumeTotal.WithLabelValues(l.requestQueueKey, "stop").Inc()
						go l.OnStop(ctx, &req)
					} else {
						telemetry.QueueProcessingErrorsTotal.WithLabelValues(l.requestQueueKey, "stop", "json_unmarshal").Inc()
						slog.Error("ControlQueueListener failed to parse StopRequest payload", "err", err)
					}
				}
			default:
				slog.Warn("ControlQueueListener unknown command received", "command", wrapper.Command)
			}
		}
	}
}

func (l *ControlQueueListener) Stop() {
	close(l.stopCh)
}
