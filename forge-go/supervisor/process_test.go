package supervisor

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/require"
)

func getSleepCmd() []string {
	if runtime.GOOS == "windows" {
		return []string{"ping", "-n", "10", "127.0.0.1"}
	}
	return []string{"sleep", "10"}
}

func getEchoCmd() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", "echo", "hello"}
	}
	return []string{"echo", "hello"}
}

func TestProcessSupervisorLaunchAndStop(t *testing.T) {
	sup := NewProcessSupervisor(nil)
	ctx := context.Background()
	guildID := "test-guild"

	agent1 := NewManagedAgent(guildID, "agent1")
	sup.mu.Lock()
	sup.agents[scopedAgentKey(guildID, "agent1")] = agent1
	sup.mu.Unlock()

	err := sup.startProcess(ctx, guildID, agent1, &protocol.AgentSpec{}, getSleepCmd(), []string{"FOO=bar"})
	if err != nil {
		t.Fatalf("Failed to launch process: %v", err)
	}

	status, err := sup.Status(ctx, guildID, "agent1")
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	if status != string(StateRunning) {
		t.Errorf("Expected status to be running, got %s", status)
	}

	// Wait briefly so process spawns
	time.Sleep(100 * time.Millisecond)

	err = sup.Stop(ctx, guildID, "agent1")
	if err != nil {
		t.Fatalf("Failed to stop process: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	status, _ = sup.Status(ctx, guildID, "agent1")
	if status != string(StateStopped) {
		t.Errorf("Expected status to be stopped, got %s", status)
	}
}

func TestProcessSupervisorCrashRestart(t *testing.T) {
	sup := NewProcessSupervisor(nil)
	ctx := context.Background()
	guildID := "test-guild"

	// process that exits immediately
	agentCrash := NewManagedAgent(guildID, "agent-crash")
	sup.mu.Lock()
	sup.agents[scopedAgentKey(guildID, "agent-crash")] = agentCrash
	sup.mu.Unlock()

	err := sup.startProcess(ctx, guildID, agentCrash, &protocol.AgentSpec{}, getEchoCmd(), nil)
	if err != nil {
		t.Fatalf("Failed to launch process: %v", err)
	}

	// Wait for process to exit and background monitor to catch it
	time.Sleep(200 * time.Millisecond)

	status, _ := sup.Status(ctx, guildID, "agent-crash")
	if status != string(StateRestarting) {
		t.Errorf("Expected status to be restarting, got %s", status)
	}

	require.NoError(t, sup.Stop(ctx, guildID, "agent-crash"))
	time.Sleep(200 * time.Millisecond)

	status, _ = sup.Status(ctx, guildID, "agent-crash")
	if status != string(StateStopped) {
		t.Errorf("Expected status to be stopped after Stop(), got %s", status)
	}
}

func TestProcessSupervisorGuildScopedAgentKeys(t *testing.T) {
	sup := NewProcessSupervisor(nil)
	ctx := context.Background()

	agentID := "upa-dummyuserid"
	guildA := "guild-a"
	guildB := "guild-b"

	a := NewManagedAgent(guildA, agentID)
	a.SetState(StateRunning)
	b := NewManagedAgent(guildB, agentID)
	b.SetState(StateStopped)

	sup.mu.Lock()
	sup.agents[scopedAgentKey(guildA, agentID)] = a
	sup.agents[scopedAgentKey(guildB, agentID)] = b
	sup.mu.Unlock()

	statusA, err := sup.Status(ctx, guildA, agentID)
	if err != nil {
		t.Fatalf("status for guild A failed: %v", err)
	}
	if statusA != string(StateRunning) {
		t.Fatalf("expected guild A status running, got %s", statusA)
	}

	statusB, err := sup.Status(ctx, guildB, agentID)
	if err != nil {
		t.Fatalf("status for guild B failed: %v", err)
	}
	if statusB != string(StateStopped) {
		t.Fatalf("expected guild B status stopped, got %s", statusB)
	}
}
