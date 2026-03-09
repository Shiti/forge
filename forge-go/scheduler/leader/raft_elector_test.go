package leader

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func getFreeLocalhostPort() string {
	// Let the OS assign a random port for testing
	// In reality we just assign sequential ports for the test to avoid collisions
	return "0"
}

func setupTestNodes(t *testing.T, count int, startPort int) []*RaftElector {
	var electors []*RaftElector
	var baseGossip = startPort
	var baseRaft = startPort + count

	var joinPeers []string

	for i := 0; i < count; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		raftAddr := fmt.Sprintf("127.0.0.1:%d", baseRaft+i)
		gossipAddr := fmt.Sprintf("127.0.0.1:%d", baseGossip+i)

		cfg := RaftConfig{
			NodeID:          nodeID,
			RaftBindAddr:    raftAddr,
			GossipBindAddr:  gossipAddr,
			GossipJoinPeers: joinPeers, // First node has empty join peers
		}

		e, err := NewRaftElector(cfg)
		if err != nil {
			t.Fatalf("failed to create RaftElector %d: %v", i, err)
		}

		electors = append(electors, e)

		// Add to join peers so subsequent nodes can join the cluster via gossip
		if len(joinPeers) == 0 {
			joinPeers = append(joinPeers, gossipAddr)
		}
	}

	return electors
}

func TestRaftElector_QuorumFormation(t *testing.T) {
	// Spin up 3 nodes starting at port 8600
	electors := setupTestNodes(t, 3, 8600)
	defer func() {
		for _, e := range electors {
			e.Close()
		}
	}()

	// Wait for the cluster to elect a leader
	// This tests the Gossip Notification -> raft.AddVoter logic
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for all nodes to agree on a leader or at least one to claim leadership
	leaderCount := 0
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for leader election across 3 nodes")
		case <-time.After(1 * time.Second):
			leaderCount = 0
			for _, e := range electors {
				if e.IsLeader() {
					leaderCount++
				}
			}
			if leaderCount == 1 {
				// Success, we have exactly one leader!
				return
			}
			if leaderCount > 1 {
				t.Fatalf("split brain detected, %d leaders", leaderCount)
			}
		}
	}
}

func TestRaftElector_Failover(t *testing.T) {
	// Spin up 3 nodes starting at port 8700
	electors := setupTestNodes(t, 3, 8700)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var leaderIdx = -1

	// Wait for initial leader
waitForLeader:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for initial leader")
		case <-time.After(1 * time.Second):
			for i, e := range electors {
				if e.IsLeader() {
					leaderIdx = i
					break waitForLeader
				}
			}
		}
	}

	if leaderIdx == -1 {
		t.Fatalf("failed to find leader")
	}

	// Kill the leader
	electors[leaderIdx].Close()

	// Wait for a follower to take over
waitForFailover:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for failover")
		case <-time.After(1 * time.Second):
			for i, e := range electors {
				if i != leaderIdx && e.IsLeader() {
					// We have a new leader!
					break waitForFailover
				}
			}
		}
	}

	// Clean up remaining
	for i, e := range electors {
		if i != leaderIdx {
			e.Close()
		}
	}
}
