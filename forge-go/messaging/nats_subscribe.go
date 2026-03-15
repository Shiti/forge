package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

// natsSubscription holds a set of core NATS subscriptions (one per namespaced topic)
// and fans messages into a single SubMessage channel, mirroring the Redis implementation.
type natsSubscription struct {
	subs   []*nats.Subscription
	msgCh  chan SubMessage
	errCh  chan error
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Compile-time check that natsSubscription satisfies the Subscription interface.
var _ Subscription = (*natsSubscription)(nil)

// Subscribe creates a core NATS subscription for each topic (namespaced), starting a
// background goroutine that unmarshals incoming messages and forwards them to the channel.
func (b *NATSBackend) Subscribe(ctx context.Context, namespace string, topics ...string) (Subscription, error) {
	subCtx, cancel := context.WithCancel(ctx)

	ns := &natsSubscription{
		msgCh:  make(chan SubMessage, 100),
		errCh:  make(chan error, 1),
		cancel: cancel,
	}

	for _, topic := range topics {
		nsTopic := namespace + ":" + topic

		// Capture nsTopic for the closure.
		capturedTopic := nsTopic

		sub, err := b.nc.Subscribe(capturedTopic, func(m *nats.Msg) {
			var msg protocol.Message
			if err := json.Unmarshal(m.Data, &msg); err != nil {
				slog.Error("Failed to unmarshal NATS message", "err", err, "topic", capturedTopic)
				return
			}

			select {
			case ns.msgCh <- SubMessage{Topic: capturedTopic, Message: &msg}:
			case <-time.After(50 * time.Millisecond):
				slog.Warn("Consumer channel full, dropping incoming NATS message", "topic", capturedTopic, "msgID", msg.ID)
			case <-subCtx.Done():
				return
			}
		})
		if err != nil {
			cancel()
			// Unsubscribe any already-registered subs.
			for _, s := range ns.subs {
				_ = s.Unsubscribe()
			}
			return nil, fmt.Errorf("failed to subscribe to NATS topic %q: %w", capturedTopic, err)
		}

		ns.subs = append(ns.subs, sub)
	}

	// Monitor context cancellation to detect when the subscriber should stop.
	ns.wg.Add(1)
	go func() {
		defer ns.wg.Done()
		<-subCtx.Done()
	}()

	return ns, nil
}

// Channel returns the receive-only message channel driven by this subscription.
func (s *natsSubscription) Channel() <-chan SubMessage {
	return s.msgCh
}

// ErrChannel returns a channel that emits terminal subscription errors.
func (s *natsSubscription) ErrChannel() <-chan error {
	return s.errCh
}

// Close unsubscribes from all NATS topics and waits for the background goroutine to finish.
func (s *natsSubscription) Close() error {
	s.cancel()
	var lastErr error
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			lastErr = err
		}
	}
	s.wg.Wait()
	return lastErr
}
