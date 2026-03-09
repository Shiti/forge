//go:build !windows

package supervisor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
	"github.com/rustic-ai/forge/forge-go/telemetry"
)

type BubblewrapSupervisor struct {
	mu     sync.RWMutex
	agents map[string]*ManagedAgent
	rdb    *redis.Client
}

func NewBubblewrapSupervisor(rdb *redis.Client) *BubblewrapSupervisor {
	return &BubblewrapSupervisor{
		agents: make(map[string]*ManagedAgent),
		rdb:    rdb,
	}
}

func (p *BubblewrapSupervisor) Available() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func (p *BubblewrapSupervisor) Launch(ctx context.Context, guildID string, agentSpec *protocol.AgentSpec, reg *registry.Registry, env []string) error {
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
	if len(runtimeCmd) == 0 {
		return fmt.Errorf("runtimeCmd is empty")
	}

	bwrapArgs := p.buildBwrapArgs(entry, runtimeCmd)

	return p.startProcess(ctx, guildID, agent, agentSpec, bwrapArgs, env)
}

func (p *BubblewrapSupervisor) buildBwrapArgs(entry *registry.AgentRegistryEntry, cmd []string) []string {
	var args []string

	args = append(args,
		"--unshare-all",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--die-with-parent",
	)

	if len(entry.Network) > 0 && !containsString(entry.Network, "none") {
		args = append(args, "--share-net")
	}

	for _, fs := range entry.Filesystem {
		if fs.Mode == "rw" {
			args = append(args, "--bind", fs.Path, fs.Path)
		} else {
			args = append(args, "--ro-bind", fs.Path, fs.Path)
		}
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		bindPath := func(path string) {
			if err := os.MkdirAll(path, 0755); err != nil {
				slog.Warn("failed to create host path for bubblewrap bind", "path", path, "err", err)
				return
			}
			args = append(args, "--bind", path, path)
		}

		bindPath(homeDir + "/.local/share/uv")
		bindPath(homeDir + "/.cache/uv")
		bindPath(homeDir + "/.forge")
	}

	args = append(args, "--")
	args = append(args, cmd...)

	return args
}

func (p *BubblewrapSupervisor) startProcess(ctx context.Context, guildID string, agent *ManagedAgent, agentSpec *protocol.AgentSpec, bwrapArgs []string, env []string) error {
	guildID = normalizeGuildID(guildID)
	ctx, span := otel.Tracer("forge.supervisor").Start(ctx, "supervisor.bwrap.spawn")
	defer span.End()

	cmd := exec.CommandContext(ctx, "bwrap", bwrapArgs...)
	cmd.Env = append(os.Environ(), env...)

	propagator := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	if tp, ok := carrier["traceparent"]; ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TRACEPARENT=%s", tp))
	}

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	startBootTime := time.Now()
	if err := cmd.Start(); err != nil {
		agent.SetState(StateFailed)
		agent.LastError = err
		return fmt.Errorf("failed to start agent process %s: %w", agent.ID, err)
	}

	telemetry.SupervisorBootDuration.WithLabelValues("local-node", "bwrap").Observe(time.Since(startBootTime).Seconds())

	agent.SetPID(cmd.Process.Pid)
	agent.SetState(StateRunning)

	logger := slog.With("agent_id", agent.ID, "guild_id", guildID, "node_id", "local-node", "supervisor", "bwrap")

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

	go p.monitorProcess(guildID, agent, agentSpec, cmd, bwrapArgs, env)

	return nil
}

func (p *BubblewrapSupervisor) monitorProcess(guildID string, agent *ManagedAgent, agentSpec *protocol.AgentSpec, cmd *exec.Cmd, bwrapArgs []string, env []string) {
	guildID = normalizeGuildID(guildID)
	ctx, lifecycleSpan := otel.Tracer("forge.supervisor").Start(context.Background(), "bwrap.lifecycle")
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
								telemetry.AgentCPUCores.WithLabelValues(guildID, agent.ID, "local-node-bwrap").Set(cpuPct)
							}
							if memInfo, err := proc.MemoryInfo(); err == nil {
								telemetry.AgentMemoryBytes.WithLabelValues(guildID, agent.ID, "local-node-bwrap").Set(float64(memInfo.RSS))
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

	telemetry.AgentExitCodes.WithLabelValues(guildID, agent.ID, "local-node-bwrap", exitCode).Inc()

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
			if err := p.startProcess(ctx, guildID, agent, agentSpec, bwrapArgs, env); err != nil {
				slog.Error("failed to restart bwrap-managed agent", "guild_id", guildID, "agent_id", agent.ID, "error", err)
				agent.SetState(StateFailed)
				agent.LastError = err
				if p.rdb != nil {
					_ = SetFailedStatus(ctx, p.rdb, guildID, agent.ID)
				}
			}
		}
	case <-agent.stopCh:
		agent.SetState(StateStopped)
		if p.rdb != nil {
			_ = DeleteStatusKey(ctx, p.rdb, guildID, agent.ID)
		}
	}
}

func (p *BubblewrapSupervisor) Stop(ctx context.Context, guildID, agentID string) error {
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
		pgid, err := syscall.Getpgid(pid)
		if err == nil {
			if killErr := syscall.Kill(-pgid, syscall.SIGTERM); killErr != nil && killErr != syscall.ESRCH {
				slog.Warn("failed to SIGTERM process group", "pid", pid, "pgid", pgid, "error", killErr)
			}
		} else {
			if killErr := syscall.Kill(pid, syscall.SIGTERM); killErr != nil && killErr != syscall.ESRCH {
				slog.Warn("failed to SIGTERM process", "pid", pid, "error", killErr)
			}
		}

		for i := 0; i < 50; i++ {
			if syscall.Kill(pid, 0) != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if syscall.Kill(pid, 0) == nil {
			if pgid > 0 {
				if killErr := syscall.Kill(-pgid, syscall.SIGKILL); killErr != nil && killErr != syscall.ESRCH {
					slog.Warn("failed to SIGKILL process group", "pid", pid, "pgid", pgid, "error", killErr)
				}
			} else {
				if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil && killErr != syscall.ESRCH {
					slog.Warn("failed to SIGKILL process", "pid", pid, "error", killErr)
				}
			}
		}
	}

	if p.rdb != nil {
		_ = DeleteStatusKey(ctx, p.rdb, agent.GuildID, agent.ID)
		_ = DeleteStatusKey(ctx, p.rdb, unknownGuildKey, agent.ID)
	}

	return nil
}

func (p *BubblewrapSupervisor) Status(ctx context.Context, guildID, agentID string) (string, error) {
	key := scopedAgentKey(guildID, agentID)
	p.mu.RLock()
	agent, exists := p.agents[key]
	p.mu.RUnlock()

	if !exists {
		return "unknown", nil
	}
	return string(agent.GetState()), nil
}

func (p *BubblewrapSupervisor) GetPID(ctx context.Context, guildID, agentID string) (int, error) {
	key := scopedAgentKey(guildID, agentID)
	p.mu.RLock()
	agent, exists := p.agents[key]
	p.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("agent %s not managed in guild %s", agentID, normalizeGuildID(guildID))
	}

	return agent.GetPID(), nil
}

func (p *BubblewrapSupervisor) StopAll(ctx context.Context) error {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	agents := make([]*ManagedAgent, 0, len(p.agents))
	for _, agent := range p.agents {
		agents = append(agents, agent)
	}
	p.mu.RUnlock()

	var firstErr error
	for _, agent := range agents {
		if err := p.Stop(ctx, agent.GuildID, agent.ID); err != nil {
			slog.Warn("failed to stop agent", "guild_id", agent.GuildID, "agent_id", agent.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
