# Camera Detail Redesign — Backend Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-stream retention, recording-to-stream association, and a storage estimate API endpoint so the Flutter UI can show per-stream retention controls with live storage estimates.

**Architecture:** Migration 28 adds `stream_id` to recordings and `retention_days`/`event_retention_days` to camera_streams. The recording writer tags each segment with the stream it came from. Retention cleanup iterates streams instead of cameras. A new storage estimate endpoint computes per-stream projected bytes from observed bitrates and event frequency.

**Tech Stack:** Go, SQLite (WAL mode), gin-gonic HTTP framework, testify

---

## File Map

**Create:**

- `internal/nvr/db/storage_estimate.go` — storage estimate query logic

**Modify:**

- `internal/nvr/db/migrations.go` — Migration 28
- `internal/nvr/db/recordings.go` — Recording struct + InsertRecording (add stream_id)
- `internal/nvr/db/camera_streams.go` — CameraStream struct + retention CRUD
- `internal/nvr/db/retention.go` — Per-stream deletion methods
- `internal/nvr/db/retention_test.go` — Tests for new methods
- `internal/nvr/db/db_test.go` — Migration version assertion
- `internal/nvr/nvr.go` — OnSegmentComplete: resolve and store stream_id
- `internal/nvr/scheduler/scheduler.go` — Per-stream retention cleanup
- `internal/nvr/api/streams.go` — Stream retention endpoint
- `internal/nvr/api/cameras.go` — Storage estimate endpoint
- `internal/nvr/api/router.go` — New routes

---

### Task 1: Migration 28 + Model Updates

**Files:**

- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/recordings.go`
- Modify: `internal/nvr/db/camera_streams.go`
- Modify: `internal/nvr/db/db_test.go`

- [ ] **Step 1: Update migration version test**

In `internal/nvr/db/db_test.go`, change the migration version assertion from 27 to 28:

```go
require.Equal(t, 28, version)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestOpenRunsMigrations -v`
Expected: FAIL — version is 27, expected 28

- [ ] **Step 3: Add migration 28**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
// Migration 28: Per-stream retention and recording-to-stream association.
{
    version: 28,
    sql: `
        ALTER TABLE recordings ADD COLUMN stream_id TEXT DEFAULT '';
        CREATE INDEX IF NOT EXISTS idx_recordings_stream ON recordings(stream_id);
        ALTER TABLE camera_streams ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;
        ALTER TABLE camera_streams ADD COLUMN event_retention_days INTEGER NOT NULL DEFAULT 0;
    `,
},
```

- [ ] **Step 4: Add StreamID to Recording struct**

In `internal/nvr/db/recordings.go`, add `StreamID` field to the `Recording` struct after `CameraID`:

```go
type Recording struct {
	ID         int64  `json:"id"`
	CameraID   string `json:"camera_id"`
	StreamID   string `json:"stream_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	DurationMs int64  `json:"duration_ms"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Format     string `json:"format"`
	InitSize   int64  `json:"init_size"`
}
```

- [ ] **Step 5: Update InsertRecording to include stream_id**

In `internal/nvr/db/recordings.go`, update the `InsertRecording` method:

```go
func (d *DB) InsertRecording(rec *Recording) error {
	if rec.Format == "" {
		rec.Format = "fmp4"
	}

	res, err := d.Exec(`
		INSERT INTO recordings (camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.CameraID, rec.StreamID, rec.StartTime, rec.EndTime, rec.DurationMs,
		rec.FilePath, rec.FileSize, rec.Format,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}
```

- [ ] **Step 6: Update all Recording SELECT/Scan queries**

Every query in `recordings.go` that selects from recordings needs to include `stream_id`. Find all `SELECT` queries and add `stream_id` after `camera_id` in both the column list and the `Scan` call. The affected methods are:

- `QueryRecordings` — add `stream_id` to SELECT and `&rec.StreamID` to Scan
- `QueryRecordingsByTimeRange` — same pattern
- `GetRecording` — same pattern
- `GetStoragePerCamera` — no change needed (aggregate query)
- `DeleteRecordingsByDateRange` — no change needed (only selects file_path)

For `QueryRecordings`, the SELECT becomes:

```go
rows, err := d.Query(`
    SELECT id, camera_id, stream_id, start_time, end_time, duration_ms,
        file_path, file_size, format, init_size
    FROM recordings
    WHERE camera_id = ? AND start_time < ? AND end_time > ?
    ORDER BY start_time`,
    cameraID, end.UTC().Format(timeFormat), start.UTC().Format(timeFormat),
)
```

And the Scan:

```go
if err := rows.Scan(
    &rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
    &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
); err != nil {
```

Apply the identical pattern to `QueryRecordingsByTimeRange` and `GetRecording`.

- [ ] **Step 7: Add retention fields to CameraStream struct**

In `internal/nvr/db/camera_streams.go`, add to the `CameraStream` struct after `CreatedAt`:

```go
RetentionDays      int `json:"retention_days"`
EventRetentionDays int `json:"event_retention_days"`
```

- [ ] **Step 8: Update all CameraStream SELECT/Scan queries**

Add `retention_days, event_retention_days` to every SELECT and Scan in `camera_streams.go`. The affected methods are `ListCameraStreams`, `GetCameraStream`, `ResolveStream` (the main query, not the fallback).

For `ListCameraStreams`, the SELECT becomes:

```go
rows, err := d.Query(`
    SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
        audio_codec, width, height, roles, created_at,
        retention_days, event_retention_days
    FROM camera_streams
    WHERE camera_id = ?
    ORDER BY (width * height) DESC, created_at ASC`, cameraID)
```

And the Scan:

```go
if err := rows.Scan(
    &s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
    &s.VideoCodec, &s.AudioCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
    &s.RetentionDays, &s.EventRetentionDays,
); err != nil {
```

Apply the identical pattern to `GetCameraStream` and the main query in `ResolveStream`.

For the `ResolveStream` fallback that builds a synthetic CameraStream (line 255), add the new fields with zero values (they're already defaulted).

- [ ] **Step 9: Add UpdateStreamRetention method**

Append to `internal/nvr/db/camera_streams.go`:

```go
// UpdateStreamRetention updates the retention policy for a stream.
func (d *DB) UpdateStreamRetention(id string, retentionDays, eventRetentionDays int) error {
	res, err := d.Exec(`
		UPDATE camera_streams SET retention_days = ?, event_retention_days = ?
		WHERE id = ?`,
		retentionDays, eventRetentionDays, id,
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

- [ ] **Step 10: Run all db tests**

Run: `cd internal/nvr/db && go test ./... -v`
Expected: All PASS

- [ ] **Step 11: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/recordings.go \
       internal/nvr/db/camera_streams.go internal/nvr/db/db_test.go
git commit -m "feat: migration 28 — per-stream retention columns and recording stream_id"
```

---

### Task 2: Recording Stream ID Association

**Files:**

- Modify: `internal/nvr/nvr.go`
- Modify: `internal/nvr/db/camera_streams.go`

- [ ] **Step 1: Add ResolveStreamByPathPrefix method**

Append to `internal/nvr/db/camera_streams.go`:

```go
// ResolveStreamByPathPrefix finds a stream ID by matching the first 8
// characters of the stream ID against the given prefix. Used to map
// recording file paths back to their source stream.
func (d *DB) ResolveStreamByPathPrefix(cameraID, prefix string) (string, error) {
	var streamID string
	err := d.QueryRow(`
		SELECT id FROM camera_streams
		WHERE camera_id = ? AND id LIKE ? || '%'
		LIMIT 1`, cameraID, prefix,
	).Scan(&streamID)
	if err != nil {
		return "", err
	}
	return streamID, nil
}
```

- [ ] **Step 2: Update OnSegmentComplete to capture stream_id**

In `internal/nvr/nvr.go`, in the `OnSegmentComplete` method, update the path parsing section (around lines 809-821). Replace:

```go
if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
    rest := filePath[idx+4:] // after "nvr/"
    parts := strings.SplitN(rest, "/", 2)
    if len(parts) >= 1 {
        candidate := parts[0]
        // Strip ~streamID suffix if present (per-stream recording paths).
        if tildeIdx := strings.Index(candidate, "~"); tildeIdx > 0 {
            candidate = candidate[:tildeIdx]
        }
        if c, err := n.database.GetCamera(candidate); err == nil {
            cam = c
        }
    }
}
```

With:

```go
var streamPrefix string
if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
    rest := filePath[idx+4:] // after "nvr/"
    parts := strings.SplitN(rest, "/", 2)
    if len(parts) >= 1 {
        candidate := parts[0]
        // Capture ~streamID prefix if present (per-stream recording paths).
        if tildeIdx := strings.Index(candidate, "~"); tildeIdx > 0 {
            streamPrefix = candidate[tildeIdx+1:]
            candidate = candidate[:tildeIdx]
        }
        if c, err := n.database.GetCamera(candidate); err == nil {
            cam = c
        }
    }
}
```

Then, after the recording struct is built (after line 865), add stream resolution before the insert:

```go
rec := &db.Recording{
    CameraID:   cam.ID,
    StartTime:  start.Format("2006-01-02T15:04:05.000Z"),
    EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
    DurationMs: duration.Milliseconds(),
    FilePath:   filePath,
    FileSize:   fileSize,
    Format:     format,
}

// Resolve stream ID from path prefix.
if streamPrefix != "" {
    if sid, err := n.database.ResolveStreamByPathPrefix(cam.ID, streamPrefix); err == nil {
        rec.StreamID = sid
    }
}
```

- [ ] **Step 3: Verify build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Clean build

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go internal/nvr/db/camera_streams.go
git commit -m "feat: associate recordings with their source stream via path prefix"
```

---

### Task 3: Per-Stream Retention Deletion Methods

**Files:**

- Modify: `internal/nvr/db/retention.go`
- Modify: `internal/nvr/db/retention_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/nvr/db/retention_test.go`:

```go
func TestDeleteStreamRecordingsWithoutEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	stream := &CameraStream{CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCameraStream(stream))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// Recording on this stream with no event.
	rec := &Recording{
		CameraID: cam.ID, StreamID: stream.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/stream-no-event.mp4", FileSize: 1000, Format: "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	// Recording on a different stream (should NOT be deleted).
	otherStream := &CameraStream{CameraID: cam.ID, Name: "Sub", RTSPURL: "rtsp://y"}
	require.NoError(t, d.CreateCameraStream(otherStream))
	rec2 := &Recording{
		CameraID: cam.ID, StreamID: otherStream.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/other-stream.mp4", FileSize: 1000, Format: "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec2))

	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteStreamRecordingsWithoutEvents(cam.ID, stream.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/stream-no-event.mp4"}, paths)

	// Other stream's recording should still exist.
	recs, err := d.QueryRecordings(cam.ID, fiveDaysAgo.Add(-1*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, recs, 1)
	assert.Equal(t, "/tmp/other-stream.mp4", recs[0].FilePath)
}

func TestDeleteStreamRecordingsWithEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	stream := &CameraStream{CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCameraStream(stream))

	now := time.Now().UTC()
	longAgo := now.AddDate(-2, 0, 0)

	rec := &Recording{
		CameraID: cam.ID, StreamID: stream.ID,
		StartTime: longAgo.Format(timeFormat),
		EndTime:   longAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/stream-event.mp4", FileSize: 2000, Format: "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	event := &MotionEvent{
		CameraID: cam.ID,
		StartedAt: longAgo.Add(2 * time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, longAgo.Add(5*time.Minute).Format(timeFormat)))

	cutoff := now.AddDate(-1, 0, 0)
	paths, err := d.DeleteStreamRecordingsWithEvents(cam.ID, stream.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/stream-event.mp4"}, paths)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/db && go test -run "TestDeleteStreamRecordings" -v`
Expected: FAIL — methods not defined

- [ ] **Step 3: Implement per-stream deletion methods**

Append to `internal/nvr/db/retention.go`:

```go
// DeleteStreamRecordingsWithoutEvents deletes recordings for a specific stream
// that ended before the cutoff and have NO overlapping motion events.
func (d *DB) DeleteStreamRecordingsWithoutEvents(cameraID, streamID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.stream_id = ? AND r.end_time < ?
		AND NOT EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, streamID, beforeStr)
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
			WHERE camera_id = ? AND stream_id = ? AND end_time < ?
			AND NOT EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, streamID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteStreamRecordingsWithEvents deletes recordings for a specific stream
// that ended before the cutoff and DO have overlapping motion events.
func (d *DB) DeleteStreamRecordingsWithEvents(cameraID, streamID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.stream_id = ? AND r.end_time < ?
		AND EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, streamID, beforeStr)
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
			WHERE camera_id = ? AND stream_id = ? AND end_time < ?
			AND EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, streamID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd internal/nvr/db && go test -run "TestDeleteStreamRecordings" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/retention.go internal/nvr/db/retention_test.go
git commit -m "feat: add per-stream recording deletion methods"
```

---

### Task 4: Storage Estimate Endpoint

**Files:**

- Create: `internal/nvr/db/storage_estimate.go`
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create storage estimate DB methods**

Create `internal/nvr/db/storage_estimate.go`:

```go
package db

import "time"

// StreamBitrateInfo holds observed bitrate data for a stream.
type StreamBitrateInfo struct {
	StreamID   string  `json:"stream_id"`
	StreamName string  `json:"stream_name"`
	BitrateBps float64 `json:"bitrate_bps"`
	Source     string  `json:"bitrate_source"` // "observed" or "estimated"
}

// GetStreamBitrates returns the average bitrate for each stream of a camera,
// calculated from recordings in the last 7 days. Returns estimated bitrates
// based on codec/resolution for streams with no recent data.
func (d *DB) GetStreamBitrates(cameraID string) ([]StreamBitrateInfo, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(timeFormat)

	// Get observed bitrates from recent recordings.
	observed := make(map[string]StreamBitrateInfo)
	rows, err := d.Query(`
		SELECT r.stream_id, COALESCE(cs.name, ''), SUM(r.file_size), SUM(r.duration_ms)
		FROM recordings r
		LEFT JOIN camera_streams cs ON r.stream_id = cs.id
		WHERE r.camera_id = ? AND r.start_time > ? AND r.duration_ms > 0
		GROUP BY r.stream_id`, cameraID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var streamID, name string
		var totalBytes, totalMs int64
		if err := rows.Scan(&streamID, &name, &totalBytes, &totalMs); err != nil {
			return nil, err
		}
		if totalMs > 0 {
			bitrate := float64(totalBytes) * 8 * 1000 / float64(totalMs) // bits per second
			observed[streamID] = StreamBitrateInfo{
				StreamID:   streamID,
				StreamName: name,
				BitrateBps: bitrate,
				Source:     "observed",
			}
		}
	}

	// Get all streams for this camera; fill in estimates for those without data.
	streams, err := d.ListCameraStreams(cameraID)
	if err != nil {
		return nil, err
	}

	var results []StreamBitrateInfo
	for _, s := range streams {
		if info, ok := observed[s.ID]; ok {
			results = append(results, info)
		} else {
			results = append(results, StreamBitrateInfo{
				StreamID:   s.ID,
				StreamName: s.Name,
				BitrateBps: estimateBitrate(s),
				Source:     "estimated",
			})
		}
	}
	return results, nil
}

// estimateBitrate returns a rough bitrate estimate based on resolution and codec.
func estimateBitrate(s *CameraStream) float64 {
	pixels := s.Width * s.Height
	if pixels <= 0 {
		pixels = 1920 * 1080 // default to 1080p
	}

	// Base rate: ~4 Mbps for 1080p H.264
	baseBps := float64(pixels) / float64(1920*1080) * 4_000_000

	codec := s.VideoCodec
	if codec == "H265" || codec == "h265" || codec == "HEVC" || codec == "hevc" {
		baseBps *= 0.6 // H.265 is ~40% more efficient
	}
	return baseBps
}

// GetEventFrequency returns the average number of events per day and average
// event duration in seconds for a camera over the last 7 days.
func (d *DB) GetEventFrequency(cameraID string) (eventsPerDay float64, avgDurationSec float64, source string, err error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(timeFormat)

	var count int64
	var totalDurationSec float64
	err = d.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM((julianday(ended_at) - julianday(started_at)) * 86400), 0)
		FROM motion_events
		WHERE camera_id = ? AND started_at > ? AND ended_at IS NOT NULL`,
		cameraID, cutoff,
	).Scan(&count, &totalDurationSec)
	if err != nil {
		return 0, 0, "", err
	}

	if count == 0 {
		// No history — assume 1 hour of events per day.
		return 1.0, 3600, "default", nil
	}

	days := 7.0
	eventsPerDay = float64(count) / days
	avgDurationSec = totalDurationSec / float64(count)
	return eventsPerDay, avgDurationSec, "historical", nil
}
```

- [ ] **Step 2: Add StorageEstimate API handler**

Append to `internal/nvr/api/cameras.go`:

```go
// StorageEstimate returns per-stream storage projections based on retention settings.
//
//	GET /api/nvr/cameras/:id/storage-estimate?retention_days=3&event_retention_days=365
func (h *CameraHandler) StorageEstimate(c *gin.Context) {
	id := c.Param("id")

	retDays, _ := strconv.Atoi(c.DefaultQuery("retention_days", "0"))
	eventRetDays, _ := strconv.Atoi(c.DefaultQuery("event_retention_days", "0"))

	bitrates, err := h.DB.GetStreamBitrates(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get stream bitrates", err)
		return
	}

	eventsPerDay, avgEventDur, freqSource, err := h.DB.GetEventFrequency(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get event frequency", err)
		return
	}

	type streamEstimate struct {
		StreamID       string  `json:"stream_id"`
		StreamName     string  `json:"stream_name"`
		BitrateBps     float64 `json:"bitrate_bps"`
		BitrateSource  string  `json:"bitrate_source"`
		NoEventBytes   int64   `json:"no_event_bytes"`
		EventBytes     int64   `json:"event_bytes"`
		EventFrequency float64 `json:"event_frequency"`
		FreqSource     string  `json:"event_frequency_source"`
		TotalBytes     int64   `json:"total_bytes"`
	}

	var streams []streamEstimate
	var total int64

	for _, br := range bitrates {
		bytesPerSec := br.BitrateBps / 8

		// No-event estimate: continuous recording minus event time.
		noEventSec := float64(retDays) * 86400
		eventSecPerDay := eventsPerDay * avgEventDur
		if eventSecPerDay > 86400 {
			eventSecPerDay = 86400
		}
		noEventSec -= float64(retDays) * eventSecPerDay
		if noEventSec < 0 {
			noEventSec = 0
		}
		noEventBytes := int64(noEventSec * bytesPerSec)

		// Event estimate: event duration × retention days.
		eventSec := float64(eventRetDays) * eventSecPerDay
		eventBytes := int64(eventSec * bytesPerSec)

		totalBytes := noEventBytes + eventBytes
		total += totalBytes

		streams = append(streams, streamEstimate{
			StreamID:       br.StreamID,
			StreamName:     br.StreamName,
			BitrateBps:     br.BitrateBps,
			BitrateSource:  br.Source,
			NoEventBytes:   noEventBytes,
			EventBytes:     eventBytes,
			EventFrequency: eventsPerDay,
			FreqSource:     freqSource,
			TotalBytes:     totalBytes,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"streams":     streams,
		"total_bytes": total,
	})
}
```

Make sure `"strconv"` is imported at the top of `cameras.go`.

- [ ] **Step 3: Register routes**

In `internal/nvr/api/router.go`, add after the existing retention route:

```go
protected.GET("/cameras/:id/storage-estimate", cameraHandler.StorageEstimate)
protected.PUT("/streams/:id/retention", streamHandler.UpdateRetention)
```

- [ ] **Step 4: Add stream retention handler**

Append to `internal/nvr/api/streams.go`:

```go
// streamRetentionRequest is the JSON body for updating a stream's retention policy.
type streamRetentionRequest struct {
	RetentionDays      int `json:"retention_days"`
	EventRetentionDays int `json:"event_retention_days"`
}

// UpdateRetention updates retention settings for a specific stream.
func (h *StreamHandler) UpdateRetention(c *gin.Context) {
	id := c.Param("id")

	var req streamRetentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.RetentionDays < 0 || req.EventRetentionDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention days must be >= 0"})
		return
	}

	if err := h.DB.UpdateStreamRetention(id, req.RetentionDays, req.EventRetentionDays); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update stream retention", err)
		return
	}

	stream, err := h.DB.GetCameraStream(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve stream", err)
		return
	}

	c.JSON(http.StatusOK, stream)
}
```

Add `"errors"` to the imports in `streams.go` if not already present.

- [ ] **Step 5: Verify build and run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Run: `cd internal/nvr/api && go test ./... -v`
Expected: Clean build, all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/db/storage_estimate.go internal/nvr/api/cameras.go \
       internal/nvr/api/streams.go internal/nvr/api/router.go
git commit -m "feat: add storage estimate endpoint and per-stream retention API"
```

---

### Task 5: Scheduler Per-Stream Retention

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go`

- [ ] **Step 1: Update runRetentionCleanup for per-stream retention**

Replace the `runRetentionCleanup` method in `internal/nvr/scheduler/scheduler.go`. The key change: for each camera, iterate its streams. If a stream has retention configured, use stream-level deletion. Otherwise fall back to camera-level.

```go
func (s *Scheduler) runRetentionCleanup(cameras []*db.Camera) {
	now := time.Now().UTC()

	// Step 1: Consolidate detections from closed events.
	consolidated, err := s.db.ConsolidateClosedEvents(1 * time.Hour)
	if err != nil {
		log.Printf("scheduler: detection consolidation failed: %v", err)
	} else if consolidated > 0 {
		log.Printf("scheduler: consolidated detections for %d events", consolidated)
	}

	// Step 2: Per-camera/stream retention.
	for _, cam := range cameras {
		streams, err := s.db.ListCameraStreams(cam.ID)
		if err != nil {
			log.Printf("scheduler: failed to list streams for camera %s: %v", cam.Name, err)
			continue
		}

		handledByStream := false
		for _, stream := range streams {
			if stream.RetentionDays <= 0 {
				continue
			}
			handledByStream = true
			noEventCutoff := now.AddDate(0, 0, -stream.RetentionDays)

			if stream.EventRetentionDays > 0 {
				paths, err := s.db.DeleteStreamRecordingsWithoutEvents(cam.ID, stream.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: stream no-event retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream no-event retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}

				eventCutoff := now.AddDate(0, 0, -stream.EventRetentionDays)
				paths, err = s.db.DeleteStreamRecordingsWithEvents(cam.ID, stream.ID, eventCutoff)
				if err != nil {
					log.Printf("scheduler: stream event retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream event retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}
			} else {
				paths, err := s.db.DeleteRecordingsByDateRange(cam.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: stream retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}
			}
		}

		// Camera-level fallback for recordings not covered by stream retention.
		if !handledByStream && cam.RetentionDays > 0 {
			noEventCutoff := now.AddDate(0, 0, -cam.RetentionDays)

			if cam.EventRetentionDays > 0 {
				paths, err := s.db.DeleteRecordingsWithoutEvents(cam.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: no-event retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: no-event retention for %s: deleted %d recordings (%d files)", cam.Name, len(paths), removed)
				}

				eventCutoff := now.AddDate(0, 0, -cam.EventRetentionDays)
				paths, err = s.db.DeleteRecordingsWithEvents(cam.ID, eventCutoff)
				if err != nil {
					log.Printf("scheduler: event retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: event retention for %s: deleted %d recordings (%d files)", cam.Name, len(paths), removed)
				}
			} else {
				paths, err := s.db.DeleteRecordingsByDateRange(cam.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: retention for %s: deleted %d recordings (%d files), cutoff %s", cam.Name, len(paths), removed, noEventCutoff.Format(time.RFC3339))
				}
			}
		}

		// Step 3: Clean old motion events.
		if cam.DetectionRetentionDays > 0 {
			eventCutoff := now.AddDate(0, 0, -cam.DetectionRetentionDays)
			thumbs, n, err := s.db.DeleteMotionEventsBefore(cam.ID, eventCutoff)
			if err != nil {
				log.Printf("scheduler: event data cleanup FAILED for camera %s: %v", cam.Name, err)
			} else if n > 0 {
				removeFiles(thumbs)
				log.Printf("scheduler: event data cleanup for %s: deleted %d events", cam.Name, n)
			}
		}
	}

	// Step 4: Clean audit log.
	auditCutoff := now.AddDate(0, 0, -90)
	_ = s.db.DeleteAuditEntriesBefore(auditCutoff)
}
```

- [ ] **Step 2: Run tests**

Run: `cd internal/nvr/scheduler && go test ./... -v`
Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: All pass, clean build

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat: scheduler supports per-stream retention with camera-level fallback"
```

---

### Task 6: Integration Test

**Files:**

- Modify: `internal/nvr/db/retention_test.go`

- [ ] **Step 1: Write per-stream retention flow test**

Append to `internal/nvr/db/retention_test.go`:

```go
func TestPerStreamRetentionFlow(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "multi-stream"}
	require.NoError(t, d.CreateCamera(cam))

	// Main stream: 3-day no-event, 365-day event retention.
	main := &CameraStream{
		CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x",
		RetentionDays: 3, EventRetentionDays: 365,
	}
	require.NoError(t, d.CreateCameraStream(main))

	// Sub stream: 7-day no-event, 30-day event retention.
	sub := &CameraStream{
		CameraID: cam.ID, Name: "Sub", RTSPURL: "rtsp://y",
		RetentionDays: 7, EventRetentionDays: 30,
	}
	require.NoError(t, d.CreateCameraStream(sub))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// Main stream recording (5 days old, no event) — should be deleted (> 3 days).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: main.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/main-quiet.mp4", FileSize: 1000, Format: "fmp4",
	}))

	// Sub stream recording (5 days old, no event) — should survive (< 7 days).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: sub.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/sub-quiet.mp4", FileSize: 1000, Format: "fmp4",
	}))

	// Delete main stream no-event recordings (3-day cutoff).
	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteStreamRecordingsWithoutEvents(cam.ID, main.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/main-quiet.mp4"}, paths)

	// Delete sub stream no-event recordings (7-day cutoff) — should find nothing.
	cutoff7 := now.AddDate(0, 0, -7)
	paths, err = d.DeleteStreamRecordingsWithoutEvents(cam.ID, sub.ID, cutoff7)
	require.NoError(t, err)
	assert.Empty(t, paths, "sub stream recording is only 5 days old, 7-day cutoff should spare it")

	// Verify sub stream recording survived.
	recs, err := d.QueryRecordings(cam.ID, fiveDaysAgo.Add(-1*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, recs, 1)
	assert.Equal(t, "/tmp/sub-quiet.mp4", recs[0].FilePath)
}

func TestUpdateStreamRetention(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test"}
	require.NoError(t, d.CreateCamera(cam))

	stream := &CameraStream{CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCameraStream(stream))

	require.NoError(t, d.UpdateStreamRetention(stream.ID, 14, 365))

	updated, err := d.GetCameraStream(stream.ID)
	require.NoError(t, err)
	assert.Equal(t, 14, updated.RetentionDays)
	assert.Equal(t, 365, updated.EventRetentionDays)
}
```

- [ ] **Step 2: Run the full test suite**

Run: `cd internal/nvr/db && go test ./... -v -count=1`
Run: `cd internal/nvr && go build ./...`
Expected: All PASS, clean build

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/retention_test.go
git commit -m "test: add per-stream retention flow and stream retention update tests"
```

---

## Summary

| Task | What it does                                                           |
| ---- | ---------------------------------------------------------------------- |
| 1    | Migration 28 + Recording.StreamID + CameraStream retention fields      |
| 2    | OnSegmentComplete resolves and stores stream_id on recordings          |
| 3    | Per-stream deletion methods (DeleteStreamRecordingsWithout/WithEvents) |
| 4    | Storage estimate endpoint + stream retention API                       |
| 5    | Scheduler per-stream retention with camera-level fallback              |
| 6    | Integration tests                                                      |

After this plan completes, the Flutter UI plan can be written to consume these new endpoints.
