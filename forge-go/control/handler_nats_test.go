package control

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/secrets"
	"github.com/stretchr/testify/require"
)

func TestHandler_NATS_SpawnStop(t *testing.T) {
	nc := newTestNATSConn(t)
	ctx := context.Background()

	cp, err := NewNATSControlTransport(nc)
	require.NoError(t, err)

	reg := loadTestRegistry(t, `entries:
  - id: TestAgent
    class_name: "test.Agent"
    runtime: binary
    executable: "/bin/echo"
`)

	fakeSup := newFakeSupervisor()

	handler := NewControlQueueHandler(cp, reg, secrets.NewEnvSecretProvider(), fakeSup, nil)
	require.NoError(t, handler.Start(ctx))
	defer handler.Stop()
	time.Sleep(100 * time.Millisecond) // allow listener goroutine to start

	// Spawn via NATS
	spawnReq := &protocol.SpawnRequest{
		RequestID: "nats-spawn-1",
		GuildID:   "guild-nats",
		AgentSpec: protocol.AgentSpec{
			ID:        "agent-nats",
			ClassName: "test.Agent",
		},
	}
	spawnWrapper := map[string]interface{}{"command": "spawn", "payload": spawnReq}
	wb, _ := json.Marshal(spawnWrapper)
	require.NoError(t, cp.Push(ctx, ControlQueueRequestKey, wb))

	respBytes, err := cp.WaitResponse(ctx, "nats-spawn-1", 5*time.Second)
	require.NoError(t, err, "No spawn response within timeout")
	require.NotNil(t, respBytes)

	var spawnResp protocol.SpawnResponse
	require.NoError(t, json.Unmarshal(respBytes, &spawnResp))
	require.True(t, spawnResp.Success, "Spawn failed: %v", spawnResp)

	// Stop via NATS
	stopReq := &protocol.StopRequest{
		RequestID: "nats-stop-1",
		GuildID:   "guild-nats",
		AgentID:   "agent-nats",
	}
	stopWrapper := map[string]interface{}{"command": "stop", "payload": stopReq}
	swb, _ := json.Marshal(stopWrapper)
	require.NoError(t, cp.Push(ctx, ControlQueueRequestKey, swb))

	stopRespBytes, err := cp.WaitResponse(ctx, "nats-stop-1", 5*time.Second)
	require.NoError(t, err, "No stop response within timeout")
	require.NotNil(t, stopRespBytes)

	var stopResp protocol.StopResponse
	require.NoError(t, json.Unmarshal(stopRespBytes, &stopResp))
	require.True(t, stopResp.Success, "Stop failed: %v", stopResp)
}
