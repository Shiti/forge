package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestRusticObserveRoute_ProxyCompatible(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	dbStore, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)
	defer func() { _ = dbStore.Close() }()

	dbPath := filepath.Join(t.TempDir(), "observe.db")
	now := time.Now()
	require.NoError(t, seedObserveSQLiteDB(dbPath, []observeFixtureSpan{
		{
			traceID:    "trace-old",
			spanID:     "span-old",
			name:       "old",
			start:      now.Add(-25 * time.Hour),
			end:        now.Add(-25*time.Hour + 2*time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:42", "root_thread_id": "id:42"},
		},
		{
			traceID:    "trace-newer",
			spanID:     "span-a",
			parentID:   "",
			name:       "send",
			start:      now.Add(-2 * time.Minute),
			end:        now.Add(-2*time.Minute + 3*time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:42", "root_thread_id": "id:100"},
			events: []observeFixtureEvent{
				{name: "queued", at: now.Add(-2*time.Minute + time.Millisecond)},
			},
		},
		{
			traceID:    "trace-newer",
			spanID:     "span-b",
			parentID:   "span-a",
			name:       "child-nonmatching",
			start:      now.Add(-2*time.Minute + 5*time.Millisecond),
			end:        now.Add(-2*time.Minute + 7*time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:999", "root_thread_id": "id:100"},
		},
		{
			traceID:    "trace-newer",
			spanID:     "span-c",
			parentID:   "span-a",
			name:       "root-match",
			start:      now.Add(-2*time.Minute + 8*time.Millisecond),
			end:        now.Add(-2*time.Minute + 12*time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:100", "root_thread_id": "id:100"},
		},
		{
			traceID:    "trace-other-guild",
			spanID:     "span-d",
			name:       "other",
			start:      now.Add(-1 * time.Minute),
			end:        now.Add(-1*time.Minute + 2*time.Millisecond),
			attributes: map[string]string{"guild_id": "g2", "message_id": "id:42", "root_thread_id": "id:100"},
		},
	}))

	s := NewServer(dbStore, nil, nil, nil, nil, ":0").WithObservability("desktop_sqlite", dbPath)
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/observe/guilds/g1/messages/42/spans?rootThreadId=100", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var spans []observeTraceSpan
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &spans))
	require.Len(t, spans, 3)
	require.Equal(t, "trace-newer", spans[0].TraceID)
	require.Equal(t, "span-a", spans[0].ID)
	require.Equal(t, int64(3000), spans[0].Duration)
	require.Equal(t, "id:42", spans[0].Tags["message_id"])
	require.Len(t, spans[0].Annotations, 1)
	require.Equal(t, "queued", spans[0].Annotations[0].Value)
	require.Equal(t, "span-b", spans[1].ID)
	require.Equal(t, "span-c", spans[2].ID)
}

func TestRusticObserveRoute_DefaultDurationMatchesProxy(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	dbStore, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)
	defer func() { _ = dbStore.Close() }()

	dbPath := filepath.Join(t.TempDir(), "observe-duration.db")
	now := time.Now()
	require.NoError(t, seedObserveSQLiteDB(dbPath, []observeFixtureSpan{
		{
			traceID:    "recent",
			spanID:     "recent-span",
			name:       "recent",
			start:      now.Add(-1 * time.Hour),
			end:        now.Add(-1*time.Hour + time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:42", "root_thread_id": "id:42"},
		},
		{
			traceID:    "old",
			spanID:     "old-span",
			name:       "old",
			start:      now.Add(-48 * time.Hour),
			end:        now.Add(-48*time.Hour + time.Millisecond),
			attributes: map[string]string{"guild_id": "g1", "message_id": "id:42", "root_thread_id": "id:42"},
		},
	}))

	s := NewServer(dbStore, nil, nil, nil, nil, ":0").WithObservability("desktop_sqlite", dbPath)
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/observe/guilds/g1/messages/42/spans", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var spans []observeTraceSpan
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &spans))
	require.Len(t, spans, 1)
	require.Equal(t, "recent", spans[0].TraceID)
}

func TestRusticObserveRoute_UnsupportedMode(t *testing.T) {
	t.Setenv("FORGE_ENABLE_PUBLIC_API", "false")
	t.Setenv("FORGE_ENABLE_UI_API", "true")
	t.Setenv("FORGE_IDENTITY_MODE", "local")
	t.Setenv("FORGE_QUOTA_MODE", "local")

	dbStore, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)
	defer func() { _ = dbStore.Close() }()

	s := NewServer(dbStore, nil, nil, nil, nil, ":0").WithObservability("external_otlp", "")
	router := s.buildRouter()

	req := httptest.NewRequest(http.MethodGet, "/rustic/observe/guilds/g1/messages/42/spans", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotImplemented, rr.Code)
}

type observeFixtureSpan struct {
	traceID    string
	spanID     string
	parentID   string
	name       string
	start      time.Time
	end        time.Time
	attributes map[string]string
	events     []observeFixtureEvent
}

type observeFixtureEvent struct {
	name string
	at   time.Time
}

func seedObserveSQLiteDB(path string, spans []observeFixtureSpan) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	for _, stmt := range []string{
		`CREATE TABLE resources (id INTEGER PRIMARY KEY AUTOINCREMENT, attributes TEXT NOT NULL DEFAULT '{}', schema_url TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE instrumentation_scopes (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', attributes TEXT NOT NULL DEFAULT '{}', schema_url TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE spans (
			trace_id TEXT NOT NULL,
			span_id TEXT NOT NULL,
			trace_state TEXT,
			parent_span_id TEXT,
			name TEXT,
			kind INTEGER,
			start_time_unix_nano INTEGER,
			end_time_unix_nano INTEGER,
			attributes TEXT,
			events TEXT,
			links TEXT,
			status_code INTEGER,
			status_message TEXT,
			resource_id INTEGER,
			scope_id INTEGER,
			PRIMARY KEY (trace_id, span_id)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	resourceAttrs, err := json.Marshal([]map[string]any{
		{"key": "service.name", "value": map[string]any{"stringValue": "forge-server"}},
	})
	if err != nil {
		return err
	}
	res, err := db.Exec(`INSERT INTO resources(attributes, schema_url) VALUES (?, '')`, string(resourceAttrs))
	if err != nil {
		return err
	}
	resourceID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	scopeRes, err := db.Exec(`INSERT INTO instrumentation_scopes(name, version, attributes, schema_url) VALUES ('', '', '[]', '')`)
	if err != nil {
		return err
	}
	scopeID, err := scopeRes.LastInsertId()
	if err != nil {
		return err
	}

	for _, span := range spans {
		attrJSON, err := observeFixtureAttrsJSON(span.attributes)
		if err != nil {
			return err
		}
		eventsJSON, err := observeFixtureEventsJSON(span.events)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`
			INSERT INTO spans (
				trace_id, span_id, trace_state, parent_span_id, name, kind,
				start_time_unix_nano, end_time_unix_nano, attributes, events, links,
				status_code, status_message, resource_id, scope_id
			) VALUES (?, ?, '', ?, ?, 0, ?, ?, ?, ?, '[]', 0, '', ?, ?)
		`,
			span.traceID,
			span.spanID,
			span.parentID,
			span.name,
			span.start.UnixNano(),
			span.end.UnixNano(),
			attrJSON,
			eventsJSON,
			resourceID,
			scopeID,
		); err != nil {
			return err
		}
	}
	return nil
}

func observeFixtureAttrsJSON(attrs map[string]string) (string, error) {
	items := make([]map[string]any, 0, len(attrs))
	for key, value := range attrs {
		items = append(items, map[string]any{
			"key":   key,
			"value": map[string]any{"stringValue": value},
		})
	}
	encoded, err := json.Marshal(items)
	return string(encoded), err
}

func observeFixtureEventsJSON(events []observeFixtureEvent) (string, error) {
	items := make([]map[string]any, 0, len(events))
	for _, event := range events {
		items = append(items, map[string]any{
			"timeUnixNano": strconv.FormatInt(event.at.UnixNano(), 10),
			"name":         event.name,
		})
	}
	encoded, err := json.Marshal(items)
	return string(encoded), err
}
