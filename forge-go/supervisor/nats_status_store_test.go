package supervisor

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startNATSForSupervisor(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("in-process NATS server not ready within 5s")
	}
	t.Cleanup(func() { s.Shutdown() })

	nc, err := nats.Connect(s.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })
	return nc
}

func TestNATSAgentStatusStore_WriteGet(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()
	guildID, agentID := "guild-1", "agent-1"

	// Not found returns (nil, nil)
	status, err := store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	assert.Nil(t, status)

	// Write and get back
	err = store.WriteStatus(ctx, guildID, agentID, &AgentStatusJSON{
		State:     "running",
		NodeID:    "node-1",
		PID:       1234,
		Timestamp: time.Now(),
	}, 30*time.Second)
	require.NoError(t, err)

	status, err = store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "running", status.State)
	assert.Equal(t, "node-1", status.NodeID)
	assert.Equal(t, 1234, status.PID)
}

func TestNATSAgentStatusStore_Refresh(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()
	guildID, agentID := "guild-refresh", "agent-refresh"

	err = store.WriteStatus(ctx, guildID, agentID, &AgentStatusJSON{State: "running", Timestamp: time.Now()}, 30*time.Second)
	require.NoError(t, err)

	// Refresh should not error and status should remain
	err = store.RefreshStatus(ctx, guildID, agentID, 30*time.Second)
	require.NoError(t, err)

	status, err := store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "running", status.State)
}

func TestNATSAgentStatusStore_RefreshNotFound(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()

	// Refreshing a non-existent key should not error
	err = store.RefreshStatus(ctx, "guild-none", "agent-none", 30*time.Second)
	require.NoError(t, err)
}

func TestNATSAgentStatusStore_Delete(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()
	guildID, agentID := "guild-del", "agent-del"

	err = store.WriteStatus(ctx, guildID, agentID, &AgentStatusJSON{State: "running", Timestamp: time.Now()}, 30*time.Second)
	require.NoError(t, err)

	err = store.DeleteStatus(ctx, guildID, agentID)
	require.NoError(t, err)

	status, err := store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	assert.Nil(t, status)
}

func TestNATSAgentStatusStore_DeleteNotFound(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()

	// Deleting a non-existent key should not error
	err = store.DeleteStatus(ctx, "guild-none", "agent-none")
	require.NoError(t, err)
}

// TestNATSAgentStatusStore_AgentIDWithHash is a regression test for the kvSanitize fix:
// agent IDs like "guild123#manager_agent" contain '#' which is invalid in NATS KV keys.
// kvSanitize must replace it with '_' so all store operations succeed.
func TestNATSAgentStatusStore_AgentIDWithHash(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()
	guildID := "guild-abc123"
	agentID := "guild-abc123#manager_agent" // '#' is invalid in NATS KV keys

	// WriteStatus must succeed without error.
	err = store.WriteStatus(ctx, guildID, agentID, &AgentStatusJSON{
		State:     "running",
		NodeID:    "node-1",
		PID:       9999,
		Timestamp: time.Now(),
	}, 30*time.Second)
	require.NoError(t, err, "WriteStatus must not fail for agent IDs containing '#'")

	// GetStatus must retrieve the entry via the sanitized key.
	status, err := store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	require.NotNil(t, status, "GetStatus must find entry for agent ID with '#'")
	assert.Equal(t, "running", status.State)
	assert.Equal(t, 9999, status.PID)

	// DeleteStatus must also work correctly.
	err = store.DeleteStatus(ctx, guildID, agentID)
	require.NoError(t, err)

	status, err = store.GetStatus(ctx, guildID, agentID)
	require.NoError(t, err)
	assert.Nil(t, status)
}

// TestNATSAgentStatusStore_StateTransitions verifies running→restarting→failed lifecycle.
func TestNATSAgentStatusStore_StateTransitions(t *testing.T) {
	nc := startNATSForSupervisor(t)
	store, err := NewNATSAgentStatusStore(nc)
	require.NoError(t, err)

	ctx := context.Background()
	guildID, agentID := "guild-trans", "agent-trans"

	for _, state := range []string{"running", "restarting", "failed"} {
		err = store.WriteStatus(ctx, guildID, agentID, &AgentStatusJSON{
			State: state, Timestamp: time.Now(),
		}, 30*time.Second)
		require.NoError(t, err)

		got, err := store.GetStatus(ctx, guildID, agentID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, state, got.State)
	}
}
