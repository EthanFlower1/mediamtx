# Smart Storage Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement event-aware retention policies that consolidate detections into compact event summaries and delete recordings intelligently based on whether they contain events, giving users full control over their storage.

**Architecture:** Detection consolidation compresses individual detection rows into a JSON summary on the motion_event row hourly, reducing detection storage by ~96%. Event-aware retention splits recording cleanup into two tiers: short retention for uneventful footage (`retention_days`) and long retention for recordings with events (`event_retention_days`). When only `retention_days` is set (existing behavior), all recordings are deleted uniformly for backward compatibility. A storage dashboard API gives users visibility into database and disk usage.

**Tech Stack:** Go, SQLite (WAL mode), gin-gonic HTTP framework, testify

---

## File Map

**Create:**
- `internal/nvr/db/retention.go` — consolidation, event-aware deletion, DB stats
- `internal/nvr/db/retention_test.go` — tests for all retention logic

**Modify:**
- `internal/nvr/db/migrations.go` — Migration 27: new columns
- `internal/nvr/db/cameras.go` — Camera struct + CRUD for new retention fields
- `internal/nvr/db/motion_events.go` — MotionEvent struct (add Embedding, DetectionSummary)
- `internal/nvr/db/db_test.go` — Update migration version assertion
- `internal/nvr/ai/search.go` — Search both detections and consolidated events
- `internal/nvr/scheduler/scheduler.go` — Extended retention cleanup logic
- `internal/nvr/api/cameras.go` — Extended retention endpoint
- `internal/nvr/api/system.go` — Database stats in storage endpoint
- `internal/nvr/api/router.go` — New manual purge route

---

### Task 1: Schema Migration + Camera Model Updates

**Files:**
- Modify: `internal/nvr/db/migrations.go` (append migration 27)
- Modify: `internal/nvr/db/cameras.go` (Camera struct + all CRUD)
- Modify: `internal/nvr/db/db_test.go:53` (version assertion)

- [ ] **Step 1: Update migration version test**

In `internal/nvr/db/db_test.go`, change the migration version assertion:

```go
require.Equal(t, 27, version)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestOpenRunsMigrations -v`
Expected: FAIL — version is 26, expected 27

- [ ] **Step 3: Add migration 27 to migrations.go**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
// Migration 27: Event-aware retention and detection consolidation.
{
    version: 27,
    sql: `
        ALTER TABLE cameras ADD COLUMN event_retention_days INTEGER NOT NULL DEFAULT 0;
        ALTER TABLE cameras ADD COLUMN detection_retention_days INTEGER NOT NULL DEFAULT 0;
        ALTER TABLE motion_events ADD COLUMN detection_summary TEXT DEFAULT '';
    `,
},
```

- [ ] **Step 4: Add new fields to Camera struct**

In `internal/nvr/db/cameras.go`, add to the `Camera` struct after `RetentionDays`:

```go
EventRetentionDays     int `json:"event_retention_days"`
DetectionRetentionDays int `json:"detection_retention_days"`
```

- [ ] **Step 5: Update CreateCamera to include new columns**

In `internal/nvr/db/cameras.go`, update `CreateCamera` INSERT statement to include the new columns. Add `event_retention_days, detection_retention_days` to the column list and `cam.EventRetentionDays, cam.DetectionRetentionDays` to the values.

The full column list becomes:
```go
_, err := d.Exec(`
    INSERT INTO cameras (id, name, onvif_endpoint, onvif_username, onvif_password,
        onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
        retention_days, event_retention_days, detection_retention_days,
        supports_ptz, supports_imaging, supports_events,
        supports_relay, supports_audio_backchannel, snapshot_uri,
        supports_media2, supports_analytics, supports_edge_recording,
        motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
        storage_path, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    cam.ID, cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
    cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
    cam.Status, cam.Tags, cam.RetentionDays,
    cam.EventRetentionDays, cam.DetectionRetentionDays,
    cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
    cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
    cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
    cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled, cam.AudioTranscode,
    cam.StoragePath, cam.CreatedAt, cam.UpdatedAt,
)
```

- [ ] **Step 6: Update GetCamera, GetCameraByPath, ListCameras SELECT + Scan**

All three methods share the same column pattern. Add `event_retention_days, detection_retention_days` to each SELECT immediately after `retention_days`, and add `&cam.EventRetentionDays, &cam.DetectionRetentionDays` to each Scan immediately after `&cam.RetentionDays`.

For `GetCamera`, the SELECT becomes:
```go
err := d.QueryRow(`
    SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
        onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
        retention_days, event_retention_days, detection_retention_days,
        supports_ptz, supports_imaging, supports_events,
        supports_relay, supports_audio_backchannel, snapshot_uri,
        supports_media2, supports_analytics, supports_edge_recording,
        motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
        storage_path, created_at, updated_at,
        ai_stream_id, ai_track_timeout, ai_confidence, recording_stream_id
    FROM cameras WHERE id = ?`, id,
).Scan(
    &cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
    &cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
    &cam.Status, &cam.Tags, &cam.RetentionDays,
    &cam.EventRetentionDays, &cam.DetectionRetentionDays,
    &cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
    &cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
    &cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
    &cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled, &cam.AudioTranscode,
    &cam.StoragePath, &cam.CreatedAt, &cam.UpdatedAt,
    &cam.AIStreamID, &cam.AITrackTimeout, &cam.AIConfidence, &cam.RecordingStreamID,
)
```

Apply the identical pattern to `GetCameraByPath` and `ListCameras`.

- [ ] **Step 7: Update UpdateCamera SET clause**

In `internal/nvr/db/cameras.go`, add to `UpdateCamera`'s SET clause:
```go
res, err := d.Exec(`
    UPDATE cameras SET name = ?, onvif_endpoint = ?, onvif_username = ?,
        onvif_password = ?, onvif_profile_token = ?, rtsp_url = ?, ptz_capable = ?,
        mediamtx_path = ?, status = ?, tags = ?, retention_days = ?,
        event_retention_days = ?, detection_retention_days = ?,
        supports_ptz = ?, supports_imaging = ?, supports_events = ?,
        supports_relay = ?, supports_audio_backchannel = ?, snapshot_uri = ?,
        supports_media2 = ?, supports_analytics = ?, supports_edge_recording = ?,
        motion_timeout_seconds = ?, sub_stream_url = ?, ai_enabled = ?,
        audio_transcode = ?, storage_path = ?, updated_at = ?
    WHERE id = ?`,
    cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
    cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
    cam.Status, cam.Tags, cam.RetentionDays,
    cam.EventRetentionDays, cam.DetectionRetentionDays,
    cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
    cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
    cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
    cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled, cam.AudioTranscode,
    cam.StoragePath, cam.UpdatedAt, cam.ID,
)
```

- [ ] **Step 8: Add UpdateCameraRetentionPolicy method**

Append to `internal/nvr/db/cameras.go`:

```go
// UpdateCameraRetentionPolicy updates all retention-related fields for a camera.
func (d *DB) UpdateCameraRetentionPolicy(id string, retentionDays, eventRetentionDays, detectionRetentionDays int) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE cameras SET retention_days = ?, event_retention_days = ?,
			detection_retention_days = ?, updated_at = ?
		WHERE id = ?`,
		retentionDays, eventRetentionDays, detectionRetentionDays, updatedAt, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 9: Run all db tests**

Run: `cd internal/nvr/db && go test ./... -v`
Expected: All PASS (including TestOpenRunsMigrations with version 27)

- [ ] **Step 10: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/cameras.go internal/nvr/db/db_test.go
git commit -m "feat: add migration 27 with event-aware retention columns and camera model updates"
```

---

### Task 2: Detection Consolidation

**Files:**
- Create: `internal/nvr/db/retention.go`
- Create: `internal/nvr/db/retention_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nvr/db/retention_test.go`:

```go
package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestConsolidateClosedEvents(t *testing.T) {
	d := openTestDB(t)

	// Create a camera.
	cam := &Camera{Name: "test-cam", RetentionDays: 7}
	require.NoError(t, d.CreateCamera(cam))

	// Insert a closed motion event (ended 2 hours ago).
	twoHoursAgo := time.Now().Add(-2 * time.Hour).UTC()
	event := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   twoHoursAgo.Add(-5 * time.Minute).Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(event))
	endedAt := twoHoursAgo.Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, endedAt))

	// Insert detections for this event, one with an embedding.
	det1 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-4 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.85,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det1))

	det2 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-3 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.92,
		BoxX: 0.15, BoxY: 0.25, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{1, 2, 3, 4}, // fake embedding
	}
	require.NoError(t, d.InsertDetection(det2))

	det3 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-2 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.88,
		BoxX: 0.2, BoxY: 0.3, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{5, 6, 7, 8}, // lower confidence embedding
	}
	require.NoError(t, d.InsertDetection(det3))

	// Run consolidation (older than 1 hour).
	count, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify: individual detections should be deleted.
	dets, err := d.ListDetectionsByEvent(event.ID)
	require.NoError(t, err)
	assert.Empty(t, dets, "individual detections should be deleted after consolidation")

	// Verify: motion event should have detection_summary and best embedding.
	var summary string
	var embedding []byte
	err = d.QueryRow(
		`SELECT COALESCE(detection_summary, ''), embedding FROM motion_events WHERE id = ?`,
		event.ID,
	).Scan(&summary, &embedding)
	require.NoError(t, err)

	assert.NotEmpty(t, summary)
	assert.Equal(t, []byte{1, 2, 3, 4}, embedding, "should keep embedding from highest confidence detection (det2=0.92)")

	var entries []DetectionSummaryEntry
	require.NoError(t, json.Unmarshal([]byte(summary), &entries))
	assert.Len(t, entries, 3, "summary should contain all 3 detections")
	assert.Equal(t, "person", entries[0].Class)
}

func TestConsolidateClosedEvents_SkipsRecentAndOpen(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert an OPEN event (no ended_at).
	event1 := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event1))
	det := &Detection{
		MotionEventID: event1.ID,
		FrameTime:     time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		Class:         "car",
		Confidence:    0.8,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det))

	// Insert a RECENT closed event (ended 5 minutes ago).
	event2 := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event2))
	require.NoError(t, d.EndMotionEvent(cam.ID, time.Now().Add(-5*time.Minute).UTC().Format(timeFormat)))
	det2 := &Detection{
		MotionEventID: event2.ID,
		FrameTime:     time.Now().Add(-8 * time.Minute).UTC().Format(timeFormat),
		Class:         "dog",
		Confidence:    0.7,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det2))

	// Consolidation with 1-hour threshold should skip both.
	count, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Both events should still have their detections.
	dets1, _ := d.ListDetectionsByEvent(event1.ID)
	assert.Len(t, dets1, 1)
	dets2, _ := d.ListDetectionsByEvent(event2.ID)
	assert.Len(t, dets2, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestConsolidate -v`
Expected: FAIL — `ConsolidateClosedEvents` and `DetectionSummaryEntry` not defined

- [ ] **Step 3: Implement retention.go**

Create `internal/nvr/db/retention.go`:

```go
package db

import (
	"encoding/json"
	"time"
)

// DetectionSummaryEntry is a compact representation of a detection for storage
// in the motion_event's detection_summary JSON field after consolidation.
type DetectionSummaryEntry struct {
	FrameTime  string  `json:"t"`
	Class      string  `json:"c"`
	Confidence float64 `json:"cf"`
	BoxX       float64 `json:"x"`
	BoxY       float64 `json:"y"`
	BoxW       float64 `json:"w"`
	BoxH       float64 `json:"h"`
}

// ConsolidateClosedEvents finds closed motion events older than the given
// threshold that still have individual detection rows. For each, it builds a
// compact JSON summary, keeps the best CLIP embedding, stores both on the
// motion_event row, and deletes the individual detection rows.
// Returns the number of events consolidated.
func (d *DB) ConsolidateClosedEvents(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT DISTINCT me.id
		FROM motion_events me
		INNER JOIN detections det ON det.motion_event_id = me.id
		WHERE me.ended_at IS NOT NULL
		  AND me.ended_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var eventIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		eventIDs = append(eventIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	consolidated := 0
	for _, eventID := range eventIDs {
		if err := d.consolidateEvent(eventID); err != nil {
			continue // log in caller if needed, don't fail entire batch
		}
		consolidated++
	}
	return consolidated, nil
}

// consolidateEvent reads all detections for an event, builds a JSON summary,
// selects the best embedding, updates the event, and deletes the detections.
func (d *DB) consolidateEvent(eventID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	detRows, err := tx.Query(`
		SELECT frame_time, class, confidence, box_x, box_y, box_w, box_h, embedding
		FROM detections WHERE motion_event_id = ?
		ORDER BY frame_time`, eventID)
	if err != nil {
		return err
	}

	var entries []DetectionSummaryEntry
	var bestEmbedding []byte
	var bestConfidence float64

	for detRows.Next() {
		var e DetectionSummaryEntry
		var embedding []byte
		if err := detRows.Scan(&e.FrameTime, &e.Class, &e.Confidence,
			&e.BoxX, &e.BoxY, &e.BoxW, &e.BoxH, &embedding); err != nil {
			detRows.Close()
			return err
		}
		entries = append(entries, e)
		if len(embedding) > 0 && e.Confidence > bestConfidence {
			bestEmbedding = embedding
			bestConfidence = e.Confidence
		}
	}
	detRows.Close()

	if len(entries) == 0 {
		return nil
	}

	summaryJSON, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE motion_events SET detection_summary = ?, embedding = ?
		WHERE id = ?`,
		string(summaryJSON), bestEmbedding, eventID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM detections WHERE motion_event_id = ?`, eventID); err != nil {
		return err
	}

	return tx.Commit()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/db && go test -run TestConsolidate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/retention.go internal/nvr/db/retention_test.go
git commit -m "feat: add detection consolidation into compact event summaries"
```

---

### Task 3: Event-Aware Recording Deletion + Motion Event Cleanup

**Files:**
- Modify: `internal/nvr/db/retention.go`
- Modify: `internal/nvr/db/retention_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/nvr/db/retention_test.go`:

```go
func TestDeleteRecordingsWithoutEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// Insert an old recording WITH NO event overlap.
	recNoEvent := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/no-event.mp4",
		FileSize:  1000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recNoEvent))

	// Insert an old recording WITH an overlapping event.
	recWithEvent := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Add(1 * time.Hour).Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(1*time.Hour + 10*time.Minute).Format(timeFormat),
		FilePath:  "/tmp/with-event.mp4",
		FileSize:  2000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recWithEvent))

	// Insert a motion event overlapping the second recording.
	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: fiveDaysAgo.Add(1*time.Hour + 2*time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	endStr := fiveDaysAgo.Add(1*time.Hour + 5*time.Minute).Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, endStr))

	// Delete recordings without events older than 3 days.
	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteRecordingsWithoutEvents(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/no-event.mp4"}, paths)

	// The event-linked recording should still exist.
	recs, err := d.QueryRecordings(cam.ID, fiveDaysAgo, now)
	require.NoError(t, err)
	assert.Len(t, recs, 1)
	assert.Equal(t, "/tmp/with-event.mp4", recs[0].FilePath)
}

func TestDeleteRecordingsWithEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	oneYearAgo := now.AddDate(-1, -1, 0) // 13 months ago

	// Insert an old recording WITH an overlapping event.
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: oneYearAgo.Format(timeFormat),
		EndTime:   oneYearAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/old-event.mp4",
		FileSize:  3000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: oneYearAgo.Add(2 * time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, oneYearAgo.Add(5*time.Minute).Format(timeFormat)))

	// Delete event-linked recordings older than 1 year.
	cutoff := now.AddDate(-1, 0, 0)
	paths, err := d.DeleteRecordingsWithEvents(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/old-event.mp4"}, paths)
}

func TestDeleteMotionEventsBefore(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	old := now.AddDate(0, -2, 0) // 2 months ago

	// Insert old closed event with thumbnail.
	event := &MotionEvent{
		CameraID:      cam.ID,
		StartedAt:     old.Format(timeFormat),
		EventType:     "ai_detection",
		ObjectClass:   "person",
		Confidence:    0.9,
		ThumbnailPath: "/tmp/thumb-old.jpg",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, old.Add(5*time.Minute).Format(timeFormat)))

	// Insert recent event.
	recent := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: now.Add(-1 * time.Hour).Format(timeFormat),
		EventType: "motion",
	}
	require.NoError(t, d.InsertMotionEvent(recent))
	require.NoError(t, d.EndMotionEvent(cam.ID, now.Add(-50*time.Minute).Format(timeFormat)))

	// Delete events older than 1 month.
	cutoff := now.AddDate(0, -1, 0)
	thumbs, deleted, err := d.DeleteMotionEventsBefore(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	assert.Equal(t, []string{"/tmp/thumb-old.jpg"}, thumbs)

	// Recent event should still exist.
	events, err := d.QueryMotionEvents(cam.ID, now.Add(-2*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/db && go test -run "TestDeleteRecordingsWithout|TestDeleteRecordingsWith[^o]|TestDeleteMotionEvents" -v`
Expected: FAIL — methods not defined

- [ ] **Step 3: Implement event-aware deletion methods**

Append to `internal/nvr/db/retention.go`:

```go
// DeleteRecordingsWithoutEvents deletes recordings for a camera that ended
// before the cutoff and have NO overlapping motion events. Returns the file
// paths of deleted recordings for disk cleanup.
func (d *DB) DeleteRecordingsWithoutEvents(cameraID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.end_time < ?
		AND NOT EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND end_time < ?
			AND NOT EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteRecordingsWithEvents deletes recordings for a camera that ended before
// the cutoff and DO have overlapping motion events. Returns the file paths of
// deleted recordings for disk cleanup.
func (d *DB) DeleteRecordingsWithEvents(cameraID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.end_time < ?
		AND EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND end_time < ?
			AND EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteMotionEventsBefore deletes closed motion events for a camera that
// ended before the cutoff. Returns thumbnail paths for disk cleanup and the
// number of deleted events. Associated detections are CASCADE-deleted.
func (d *DB) DeleteMotionEventsBefore(cameraID string, before time.Time) (thumbnailPaths []string, deleted int64, err error) {
	beforeStr := before.UTC().Format(timeFormat)

	thumbRows, err := d.Query(`
		SELECT thumbnail_path FROM motion_events
		WHERE camera_id = ? AND ended_at IS NOT NULL AND ended_at < ?
		AND thumbnail_path != ''`, cameraID, beforeStr)
	if err != nil {
		return nil, 0, err
	}
	defer thumbRows.Close()

	for thumbRows.Next() {
		var p string
		if err := thumbRows.Scan(&p); err != nil {
			return nil, 0, err
		}
		thumbnailPaths = append(thumbnailPaths, p)
	}
	if err := thumbRows.Err(); err != nil {
		return nil, 0, err
	}

	res, err := d.Exec(`
		DELETE FROM motion_events
		WHERE camera_id = ? AND ended_at IS NOT NULL AND ended_at < ?`,
		cameraID, beforeStr)
	if err != nil {
		return nil, 0, err
	}

	deleted, _ = res.RowsAffected()
	return thumbnailPaths, deleted, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/nvr/db && go test -run "TestDeleteRecordings|TestDeleteMotionEvents" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/retention.go internal/nvr/db/retention_test.go
git commit -m "feat: add event-aware recording deletion and motion event cleanup"
```

---

### Task 4: Database Stats + Search Adaptation

**Files:**
- Modify: `internal/nvr/db/retention.go`
- Modify: `internal/nvr/db/retention_test.go`
- Modify: `internal/nvr/db/motion_events.go`
- Modify: `internal/nvr/ai/search.go`

- [ ] **Step 1: Write failing test for GetDatabaseStats**

Append to `internal/nvr/db/retention_test.go`:

```go
func TestGetDatabaseStats(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "stats-cam"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert some data.
	rec := &Recording{
		CameraID: cam.ID, StartTime: time.Now().UTC().Format(timeFormat),
		EndTime: time.Now().UTC().Format(timeFormat), FilePath: "/tmp/r.mp4",
		FileSize: 100, Format: "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	event := &MotionEvent{CameraID: cam.ID, StartedAt: time.Now().UTC().Format(timeFormat), EventType: "motion"}
	require.NoError(t, d.InsertMotionEvent(event))

	stats, err := d.GetDatabaseStats()
	require.NoError(t, err)
	assert.Greater(t, stats.FileSizeBytes, int64(0))
	assert.Equal(t, int64(1), stats.Tables["recordings"].RowCount)
	assert.Equal(t, int64(1), stats.Tables["motion_events"].RowCount)
	assert.Equal(t, int64(0), stats.Tables["detections"].RowCount)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestGetDatabaseStats -v`
Expected: FAIL — `GetDatabaseStats`, `TableStats`, `DatabaseStats` not defined

- [ ] **Step 3: Implement GetDatabaseStats**

Append to `internal/nvr/db/retention.go`:

```go
// TableStats holds row count for a database table.
type TableStats struct {
	RowCount int64 `json:"row_count"`
}

// DatabaseStats holds size and per-table statistics for the SQLite database.
type DatabaseStats struct {
	FileSizeBytes int64                `json:"file_size_bytes"`
	Tables        map[string]TableStats `json:"tables"`
}

// GetDatabaseStats returns the database file size and row counts for key tables.
func (d *DB) GetDatabaseStats() (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: make(map[string]TableStats),
	}

	var pageCount, pageSize int64
	_ = d.QueryRow("PRAGMA page_count").Scan(&pageCount)
	_ = d.QueryRow("PRAGMA page_size").Scan(&pageSize)
	stats.FileSizeBytes = pageCount * pageSize

	tables := []string{
		"recordings", "recording_fragments", "motion_events",
		"detections", "audit_log", "pending_syncs", "screenshots",
	}
	for _, table := range tables {
		var count int64
		_ = d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		stats.Tables[table] = TableStats{RowCount: count}
	}

	return stats, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/nvr/db && go test -run TestGetDatabaseStats -v`
Expected: PASS

- [ ] **Step 5: Add Embedding and DetectionSummary to MotionEvent struct**

In `internal/nvr/db/motion_events.go`, update the `MotionEvent` struct:

```go
type MotionEvent struct {
	ID               int64   `json:"id"`
	CameraID         string  `json:"camera_id"`
	StartedAt        string  `json:"started_at"`
	EndedAt          *string `json:"ended_at"`
	ThumbnailPath    string  `json:"thumbnail_path,omitempty"`
	EventType        string  `json:"event_type"`
	ObjectClass      string  `json:"object_class"`
	Confidence       float64 `json:"confidence"`
	Embedding        []byte  `json:"-"`
	DetectionSummary string  `json:"detection_summary,omitempty"`
}
```

Note: Existing `QueryMotionEvents` and `QueryMotionEventsByClass` do NOT select these new fields, so they'll remain zero-valued. This is intentional — only queries that need them will fetch them.

- [ ] **Step 6: Add ListSearchableEvents method**

Append to `internal/nvr/db/motion_events.go`:

```go
// SearchableEvent represents a consolidated motion event with its embedding,
// used by the search system to find events by similarity.
type SearchableEvent struct {
	EventID       int64   `json:"event_id"`
	CameraID      string  `json:"camera_id"`
	CameraName    string  `json:"camera_name"`
	Class         string  `json:"class"`
	Confidence    float64 `json:"confidence"`
	StartedAt     string  `json:"started_at"`
	Embedding     []byte  `json:"-"`
	ThumbnailPath string  `json:"thumbnail_path,omitempty"`
}

// ListSearchableEvents returns consolidated motion events that have non-null
// embeddings within the given time range. These are events whose detections
// have been consolidated, making their embedding the search target.
func (d *DB) ListSearchableEvents(cameraID string, start, end time.Time) ([]*SearchableEvent, error) {
	query := `
		SELECT me.id, me.camera_id, COALESCE(c.name, ''),
			COALESCE(me.object_class, ''), COALESCE(me.confidence, 0),
			me.started_at, me.embedding, COALESCE(me.thumbnail_path, '')
		FROM motion_events me
		LEFT JOIN cameras c ON me.camera_id = c.id
		WHERE me.embedding IS NOT NULL
		  AND length(me.embedding) > 0
		  AND me.started_at >= ?
		  AND me.started_at <= ?`

	args := []interface{}{
		start.UTC().Format(timeFormat),
		end.UTC().Format(timeFormat),
	}

	if cameraID != "" {
		query += ` AND me.camera_id = ?`
		args = append(args, cameraID)
	}

	query += ` ORDER BY me.started_at DESC LIMIT 500`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*SearchableEvent
	for rows.Next() {
		ev := &SearchableEvent{}
		if err := rows.Scan(
			&ev.EventID, &ev.CameraID, &ev.CameraName,
			&ev.Class, &ev.Confidence,
			&ev.StartedAt, &ev.Embedding, &ev.ThumbnailPath,
		); err != nil {
			return nil, err
		}
		results = append(results, ev)
	}
	return results, rows.Err()
}
```

- [ ] **Step 7: Update ai.Search to merge both data sources**

In `internal/nvr/ai/search.go`, update the `Search` function to query both live detections and consolidated events:

```go
func Search(embedder *Embedder, database *db.DB, query string, cameraID string, start, end time.Time, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWords := strings.Fields(queryLower)

	var textEmb []float32
	if embedder != nil {
		var err error
		textEmb, err = embedder.EncodeText(query)
		if err != nil {
			textEmb = nil
		}
	}

	type scored struct {
		result SearchResult
		score  float64
	}
	var results []scored

	// Source 1: Live detections (not yet consolidated).
	dets, err := database.ListDetectionsWithEvents(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	for _, det := range dets {
		score := classMatchScore(det.Class, queryWords)
		if textEmb != nil && len(det.Embedding) > 0 {
			visualEmb := bytesToFloat32Slice(det.Embedding)
			if visualEmb != nil {
				compareEmb := visualEmb
				if len(compareEmb) != len(textEmb) {
					compareEmb = embedder.ProjectVisual(visualEmb)
				}
				if compareEmb != nil && len(compareEmb) == len(textEmb) {
					sim := CosineSimilarity(textEmb, compareEmb)
					score = 0.3*score + 0.7*sim
				}
			}
		}
		if score > 0 {
			results = append(results, scored{
				result: SearchResult{
					DetectionID:   det.ID,
					EventID:       det.MotionEventID,
					CameraID:      det.CameraID,
					CameraName:    det.CameraName,
					Class:         det.Class,
					Confidence:    det.Confidence,
					Similarity:    score,
					FrameTime:     det.FrameTime,
					ThumbnailPath: det.ThumbnailPath,
				},
				score: score,
			})
		}
	}

	// Source 2: Consolidated events (already compacted).
	events, err := database.ListSearchableEvents(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		score := classMatchScore(ev.Class, queryWords)
		if textEmb != nil && len(ev.Embedding) > 0 {
			visualEmb := bytesToFloat32Slice(ev.Embedding)
			if visualEmb != nil {
				compareEmb := visualEmb
				if len(compareEmb) != len(textEmb) {
					compareEmb = embedder.ProjectVisual(visualEmb)
				}
				if compareEmb != nil && len(compareEmb) == len(textEmb) {
					sim := CosineSimilarity(textEmb, compareEmb)
					score = 0.3*score + 0.7*sim
				}
			}
		}
		if score > 0 {
			results = append(results, scored{
				result: SearchResult{
					EventID:       ev.EventID,
					CameraID:      ev.CameraID,
					CameraName:    ev.CameraName,
					Class:         ev.Class,
					Confidence:    ev.Confidence,
					Similarity:    score,
					FrameTime:     ev.StartedAt,
					ThumbnailPath: ev.ThumbnailPath,
				},
				score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = r.result
	}
	return out, nil
}
```

- [ ] **Step 8: Run all db and search tests**

Run: `cd internal/nvr && go test ./db/... ./ai/... -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/nvr/db/retention.go internal/nvr/db/retention_test.go \
       internal/nvr/db/motion_events.go internal/nvr/ai/search.go
git commit -m "feat: add database stats, searchable events, and merge search sources"
```

---

### Task 5: Scheduler Retention Overhaul

**Files:**
- Modify: `internal/nvr/scheduler/scheduler.go`

- [ ] **Step 1: Add removeFiles helper**

Add near the bottom of `internal/nvr/scheduler/scheduler.go` (before `startEventPipelineLocked`):

```go
// removeFiles deletes files from disk and returns the count successfully removed.
func removeFiles(paths []string) int {
	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed
}
```

- [ ] **Step 2: Refactor runRetentionCleanup**

Replace the entire `runRetentionCleanup` method in `internal/nvr/scheduler/scheduler.go`:

```go
// runRetentionCleanup consolidates old detections, applies event-aware recording
// retention, cleans up old motion events, and prunes the audit log.
func (s *Scheduler) runRetentionCleanup(cameras []*db.Camera) {
	now := time.Now().UTC()

	// Step 1: Consolidate detections from closed events (older than 1 hour)
	// into compact JSON summaries on the motion_event row.
	consolidated, err := s.db.ConsolidateClosedEvents(1 * time.Hour)
	if err != nil {
		log.Printf("scheduler: detection consolidation failed: %v", err)
	} else if consolidated > 0 {
		log.Printf("scheduler: consolidated detections for %d events", consolidated)
	}

	// Step 2: Per-camera retention.
	for _, cam := range cameras {
		if cam.RetentionDays <= 0 {
			continue
		}

		noEventCutoff := now.AddDate(0, 0, -cam.RetentionDays)

		if cam.EventRetentionDays > 0 {
			// Smart mode: retention_days for no-event recordings,
			// event_retention_days for recordings with events.
			paths, err := s.db.DeleteRecordingsWithoutEvents(cam.ID, noEventCutoff)
			if err != nil {
				log.Printf("scheduler: no-event retention FAILED for camera %s (id=%s): %v", cam.Name, cam.ID, err)
			} else if len(paths) > 0 {
				removed := removeFiles(paths)
				log.Printf("scheduler: no-event retention for %s: deleted %d recordings (%d files removed)", cam.Name, len(paths), removed)
			}

			eventCutoff := now.AddDate(0, 0, -cam.EventRetentionDays)
			paths, err = s.db.DeleteRecordingsWithEvents(cam.ID, eventCutoff)
			if err != nil {
				log.Printf("scheduler: event retention FAILED for camera %s (id=%s): %v", cam.Name, cam.ID, err)
			} else if len(paths) > 0 {
				removed := removeFiles(paths)
				log.Printf("scheduler: event retention for %s: deleted %d recordings (%d files removed)", cam.Name, len(paths), removed)
			}
		} else {
			// Legacy mode: retention_days applies to ALL recordings.
			paths, err := s.db.DeleteRecordingsByDateRange(cam.ID, noEventCutoff)
			if err != nil {
				log.Printf("scheduler: retention cleanup FAILED for camera %s (id=%s): %v", cam.Name, cam.ID, err)
				continue
			}
			if len(paths) > 0 {
				removed := removeFiles(paths)
				log.Printf("scheduler: retention cleanup for camera %s: deleted %d recordings (%d files removed), cutoff %s",
					cam.Name, len(paths), removed, noEventCutoff.Format(time.RFC3339))
			}
		}

		// Step 3: Clean old motion events if detection retention is configured.
		if cam.DetectionRetentionDays > 0 {
			eventCutoff := now.AddDate(0, 0, -cam.DetectionRetentionDays)
			thumbs, n, err := s.db.DeleteMotionEventsBefore(cam.ID, eventCutoff)
			if err != nil {
				log.Printf("scheduler: event data cleanup FAILED for camera %s (id=%s): %v", cam.Name, cam.ID, err)
			} else if n > 0 {
				removeFiles(thumbs)
				log.Printf("scheduler: event data cleanup for %s: deleted %d events", cam.Name, n)
			}
		}
	}

	// Step 4: Clean audit log entries older than 90 days.
	auditCutoff := now.AddDate(0, 0, -90)
	_ = s.db.DeleteAuditEntriesBefore(auditCutoff)
}
```

- [ ] **Step 3: Run scheduler tests**

Run: `cd internal/nvr/scheduler && go test ./... -v`
Expected: PASS (existing tests should still pass since they don't exercise retention directly)

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat: scheduler uses event-aware retention with detection consolidation"
```

---

### Task 6: API Endpoints — Retention Policy, Storage Dashboard, Manual Purge

**Files:**
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/system.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Extend retentionRequest struct**

In `internal/nvr/api/cameras.go`, replace the `retentionRequest` struct:

```go
// retentionRequest is the JSON body for updating a camera's retention policy.
type retentionRequest struct {
	RetentionDays          int `json:"retention_days"`
	EventRetentionDays     int `json:"event_retention_days"`
	DetectionRetentionDays int `json:"detection_retention_days"`
}
```

- [ ] **Step 2: Update UpdateRetention handler**

In `internal/nvr/api/cameras.go`, replace the `UpdateRetention` method:

```go
// UpdateRetention updates the retention policy for a specific camera.
// Accepts retention_days (no-event recordings), event_retention_days
// (recordings with events), and detection_retention_days (motion events/detections).
func (h *CameraHandler) UpdateRetention(c *gin.Context) {
	id := c.Param("id")

	var req retentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.RetentionDays < 0 || req.EventRetentionDays < 0 || req.DetectionRetentionDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention days must be >= 0"})
		return
	}

	if err := h.DB.UpdateCameraRetentionPolicy(id, req.RetentionDays, req.EventRetentionDays, req.DetectionRetentionDays); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update retention policy", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera after retention update", err)
		return
	}

	c.JSON(http.StatusOK, cam)
}
```

- [ ] **Step 3: Add database stats to the Storage endpoint**

In `internal/nvr/api/system.go`, update the `Storage` method. Add after the `perCamera` loop and before the response JSON:

```go
// Database stats.
var dbStats *db.DatabaseStats
if h.ConfigDB != nil {
    dbStats, _ = h.ConfigDB.GetDatabaseStats()
}
```

And update the response to include it:

```go
c.JSON(http.StatusOK, gin.H{
    "total_bytes":      totalBytes,
    "free_bytes":       freeBytes,
    "used_bytes":       usedBytes,
    "recordings_bytes": recordingsBytes,
    "per_camera":       perCamera,
    "database":         dbStats,
    "warning":          usedPercent > 85,
    "critical":         usedPercent > 95,
})
```

- [ ] **Step 4: Add PurgeEvents handler to CameraHandler**

Append to `internal/nvr/api/cameras.go`:

```go
// PurgeEvents deletes closed motion events (and their consolidated data) for
// a camera that ended before the given timestamp.
//
//	DELETE /api/nvr/cameras/:id/events?before=RFC3339
func (h *CameraHandler) PurgeEvents(c *gin.Context) {
	id := c.Param("id")

	beforeStr := c.Query("before")
	if beforeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'before' is required (RFC3339)"})
		return
	}
	before, err := time.Parse(time.RFC3339, beforeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'before' time format, use RFC3339"})
		return
	}

	thumbs, deleted, err := h.DB.DeleteMotionEventsBefore(id, before)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to purge events", err)
		return
	}

	// Clean up thumbnail files.
	for _, p := range thumbs {
		os.Remove(p)
	}

	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"message": fmt.Sprintf("purged %d events before %s", deleted, before.Format(time.RFC3339)),
	})
}
```

Add the required imports at the top of the file if not already present: `"fmt"`, `"os"`.

- [ ] **Step 5: Register the purge route**

In `internal/nvr/api/router.go`, add after the motion-events route (line ~221):

```go
protected.DELETE("/cameras/:id/events", cameraHandler.PurgeEvents)
```

- [ ] **Step 6: Run API tests**

Run: `cd internal/nvr/api && go test ./... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/system.go internal/nvr/api/router.go
git commit -m "feat: extended retention API, storage dashboard with DB stats, manual event purge"
```

---

### Task 7: Integration Tests

**Files:**
- Modify: `internal/nvr/db/retention_test.go`

- [ ] **Step 1: Write end-to-end retention flow test**

Append to `internal/nvr/db/retention_test.go`:

```go
func TestFullRetentionFlow(t *testing.T) {
	d := openTestDB(t)

	// Create camera with smart retention: 3-day no-event, 365-day event, 365-day detection.
	cam := &Camera{
		Name:                   "front-door",
		RetentionDays:          3,
		EventRetentionDays:     365,
		DetectionRetentionDays: 365,
	}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// --- Setup: 3 recordings from 5 days ago ---

	// Recording 1: no event overlap (should be deleted by 3-day retention).
	recQuiet := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/quiet.mp4",
		FileSize:  1000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recQuiet))

	// Recording 2: has event overlap (should survive 3-day, deleted after 365-day).
	recEvent := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Add(1 * time.Hour).Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(1*time.Hour + 10*time.Minute).Format(timeFormat),
		FilePath:  "/tmp/event.mp4",
		FileSize:  2000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recEvent))

	// Recording 3: recent (should survive everything).
	recRecent := &Recording{
		CameraID:  cam.ID,
		StartTime: now.Add(-1 * time.Hour).Format(timeFormat),
		EndTime:   now.Add(-50 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/recent.mp4",
		FileSize:  3000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recRecent))

	// Motion event overlapping Recording 2.
	event := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   fiveDaysAgo.Add(1*time.Hour + 2*time.Minute).Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID,
		fiveDaysAgo.Add(1*time.Hour+5*time.Minute).Format(timeFormat)))

	// Detections for the event.
	det := &Detection{
		MotionEventID: event.ID,
		FrameTime:     fiveDaysAgo.Add(1*time.Hour + 3*time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.92,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{1, 2, 3, 4},
	}
	require.NoError(t, d.InsertDetection(det))

	// --- Step 1: Consolidate ---
	consolidated, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, consolidated)

	// Verify detections are gone, summary exists.
	dets, _ := d.ListDetectionsByEvent(event.ID)
	assert.Empty(t, dets)
	var summary string
	d.QueryRow("SELECT detection_summary FROM motion_events WHERE id = ?", event.ID).Scan(&summary)
	assert.Contains(t, summary, "person")

	// --- Step 2: Delete no-event recordings (3-day cutoff) ---
	noEventCutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteRecordingsWithoutEvents(cam.ID, noEventCutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/quiet.mp4"}, paths)

	// --- Step 3: Event recordings should survive ---
	eventCutoff := now.AddDate(0, 0, -365)
	paths, err = d.DeleteRecordingsWithEvents(cam.ID, eventCutoff)
	require.NoError(t, err)
	assert.Empty(t, paths, "event recording is only 5 days old, 365-day cutoff should not delete it")

	// Verify: event recording + recent recording remain.
	allRecs, err := d.QueryRecordings(cam.ID, fiveDaysAgo.Add(-1*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, allRecs, 2)
}

func TestBackwardCompatibility_LegacyRetention(t *testing.T) {
	d := openTestDB(t)

	// Camera with only retention_days set (event_retention_days=0 = legacy mode).
	cam := &Camera{Name: "legacy-cam", RetentionDays: 3}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// Recording with event overlap.
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/legacy.mp4",
		FileSize:  1000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: fiveDaysAgo.Add(2 * time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, fiveDaysAgo.Add(5*time.Minute).Format(timeFormat)))

	// Legacy mode: DeleteRecordingsByDateRange deletes ALL old recordings.
	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteRecordingsByDateRange(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/legacy.mp4"}, paths, "legacy mode deletes regardless of events")
}
```

- [ ] **Step 2: Run the full test suite**

Run: `cd internal/nvr/db && go test ./... -v -count=1`
Expected: All PASS

- [ ] **Step 3: Run API tests to verify nothing is broken**

Run: `cd internal/nvr/api && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/db/retention_test.go
git commit -m "test: add integration tests for event-aware retention and consolidation flow"
```

---

## Summary of Behavior

| Camera Setting | Effect |
|---|---|
| `retention_days=3` only | **Legacy mode**: all recordings deleted after 3 days (unchanged behavior) |
| `retention_days=3` + `event_retention_days=365` | **Smart mode**: no-event recordings deleted after 3 days, event recordings after 365 days |
| `detection_retention_days=365` | Motion events + consolidated summaries deleted after 365 days |
| All zero (default) | No automatic cleanup |

**Consolidation** runs every hour during the scheduler's retention cycle, compacting closed events older than 1 hour. This reduces per-event storage from ~129KB to ~5KB (~96% reduction).

**Search** merges results from both live detections (hot buffer) and consolidated events, so results are seamless regardless of whether an event has been consolidated yet.
