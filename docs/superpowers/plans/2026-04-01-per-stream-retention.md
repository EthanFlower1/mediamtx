# Per-Stream Retention Policies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the critical bug in per-stream retention cleanup and complete the missing pieces (stream-scoped date-range deletion, per-stream storage stats API) so that per-stream retention works end-to-end.

**Architecture:** The schema, API endpoint, scheduler loop, and Flutter UI for per-stream retention already exist. This plan fixes a bug where stream retention without event retention deletes ALL camera recordings instead of only that stream's, adds the missing `DeleteStreamRecordingsByDateRange` DB function, adds a `GetStoragePerStream` query with API endpoint, and verifies the full flow with tests.

**Tech Stack:** Go (SQLite via modernc.org/sqlite), Gin HTTP framework, Flutter/Dart

---

### Task 1: Add `DeleteStreamRecordingsByDateRange` DB Function

**Files:**

- Modify: `internal/nvr/db/retention.go` (after line 356)
- Modify: `internal/nvr/db/retention_test.go` (append)

- [ ] **Step 1: Write the failing test**

Add to `internal/nvr/db/retention_test.go`:

```go
func TestDeleteStreamRecordingsByDateRange(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	mainStream := &CameraStream{CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCameraStream(mainStream))

	subStream := &CameraStream{CameraID: cam.ID, Name: "Sub", RTSPURL: "rtsp://y"}
	require.NoError(t, d.CreateCameraStream(subStream))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	// Main stream recording (5 days old).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: mainStream.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/main-old.mp4", FileSize: 1000, Format: "fmp4",
	}))

	// Sub stream recording (5 days old).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: subStream.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/sub-old.mp4", FileSize: 1000, Format: "fmp4",
	}))

	// Delete only main stream recordings older than 3 days.
	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteStreamRecordingsByDateRange(cam.ID, mainStream.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/main-old.mp4"}, paths)

	// Sub stream recording must survive.
	recs, err := d.QueryRecordings(cam.ID, fiveDaysAgo.Add(-1*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, recs, 1)
	assert.Equal(t, "/tmp/sub-old.mp4", recs[0].FilePath)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestDeleteStreamRecordingsByDateRange -v`
Expected: FAIL — `d.DeleteStreamRecordingsByDateRange` undefined

- [ ] **Step 3: Write minimal implementation**

Add to `internal/nvr/db/retention.go` after `DeleteStreamRecordingsWithEvents` (after line 356):

```go
// DeleteStreamRecordingsByDateRange deletes all recordings for a specific
// stream whose end_time is before the given cutoff, regardless of event
// overlap. Returns deleted file paths for disk cleanup.
func (d *DB) DeleteStreamRecordingsByDateRange(cameraID, streamID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(
		`SELECT file_path FROM recordings WHERE camera_id = ? AND stream_id = ? AND end_time < ?`,
		cameraID, streamID, beforeStr,
	)
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
		_, err = d.Exec(
			`DELETE FROM recordings WHERE camera_id = ? AND stream_id = ? AND end_time < ?`,
			cameraID, streamID, beforeStr,
		)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestDeleteStreamRecordingsByDateRange -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/retention.go internal/nvr/db/retention_test.go
git commit -m "feat(db): add DeleteStreamRecordingsByDateRange for stream-scoped cleanup"
```

---

### Task 2: Fix Scheduler Bug — Use Stream-Scoped Deletion

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go:622`

- [ ] **Step 1: Fix the bug**

In `internal/nvr/scheduler/scheduler.go`, replace line 622:

```go
// BEFORE (bug: deletes ALL camera recordings, not just this stream's):
paths, err := s.db.DeleteRecordingsByDateRange(cam.ID, noEventCutoff)
```

With:

```go
// AFTER (correct: deletes only this stream's recordings):
paths, err := s.db.DeleteStreamRecordingsByDateRange(cam.ID, stream.ID, noEventCutoff)
```

The full else block (lines 621-629) should read:

```go
			} else {
				paths, err := s.db.DeleteStreamRecordingsByDateRange(cam.ID, stream.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: stream retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}
			}
```

- [ ] **Step 2: Run existing tests to verify nothing breaks**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Compiles successfully

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "fix(scheduler): use stream-scoped deletion to avoid deleting other streams' recordings"
```

---

### Task 3: Add `GetStoragePerStream` DB Function

**Files:**

- Modify: `internal/nvr/db/recordings.go` (after `GetStoragePerCamera`, ~line 272)
- Modify: `internal/nvr/db/retention_test.go` (append)

- [ ] **Step 1: Write the failing test**

Add to `internal/nvr/db/retention_test.go`:

```go
func TestGetStoragePerStream(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	main := &CameraStream{CameraID: cam.ID, Name: "Main", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCameraStream(main))

	sub := &CameraStream{CameraID: cam.ID, Name: "Sub", RTSPURL: "rtsp://y"}
	require.NoError(t, d.CreateCameraStream(sub))

	now := time.Now().UTC()
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: main.ID,
		StartTime: now.Format(timeFormat),
		EndTime:   now.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/main1.mp4", FileSize: 5000, Format: "fmp4",
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: main.ID,
		StartTime: now.Add(20 * time.Minute).Format(timeFormat),
		EndTime:   now.Add(30 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/main2.mp4", FileSize: 3000, Format: "fmp4",
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StreamID: sub.ID,
		StartTime: now.Format(timeFormat),
		EndTime:   now.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/sub1.mp4", FileSize: 1000, Format: "fmp4",
	}))

	results, err := d.GetStoragePerStream(cam.ID)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Results ordered by stream name.
	assert.Equal(t, main.ID, results[0].StreamID)
	assert.Equal(t, "Main", results[0].StreamName)
	assert.Equal(t, int64(8000), results[0].TotalBytes)
	assert.Equal(t, int64(2), results[0].SegmentCount)

	assert.Equal(t, sub.ID, results[1].StreamID)
	assert.Equal(t, "Sub", results[1].StreamName)
	assert.Equal(t, int64(1000), results[1].TotalBytes)
	assert.Equal(t, int64(1), results[1].SegmentCount)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetStoragePerStream -v`
Expected: FAIL — `d.GetStoragePerStream` undefined

- [ ] **Step 3: Write minimal implementation**

Add to `internal/nvr/db/recordings.go` after `GetStoragePerCamera` (after line 272):

```go
// StreamStorage holds aggregate storage statistics for a single stream.
type StreamStorage struct {
	StreamID     string `json:"stream_id"`
	StreamName   string `json:"stream_name"`
	TotalBytes   int64  `json:"total_bytes"`
	SegmentCount int64  `json:"segment_count"`
}

// GetStoragePerStream returns total storage used and segment count per stream
// for a given camera.
func (d *DB) GetStoragePerStream(cameraID string) ([]StreamStorage, error) {
	rows, err := d.Query(`
		SELECT r.stream_id, COALESCE(cs.name, ''), COALESCE(SUM(r.file_size), 0), COUNT(*)
		FROM recordings r
		LEFT JOIN camera_streams cs ON cs.id = r.stream_id
		WHERE r.camera_id = ?
		GROUP BY r.stream_id
		ORDER BY COALESCE(cs.name, '')`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []StreamStorage
	for rows.Next() {
		var ss StreamStorage
		if err := rows.Scan(&ss.StreamID, &ss.StreamName, &ss.TotalBytes, &ss.SegmentCount); err != nil {
			return nil, err
		}
		results = append(results, ss)
	}
	return results, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetStoragePerStream -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/recordings.go internal/nvr/db/retention_test.go
git commit -m "feat(db): add GetStoragePerStream for per-stream storage breakdown"
```

---

### Task 4: Add Stream Storage API Endpoint

**Files:**

- Modify: `internal/nvr/api/streams.go` (append handler)
- Modify: `internal/nvr/api/router.go` (register route)

- [ ] **Step 1: Add the handler**

Add to `internal/nvr/api/streams.go` after `UpdateRetention` (after line 335):

```go
// GetStreamStorage returns per-stream storage breakdown for a camera.
func (h *StreamHandler) GetStreamStorage(c *gin.Context) {
	cameraID := c.Param("id")

	results, err := h.DB.GetStoragePerStream(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query stream storage", err)
		return
	}

	if results == nil {
		results = []db.StreamStorage{}
	}

	c.JSON(http.StatusOK, gin.H{"streams": results})
}
```

- [ ] **Step 2: Register the route**

In `internal/nvr/api/router.go`, find the line:

```go
protected.PUT("/streams/:id/retention", streamHandler.UpdateRetention)
```

Add after it:

```go
protected.GET("/cameras/:id/stream-storage", streamHandler.GetStreamStorage)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Compiles successfully

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/streams.go internal/nvr/api/router.go
git commit -m "feat(api): add GET /cameras/:id/stream-storage endpoint"
```

---

### Task 5: Run Full Test Suite

**Files:** None (verification only)

- [ ] **Step 1: Run all DB retention tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -v -count=1`
Expected: All tests pass

- [ ] **Step 2: Run full NVR package build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Compiles with no errors
