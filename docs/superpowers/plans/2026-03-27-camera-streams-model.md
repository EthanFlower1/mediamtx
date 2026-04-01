# Camera Multi-Stream Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `camera_streams` table so cameras can own multiple streams with configurable roles, and update the backend to resolve streams by role for recording, live view, and AI detection.

**Architecture:** New DB migration adds `camera_streams` table and `stream_id` column to `recording_rules`. A `CameraStream` CRUD layer provides stream management. The camera creation flow auto-populates streams from ONVIF profiles. The YAML writer, scheduler, and AI pipeline resolve stream URLs by role instead of reading the legacy `rtsp_url`/`sub_stream_url` fields. A data migration converts existing camera data to stream records.

**Tech Stack:** Go, SQLite, gin HTTP framework

---

### Task 1: DB migration — Create `camera_streams` table and add `stream_id` to `recording_rules`

**Files:**

- Modify: `internal/nvr/db/migrations.go`

- [ ] **Step 1: Add migration version 21**

In `internal/nvr/db/migrations.go`, add a new migration entry at the end of the `migrations` slice:

```go
{
    version: 21,
    sql: `
CREATE TABLE camera_streams (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    rtsp_url TEXT NOT NULL,
    profile_token TEXT NOT NULL DEFAULT '',
    video_codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    roles TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_camera_streams_camera ON camera_streams(camera_id);

ALTER TABLE recording_rules ADD COLUMN stream_id TEXT NOT NULL DEFAULT '';

-- Migrate existing cameras: create stream records from rtsp_url
INSERT INTO camera_streams (id, camera_id, name, rtsp_url, roles, created_at)
SELECT
    lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6))),
    id,
    'Main Stream',
    rtsp_url,
    CASE
        WHEN sub_stream_url IS NOT NULL AND sub_stream_url != '' THEN 'live_view'
        ELSE 'live_view,recording,ai_detection,mobile'
    END,
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM cameras
WHERE rtsp_url IS NOT NULL AND rtsp_url != '';

-- Migrate sub_stream_url where present
INSERT INTO camera_streams (id, camera_id, name, rtsp_url, roles, created_at)
SELECT
    lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)),2) || '-' || hex(randomblob(6))),
    id,
    'Sub Stream',
    sub_stream_url,
    'recording,ai_detection,mobile',
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM cameras
WHERE sub_stream_url IS NOT NULL AND sub_stream_url != '';
`,
},
```

- [ ] **Step 2: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/db/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/migrations.go
git commit -m "feat(db): add camera_streams table and stream_id to recording_rules"
```

---

### Task 2: DB layer — CameraStream struct and CRUD operations

**Files:**

- Create: `internal/nvr/db/camera_streams.go`

- [ ] **Step 1: Create the CameraStream CRUD file**

Create `internal/nvr/db/camera_streams.go`:

```go
package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StreamRole constants define the predefined roles a stream can have.
const (
	StreamRoleLiveView    = "live_view"
	StreamRoleRecording   = "recording"
	StreamRoleMobile      = "mobile"
	StreamRoleAIDetection = "ai_detection"
)

// CameraStream represents a media stream belonging to a camera.
type CameraStream struct {
	ID           string `json:"id"`
	CameraID     string `json:"camera_id"`
	Name         string `json:"name"`
	RTSPURL      string `json:"rtsp_url"`
	ProfileToken string `json:"profile_token"`
	VideoCodec   string `json:"video_codec"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Roles        string `json:"roles"`
	CreatedAt    string `json:"created_at"`
}

// HasRole returns true if the stream has the given role.
func (s *CameraStream) HasRole(role string) bool {
	for _, r := range strings.Split(s.Roles, ",") {
		if strings.TrimSpace(r) == role {
			return true
		}
	}
	return false
}

// RoleList returns the roles as a slice.
func (s *CameraStream) RoleList() []string {
	if s.Roles == "" {
		return nil
	}
	parts := strings.Split(s.Roles, ",")
	out := make([]string, 0, len(parts))
	for _, r := range parts {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// CreateCameraStream inserts a new stream record.
func (d *DB) CreateCameraStream(s *CameraStream) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format(timeFormat)
	}

	_, err := d.Exec(`
		INSERT INTO camera_streams (id, camera_id, name, rtsp_url, profile_token,
			video_codec, width, height, roles, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.CameraID, s.Name, s.RTSPURL, s.ProfileToken,
		s.VideoCodec, s.Width, s.Height, s.Roles, s.CreatedAt,
	)
	return err
}

// ListCameraStreams returns all streams for a camera, ordered by width descending
// (highest resolution first).
func (d *DB) ListCameraStreams(cameraID string) ([]*CameraStream, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams WHERE camera_id = ?
		ORDER BY (width * height) DESC`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var streams []*CameraStream
	for rows.Next() {
		s := &CameraStream{}
		if err := rows.Scan(
			&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
			&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		streams = append(streams, s)
	}
	return streams, rows.Err()
}

// GetCameraStream retrieves a single stream by ID.
func (d *DB) GetCameraStream(id string) (*CameraStream, error) {
	s := &CameraStream{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
		&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

// UpdateCameraStream updates a stream's mutable fields.
func (d *DB) UpdateCameraStream(s *CameraStream) error {
	res, err := d.Exec(`
		UPDATE camera_streams SET name = ?, rtsp_url = ?, profile_token = ?,
			video_codec = ?, width = ?, height = ?, roles = ?
		WHERE id = ?`,
		s.Name, s.RTSPURL, s.ProfileToken,
		s.VideoCodec, s.Width, s.Height, s.Roles, s.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCameraStream deletes a stream by ID.
func (d *DB) DeleteCameraStream(id string) error {
	res, err := d.Exec("DELETE FROM camera_streams WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveStreamURL finds the RTSP URL for a camera by role. It searches the
// camera's streams for one with the matching role. If not found, falls back to
// the camera's legacy rtsp_url field.
func (d *DB) ResolveStreamURL(cameraID, role string) (string, error) {
	var url string
	err := d.QueryRow(`
		SELECT rtsp_url FROM camera_streams
		WHERE camera_id = ? AND (',' || roles || ',') LIKE '%,' || ? || ',%'
		ORDER BY (width * height) DESC
		LIMIT 1`, cameraID, role,
	).Scan(&url)
	if err == nil && url != "" {
		return url, nil
	}

	// Fall back to legacy field.
	err = d.QueryRow("SELECT rtsp_url FROM cameras WHERE id = ?", cameraID).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return url, err
}

// ResolveStream finds a stream for a camera by role. Returns nil if no match.
func (d *DB) ResolveStream(cameraID, role string) (*CameraStream, error) {
	s := &CameraStream{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, rtsp_url, profile_token, video_codec,
			width, height, roles, created_at
		FROM camera_streams
		WHERE camera_id = ? AND (',' || roles || ',') LIKE '%,' || ? || ',%'
		ORDER BY (width * height) DESC
		LIMIT 1`, cameraID, role,
	).Scan(
		&s.ID, &s.CameraID, &s.Name, &s.RTSPURL, &s.ProfileToken,
		&s.VideoCodec, &s.Width, &s.Height, &s.Roles, &s.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}
```

- [ ] **Step 2: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/db/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/camera_streams.go
git commit -m "feat(db): add CameraStream CRUD and role resolution"
```

---

### Task 3: DB layer — Add `StreamID` to `RecordingRule`

**Files:**

- Modify: `internal/nvr/db/recording_rules.go`

- [ ] **Step 1: Add StreamID field to struct and all queries**

Add `StreamID string` field to the `RecordingRule` struct:

```go
type RecordingRule struct {
	ID               string `json:"id"`
	CameraID         string `json:"camera_id"`
	StreamID         string `json:"stream_id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Days             string `json:"days"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	PostEventSeconds int    `json:"post_event_seconds"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}
```

Update all SQL queries in the file to include `stream_id`:

**`CreateRecordingRule`** — add `stream_id` to INSERT:

```go
_, err := d.Exec(`
    INSERT INTO recording_rules (id, camera_id, stream_id, name, mode, days, start_time,
        end_time, post_event_seconds, enabled, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    rule.ID, rule.CameraID, rule.StreamID, rule.Name, rule.Mode, rule.Days, rule.StartTime,
    rule.EndTime, rule.PostEventSeconds, rule.Enabled, rule.CreatedAt, rule.UpdatedAt,
)
```

**All SELECT queries** (`GetRecordingRule`, `ListRecordingRules`, `ListAllEnabledRecordingRules`) — add `stream_id` to the SELECT column list and Scan call. It goes after `camera_id` in both.

**`UpdateRecordingRule`** — add `stream_id = ?` to the UPDATE SET clause:

```go
res, err := d.Exec(`
    UPDATE recording_rules SET camera_id = ?, stream_id = ?, name = ?, mode = ?, days = ?,
        start_time = ?, end_time = ?, post_event_seconds = ?, enabled = ?,
        updated_at = ?
    WHERE id = ?`,
    rule.CameraID, rule.StreamID, rule.Name, rule.Mode, rule.Days, rule.StartTime,
    rule.EndTime, rule.PostEventSeconds, rule.Enabled, rule.UpdatedAt, rule.ID,
)
```

- [ ] **Step 2: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/recording_rules.go
git commit -m "feat(db): add stream_id to RecordingRule"
```

---

### Task 4: API — Stream CRUD endpoints

**Files:**

- Create: `internal/nvr/api/streams.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create the streams handler**

Create `internal/nvr/api/streams.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// StreamHandler handles CRUD operations for camera streams.
type StreamHandler struct {
	DB *db.DB
}

type streamRequest struct {
	Name         string `json:"name" binding:"required"`
	RTSPURL      string `json:"rtsp_url" binding:"required"`
	ProfileToken string `json:"profile_token"`
	VideoCodec   string `json:"video_codec"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Roles        string `json:"roles"`
}

// List returns all streams for a camera.
func (h *StreamHandler) List(c *gin.Context) {
	cameraID := c.Param("id")
	streams, err := h.DB.ListCameraStreams(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list streams", err)
		return
	}
	if streams == nil {
		streams = []*db.CameraStream{}
	}
	c.JSON(http.StatusOK, streams)
}

// Create adds a new stream to a camera.
func (h *StreamHandler) Create(c *gin.Context) {
	cameraID := c.Param("id")

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify camera exists.
	if _, err := h.DB.GetCamera(cameraID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	stream := &db.CameraStream{
		CameraID:     cameraID,
		Name:         req.Name,
		RTSPURL:      req.RTSPURL,
		ProfileToken: req.ProfileToken,
		VideoCodec:   req.VideoCodec,
		Width:        req.Width,
		Height:       req.Height,
		Roles:        req.Roles,
	}

	if err := h.DB.CreateCameraStream(stream); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create stream", err)
		return
	}

	c.JSON(http.StatusCreated, stream)
}

// Update modifies an existing stream.
func (h *StreamHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetCameraStream(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing.Name = req.Name
	existing.RTSPURL = req.RTSPURL
	existing.ProfileToken = req.ProfileToken
	existing.VideoCodec = req.VideoCodec
	existing.Width = req.Width
	existing.Height = req.Height
	existing.Roles = req.Roles

	if err := h.DB.UpdateCameraStream(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update stream", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a stream.
func (h *StreamHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.DeleteCameraStream(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
```

- [ ] **Step 2: Register routes in router.go**

In `internal/nvr/api/router.go`, in the `RegisterRoutes` function, add stream handler initialization and routes. After the existing camera routes block (around line 172):

Add handler initialization after the other handlers (around line 110):

```go
streamHandler := &StreamHandler{DB: cfg.DB}
```

Add routes in the protected group after camera routes:

```go
// Camera streams.
protected.GET("/cameras/:id/streams", streamHandler.List)
protected.POST("/cameras/:id/streams", streamHandler.Create)
protected.PUT("/streams/:id", streamHandler.Update)
protected.DELETE("/streams/:id", streamHandler.Delete)
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/streams.go internal/nvr/api/router.go
git commit -m "feat(api): add camera stream CRUD endpoints"
```

---

### Task 5: API — Include streams in camera responses and auto-populate on create

**Files:**

- Modify: `internal/nvr/api/cameras.go`

- [ ] **Step 1: Add streams to camera list and get responses**

In `internal/nvr/api/cameras.go`, find the `List` handler (the one that calls `h.DB.ListCameras()`). After fetching cameras, fetch streams for each and attach them. The simplest approach: add a `Streams` field to the JSON response.

Find the `List` method and after the `cameras` are fetched from DB, add stream loading. Wrap each camera in a response struct that includes streams:

Create a response type near the top of cameras.go:

```go
type cameraWithStreams struct {
	*db.Camera
	Streams []*db.CameraStream `json:"streams"`
}
```

In the `List` handler, after `cameras, err := h.DB.ListCameras()`, build the response:

```go
result := make([]cameraWithStreams, len(cameras))
for i, cam := range cameras {
    streams, _ := h.DB.ListCameraStreams(cam.ID)
    if streams == nil {
        streams = []*db.CameraStream{}
    }
    result[i] = cameraWithStreams{Camera: cam, Streams: streams}
}
c.JSON(http.StatusOK, result)
```

Do the same in the `Get` handler — wrap the single camera response with its streams.

- [ ] **Step 2: Auto-populate streams on camera creation**

In the `Create` handler, after the camera is inserted into the DB, check if the request included profile data. The existing `cameraRequest` struct has fields from ONVIF discovery. Add stream creation logic after the camera insert.

Add a `Profiles` field to `cameraRequest` if not already present:

```go
type cameraRequest struct {
    // ... existing fields ...
    Profiles []streamRequest `json:"profiles"`
}
```

After `h.DB.CreateCamera(cam)` succeeds, if profiles were provided, create streams with auto-assigned roles:

```go
if len(req.Profiles) > 0 {
    // Sort by resolution descending (profiles typically arrive highest-first from ONVIF).
    for i, p := range req.Profiles {
        roles := ""
        if i == 0 {
            // Highest resolution → live_view
            roles = "live_view"
            if len(req.Profiles) == 1 {
                roles = "live_view,recording,ai_detection,mobile"
            }
        } else if i == len(req.Profiles)-1 {
            // Lowest resolution → recording, ai, mobile
            roles = "recording,ai_detection,mobile"
        }

        stream := &db.CameraStream{
            CameraID:     cam.ID,
            Name:         p.Name,
            RTSPURL:      p.RTSPURL,
            ProfileToken: p.ProfileToken,
            VideoCodec:   p.VideoCodec,
            Width:        p.Width,
            Height:       p.Height,
            Roles:        roles,
        }
        _ = h.DB.CreateCameraStream(stream)
    }
}
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(api): include streams in camera responses, auto-populate on create"
```

---

### Task 6: Update recording rules API to accept `stream_id`

**Files:**

- Modify: `internal/nvr/api/recording_rules.go`

- [ ] **Step 1: Add `StreamID` to the request struct and handlers**

In `internal/nvr/api/recording_rules.go`, add `StreamID` to `recordingRuleRequest`:

```go
type recordingRuleRequest struct {
    Name             string `json:"name" binding:"required"`
    Mode             string `json:"mode" binding:"required"`
    StreamID         string `json:"stream_id"`
    Days             []int  `json:"days" binding:"required"`
    // ... rest unchanged
}
```

In the `Create` handler, set `rule.StreamID = req.StreamID` when building the `RecordingRule` struct.

In the `Update` handler, set `rule.StreamID = req.StreamID` when updating.

- [ ] **Step 2: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/recording_rules.go
git commit -m "feat(api): accept stream_id in recording rule create/update"
```

---

### Task 7: Update YAML writer to use stream URLs

**Files:**

- Modify: `internal/nvr/nvr.go` (camera path creation and migration)

- [ ] **Step 1: Update `ensureCameraPaths` to use stream URLs**

In `internal/nvr/nvr.go`, find the `ensureCameraPaths` method where it builds `yamlConfig` with `cam.RTSPURL` (around line 380). Change it to resolve the recording-role stream URL:

```go
// Resolve recording stream URL (prefer camera_streams, fall back to legacy rtsp_url).
sourceURL, err := n.database.ResolveStreamURL(cam.ID, db.StreamRoleRecording)
if err != nil || sourceURL == "" {
    sourceURL = cam.RTSPURL
}

yamlConfig := map[string]interface{}{
    "source": sourceURL,
    "record": true,
}
```

- [ ] **Step 2: Update `startAIPipelines` to resolve AI detection stream**

In the `startAIPipelines` method (around line 406), after checking `cam.AIEnabled`, resolve the `ai_detection` stream to extract credentials for the snapshot URL. The AI pipeline currently uses `cam.SnapshotURI` with ONVIF credentials. The stream resolution doesn't change the snapshot URI logic directly, but if we want to use stream credentials:

No change needed here — the AI pipeline uses ONVIF snapshot URIs, not RTSP streams. The `ai_detection` role is for future use when the pipeline switches to reading from an RTSP sub-stream. Leave this as-is for now.

- [ ] **Step 3: Verify Go compiles and tests pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(nvr): resolve recording stream URL from camera_streams"
```

---

### Task 8: Build and verify end-to-end

**Files:** None (verification only)

- [ ] **Step 1: Build the full binary**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build -o mediamtx .`
Expected: Build succeeds

- [ ] **Step 2: Run existing Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/db/... ./internal/nvr/api/... -v -count=1 2>&1 | tail -30`
Expected: Tests pass (some may need minor updates for the new `stream_id` field in recording rules — if so, fix them)

- [ ] **Step 3: Verify migration runs on existing DB**

Run: Start the server briefly to trigger migration: `./mediamtx &; sleep 5; kill %1`
Check logs for migration success and verify streams were created:

```bash
sqlite3 ~/.mediamtx/nvr.db "SELECT id, camera_id, name, roles FROM camera_streams;"
```

Expected: Stream records exist for each camera

- [ ] **Step 4: Test stream API endpoints**

```bash
TOKEN=$(curl -s -X POST http://localhost:9997/api/nvr/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")

# List streams for a camera
CAMERA_ID=$(sqlite3 ~/.mediamtx/nvr.db "SELECT id FROM cameras LIMIT 1;")
curl -s -H "Authorization: Bearer $TOKEN" "http://localhost:9997/api/nvr/cameras/$CAMERA_ID/streams" | python3 -m json.tool
```

Expected: JSON array of stream objects with roles

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: address build/test issues from camera streams implementation"
```
