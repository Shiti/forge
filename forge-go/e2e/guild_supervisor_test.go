package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rustic-ai/forge/forge-go/testutil/probe"
)

// TestE2E_GuildSupervisorModes tests a full guild lifecycle (forge run → agent boot → message flow → shutdown)
// across all supervisor modes: process, docker, and bwrap.
//
// This validates that the DispatchingSupervisor correctly routes agent execution
// through the entire forge run CLI path, not just direct supervisor calls.
func TestE2E_GuildSupervisorModes(t *testing.T) {
	pwd, err := os.Getwd()
	require.NoError(t, err)
	binPath := requireE2EForgeBin(t)

	// 2. Resolve paths
	forgePythonPath, _ := filepath.Abs(filepath.Join(pwd, "..", "..", "forge-python"))
	registryPath := filepath.Join(pwd, "..", "conf", "forge-agent-registry.yaml")

	supervisors := []string{"process", "docker", "bwrap"}

	for _, supMode := range supervisors {
		supMode := supMode // capture range variable
		t.Run(supMode, func(t *testing.T) {
			if supMode == "bwrap" && !bubblewrapUsable() {
				t.Skip("Bubblewrap not usable in this environment")
			}
			// 3. Create a guild spec YAML with a single EchoAgent
			guildID := "guild-sup-" + supMode
			yamlContent := `
id: ` + guildID + `
name: E2E Supervisor Guild
version: 1.0.0
description: Validates guild lifecycle under ` + supMode + ` supervisor
agents:
  - id: echo-agent
    name: Echo Agent
    description: Echoes incoming messages
    class_name: rustic_ai.core.agents.testutils.echo_agent.EchoAgent
    additional_topics:
      - echo_topic
    listen_to_default_topic: false
    properties: {}
`
			specPath := filepath.Join(t.TempDir(), guildID+".yaml")
			require.NoError(t, os.WriteFile(specPath, []byte(yamlContent), 0644))

			// 4. Start an embedded miniredis
			mr, err := miniredis.Run()
			require.NoError(t, err)
			defer mr.Close()

			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rdb.Close()

			var dbPath string
			if supMode == "docker" || supMode == "bwrap" {
				// Use a stable /tmp path so Docker containers can access the SQLite DB
				dbDir := filepath.Join("/tmp", "forge_e2e_"+supMode)
				require.NoError(t, os.MkdirAll(dbDir, 0755))
				dbPath = filepath.Join(dbDir, "forge.db")
				defer os.RemoveAll(dbDir)
			} else {
				dbPath = filepath.Join(t.TempDir(), "forge_"+supMode+".db")
			}

			// 5. Launch forge run with --default-supervisor
			runCmd := exec.Command(binPath, "run", specPath,
				"--redis", mr.Addr(),
				"--registry", registryPath,
				"--db-path", dbPath,
				"--default-supervisor", supMode,
			)

			// Extract Redis host/port for agent env
			var redisHost, redisPort string
			addr := mr.Addr()
			for i := len(addr) - 1; i >= 0; i-- {
				if addr[i] == ':' {
					redisHost = addr[:i]
					redisPort = addr[i+1:]
					break
				}
			}
			if redisHost == "" {
				redisHost = "localhost"
			}

			runCmd.Env = append(os.Environ(),
				"FORGE_PYTHON_PKG="+forgePythonPath,
				"PYTHONUNBUFFERED=1",
				"REDIS_HOST="+redisHost,
				"REDIS_PORT="+redisPort,
			)

			// For Docker/Bwrap modes, inject filesystem binds and network access
			// so containers can resolve the local Python package and reach miniredis.
			if supMode == "docker" || supMode == "bwrap" {
				dbDir := filepath.Dir(dbPath)
				runCmd.Env = append(runCmd.Env,
					"FORGE_INJECT_FS="+forgePythonPath+":rw,"+dbDir+":rw",
					"FORGE_INJECT_NET=host",
				)
			}

			runCmd.Stdout = os.Stdout
			runCmd.Stderr = os.Stderr
			runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

			err = runCmd.Start()
			require.NoError(t, err, "Failed to start forge run")

			forgePID := runCmd.Process.Pid

			// Ensure cleanup on panic/failure
			defer func() {
				_ = syscall.Kill(-forgePID, syscall.SIGKILL)
			}()

			// 6. Use ProbeAgent to validate message flow
			probeAgent := probe.NewProbeAgent(rdb)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			testPayload := map[string]interface{}{
				"message": "Hello from " + supMode + " supervisor!",
			}
			msg := probe.DefaultMessage(1001, "probe-agent", testPayload)
			topicIn := guildID + ":echo_topic"
			msg.TopicPublishedTo = topicIn

			// Continuously ping until the agent boots and subscribes
			donePing := make(chan struct{})

			go func() {
				counter := int64(1001)
				for {
					select {
					case <-donePing:
						return
					case <-time.After(2 * time.Second):
						counter++
						msg.ID = counter
						_ = probeAgent.Publish(ctx, guildID, topicIn, msg)
					}
				}
			}()

			// Wait for echo response
			topicOut := guildID + ":default_topic"
			respMsg, err := probeAgent.WaitForMessage(ctx, topicOut, 60*time.Second)
			close(donePing)
			require.NoError(t, err, "EchoAgent (%s mode) did not respond in time", supMode)

			assert.Equal(t, "Hello from "+supMode+" supervisor!", respMsg.Payload["message"])

			// 7. Graceful shutdown
			err = runCmd.Process.Signal(syscall.SIGTERM)
			require.NoError(t, err)

			done := make(chan error, 1)
			go func() {
				done <- runCmd.Wait()
			}()

			select {
			case err := <-done:
				if err != nil {
					t.Logf("Forge (%s) exited with: %v", supMode, err)
				} else {
					t.Logf("Forge (%s) shut down cleanly", supMode)
				}
			case <-time.After(15 * time.Second):
				t.Fatalf("Forge (%s) did not shutdown within 15 seconds", supMode)
			}
		})
	}
}
