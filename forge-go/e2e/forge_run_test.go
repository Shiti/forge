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

func TestE2E_ForgeRun_CleanShutdown(t *testing.T) {
	pwd, err := os.Getwd()
	require.NoError(t, err)
	binPath := requireE2EForgeBin(t)

	yamlContent := `
id: echo-guild-run
name: E2E Echo Guild
version: 1.0.0
description: E2E Run test
agents:
  - id: echo-agent
    name: Echo Agent
    description: Echoes incoming messages
    class_name: rustic_ai.core.agents.testutils.echo_agent.EchoAgent
    additional_topics:
      - echo_topic_run
    listen_to_default_topic: false
    properties: {}
`
	specPath := filepath.Join(t.TempDir(), "echo-guild-run.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(yamlContent), 0644))

	// Setup Redis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	registryPath := filepath.Join(pwd, "..", "conf", "forge-agent-registry.yaml")
	dbPath := filepath.Join(t.TempDir(), "forge_e2e_cleanshutdown.db")

	// Start forge run
	runCmd := exec.Command(binPath, "run", specPath, "--redis", mr.Addr(), "--registry", registryPath, "--db-path", dbPath)

	// Extract Redis connection details
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

	// Ensure child process finds Python codebase
	forgePythonPath := filepath.Join(pwd, "..", "..", "forge-python")
	runCmd.Env = append(os.Environ(),
		"FORGE_PYTHON_PKG="+forgePythonPath,
		"PYTHONUNBUFFERED=1",
		"REDIS_HOST="+redisHost,
		"REDIS_PORT="+redisPort,
	)

	// Create buffers to capture stdout and stderr natively if needed later but connect them so we can log them if failures emerge
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	err = runCmd.Start()
	require.NoError(t, err, "Failed to start forge run command")

	// Clean up child process in case of panic
	defer func() {
		if runCmd.Process != nil {
			runCmd.Process.Signal(syscall.SIGKILL)
		}
	}()

	// Verify agent starts
	probeAgent := probe.NewProbeAgent(rdb)
	// 30 seconds for the EchoAgent to start up, python venv to kick in, and respond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send message
	testPayload := map[string]interface{}{
		"message": "Hello Forge Run",
	}
	msg := probe.DefaultMessage(1001, "probe-agent", testPayload)
	topicIn := "echo-guild-run:echo_topic_run"
	msg.TopicPublishedTo = topicIn

	// Background routine to continually ping the agent until it finishes uvx boots and subscribes
	donePing := make(chan struct{})
	go func() {
		for {
			select {
			case <-donePing:
				return
			case <-time.After(2 * time.Second):
				_ = probeAgent.Publish(ctx, "echo-guild-run", topicIn, msg)
			}
		}
	}()

	// Wait up to 30 seconds for the EchoAgent to receive, process, and publish a response
	topicOut := "echo-guild-run:default_topic"
	respMsg, err := probeAgent.WaitForMessage(ctx, topicOut, 30*time.Second)
	close(donePing)
	require.NoError(t, err, "EchoAgent did not respond in time")

	assert.Equal(t, "Hello Forge Run", respMsg.Payload["message"])
	assert.Equal(t, "Echo Agent", *respMsg.Sender.Name)

	// Emit Graceful Shutdown!
	err = runCmd.Process.Signal(syscall.SIGTERM)
	require.NoError(t, err)

	// Wait for the program to exit cleanly.
	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "App should exit with code 0 on graceful SIGTERM")
	case <-time.After(10 * time.Second):
		t.Fatal("App did not shutdown gracefully within 10 seconds")
	}
}
