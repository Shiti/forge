package scheduler

import (
	"sync"
	"time"

	"github.com/rustic-ai/forge/forge-go/telemetry"
)

type ResourceCapacity struct {
	CPUs   int `json:"cpus"`
	Memory int `json:"memory"`
	GPUs   int `json:"gpus"`
}

type NodeState struct {
	NodeID        string           `json:"node_id"`
	TotalCapacity ResourceCapacity `json:"total_capacity"`
	UsedCapacity  ResourceCapacity `json:"used_capacity"`
	LastHeartbeat time.Time        `json:"last_heartbeat"`
}

type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*NodeState
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[string]*NodeState),
	}
}

func (r *NodeRegistry) Register(nodeID string, capacity ResourceCapacity) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if state, exists := r.nodes[nodeID]; exists {
		state.TotalCapacity = capacity
		state.LastHeartbeat = time.Now()
	} else {
		r.nodes[nodeID] = &NodeState{
			NodeID:        nodeID,
			TotalCapacity: capacity,
			UsedCapacity:  ResourceCapacity{},
			LastHeartbeat: time.Now(),
		}
	}
	r.recordMetricsLocked()
}

func (r *NodeRegistry) Heartbeat(nodeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if state, exists := r.nodes[nodeID]; exists {
		latency := time.Since(state.LastHeartbeat)
		telemetry.ObserveNodeHeartbeatLatency(nodeID, latency)

		state.LastHeartbeat = time.Now()
		return true
	}

	return false
}

func (r *NodeRegistry) Deregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.nodes, nodeID)
	r.recordMetricsLocked()
}

// recordMetricsLocked must be called with r.mu held.
func (r *NodeRegistry) recordMetricsLocked() {
	nodesRegistered := len(r.nodes)
	telemetry.SetNodesRegistered(float64(nodesRegistered))

	var availableSlots float64
	for _, state := range r.nodes {
		freeCPU := state.TotalCapacity.CPUs - state.UsedCapacity.CPUs
		if freeCPU > 0 {
			availableSlots += float64(freeCPU)
		}
	}
	telemetry.SetAvailableAgentSlots(availableSlots)
}

func (r *NodeRegistry) IsHealthy(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, exists := r.nodes[nodeID]
	if !exists {
		return false
	}

	return time.Since(state.LastHeartbeat) < 10*time.Second
}

func (r *NodeRegistry) ListHealthy() []NodeState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var healthy []NodeState
	now := time.Now()
	for _, state := range r.nodes {
		if now.Sub(state.LastHeartbeat) < 10*time.Second {
			healthy = append(healthy, *state)
		}
	}
	return healthy
}

var GlobalNodeRegistry = NewNodeRegistry()
