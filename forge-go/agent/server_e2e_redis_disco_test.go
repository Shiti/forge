package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
)

func TestServerE2E_Redis_Disconnection(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dataDir := filepath.Join(os.TempDir(), "forge_test_node_redis_disco")
	os.MkdirAll(dataDir, 0755)

	cfg := &ServerConfig{
		DatabaseURL:        "file:testnodedisco?mode=memory&cache=shared",
		RedisURL:           s.Addr(),
		ListenAddress:      getTestPort(9400, 0),
		DataDir:            dataDir,
		LeaderElectionMode: "redis",
	}

	go func() {
		err := StartServer(ctx, cfg)
		if err != nil && err != context.Canceled {
			t.Logf("Node closed with err: %v", err)
		}
	}()

	time.Sleep(3 * time.Second)

	leaderKey, err := s.Get("forge:control:leader")
	assert.NoError(t, err)
	assert.Contains(t, leaderKey, "server-")
	t.Logf("Leader elected: %s", leaderKey)

	// Simulate catastrophic backend Redis failure (network partition or DB crash)
	s.Close()

	// Wait for the heartbeat ticker in RedisElector (runs every TTL/3, so ~1.6s for 5s TTL)
	// to fail and step down.
	time.Sleep(3 * time.Second)

	// Since go tests capture output, this test primarily ensures that the disconnection doesn't panic the Server.
}
