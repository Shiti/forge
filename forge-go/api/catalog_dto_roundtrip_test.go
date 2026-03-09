package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBlueprintCreateRequest_JSONRoundTrip(t *testing.T) {
	raw := []byte(`{
		"name":"Simple Echo",
		"description":"Echo app",
		"exposure":"public",
		"author_id":"dummyuserid",
		"organization_id":"acmeorganizationid",
		"category_id":null,
		"version":"v1",
		"icon":null,
		"intro_msg":"Hi there! I will echo back anything you send me.",
		"spec":{
			"name":"Simple Echo",
			"description":"Echo app",
			"properties":{},
			"agents":[{"name":"Echo Agent","class_name":"rustic_ai.core.agents.testutils.echo_agent.EchoAgent","additional_topics":["echo_topic"],"properties":{}}],
			"dependency_map":{},
			"routes":{"steps":[{"agent_type":"rustic_ai.core.agents.utils.user_proxy_agent.UserProxyAgent","method_name":"unwrap_and_forward_message","destination":{"topics":"echo_topic","recipient_list":[],"priority":null},"mark_forwarded":false,"route_times":1}]}
		},
		"tags":[],
		"commands":[],
		"starter_prompts":[],
		"agent_icons":{"Echo Agent":"https://example.com/icon.png"}
	}`)

	var req BlueprintCreateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal blueprint create request: %v", err)
	}

	gotJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal blueprint create request: %v", err)
	}

	var gotReq BlueprintCreateRequest
	if err := json.Unmarshal(gotJSON, &gotReq); err != nil {
		t.Fatalf("unmarshal round-tripped blueprint create request: %v", err)
	}
	assertStructJSONEqual(t, req, gotReq)
}

func TestLaunchGuildFromBlueprintRequest_JSONRoundTrip(t *testing.T) {
	emptyCfg := []byte(`{"guild_id":"g-1","guild_name":"test001","user_id":"dummyuserid","org_id":"acmeorganizationid","description":"desc","configuration":{}}`)
	nilCfg := []byte(`{"guild_id":"g-2","guild_name":"test002","user_id":"dummyuserid","org_id":"acmeorganizationid","description":"desc","configuration":null}`)

	assertLaunchGuildDTOCase(t, emptyCfg)
	assertLaunchGuildDTOCase(t, nilCfg)
}

func assertLaunchGuildDTOCase(t *testing.T, raw []byte) {
	t.Helper()
	var req LaunchGuildFromBlueprintRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal launch request: %v", err)
	}
	gotJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal launch request: %v", err)
	}
	var gotReq LaunchGuildFromBlueprintRequest
	if err := json.Unmarshal(gotJSON, &gotReq); err != nil {
		t.Fatalf("unmarshal round-tripped launch request: %v", err)
	}
	assertStructJSONEqual(t, req, gotReq)
}

func assertJSONObjectsEqual(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	var wantObj map[string]interface{}
	var gotObj map[string]interface{}
	if err := json.Unmarshal(wantRaw, &wantObj); err != nil {
		t.Fatalf("unmarshal want object: %v", err)
	}
	if err := json.Unmarshal(gotRaw, &gotObj); err != nil {
		t.Fatalf("unmarshal got object: %v", err)
	}
	if !reflect.DeepEqual(wantObj, gotObj) {
		t.Fatalf("json round-trip mismatch\nwant=%s\ngot=%s", string(wantRaw), string(gotRaw))
	}
}

func assertStructJSONEqual(t *testing.T, want, got interface{}) {
	t.Helper()
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want struct: %v", err)
	}
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got struct: %v", err)
	}
	assertJSONObjectsEqual(t, wantRaw, gotRaw)
}
