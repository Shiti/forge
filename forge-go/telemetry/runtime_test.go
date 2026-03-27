package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestStart_ExternalOTLP_ExportsTraceAndMetric(t *testing.T) {
	var (
		mu     sync.Mutex
		counts = map[string]int{}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	rt, err := Start(context.Background(), Config{
		Enabled:        true,
		Mode:           TelemetryModeExternalOTLP,
		EndpointURL:    srv.URL,
		ServiceName:    "forge-test",
		ServiceVersion: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()
	_, span := otel.Tracer("telemetry-test").Start(ctx, "test-span")
	span.End()

	meter := otel.Meter("telemetry-test")
	counter, err := meter.Int64Counter("test.counter")
	require.NoError(t, err)
	counter.Add(ctx, 1)

	require.NoError(t, rt.tracerProvider.ForceFlush(ctx))
	require.NoError(t, rt.meterProvider.ForceFlush(ctx))
	require.NoError(t, rt.Shutdown(context.Background()))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return counts["/v1/traces"] > 0 && counts["/v1/metrics"] > 0
	}, 5*time.Second, 100*time.Millisecond)
}

func TestStart_DesktopSQLite_StartsConfiguredBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a shell script")
	}

	binPath := filepath.Join(t.TempDir(), "sqlite-otel")
	script := `#!/usr/bin/env bash
set -euo pipefail
port=4318
while [[ $# -gt 0 ]]; do
  case "$1" in
    -port)
      port="$2"
      shift 2
      ;;
    -db-path)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
exec python3 - "$port" <<'PY'
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
port = int(sys.argv[1])
class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        if length:
            self.rfile.read(length)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"{}")
    def log_message(self, *args):
        return
HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
`
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	rt, err := Start(context.Background(), Config{
		Enabled:          true,
		Mode:             TelemetryModeDesktopSQLite,
		ServiceName:      "forge-test",
		ServiceVersion:   "test",
		SQLiteBinaryPath: binPath,
		SQLiteDBPath:     filepath.Join(t.TempDir(), "telemetry.db"),
		SQLitePort:       4319,
	})
	require.NoError(t, err)
	require.NotNil(t, rt.sidecar)

	ctx := context.Background()
	_, span := otel.Tracer("telemetry-test").Start(ctx, "sidecar-span")
	span.End()
	require.NoError(t, rt.tracerProvider.ForceFlush(ctx))
	require.NoError(t, rt.Shutdown(context.Background()))
}

func TestStart_ValidateConfig(t *testing.T) {
	_, err := Start(context.Background(), Config{
		Enabled: true,
		Mode:    TelemetryModeExternalOTLP,
	})
	require.ErrorContains(t, err, "requires an OTLP endpoint URL")

	_, err = Start(context.Background(), Config{
		Enabled: true,
		Mode:    TelemetryModeDesktopSQLite,
	})
	require.ErrorContains(t, err, "requires a sqlite-otel binary path")
}
