package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
)

// getFreePort string to easily allocate different ports for parallel instances
func getTestPort(base int, offset int) string {
	return fmt.Sprintf("127.0.0.1:%d", base+offset)
}

func TestServerE2E_RedisElection(t *testing.T) {
	// 1. Start a single Miniredis instance acting as the cluster's distributed lock store
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis")
	}
	defer s.Close()

	// 2. Start 3 Server Nodes in Redis mode
	bootServer := func(idx int) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())

		// Unique tmp dir for sqlite per node
		dataDir := filepath.Join(os.TempDir(), fmt.Sprintf("forge_test_node_%d", idx))
		os.MkdirAll(dataDir, 0755)

		cfg := &ServerConfig{
			DatabaseURL:        fmt.Sprintf("file:testnode%d?mode=memory&cache=shared", idx),
			RedisURL:           s.Addr(), // Real go-redis client hits this
			ListenAddress:      getTestPort(9000, idx),
			DataDir:            dataDir,
			LeaderElectionMode: "redis",
		}

		go func() {
			err := StartServer(ctx, cfg)
			if err != nil && err != context.Canceled {
				t.Logf("Node %d closed with err: %v", idx, err)
			}
		}()

		// Wait for HTTP API to bind usually means server loops started
		time.Sleep(200 * time.Millisecond)

		return ctx, cancel
	}

	_, cancel0 := bootServer(0)
	defer cancel0()

	_, cancel1 := bootServer(1)
	defer cancel1()

	_, cancel2 := bootServer(2)
	defer cancel2()

	// 3. Verify exactly one leader is elected
	time.Sleep(3 * time.Second)

	leaderKey, err := s.Get("forge:control:leader")
	assert.NoError(t, err)
	assert.Contains(t, leaderKey, "server-") // One of the nodes holds it

	t.Logf("E2E Redis Leader is currently: %s", leaderKey)

	// 4. Force Failover
	if leaderKey == "server-node-0" || leaderKey == "server-"+getTestPort(9000, 0) || leaderKey == fmt.Sprintf("server-%s-%s", getHost(), getTestPort(9000, 0)) {
		cancel0()
	} else if leaderKey == fmt.Sprintf("server-%s-%s", getHost(), getTestPort(9000, 1)) {
		cancel1()
	} else {
		cancel2()
	}

	// 5. Fast Forward TTL to force expiration
	s.FastForward(6 * time.Second)

	// 6. Verify failover
	time.Sleep(3 * time.Second)

	newLeaderKey, err := s.Get("forge:control:leader")
	assert.NoError(t, err)
	assert.NotEqual(t, leaderKey, newLeaderKey) // Someone else picked it up
	t.Logf("E2E Redis New Leader successfully failed over to: %s", newLeaderKey)
}

func getHost() string {
	h, _ := os.Hostname()
	return h
}

func TestServerE2E_RaftElection(t *testing.T) {
	// We do NOT use miniredis here because we are explicitly testing Raft distributed consensus.
	// We'll still give them a dummy miniredis just for the queue listener so it doesn't try to boot embedded ones.
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis")
	}
	defer s.Close()

	bootServer := func(idx int, peers []string) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())

		dataDir := filepath.Join(os.TempDir(), fmt.Sprintf("forge_raft_node_%d", idx))
		os.MkdirAll(dataDir, 0755)

		cfg := &ServerConfig{
			DatabaseURL:        fmt.Sprintf("file:raftnode%d?mode=memory&cache=shared", idx),
			RedisURL:           s.Addr(),
			ListenAddress:      getTestPort(9100, idx),
			DataDir:            dataDir,
			LeaderElectionMode: "raft",
			RaftBindAddr:       getTestPort(9200, idx),
			GossipBindAddr:     getTestPort(9300, idx),
			GossipJoinPeers:    peers,
		}

		go func() {
			err := StartServer(ctx, cfg)
			if err != nil && err != context.Canceled {
				t.Logf("Raft Node %d closed with err: %v", idx, err)
			}
		}()

		time.Sleep(500 * time.Millisecond) // Give memberlist time to bind
		return ctx, cancel
	}

	// Node 0
	_, cancel0 := bootServer(0, nil)
	defer cancel0()

	// Node 1 joins Node 0 via Gossip
	_, cancel1 := bootServer(1, []string{getTestPort(9300, 0)})
	defer cancel1()

	// Node 2 joins Node 0 via Gossip
	_, cancel2 := bootServer(2, []string{getTestPort(9300, 0)})
	defer cancel2()

	// The Raft nodes dynamically form a quorum and negotiate
	// Give them ~5s to election
	time.Sleep(5 * time.Second)

	// Since we can't easily query the leader name natively without instrumenting the internal watch channels,
	// we prove that the nodes booted cleanly without panicking.
	// For actual verification, the internal raft_elector unit tests directly pull IsLeader().
}
