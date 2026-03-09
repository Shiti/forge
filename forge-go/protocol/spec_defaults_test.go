package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewGuildSpec_DefaultParity(t *testing.T) {
	spec := NewGuildSpec()

	if spec.ID == "" {
		t.Fatalf("expected guild id default")
	}
	if spec.Properties == nil {
		t.Fatalf("expected properties default map")
	}
	if spec.Agents == nil {
		t.Fatalf("expected agents default slice")
	}
	if spec.DependencyMap == nil {
		t.Fatalf("expected dependency_map default map")
	}
	if spec.Routes == nil {
		t.Fatalf("expected routes default routing slip")
	}
	if spec.Routes.Steps == nil {
		t.Fatalf("expected routes.steps default slice")
	}
}

func TestGuildSpecUnmarshal_Defaults(t *testing.T) {
	raw := []byte(`{
		"name":"Simple Echo",
		"description":"echo",
		"agents":[{"name":"Echo Agent","description":"echo","class_name":"rustic_ai.core.agents.testutils.echo_agent.EchoAgent"}]
	}`)

	var spec GuildSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("unmarshal guild spec: %v", err)
	}

	if spec.ID == "" {
		t.Fatalf("expected guild id default")
	}
	if spec.Properties == nil || spec.DependencyMap == nil {
		t.Fatalf("expected non-nil guild maps")
	}
	if spec.Routes == nil || spec.Routes.Steps == nil {
		t.Fatalf("expected default routing slip")
	}
	if len(spec.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(spec.Agents))
	}

	agent := spec.Agents[0]
	if agent.ID == "" {
		t.Fatalf("expected agent id default")
	}
	if agent.AdditionalTopics == nil || agent.Properties == nil {
		t.Fatalf("expected default agent list/map fields")
	}
	if agent.Predicates == nil || agent.DependencyMap == nil {
		t.Fatalf("expected default agent predicate/dependency maps")
	}
	if agent.AdditionalDependencies == nil {
		t.Fatalf("expected default additional_dependencies")
	}
	if agent.ListenToDefaultTopic == nil || !*agent.ListenToDefaultTopic {
		t.Fatalf("expected listen_to_default_topic default true")
	}
	if agent.ActOnlyWhenTagged == nil || *agent.ActOnlyWhenTagged {
		t.Fatalf("expected act_only_when_tagged default false")
	}
	if agent.Resources.CustomResources == nil {
		t.Fatalf("expected default resource custom_resources")
	}
}

func TestGatewayConfigUnmarshal_EnabledDefaultTrue(t *testing.T) {
	raw := []byte(`{"input_formats":["f1"],"output_formats":["f2"]}`)

	var cfg GatewayConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal gateway config: %v", err)
	}

	if !cfg.Enabled {
		t.Fatalf("expected enabled default true")
	}
	if cfg.ReturnedFormats == nil {
		t.Fatalf("expected returned_formats default slice")
	}
}

func TestRoutingRuleUnmarshal_DefaultRouteTimes(t *testing.T) {
	raw := []byte(`{"agent_type":"x","method_name":"y"}`)

	var rule RoutingRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		t.Fatalf("unmarshal routing rule: %v", err)
	}

	if rule.RouteTimes == nil || *rule.RouteTimes != 1 {
		t.Fatalf("expected route_times default 1")
	}
}
