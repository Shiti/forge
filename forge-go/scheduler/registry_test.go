package scheduler

import (
	"testing"
)

func TestNodeRegistry_RegistrationAndHeartbeat(t *testing.T) {
	r := NewNodeRegistry()

	r.Register("node-1", ResourceCapacity{CPUs: 4, Memory: 8192, GPUs: 0})

	if !r.IsHealthy("node-1") {
		t.Fatalf("Expected node-1 to be healthy immediately after registration")
	}

	nodes := r.ListHealthy()
	if len(nodes) != 1 || nodes[0].NodeID != "node-1" {
		t.Fatalf("Expected 1 healthy node, got %d", len(nodes))
	}

	r.AllocateCapacity("node-1", ResourceCapacity{CPUs: 2, Memory: 1024, GPUs: 0})

	r.mu.RLock()
	used := r.nodes["node-1"].UsedCapacity
	r.mu.RUnlock()

	if used.CPUs != 2 || used.Memory != 1024 {
		t.Fatalf("Expected used capacity CPUs:2 Mem:1024, got CPUs:%d Mem:%d", used.CPUs, used.Memory)
	}

	r.DeallocateCapacity("node-1", ResourceCapacity{CPUs: 1, Memory: 512, GPUs: 0})

	r.mu.RLock()
	used = r.nodes["node-1"].UsedCapacity
	r.mu.RUnlock()

	if used.CPUs != 1 || used.Memory != 512 {
		t.Fatalf("Expected used capacity CPUs:1 Mem:512, got CPUs:%d Mem:%d", used.CPUs, used.Memory)
	}
}

func TestNodeRegistry_Deregistration(t *testing.T) {
	r := NewNodeRegistry()
	r.Register("node-1", ResourceCapacity{CPUs: 4, Memory: 8192, GPUs: 0})

	r.Deregister("node-1")
	if r.IsHealthy("node-1") {
		t.Fatalf("Expected node-1 to be unhealthy (deregistered)")
	}

	if len(r.ListHealthy()) != 0 {
		t.Fatalf("Expected 0 healthy nodes after deregistration")
	}
}

func TestPlacementMap_Lifecycle(t *testing.T) {
	p := NewPlacementMap()

	payload := []byte(`{"request_id": "test"}`)
	p.Place("guild-1", "agent-1", "node-1", payload)

	orphans := p.AgentsOnNode("node-1")
	if len(orphans) != 1 {
		t.Fatalf("Expected 1 placed agent on node-1, got %d", len(orphans))
	}

	if string(orphans[0].Payload) != string(payload) {
		t.Fatalf("Expected payload to match")
	}

	p.Remove("guild-1", "agent-1")
	if len(p.AgentsOnNode("node-1")) != 0 {
		t.Fatalf("Expected 0 placed agents after removal")
	}
}
