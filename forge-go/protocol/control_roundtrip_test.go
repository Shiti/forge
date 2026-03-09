package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestControlDTOs_JSONRoundTrip(t *testing.T) {
	listenToDefault := false
	actOnlyWhenTagged := false

	spawn := SpawnRequest{
		RequestID:      "req-1",
		OrganizationID: "acmeorganizationid",
		GuildID:        "guild-1",
		AgentSpec: AgentSpec{
			ID:                   "guild-1#a-0",
			Name:                 "Echo Agent",
			Description:          "Echo",
			ClassName:            "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
			AdditionalTopics:     []string{"echo_topic"},
			Properties:           map[string]interface{}{"temperature": 0.1},
			ListenToDefaultTopic: &listenToDefault,
			ActOnlyWhenTagged:    &actOnlyWhenTagged,
			Predicates:           map[string]RuntimePredicate{},
			DependencyMap: map[string]DependencySpec{
				"llm": {
					ClassName:  "rustic_ai.litellm.agent_ext.llm.LiteLLMResolver",
					Properties: map[string]interface{}{"model": "gpt-4o-mini"},
				},
			},
			AdditionalDependencies: []string{"forge-python"},
		},
		MessagingConfig: &MessagingConfig{
			BackendModule: "rustic_ai.redis.messaging.backend",
			BackendClass:  "RedisMessagingBackend",
			BackendConfig: map[string]interface{}{
				"redis_client": map[string]interface{}{"host": "redis", "port": "6379", "db": 0},
			},
		},
		MachineID:  3,
		ClientType: "forge",
		ClientProperties: JSONB{
			"guild_spec":      "{}",
			"organization_id": "acmeorganizationid",
		},
		TraceContext: map[string]string{"traceparent": "00-abcdef1234567890abcdef1234567890-abcdef1234567890-01"},
	}

	stop := StopRequest{RequestID: "req-2", OrganizationID: "acmeorganizationid", GuildID: "guild-1", AgentID: "guild-1#a-0"}
	spawnResp := SpawnResponse{RequestID: "req-1", Success: true, Message: "ok", NodeID: "node-1", PID: 1234}
	stopResp := StopResponse{RequestID: "req-2", Success: true, Message: "stopped"}
	errResp := ErrorResponse{RequestID: "req-3", Success: false, Error: "launch failed"}

	assertRoundTrip(t, spawn)
	assertRoundTrip(t, stop)
	assertRoundTrip(t, spawnResp)
	assertRoundTrip(t, stopResp)
	assertRoundTrip(t, errResp)
}

func assertRoundTrip[T any](t *testing.T, in T) {
	t.Helper()

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	inJSON, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input for compare: %v", err)
	}
	outJSON, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output for compare: %v", err)
	}

	var inObj map[string]interface{}
	var outObj map[string]interface{}
	if err := json.Unmarshal(inJSON, &inObj); err != nil {
		t.Fatalf("unmarshal input compare map: %v", err)
	}
	if err := json.Unmarshal(outJSON, &outObj); err != nil {
		t.Fatalf("unmarshal output compare map: %v", err)
	}
	if !reflect.DeepEqual(inObj, outObj) {
		t.Fatalf("round-trip key mismatch\nin=%s\nout=%s", string(inJSON), string(outJSON))
	}
}
