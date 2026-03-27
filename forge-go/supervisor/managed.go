package supervisor

import (
	"sync"
	"time"
)

type AgentState string

const (
	StateStarting   AgentState = "starting"
	StateRunning    AgentState = "running"
	StateStopped    AgentState = "stopped"
	StateRestarting AgentState = "restarting"
	StateFailed     AgentState = "failed"
)

// ManagedAgent tracks the lifecycle, PID, and restart metrics for a specific agent.
type ManagedAgent struct {
	mu sync.RWMutex

	GuildID      string
	ID           string
	State        AgentState
	PID          int
	RestartCount int
	LastError    error

	// Timestamps
	StartedAt  time.Time
	LastExitAt time.Time

	// Channels for coordinated shutdown
	stopCh chan struct{}
}

func NewManagedAgent(guildID, id string) *ManagedAgent {
	return &ManagedAgent{
		GuildID: guildID,
		ID:      id,
		State:   StateStarting,
		stopCh:  make(chan struct{}),
	}
}

func (m *ManagedAgent) SetState(state AgentState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.State = state
}

func (m *ManagedAgent) GetState() AgentState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.State
}

func (m *ManagedAgent) SetPID(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PID = pid
	m.StartedAt = time.Now()
}

func (m *ManagedAgent) ClearPID() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PID = 0
}

func (m *ManagedAgent) GetPID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PID
}

func (m *ManagedAgent) RequestStop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stopCh:
		// already closed
	default:
		close(m.stopCh)
	}
}

func (m *ManagedAgent) IsStopRequested() bool {
	select {
	case <-m.stopCh:
		return true
	default:
		return false
	}
}
