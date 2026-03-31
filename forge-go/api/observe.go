package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"github.com/rustic-ai/forge/forge-go/telemetry"
	_ "modernc.org/sqlite"
)

const defaultObserveLookbackMs = 86400000

var errObserveUnsupported = errors.New("observability spans query is only available with desktop sqlite telemetry")

type observeService struct {
	mode         string
	sqliteDBPath string
}

type observeTraceSpan struct {
	TraceID     string                   `json:"traceId"`
	ID          string                   `json:"id"`
	ParentID    string                   `json:"parentId,omitempty"`
	Name        string                   `json:"name,omitempty"`
	Timestamp   int64                    `json:"timestamp,omitempty"`
	Duration    int64                    `json:"duration,omitempty"`
	Tags        map[string]string        `json:"tags,omitempty"`
	Annotations []observeTraceAnnotation `json:"annotations,omitempty"`
}

type observeTraceAnnotation struct {
	Timestamp int64  `json:"timestamp"`
	Value     string `json:"value"`
}

type observeSpanRow struct {
	TraceID           string
	SpanID            string
	ParentSpanID      sql.NullString
	Name              sql.NullString
	StartTimeUnixNano sql.NullInt64
	EndTimeUnixNano   sql.NullInt64
	AttributesJSON    sql.NullString
	EventsJSON        sql.NullString
	ResourceJSON      sql.NullString
}

type observeSearchRequest struct {
	GuildID      string
	MsgID        string
	RootThreadID string
	DurationMs   int64
}

func newObserveService(mode, sqliteDBPath string) *observeService {
	mode = strings.TrimSpace(strings.ToLower(mode))
	sqliteDBPath = strings.TrimSpace(sqliteDBPath)
	if mode == "" && sqliteDBPath != "" {
		mode = telemetry.TelemetryModeDesktopSQLite
	}
	return &observeService{
		mode:         mode,
		sqliteDBPath: sqliteDBPath,
	}
}

func (s *Server) handleObserveMessageSpans() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.observeService == nil {
			s.observeService = newObserveService("", "")
		}

		durationMs := int64(defaultObserveLookbackMs)
		if raw := strings.TrimSpace(r.URL.Query().Get("durationInMs")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				ReplyError(w, http.StatusBadRequest, "invalid durationInMs")
				return
			}
			durationMs = parsed
		}

		result, err := s.observeService.GetMessageSpans(r.Context(), observeSearchRequest{
			GuildID:      r.PathValue("guild_id"),
			MsgID:        r.PathValue("msg_id"),
			RootThreadID: strings.TrimSpace(r.URL.Query().Get("rootThreadId")),
			DurationMs:   durationMs,
		})
		if err != nil {
			switch {
			case errors.Is(err, errObserveUnsupported):
				ReplyError(w, http.StatusNotImplemented, err.Error())
			default:
				ReplyError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		ReplyJSON(w, http.StatusOK, result)
	}
}

func (s *observeService) GetMessageSpans(_ context.Context, req observeSearchRequest) ([]observeTraceSpan, error) {
	if s == nil {
		return nil, errObserveUnsupported
	}
	if s.mode == "" {
		return nil, errObserveUnsupported
	}
	if s.mode != telemetry.TelemetryModeDesktopSQLite {
		return nil, errObserveUnsupported
	}

	dbPath := s.sqliteDBPath
	if dbPath == "" {
		dbPath = filepath.Join(forgepath.Resolve("telemetry"), "sqlite-otel.db")
	}
	if dbPath == "" {
		return nil, errObserveUnsupported
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite-otel db: %w", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`
		SELECT
			s.trace_id,
			s.span_id,
			s.parent_span_id,
			s.name,
			s.start_time_unix_nano,
			s.end_time_unix_nano,
			s.attributes,
			s.events,
			r.attributes
		FROM spans s
		LEFT JOIN resources r ON r.id = s.resource_id
		WHERE s.start_time_unix_nano >= ?
		ORDER BY s.start_time_unix_nano DESC
	`, observeLookbackThresholdNanos(req.DurationMs))
	if err != nil {
		return nil, fmt.Errorf("query sqlite-otel spans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	rootThreadStr := observeMessageTagValue(req.MsgID)
	if req.RootThreadID != "" {
		rootThreadStr = observeMessageTagValue(req.RootThreadID)
	}
	msgSearchStr := observeMessageTagValue(req.MsgID)

	var matches []observeSpanRow
	for rows.Next() {
		var row observeSpanRow
		if err := rows.Scan(
			&row.TraceID,
			&row.SpanID,
			&row.ParentSpanID,
			&row.Name,
			&row.StartTimeUnixNano,
			&row.EndTimeUnixNano,
			&row.AttributesJSON,
			&row.EventsJSON,
			&row.ResourceJSON,
		); err != nil {
			return nil, fmt.Errorf("scan sqlite-otel span row: %w", err)
		}
		if observeRowMatches(row, req.GuildID, msgSearchStr, rootThreadStr) {
			matches = append(matches, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite-otel span rows: %w", err)
	}
	if len(matches) == 0 {
		return []observeTraceSpan{}, nil
	}

	selectedTraceID := selectObserveTrace(matches)
	filtered := make([]observeTraceSpan, 0, len(matches))
	for _, row := range matches {
		if row.TraceID != selectedTraceID {
			continue
		}
		filtered = append(filtered, convertObserveRow(row))
	}
	slices.SortFunc(filtered, func(a, b observeTraceSpan) int {
		switch {
		case a.Timestamp < b.Timestamp:
			return -1
		case a.Timestamp > b.Timestamp:
			return 1
		default:
			return strings.Compare(a.ID, b.ID)
		}
	})
	return filtered, nil
}

func observeLookbackThresholdNanos(durationMs int64) int64 {
	if durationMs <= 0 {
		durationMs = defaultObserveLookbackMs
	}
	return time.Now().Add(-time.Duration(durationMs) * time.Millisecond).UnixNano()
}

func observeMessageTagValue(id string) string {
	return "id:" + strings.TrimSpace(id)
}

func observeRowMatches(row observeSpanRow, guildID, msgSearchStr, rootThreadStr string) bool {
	attrs := observeMergedAttrs(row)
	if attrs["guild_id"] != guildID {
		return false
	}
	return attrs["root_thread_id"] == rootThreadStr ||
		attrs["message_id"] == msgSearchStr ||
		attrs["message_id"] == rootThreadStr
}

func selectObserveTrace(rows []observeSpanRow) string {
	bestTraceID := rows[0].TraceID
	bestTime := observeRowTime(rows[0])
	for _, row := range rows[1:] {
		rowTime := observeRowTime(row)
		if rowTime > bestTime {
			bestTraceID = row.TraceID
			bestTime = rowTime
		}
	}
	return bestTraceID
}

func observeRowTime(row observeSpanRow) int64 {
	if row.EndTimeUnixNano.Valid {
		return row.EndTimeUnixNano.Int64
	}
	if row.StartTimeUnixNano.Valid {
		return row.StartTimeUnixNano.Int64
	}
	return 0
}

func convertObserveRow(row observeSpanRow) observeTraceSpan {
	start := int64(0)
	if row.StartTimeUnixNano.Valid {
		start = row.StartTimeUnixNano.Int64 / 1_000
	}
	duration := int64(0)
	if row.StartTimeUnixNano.Valid && row.EndTimeUnixNano.Valid && row.EndTimeUnixNano.Int64 >= row.StartTimeUnixNano.Int64 {
		duration = (row.EndTimeUnixNano.Int64 - row.StartTimeUnixNano.Int64) / 1_000
	}
	result := observeTraceSpan{
		TraceID:   row.TraceID,
		ID:        row.SpanID,
		Name:      row.Name.String,
		Timestamp: start,
		Duration:  duration,
		Tags:      observeMergedAttrs(row),
	}
	if row.ParentSpanID.Valid {
		result.ParentID = row.ParentSpanID.String
	}
	if annotations := decodeObserveAnnotations(row.EventsJSON.String); len(annotations) > 0 {
		result.Annotations = annotations
	}
	return result
}

func observeMergedAttrs(row observeSpanRow) map[string]string {
	result := decodeObserveAttrs(row.ResourceJSON.String)
	for key, value := range decodeObserveAttrs(row.AttributesJSON.String) {
		result[key] = value
	}
	return result
}

func decodeObserveAttrs(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		key, _ := item["key"].(string)
		if key == "" {
			continue
		}
		if value, ok := observeAttrValue(item["value"]); ok {
			result[key] = value
		}
	}
	return result
}

func observeAttrValue(raw any) (string, bool) {
	valueMap, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
		if value, exists := valueMap[key]; exists {
			return fmt.Sprint(value), true
		}
	}
	return "", false
}

func decodeObserveAnnotations(raw string) []observeTraceAnnotation {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	result := make([]observeTraceAnnotation, 0, len(items))
	for _, item := range items {
		value, _ := item["name"].(string)
		if value == "" {
			continue
		}
		tsRaw, _ := item["timeUnixNano"].(string)
		var ts int64
		if tsRaw != "" {
			if parsed, err := strconv.ParseInt(tsRaw, 10, 64); err == nil {
				ts = parsed / 1_000
			}
		}
		result = append(result, observeTraceAnnotation{
			Timestamp: ts,
			Value:     value,
		})
	}
	slices.SortFunc(result, func(a, b observeTraceAnnotation) int {
		switch {
		case a.Timestamp < b.Timestamp:
			return -1
		case a.Timestamp > b.Timestamp:
			return 1
		default:
			return strings.Compare(a.Value, b.Value)
		}
	})
	return result
}
