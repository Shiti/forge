package supervisor

import (
	"context"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
)

// AgentSupervisor defines the lifecycle management interface for forging agents.
type AgentSupervisor interface {
	// Launch starting an agent instance based on a provided configuration
	Launch(ctx context.Context, guildID string, agentSpec *protocol.AgentSpec, reg *registry.Registry, env []string) error

	// Stop requests graceful termination of an agent
	Stop(ctx context.Context, guildID, agentID string) error

	// Status returns the current lifecycle state of an agent
	Status(ctx context.Context, guildID, agentID string) (string, error)

	// StopAll terminates all agents managed by this supervisor
	StopAll(ctx context.Context) error
}
