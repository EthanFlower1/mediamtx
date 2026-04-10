package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Run migrations inline.
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE camera_states (
			camera_id    TEXT    NOT NULL,
			recorder_id  TEXT    NOT NULL,
			state        TEXT    NOT NULL CHECK(state IN ('online','degraded','offline','error')),
			error_message TEXT   NOT NULL DEFAULT '',
			current_bitrate_kbps INTEGER NOT NULL DEFAULT 0,
			current_framerate    INTEGER NOT NULL DEFAULT 0,
			last_frame_at        DATETIME,
			config_version       INTEGER NOT NULL DEFAULT 0,
			observed_at          DATETIME NOT NULL,
			updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (camera_id, recorder_id)
		);
		CREATE TABLE segment_index (
			segment_id   TEXT    PRIMARY KEY,
			camera_id    TEXT    NOT NULL,
			recorder_id  TEXT    NOT NULL,
			start_time   DATETIME NOT NULL,
			end_time     DATETIME NOT NULL,
			bytes        INTEGER NOT NULL DEFAULT 0,
			codec        TEXT    NOT NULL DEFAULT '',
			has_audio    BOOLEAN NOT NULL DEFAULT 0,
			is_event_clip BOOLEAN NOT NULL DEFAULT 0,
			storage_tier TEXT    NOT NULL DEFAULT 'local',
			sequence     INTEGER NOT NULL DEFAULT 0,
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE ai_events (
			event_id     TEXT    PRIMARY KEY,
			camera_id    TEXT    NOT NULL,
			recorder_id  TEXT    NOT NULL,
			kind         TEXT    NOT NULL,
			kind_label   TEXT    NOT NULL DEFAULT '',
			observed_at  DATETIME NOT NULL,
			confidence   REAL    NOT NULL DEFAULT 0,
			bbox_x       REAL    NOT NULL DEFAULT 0,
			bbox_y       REAL    NOT NULL DEFAULT 0,
			bbox_width   REAL    NOT NULL DEFAULT 0,
			bbox_height  REAL    NOT NULL DEFAULT 0,
			track_id     TEXT    NOT NULL DEFAULT '',
			segment_id   TEXT    NOT NULL DEFAULT '',
			thumbnail_ref TEXT   NOT NULL DEFAULT '',
			attributes   TEXT    NOT NULL DEFAULT '{}',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

func testAuth(recorderID string) RecorderAuthenticator {
	return func(_ *http.Request) (string, bool) {
		if recorderID == "" {
			return "", false
		}
		return recorderID, true
	}
}

var testLog = slog.Default()

// --- Store tests ---

func TestStore_UpsertCameraStates(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rows := []CameraStateRow{
		{CameraID: "cam-1", RecorderID: "rec-1", State: "online", ObservedAt: now},
		{CameraID: "cam-2", RecorderID: "rec-1", State: "degraded", ErrorMessage: "low fps", ObservedAt: now},
	}
	require.NoError(t, store.UpsertCameraStates(ctx, rows))

	states, err := store.AllCameraStates(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 2)

	// Upsert same camera — should update, not duplicate.
	rows[0].State = "offline"
	require.NoError(t, store.UpsertCameraStates(ctx, rows[:1]))

	states, err = store.AllCameraStates(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 2)
	for _, s := range states {
		if s.CameraID == "cam-1" {
			assert.Equal(t, "offline", s.State)
		}
	}
}

func TestStore_UpsertCameraStates_Empty(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	require.NoError(t, store.UpsertCameraStates(context.Background(), nil))
}

func TestStore_InsertSegments(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rows := []SegmentIndexRow{
		{SegmentID: "seg-1", CameraID: "cam-1", RecorderID: "rec-1", StartTime: now, EndTime: now.Add(30 * time.Second), Bytes: 1024, Codec: "h264"},
		{SegmentID: "seg-2", CameraID: "cam-1", RecorderID: "rec-1", StartTime: now.Add(30 * time.Second), EndTime: now.Add(60 * time.Second), Bytes: 2048},
	}
	require.NoError(t, store.InsertSegments(ctx, rows))

	// Duplicate insert should be ignored.
	require.NoError(t, store.InsertSegments(ctx, rows[:1]))

	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM segment_index").Scan(&count))
	assert.Equal(t, 2, count)
}

func TestStore_InsertAIEvents(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rows := []AIEventRow{
		{EventID: "evt-1", CameraID: "cam-1", RecorderID: "rec-1", Kind: "person", ObservedAt: now, Confidence: 0.95},
		{EventID: "evt-2", CameraID: "cam-1", RecorderID: "rec-1", Kind: "vehicle", ObservedAt: now, Attributes: map[string]string{"plate": "ABC123"}},
	}
	require.NoError(t, store.InsertAIEvents(ctx, rows))

	// Duplicate should be ignored.
	require.NoError(t, store.InsertAIEvents(ctx, rows[:1]))

	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_events").Scan(&count))
	assert.Equal(t, 2, count)
}

func TestStore_CameraState_PerCamera(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rows := []CameraStateRow{
		{CameraID: "cam-1", RecorderID: "rec-1", State: "online", ObservedAt: now},
		{CameraID: "cam-2", RecorderID: "rec-1", State: "offline", ObservedAt: now},
	}
	require.NoError(t, store.UpsertCameraStates(ctx, rows))

	states, err := store.CameraState(ctx, "cam-1")
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "online", states[0].State)
}

// --- Handler: StreamCameraState ---

func TestStreamCameraState_Success(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := StreamCameraStateHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-1","updates":[{"camera_id":"cam-1","state":"online","observed_at":"2026-04-08T12:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/kaivue.v1.DirectoryIngest/StreamCameraState", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	states, err := store.AllCameraStates(context.Background())
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "cam-1", states[0].CameraID)
}

func TestStreamCameraState_NoAuth(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := StreamCameraStateHandler(store, testAuth(""), testLog)
	body := `{"updates":[{"camera_id":"cam-1","state":"online","observed_at":"2026-04-08T12:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStreamCameraState_RecorderMismatch(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := StreamCameraStateHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-999","updates":[{"camera_id":"cam-1","state":"online","observed_at":"2026-04-08T12:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestStreamCameraState_BadJSON(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := StreamCameraStateHandler(store, testAuth("rec-1"), testLog)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStreamCameraState_WrongMethod(t *testing.T) {
	h := StreamCameraStateHandler(NewStore(nil), testAuth("rec-1"), testLog)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- Handler: PublishSegmentIndex ---

func TestPublishSegmentIndex_Success(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := PublishSegmentIndexHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-1","entries":[{"segment_id":"seg-1","camera_id":"cam-1","start_time":"2026-04-08T12:00:00Z","end_time":"2026-04-08T12:00:30Z","bytes":1024,"codec":"h264"}]}`
	req := httptest.NewRequest(http.MethodPost, "/kaivue.v1.DirectoryIngest/PublishSegmentIndex", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM segment_index").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPublishSegmentIndex_NoAuth(t *testing.T) {
	h := PublishSegmentIndexHandler(NewStore(nil), testAuth(""), testLog)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPublishSegmentIndex_SkipsInvalidEntries(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := PublishSegmentIndexHandler(store, testAuth("rec-1"), testLog)
	// Missing segment_id and bad time should be skipped.
	body := `{"recorder_id":"rec-1","entries":[{"segment_id":"","camera_id":"cam-1","start_time":"2026-04-08T12:00:00Z","end_time":"2026-04-08T12:00:30Z"},{"segment_id":"seg-ok","camera_id":"cam-1","start_time":"2026-04-08T12:00:00Z","end_time":"2026-04-08T12:00:30Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM segment_index").Scan(&count))
	assert.Equal(t, 1, count)
}

// --- Handler: PublishAIEvents ---

func TestPublishAIEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := PublishAIEventsHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-1","events":[{"event_id":"evt-1","camera_id":"cam-1","kind":"person","observed_at":"2026-04-08T12:00:00Z","confidence":0.95,"bbox":{"x":0.1,"y":0.2,"width":0.3,"height":0.4}}]}`
	req := httptest.NewRequest(http.MethodPost, "/kaivue.v1.DirectoryIngest/PublishAIEvents", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_events").Scan(&count))
	assert.Equal(t, 1, count)

	// Verify attributes stored correctly.
	var kind, attrs string
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT kind, attributes FROM ai_events WHERE event_id='evt-1'").Scan(&kind, &attrs))
	assert.Equal(t, "person", kind)
}

func TestPublishAIEvents_WithAttributes(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := PublishAIEventsHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-1","events":[{"event_id":"evt-2","camera_id":"cam-1","kind":"vehicle","observed_at":"2026-04-08T12:00:00Z","bbox":{},"attributes":{"plate":"ABC123","color":"red"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var attrs string
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT attributes FROM ai_events WHERE event_id='evt-2'").Scan(&attrs))
	var m map[string]string
	require.NoError(t, json.Unmarshal([]byte(attrs), &m))
	assert.Equal(t, "ABC123", m["plate"])
	assert.Equal(t, "red", m["color"])
}

func TestPublishAIEvents_NoAuth(t *testing.T) {
	h := PublishAIEventsHandler(NewStore(nil), testAuth(""), testLog)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPublishAIEvents_SkipsInvalid(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := PublishAIEventsHandler(store, testAuth("rec-1"), testLog)
	// Missing event_id + missing kind should be skipped; valid one kept.
	body := `{"recorder_id":"rec-1","events":[{"event_id":"","camera_id":"cam-1","kind":"person","observed_at":"2026-04-08T12:00:00Z","bbox":{}},{"event_id":"evt-ok","camera_id":"cam-1","kind":"person","observed_at":"2026-04-08T12:00:00Z","bbox":{}}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ai_events").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestStreamCameraState_WithLastFrameAt(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	h := StreamCameraStateHandler(store, testAuth("rec-1"), testLog)
	body := `{"recorder_id":"rec-1","updates":[{"camera_id":"cam-1","state":"online","observed_at":"2026-04-08T12:00:00Z","last_frame_at":"2026-04-08T11:59:59Z","current_bitrate_kbps":4000,"current_framerate":30}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	states, err := store.CameraState(context.Background(), "cam-1")
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.NotNil(t, states[0].LastFrameAt)
	assert.Equal(t, int32(4000), states[0].CurrentBitrateKbps)
	assert.Equal(t, int32(30), states[0].CurrentFramerate)
}
