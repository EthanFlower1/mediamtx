# Per-Camera Storage Paths Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow each camera to record to a configurable storage location (e.g., NAS mount), with automatic failover to local storage and deferred sync-back.

**Architecture:** NVR-layer StorageManager service owns all storage intelligence. MediaMTX records wherever its `recordPath` config points — the NVR dynamically rewrites that config based on storage health. Per-camera storage paths are stored in the DB, and the MediaMTX path naming convention changes from `nvr/<name>` to `nvr/<camera_id>/main` (and `/sub`) to encode camera ID and stream type.

**Tech Stack:** Go (server), SQLite (database), Flutter/Dart (client), MediaMTX YAML config

**Spec:** `docs/superpowers/specs/2026-03-25-per-camera-storage-design.md`

---

### Task 1: Database Migration — Add `storage_path` to cameras and create `pending_syncs` table

**Files:**
- Modify: `internal/nvr/db/migrations.go:237-253` (append migration 18)
- Modify: `internal/nvr/db/cameras.go:14-43` (add StoragePath field to Camera struct)
- Modify: `internal/nvr/db/cameras.go:48-78` (update CreateCamera INSERT)
- Modify: `internal/nvr/db/cameras.go:80-100` (update GetCamera SELECT/Scan)
- Modify: `internal/nvr/db/cameras.go:112-140` (update GetCameraByPath SELECT/Scan)
- Modify: `internal/nvr/db/cameras.go:179-213` (update UpdateCamera)
- Modify: `internal/nvr/db/cameras.go` (update ListCameras SELECT/Scan)
- Test: `internal/nvr/db/cameras_test.go`

- [ ] **Step 1: Write failing test for storage_path on Camera**

Add to `internal/nvr/db/cameras_test.go`:

```go
func TestCameraStoragePath(t *testing.T) {
	d := newTestDB(t)

	cam := &db.Camera{
		Name:        "NAS Camera",
		RTSPURL:     "rtsp://example.com/stream",
		StoragePath: "/mnt/nas1/recordings",
	}
	require.NoError(t, d.CreateCamera(cam))
	require.NotEmpty(t, cam.ID)

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, "/mnt/nas1/recordings", got.StoragePath)

	// Update storage path
	got.StoragePath = "/mnt/nas2/recordings"
	require.NoError(t, d.UpdateCamera(got))

	got2, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, "/mnt/nas2/recordings", got2.StoragePath)

	// Empty storage path means default
	got2.StoragePath = ""
	require.NoError(t, d.UpdateCamera(got2))
	got3, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got3.StoragePath)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestCameraStoragePath -v`
Expected: FAIL — `StoragePath` field doesn't exist on Camera struct

- [ ] **Step 3: Add migration 18 and update Camera struct**

In `internal/nvr/db/migrations.go`, append after migration 17:

```go
{
	version: 18,
	sql: `
ALTER TABLE cameras ADD COLUMN storage_path TEXT NOT NULL DEFAULT '';
CREATE TABLE pending_syncs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recording_id INTEGER NOT NULL,
    camera_id TEXT NOT NULL,
    local_path TEXT NOT NULL,
    target_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL,
    last_attempt_at TEXT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_pending_syncs_status ON pending_syncs(status);
CREATE INDEX idx_pending_syncs_camera ON pending_syncs(camera_id);
`,
},
```

In `internal/nvr/db/cameras.go`, add `StoragePath` to the Camera struct (after `AudioTranscode`):

```go
StoragePath  string `json:"storage_path"`
```

Update `CreateCamera` INSERT to include `storage_path` in column list and `cam.StoragePath` in values.

Update `GetCamera` SELECT to include `storage_path` in the column list and `&cam.StoragePath` in the Scan.

Update `UpdateCamera` SET to include `storage_path = ?` and `cam.StoragePath` in parameters.

Update `ListCameras` SELECT/Scan the same way.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestCameraStoragePath -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/cameras.go internal/nvr/db/cameras_test.go
git commit -m "feat(storage): add storage_path to cameras and pending_syncs table"
```

---

### Task 2: Database — `pending_syncs` CRUD operations

**Files:**
- Create: `internal/nvr/db/pending_syncs.go`
- Create: `internal/nvr/db/pending_syncs_test.go`

- [ ] **Step 1: Write failing tests for PendingSync CRUD**

Create `internal/nvr/db/pending_syncs_test.go`:

```go
package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingSyncCRUD(t *testing.T) {
	d := newTestDB(t)

	// Create a camera and recording first.
	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	// Insert pending sync.
	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas1/recordings/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))
	assert.NotZero(t, ps.ID)

	// List pending by status.
	pending, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, ps.LocalPath, pending[0].LocalPath)

	// Update status to syncing.
	require.NoError(t, d.SetPendingSyncStatus(ps.ID, "syncing"))

	pending, err = d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)

	syncing, err := d.ListPendingSyncs("syncing")
	require.NoError(t, err)
	assert.Len(t, syncing, 1)

	// Mark failed with error message (increments attempts).
	require.NoError(t, d.RecordPendingSyncFailure(ps.ID, "failed", "connection refused"))
	failed, err := d.ListPendingSyncs("failed")
	require.NoError(t, err)
	assert.Equal(t, "connection refused", failed[0].ErrorMessage)

	// Delete after sync.
	require.NoError(t, d.DeletePendingSync(ps.ID))
	all, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, all, 0)
}

func TestPendingSyncCascadeDelete(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))

	// Deleting the recording should cascade-delete the pending sync.
	require.NoError(t, d.DeleteRecordingByPath(rec.FilePath))
	pending, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestPendingSyncCountByCamera(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))

	counts, err := d.PendingSyncCountByCamera()
	require.NoError(t, err)
	assert.Equal(t, 1, counts[cam.ID])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestPendingSync -v`
Expected: FAIL — types and functions don't exist

- [ ] **Step 3: Implement pending_syncs CRUD**

Create `internal/nvr/db/pending_syncs.go`:

```go
package db

import "time"

// PendingSync represents a recording file that needs to be moved from local
// fallback storage to its target (e.g., NAS) location.
type PendingSync struct {
	ID            int64  `json:"id"`
	RecordingID   int64  `json:"recording_id"`
	CameraID      string `json:"camera_id"`
	LocalPath     string `json:"local_path"`
	TargetPath    string `json:"target_path"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	ErrorMessage  string `json:"error_message"`
	CreatedAt     string `json:"created_at"`
	LastAttemptAt string `json:"last_attempt_at"`
}

// InsertPendingSync inserts a new pending sync record.
func (d *DB) InsertPendingSync(ps *PendingSync) error {
	if ps.Status == "" {
		ps.Status = "pending"
	}
	ps.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	result, err := d.Exec(`
		INSERT INTO pending_syncs (recording_id, camera_id, local_path, target_path, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ps.RecordingID, ps.CameraID, ps.LocalPath, ps.TargetPath, ps.Status, ps.CreatedAt,
	)
	if err != nil {
		return err
	}
	ps.ID, _ = result.LastInsertId()
	return nil
}

// ListPendingSyncs returns all pending syncs with the given status.
func (d *DB) ListPendingSyncs(status string) ([]*PendingSync, error) {
	rows, err := d.Query(`
		SELECT id, recording_id, camera_id, local_path, target_path, status,
			attempts, COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs WHERE status = ? ORDER BY created_at ASC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PendingSync
	for rows.Next() {
		ps := &PendingSync{}
		if err := rows.Scan(&ps.ID, &ps.RecordingID, &ps.CameraID,
			&ps.LocalPath, &ps.TargetPath, &ps.Status, &ps.Attempts,
			&ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt); err != nil {
			return nil, err
		}
		result = append(result, ps)
	}
	return result, rows.Err()
}

// SetPendingSyncStatus updates just the status of a pending sync (e.g., to "syncing").
func (d *DB) SetPendingSyncStatus(id int64, status string) error {
	_, err := d.Exec(`UPDATE pending_syncs SET status = ? WHERE id = ?`, status, id)
	return err
}

// RecordPendingSyncFailure increments attempt count, sets error message and last_attempt_at.
func (d *DB) RecordPendingSyncFailure(id int64, status, errorMsg string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := d.Exec(`
		UPDATE pending_syncs
		SET status = ?, error_message = ?, attempts = attempts + 1, last_attempt_at = ?
		WHERE id = ?`, status, errorMsg, now, id)
	return err
}

// GetPendingSync retrieves a single pending sync by ID.
func (d *DB) GetPendingSync(id int64) (*PendingSync, error) {
	ps := &PendingSync{}
	err := d.QueryRow(`
		SELECT id, recording_id, camera_id, local_path, target_path, status,
			attempts, COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs WHERE id = ?`, id,
	).Scan(&ps.ID, &ps.RecordingID, &ps.CameraID,
		&ps.LocalPath, &ps.TargetPath, &ps.Status, &ps.Attempts,
		&ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// DeletePendingSync removes a pending sync record by ID.
func (d *DB) DeletePendingSync(id int64) error {
	_, err := d.Exec(`DELETE FROM pending_syncs WHERE id = ?`, id)
	return err
}

// FileIsReferenced checks if a file path is referenced by any recording or pending sync.
func (d *DB) FileIsReferenced(filePath string) bool {
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM recordings WHERE file_path = ?`, filePath).Scan(&count)
	if count > 0 {
		return true
	}
	d.QueryRow(`SELECT COUNT(*) FROM pending_syncs WHERE local_path = ?`, filePath).Scan(&count)
	return count > 0
}

// PendingSyncCountByCamera returns a map of camera_id -> count of pending syncs.
func (d *DB) PendingSyncCountByCamera() (map[string]int, error) {
	rows, err := d.Query(`
		SELECT camera_id, COUNT(*) FROM pending_syncs
		WHERE status IN ('pending', 'syncing')
		GROUP BY camera_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var camID string
		var count int
		if err := rows.Scan(&camID, &count); err != nil {
			return nil, err
		}
		counts[camID] = count
	}
	return counts, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/ -run TestPendingSync -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/pending_syncs.go internal/nvr/db/pending_syncs_test.go
git commit -m "feat(storage): add pending_syncs CRUD operations"
```

---

### Task 3: Update MediaMTX path naming to use camera ID

**Files:**
- Modify: `internal/nvr/api/cameras.go:194-277` (Create handler — change path naming)
- Modify: `internal/nvr/nvr.go:553-624` (OnSegmentComplete — update camera discovery)
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write failing test for new path naming convention**

Add to `internal/nvr/api/cameras_test.go`:

```go
func TestCreateCameraPathUsesID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, cleanup := setupCameraTest(t)
	defer cleanup()

	body := `{"name":"Backyard","rtsp_url":"rtsp://example.com/stream"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/cameras", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Create(c)

	require.Equal(t, http.StatusCreated, w.Code)

	var cam db.Camera
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cam))

	// Path should be nvr/<camera-id>/main, not nvr/<sanitized-name>
	assert.Equal(t, "nvr/"+cam.ID+"/main", cam.MediaMTXPath)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCreateCameraPathUsesID -v`
Expected: FAIL — path is `nvr/backyard` instead of `nvr/<id>/main`

- [ ] **Step 3: Update Create handler to use camera ID in path**

In `internal/nvr/api/cameras.go`, modify the Create handler. The camera needs its ID generated before constructing the path. Change the flow:

```go
cam := &db.Camera{
	Name:          req.Name,
	// ... other fields
	StoragePath:   req.StoragePath,
}

// Generate ID early so we can use it in the path name.
if cam.ID == "" {
	cam.ID = uuid.New().String()
}
pathName := "nvr/" + cam.ID + "/main"
cam.MediaMTXPath = pathName
```

Add `uuid` import: `"github.com/google/uuid"`

Add `StoragePath string \`json:"storage_path"\`` to `cameraRequest` struct.

Add storage path validation before creating the camera:

```go
if req.StoragePath != "" {
	if !filepath.IsAbs(req.StoragePath) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path must be an absolute path"})
		return
	}
	// Normalize trailing slash.
	req.StoragePath = filepath.Clean(req.StoragePath)
	// Verify path exists and is writable.
	testFile := filepath.Join(req.StoragePath, ".nvr_write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path is not writable: " + err.Error()})
		return
	}
	os.Remove(testFile)
}
```

Update the YAML config to include `recordPath` and `record: true`:

```go
storagePath := cam.StoragePath
if storagePath == "" {
	storagePath = "./recordings"
}
recordPath := storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"

yamlConfig := map[string]interface{}{
	"source":     cam.RTSPURL,
	"record":     true,
	"recordPath": recordPath,
}
```

- [ ] **Step 4: Update OnSegmentComplete to discover camera by ID from path**

In `internal/nvr/nvr.go`, update `OnSegmentComplete`:

```go
func (n *NVR) OnSegmentComplete(filePath string, duration time.Duration) {
	var cam *db.Camera

	// Try to extract camera ID from new path convention: .../nvr/<camera-id>/main/...
	if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
		rest := filePath[idx+4:] // after "nvr/"
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) >= 1 {
			candidate := parts[0]
			if c, err := n.database.GetCamera(candidate); err == nil {
				cam = c
			}
		}
	}

	// Fallback: legacy substring match for pre-migration recordings.
	if cam == nil {
		cameras, err := n.database.ListCameras()
		if err != nil {
			return
		}
		for _, c := range cameras {
			if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
				cam = c
				break
			}
		}
	}

	if cam == nil {
		return
	}

	// ... rest of function unchanged
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCreateCameraPathUsesID -v && go test ./internal/nvr/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go internal/nvr/nvr.go
git commit -m "feat(storage): use camera ID in MediaMTX path naming convention"
```

---

### Task 4: Update camera API — storage_path on create/update, storage_status in response

**Files:**
- Modify: `internal/nvr/api/cameras.go:280-348` (Update handler — accept storage_path)
- Modify: `internal/nvr/api/cameras.go:108-192` (List/Get — add storage_status to response)
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write failing test for storage_path on update**

Add to `internal/nvr/api/cameras_test.go`:

```go
func TestUpdateCameraStoragePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a writable temp dir to use as NAS path.
	nasDir := t.TempDir()

	// Create camera first.
	cam := &db.Camera{Name: "Test", RTSPURL: "rtsp://x", MediaMTXPath: "nvr/test-id/main"}
	require.NoError(t, h.DB.CreateCamera(cam))

	body := fmt.Sprintf(`{"name":"Test","rtsp_url":"rtsp://x","storage_path":"%s"}`, nasDir)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/cameras/"+cam.ID, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: cam.ID}}
	h.Update(c)

	require.Equal(t, http.StatusOK, w.Code)

	got, err := h.DB.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, nasDir, got.StoragePath)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestUpdateCameraStoragePath -v`
Expected: FAIL

- [ ] **Step 3: Update camera Update handler to handle storage_path**

In `internal/nvr/api/cameras.go`, Update handler: add storage path validation (same as Create), update `existing.StoragePath = req.StoragePath`, and update the YAML `recordPath` when storage path changes via `h.YAMLWriter.SetPathValue()`.

```go
// In Update handler, after updating other fields:
if req.StoragePath != existing.StoragePath {
	existing.StoragePath = req.StoragePath
	storagePath := existing.StoragePath
	if storagePath == "" {
		storagePath = "./recordings"
	}
	recordPath := storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
	if err := h.YAMLWriter.SetPathValue(existing.MediaMTXPath, "recordPath", recordPath); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update record path", err)
		return
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestUpdateCameraStoragePath -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go
git commit -m "feat(storage): support storage_path on camera create/update API"
```

---

### Task 5: StorageManager — Health Monitor

**Files:**
- Create: `internal/nvr/storage/manager.go`
- Create: `internal/nvr/storage/manager_test.go`

- [ ] **Step 1: Write failing test for health monitoring**

Create `internal/nvr/storage/manager_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPathHealth_Healthy(t *testing.T) {
	dir := t.TempDir()
	healthy, err := checkPathHealth(dir)
	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestCheckPathHealth_Unreachable(t *testing.T) {
	healthy, err := checkPathHealth("/nonexistent/path/that/does/not/exist")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestCheckPathHealth_NotWritable(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o444)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	healthy, err := checkPathHealth(dir)
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestManager_EvaluateHealth(t *testing.T) {
	healthyDir := t.TempDir()
	badDir := "/nonexistent/storage/path"

	m := &Manager{
		health: make(map[string]bool),
	}

	m.evaluateHealth(map[string][]string{
		healthyDir: {"cam1"},
		badDir:     {"cam2"},
	})

	assert.True(t, m.GetHealth(healthyDir))
	assert.False(t, m.GetHealth(badDir))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/storage/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement StorageManager with health monitoring**

Create `internal/nvr/storage/manager.go`:

```go
// Package storage provides per-camera storage path management with health
// monitoring, automatic failover to local storage, and sync-back.
package storage

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

const (
	defaultCheckInterval = 30 * time.Second
	defaultMaxRetries    = 10
)

// Manager monitors storage path health, handles failover to local storage,
// and syncs fallback recordings to their target locations when storage recovers.
type Manager struct {
	db             *db.DB
	yamlWriter     *yamlwriter.Writer
	recordingsPath string // local fallback base path, e.g. "./recordings"
	apiAddress     string // for triggering MediaMTX config reload
	checkInterval  time.Duration
	maxRetries     int

	mu     sync.RWMutex
	health map[string]bool // storage path -> healthy

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new StorageManager.
func New(database *db.DB, yw *yamlwriter.Writer, recordingsPath, apiAddress string) *Manager {
	return &Manager{
		db:             database,
		yamlWriter:     yw,
		recordingsPath: recordingsPath,
		apiAddress:     apiAddress,
		checkInterval:  defaultCheckInterval,
		maxRetries:     defaultMaxRetries,
		health:         make(map[string]bool),
		stopCh:         make(chan struct{}),
	}
}

// Start runs the initial health check synchronously, then starts the
// background health monitor and sync worker goroutines.
func (m *Manager) Start() {
	m.runHealthCheck()
	m.wg.Add(2)
	go m.healthLoop()
	go m.syncLoop()
}

// Stop signals background goroutines to exit and waits for them.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// GetHealth returns the current health status of a storage path.
func (m *Manager) GetHealth(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health[path]
}

// GetAllHealth returns a copy of the health status map.
func (m *Manager) GetAllHealth() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]bool, len(m.health))
	for k, v := range m.health {
		result[k] = v
	}
	return result
}

// StorageStatus returns the storage status string for a camera.
func (m *Manager) StorageStatus(cam *db.Camera) string {
	if cam.StoragePath == "" {
		return "default"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.health[cam.StoragePath] {
		return "healthy"
	}
	return "degraded"
}

func (m *Manager) healthLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.runHealthCheck()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) runHealthCheck() {
	cameras, err := m.db.ListCameras()
	if err != nil {
		log.Printf("[NVR] [storage] failed to list cameras: %v", err)
		return
	}

	// Group cameras by storage path.
	pathCameras := make(map[string][]string)
	for _, cam := range cameras {
		if cam.StoragePath != "" {
			pathCameras[cam.StoragePath] = append(pathCameras[cam.StoragePath], cam.ID)
		}
	}

	m.evaluateHealth(pathCameras)
}

func (m *Manager) evaluateHealth(pathCameras map[string][]string) {
	for path, cameraIDs := range pathCameras {
		healthy, err := checkPathHealth(path)
		if err != nil {
			log.Printf("[NVR] [storage] health check error for %s: %v", path, err)
			continue
		}

		m.mu.Lock()
		wasHealthy, existed := m.health[path]
		m.health[path] = healthy
		m.mu.Unlock()

		if !existed {
			continue // First check, no transition to handle.
		}

		if wasHealthy && !healthy {
			log.Printf("[NVR] [storage] path %s became UNREACHABLE, failing over cameras %v", path, cameraIDs)
			m.handleFailover(path, cameraIDs)
		} else if !wasHealthy && healthy {
			log.Printf("[NVR] [storage] path %s is REACHABLE again, recovering cameras %v", path, cameraIDs)
			m.handleRecovery(path, cameraIDs)
		}
	}
}

// checkPathHealth verifies a storage path is reachable and writable.
func checkPathHealth(path string) (bool, error) {
	// Check path exists.
	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}
	if !info.IsDir() {
		return false, nil
	}

	// Write test file to verify write access.
	testFile := filepath.Join(path, ".nvr_health_check")
	if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
		return false, nil
	}
	os.Remove(testFile)

	return true, nil
}

func (m *Manager) handleFailover(storagePath string, cameraIDs []string) {
	for _, camID := range cameraIDs {
		cam, err := m.db.GetCamera(camID)
		if err != nil {
			continue
		}
		fallbackPath := m.recordingsPath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
		if err := m.yamlWriter.SetPathValue(cam.MediaMTXPath, "recordPath", fallbackPath); err != nil {
			log.Printf("[NVR] [storage] failed to failover camera %s: %v", camID, err)
		}
	}
	m.triggerConfigReload()
}

func (m *Manager) handleRecovery(storagePath string, cameraIDs []string) {
	for _, camID := range cameraIDs {
		cam, err := m.db.GetCamera(camID)
		if err != nil {
			continue
		}
		primaryPath := cam.StoragePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
		if err := m.yamlWriter.SetPathValue(cam.MediaMTXPath, "recordPath", primaryPath); err != nil {
			log.Printf("[NVR] [storage] failed to recover camera %s: %v", camID, err)
		}
	}
	m.triggerConfigReload()
}

// triggerConfigReload tells MediaMTX to reload its configuration.
func (m *Manager) triggerConfigReload() {
	url := fmt.Sprintf("http://localhost%s/v3/config/globalconf/patch", m.apiAddress)
	req, err := http.NewRequest("PATCH", url, strings.NewReader("{}"))
	if err != nil {
		log.Printf("[NVR] [storage] failed to create reload request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[NVR] [storage] config reload failed: %v", err)
		return
	}
	resp.Body.Close()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/storage/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/storage/manager.go internal/nvr/storage/manager_test.go
git commit -m "feat(storage): add StorageManager with health monitoring and failover"
```

---

### Task 6: StorageManager — Sync Worker

**Files:**
- Modify: `internal/nvr/storage/manager.go` (add syncLoop, processSync)
- Create: `internal/nvr/storage/sync_test.go`

- [ ] **Step 1: Write failing test for sync worker**

Create `internal/nvr/storage/sync_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSyncWorker_ProcessSync(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()
	targetDir := t.TempDir()

	cam := &db.Camera{Name: "test", RTSPURL: "rtsp://x", StoragePath: targetDir}
	require.NoError(t, database.CreateCamera(cam))

	// Create a local recording file.
	localFile := filepath.Join(localDir, "test.mp4")
	require.NoError(t, os.WriteFile(localFile, []byte("fake mp4 data"), 0o644))

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  localFile,
		FileSize:  13,
		Format:    "fmp4",
	}
	require.NoError(t, database.InsertRecording(rec))

	targetFile := filepath.Join(targetDir, "test.mp4")
	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   localFile,
		TargetPath:  targetFile,
	}
	require.NoError(t, database.InsertPendingSync(ps))

	m := &Manager{
		db:         database,
		maxRetries: 10,
		health:     make(map[string]bool),
	}

	m.processSync(ps)

	// Target file should exist.
	_, err := os.Stat(targetFile)
	require.NoError(t, err)

	// Local file should be deleted.
	_, err = os.Stat(localFile)
	assert.True(t, os.IsNotExist(err))

	// Recording file_path should be updated.
	updated, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, targetFile, updated.FilePath)

	// Pending sync should be deleted.
	pending, err := database.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestSyncWorker_ProcessSync_TargetUnreachable(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()

	cam := &db.Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, database.CreateCamera(cam))

	localFile := filepath.Join(localDir, "test.mp4")
	require.NoError(t, os.WriteFile(localFile, []byte("data"), 0o644))

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  localFile,
		Format:    "fmp4",
	}
	require.NoError(t, database.InsertRecording(rec))

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   localFile,
		TargetPath:  "/nonexistent/path/test.mp4",
	}
	require.NoError(t, database.InsertPendingSync(ps))

	m := &Manager{
		db:         database,
		maxRetries: 10,
		health:     make(map[string]bool),
	}

	m.processSync(ps)

	// Local file should still exist.
	_, err := os.Stat(localFile)
	require.NoError(t, err)

	// Pending sync should be marked syncing with error.
	syncing, err := database.ListPendingSyncs("syncing")
	require.NoError(t, err)
	assert.Len(t, syncing, 1)
	assert.NotEmpty(t, syncing[0].ErrorMessage)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/storage/ -run TestSyncWorker -v`
Expected: FAIL — processSync doesn't exist

- [ ] **Step 3: Implement sync worker**

Add to `internal/nvr/storage/manager.go`:

```go
const syncInterval = 60 * time.Second

func (m *Manager) syncLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.runSyncPass()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) runSyncPass() {
	pending, err := m.db.ListPendingSyncs("pending")
	if err != nil {
		log.Printf("[NVR] [storage] failed to list pending syncs: %v", err)
		return
	}

	for _, ps := range pending {
		select {
		case <-m.stopCh:
			return
		default:
		}
		m.processSync(ps)
	}

	// Orphan sweep: clean up local fallback files that have no DB references.
	m.sweepOrphans()
}

// sweepOrphans removes files in the local fallback directory that have no
// corresponding pending_syncs or recordings entry. This handles the case
// where a recording is deleted by retention while a sync is pending
// (cascade deletes the pending_syncs row, but the local file remains).
func (m *Manager) sweepOrphans() {
	cameras, err := m.db.ListCameras()
	if err != nil {
		return
	}

	for _, cam := range cameras {
		if cam.StoragePath == "" {
			continue // No custom storage, local IS the primary.
		}

		// Check the local fallback dir for this camera.
		localCamDir := filepath.Join(m.recordingsPath, "nvr", cam.ID)
		if _, err := os.Stat(localCamDir); os.IsNotExist(err) {
			continue
		}

		filepath.Walk(localCamDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// Check if this file is referenced by any recording or pending sync.
			if !m.db.FileIsReferenced(path) {
				os.Remove(path)
				log.Printf("[NVR] [storage] removed orphan file: %s", path)
			}
			return nil
		})
	}
}

func (m *Manager) processSync(ps *db.PendingSync) {
	// Mark as syncing to prevent double-processing (does not increment attempts).
	if err := m.db.SetPendingSyncStatus(ps.ID, "syncing"); err != nil {
		return
	}

	// Ensure target directory exists.
	targetDir := filepath.Dir(ps.TargetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		m.db.RecordPendingSyncFailure(ps.ID, "pending", fmt.Sprintf("mkdir failed: %v", err))
		m.checkMaxRetries(ps)
		return
	}

	// Read source file.
	data, err := os.ReadFile(ps.LocalPath)
	if err != nil {
		m.db.RecordPendingSyncFailure(ps.ID, "pending", fmt.Sprintf("read failed: %v", err))
		m.checkMaxRetries(ps)
		return
	}

	// Write to target.
	if err := os.WriteFile(ps.TargetPath, data, 0o644); err != nil {
		m.db.RecordPendingSyncFailure(ps.ID, "pending", fmt.Sprintf("write failed: %v", err))
		m.checkMaxRetries(ps)
		return
	}

	// Verify size.
	targetInfo, err := os.Stat(ps.TargetPath)
	if err != nil || targetInfo.Size() != int64(len(data)) {
		m.db.RecordPendingSyncFailure(ps.ID, "pending", "size verification failed")
		m.checkMaxRetries(ps)
		return
	}

	// Update recording file_path in DB.
	if err := m.db.UpdateRecordingFilePath(ps.RecordingID, ps.TargetPath); err != nil {
		log.Printf("[NVR] [storage] failed to update recording path: %v", err)
	}

	// Delete local copy.
	os.Remove(ps.LocalPath)

	// Delete pending sync record.
	m.db.DeletePendingSync(ps.ID)

	log.Printf("[NVR] [storage] synced recording %d: %s -> %s", ps.RecordingID, ps.LocalPath, ps.TargetPath)
}

func (m *Manager) checkMaxRetries(ps *db.PendingSync) {
	// Re-read to get the current attempt count after RecordPendingSyncFailure incremented it.
	current, err := m.db.GetPendingSync(ps.ID)
	if err != nil {
		return
	}
	if current.Attempts >= m.maxRetries {
		m.db.SetPendingSyncStatus(ps.ID, "failed")
		log.Printf("[NVR] [storage] sync permanently failed for recording %d after %d attempts", ps.RecordingID, m.maxRetries)
	}
}
```

Also add `UpdateRecordingFilePath` to `internal/nvr/db/recordings.go`:

```go
// UpdateRecordingFilePath updates the file_path of a recording.
func (d *DB) UpdateRecordingFilePath(id int64, filePath string) error {
	_, err := d.Exec(`UPDATE recordings SET file_path = ? WHERE id = ?`, filePath, id)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/storage/ -run TestSyncWorker -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/storage/manager.go internal/nvr/storage/sync_test.go internal/nvr/db/recordings.go
git commit -m "feat(storage): add sync worker for fallback recording recovery"
```

---

### Task 7: OnSegmentComplete — Pending sync detection

**Files:**
- Modify: `internal/nvr/nvr.go:553-624` (add pending sync detection after recording insert)

- [ ] **Step 1: Write failing test for pending sync auto-detection**

Add test in `internal/nvr/nvr_test.go` (create if needed):

```go
func TestOnSegmentComplete_DetectsFallback(t *testing.T) {
	// Create test DB with a camera that has storage_path set.
	d := newTestDB(t)
	cam := &db.Camera{
		Name:        "NAS Camera",
		RTSPURL:     "rtsp://x",
		StoragePath: "/mnt/nas1/recordings",
		MediaMTXPath: "nvr/test-cam-id/main",
	}
	require.NoError(t, d.CreateCamera(cam))

	// Simulate a recording that landed in local fallback (./recordings/...)
	// even though the camera is configured for /mnt/nas1/recordings.
	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/nvr/test-cam-id/main/2026-03/25/10-00-00-000000.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	// Call the detection function.
	detectAndInsertPendingSync(d, rec, cam)

	pending, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, rec.FilePath, pending[0].LocalPath)
	assert.Contains(t, pending[0].TargetPath, "/mnt/nas1/recordings/")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ -run TestOnSegmentComplete_DetectsFallback -v`
Expected: FAIL

- [ ] **Step 3: Implement pending sync detection**

Add helper function in `internal/nvr/nvr.go`:

```go
// detectAndInsertPendingSync checks if a recording landed in local fallback
// storage instead of the camera's configured storage path, and if so inserts
// a pending_syncs record.
func detectAndInsertPendingSync(database *db.DB, rec *db.Recording, cam *db.Camera) {
	if cam.StoragePath == "" {
		return // No custom storage, nothing to sync.
	}

	// If the file is already under the camera's storage path, no sync needed.
	if strings.HasPrefix(rec.FilePath, cam.StoragePath) {
		return
	}

	// Build target path by replacing the local prefix with the NAS path.
	// Local: ./recordings/nvr/<id>/main/2026-03/25/file.mp4
	// Target: /mnt/nas1/recordings/nvr/<id>/main/2026-03/25/file.mp4
	relPath := rec.FilePath
	if idx := strings.Index(relPath, "nvr/"); idx >= 0 {
		relPath = relPath[idx:]
	}
	targetPath := filepath.Join(cam.StoragePath, relPath)

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   rec.FilePath,
		TargetPath:  targetPath,
	}
	if err := database.InsertPendingSync(ps); err != nil {
		log.Printf("[NVR] [storage] failed to create pending sync for recording %d: %v", rec.ID, err)
	}
}
```

Call it from `OnSegmentComplete` after successful recording insert:

```go
// After: if insertErr != nil { ... return }
detectAndInsertPendingSync(n.database, rec, cam)
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ -run TestOnSegmentComplete_DetectsFallback -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/nvr.go internal/nvr/nvr_test.go
git commit -m "feat(storage): auto-detect fallback recordings and queue for sync"
```

---

### Task 8: Wire StorageManager into NVR startup

**Files:**
- Modify: `internal/nvr/nvr.go:32-55` (add storageManager field)
- Modify: `internal/nvr/nvr.go:60-170` (initialize and start StorageManager)

- [ ] **Step 1: Add StorageManager to NVR struct**

In `internal/nvr/nvr.go`, add to the NVR struct:

```go
storageMgr *storage.Manager
```

Add import: `"github.com/bluenviron/mediamtx/internal/nvr/storage"`

- [ ] **Step 2: Initialize and start StorageManager in Initialize()**

After the scheduler is started (after line 111), add:

```go
n.storageMgr = storage.New(n.database, n.yamlWriter, n.RecordingsPath, n.APIAddress)
n.storageMgr.Start()
```

- [ ] **Step 3: Pass StorageManager to router config**

The StorageManager needs to be accessible by the API handlers for `storage_status` on camera responses and the new storage endpoints. Add it to `RouterConfig` and pass it when creating the router.

- [ ] **Step 4: Run existing tests to verify nothing broke**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(storage): wire StorageManager into NVR startup"
```

---

### Task 9: Storage API endpoints

**Files:**
- Create: `internal/nvr/api/storage.go`
- Modify: `internal/nvr/api/router.go` (register routes)
- Create: `internal/nvr/api/storage_test.go`

- [ ] **Step 1: Write failing tests for storage endpoints**

Create `internal/nvr/api/storage_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/storage"
)

func newTestDBForStorage(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestStorageStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	d := newTestDBForStorage(t)
	mgr := storage.New(d, nil, "./recordings", ":9997")

	h := &StorageHandler{DB: d, Manager: mgr}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/storage/status", nil)
	h.Status(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestStoragePending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	d := newTestDBForStorage(t)
	mgr := storage.New(d, nil, "./recordings", ":9997")

	h := &StorageHandler{DB: d, Manager: mgr}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/storage/pending", nil)
	h.Pending(c)

	require.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestStorage -v`
Expected: FAIL

- [ ] **Step 3: Implement storage handler**

Create `internal/nvr/api/storage.go`:

```go
package api

import (
	"net/http"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
)

// StorageHandler handles storage management API endpoints.
type StorageHandler struct {
	DB      *db.DB
	Manager *storage.Manager
}

type storagePathStatus struct {
	Path        string `json:"path"`
	Healthy     bool   `json:"healthy"`
	CameraCount int    `json:"camera_count"`
	TotalBytes  uint64 `json:"total_bytes"`
	UsedBytes   uint64 `json:"used_bytes"`
	FreeBytes   uint64 `json:"free_bytes"`
}

// Status returns the health status of all configured storage paths.
func (h *StorageHandler) Status(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}

	// Group cameras by storage path and count.
	pathCounts := make(map[string]int)
	for _, cam := range cameras {
		if cam.StoragePath != "" {
			pathCounts[cam.StoragePath]++
		}
	}

	healthMap := h.Manager.GetAllHealth()
	var result []storagePathStatus

	for path, count := range pathCounts {
		status := storagePathStatus{
			Path:        path,
			Healthy:     healthMap[path],
			CameraCount: count,
		}

		// Get disk usage.
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err == nil {
			status.TotalBytes = stat.Blocks * uint64(stat.Bsize)
			status.FreeBytes = stat.Bavail * uint64(stat.Bsize)
			status.UsedBytes = status.TotalBytes - status.FreeBytes
		}

		result = append(result, status)
	}

	c.JSON(http.StatusOK, gin.H{"paths": result})
}

// Pending returns the pending sync queue.
func (h *StorageHandler) Pending(c *gin.Context) {
	counts, err := h.DB.PendingSyncCountByCamera()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get pending counts", err)
		return
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"by_camera": counts,
	})
}

// TriggerSync manually triggers sync for a specific camera's pending files.
func (h *StorageHandler) TriggerSync(c *gin.Context) {
	cameraID := c.Param("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	h.Manager.TriggerSyncForCamera(cameraID)

	c.JSON(http.StatusOK, gin.H{"message": "sync triggered"})
}
```

Add `TriggerSyncForCamera` to `internal/nvr/storage/manager.go`:

```go
// TriggerSyncForCamera immediately processes pending syncs for a specific camera.
func (m *Manager) TriggerSyncForCamera(cameraID string) {
	pending, err := m.db.ListPendingSyncsByCamera(cameraID)
	if err != nil {
		log.Printf("[NVR] [storage] failed to list pending syncs for camera %s: %v", cameraID, err)
		return
	}
	for _, ps := range pending {
		m.processSync(ps)
	}
}
```

Add `ListPendingSyncsByCamera` to `internal/nvr/db/pending_syncs.go`:

```go
// ListPendingSyncsByCamera returns pending syncs for a specific camera.
func (d *DB) ListPendingSyncsByCamera(cameraID string) ([]*PendingSync, error) {
	rows, err := d.Query(`
		SELECT id, recording_id, camera_id, local_path, target_path, status,
			attempts, COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs WHERE camera_id = ? AND status = 'pending' ORDER BY created_at ASC`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PendingSync
	for rows.Next() {
		ps := &PendingSync{}
		if err := rows.Scan(&ps.ID, &ps.RecordingID, &ps.CameraID,
			&ps.LocalPath, &ps.TargetPath, &ps.Status, &ps.Attempts,
			&ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt); err != nil {
			return nil, err
		}
		result = append(result, ps)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Register routes in router.go**

In `internal/nvr/api/router.go`, add to the protected routes section:

```go
storageHandler := &StorageHandler{DB: cfg.DB, Manager: cfg.StorageManager}
protected.GET("/storage/status", storageHandler.Status)
protected.GET("/storage/pending", storageHandler.Pending)
protected.POST("/storage/sync/:camera_id", storageHandler.TriggerSync)
```

Add `StorageManager *storage.Manager` to `RouterConfig` struct.

- [ ] **Step 5: Run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestStorage -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/storage.go internal/nvr/api/storage_test.go internal/nvr/api/router.go internal/nvr/storage/manager.go internal/nvr/db/pending_syncs.go
git commit -m "feat(storage): add storage status/pending/sync API endpoints"
```

---

### Task 10: Update HLS serving to use recording ID

**Files:**
- Modify: `internal/nvr/api/hls.go:360-424` (ServeSegment and segmentURLFromFilePath)

- [ ] **Step 1: Write failing test for recording-ID-based serving**

Add to existing HLS test file or create `internal/nvr/api/hls_serve_test.go`:

```go
func TestServeSegmentByRecordingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := newTestDB(t)

	// Create a recording with a known file.
	cam := &db.Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))

	tmpFile := filepath.Join(t.TempDir(), "test.mp4")
	require.NoError(t, os.WriteFile(tmpFile, []byte("fake"), 0o644))

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  tmpFile,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	h := &HLSHandler{DB: d}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/vod/segments/"+strconv.FormatInt(rec.ID, 10), nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(rec.ID, 10)}}
	h.ServeSegment(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestServeSegmentByRecordingID -v`
Expected: FAIL

- [ ] **Step 3: Rewrite ServeSegment to use recording ID lookup**

In `internal/nvr/api/hls.go`, replace `ServeSegment`:

```go
// ServeSegment serves a recording file by its database ID.
// Supports HTTP Range requests for byte-range access.
//
// GET /vod/segments/:id
func (h *HLSHandler) ServeSegment(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording id is required"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
		return
	}

	rec, err := h.DB.GetRecording(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}

	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment file not found"})
		return
	}

	http.ServeFile(c.Writer, c.Request, rec.FilePath)
}
```

Replace `segmentURLFromFilePath` with an ID-based version:

```go
func segmentURLFromRecordingID(recordingID int64, token string) string {
	return fmt.Sprintf("/api/nvr/vod/segments/%d?jwt=%s", recordingID, token)
}
```

Update all callers in `hls.go`. The main call site is in the playlist builder where `segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)` is called — change to `segmentURLFromRecordingID(rec.ID, token)`. Search for all usages of `segmentURLFromFilePath` and replace.

Update route registration in `internal/nvr/api/router.go` from:
```go
nvr.GET("/vod/segments/*filepath", cfg.HLSHandler.ServeSegment)
```
to:
```go
nvr.GET("/vod/segments/:id", cfg.HLSHandler.ServeSegment)
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestServeSegment -v`
Expected: PASS

- [ ] **Step 5: Run all HLS tests to verify nothing else broke**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/hls.go internal/nvr/api/hls_serve_test.go internal/nvr/api/router.go
git commit -m "feat(storage): serve recording segments by DB ID instead of file path"
```

---

### Task 11: Add `storage_status` to camera API responses

**Files:**
- Modify: `internal/nvr/api/cameras.go` (Get, List handlers — inject storage_status)

- [ ] **Step 1: Add StorageManager reference to CameraHandler**

In `internal/nvr/api/cameras.go`, add to `CameraHandler` struct:

```go
StorageMgr *storage.Manager
```

- [ ] **Step 2: Create a response helper that injects storage_status**

```go
type cameraResponse struct {
	db.Camera
	StorageStatus string `json:"storage_status"`
}

func (h *CameraHandler) buildCameraResponse(cam *db.Camera) cameraResponse {
	status := "default"
	if h.StorageMgr != nil {
		status = h.StorageMgr.StorageStatus(cam)
	}
	return cameraResponse{Camera: *cam, StorageStatus: status}
}
```

- [ ] **Step 3: Update Get and List handlers to use the response helper**

In the Get handler, change `c.JSON(http.StatusOK, cam)` to `c.JSON(http.StatusOK, h.buildCameraResponse(cam))`.

In the List handler, build a `[]cameraResponse` slice and return that instead of the raw camera list.

- [ ] **Step 4: Run existing camera API tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCamera -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(storage): add storage_status to camera API responses"
```

---

### Task 12: Flutter — Add storage_path to Camera model

**Files:**
- Modify: `clients/flutter/lib/models/camera.dart`

- [ ] **Step 1: Add storage fields to Camera model**

In `clients/flutter/lib/models/camera.dart`, add to the Camera factory:

```dart
@JsonKey(name: 'storage_path') @Default('') String storagePath,
@JsonKey(name: 'storage_status') @Default('default') String storageStatus,
```

- [ ] **Step 2: Run Flutter code generation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart run build_runner build --delete-conflicting-outputs`
Expected: Generates updated `camera.freezed.dart` and `camera.g.dart`

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/models/camera.dart clients/flutter/lib/models/camera.freezed.dart clients/flutter/lib/models/camera.g.dart
git commit -m "feat(storage): add storage_path and storage_status to Flutter camera model"
```

---

### Task 13: Flutter — Storage tab on Camera Detail Screen

**Files:**
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart` (add 6th tab)
- Create: `clients/flutter/lib/screens/cameras/tabs/storage_tab.dart`

- [ ] **Step 1: Create the StorageTab widget**

Create `clients/flutter/lib/screens/cameras/tabs/storage_tab.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../models/camera.dart';
import '../../../services/api_service.dart';

class StorageTab extends ConsumerStatefulWidget {
  final Camera camera;
  const StorageTab({super.key, required this.camera});

  @override
  ConsumerState<StorageTab> createState() => _StorageTabState();
}

class _StorageTabState extends ConsumerState<StorageTab> {
  late TextEditingController _storagePathController;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _storagePathController = TextEditingController(text: widget.camera.storagePath);
  }

  @override
  void dispose() {
    _storagePathController.dispose();
    super.dispose();
  }

  Color _statusColor(String status) {
    switch (status) {
      case 'healthy':
        return Colors.green;
      case 'degraded':
        return Colors.amber;
      default:
        return Colors.grey;
    }
  }

  String _statusLabel(String status) {
    switch (status) {
      case 'healthy':
        return 'Healthy';
      case 'degraded':
        return 'Degraded';
      default:
        return 'Default';
    }
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      final api = ref.read(apiServiceProvider);
      await api.updateCamera(widget.camera.id, {
        'name': widget.camera.name,
        'rtsp_url': widget.camera.rtspUrl,
        'storage_path': _storagePathController.text.trim(),
      });
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Storage path updated')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to update: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Storage status chip.
        Row(
          children: [
            const Text('Status: ', style: TextStyle(fontWeight: FontWeight.bold)),
            Chip(
              label: Text(_statusLabel(widget.camera.storageStatus)),
              backgroundColor: _statusColor(widget.camera.storageStatus).withOpacity(0.2),
              side: BorderSide(color: _statusColor(widget.camera.storageStatus)),
            ),
          ],
        ),
        const SizedBox(height: 16),

        // Storage path field.
        TextField(
          controller: _storagePathController,
          decoration: const InputDecoration(
            labelText: 'Storage Path',
            hintText: 'Leave empty to use default local storage',
            helperText: 'Absolute path to recording storage (e.g., /mnt/nas/recordings)',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 16),

        // Save button.
        ElevatedButton(
          onPressed: _saving ? null : _save,
          child: _saving
              ? const SizedBox(height: 20, width: 20, child: CircularProgressIndicator(strokeWidth: 2))
              : const Text('Save'),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Add Storage tab to CameraDetailScreen**

In `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`:

1. Change `TabController` length from 5 to 6
2. Add `const Tab(text: 'Storage')` to the TabBar tabs list
3. Add `StorageTab(camera: camera)` to the TabBarView children list
4. Import the new tab file

- [ ] **Step 3: Run Flutter build to verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter build apk --debug 2>&1 | tail -5`
Expected: BUILD SUCCESSFUL

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/cameras/tabs/storage_tab.dart clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(storage): add Storage tab to Flutter camera detail screen"
```

---

### Task 14: Migration — Rename existing camera MediaMTX paths

**Files:**
- Modify: `internal/nvr/nvr.go` (add migration function called during Initialize)

- [ ] **Step 1: Write migration function**

Add to `internal/nvr/nvr.go`:

```go
// migrateMediaMTXPaths updates camera MediaMTX paths from the old naming
// convention (nvr/<sanitized-name>) to the new convention (nvr/<camera-id>/main).
// It also verifies that every camera's MediaMTX path exists in the YAML config.
func (n *NVR) migrateMediaMTXPaths() {
	cameras, err := n.database.ListCameras()
	if err != nil {
		log.Printf("[NVR] [migration] failed to list cameras: %v", err)
		return
	}

	for _, cam := range cameras {
		expectedPath := "nvr/" + cam.ID + "/main"
		if cam.MediaMTXPath == expectedPath {
			continue // Already migrated.
		}

		oldPath := cam.MediaMTXPath
		cam.MediaMTXPath = expectedPath

		// Update DB.
		if err := n.database.UpdateCamera(cam); err != nil {
			log.Printf("[NVR] [migration] failed to update path for camera %s: %v", cam.ID, err)
			continue
		}

		// Rename in YAML: add new path with same config, remove old path.
		yamlConfig := map[string]interface{}{
			"source": cam.RTSPURL,
			"record": true,
		}
		storagePath := cam.StoragePath
		if storagePath == "" {
			storagePath = n.RecordingsPath
		}
		yamlConfig["recordPath"] = storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"

		if err := n.yamlWriter.AddPath(expectedPath, yamlConfig); err != nil {
			log.Printf("[NVR] [migration] failed to add new path for camera %s: %v", cam.ID, err)
			continue
		}

		if oldPath != "" {
			_ = n.yamlWriter.RemovePath(oldPath)
		}

		log.Printf("[NVR] [migration] migrated camera %s path: %s -> %s", cam.ID, oldPath, expectedPath)
	}
}
```

- [ ] **Step 2: Call migration from Initialize()**

In `Initialize()`, call `n.migrateMediaMTXPaths()` after the database is opened and the YAML writer is created (after line 103), before the scheduler starts.

- [ ] **Step 3: Run existing tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(storage): migrate existing camera paths to nvr/<id>/main convention"
```

---

### Task 15: Integration test — End-to-end storage failover

**Files:**
- Create: `internal/nvr/storage/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `internal/nvr/storage/integration_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FailoverAndRecovery(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()
	nasDir := t.TempDir()

	// Create camera with NAS storage path.
	cam := &db.Camera{
		Name:         "NAS Camera",
		RTSPURL:      "rtsp://x",
		StoragePath:  nasDir,
		MediaMTXPath: "nvr/test-cam/main",
	}
	require.NoError(t, database.CreateCamera(cam))

	m := New(database, nil, localDir, ":9997")
	m.checkInterval = 100 * time.Millisecond

	// Initial health check — NAS is healthy.
	m.runHealthCheck()
	assert.True(t, m.GetHealth(nasDir))
	assert.Equal(t, "healthy", m.StorageStatus(cam))

	// Simulate NAS going offline.
	os.RemoveAll(nasDir)
	m.runHealthCheck()
	assert.False(t, m.GetHealth(nasDir))
	assert.Equal(t, "degraded", m.StorageStatus(cam))

	// Simulate a fallback recording.
	os.MkdirAll(nasDir, 0o755) // Restore for sync test.
	fallbackFile := filepath.Join(localDir, "nvr/test-cam/main/2026-03/25/10-00-00-000000.mp4")
	os.MkdirAll(filepath.Dir(fallbackFile), 0o755)
	os.WriteFile(fallbackFile, []byte("recording data"), 0o644)

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  fallbackFile,
		Format:    "fmp4",
		FileSize:  14,
	}
	require.NoError(t, database.InsertRecording(rec))

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   fallbackFile,
		TargetPath:  filepath.Join(nasDir, "nvr/test-cam/main/2026-03/25/10-00-00-000000.mp4"),
	}
	require.NoError(t, database.InsertPendingSync(ps))

	// NAS comes back online — run sync.
	m.runHealthCheck()
	assert.True(t, m.GetHealth(nasDir))

	m.runSyncPass()

	// Verify: file synced to NAS.
	_, err := os.Stat(ps.TargetPath)
	require.NoError(t, err)

	// Verify: local file deleted.
	_, err = os.Stat(fallbackFile)
	assert.True(t, os.IsNotExist(err))

	// Verify: recording path updated in DB.
	updated, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, ps.TargetPath, updated.FilePath)

	// Verify: pending sync cleaned up.
	pending, err := database.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}
```

- [ ] **Step 2: Run integration test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/storage/ -run TestIntegration -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/storage/integration_test.go
git commit -m "test(storage): add integration test for failover and recovery"
```

---

### Task 16: Run full test suite and fix any issues

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: All PASS

- [ ] **Step 2: Run Go vet**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go vet ./internal/nvr/...`
Expected: No issues

- [ ] **Step 3: Run Flutter analysis**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze`
Expected: No issues

- [ ] **Step 4: Fix any issues found and commit**

```bash
git add -A
git commit -m "fix: address test and lint issues from per-camera storage implementation"
```
