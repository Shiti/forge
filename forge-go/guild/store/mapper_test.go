package store_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

func TestMapper_GuildSpec(t *testing.T) {
	// 1. Create a rich domain GuildSpec
	original := &protocol.GuildSpec{
		ID:          "mapper-test-id",
		Name:        "Mapper Guild",
		Description: "A guild for testing mappers",
		Properties: map[string]interface{}{
			"execution_engine": "custom_engine",
			"messaging": map[string]interface{}{
				"backend_module": "custom_module",
				"backend_class":  "CustomClass",
				"backend_config": map[string]interface{}{
					"redis_url": "redis://localhost",
				},
			},
		},
		Agents: []protocol.AgentSpec{
			{
				ID:                   "agent-1",
				Name:                 "First Agent",
				Description:          "The first agent",
				ClassName:            "FirstClass",
				AdditionalTopics:     []string{"topic-a"},
				ListenToDefaultTopic: func() *bool { b := true; return &b }(),
				Properties: map[string]interface{}{
					"temperature": 0.5,
				},
			},
		},
		Routes: &protocol.RoutingSlip{
			Steps: []protocol.RoutingRule{
				{
					Agent: &protocol.AgentTag{
						Name: pointerStr("First Agent"),
					},
					OriginFilter: &protocol.RoutingOrigin{
						OriginTopic: pointerStr("topic-a"),
					},
					Destination: &protocol.RoutingDestination{
						Topics: protocol.TopicsFromSlice([]string{"topic-b"}),
						RecipientList: []protocol.AgentTag{
							{Name: pointerStr("Second Agent")},
						},
					},
					RouteTimes:  func(i int) *int { return &i }(2),
					Transformer: protocol.RawJSON(`{"style":"content_based_router","expression_type":"jsonata","handler":"custom"}`),
				},
			},
		},
	}

	// 2. Map to GORM Model
	model := store.FromGuildSpec(original, "org-id-1")

	// Validate Model fields
	if model.ID != original.ID || model.Name != original.Name {
		t.Errorf("Top level ID/Name mismatch")
	}
	if model.ExecutionEngine != "custom_engine" {
		t.Errorf("Execution engine mismatch")
	}
	if model.BackendClass != "CustomClass" {
		t.Errorf("Messaging config mismatch")
	}
	if len(model.Agents) != 1 {
		t.Fatalf("Expected 1 mapped agent")
	}
	if model.Agents[0].ClassName != "FirstClass" {
		t.Errorf("Agent ClassName mismatch")
	}
	if model.Agents[0].Properties["temperature"] != 0.5 {
		t.Errorf("Agent Properties mismatch")
	}
	if len(model.Routes) != 1 {
		t.Fatalf("Expected 1 mapped route")
	}
	if model.Routes[0].RouteTimes != 2 {
		t.Errorf("Route route times mismatch")
	}

	// 3. Map back to Domain Spec
	reconstructed := store.ToGuildSpec(model)

	if reconstructed.ID != original.ID {
		t.Errorf("Reconstructed ID mismatch")
	}
	if len(reconstructed.Agents) != 1 {
		t.Fatalf("Reconstructed missing agents")
	}
	if len(reconstructed.Agents[0].AdditionalTopics) != 1 || reconstructed.Agents[0].AdditionalTopics[0] != "topic-a" {
		t.Errorf("Reconstructed additional topics mismatch")
	}

	// Compare routes cleanly
	if len(reconstructed.Routes.Steps) != 1 {
		t.Fatalf("Reconstructed routes missing")
	}
	step := reconstructed.Routes.Steps[0]
	if step.Agent == nil || *step.Agent.Name != "First Agent" {
		t.Errorf("Reconstructed route agent name mismatch")
	}

	destTopics := step.Destination.Topics.ToSlice()
	if len(destTopics) != 1 || destTopics[0] != "topic-b" {
		t.Errorf("Reconstructed destination topics mismatch")
	}

	var transformer map[string]interface{}
	if err := json.Unmarshal([]byte(step.Transformer), &transformer); err != nil {
		t.Fatalf("expected reconstructed transformer to be valid json: %v", err)
	}
	if transformer["style"] != "content_based_router" || transformer["handler"] != "custom" || transformer["expression_type"] != "jsonata" {
		t.Errorf("Reconstructed transformer mismatch")
	}

	destList := step.Destination.RecipientList
	if len(destList) != 1 || *destList[0].Name != "Second Agent" {
		t.Errorf("Reconstructed route recipient list mismatch")
	}

	if !reflect.DeepEqual(reconstructed.Agents[0].Properties, original.Agents[0].Properties) {
		t.Errorf("Reconstructed agent properties deep equal failed")
	}
}
