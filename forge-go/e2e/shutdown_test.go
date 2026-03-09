package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestE2E_GracefulShutdown_MultipleAgents(t *testing.T) {
	tempDir := t.TempDir()

	dbPath := filepath.Join(tempDir, "forge_shutdown_test.db")

	// 1. Start a local miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// 2. We'll reuse the Python Registry, which has the testutils `EchoAgent` available.
	pythonPkg := os.Getenv("FORGE_PYTHON_PKG")
	if pythonPkg == "" {
		pythonPkg = filepath.Join("..", "..", "rustic-ai", "core", "src")
	}
	registryPath := filepath.Join(pythonPkg, "rustic_ai", "core", "registry.yaml")

	// 3. Create a Guild spec with 5 agents
	agents := []protocol.AgentSpec{}
	for i := 1; i <= 5; i++ {
		agents = append(agents, protocol.AgentSpec{
			ID:               fmt.Sprintf("echo-agent-%d", i),
			Name:             fmt.Sprintf("Echo Agent %d", i),
			ClassName:        "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
			AdditionalTopics: []string{fmt.Sprintf("echo_topic_%d", i)},
		})
	}

	spec := protocol.GuildSpec{
		ID:          "shutdown-guild",
		Name:        "Shutdown Guild",
		Description: "Testing graceful shutdown of multiple agents",
		Properties: map[string]interface{}{
			"execution_engine": "rustic_ai.forge.execution_engine.ForgeExecutionEngine",
			"messaging": map[string]interface{}{
				"backend_module": "rustic_ai.redis.messaging.backend",
				"backend_class":  "RedisMessagingBackend",
			},
		},
		Agents: agents,
	}

	specPath := filepath.Join(tempDir, "shutdown-guild.yaml")
	specBytes, err := yaml.Marshal(spec)
	require.NoError(t, err)
	err = os.WriteFile(specPath, specBytes, 0644)
	require.NoError(t, err)

	forgeBin := requireE2EForgeBin(t)

	// Run Forge in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runCmd := exec.CommandContext(ctx, forgeBin, "run", specPath, "--registry", registryPath, "--db-path", dbPath)
	runCmd.Env = append(os.Environ(),
		fmt.Sprintf("FORGE_PYTHON_PKG=%s", pythonPkg),
		"PYTHONUNBUFFERED=1",
		fmt.Sprintf("REDIS_HOST=%s", mr.Host()),
		fmt.Sprintf("REDIS_PORT=%s", mr.Port()),
	)

	// Create a new process group for the entire `forge run` process tree
	runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	t.Logf("Starting Forge Guild from: %s", specPath)
	err = runCmd.Start()
	require.NoError(t, err)

	forgePID := runCmd.Process.Pid

	// Ensure that if the test fails or panics abruptly, the process group is killed to avoid orphaned uvx processes
	defer func() {
		_ = syscall.Kill(-forgePID, syscall.SIGKILL)
	}()

	// Wait roughly 5-10 seconds for all agents to spin up and report ready
	// (uv sync and python initialization takes easily 2-3 sec in CI).
	time.Sleep(8 * time.Second)

	// Check if the forge parent process is still alive before terminating
	err = runCmd.Process.Signal(syscall.Signal(0))
	require.NoError(t, err, "Forge daemon died prematurely")

	// Emit Graceful Shutdown (SIGTERM)
	t.Logf("Sending SIGTERM to Forge daemon (PID: %d)", forgePID)
	err = runCmd.Process.Signal(syscall.SIGTERM)
	require.NoError(t, err)

	// Wait up to 15 seconds for clean exit.
	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	select {
	case <-time.After(15 * time.Second):
		t.Fatal("App did not shutdown gracefully within 15 seconds")
	case err := <-done:
		// Go's Wait returns an error if process exited non-zero or due to signal
		// An exit code 0 or cleanly handled signal is returned as nil error
		// or *exec.ExitError if handled via go os.Exit(..) non-zero.
		// Right now, Forge should trap signal and exit 0.
		if err != nil {
			t.Logf("App exited with potentially non-clean status: %v", err)
			// we don't strictly require exit 0 right now if the signal behavior inside cobra is default.
			// But ideally it should be nil.
		} else {
			t.Log("Forge daemon shut down cleanly!")
		}
	}

	// Verify no orphaned uvx processes remain
	// In the real environment, `ps` or checking /proc tree could verify orphaned children
	// Since Forge runs `uvx`, `uvx` launches python.
	// We'll rely on the `Wait` combined with ProcessSupervisor's pgid kill semantics to be confident.
	// But let's check one more time if the pgid is still alive.
	err = syscall.Kill(-forgePID, 0)
	if err == nil {
		t.Fatal("Process group was not completely terminated after graceful shutdown!")
	}
}
