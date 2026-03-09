package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rustic-ai/forge/forge-go/embed"
	"github.com/rustic-ai/forge/forge-go/guild/store"
)

func TestStartLocal_DBPersistence(t *testing.T) {
	t.Setenv("FORGE_MANAGER_API_BASE_URL", "")
	t.Setenv("FORGE_MANAGER_LOCAL_LISTEN", "")

	// Create a temporary database file and spec file for the test
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "forge_test.db")
	specPath := filepath.Join(tempDir, "test-guild.yaml")
	regPath := filepath.Join(tempDir, "test-registry.yaml")

	// Dummy registry content
	regContent := `
entries:
  - id: CodeClassName
    class_name: CodeClassName
    runtime: binary
    executable: "sleep"
  - id: manager
    class_name: rustic_ai.forge.agents.system.guild_manager_agent.GuildManagerAgent
    runtime: binary
    executable: "sleep"
`
	err := os.WriteFile(regPath, []byte(regContent), 0644)
	require.NoError(t, err)

	specContent := `
id: test_guild_1
name: Test Guild
description: A test guild
agents:
  - id: echo_agent_1
    name: Echo Agent
    class_name: CodeClassName
`
	err = os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	cfg := &Config{
		SpecFile:     specPath,
		DBPath:       dbPath,
		RedisAddr:    "", // Let it start miniredis
		RegistryPath: regPath,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Act: Invoke StartLocal
	ag, err := StartLocal(ctx, cfg)
	require.NoError(t, err, "StartLocal should succeed")

	// Wait a moment for any background initialization
	time.Sleep(100 * time.Millisecond)

	// Cleanly stop the agent coordinate loop
	cancel() // Stops context
	ag.Stop()

	// 2. Assert: Check if the SQLite DB was created and data persisted
	s, err := store.NewGormStore(store.DriverSQLite, dbPath)
	require.NoError(t, err, "Should be able to connect to the SQLite DB that StartLocal was supposed to initialize")
	defer s.Close()

	// 2a. Fetch the Guild
	gm, err := s.GetGuild("test_guild_1")
	require.NoError(t, err, "Guild 'test_guild_1' must be persisted to the DB")

	assert.Equal(t, "test_guild_1", gm.ID)
	assert.Equal(t, "Test Guild", gm.Name)

	// 2b. Fetch the nested Agents
	agents, err := s.ListAgentsByGuild("test_guild_1")
	require.NoError(t, err, "Agents must be persisted and retrievable")
	require.Len(t, agents, 1, "There should be exactly 1 agent saved")

	assert.Equal(t, "echo_agent_1", agents[0].ID)
	assert.Equal(t, "Echo Agent", agents[0].Name)
}

func TestStartLocal_OrchestrationBridge(t *testing.T) {
	t.Setenv("FORGE_MANAGER_API_BASE_URL", "")
	t.Setenv("FORGE_MANAGER_LOCAL_LISTEN", "")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "forge_test.db")
	specPath := filepath.Join(tempDir, "test-guild-orch.yaml")
	regPath := filepath.Join(tempDir, "test-registry-orch.yaml")

	// Dummy registry mapping normal agents and the manager agent
	regContent := `
entries:
  - id: dummy
    class_name: "CodeClassName"
    runtime: binary
    executable: "sleep"
    args: ["10"]
  - id: manager
    class_name: "rustic_ai.forge.agents.system.guild_manager_agent.GuildManagerAgent"
    runtime: binary
    executable: "sleep"
    args: ["10"]
`
	err := os.WriteFile(regPath, []byte(regContent), 0644)
	require.NoError(t, err)

	specContent := `
id: test_guild_orch
name: Orchestration Guild
description: Testing the GMA bridge
properties:
  execution_engine: "rustic_ai.core.guild.execution.sync.sync_exec_engine.SyncExecutionEngine"
  messaging:
    backend_module: "rustic_ai.core.messaging.backend"
    backend_class: "InMemoryMessagingBackend"
agents:
  - id: echo_agent_1
    name: Echo Agent
    class_name: CodeClassName
  - id: gateway_agent_1
    name: Gateway Agent
    class_name: CodeClassName
`
	err = os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	er, err := embed.StartEmbeddedRedis()
	require.NoError(t, err)

	cfg := &Config{
		SpecFile:     specPath,
		DBPath:       dbPath,
		RedisAddr:    er.Addr(),
		RegistryPath: regPath,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ag, err := StartLocal(ctx, cfg)
	require.NoError(t, err)

	// Wait enough time for the listener to pop the queue and supervisor to create statuses in Redis
	time.Sleep(200 * time.Millisecond)

	rdb := redis.NewClient(&redis.Options{Addr: er.Addr()})
	defer rdb.Close()

	// Fetch redis keys created by the supervisor
	keys, err := rdb.Keys(ctx, "forge:agent:status:*").Result()
	require.NoError(t, err)

	ag.Stop()
	cancel()

	// 1. Assert exactly ONE agent was launched! (The manager agent, not the 2 normal agents)
	require.Len(t, keys, 1, "Exactly one agent should have been launched by StartLocal")

	// 2. Assert that the one agent is the GuildManagerAgent
	expectedManagerKey := "forge:agent:status:test_guild_orch:test_guild_orch#manager_agent"
	assert.Equal(t, expectedManagerKey, keys[0], "The spawned agent must be the GuildManagerAgent")

	// 3. Assert the DB properties were dynamically overridden to hook into Forge
	s, err := store.NewGormStore(store.DriverSQLite, dbPath)
	require.NoError(t, err)
	defer s.Close()

	gm, err := s.GetGuild("test_guild_orch")
	require.NoError(t, err)

	assert.Equal(t, "rustic_ai.forge.execution_engine.ForgeExecutionEngine", gm.ExecutionEngine)
	assert.Equal(t, "rustic_ai.redis.messaging.backend", gm.BackendModule)
	assert.Equal(t, "RedisMessagingBackend", gm.BackendClass)
}
