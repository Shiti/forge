package agent

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/rustic-ai/forge/forge-go/control"
	"github.com/rustic-ai/forge/forge-go/registry"
	"github.com/rustic-ai/forge/forge-go/secrets"
	"github.com/rustic-ai/forge/forge-go/supervisor"
)

// Agent acts as the central coordinator for running a guild locally or processing queue tasks.
type Agent struct {
	Config              *Config
	RedisClient         *redis.Client
	Supervisor          supervisor.AgentSupervisor
	Registry            *registry.Registry
	SecretProvider      secrets.SecretProvider
	ControlQueueHandler *control.ControlQueueHandler
}

// NewAgent creates a new Agent coordinator instance.
func NewAgent(
	cfg *Config,
	rdb *redis.Client,
	sup supervisor.AgentSupervisor,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	cq *control.ControlQueueHandler,
) *Agent {
	return &Agent{
		Config:              cfg,
		RedisClient:         rdb,
		Supervisor:          sup,
		Registry:            reg,
		SecretProvider:      sec,
		ControlQueueHandler: cq,
	}
}

// Start launches the control queue handler loop and waits on context.
func (a *Agent) Start(ctx context.Context) error {
	return a.ControlQueueHandler.Start(ctx)
}

// Stop gracefully shuts down all supervised processes and the control handler.
func (a *Agent) Stop() {
	if a.ControlQueueHandler != nil {
		a.ControlQueueHandler.Stop()
	}
	if a.Supervisor != nil {
		a.Supervisor.StopAll(context.Background())
	}
}
