package guild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/require"
)

func TestApplyDefaults_AssignsMissingAgentIDs(t *testing.T) {
	spec := &protocol.GuildSpec{
		ID:          "g-1",
		Name:        "Guild",
		Description: "Guild description",
		Agents: []protocol.AgentSpec{
			{
				ID:          "",
				Name:        "Echo Agent",
				Description: "Echo",
				ClassName:   "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
			},
			{
				ID:          "custom-agent-id",
				Name:        "Helper Agent",
				Description: "Helper",
				ClassName:   "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
			},
		},
	}

	applyDefaults(spec)

	require.Equal(t, "g-1#a-0", spec.Agents[0].ID)
	require.Equal(t, "custom-agent-id", spec.Agents[1].ID)
}

func TestBuildModels_PreservesDependencyMapsAndPredicates(t *testing.T) {
	spec := &protocol.GuildSpec{
		ID:          "g-1",
		Name:        "Guild",
		Description: "Guild description",
		Properties: map[string]interface{}{
			"messaging": map[string]interface{}{
				"backend_module": "rustic_ai.redis.messaging.backend",
				"backend_class":  "RedisMessagingBackend",
				"backend_config": map[string]interface{}{
					"redis_client": map[string]interface{}{
						"host": "redis",
						"port": "6379",
						"db":   0,
					},
				},
			},
		},
		DependencyMap: map[string]protocol.DependencySpec{
			"llm": {
				ClassName: "rustic_ai.litellm.agent_ext.llm.LiteLLMResolver",
				Properties: map[string]interface{}{
					"model": "gpt-4o-mini",
				},
			},
		},
		Agents: []protocol.AgentSpec{
			{
				ID:          "g-1#a-0",
				Name:        "Echo Agent",
				Description: "Echo",
				ClassName:   "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
				DependencyMap: map[string]protocol.DependencySpec{
					"filesystem": {
						ClassName: "rustic_ai.core.guild.agent_ext.depends.filesystem.FileSystemResolver",
						Properties: map[string]interface{}{
							"protocol": "file",
						},
					},
				},
				Predicates: map[string]protocol.RuntimePredicate{
					"on_message": {PredicateType: protocol.PredicateJSONata, Expression: strPtr("true")},
				},
			},
		},
	}

	gm, agents := buildModels(spec, "org-1")

	require.Equal(t, "rustic_ai.redis.messaging.backend", gm.BackendModule)
	require.Equal(t, "RedisMessagingBackend", gm.BackendClass)
	require.Contains(t, gm.BackendConfig, "redis_client")
	require.Contains(t, gm.DependencyMap, "llm")
	llmEntry, ok := gm.DependencyMap["llm"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "rustic_ai.litellm.agent_ext.llm.LiteLLMResolver", llmEntry["class_name"])

	require.Len(t, agents, 1)
	require.Contains(t, agents[0].DependencyMap, "filesystem")
	fsEntry, ok := agents[0].DependencyMap["filesystem"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "rustic_ai.core.guild.agent_ext.depends.filesystem.FileSystemResolver", fsEntry["class_name"])
	require.Contains(t, agents[0].Predicates, "on_message")
}

func TestBuildModels_PersistsRoutes(t *testing.T) {
	routeTimes := 1
	spec := &protocol.GuildSpec{
		ID:          "g-routes",
		Name:        "Guild Routes",
		Description: "Guild with routing",
		Properties: map[string]interface{}{
			"messaging": map[string]interface{}{
				"backend_module": "rustic_ai.redis.messaging.backend",
				"backend_class":  "RedisMessagingBackend",
				"backend_config": map[string]interface{}{
					"redis_client": map[string]interface{}{"host": "redis", "port": "6379", "db": 0},
				},
			},
		},
		Routes: &protocol.RoutingSlip{
			Steps: []protocol.RoutingRule{
				{
					AgentType:  strPtr("rustic_ai.core.agents.utils.user_proxy_agent.UserProxyAgent"),
					MethodName: strPtr("unwrap_and_forward_message"),
					Destination: &protocol.RoutingDestination{
						Topics: protocol.TopicsFromSlice([]string{"echo_topic"}),
					},
					RouteTimes: &routeTimes,
				},
			},
		},
	}

	gm, _ := buildModels(spec, "org-1")

	require.Len(t, gm.Routes, 1)
	require.NotNil(t, gm.Routes[0].GuildID)
	require.Equal(t, "g-routes", *gm.Routes[0].GuildID)
	require.Equal(t, "unwrap_and_forward_message", *gm.Routes[0].MethodName)
	require.Equal(t, []string{"echo_topic"}, []string(gm.Routes[0].DestinationTopics))
}

func TestNormalizeRuntimeSpecIDs_AssignsGuildScopedDefaults(t *testing.T) {
	spec := &protocol.GuildSpec{
		Name:        "Guild",
		Description: "Guild description",
		Agents: []protocol.AgentSpec{
			{ID: "a-0", Name: "A"},
			{ID: "", Name: "B"},
			{ID: "custom", Name: "C"},
		},
	}

	normalizeRuntimeSpecIDs(spec, "g-123")

	require.Equal(t, "g-123", spec.ID)
	require.Equal(t, "g-123#a-0", spec.Agents[0].ID)
	require.Equal(t, "g-123#a-1", spec.Agents[1].ID)
	require.Equal(t, "custom", spec.Agents[2].ID)
}

func TestNormalizeAgentModelIDs_AssignsGuildScopedDefaults(t *testing.T) {
	models := []store.AgentModel{
		{ID: "a-0"},
		{ID: ""},
		{ID: "custom"},
	}

	normalizeAgentModelIDs(models, "g-123")

	require.Equal(t, "g-123#a-0", models[0].ID)
	require.Equal(t, "g-123#a-1", models[1].ID)
	require.Equal(t, "custom", models[2].ID)
}

func TestMergeDependencies_MissingConfigIsNoop(t *testing.T) {
	spec := &protocol.GuildSpec{
		DependencyMap: map[string]protocol.DependencySpec{
			"llm": {
				ClassName: "custom.llm.Resolver",
				Properties: map[string]interface{}{
					"model": "custom-model",
				},
			},
		},
	}

	err := mergeDependencies(spec, filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	require.NoError(t, err)

	// Missing file should be a no-op.
	require.Equal(t, "custom.llm.Resolver", spec.DependencyMap["llm"].ClassName)
	require.Len(t, spec.DependencyMap, 1)
}

func TestMergeDependencies_LoadsClassNameFromYAML(t *testing.T) {
	spec := &protocol.GuildSpec{
		DependencyMap: map[string]protocol.DependencySpec{},
	}

	cfg := []byte(`
filesystem:
  class_name: rustic_ai.core.guild.agent_ext.depends.filesystem.FileSystemResolver
  properties:
    path_base: /tmp
`)
	configPath := filepath.Join(t.TempDir(), "deps.yaml")
	require.NoError(t, os.WriteFile(configPath, cfg, 0o644))

	err := mergeDependencies(spec, configPath)
	require.NoError(t, err)

	require.Contains(t, spec.DependencyMap, "filesystem")
	require.Equal(
		t,
		"rustic_ai.core.guild.agent_ext.depends.filesystem.FileSystemResolver",
		spec.DependencyMap["filesystem"].ClassName,
	)
}
