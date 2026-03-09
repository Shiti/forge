package gateway

import (
	"encoding/json"
	"testing"
)

func TestEnsureHumanParticipant_AppendsWhenMissing(t *testing.T) {
	in := json.RawMessage(`{"participants":[{"id":"a1","name":"Echo Agent","type":"bot"}]}`)
	out := ensureHumanParticipant(in, "dummyuserid")

	var body map[string]interface{}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	parts, ok := body["participants"].([]interface{})
	if !ok {
		t.Fatalf("participants missing")
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(parts))
	}
}

func TestEnsureHumanParticipant_NoopWhenHumanExists(t *testing.T) {
	in := json.RawMessage(`{"participants":[{"id":"upa-dummyuserid","name":"Anonymous User","type":"human"}]}`)
	out := ensureHumanParticipant(in, "dummyuserid")
	if string(out) != string(in) {
		t.Fatalf("expected payload unchanged")
	}
}
