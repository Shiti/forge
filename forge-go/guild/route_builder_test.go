package guild_test

import (
	"testing"

	"github.com/rustic-ai/forge/forge-go/guild"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

func TestRouteBuilder_FromAgentTag(t *testing.T) {
	tag := protocol.AgentTag{
		ID:   strPtr("agent-1"),
		Name: strPtr("Agent One"),
	}

	rule, err := guild.NewRouteBuilder(tag).
		FromMethod("handleMessage").
		OnMessageFormat("text/plain").
		SetRouteTimes(3).
		MarkForwarded(true).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Agent == nil {
		t.Fatal("expected Agent to be set")
	}
	if *rule.Agent.ID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got %s", *rule.Agent.ID)
	}
	if rule.MethodName == nil || *rule.MethodName != "handleMessage" {
		t.Errorf("expected method name 'handleMessage'")
	}
	if rule.MessageFormat == nil || *rule.MessageFormat != "text/plain" {
		t.Errorf("expected message format 'text/plain'")
	}
	if rule.RouteTimes == nil || *rule.RouteTimes != 3 {
		t.Errorf("expected route times 3")
	}
	if !rule.MarkForwarded {
		t.Errorf("expected mark_forwarded=true")
	}
}

func TestRouteBuilder_FromAgentSpec(t *testing.T) {
	agentSpec := protocol.AgentSpec{
		ID:   "spec-agent",
		Name: "Spec Agent",
	}

	rule, err := guild.NewRouteBuilder(agentSpec).Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Agent == nil {
		t.Fatal("expected Agent to be set")
	}
	if *rule.Agent.ID != "spec-agent" {
		t.Errorf("expected agent ID 'spec-agent', got %s", *rule.Agent.ID)
	}
	if *rule.Agent.Name != "Spec Agent" {
		t.Errorf("expected agent name 'Spec Agent', got %s", *rule.Agent.Name)
	}
}

func TestRouteBuilder_FromString(t *testing.T) {
	rule, err := guild.NewRouteBuilder("my.agent.Class").Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.AgentType == nil || *rule.AgentType != "my.agent.Class" {
		t.Errorf("expected agent_type 'my.agent.Class'")
	}
	if rule.Agent != nil {
		t.Errorf("expected Agent to be nil for string source")
	}
}

func TestRouteBuilder_FilterOnOrigin(t *testing.T) {
	sender := &protocol.AgentTag{Name: strPtr("Sender")}
	topic := "my_topic"
	format := "json"

	rule, err := guild.NewRouteBuilder("my.Class").
		FilterOnOrigin(sender, &topic, &format).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.OriginFilter == nil {
		t.Fatal("expected origin filter to be set")
	}
	if rule.OriginFilter.OriginSender == nil || *rule.OriginFilter.OriginSender.Name != "Sender" {
		t.Errorf("expected origin sender name 'Sender'")
	}
	if rule.OriginFilter.OriginTopic == nil || *rule.OriginFilter.OriginTopic != "my_topic" {
		t.Errorf("expected origin topic 'my_topic'")
	}
	if rule.OriginFilter.OriginMessageFormat == nil || *rule.OriginFilter.OriginMessageFormat != "json" {
		t.Errorf("expected origin message format 'json'")
	}
}

func TestRouteBuilder_SetDestinationTopics(t *testing.T) {
	topics := protocol.TopicsFromSlice([]string{"topic_a", "topic_b"})

	rule, err := guild.NewRouteBuilder("my.Class").
		SetDestinationTopics(topics).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Destination == nil {
		t.Fatal("expected destination to be set")
	}
	topicSlice := rule.Destination.Topics.ToSlice()
	if len(topicSlice) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topicSlice))
	}
}

func TestRouteBuilder_AddRecipients(t *testing.T) {
	recipients := []protocol.AgentTag{
		{ID: strPtr("r1")},
		{ID: strPtr("r2")},
	}

	rule, err := guild.NewRouteBuilder("my.Class").
		AddRecipients(recipients).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Destination == nil {
		t.Fatal("expected destination to be set")
	}
	if len(rule.Destination.RecipientList) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(rule.Destination.RecipientList))
	}
}

func TestRouteBuilder_SetProcessStatus(t *testing.T) {
	rule, err := guild.NewRouteBuilder("my.Class").
		SetProcessStatus(protocol.ProcessStatusCompleted).
		SetReason("done").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.ProcessStatus == nil || *rule.ProcessStatus != protocol.ProcessStatusCompleted {
		t.Errorf("expected process status 'completed'")
	}
	if rule.Reason == nil || *rule.Reason != "done" {
		t.Errorf("expected reason 'done'")
	}
}

func TestRouteBuilder_UnsupportedType(t *testing.T) {
	_, err := guild.NewRouteBuilder(123).Build()
	if err == nil {
		t.Error("expected error for unsupported source type")
	}
}

func TestRouteBuilder_ErrorChaining(t *testing.T) {
	// Error from unsupported type should propagate through chain
	_, err := guild.NewRouteBuilder(123).
		FromMethod("test").
		OnMessageFormat("json").
		Build()

	if err == nil {
		t.Error("expected error to propagate through chain")
	}
}
