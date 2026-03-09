package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusKeyLifecycle(t *testing.T) {
	// 1. Setup Miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ctx := context.Background()

	// 2. Write the initial Running status
	guildID := "guild-abc"
	agentID := "agent-xyz"
	nodeID := "node-1"
	pid := 1024

	err = WriteStatusKey(ctx, rdb, guildID, agentID, nodeID, pid)
	require.NoError(t, err)

	// Verify key exists and contents are correct
	key := fmt.Sprintf("forge:agent:status:%s:%s", guildID, agentID)
	val, err := rdb.Get(ctx, key).Result()
	require.NoError(t, err)

	var status AgentStatusJSON
	err = json.Unmarshal([]byte(val), &status)
	require.NoError(t, err)

	assert.Equal(t, "running", status.State)
	assert.Equal(t, nodeID, status.NodeID)
	assert.Equal(t, pid, status.PID)
	assert.WithinDuration(t, time.Now(), status.Timestamp, 2*time.Second)

	// Verify initial TTL is 30s
	ttl, err := rdb.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.True(t, ttl > 28*time.Second && ttl <= 30*time.Second, "Expected ~30s TTL, got %v", ttl)

	// 3. Fast-forward time in miniredis to simulate TTL decay and test Refresh
	mr.FastForward(15 * time.Second)

	err = RefreshStatusKey(ctx, rdb, guildID, agentID)
	require.NoError(t, err)

	// Verify TTL bounced back up to 30s
	ttl, _ = rdb.TTL(ctx, key).Result()
	assert.True(t, ttl > 28*time.Second && ttl <= 30*time.Second, "Expected TTL to reset to 30s, got %v", ttl)

	// 4. Test Restarting overlay status
	err = SetRestartingStatus(ctx, rdb, guildID, agentID)
	require.NoError(t, err)

	val, _ = rdb.Get(ctx, key).Result()
	json.Unmarshal([]byte(val), &status)
	assert.Equal(t, "restarting", status.State)

	// 5. Test Failed overlay status (300s TTL)
	err = SetFailedStatus(ctx, rdb, guildID, agentID)
	require.NoError(t, err)

	val, _ = rdb.Get(ctx, key).Result()
	json.Unmarshal([]byte(val), &status)
	assert.Equal(t, "failed", status.State)

	ttl, _ = rdb.TTL(ctx, key).Result()
	assert.True(t, ttl > 298*time.Second && ttl <= 300*time.Second, "Expected Failed TTL to be 300s, got %v", ttl)

	// 6. Test Deletion
	err = DeleteStatusKey(ctx, rdb, guildID, agentID)
	require.NoError(t, err)

	_, err = rdb.Get(ctx, key).Result()
	assert.Equal(t, redis.Nil, err, "Expected key to be cleanly deleted")
}
