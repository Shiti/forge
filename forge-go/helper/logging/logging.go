package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const loggerFieldsKey = contextKey("logger_fields")

// TraceHandler wraps an slog.Handler to inject OpenTelemetry trace and span IDs.
type TraceHandler struct {
	slog.Handler
}

// Handle adds the trace_id and span_id to the log record if available in the context.
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new TraceHandler with the additional attributes.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new TraceHandler with the given group name.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{Handler: h.Handler.WithGroup(name)}
}

// NewLogger creates a new configured JSON slog.Logger injected with tracing attributes.
func NewLogger(out io.Writer, levelStr string) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}
	innerHandler := slog.NewJSONHandler(out, opts)
	traceHandler := &TraceHandler{Handler: innerHandler}

	return slog.New(traceHandler)
}

// WithField adds a key-value attribute to the context logger state.
func WithField(ctx context.Context, key string, val interface{}) context.Context {
	fields, _ := ctx.Value(loggerFieldsKey).([]slog.Attr)

	newFields := make([]slog.Attr, len(fields), len(fields)+1)
	copy(newFields, fields)
	newFields = append(newFields, slog.Any(key, val))

	return context.WithValue(ctx, loggerFieldsKey, newFields)
}

// FromContext extracts contextual fields and returns an augmented logger instance.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	fields, ok := ctx.Value(loggerFieldsKey).([]slog.Attr)
	if !ok || len(fields) == 0 {
		return base
	}

	var anyFields []any
	for _, f := range fields {
		anyFields = append(anyFields, f)
	}

	return base.With(anyFields...)
}

// SetGlobalLogger replaces the default global logger.
func SetGlobalLogger(l *slog.Logger) {
	slog.SetDefault(l)
}
