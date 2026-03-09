package telemetry

import (
	"testing"
)

func TestMetricsDefinition(t *testing.T) {
	if APIRequestsTotal == nil {
		t.Fatal("APIRequestsTotal not initialized")
	}
	if QueueDepth == nil {
		t.Fatal("QueueDepth not initialized")
	}
	if NodesRegistered == nil {
		t.Fatal("NodesRegistered not initialized")
	}
}
