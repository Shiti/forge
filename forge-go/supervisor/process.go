package supervisor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
	"github.com/rustic-ai/forge/forge-go/telemetry"
)

type ProcessSupervisor struct {
	mu     sync.RWMutex
	agents map[string]*ManagedAgent
	rdb    *redis.Client
}

func NewProcessSupervisor(rdb *redis.Client) *ProcessSupervisor {
	return &ProcessSupervisor{
		agents: make(map[string]*ManagedAgent),
		rdb:    rdb,
	}
}

func (p *ProcessSupervisor) Launch(ctx context.Context, guildID string, agentSpec *protocol.AgentSpec, reg *registry.Registry, env []string) error {
	guildID = normalizeGuildID(guildID)
	key := scopedAgentKey(guildID, agentSpec.ID)
	p.mu.Lock()
	if existing, exists := p.agents[key]; exists {
		state := existing.GetState()
		if state != StateStopped && state != StateFailed {
			p.mu.Unlock()
			return fmt.Errorf("agent %s is already managed in guild %s", agentSpec.ID, normalizeGuildID(guildID))
		}
		delete(p.agents, key)
	}

	agent := NewManagedAgent(guildID, agentSpec.ID)
	p.agents[key] = agent
	p.mu.Unlock()

	entry, err := reg.Lookup(agentSpec.ClassName)
	if err != nil {
		agent.SetState(StateFailed)
		return fmt.Errorf("failed to lookup agent class %s: %w", agentSpec.ClassName, err)
	}

	runtimeCmd := registry.ResolveCommand(entry)

	return p.startProcess(ctx, guildID, agent, agentSpec, runtimeCmd, env)
}

func (p *ProcessSupervisor) startProcess(ctx context.Context, guildID string, agent *ManagedAgent, agentSpec *protocol.AgentSpec, runtimeCmd []string, env []string) error {
	ctx, span := otel.Tracer("forge.supervisor").Start(ctx, "supervisor.spawn")
	defer span.End()

	if len(runtimeCmd) == 0 {
		return fmt.Errorf("runtimeCmd is empty")
	}

	cmd := exec.CommandContext(ctx, runtimeCmd[0], runtimeCmd[1:]...)
	cmd.Env = append(os.Environ(), env...)

	propagator := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	if tp, ok := carrier["traceparent"]; ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TRACEPARENT=%s", tp))
	}

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	configureCommandForProcessGroup(cmd)

	startBootTime := time.Now()
	if err := cmd.Start(); err != nil {
		agent.SetState(StateFailed)
		agent.LastError = err
		return fmt.Errorf("failed to start agent process %s: %w", agent.ID, err)
	}

	telemetry.SupervisorBootDuration.WithLabelValues("local-node", "process").Observe(time.Since(startBootTime).Seconds())

	agent.SetPID(cmd.Process.Pid)
	agent.SetState(StateRunning)

	logger := slog.With("agent_id", agent.ID, "guild_id", guildID, "node_id", "local-node")

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Info(scanner.Text(), "source", "agent_stdout")
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Error(scanner.Text(), "source", "agent_stderr")
		}
	}()

	_ = applyResourceLimits(cmd.Process.Pid, agentSpec)

	if p.rdb != nil {
		_ = WriteStatusKey(ctx, p.rdb, guildID, agent.ID, "local-node", cmd.Process.Pid)
	}

	go p.monitorProcess(guildID, agent, agentSpec, cmd, runtimeCmd, env)

	return nil
}

func (p *ProcessSupervisor) monitorProcess(guildID string, agent *ManagedAgent, agentSpec *protocol.AgentSpec, cmd *exec.Cmd, runtimeCmd []string, env []string) {
	guildID = normalizeGuildID(guildID)
	ctx, lifecycleSpan := otel.Tracer("forge.supervisor").Start(context.Background(), "agent.lifecycle")
	defer lifecycleSpan.End()

	ticker := time.NewTicker(10 * time.Second)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				if agent.GetState() == StateRunning {
					if p.rdb != nil {
						_ = RefreshStatusKey(ctx, p.rdb, guildID, agent.ID)
					}
					pid := agent.GetPID()
					if pid > 0 {
						if proc, err := process.NewProcess(int32(pid)); err == nil {
							if cpuPct, err := proc.CPUPercent(); err == nil {
								telemetry.AgentCPUCores.WithLabelValues(guildID, agent.ID, "local-node").Set(cpuPct)
							}
							if memInfo, err := proc.MemoryInfo(); err == nil {
								telemetry.AgentMemoryBytes.WithLabelValues(guildID, agent.ID, "local-node").Set(float64(memInfo.RSS))
							}
						}
					}
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	err := cmd.Wait()
	close(done)

	agent.LastExitAt = time.Now()
	exitCode := "1"
	if err == nil {
		exitCode = "0"
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = fmt.Sprintf("%d", exitErr.ExitCode())
	}

	telemetry.AgentExitCodes.WithLabelValues(guildID, agent.ID, "local-node", exitCode).Inc()

	if agent.IsStopRequested() {
		agent.SetState(StateStopped)
		if p.rdb != nil {
			_ = DeleteStatusKey(ctx, p.rdb, guildID, agent.ID)
		}
		return
	}

	agent.SetState(StateRestarting)
	agent.LastError = err

	if p.rdb != nil {
		_ = SetRestartingStatus(ctx, p.rdb, guildID, agent.ID)
	}

	if time.Since(agent.StartedAt) > StableTime {
		agent.RestartCount = 0
	}

	agent.RestartCount++

	delay := ComputeBackoff(agent.RestartCount)
	if delay == 0 {
		agent.SetState(StateFailed)
		if p.rdb != nil {
			_ = SetFailedStatus(ctx, p.rdb, guildID, agent.ID)
		}
		return
	}

	slog.Info("agent crashed, restarting", "agent_id", agent.ID, "delay", delay, "attempt", agent.RestartCount)

	select {
	case <-time.After(delay):
		if !agent.IsStopRequested() {
			p.startProcess(ctx, guildID, agent, agentSpec, runtimeCmd, env)
		}
	case <-agent.stopCh:
		agent.SetState(StateStopped)
		if p.rdb != nil {
			_ = DeleteStatusKey(ctx, p.rdb, guildID, agent.ID)
		}
	}
}

func (p *ProcessSupervisor) Stop(ctx context.Context, guildID, agentID string) error {
	key := scopedAgentKey(guildID, agentID)
	p.mu.RLock()
	agent, exists := p.agents[key]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not managed in guild %s", agentID, normalizeGuildID(guildID))
	}

	agent.RequestStop()
	pid := agent.GetPID()

	if pid > 0 {
		_ = terminateProcessTree(pid)
	}

	if p.rdb != nil {
		_ = DeleteStatusKey(ctx, p.rdb, agent.GuildID, agent.ID)
		_ = DeleteStatusKey(ctx, p.rdb, unknownGuildKey, agent.ID)
	}

	return nil
}

func (p *ProcessSupervisor) Status(ctx context.Context, guildID, agentID string) (string, error) {
	key := scopedAgentKey(guildID, agentID)
	p.mu.RLock()
	agent, exists := p.agents[key]
	p.mu.RUnlock()

	if !exists {
		return "unknown", nil
	}
	return string(agent.GetState()), nil
}

func (p *ProcessSupervisor) GetPID(ctx context.Context, guildID, agentID string) (int, error) {
	key := scopedAgentKey(guildID, agentID)
	p.mu.RLock()
	agent, exists := p.agents[key]
	p.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("agent %s not managed in guild %s", agentID, normalizeGuildID(guildID))
	}

	return agent.GetPID(), nil
}

func (p *ProcessSupervisor) StopAll(ctx context.Context) error {
	p.mu.RLock()
	agents := make([]*ManagedAgent, 0, len(p.agents))
	for _, agent := range p.agents {
		agents = append(agents, agent)
	}
	p.mu.RUnlock()

	for _, agent := range agents {
		p.Stop(ctx, agent.GuildID, agent.ID)
	}

	return nil
}
