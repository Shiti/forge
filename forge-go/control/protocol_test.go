package control

import (
	"encoding/json"
	"testing"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolSerialization(t *testing.T) {
	// RED phase: we expect these types to not exist yet, so this will fail to compile
	// if we haven't written protocol.go.

	// 1. Test protocol.SpawnRequest
	req := &protocol.SpawnRequest{
		RequestID: "req-123",
		GuildID:   "guild-abc",
		AgentSpec: protocol.AgentSpec{
			ID:        "agent-xyz",
			ClassName: "test.Agent",
		},
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var reqOut protocol.SpawnRequest
	err = json.Unmarshal(b, &reqOut)
	require.NoError(t, err)
	assert.Equal(t, "req-123", reqOut.RequestID)
	assert.Equal(t, "guild-abc", reqOut.GuildID)
	assert.Equal(t, "agent-xyz", reqOut.AgentSpec.ID)

	// 2. Test protocol.StopRequest
	stopReq := &protocol.StopRequest{
		RequestID: "req-456",
		GuildID:   "guild-abc",
		AgentID:   "agent-xyz",
	}

	b, err = json.Marshal(stopReq)
	require.NoError(t, err)

	var stopOut protocol.StopRequest
	err = json.Unmarshal(b, &stopOut)
	require.NoError(t, err)
	assert.Equal(t, "req-456", stopOut.RequestID)
	assert.Equal(t, "agent-xyz", stopOut.AgentID)

	// 3. Test protocol.SpawnResponse
	resp := &protocol.SpawnResponse{
		RequestID: "req-123",
		Success:   true,
		Message:   "Agent spawned",
		NodeID:    "node-1",
		PID:       1234,
	}

	b, err = json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "req-123")
}
