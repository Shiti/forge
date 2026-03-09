package e2e

import (
	"context"
	"encoding/json"
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

func TestE2E_AgentCrashAndRestart(t *testing.T) {
	pwd, err := os.Getwd()
	require.NoError(t, err)
	binPath := requireE2EForgeBin(t)

	yamlContent := `
id: echo-guild-crash
name: E2E Echo Guild
version: 1.0.0
description: E2E Crash and Restart test
agents:
  - id: echo-agent
    name: Echo Agent
    description: Echoes incoming messages
    class_name: rustic_ai.core.agents.testutils.echo_agent.EchoAgent
    additional_topics:
      - echo_topic_crash
    listen_to_default_topic: false
    properties: {}
`
	specPath := filepath.Join(t.TempDir(), "echo-guild-crash.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(yamlContent), 0644))

	// Setup Redis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	registryPath := filepath.Join(pwd, "..", "conf", "forge-agent-registry.yaml")
	dbPath := filepath.Join(t.TempDir(), "forge_e2e_crashrestart.db")

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Initial test to confirm EchoAgent is running normally
	testPayload := map[string]interface{}{
		"message": "Hello before crash",
	}
	msg := probe.DefaultMessage(1001, "probe-agent", testPayload)
	topicIn := "echo-guild-crash:echo_topic_crash"
	msg.TopicPublishedTo = topicIn

	// Background routine to continually ping the agent until it finishes uvx boots and subscribes
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
				_ = probeAgent.Publish(ctx, "echo-guild-crash", topicIn, msg)
			}
		}
	}()

	topicOut := "echo-guild-crash:default_topic"
	respMsg, err := probeAgent.WaitForMessage(ctx, topicOut, 30*time.Second)
	close(donePing)
	require.NoError(t, err, "EchoAgent did not respond in time initially")

	assert.Equal(t, "Hello before crash", respMsg.Payload["message"])
	assert.Equal(t, "Echo Agent", *respMsg.Sender.Name)

	// 2. Fetch the current PID from Redis
	statusKey := "forge:agent:status:echo-guild-crash:echo-agent"
	statusStr, err := rdb.Get(ctx, statusKey).Result()
	require.NoError(t, err, "Failed to fetch agent status from Redis")

	var statusData map[string]interface{}
	err = json.Unmarshal([]byte(statusStr), &statusData)
	require.NoError(t, err)

	require.Equal(t, "running", statusData["state"], "Agent is not currently in running state")

	pidObj, ok := statusData["pid"]
	require.True(t, ok, "PID field not found in status JSON")

	firstPID := int(pidObj.(float64))
	t.Logf("Found EchoAgent running with PID: %d", firstPID)

	// 3. Kill the agent manually
	process, err := os.FindProcess(firstPID)
	require.NoError(t, err)

	err = process.Kill()
	require.NoError(t, err)
	t.Logf("Sent SIGKILL to PID %d", firstPID)

	// 4. Wait for Supervisor to detect crash and restart
	// Check Redis status every 500ms for up to 10 seconds.
	// Status should go from 'running' -> 'restarting' -> 'running' (with new PID)
	var newPID int
	require.Eventually(t, func() bool {
		sStr, err := rdb.Get(context.Background(), statusKey).Result()
		if err != nil {
			return false // key might be temporarily missing or error
		}

		var sData map[string]interface{}
		err = json.Unmarshal([]byte(sStr), &sData)
		if err != nil {
			return false
		}

		if sData["state"] == "running" {
			currentPIDObj, hasPid := sData["pid"]
			if hasPid {
				currentPID := int(currentPIDObj.(float64))
				if currentPID != firstPID {
					newPID = currentPID
					return true
				}
			}
		}

		return false
	}, 10*time.Second, 500*time.Millisecond, "EchoAgent was not restarted by supervisor within 10 seconds")

	t.Logf("EchoAgent was successfully restarted! New PID: %d", newPID)

	// 5. Verify the newly restarted agent is fully functional
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	msg2 := probe.DefaultMessage(1002, "probe-agent", map[string]interface{}{
		"message": "Hello after crash",
	})
	msg2.TopicPublishedTo = topicIn

	// Wait a bit for the agent to boot up Python context again if needed
	donePing2 := make(chan struct{})
	go func() {
		counter := int64(2001)
		for {
			select {
			case <-donePing2:
				return
			case <-time.After(2 * time.Second):
				counter++
				msg2.ID = counter
				_ = probeAgent.Publish(ctx2, "echo-guild-crash", topicIn, msg2)
			}
		}
	}()

	respMsg2, err := probeAgent.WaitForMessage(ctx2, topicOut, 15*time.Second)
	close(donePing2)
	require.NoError(t, err, "EchoAgent did not respond in time after restart")

	assert.Equal(t, "Hello after crash", respMsg2.Payload["message"])
	assert.Equal(t, "Echo Agent", *respMsg2.Sender.Name)

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
