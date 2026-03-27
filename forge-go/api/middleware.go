package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// responseWriter is a wrapper around http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack exposes the underlying hijacker interface if the wrapped ResponseWriter supports it.
// This is essential for gorilla/websocket to upgrade HTTP connections to WebSockets.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

// WithLogging logs the request duration, method, path, and status code.
func WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		statusCode := strconv.Itoa(rw.status)

		telemetry.RecordAPIRequest(r.Method, r.URL.Path, statusCode, duration)

		logger := slog.Default()
		logger.Info("HTTP Request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", duration,
			"remote_ip", r.RemoteAddr,
		)
	})
}

// WithTelemetry wraps the request with OpenTelemetry Context Extraction and Prometheus inflight tracking.
// It also sets up standard span tracking via otelhttp.
func WithTelemetry(operation string, next http.Handler) http.Handler {
	// First, wrap with otelhttp which handles the W3C traceparent extraction
	// and automatically creates the server span.
	otelHandler := otelhttp.NewHandler(next, operation)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.AddAPIInflight(r.Method, r.URL.Path, 1)
		defer telemetry.AddAPIInflight(r.Method, r.URL.Path, -1)

		otelHandler.ServeHTTP(w, r)
	})
}

// WithRecovery catches panics and returns a 500 Internal Server Error.
func WithRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("HTTP Panic Recovered", "error", err, "path", r.URL.Path)
				ReplyError(w, http.StatusInternalServerError, "Internal Server Error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// WithCORS adds generous CORS headers to allow browser clients (like Atelier) to connect.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// WithJSONResponse ensures the response content type is application/json.
func WithJSONResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// WithPathNormalization rewrites non-root trailing-slash paths to their canonical
// no-trailing-slash form so `/x/` and `/x` route consistently.
func WithPathNormalization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL != nil && len(r.URL.Path) > 1 && strings.HasSuffix(r.URL.Path, "/") {
			trimmed := strings.TrimRight(r.URL.Path, "/")
			if trimmed == "" {
				trimmed = "/"
			}
			clone := r.Clone(r.Context())
			clone.URL.Path = trimmed
			if clone.RequestURI != "" {
				if clone.URL.RawQuery != "" {
					clone.RequestURI = trimmed + "?" + clone.URL.RawQuery
				} else {
					clone.RequestURI = trimmed
				}
			}
			next.ServeHTTP(w, clone)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ReplyJSON is a helper to serialize an object and write it to the response.
func ReplyJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			slog.Error("Failed to encode JSON response", "error", err)
		}
	}
}

// ReplyError is a helper to write a standardized JSON error response.
func ReplyError(w http.ResponseWriter, status int, message string) {
	ReplyJSON(w, status, map[string]string{"detail": message})
}
