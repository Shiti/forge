package control

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

// ControlQueueResponder sends success or error responses back through Redis
type ControlQueueResponder struct {
	rdb *redis.Client
}

// NewControlQueueResponder creates a new responder attached to the given Redis instance
func NewControlQueueResponder(rdb *redis.Client) *ControlQueueResponder {
	return &ControlQueueResponder{
		rdb: rdb,
	}
}

// SendResponse marshals a success response payload and LPUSHes it to the specific request's response queue
func (r *ControlQueueResponder) SendResponse(ctx context.Context, requestID string, response interface{}) error {
	b, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response payload: %w", err)
	}

	return r.push(ctx, requestID, b)
}

// SendError constructs an ErrorResponse payload and LPUSHes it to the specific request's response queue
func (r *ControlQueueResponder) SendError(ctx context.Context, requestID, errMsg string) error {
	resp := &protocol.ErrorResponse{
		RequestID: requestID,
		Success:   false,
		Error:     errMsg,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal error payload: %w", err)
	}

	return r.push(ctx, requestID, b)
}

func (r *ControlQueueResponder) push(ctx context.Context, requestID string, payload []byte) error {
	key := fmt.Sprintf("forge:control:response:%s", requestID)

	pipe := r.rdb.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.Expire(ctx, key, 30*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to push response to queue %s: %w", key, err)
	}
	return nil
}
