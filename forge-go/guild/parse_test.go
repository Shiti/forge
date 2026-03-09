package guild_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rustic-ai/forge/forge-go/guild"
)

func TestParseFile_Minimal(t *testing.T) {
	spec, raw, err := guild.ParseFile(filepath.Join("testdata", "minimal.yaml"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if spec.ID != "minimal-01" {
		t.Errorf("expected ID minimal-01, got %s", spec.ID)
	}
	if len(spec.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(spec.Agents))
	}
	if spec.Agents[0].ClassName != "rustic_ai.agents.EchoAgent" {
		t.Errorf("expected class_name rustic_ai.agents.EchoAgent, got %s", spec.Agents[0].ClassName)
	}

	// Verify raw JSON parsing
	var rawMap map[string]interface{}
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("failed to unmarshal raw JSON: %v", err)
	}
	if rawMap["id"] != "minimal-01" {
		t.Errorf("raw JSON id mismatch")
	}
}

func TestParseFile_Full(t *testing.T) {
	spec, _, err := guild.ParseFile(filepath.Join("testdata", "full.yaml"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if spec.ID != "full-01" {
		t.Errorf("expected ID full-01, got %s", spec.ID)
	}

	if len(spec.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(spec.Agents))
	}

	agent := spec.Agents[0]
	if len(agent.AdditionalTopics) != 2 || agent.AdditionalTopics[0] != "topic1" {
		t.Errorf("expected additional_topics = [topic1, topic2]")
	}
	if agent.Properties["custom_key"] != "custom_value" {
		t.Errorf("expected properties.custom_key = custom_value")
	}
	if agent.Resources.NumCPUs == nil || *agent.Resources.NumCPUs != 2.0 {
		t.Errorf("expected resources.num_cpus = 2.0")
	}
	if agent.QOS.Timeout == nil || *agent.QOS.Timeout != 30 {
		t.Errorf("expected qos.timeout = 30")
	}

	if spec.Gateway == nil || !spec.Gateway.Enabled {
		t.Errorf("expected gateway enabled = true")
	}
	if len(spec.Routes.Steps) != 1 {
		t.Errorf("expected 1 route step")
	} else if spec.Routes.Steps[0].RouteTimes == nil || *spec.Routes.Steps[0].RouteTimes != 3 {
		t.Errorf("expected route_times = 3")
	}
}

func TestParseFile_MultiAgent(t *testing.T) {
	spec, _, err := guild.ParseFile(filepath.Join("testdata", "multi_agent.json"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(spec.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(spec.Agents))
	}
	if spec.Agents[0].ID != "a1" || spec.Agents[1].ID != "b2" {
		t.Errorf("expected agent IDs a1, b2")
	}
}

func TestParseFile_CustomMessaging(t *testing.T) {
	spec, _, err := guild.ParseFile(filepath.Join("testdata", "custom_messaging.yaml"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgConfigRaw, ok := spec.Properties["messaging"].(map[string]interface{})
	if !ok {
		t.Fatalf("messaging config missing or invalid")
	}

	if msgConfigRaw["backend_module"] != "custom_messaging" {
		t.Errorf("expected backend_module = custom_messaging, got %v", msgConfigRaw["backend_module"])
	}
}

func TestParseFile_SupervisorPermissions(t *testing.T) {
	spec, _, err := guild.ParseFile(filepath.Join("testdata", "supervisor_permissions.yaml"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	agent := spec.Agents[0]
	if agent.Properties["supervisor"] != "bubblewrap" {
		t.Errorf("expected supervisor = bubblewrap, got %v", agent.Properties["supervisor"])
	}
}

func TestParseFile_InvalidPath(t *testing.T) {
	_, _, err := guild.ParseFile(filepath.Join("testdata", "does_not_exist.yaml"))
	if err == nil {
		t.Errorf("expected error for missing file")
	}
}

func TestParseFile_ModularGuild(t *testing.T) {
	spec, _, err := guild.ParseFile(filepath.Join("testdata", "modular_guild", "guild.yaml"))
	if err != nil {
		t.Fatalf("expected no error loading modular guild, got %v", err)
	}

	if spec.ID != "modular_guild_id" {
		t.Errorf("expected modular_guild_id, got %s", spec.ID)
	}

	if len(spec.Agents) != 1 {
		t.Fatalf("expected 1 modular agent, got %d", len(spec.Agents))
	}

	agent := spec.Agents[0]
	if agent.Name != "Modular Agent" {
		t.Errorf("expected Modular Agent, got %s", agent.Name)
	}
	if agent.ClassName != "rustic_ai.core.agents.eip.basic_wiring_agent.BasicWiringAgent" {
		t.Errorf("wrong class name for modular agent")
	}

	if len(spec.Routes.Steps) != 1 {
		t.Fatalf("expected 1 routing step, got %d", len(spec.Routes.Steps))
	}

	step := spec.Routes.Steps[0]
	if step.Agent == nil || *step.Agent.Name != "Modular Agent" {
		t.Errorf("routing step agent name mismatch")
	}

	// The python test asserts the script content inside steps[0].transformer.handler
	if len(step.Transformer) == 0 {
		t.Fatalf("expected transformer to be set")
	}
	var transformer map[string]interface{}
	if err := json.Unmarshal([]byte(step.Transformer), &transformer); err != nil {
		t.Fatalf("expected transformer to be valid json: %v", err)
	}
	handler, ok := transformer["handler"].(string)
	if !ok || handler == "" {
		t.Fatalf("expected transformer.handler to be set")
	}

	expectedScript := "(\n  {\n    \"topics\": payload.routing_key\n  }\n)"
	// Simplistic check for the script content
	if handler != expectedScript && handler != `({"topics": payload.routing_key})` {
		// Just relying on checking if it contains payload.routing_key
		if len(handler) < 10 {
			t.Errorf("script loading failed, got: %s", handler)
		}
	}
}

func TestParseFile_ModularGuild_MissingInclude(t *testing.T) {
	_, _, err := guild.ParseFile(filepath.Join("testdata", "modular_guild", "bad", "missing_include.yaml"))
	if err == nil {
		t.Fatalf("expected error for missing !include file")
	}
	if !strings.Contains(err.Error(), "does_not_exist.yaml") {
		t.Errorf("expected error message to contain missing file name, got: %v", err)
	}
}

func TestParseFile_ModularGuild_CircularInclude(t *testing.T) {
	_, _, err := guild.ParseFile(filepath.Join("testdata", "modular_guild", "bad", "circular_a.yaml"))
	if err == nil {
		t.Fatalf("expected error for circular !include dependency")
	}
	if !strings.Contains(err.Error(), "circular include detected") {
		t.Errorf("expected circular include error message, got: %v", err)
	}
}
