package contract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLaunchGuildReq_JSONRoundTrip_PreservesRoutesAndSpec(t *testing.T) {
	raw := []byte(`{
		"org_id": "acmeorganizationid",
		"spec": {
			"id": "guild-rt-1",
			"name": "Simple Echo",
			"description": "Echo guild",
			"properties": {
				"execution_engine": "rustic_ai.forge.execution_engine.ForgeExecutionEngine",
				"messaging": {
					"backend_module": "rustic_ai.redis.messaging.backend",
					"backend_class": "RedisMessagingBackend",
					"backend_config": {"redis_client": {"host":"redis","port":"6379","db":0}}
				}
			},
			"agents": [{
				"id": "guild-rt-1#a-0",
				"name": "Echo Agent",
				"description": "Echo",
				"class_name": "rustic_ai.core.agents.testutils.echo_agent.EchoAgent",
				"additional_topics": ["echo_topic"],
				"properties": {},
				"listen_to_default_topic": false,
				"act_only_when_tagged": false,
				"predicates": {},
				"dependency_map": {},
				"additional_dependencies": []
			}],
			"dependency_map": {},
			"routes": {
				"steps": [{
					"agent_type": "rustic_ai.core.agents.utils.user_proxy_agent.UserProxyAgent",
					"method_name": "unwrap_and_forward_message",
					"destination": {"topics": "echo_topic", "recipient_list": [], "priority": null},
					"mark_forwarded": false,
					"route_times": 1,
					"transformer": {"style":"simple","expression_type":"jsonata","output_format":"generic_json","expression":null},
					"agent_state_update": {"expression_type":"jsonata","update_format":"json-merge-patch","state_update":null},
					"guild_state_update": {"expression_type":"jsonata","update_format":"json-merge-patch","state_update":null},
					"process_status": null,
					"reason": null
				}]
			}
		}
	}`)

	var req LaunchGuildReq
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal LaunchGuildReq: %v", err)
	}

	gotJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal LaunchGuildReq: %v", err)
	}

	var gotReq LaunchGuildReq
	if err := json.Unmarshal(gotJSON, &gotReq); err != nil {
		t.Fatalf("unmarshal round-tripped LaunchGuildReq: %v", err)
	}

	if !reflect.DeepEqual(req, gotReq) {
		t.Fatalf("launch guild request semantic round-trip mismatch\ninitial=%+v\nroundtrip=%+v", req, gotReq)
	}
}

func TestUnionTopics_JSONRoundTrip_StringAndList(t *testing.T) {
	stringCase := []byte(`{"topics":"echo_topic","recipient_list":[]}`)
	listCase := []byte(`{"topics":["echo_topic","user_message_broadcast"],"recipient_list":[]}`)

	assertRoutingDestinationRoundTrip(t, stringCase)
	assertRoutingDestinationRoundTrip(t, listCase)
}

func assertRoutingDestinationRoundTrip(t *testing.T, raw []byte) {
	t.Helper()

	var dest RoutingDestination
	if err := json.Unmarshal(raw, &dest); err != nil {
		t.Fatalf("unmarshal routing destination: %v", err)
	}

	gotJSON, err := json.Marshal(dest)
	if err != nil {
		t.Fatalf("marshal routing destination: %v", err)
	}

	var wantObj map[string]interface{}
	var gotObj map[string]interface{}
	if err := json.Unmarshal(raw, &wantObj); err != nil {
		t.Fatalf("unmarshal want map: %v", err)
	}
	if err := json.Unmarshal(gotJSON, &gotObj); err != nil {
		t.Fatalf("unmarshal got map: %v", err)
	}

	if !reflect.DeepEqual(wantObj, gotObj) {
		t.Fatalf("routing destination round-trip mismatch\nwant=%s\ngot=%s", string(raw), string(gotJSON))
	}
}
