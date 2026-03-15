package control

import (
	"context"
	"time"
)

// ControlTransport is the interface for pushing to and popping from control queues.
type ControlTransport interface {
	// Push enqueues a payload to the given queue key.
	Push(ctx context.Context, queueKey string, payload []byte) error
	// Pop dequeues a payload from the given queue key, blocking up to timeout.
	// Returns (nil, nil) on timeout.
	Pop(ctx context.Context, queueKey string, timeout time.Duration) ([]byte, error)
	// QueueDepth returns the current number of items in the queue.
	QueueDepth(ctx context.Context, queueKey string) (int64, error)
}

// ResponseTransport is the interface for sending and receiving control responses.
type ResponseTransport interface {
	// PushResponse publishes a response payload keyed by requestID with an expiry TTL.
	PushResponse(ctx context.Context, requestID string, payload []byte, ttl time.Duration) error
	// WaitResponse blocks until a response for requestID is available or timeout elapses.
	// Returns (nil, nil) on timeout.
	WaitResponse(ctx context.Context, requestID string, timeout time.Duration) ([]byte, error)
}

// ControlPlane combines ControlTransport and ResponseTransport.
// Both RedisControlTransport and NATSControlTransport implement ControlPlane.
type ControlPlane interface {
	ControlTransport
	ResponseTransport
}
