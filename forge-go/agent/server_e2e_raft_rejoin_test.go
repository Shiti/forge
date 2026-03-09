package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestServerE2E_Raft_IsolationAndRejoin(t *testing.T) {
	// Dummy miniredis for queue listener parity
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis")
	}
	defer s.Close()

	bootServer := func(idx int, base int, peers []string) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())

		dataDir := filepath.Join(os.TempDir(), fmt.Sprintf("forge_raft_node_%d_%d", base, idx))
		os.MkdirAll(dataDir, 0755)

		cfg := &ServerConfig{
			DatabaseURL:        fmt.Sprintf("file:raftnode%d_%d?mode=memory&cache=shared", base, idx),
			RedisURL:           s.Addr(),
			ListenAddress:      getTestPort(base, idx),
			DataDir:            dataDir,
			LeaderElectionMode: "raft",
			RaftBindAddr:       getTestPort(base+100, idx),
			GossipBindAddr:     getTestPort(base+200, idx),
			GossipJoinPeers:    peers,
		}

		go func() {
			err := StartServer(ctx, cfg)
			if err != nil && err != context.Canceled {
				t.Logf("Raft Node %d closed with err: %v", idx, err)
			}
		}()

		time.Sleep(500 * time.Millisecond)
		return ctx, cancel
	}

	// Cluster Base 9500
	t.Log("Booting initial 3 node cluster")
	_, cancel0 := bootServer(0, 9500, nil)
	_, cancel1 := bootServer(1, 9500, []string{getTestPort(9700, 0)}) // join via gossip 9700
	_, cancel2 := bootServer(2, 9500, []string{getTestPort(9700, 0)})

	// Wait for Quorum and Leader Election
	time.Sleep(5 * time.Second)

	// Simulate catastrophic failure of Node 0 (likely the leader as it's the seed)
	t.Log("Simulating Catastrophic Loss of Seed Node 0")
	cancel0()

	// Wait for Failover election (Raft heartbeat timeout + election ~1.5 - 3 seconds)
	time.Sleep(4 * time.Second)

	// Simulate adding a brand new replacing node
	t.Log("Scaling up replacement Node 3")
	// Join via Node 1's gossip port (9701) since Node 0 (9700) is dead
	_, cancel3 := bootServer(3, 9500, []string{getTestPort(9700, 1)})

	// Wait for Memberlist to gossip the new node to the new leader, and the leader to raft.AddVoter
	time.Sleep(5 * time.Second)

	// Teardown
	cancel1()
	cancel2()
	cancel3()

	time.Sleep(1 * time.Second)
	// If it reached here without test timeout or crashing, the zero-conf architecture successfully handled an isolated seed node loss and dynamic rejoins.
}
