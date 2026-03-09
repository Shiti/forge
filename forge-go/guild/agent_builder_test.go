package guild_test

import (
	"testing"

	"github.com/rustic-ai/forge/forge-go/guild"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

func TestAgentBuilder_Fluent_Full(t *testing.T) {
	spec, err := guild.NewAgentBuilder().
		SetID("agent-1").
		SetName("Test Agent").
		SetDescription("A test agent").
		SetClassName("my.agent.Class").
		AddAdditionalTopic("topic1").
		AddAdditionalTopic("topic2").
		ListenToDefaultTopic(false).
		ActOnlyWhenTagged(true).
		AddDependencyResolver("dep1", protocol.DependencySpec{ClassName: "DepClass"}).
		AddPredicate("handleMessage", protocol.RuntimePredicate{PredicateType: protocol.PredicateJSONata, Expression: strPtr("value")}).
		AddAdditionalDependency("extra_dep").
		BuildSpec()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.ID != "agent-1" {
		t.Errorf("expected ID 'agent-1', got %s", spec.ID)
	}
	if spec.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %s", spec.Name)
	}
	if spec.Description != "A test agent" {
		t.Errorf("expected description 'A test agent', got %s", spec.Description)
	}
	if spec.ClassName != "my.agent.Class" {
		t.Errorf("expected className 'my.agent.Class', got %s", spec.ClassName)
	}
	if len(spec.AdditionalTopics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(spec.AdditionalTopics))
	}
	if *spec.ListenToDefaultTopic != false {
		t.Errorf("expected ListenToDefaultTopic=false")
	}
	if *spec.ActOnlyWhenTagged != true {
		t.Errorf("expected ActOnlyWhenTagged=true")
	}
	if spec.DependencyMap["dep1"].ClassName != "DepClass" {
		t.Errorf("expected dep1 ClassName 'DepClass'")
	}
	pred := spec.Predicates["handleMessage"]
	if pred.PredicateType != protocol.PredicateJSONata || pred.Expression == nil || *pred.Expression != "value" {
		t.Errorf("expected predicate with jsonata_fn type and expression=value")
	}
	if len(spec.AdditionalDependencies) != 1 || spec.AdditionalDependencies[0] != "extra_dep" {
		t.Errorf("expected additional dependency 'extra_dep'")
	}
}

func TestAgentBuilder_Fluent_MissingName(t *testing.T) {
	_, err := guild.NewAgentBuilder().
		SetDescription("desc").
		SetClassName("class").
		BuildSpec()

	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestAgentBuilder_Fluent_MissingDescription(t *testing.T) {
	_, err := guild.NewAgentBuilder().
		SetName("Test").
		SetClassName("class").
		BuildSpec()

	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestAgentBuilder_Fluent_MissingClassName(t *testing.T) {
	_, err := guild.NewAgentBuilder().
		SetName("Test").
		SetDescription("desc").
		BuildSpec()

	if err == nil {
		t.Error("expected error for missing class_name")
	}
}

func TestAgentBuilder_Fluent_EmptyID(t *testing.T) {
	_, err := guild.NewAgentBuilder().
		SetID("").
		SetName("Test").
		SetDescription("desc").
		SetClassName("class").
		BuildSpec()

	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestAgentBuilder_Fluent_NegativeCPUs(t *testing.T) {
	b := guild.NewAgentBuilder().
		SetName("Test").
		SetDescription("desc").
		SetClassName("class")

	// ValidateResources should catch this
	err := b.ValidateResources()
	if err != nil {
		t.Errorf("expected no error for zero resources, got %v", err)
	}
}

func TestAgentBuilder_Fluent_DefaultsApplied(t *testing.T) {
	spec, err := guild.NewAgentBuilder().
		SetName("Test").
		SetDescription("desc").
		SetClassName("class").
		BuildSpec()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Properties == nil {
		t.Error("expected Properties to be initialized")
	}
	if spec.Predicates == nil {
		t.Error("expected Predicates to be initialized")
	}
	if spec.DependencyMap == nil {
		t.Error("expected DependencyMap to be initialized")
	}
	if spec.Resources.CustomResources == nil {
		t.Error("expected CustomResources to be initialized")
	}
	if spec.ListenToDefaultTopic == nil || *spec.ListenToDefaultTopic != true {
		t.Error("expected ListenToDefaultTopic default true")
	}
	if spec.ActOnlyWhenTagged == nil || *spec.ActOnlyWhenTagged != false {
		t.Error("expected ActOnlyWhenTagged default false")
	}
}

func TestAgentBuilder_Fluent_SetProperties(t *testing.T) {
	spec, err := guild.NewAgentBuilder().
		SetName("Test").
		SetDescription("desc").
		SetClassName("class").
		SetProperties(map[string]interface{}{"key": "value"}).
		BuildSpec()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Properties["key"] != "value" {
		t.Errorf("expected key=value in properties")
	}
}

func TestAgentBuilder_ErrorChaining(t *testing.T) {
	_, err := guild.NewAgentBuilder().
		SetName("").
		SetDescription("should not matter").
		SetClassName("class").
		BuildSpec()

	if err == nil {
		t.Error("expected error from empty name in chain")
	}
}

func TestAgentBuilder_AutoID(t *testing.T) {
	spec, err := guild.NewAgentBuilder().
		SetName("Test").
		SetDescription("desc").
		SetClassName("class").
		BuildSpec()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.ID == "" {
		t.Error("expected auto-generated ID")
	}
}
