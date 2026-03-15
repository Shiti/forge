package control

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	// ControlQueueRequestKey is the queue key for incoming control requests.
	ControlQueueRequestKey = "forge:control:requests"
)

// ControlMessageWrapper represents the raw JSON packet pushed to the queue.
type ControlMessageWrapper struct {
	Command string          `json:"command"`
	Payload json.RawMessage `json:"payload"`
}

// ControlQueueListener listens for and dispatches incoming control requests.
type ControlQueueListener struct {
	transport       ControlTransport
	requestQueueKey string
	stopCh          chan struct{}
	OnSpawn         func(ctx context.Context, req *protocol.SpawnRequest)
	OnStop          func(ctx context.Context, req *protocol.StopRequest)
}

// NewControlQueueListener creates a listener on the default control queue.
func NewControlQueueListener(transport ControlTransport) *ControlQueueListener {
	return NewControlQueueListenerWithQueue(transport, ControlQueueRequestKey)
}

// NewControlQueueListenerWithQueue creates a listener on the specified queue key.
func NewControlQueueListenerWithQueue(transport ControlTransport, queueKey string) *ControlQueueListener {
	if queueKey == "" {
		queueKey = ControlQueueRequestKey
	}
	return &ControlQueueListener{
		transport:       transport,
		requestQueueKey: queueKey,
		stopCh:          make(chan struct{}),
	}
}

// Start begins the polling loop; blocks until Stop() or ctx cancellation.
func (l *ControlQueueListener) Start(ctx context.Context) {
	for {
		select {
		case <-l.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			if depth, err := l.transport.QueueDepth(ctx, l.requestQueueKey); err == nil {
				telemetry.QueueDepth.WithLabelValues(l.requestQueueKey).Set(float64(depth))
			}

			data, err := l.transport.Pop(ctx, l.requestQueueKey, time.Second)
			if err != nil {
				slog.Error("ControlQueueListener error reading queue", "err", err)
				continue
			}
			if data == nil {
				// timeout — loop and check stopCh/ctx again
				continue
			}

			var wrapper ControlMessageWrapper
			if err := json.Unmarshal(data, &wrapper); err != nil {
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
						spanCtx, span := otel.Tracer("forge.control").Start(spanCtx, "queue.consume")
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

// Stop signals the listener loop to exit.
func (l *ControlQueueListener) Stop() {
	close(l.stopCh)
}
