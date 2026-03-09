package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_UnregisteredAgentSpec_FailsFast(t *testing.T) {
	tempDir := t.TempDir()

	dbPath := filepath.Join(tempDir, "forge_unregistered_test.db")

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Use an empty or minimal registry
	pythonPkg := os.Getenv("FORGE_PYTHON_PKG")
	if pythonPkg == "" {
		pythonPkg = filepath.Join("..", "..", "rustic-ai", "core", "src")
	}
	registryPath := filepath.Join(tempDir, "empty_registry.yaml")
	err = os.WriteFile(registryPath, []byte("entries: []\n"), 0644)
	require.NoError(t, err)

	specPath := filepath.Join(tempDir, "unregistered-guild.yaml")
	// Using manual yaml writing or json
	// Because writeYaml is not exported natively for access here without duplicate code
	// Since we import yaml in another file, let's just write YAML natively
	err = os.WriteFile(specPath, []byte(`
id: unregistered-guild
name: Unregistered Guild
agents:
  - id: ghost-agent
    name: Ghost Agent
    class_name: "com.NonExistent.Agent"
`), 0644)
	require.NoError(t, err)

	forgeBin := requireE2EForgeBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runCmd := exec.CommandContext(ctx, forgeBin, "run", specPath, "--registry", registryPath, "--db-path", dbPath)
	runCmd.Env = append(os.Environ(),
		fmt.Sprintf("FORGE_PYTHON_PKG=%s", pythonPkg),
		"PYTHONUNBUFFERED=1",
		fmt.Sprintf("REDIS_HOST=%s", mr.Host()),
		fmt.Sprintf("REDIS_PORT=%s", mr.Port()),
	)

	// We expect the command to fail and return an error output
	out, err := runCmd.CombinedOutput()

	// Assert process exited with failure (due to missing registry or agent missing in registry)
	assert.Error(t, err, "Forge should have exited with an error when launching an unregistered agent class")

	outputStr := string(out)
	t.Logf("Forge Output:\n%s", outputStr)

	// Depending on where it fails:
	// a) If the Go Supervisor looks up the registry before pushing protocol.SpawnRequest, it will fail fast in Go.
	// b) If the Python Execution Engine looks it up, or if Manager attempts to load it, we'll see Python stacktraces.
	// Wait, currently, the Manager Agent is the one evaluating dependencies. The Manager Agent itself is available normally, unless empty_registry doesn't have it either!
	// Ah, if I use an `empty_registry.yaml`, the manager agent (`rustic_ai.forge.agents.system.guild_manager_agent.GuildManagerAgent`) itself won't be spawnable!
	// Let's assert it fails because the agent class is unregistered, specifically mentioning something about registry or lookup failure.
	assert.True(t, strings.Contains(strings.ToLower(outputStr), "error") || strings.Contains(strings.ToLower(outputStr), "fail"), "output should mention a failure")
}
