package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

// SubMessage wraps a protocol message with its source Redis topic.
type SubMessage struct {
	Topic   string
	Message *protocol.Message
}

// Subscription represents an active Redis PubSub listener piping messages into a Go channel.
type Subscription struct {
	pubsub *redis.PubSub
	msgCh  chan SubMessage
	errCh  chan error
	cancel context.CancelFunc

	wg sync.WaitGroup
}

// Subscribe opens a connection to the specified Redis PubSub topics and spawns a background runtime
// equivalent to Python's `run_in_thread` that automatically parses JSON strings into core Message structs.
func (c *Client) Subscribe(ctx context.Context, namespace string, topics ...string) (*Subscription, error) {
	// Namespace all topics, matching Python's MessagingInterface which
	// internally prepends {guild_id}: to all topic subscriptions.
	nsTopics := make([]string, len(topics))
	for i, t := range topics {
		nsTopics[i] = namespace + ":" + t
	}
	pubsub := c.rdb.Subscribe(ctx, nsTopics...)

	subscribeCtx, cancel := context.WithCancel(ctx)

	sub := &Subscription{
		pubsub: pubsub,
		msgCh:  make(chan SubMessage, 100), // Buffer handling sporadic spikes
		errCh:  make(chan error, 1),
		cancel: cancel,
	}

	// Move the Receive check AFTER creating sub so the cancel is bound
	// Wait for subscription confirmation to ensure topics are fully bound before returning
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("failed to subscribe to topics %v: %w", topics, err)
	}

	sub.wg.Add(1)
	go sub.runSubscriber(subscribeCtx)

	return sub, nil
}

func (s *Subscription) runSubscriber(ctx context.Context) {
	defer s.wg.Done()

	ch := s.pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			// Context canceled, shut down gracefully
			return
		case redisMsg, ok := <-ch:
			if !ok {
				slog.Warn("Redis PubSub channel closed unexpectedly")
				s.errCh <- fmt.Errorf("pubsub channel closed")
				return
			}

			var m protocol.Message
			if err := json.Unmarshal([]byte(redisMsg.Payload), &m); err != nil {
				slog.Error("Failed to unmarshal Redis message", "err", err, "topic", redisMsg.Channel, "payload", redisMsg.Payload)
				continue // Skip corrupt messages rather than tearing down the listener
			}

			// Non-blocking send in case the consumer hangs
			select {
			case s.msgCh <- SubMessage{Topic: redisMsg.Channel, Message: &m}:
			case <-time.After(50 * time.Millisecond):
				slog.Warn("Consumer channel full, dropping incoming PubSub message", "topic", redisMsg.Channel, "msgID", m.ID)
			case <-ctx.Done():
				return
			}
		}
	}
}

// Channel returns the receive-only message channel driven by this subscription.
func (s *Subscription) Channel() <-chan SubMessage {
	return s.msgCh
}

// ErrChannel returns a channel that emits terminal subscription errors.
func (s *Subscription) ErrChannel() <-chan error {
	return s.errCh
}

// Close gracefully terminates the Pub/Sub background pipeline.
func (s *Subscription) Close() error {
	s.cancel()              // Request the goroutine to exit
	err := s.pubsub.Close() // Disconnect Redis
	s.wg.Wait()             // Ensure runSubscriber has completely finished
	return err
}
