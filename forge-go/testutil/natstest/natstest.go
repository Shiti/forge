// Package natstest provides shared in-process NATS test helpers for forge-go tests.
package natstest

import (
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/rustic-ai/forge/forge-go/messaging"
)

// StartServer launches an in-process JetStream-enabled NATS server on a random port
// and registers a test-cleanup shutdown.
func StartServer(t *testing.T) *natsserver.Server {
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
	return s
}

// NewBackend creates an in-process NATS server, connects to it, and returns a
// *messaging.NATSBackend with all cleanup registered on t.
func NewBackend(t *testing.T) *messaging.NATSBackend {
	t.Helper()
	s := StartServer(t)

	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err, "failed to connect to in-process NATS server")
	t.Cleanup(func() { nc.Close() })

	backend, err := messaging.NewNATSBackend(nc)
	require.NoError(t, err, "failed to create NATSBackend")
	t.Cleanup(func() { _ = backend.Close() })

	return backend
}
