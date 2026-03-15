package supervisor

import (
	"time"
)

// AgentStatusJSON defines the JSON payload written to the status store.
type AgentStatusJSON struct {
	State     string    `json:"state"`
	NodeID    string    `json:"node_id,omitempty"`
	PID       int       `json:"pid,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
