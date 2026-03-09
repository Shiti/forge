package scheduler

import (
	"sync"
	"time"
)

type AgentPlacement struct {
	GuildID  string
	AgentID  string
	NodeID   string
	PlacedAt time.Time
	Payload  []byte
}

type PlacementMap struct {
	mu         sync.RWMutex
	placements map[string]AgentPlacement // map[guildID:agentID]AgentPlacement
}

func NewPlacementMap() *PlacementMap {
	return &PlacementMap{
		placements: make(map[string]AgentPlacement),
	}
}

func (p *PlacementMap) Place(guildID, agentID, nodeID string, payload []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := guildID + ":" + agentID
	p.placements[key] = AgentPlacement{
		GuildID:  guildID,
		AgentID:  agentID,
		NodeID:   nodeID,
		PlacedAt: time.Now(),
		Payload:  payload,
	}
}

func (p *PlacementMap) Remove(guildID, agentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := guildID + ":" + agentID
	delete(p.placements, key)
}

func (p *PlacementMap) Find(guildID, agentID string) (AgentPlacement, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := guildID + ":" + agentID
	placement, ok := p.placements[key]
	return placement, ok
}

func (p *PlacementMap) AgentsOnNode(nodeID string) []AgentPlacement {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []AgentPlacement
	for _, placement := range p.placements {
		if placement.NodeID == nodeID {
			result = append(result, placement)
		}
	}
	return result
}

// Global placement map for the server
var GlobalPlacementMap = NewPlacementMap()
var GlobalScheduler = NewScheduler(GlobalNodeRegistry)
