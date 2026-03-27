package telemetry

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusHandlerExportsFacadeMetrics(t *testing.T) {
	RecordAPIRequest("GET", "/healthz", "200", 25*time.Millisecond)
	AddAPIInflight("GET", "/healthz", 1)
	SetQueueDepth("forge:control:global", 3)
	SetNodesRegistered(2)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	PrometheusHandler().ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)

	for _, metricName := range []string{
		"forge_api_requests_total",
		"forge_api_request_duration_seconds",
		"forge_api_inflight_requests",
		"forge_queue_depth",
		"forge_nodes_registered_total",
	} {
		if !strings.Contains(text, metricName) {
			t.Fatalf("expected metric %q in output:\n%s", metricName, text)
		}
	}
}
