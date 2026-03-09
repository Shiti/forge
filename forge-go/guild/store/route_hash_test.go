package store

import (
	"testing"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

func TestRoutingRuleHash_ExcludesMutableFields(t *testing.T) {
	method := "unwrap_and_forward_message"
	agentType := "rustic_ai.core.agents.utils.user_proxy_agent.UserProxyAgent"
	running := protocol.ProcessStatusRunning
	reason := "temporary"
	routeTimesA := 1
	routeTimesB := 3

	a := &protocol.RoutingRule{
		AgentType:  &agentType,
		MethodName: &method,
		Destination: &protocol.RoutingDestination{
			Topics: protocol.TopicsFromSlice([]string{"echo_topic"}),
		},
		RouteTimes:    &routeTimesA,
		ProcessStatus: &running,
		Reason:        &reason,
	}
	b := &protocol.RoutingRule{
		AgentType:  &agentType,
		MethodName: &method,
		Destination: &protocol.RoutingDestination{
			Topics: protocol.TopicsFromSlice([]string{"echo_topic"}),
		},
		RouteTimes: &routeTimesB,
	}

	ha, err := RoutingRuleHash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	hb, err := RoutingRuleHash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}

	if ha != hb {
		t.Fatalf("expected equal hashes; got %s != %s", ha, hb)
	}
}

func TestRoutingRuleHash_StableDeterministic(t *testing.T) {
	method := "unwrap_and_forward_message"
	agentType := "rustic_ai.core.agents.utils.user_proxy_agent.UserProxyAgent"
	routeTimes := 1

	rule := &protocol.RoutingRule{
		AgentType:  &agentType,
		MethodName: &method,
		Destination: &protocol.RoutingDestination{
			Topics: protocol.TopicsFromSlice([]string{"echo_topic"}),
		},
		RouteTimes: &routeTimes,
	}

	first, err := RoutingRuleHash(rule)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := RoutingRuleHash(rule)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}

	if first != second {
		t.Fatalf("expected deterministic hash; got %s != %s", first, second)
	}
}
