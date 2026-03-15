package control

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

// ControlQueueResponder sends success or error responses back through the transport.
type ControlQueueResponder struct {
	transport ResponseTransport
}

// NewControlQueueResponder creates a new responder using the given transport.
func NewControlQueueResponder(transport ResponseTransport) *ControlQueueResponder {
	return &ControlQueueResponder{
		transport: transport,
	}
}

// SendResponse marshals a success response payload and delivers it to the request's response queue.
func (r *ControlQueueResponder) SendResponse(ctx context.Context, requestID string, response interface{}) error {
	b, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response payload: %w", err)
	}

	return r.transport.PushResponse(ctx, requestID, b, 30*time.Second)
}

// SendError constructs an ErrorResponse payload and delivers it to the request's response queue.
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

	return r.transport.PushResponse(ctx, requestID, b, 30*time.Second)
}
