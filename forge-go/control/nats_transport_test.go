package control

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestNATSConn starts an in-process JetStream NATS server and returns a connected *nats.Conn.
// Both the server and connection are cleaned up via t.Cleanup.
func newTestNATSConn(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s, err := natsserver.NewServer(opts)
	require.NoError(t, err, "failed to create in-process NATS server")
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("in-process NATS server did not become ready within 5s")
	}
	t.Cleanup(func() { s.Shutdown() })

	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err, "failed to connect to in-process NATS server")
	t.Cleanup(func() { nc.Close() })
	return nc
}

func TestNATSControlTransport_PushPop(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	payload := []byte(`{"command":"test"}`)
	require.NoError(t, transport.Push(ctx, "test:queue:nats", payload))

	got, err := transport.Pop(ctx, "test:queue:nats", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, payload, got)
}

func TestNATSControlTransport_PopTimeout(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	got, err := transport.Pop(ctx, "empty:queue:nats", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Nil(t, got, "Expected nil on timeout")
}

func TestNATSControlTransport_QueueDepth(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	depth, err := transport.QueueDepth(ctx, "depth:queue:nats")
	require.NoError(t, err)
	assert.Equal(t, int64(0), depth)

	require.NoError(t, transport.Push(ctx, "depth:queue:nats", []byte("a")))
	require.NoError(t, transport.Push(ctx, "depth:queue:nats", []byte("b")))

	depth, err = transport.QueueDepth(ctx, "depth:queue:nats")
	require.NoError(t, err)
	assert.Equal(t, int64(2), depth)
}

func TestNATSControlTransport_PushWaitResponse(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	payload := []byte(`{"success":true}`)
	require.NoError(t, transport.PushResponse(ctx, "req-nats-1", payload, 30*time.Second))

	got, err := transport.WaitResponse(ctx, "req-nats-1", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, payload, got)
}

func TestNATSControlTransport_WaitResponseTimeout(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	got, err := transport.WaitResponse(ctx, "no-such-req-nats", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Nil(t, got, "Expected nil on timeout")
}

// TestNATSControlTransport_MessageDeliveredExactlyOnce is a regression test for the
// WorkQueuePolicy fix: a popped message must not be re-delivered to subsequent Pop
// calls. Without WorkQueuePolicy (LimitsPolicy default), each new ephemeral pull
// consumer re-sees all undeleted messages, causing handlers to fire multiple times.
func TestNATSControlTransport_MessageDeliveredExactlyOnce(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	msg := []byte(`{"command":"spawn","once":true}`)
	require.NoError(t, transport.Push(ctx, "once:queue:nats", msg))

	// First Pop must deliver the message.
	got, err := transport.Pop(ctx, "once:queue:nats", 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, got, "first Pop must receive the message")
	assert.Equal(t, msg, got)

	// Second Pop must time out — message must NOT be re-delivered after ack.
	got2, err := transport.Pop(ctx, "once:queue:nats", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Nil(t, got2, "message must not be re-delivered after being consumed (WorkQueuePolicy regression)")
}

// TestNATSControlTransport_PopSurvivesReconnect verifies that closing and
// recreating a transport for the same queue does not produce a "filtered consumer
// not unique on workqueue stream" error. Before the fix (ephemeral consumers),
// a leftover server-side consumer would conflict with a new one.
func TestNATSControlTransport_PopSurvivesReconnect(t *testing.T) {
	nc := newTestNATSConn(t)

	// First transport — push a message and pop it (creates the durable consumer).
	t1, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, t1.Push(ctx, "reconnect:queue", []byte("msg1")))
	got, err := t1.Pop(ctx, "reconnect:queue", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("msg1"), got)

	// Close the first transport (simulates process restart).
	t1.Close()

	// Second transport — must reuse the durable consumer without error.
	t2, err := NewNATSControlTransport(nc)
	require.NoError(t, err)

	require.NoError(t, t2.Push(ctx, "reconnect:queue", []byte("msg2")))
	got2, err := t2.Pop(ctx, "reconnect:queue", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("msg2"), got2)

	// Queue should be empty.
	empty, err := t2.Pop(ctx, "reconnect:queue", 200*time.Millisecond)
	require.NoError(t, err)
	assert.Nil(t, empty)

	t2.Close()
}

// TestNATSControlTransport_AllMessagesDeliveredExactlyOnce verifies that every
// pushed message is received exactly once and the queue is empty afterwards.
// (Strict FIFO is not guaranteed across sequential ephemeral pull consumers.)
func TestNATSControlTransport_AllMessagesDeliveredExactlyOnce(t *testing.T) {
	nc := newTestNATSConn(t)
	transport, err := NewNATSControlTransport(nc)
	require.NoError(t, err)
	ctx := context.Background()

	sent := map[string]bool{"alpha": false, "beta": false, "gamma": false}
	for m := range sent {
		require.NoError(t, transport.Push(ctx, "all:queue:nats", []byte(m)))
	}

	received := map[string]int{}
	for range sent {
		got, popErr := transport.Pop(ctx, "all:queue:nats", 2*time.Second)
		require.NoError(t, popErr)
		require.NotNil(t, got, "expected a message but got nil")
		received[string(got)]++
	}

	// Every message must appear exactly once.
	for m := range sent {
		assert.Equal(t, 1, received[m], "message %q delivered %d times, want exactly 1", m, received[m])
	}

	// Queue must be empty — no extra deliveries.
	empty, popErr := transport.Pop(ctx, "all:queue:nats", 200*time.Millisecond)
	require.NoError(t, popErr)
	assert.Nil(t, empty, "queue should be empty after consuming all messages")
}
