//go:build windows

package supervisor

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
)

type BubblewrapSupervisor struct {
	rdb *redis.Client
}

func NewBubblewrapSupervisor(rdb *redis.Client) *BubblewrapSupervisor {
	return &BubblewrapSupervisor{rdb: rdb}
}

func (p *BubblewrapSupervisor) Available() bool {
	return false
}

func (p *BubblewrapSupervisor) Launch(ctx context.Context, guildID string, agentSpec *protocol.AgentSpec, reg *registry.Registry, env []string) error {
	return fmt.Errorf("bubblewrap supervisor is not supported on windows")
}

func (p *BubblewrapSupervisor) Stop(ctx context.Context, guildID, agentID string) error {
	return fmt.Errorf("bubblewrap supervisor is not supported on windows")
}

func (p *BubblewrapSupervisor) Status(ctx context.Context, guildID, agentID string) (string, error) {
	return "unknown", nil
}

func (p *BubblewrapSupervisor) GetPID(ctx context.Context, guildID, agentID string) (int, error) {
	return 0, fmt.Errorf("bubblewrap supervisor is not supported on windows")
}

func (p *BubblewrapSupervisor) StopAll(ctx context.Context) error {
	return nil
}
