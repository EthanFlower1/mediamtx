# KAI-14: Recording Statistics API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose per-camera recording uptime, total storage, segment count, and gap history via two API endpoints.

**Architecture:** Two new endpoints on the existing NVR API: a summary endpoint (`GET /api/nvr/recordings/stats`) returning aggregate stats for all cameras, and a per-camera gap history endpoint (`GET /api/nvr/recordings/stats/:camera_id/gaps`). New DB methods compute aggregates and detect gaps using SQL window functions. A new `StatsHandler` struct in a dedicated file follows the existing handler pattern.

**Tech Stack:** Go, Gin, SQLite (window functions), testify

---

## File Structure

| Action | File                             | Responsibility                                                                              |
| ------ | -------------------------------- | ------------------------------------------------------------------------------------------- |
| Create | `internal/nvr/db/stats.go`       | `RecordingStats` and `Gap` types, `GetRecordingStats()` and `GetRecordingGaps()` DB methods |
| Create | `internal/nvr/db/stats_test.go`  | Unit tests for the DB stats methods                                                         |
| Create | `internal/nvr/api/stats.go`      | `StatsHandler` struct with `GetStats` and `GetGaps` HTTP handlers                           |
| Create | `internal/nvr/api/stats_test.go` | Unit tests for the API handlers                                                             |
| Modify | `internal/nvr/api/router.go:244` | Register the two new routes on the protected group                                          |

---

### Task 1: DB Stats Types and GetRecordingStats Query

**Files:**

- Create: `internal/nvr/db/stats.go`
- Test: `internal/nvr/db/stats_test.go`

- [ ] **Step 1: Write the failing test for GetRecordingStats**

Create `internal/nvr/db/stats_test.go`:

```go
package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRecordingStats_NoRecordings(t *testing.T) {
	d := newTestDB(t)

	stats, err := d.GetRecordingStats("")
	require.NoError(t, err)
	require.Empty(t, stats)
}

func TestGetRecordingStats_SingleCamera(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T10:00:00.000Z",
		EndTime:    "2025-01-15T11:00:00.000Z",
		DurationMs: 3600000,
		FilePath:   "/recordings/a.mp4",
		FileSize:   500000,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T11:00:00.000Z",
		EndTime:    "2025-01-15T12:00:00.000Z",
		DurationMs: 3600000,
		FilePath:   "/recordings/b.mp4",
		FileSize:   600000,
	}))

	stats, err := d.GetRecordingStats("")
	require.NoError(t, err)
	require.Len(t, stats, 1)

	s := stats[0]
	require.Equal(t, cam.ID, s.CameraID)
	require.Equal(t, "RecCam", s.CameraName)
	require.Equal(t, int64(1100000), s.TotalBytes)
	require.Equal(t, int64(2), s.SegmentCount)
	require.Equal(t, int64(7200000), s.TotalRecordedMs)
	require.Equal(t, "2025-01-15T10:00:00.000Z", s.OldestRecording)
	require.Equal(t, "2025-01-15T12:00:00.000Z", s.NewestRecording)
}

func TestGetRecordingStats_FilterByCamera(t *testing.T) {
	d := newTestDB(t)
	cam1 := createTestCameraForRecordings(t, d)
	cam2 := &Camera{Name: "OtherCam", RTSPURL: "rtsp://192.168.1.21/stream"}
	require.NoError(t, d.CreateCamera(cam2))

	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam1.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/c1.mp4", FileSize: 100000,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam2.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/c2.mp4", FileSize: 200000,
	}))

	stats, err := d.GetRecordingStats(cam1.ID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, cam1.ID, stats[0].CameraID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetRecordingStats -v`
Expected: Compilation error — `GetRecordingStats` not defined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/nvr/db/stats.go`:

```go
package db

// RecordingStats holds aggregate recording statistics for a single camera.
type RecordingStats struct {
	CameraID        string `json:"camera_id"`
	CameraName      string `json:"camera_name"`
	TotalBytes      int64  `json:"total_bytes"`
	SegmentCount    int64  `json:"segment_count"`
	TotalRecordedMs int64  `json:"total_recorded_ms"`
	OldestRecording string `json:"oldest_recording"`
	NewestRecording string `json:"newest_recording"`
}

// GetRecordingStats returns aggregate recording statistics per camera.
// If cameraID is non-empty, results are filtered to that camera only.
func (d *DB) GetRecordingStats(cameraID string) ([]RecordingStats, error) {
	query := `
		SELECT r.camera_id, COALESCE(c.name, ''),
			COALESCE(SUM(r.file_size), 0),
			COUNT(*),
			COALESCE(SUM(r.duration_ms), 0),
			MIN(r.start_time),
			MAX(r.end_time)
		FROM recordings r
		LEFT JOIN cameras c ON c.id = r.camera_id`

	var args []interface{}
	if cameraID != "" {
		query += " WHERE r.camera_id = ?"
		args = append(args, cameraID)
	}
	query += " GROUP BY r.camera_id ORDER BY COALESCE(c.name, '')"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RecordingStats
	for rows.Next() {
		var s RecordingStats
		if err := rows.Scan(&s.CameraID, &s.CameraName, &s.TotalBytes,
			&s.SegmentCount, &s.TotalRecordedMs, &s.OldestRecording, &s.NewestRecording); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetRecordingStats -v`
Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/stats.go internal/nvr/db/stats_test.go
git commit -m "feat(kai-14): add GetRecordingStats DB method with aggregate per-camera stats"
```

---

### Task 2: DB Gap Detection Query

**Files:**

- Modify: `internal/nvr/db/stats.go`
- Modify: `internal/nvr/db/stats_test.go`

- [ ] **Step 1: Write the failing tests for GetRecordingGaps**

Append to `internal/nvr/db/stats_test.go`:

```go
func TestGetRecordingGaps_NoRecordings(t *testing.T) {
	d := newTestDB(t)

	gaps, err := d.GetRecordingGaps("nonexistent", 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}

func TestGetRecordingGaps_NoGaps(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Two contiguous recordings (no gap).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/g1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/g2.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}

func TestGetRecordingGaps_WithGaps(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Three recordings with a 5-minute gap between second and third.
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/h1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/h2.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T12:05:00.000Z",
		EndTime: "2025-01-15T13:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/h3.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	require.Equal(t, "2025-01-15T12:00:00.000Z", gaps[0].Start)
	require.Equal(t, "2025-01-15T12:05:00.000Z", gaps[0].End)
	require.Equal(t, int64(300000), gaps[0].DurationMs)
}

func TestGetRecordingGaps_SmallGapFiltered(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Two recordings with a 1-second gap (below 2000ms threshold).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/s1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:01.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3599000,
		FilePath: "/recordings/s2.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetRecordingGaps -v`
Expected: Compilation error — `GetRecordingGaps` not defined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/nvr/db/stats.go`:

```go
// Gap represents a period where no recording exists for a camera.
type Gap struct {
	Start      string `json:"start"`
	End        string `json:"end"`
	DurationMs int64  `json:"duration_ms"`
}

// GetRecordingGaps returns all gaps between consecutive recordings for a camera
// where the gap duration exceeds gapThresholdMs milliseconds.
func (d *DB) GetRecordingGaps(cameraID string, gapThresholdMs int64) ([]Gap, error) {
	rows, err := d.Query(`
		SELECT end_time, next_start,
			CAST((julianday(next_start) - julianday(end_time)) * 86400000 AS INTEGER) AS gap_ms
		FROM (
			SELECT end_time,
				LEAD(start_time) OVER (ORDER BY start_time) AS next_start
			FROM recordings
			WHERE camera_id = ?
		)
		WHERE next_start IS NOT NULL
		  AND CAST((julianday(next_start) - julianday(end_time)) * 86400000 AS INTEGER) > ?
		ORDER BY end_time`, cameraID, gapThresholdMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gaps []Gap
	for rows.Next() {
		var g Gap
		if err := rows.Scan(&g.Start, &g.End, &g.DurationMs); err != nil {
			return nil, err
		}
		gaps = append(gaps, g)
	}
	return gaps, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestGetRecordingGaps -v`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/stats.go internal/nvr/db/stats_test.go
git commit -m "feat(kai-14): add GetRecordingGaps DB method with window function gap detection"
```

---

### Task 3: Stats API Handler

**Files:**

- Create: `internal/nvr/api/stats.go`
- Create: `internal/nvr/api/stats_test.go`

- [ ] **Step 1: Write the failing test for GetStats handler**

Create `internal/nvr/api/stats_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupStatsTest(t *testing.T) (*db.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	engine := gin.New()
	handler := &StatsHandler{DB: d}

	// Simulate authenticated user with wildcard camera permissions.
	engine.Use(func(c *gin.Context) {
		c.Set("camera_permissions", "*")
		c.Next()
	})
	engine.GET("/api/nvr/recordings/stats", handler.GetStats)
	engine.GET("/api/nvr/recordings/stats/:camera_id/gaps", handler.GetGaps)
	return d, engine
}

func TestGetStats_Empty(t *testing.T) {
	_, engine := setupStatsTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []json.RawMessage `json:"cameras"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Cameras)
}

func TestGetStats_WithRecordings(t *testing.T) {
	d, engine := setupStatsTest(t)

	cam := &db.Camera{Name: "TestCam", RTSPURL: "rtsp://test/stream"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/t1.mp4", FileSize: 500000,
	}))
	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:05:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/t2.mp4", FileSize: 400000,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []struct {
			CameraID        string `json:"camera_id"`
			CameraName      string `json:"camera_name"`
			TotalBytes      int64  `json:"total_bytes"`
			SegmentCount    int64  `json:"segment_count"`
			TotalRecordedMs int64  `json:"total_recorded_ms"`
			CurrentUptimeMs int64  `json:"current_uptime_ms"`
			GapCount        int    `json:"gap_count"`
		} `json:"cameras"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Cameras, 1)

	s := resp.Cameras[0]
	require.Equal(t, cam.ID, s.CameraID)
	require.Equal(t, int64(900000), s.TotalBytes)
	require.Equal(t, int64(2), s.SegmentCount)
	require.Equal(t, int64(6900000), s.TotalRecordedMs)
	require.Equal(t, 1, s.GapCount)
	// Current uptime = newest recording end - last gap end = 12:00 - 11:05 = 55 min = 3300000ms
	require.Equal(t, int64(3300000), s.CurrentUptimeMs)
}

func TestGetGaps_WithGap(t *testing.T) {
	d, engine := setupStatsTest(t)

	cam := &db.Camera{Name: "GapCam", RTSPURL: "rtsp://test/stream"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/gap1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:05:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/gap2.mp4", FileSize: 100,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats/"+cam.ID+"/gaps", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		CameraID string   `json:"camera_id"`
		Gaps     []db.Gap `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, cam.ID, resp.CameraID)
	require.Len(t, resp.Gaps, 1)
	require.Equal(t, "2025-01-15T11:00:00.000Z", resp.Gaps[0].Start)
	require.Equal(t, "2025-01-15T11:05:00.000Z", resp.Gaps[0].End)
	require.Equal(t, int64(300000), resp.Gaps[0].DurationMs)
}

func TestGetGaps_NotFound(t *testing.T) {
	_, engine := setupStatsTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats/nonexistent/gaps", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Gaps []db.Gap `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Gaps)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run "TestGetStats|TestGetGaps" -v`
Expected: Compilation error — `StatsHandler` not defined.

- [ ] **Step 3: Write the StatsHandler implementation**

Create `internal/nvr/api/stats.go`:

```go
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

const defaultGapThresholdMs = 2000

// StatsHandler implements HTTP endpoints for recording statistics.
type StatsHandler struct {
	DB *db.DB
}

// cameraStatsResponse is the per-camera entry in the stats response.
type cameraStatsResponse struct {
	CameraID        string  `json:"camera_id"`
	CameraName      string  `json:"camera_name"`
	TotalBytes      int64   `json:"total_bytes"`
	SegmentCount    int64   `json:"segment_count"`
	TotalRecordedMs int64   `json:"total_recorded_ms"`
	CurrentUptimeMs int64   `json:"current_uptime_ms"`
	LastGapEnd      *string `json:"last_gap_end"`
	OldestRecording string  `json:"oldest_recording"`
	NewestRecording string  `json:"newest_recording"`
	GapCount        int     `json:"gap_count"`
}

// GetStats returns aggregate recording statistics per camera.
// Optional query param: camera_id to filter to a single camera.
func (h *StatsHandler) GetStats(c *gin.Context) {
	cameraID := c.Query("camera_id")

	stats, err := h.DB.GetRecordingStats(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recording stats", err)
		return
	}

	cameras := make([]cameraStatsResponse, 0, len(stats))
	for _, s := range stats {
		gaps, err := h.DB.GetRecordingGaps(s.CameraID, defaultGapThresholdMs)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "failed to query recording gaps", err)
			return
		}

		entry := cameraStatsResponse{
			CameraID:        s.CameraID,
			CameraName:      s.CameraName,
			TotalBytes:      s.TotalBytes,
			SegmentCount:    s.SegmentCount,
			TotalRecordedMs: s.TotalRecordedMs,
			OldestRecording: s.OldestRecording,
			NewestRecording: s.NewestRecording,
			GapCount:        len(gaps),
		}

		if len(gaps) > 0 {
			lastGapEnd := gaps[len(gaps)-1].End
			entry.LastGapEnd = &lastGapEnd
			// Current uptime = newest recording end - last gap end.
			newest, err1 := time.Parse("2006-01-02T15:04:05.000Z", s.NewestRecording)
			lastEnd, err2 := time.Parse("2006-01-02T15:04:05.000Z", lastGapEnd)
			if err1 == nil && err2 == nil {
				entry.CurrentUptimeMs = newest.Sub(lastEnd).Milliseconds()
			}
		} else if s.OldestRecording != "" {
			// No gaps: uptime = total span from oldest to newest.
			oldest, err1 := time.Parse("2006-01-02T15:04:05.000Z", s.OldestRecording)
			newest, err2 := time.Parse("2006-01-02T15:04:05.000Z", s.NewestRecording)
			if err1 == nil && err2 == nil {
				entry.CurrentUptimeMs = newest.Sub(oldest).Milliseconds()
			}
		}

		cameras = append(cameras, entry)
	}

	c.JSON(http.StatusOK, gin.H{"cameras": cameras})
}

// GetGaps returns the full gap history for a single camera.
// Path param: camera_id.
func (h *StatsHandler) GetGaps(c *gin.Context) {
	cameraID := c.Param("camera_id")

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	gaps, err := h.DB.GetRecordingGaps(cameraID, defaultGapThresholdMs)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recording gaps", err)
		return
	}

	if gaps == nil {
		gaps = []db.Gap{}
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": cameraID,
		"gaps":      gaps,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run "TestGetStats|TestGetGaps" -v`
Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/stats.go internal/nvr/api/stats_test.go
git commit -m "feat(kai-14): add StatsHandler with GetStats and GetGaps API endpoints"
```

---

### Task 4: Register Routes

**Files:**

- Modify: `internal/nvr/api/router.go:72-74` (add handler init) and `internal/nvr/api/router.go:244` (add routes)

- [ ] **Step 1: Add StatsHandler initialization in RegisterRoutes**

In `internal/nvr/api/router.go`, after the `recordingHandler` initialization (line 74), add:

```go
	statsHandler := &StatsHandler{
		DB: cfg.DB,
	}
```

- [ ] **Step 2: Register the stats routes**

In `internal/nvr/api/router.go`, after the recording routes block (after line 249, the `protected.GET("/timeline/intensity", ...)` line), add:

```go
	// Recording statistics.
	protected.GET("/recordings/stats", statsHandler.GetStats)
	protected.GET("/recordings/stats/:camera_id/gaps", statsHandler.GetGaps)
```

- [ ] **Step 3: Run all tests to verify nothing is broken**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v -count=1 2>&1 | tail -30`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(kai-14): register recording stats endpoints in router"
```

---

### Task 5: Final Verification

- [ ] **Step 1: Run the full NVR test suite**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v -count=1`
Expected: All tests PASS with no failures.

- [ ] **Step 2: Verify the project compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Clean build with no errors.

- [ ] **Step 3: Commit the spec document**

```bash
git add docs/superpowers/specs/2026-04-01-recording-statistics-api-design.md docs/superpowers/plans/2026-04-01-recording-statistics-api.md
git commit -m "docs(kai-14): add recording statistics API design spec and implementation plan"
```
