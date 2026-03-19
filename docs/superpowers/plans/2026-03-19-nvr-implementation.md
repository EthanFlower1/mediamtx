# MediaMTX NVR v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add NVR functionality (camera management, ONVIF, recording timeline, user auth, React UI) to MediaMTX as internal packages, shipping as a single binary.

**Architecture:** NVR is a subsystem initialized by Core when `nvr: yes` is set in config. It owns a SQLite database, a local JWKS endpoint, ONVIF integration, and an embedded React UI. Camera streaming config is written to `mediamtx.yml` and picked up via hot-reload. The NVR API is registered as a gin route group on the existing API server.

**Tech Stack:** Go 1.25, modernc.org/sqlite, kerberos-io/onvif, gin (existing), React 19 + Vite, golang-jwt/jwt/v5 (existing)

**Spec:** `docs/superpowers/specs/2026-03-19-nvr-design.md`

---

## File Structure

```
# New files
internal/nvr/nvr.go                    # NVR subsystem lifecycle (init, shutdown, config reload)
internal/nvr/db/db.go                  # SQLite connection, WAL mode, migration runner
internal/nvr/db/migrations.go          # Embedded SQL migrations
internal/nvr/db/cameras.go             # Camera CRUD queries
internal/nvr/db/recordings.go          # Recording metadata queries
internal/nvr/db/users.go               # User CRUD + password hashing
internal/nvr/db/tokens.go              # Refresh token queries
internal/nvr/db/config.go              # Config key-value store (RSA keys, setup state)
internal/nvr/db/db_test.go             # Database tests
internal/nvr/db/cameras_test.go        # Camera query tests
internal/nvr/db/recordings_test.go     # Recording query tests
internal/nvr/db/users_test.go          # User query tests
internal/nvr/db/tokens_test.go         # Token query tests
internal/nvr/onvif/discovery.go        # WS-Discovery network scanning
internal/nvr/onvif/device.go           # Device info, profile fetching
internal/nvr/onvif/media.go            # Media profile management
internal/nvr/onvif/imaging.go          # Imaging settings
internal/nvr/onvif/ptz.go              # PTZ control
internal/nvr/onvif/onvif_test.go       # ONVIF integration tests
internal/nvr/api/router.go             # Gin route group registration
internal/nvr/api/middleware.go         # JWT auth middleware
internal/nvr/api/auth.go               # Login/refresh/revoke/setup endpoints
internal/nvr/api/cameras.go            # Camera CRUD + discovery endpoints
internal/nvr/api/recordings.go         # Recording/timeline query endpoints
internal/nvr/api/users.go              # User management endpoints
internal/nvr/api/system.go             # System info, health, SSE events
internal/nvr/api/jwks.go               # JWKS endpoint serving public key
internal/nvr/api/middleware_test.go     # Middleware tests
internal/nvr/api/auth_test.go          # Auth endpoint tests
internal/nvr/api/cameras_test.go       # Camera endpoint tests
internal/nvr/api/recordings_test.go    # Recording endpoint tests
internal/nvr/api/users_test.go         # User endpoint tests
internal/nvr/api/ptz.go                # PTZ and camera settings endpoints
internal/nvr/api/storage.go            # Storage stats endpoint
internal/nvr/crypto/keys.go            # RSA key generation, HKDF derivation
internal/nvr/crypto/encrypt.go         # AES-256-GCM encryption for ONVIF passwords
internal/nvr/crypto/keys_test.go       # Crypto tests
internal/nvr/crypto/encrypt_test.go    # Encryption tests
internal/nvr/yamlwriter/writer.go      # Safe YAML read-modify-write with locking
internal/nvr/yamlwriter/writer_test.go # YAML writer tests
internal/nvr/ui/embed.go               # go:embed directive for React build
internal/nvr/ui/dist/.gitkeep          # Placeholder for build output
internal/nvr/nvr_test.go               # NVR subsystem tests
ui/                                    # React application (detailed in Task 12)

# Modified files
internal/conf/conf.go                  # Add NVR config fields
internal/core/core.go                  # Initialize NVR subsystem
internal/recordcleaner/cleaner.go      # Add OnSegmentDelete callback
internal/recorder/recorder.go          # Wire OnSegmentComplete to NVR
internal/api/api.go                    # Pass NVR router to gin
go.mod                                 # Add modernc.org/sqlite, kerberos-io/onvif
Makefile                               # Add nvr-ui target
scripts/binaries.mk                    # Include UI build step
mediamtx.yml                           # Add NVR config section
```

---

## Task 1: SQLite Database Foundation

**Files:**
- Create: `internal/nvr/db/db.go`
- Create: `internal/nvr/db/migrations.go`
- Create: `internal/nvr/db/db_test.go`
- Modify: `go.mod`

- [ ] **Step 1: Add modernc.org/sqlite dependency**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
go get modernc.org/sqlite
```

- [ ] **Step 2: Write failing test for database initialization**

Create `internal/nvr/db/db_test.go`:

```go
package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	require.NoError(t, err)
	require.NotNil(t, database)
	defer database.Close()

	// Verify file was created
	_, err = os.Stat(dbPath)
	require.NoError(t, err)
}

func TestOpenRunsMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	// Verify tables exist by querying them
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM cameras").Scan(&count)
	require.NoError(t, err)

	err = database.QueryRow("SELECT COUNT(*) FROM recordings").Scan(&count)
	require.NoError(t, err)

	err = database.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)

	err = database.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&count)
	require.NoError(t, err)

	err = database.QueryRow("SELECT COUNT(*) FROM config").Scan(&count)
	require.NoError(t, err)
}

func TestOpenWALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	var journalMode string
	err = database.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	require.Equal(t, "wal", journalMode)
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
go test ./internal/nvr/db/ -v -run TestOpen
```

Expected: FAIL — package does not exist

- [ ] **Step 4: Write migrations**

Create `internal/nvr/db/migrations.go`:

```go
package db

import "database/sql"

var migrations = []func(*sql.Tx) error{
	migration001,
}

func migration001(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS cameras (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			onvif_endpoint TEXT,
			onvif_username TEXT,
			onvif_password TEXT,
			onvif_profile_token TEXT,
			rtsp_url TEXT NOT NULL,
			ptz_capable INTEGER NOT NULL DEFAULT 0,
			mediamtx_path TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'offline',
			tags TEXT DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS recordings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
			start_time DATETIME NOT NULL,
			end_time DATETIME NOT NULL,
			duration_ms INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			format TEXT NOT NULL,
			FOREIGN KEY (camera_id) REFERENCES cameras(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recordings_timeline
			ON recordings(camera_id, start_time, end_time)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			camera_permissions TEXT NOT NULL DEFAULT '"*"',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			revoked_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
			ON refresh_tokens(user_id)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Write database Open function**

Create `internal/nvr/db/db.go`:

```go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB with NVR-specific helpers.
type DB struct {
	*sql.DB
}

// Open creates or opens a SQLite database at the given path,
// enables WAL mode, and runs any pending migrations.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent readers
	if _, err := sqlDB.Exec("PRAGMA journal_mode=wal"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	db := &DB{DB: sqlDB}

	if err := db.runMigrations(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) runMigrations() error {
	// Create migrations tracking table
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY
	)`); err != nil {
		return err
	}

	var currentVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return err
	}

	for i := currentVersion; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}

		if err := migrations[i](tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}

	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/nvr/db/ -v -run TestOpen
```

Expected: All 3 tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/db/db.go internal/nvr/db/migrations.go internal/nvr/db/db_test.go go.mod go.sum
git commit -m "feat(nvr): add SQLite database foundation with migrations"
```

---

## Task 2: Camera CRUD Queries

**Files:**
- Create: `internal/nvr/db/cameras.go`
- Create: `internal/nvr/db/cameras_test.go`

- [ ] **Step 1: Write failing tests for camera CRUD**

Create `internal/nvr/db/cameras_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCameraCreate(t *testing.T) {
	db := newTestDB(t)

	cam := &Camera{
		Name:         "Front Door",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/front-door",
	}

	err := db.CreateCamera(cam)
	require.NoError(t, err)
	require.NotEmpty(t, cam.ID)
}

func TestCameraGet(t *testing.T) {
	db := newTestDB(t)

	cam := &Camera{
		Name:         "Front Door",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/front-door",
		PTZCapable:   true,
	}
	err := db.CreateCamera(cam)
	require.NoError(t, err)

	got, err := db.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, cam.Name, got.Name)
	require.Equal(t, cam.RTSPURL, got.RTSPURL)
	require.Equal(t, cam.MediaMTXPath, got.MediaMTXPath)
	require.Equal(t, true, got.PTZCapable)
	require.Equal(t, "offline", got.Status)
}

func TestCameraGetNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.GetCamera("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCameraList(t *testing.T) {
	db := newTestDB(t)

	for i, name := range []string{"Camera A", "Camera B", "Camera C"} {
		err := db.CreateCamera(&Camera{
			Name:         name,
			RTSPURL:      "rtsp://192.168.1.100:554/stream",
			MediaMTXPath: "nvr/" + string(rune('a'+i)),
		})
		require.NoError(t, err)
	}

	cameras, err := db.ListCameras()
	require.NoError(t, err)
	require.Len(t, cameras, 3)
}

func TestCameraUpdate(t *testing.T) {
	db := newTestDB(t)

	cam := &Camera{
		Name:         "Old Name",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/cam",
	}
	err := db.CreateCamera(cam)
	require.NoError(t, err)

	cam.Name = "New Name"
	cam.Status = "online"
	err = db.UpdateCamera(cam)
	require.NoError(t, err)

	got, err := db.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, "New Name", got.Name)
	require.Equal(t, "online", got.Status)
}

func TestCameraDelete(t *testing.T) {
	db := newTestDB(t)

	cam := &Camera{
		Name:         "To Delete",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/delete-me",
	}
	err := db.CreateCamera(cam)
	require.NoError(t, err)

	err = db.DeleteCamera(cam.ID)
	require.NoError(t, err)

	_, err = db.GetCamera(cam.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCameraGetByPath(t *testing.T) {
	db := newTestDB(t)

	cam := &Camera{
		Name:         "By Path",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/by-path",
	}
	err := db.CreateCamera(cam)
	require.NoError(t, err)

	got, err := db.GetCameraByPath("nvr/by-path")
	require.NoError(t, err)
	require.Equal(t, cam.ID, got.ID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/db/ -v -run TestCamera
```

Expected: FAIL — Camera type and methods not defined

- [ ] **Step 3: Implement camera CRUD**

Create `internal/nvr/db/cameras.go`:

```go
package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Camera struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ONVIFEndpoint     string    `json:"onvif_endpoint,omitempty"`
	ONVIFUsername      string    `json:"onvif_username,omitempty"`
	ONVIFPassword     string    `json:"-"`
	ONVIFProfileToken string    `json:"onvif_profile_token,omitempty"`
	RTSPURL           string    `json:"rtsp_url"`
	PTZCapable        bool      `json:"ptz_capable"`
	MediaMTXPath      string    `json:"mediamtx_path"`
	Status            string    `json:"status"`
	Tags              string    `json:"tags"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (db *DB) CreateCamera(cam *Camera) error {
	if cam.ID == "" {
		cam.ID = uuid.New().String()
	}
	if cam.Status == "" {
		cam.Status = "offline"
	}
	if cam.Tags == "" {
		cam.Tags = "[]"
	}
	now := time.Now().UTC()
	cam.CreatedAt = now
	cam.UpdatedAt = now

	_, err := db.Exec(`INSERT INTO cameras
		(id, name, onvif_endpoint, onvif_username, onvif_password, onvif_profile_token,
		 rtsp_url, ptz_capable, mediamtx_path, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cam.ID, cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.CreatedAt, cam.UpdatedAt,
	)
	return err
}

func (db *DB) GetCamera(id string) (*Camera, error) {
	cam := &Camera{}
	err := db.QueryRow(`SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
		onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
		created_at, updated_at FROM cameras WHERE id = ?`, id).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return cam, err
}

func (db *DB) GetCameraByPath(path string) (*Camera, error) {
	cam := &Camera{}
	err := db.QueryRow(`SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
		onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
		created_at, updated_at FROM cameras WHERE mediamtx_path = ?`, path).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return cam, err
}

func (db *DB) ListCameras() ([]*Camera, error) {
	rows, err := db.Query(`SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
		onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
		created_at, updated_at FROM cameras ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []*Camera
	for rows.Next() {
		cam := &Camera{}
		err := rows.Scan(
			&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
			&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
			&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		cameras = append(cameras, cam)
	}
	return cameras, rows.Err()
}

func (db *DB) UpdateCamera(cam *Camera) error {
	cam.UpdatedAt = time.Now().UTC()
	result, err := db.Exec(`UPDATE cameras SET
		name = ?, onvif_endpoint = ?, onvif_username = ?, onvif_password = ?,
		onvif_profile_token = ?, rtsp_url = ?, ptz_capable = ?, mediamtx_path = ?,
		status = ?, tags = ?, updated_at = ?
		WHERE id = ?`,
		cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.UpdatedAt, cam.ID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteCamera(id string) error {
	result, err := db.Exec("DELETE FROM cameras WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Add google/uuid dependency**

```bash
go get github.com/google/uuid
```

Note: Check if `github.com/google/uuid` is already an indirect dependency in go.mod — it likely is via pion/webrtc. If so, this step just promotes it to direct.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/nvr/db/ -v -run TestCamera
```

Expected: All 7 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/db/cameras.go internal/nvr/db/cameras_test.go go.mod go.sum
git commit -m "feat(nvr): add camera CRUD database queries"
```

---

## Task 3: Recording Metadata Queries

**Files:**
- Create: `internal/nvr/db/recordings.go`
- Create: `internal/nvr/db/recordings_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/nvr/db/recordings_test.go`:

```go
package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createTestCamera(t *testing.T, db *DB) *Camera {
	t.Helper()
	cam := &Camera{
		Name:         "Test Camera",
		RTSPURL:      "rtsp://192.168.1.100:554/stream1",
		MediaMTXPath: "nvr/test-" + time.Now().Format("150405.000"),
	}
	err := db.CreateCamera(cam)
	require.NoError(t, err)
	return cam
}

func TestRecordingInsert(t *testing.T) {
	db := newTestDB(t)
	cam := createTestCamera(t, db)

	rec := &Recording{
		CameraID:   cam.ID,
		StartTime:  time.Now().Add(-10 * time.Minute).UTC(),
		EndTime:    time.Now().Add(-5 * time.Minute).UTC(),
		DurationMs: 300000,
		FilePath:   "/recordings/nvr/test/2026-03-19_14-00-00.mp4",
		FileSize:   1024 * 1024 * 50,
		Format:     "fmp4",
	}

	err := db.InsertRecording(rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID)
}

func TestRecordingQueryByTimeRange(t *testing.T) {
	db := newTestDB(t)
	cam := createTestCamera(t, db)

	base := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)

	// Insert 3 recordings: 12:00-12:05, 12:10-12:15, 12:20-12:25
	for i := 0; i < 3; i++ {
		start := base.Add(time.Duration(i*10) * time.Minute)
		end := start.Add(5 * time.Minute)
		err := db.InsertRecording(&Recording{
			CameraID:   cam.ID,
			StartTime:  start,
			EndTime:    end,
			DurationMs: 300000,
			FilePath:   "/recordings/seg" + string(rune('0'+i)),
			FileSize:   1000,
			Format:     "fmp4",
		})
		require.NoError(t, err)
	}

	// Query 12:05-12:20 should return middle recording (12:10-12:15 overlaps the range)
	recs, err := db.QueryRecordings(cam.ID, base.Add(5*time.Minute), base.Add(20*time.Minute))
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, base.Add(10*time.Minute), recs[0].StartTime)

	// Query entire range should return all 3
	recs, err = db.QueryRecordings(cam.ID, base, base.Add(25*time.Minute))
	require.NoError(t, err)
	require.Len(t, recs, 3)
}

func TestRecordingTimeline(t *testing.T) {
	db := newTestDB(t)
	cam := createTestCamera(t, db)

	base := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)

	// Insert recordings with gaps
	err := db.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: base.Add(1 * time.Hour),
		EndTime: base.Add(2 * time.Hour), DurationMs: 3600000,
		FilePath: "/rec/1", FileSize: 1000, Format: "fmp4",
	})
	require.NoError(t, err)

	err = db.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: base.Add(3 * time.Hour),
		EndTime: base.Add(4 * time.Hour), DurationMs: 3600000,
		FilePath: "/rec/2", FileSize: 1000, Format: "fmp4",
	})
	require.NoError(t, err)

	ranges, err := db.GetTimeline(cam.ID, base, base.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	require.Equal(t, base.Add(1*time.Hour), ranges[0].Start)
	require.Equal(t, base.Add(2*time.Hour), ranges[0].End)
}

func TestRecordingDeleteByPath(t *testing.T) {
	db := newTestDB(t)
	cam := createTestCamera(t, db)

	err := db.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: time.Now().UTC(), EndTime: time.Now().UTC(),
		DurationMs: 1000, FilePath: "/rec/to-delete", FileSize: 100, Format: "fmp4",
	})
	require.NoError(t, err)

	err = db.DeleteRecordingByPath("/rec/to-delete")
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/db/ -v -run TestRecording
```

Expected: FAIL — Recording type and methods not defined

- [ ] **Step 3: Implement recording queries**

Create `internal/nvr/db/recordings.go`:

```go
package db

import "time"

type Recording struct {
	ID         int64     `json:"id"`
	CameraID   string    `json:"camera_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DurationMs int64     `json:"duration_ms"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	Format     string    `json:"format"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (db *DB) InsertRecording(rec *Recording) error {
	result, err := db.Exec(`INSERT INTO recordings
		(camera_id, start_time, end_time, duration_ms, file_path, file_size, format)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.CameraID, rec.StartTime, rec.EndTime, rec.DurationMs,
		rec.FilePath, rec.FileSize, rec.Format,
	)
	if err != nil {
		return err
	}
	rec.ID, _ = result.LastInsertId()
	return nil
}

func (db *DB) QueryRecordings(cameraID string, start, end time.Time) ([]*Recording, error) {
	// Use overlap logic: recording overlaps range if it starts before range ends
	// AND ends after range starts
	rows, err := db.Query(`SELECT id, camera_id, start_time, end_time, duration_ms,
		file_path, file_size, format FROM recordings
		WHERE camera_id = ? AND end_time > ? AND start_time < ?
		ORDER BY start_time`,
		cameraID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*Recording
	for rows.Next() {
		rec := &Recording{}
		err := rows.Scan(&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

func (db *DB) GetTimeline(cameraID string, start, end time.Time) ([]TimeRange, error) {
	rows, err := db.Query(`SELECT start_time, end_time FROM recordings
		WHERE camera_id = ? AND end_time > ? AND start_time < ?
		ORDER BY start_time`,
		cameraID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ranges []TimeRange
	for rows.Next() {
		var tr TimeRange
		if err := rows.Scan(&tr.Start, &tr.End); err != nil {
			return nil, err
		}
		ranges = append(ranges, tr)
	}
	return ranges, rows.Err()
}

func (db *DB) DeleteRecordingByPath(filePath string) error {
	_, err := db.Exec("DELETE FROM recordings WHERE file_path = ?", filePath)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/nvr/db/ -v -run TestRecording
```

Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/recordings.go internal/nvr/db/recordings_test.go
git commit -m "feat(nvr): add recording metadata queries with timeline support"
```

---

## Task 4: User & Token Queries

**Files:**
- Create: `internal/nvr/db/users.go`
- Create: `internal/nvr/db/tokens.go`
- Create: `internal/nvr/db/config.go`
- Create: `internal/nvr/db/users_test.go`
- Create: `internal/nvr/db/tokens_test.go`

- [ ] **Step 1: Write failing tests for users**

Create `internal/nvr/db/users_test.go`:

```go
package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserCreate(t *testing.T) {
	db := newTestDB(t)

	user := &User{
		Username:          "admin",
		PasswordHash:      "$argon2id$v=19$m=65536,t=3,p=4$fakesalt$fakehash",
		Role:              "admin",
		CameraPermissions: `"*"`,
	}
	err := db.CreateUser(user)
	require.NoError(t, err)
	require.NotEmpty(t, user.ID)
}

func TestUserGetByUsername(t *testing.T) {
	db := newTestDB(t)

	user := &User{
		Username:          "admin",
		PasswordHash:      "$argon2id$fakehash",
		Role:              "admin",
		CameraPermissions: `"*"`,
	}
	err := db.CreateUser(user)
	require.NoError(t, err)

	got, err := db.GetUserByUsername("admin")
	require.NoError(t, err)
	require.Equal(t, user.ID, got.ID)
	require.Equal(t, "admin", got.Role)
}

func TestUserDuplicateUsername(t *testing.T) {
	db := newTestDB(t)

	user1 := &User{Username: "admin", PasswordHash: "hash1", Role: "admin", CameraPermissions: `"*"`}
	err := db.CreateUser(user1)
	require.NoError(t, err)

	user2 := &User{Username: "admin", PasswordHash: "hash2", Role: "viewer", CameraPermissions: `"*"`}
	err = db.CreateUser(user2)
	require.Error(t, err) // UNIQUE constraint violation
}

func TestUserList(t *testing.T) {
	db := newTestDB(t)

	for _, name := range []string{"alice", "bob", "charlie"} {
		err := db.CreateUser(&User{Username: name, PasswordHash: "hash", Role: "viewer", CameraPermissions: `"*"`})
		require.NoError(t, err)
	}

	users, err := db.ListUsers()
	require.NoError(t, err)
	require.Len(t, users, 3)
}

func TestUserUpdate(t *testing.T) {
	db := newTestDB(t)

	user := &User{Username: "alice", PasswordHash: "hash1", Role: "viewer", CameraPermissions: `"*"`}
	err := db.CreateUser(user)
	require.NoError(t, err)

	user.Role = "admin"
	err = db.UpdateUser(user)
	require.NoError(t, err)

	got, err := db.GetUser(user.ID)
	require.NoError(t, err)
	require.Equal(t, "admin", got.Role)
}

func TestUserDelete(t *testing.T) {
	db := newTestDB(t)

	user := &User{Username: "delete-me", PasswordHash: "hash", Role: "viewer", CameraPermissions: `"*"`}
	err := db.CreateUser(user)
	require.NoError(t, err)

	err = db.DeleteUser(user.ID)
	require.NoError(t, err)

	_, err = db.GetUser(user.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUserCount(t *testing.T) {
	db := newTestDB(t)

	count, err := db.CountUsers()
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = db.CreateUser(&User{Username: "admin", PasswordHash: "hash", Role: "admin", CameraPermissions: `"*"`})
	require.NoError(t, err)

	count, err = db.CountUsers()
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/db/ -v -run TestUser
```

Expected: FAIL

- [ ] **Step 3: Implement user queries**

Create `internal/nvr/db/users.go`:

```go
package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	PasswordHash      string    `json:"-"`
	Role              string    `json:"role"`
	CameraPermissions string    `json:"camera_permissions"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (db *DB) CreateUser(user *User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := db.Exec(`INSERT INTO users
		(id, username, password_hash, role, camera_permissions, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.Role,
		user.CameraPermissions, user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (db *DB) GetUser(id string) (*User, error) {
	user := &User{}
	err := db.QueryRow(`SELECT id, username, password_hash, role, camera_permissions,
		created_at, updated_at FROM users WHERE id = ?`, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role,
		&user.CameraPermissions, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return user, err
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := db.QueryRow(`SELECT id, username, password_hash, role, camera_permissions,
		created_at, updated_at FROM users WHERE username = ?`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role,
		&user.CameraPermissions, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return user, err
}

func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.Query(`SELECT id, username, password_hash, role, camera_permissions,
		created_at, updated_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role,
			&user.CameraPermissions, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (db *DB) UpdateUser(user *User) error {
	user.UpdatedAt = time.Now().UTC()
	result, err := db.Exec(`UPDATE users SET
		username = ?, password_hash = ?, role = ?, camera_permissions = ?, updated_at = ?
		WHERE id = ?`,
		user.Username, user.PasswordHash, user.Role,
		user.CameraPermissions, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteUser(id string) error {
	result, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) CountUsers() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
```

- [ ] **Step 4: Implement token queries**

Create `internal/nvr/db/tokens.go`:

```go
package db

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (db *DB) CreateRefreshToken(token *RefreshToken) error {
	if token.ID == "" {
		token.ID = uuid.New().String()
	}
	_, err := db.Exec(`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES (?, ?, ?, ?)`,
		token.ID, token.UserID, token.TokenHash, token.ExpiresAt,
	)
	return err
}

func (db *DB) GetRefreshToken(tokenHash string) (*RefreshToken, error) {
	token := &RefreshToken{}
	err := db.QueryRow(`SELECT id, user_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = ?`, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (db *DB) RevokeRefreshToken(tokenHash string) error {
	now := time.Now().UTC()
	_, err := db.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE token_hash = ?",
		now, tokenHash,
	)
	return err
}

func (db *DB) RevokeAllUserTokens(userID string) error {
	now := time.Now().UTC()
	_, err := db.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL",
		now, userID,
	)
	return err
}

func (db *DB) CleanExpiredTokens() error {
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", time.Now().UTC())
	return err
}
```

- [ ] **Step 5: Implement config key-value store**

Create `internal/nvr/db/config.go`:

```go
package db

func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	return value, err
}

func (db *DB) SetConfig(key, value string) error {
	_, err := db.Exec(`INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (db *DB) DeleteConfig(key string) error {
	_, err := db.Exec("DELETE FROM config WHERE key = ?", key)
	return err
}
```

- [ ] **Step 6: Write token tests**

Create `internal/nvr/db/tokens_test.go`:

```go
package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createTestUser(t *testing.T, db *DB) *User {
	t.Helper()
	user := &User{
		Username:          "testuser-" + time.Now().Format("150405.000"),
		PasswordHash:      "fakehash",
		Role:              "admin",
		CameraPermissions: `"*"`,
	}
	err := db.CreateUser(user)
	require.NoError(t, err)
	return user
}

func TestRefreshTokenCreate(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db)

	token := &RefreshToken{
		UserID:    user.ID,
		TokenHash: "sha256hashoftoken",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
	}
	err := db.CreateRefreshToken(token)
	require.NoError(t, err)
	require.NotEmpty(t, token.ID)
}

func TestRefreshTokenGet(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db)

	token := &RefreshToken{
		UserID:    user.ID,
		TokenHash: "uniquehash",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
	}
	err := db.CreateRefreshToken(token)
	require.NoError(t, err)

	got, err := db.GetRefreshToken("uniquehash")
	require.NoError(t, err)
	require.Equal(t, user.ID, got.UserID)
	require.Nil(t, got.RevokedAt)
}

func TestRefreshTokenRevoke(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db)

	token := &RefreshToken{
		UserID:    user.ID,
		TokenHash: "torevoke",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
	}
	err := db.CreateRefreshToken(token)
	require.NoError(t, err)

	err = db.RevokeRefreshToken("torevoke")
	require.NoError(t, err)

	got, err := db.GetRefreshToken("torevoke")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

func TestRefreshTokenRevokeAllForUser(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db)

	for i := 0; i < 3; i++ {
		err := db.CreateRefreshToken(&RefreshToken{
			UserID:    user.ID,
			TokenHash: "hash" + string(rune('0'+i)),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
		})
		require.NoError(t, err)
	}

	err := db.RevokeAllUserTokens(user.ID)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		got, err := db.GetRefreshToken("hash" + string(rune('0'+i)))
		require.NoError(t, err)
		require.NotNil(t, got.RevokedAt)
	}
}

func TestConfigGetSet(t *testing.T) {
	db := newTestDB(t)

	err := db.SetConfig("test_key", "test_value")
	require.NoError(t, err)

	val, err := db.GetConfig("test_key")
	require.NoError(t, err)
	require.Equal(t, "test_value", val)

	// Upsert
	err = db.SetConfig("test_key", "updated_value")
	require.NoError(t, err)

	val, err = db.GetConfig("test_key")
	require.NoError(t, err)
	require.Equal(t, "updated_value", val)
}
```

- [ ] **Step 7: Run all DB tests**

```bash
go test ./internal/nvr/db/ -v
```

Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/db/users.go internal/nvr/db/tokens.go internal/nvr/db/config.go internal/nvr/db/users_test.go internal/nvr/db/tokens_test.go
git commit -m "feat(nvr): add user, token, and config database queries"
```

---

## Task 5: Crypto Utilities (RSA Keys + AES Encryption)

**Files:**
- Create: `internal/nvr/crypto/keys.go`
- Create: `internal/nvr/crypto/encrypt.go`
- Create: `internal/nvr/crypto/keys_test.go`
- Create: `internal/nvr/crypto/encrypt_test.go`

- [ ] **Step 1: Write failing tests for key generation and HKDF**

Create `internal/nvr/crypto/keys_test.go`:

```go
package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRSAKeyPair(t *testing.T) {
	privPEM, pubPEM, err := GenerateRSAKeyPair()
	require.NoError(t, err)
	require.Contains(t, string(privPEM), "BEGIN RSA PRIVATE KEY")
	require.Contains(t, string(pubPEM), "BEGIN PUBLIC KEY")
}

func TestParseRSAPrivateKey(t *testing.T) {
	privPEM, _, err := GenerateRSAKeyPair()
	require.NoError(t, err)

	key, err := ParseRSAPrivateKey(privPEM)
	require.NoError(t, err)
	require.NotNil(t, key)
	require.Equal(t, 2048, key.N.BitLen())
}

func TestDeriveKey(t *testing.T) {
	key1 := DeriveKey("master-secret", "info-string-1")
	key2 := DeriveKey("master-secret", "info-string-2")
	key3 := DeriveKey("master-secret", "info-string-1")

	require.Len(t, key1, 32) // AES-256
	require.NotEqual(t, key1, key2) // Different info = different key
	require.Equal(t, key1, key3)    // Same inputs = same output
}

func TestJWKSFromPublicKey(t *testing.T) {
	_, pubPEM, err := GenerateRSAKeyPair()
	require.NoError(t, err)

	jwks, err := JWKSFromPublicKey(pubPEM)
	require.NoError(t, err)
	require.Contains(t, string(jwks), `"keys"`)
	require.Contains(t, string(jwks), `"kty":"RSA"`)
	require.Contains(t, string(jwks), `"use":"sig"`)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/crypto/ -v -run TestGenerate
```

Expected: FAIL

- [ ] **Step 3: Implement key generation**

Create `internal/nvr/crypto/keys.go`:

```go
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"

	"golang.org/x/crypto/hkdf"
	"io"
)

// GenerateRSAKeyPair generates a 2048-bit RSA key pair and returns PEM-encoded private and public keys.
func GenerateRSAKeyPair() (privPEM []byte, pubPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	privPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	return privPEM, pubPEM, nil
}

// ParseRSAPrivateKey parses a PEM-encoded RSA private key.
func ParseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// DeriveKey derives a 32-byte key from a master secret and info string using HKDF-SHA256.
func DeriveKey(masterSecret, info string) []byte {
	hkdfReader := hkdf.New(sha256.New, []byte(masterSecret), nil, []byte(info))
	key := make([]byte, 32)
	io.ReadFull(hkdfReader, key)
	return key
}

// JWKSFromPublicKey generates a JWKS JSON document from a PEM-encoded RSA public key.
func JWKSFromPublicKey(pubPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": "nvr-signing-key",
				"n":   base64URLEncode(rsaPub.N.Bytes()),
				"e":   base64URLEncode(big.NewInt(int64(rsaPub.E)).Bytes()),
			},
		},
	}

	return json.Marshal(jwks)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
```

- [ ] **Step 4: Write failing tests for encryption**

Create `internal/nvr/crypto/encrypt_test.go`:

```go
package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
	key := DeriveKey("test-secret", "test-info")
	plaintext := "my-onvif-password"

	ciphertext, err := Encrypt(key, []byte(plaintext))
	require.NoError(t, err)
	require.NotEqual(t, plaintext, string(ciphertext))

	decrypted, err := Decrypt(key, ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, string(decrypted))
}

func TestEncryptDifferentOutputs(t *testing.T) {
	key := DeriveKey("test-secret", "test-info")
	plaintext := []byte("same-input")

	ct1, _ := Encrypt(key, plaintext)
	ct2, _ := Encrypt(key, plaintext)

	// AES-GCM uses random nonce, so same plaintext produces different ciphertext
	require.NotEqual(t, ct1, ct2)
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := DeriveKey("secret-1", "info")
	key2 := DeriveKey("secret-2", "info")

	ciphertext, err := Encrypt(key1, []byte("secret data"))
	require.NoError(t, err)

	_, err = Decrypt(key2, ciphertext)
	require.Error(t, err)
}
```

- [ ] **Step 5: Implement encryption**

Create `internal/nvr/crypto/encrypt.go`:

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given 32-byte key.
// The nonce is prepended to the ciphertext.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext (with prepended nonce) using AES-256-GCM.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}
```

- [ ] **Step 6: Add golang.org/x/crypto dependency (if not already present)**

```bash
go get golang.org/x/crypto
```

Note: `golang.org/x/crypto` is likely already a dependency via other packages. Check go.mod first.

- [ ] **Step 7: Run all crypto tests**

```bash
go test ./internal/nvr/crypto/ -v
```

Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/crypto/ go.mod go.sum
git commit -m "feat(nvr): add RSA key generation, HKDF derivation, and AES-256-GCM encryption"
```

---

## Task 6: YAML Writer (Safe Config Modification)

**Files:**
- Create: `internal/nvr/yamlwriter/writer.go`
- Create: `internal/nvr/yamlwriter/writer_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/nvr/yamlwriter/writer_test.go`:

```go
package yamlwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddPath(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	initial := `# Global config
logLevel: info
paths:
  all_others:
`
	err := os.WriteFile(yamlPath, []byte(initial), 0644)
	require.NoError(t, err)

	w := New(yamlPath)

	err = w.AddPath("nvr/front-door", map[string]interface{}{
		"source":   "rtsp://192.168.1.100:554/stream1",
		"record":   true,
		"recordPath": "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f",
	})
	require.NoError(t, err)

	// Read back and verify
	data, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, "nvr/front-door")
	require.Contains(t, content, "rtsp://192.168.1.100:554/stream1")
	// Comment should be preserved
	require.Contains(t, content, "# Global config")
}

func TestRemovePath(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	initial := `paths:
  all_others:
  nvr/front-door:
    source: rtsp://192.168.1.100:554/stream1
    record: true
  nvr/garage:
    source: rtsp://192.168.1.101:554/stream1
`
	err := os.WriteFile(yamlPath, []byte(initial), 0644)
	require.NoError(t, err)

	w := New(yamlPath)
	err = w.RemovePath("nvr/front-door")
	require.NoError(t, err)

	data, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	content := string(data)
	require.NotContains(t, content, "nvr/front-door")
	require.Contains(t, content, "nvr/garage")
}

func TestGetNVRPaths(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	initial := `paths:
  all_others:
  nvr/front-door:
    source: rtsp://192.168.1.100:554/stream1
  nvr/garage:
    source: rtsp://192.168.1.101:554/stream1
  my-custom-stream:
    source: rtsp://10.0.0.1/live
`
	err := os.WriteFile(yamlPath, []byte(initial), 0644)
	require.NoError(t, err)

	w := New(yamlPath)
	paths, err := w.GetNVRPaths()
	require.NoError(t, err)
	require.Len(t, paths, 2)
	require.Contains(t, paths, "nvr/front-door")
	require.Contains(t, paths, "nvr/garage")
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	err := os.WriteFile(yamlPath, []byte("paths:\n  all_others:\n"), 0644)
	require.NoError(t, err)

	w := New(yamlPath)
	err = w.AddPath("nvr/test", map[string]interface{}{"source": "rtsp://1.2.3.4/s"})
	require.NoError(t, err)

	// Verify no temp files left behind
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1) // Only mediamtx.yml
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/yamlwriter/ -v
```

Expected: FAIL

- [ ] **Step 3: Implement YAML writer**

Create `internal/nvr/yamlwriter/writer.go`:

```go
package yamlwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Writer provides safe read-modify-write operations on mediamtx.yml.
type Writer struct {
	path string
	mu   sync.Mutex
}

// New creates a Writer for the given YAML file path.
func New(path string) *Writer {
	return &Writer{path: path}
}

// AddPath adds or updates a path entry under the `paths` key.
// Uses AST manipulation to preserve comments and formatting.
func (w *Writer) AddPath(name string, config map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Build the new path node from config map
	configBytes, err := yaml.Marshal(map[string]interface{}{name: config})
	if err != nil {
		return fmt.Errorf("marshal path config: %w", err)
	}
	configFile, err := parser.ParseBytes(configBytes, 0)
	if err != nil {
		return fmt.Errorf("parse path config: %w", err)
	}

	// Find the "paths" mapping in the AST
	pathsNode := w.findPathsNode(file)
	if pathsNode == nil {
		return fmt.Errorf("no 'paths' key found in config")
	}

	// Remove existing entry with same name if present
	w.removePathFromNode(pathsNode, name)

	// Add new entry from parsed config
	newDoc := configFile.Docs[0].Body.(*ast.MappingNode)
	for _, v := range newDoc.Values {
		pathsNode.Values = append(pathsNode.Values, v)
	}

	return w.atomicWriteAST(file)
}

// RemovePath removes a path entry from the `paths` key.
func (w *Writer) RemovePath(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	pathsNode := w.findPathsNode(file)
	if pathsNode == nil {
		return nil
	}

	w.removePathFromNode(pathsNode, name)
	return w.atomicWriteAST(file)
}

func (w *Writer) findPathsNode(file *ast.File) *ast.MappingNode {
	for _, doc := range file.Docs {
		body, ok := doc.Body.(*ast.MappingNode)
		if !ok {
			continue
		}
		for _, item := range body.Values {
			if item.Key.String() == "paths" {
				if m, ok := item.Value.(*ast.MappingNode); ok {
					return m
				}
			}
		}
	}
	return nil
}

func (w *Writer) removePathFromNode(pathsNode *ast.MappingNode, name string) {
	filtered := make([]*ast.MappingValueNode, 0, len(pathsNode.Values))
	for _, item := range pathsNode.Values {
		if item.Key.String() != name {
			filtered = append(filtered, item)
		}
	}
	pathsNode.Values = filtered
}

// GetNVRPaths returns all path names prefixed with "nvr/".
func (w *Writer) GetNVRPaths() ([]string, error) {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Use AST parser to preserve structure while reading
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	var nvrPaths []string
	for _, doc := range file.Docs {
		body, ok := doc.Body.(*ast.MappingNode)
		if !ok {
			continue
		}
		for _, item := range body.Values {
			if item.Key.String() == "paths" {
				pathsMap, ok := item.Value.(*ast.MappingNode)
				if !ok {
					continue
				}
				for _, pathItem := range pathsMap.Values {
					name := pathItem.Key.String()
					if strings.HasPrefix(name, "nvr/") {
						nvrPaths = append(nvrPaths, name)
					}
				}
			}
		}
	}

	return nvrPaths, nil
}

func (w *Writer) atomicWriteAST(file *ast.File) error {
	out := file.String()

	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".mediamtx-*.yml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, w.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
```

This approach uses AST manipulation for all write operations, preserving comments, formatting, and ordering throughout the entire YAML file. Only the NVR-managed path entries are modified.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/nvr/yamlwriter/ -v
```

Expected: All tests PASS. The comment preservation test may need adjustment if `goccy/go-yaml` marshal doesn't preserve comments — update the test accordingly.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/yamlwriter/
git commit -m "feat(nvr): add safe YAML writer with atomic writes and file locking"
```

---

## Task 7: NVR Config Fields & Record Cleaner Callback

**Files:**
- Modify: `internal/conf/conf.go`
- Modify: `internal/recordcleaner/cleaner.go`

- [ ] **Step 1: Add NVR config fields**

Read `internal/conf/conf.go` to find where config fields are defined and where defaults are set. Add after the existing field groups:

In the `Conf` struct, add after the last field group (before the closing brace or before `PathDefaults`):

```go
// NVR
NVR           bool   `json:"nvr"`
NVRDatabase   string `json:"nvrDatabase"`
NVRJWTSecret  string `json:"nvrJWTSecret"`
```

In `setDefaults()`, add:

```go
c.NVRDatabase = "~/.mediamtx/nvr.db"
```

- [ ] **Step 2: Add OnSegmentDelete callback to record cleaner**

Read `internal/recordcleaner/cleaner.go`. The current `Cleaner` struct (lines 20-29) has no callback. Add one:

In the `Cleaner` struct, add:

```go
OnSegmentDelete func(path string)
```

In the `Initialize()` method (or wherever the cleaner is created), set it to a no-op if nil:

```go
if c.OnSegmentDelete == nil {
    c.OnSegmentDelete = func(path string) {}
}
```

In `deleteExpiredSegments()`, add the callback call before `os.Remove`:

```go
for _, seg := range segments {
    c.Log(logger.Debug, "removing %s", seg.Fpath)
    c.OnSegmentDelete(seg.Fpath)
    os.Remove(seg.Fpath)
}
```

- [ ] **Step 3: Run existing tests to verify nothing is broken**

```bash
go test ./internal/conf/ -v -count=1
go test ./internal/recordcleaner/ -v -count=1
```

Expected: All existing tests PASS (the callback defaults to no-op, so no behavior changes)

- [ ] **Step 4: Write a test for the new callback**

Add to the existing record cleaner test file (or create one if none exists):

```go
func TestCleanerOnSegmentDeleteCallback(t *testing.T) {
    // Setup test that verifies the callback is invoked before file deletion
    var deletedPaths []string
    // ... (depends on existing test patterns in recordcleaner/)
}
```

Adapt this based on the existing test infrastructure in `internal/recordcleaner/`.

- [ ] **Step 5: Commit**

```bash
git add internal/conf/conf.go internal/recordcleaner/cleaner.go
git commit -m "feat(nvr): add NVR config fields and OnSegmentDelete callback to record cleaner"
```

---

## Task 8: NVR Subsystem & Core Integration

**Files:**
- Create: `internal/nvr/nvr.go`
- Create: `internal/nvr/nvr_test.go`
- Modify: `internal/core/core.go`
- Modify: `mediamtx.yml`

- [ ] **Step 1: Write failing test for NVR initialization**

Create `internal/nvr/nvr_test.go`:

```go
package nvr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNVRInitialize(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nvr.db")
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	// Write minimal config
	err := os.WriteFile(yamlPath, []byte("paths:\n  all_others:\n"), 0644)
	require.NoError(t, err)

	n := &NVR{
		DatabasePath: dbPath,
		JWTSecret:    "test-secret-key-for-development",
		ConfigPath:   yamlPath,
	}

	err = n.Initialize()
	require.NoError(t, err)
	defer n.Close()

	require.NotNil(t, n.db)
}

func TestNVRFirstRunSetup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nvr.db")
	yamlPath := filepath.Join(dir, "mediamtx.yml")

	err := os.WriteFile(yamlPath, []byte("paths:\n  all_others:\n"), 0644)
	require.NoError(t, err)

	n := &NVR{
		DatabasePath: dbPath,
		JWTSecret:    "test-secret",
		ConfigPath:   yamlPath,
	}

	err = n.Initialize()
	require.NoError(t, err)
	defer n.Close()

	require.True(t, n.IsSetupRequired())
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/nvr/ -v -run TestNVR
```

Expected: FAIL

- [ ] **Step 3: Implement NVR subsystem**

Create `internal/nvr/nvr.go`:

```go
package nvr

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// NVR is the main NVR subsystem.
type NVR struct {
	DatabasePath string
	JWTSecret    string
	ConfigPath   string

	db         *db.DB
	yamlWriter *yamlwriter.Writer
	privateKey *rsa.PrivateKey
	jwksJSON   []byte
}

// Initialize starts the NVR subsystem: opens the database, runs migrations,
// loads or generates RSA keys.
func (n *NVR) Initialize() error {
	// Auto-generate JWT secret if empty
	if n.JWTSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate JWT secret: %w", err)
		}
		n.JWTSecret = hex.EncodeToString(b)
		// Note: The generated secret is used for this session.
		// The implementer should also write it back to mediamtx.yml via the YAML writer
		// so it persists across restarts.
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(n.DatabasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	database, err := db.Open(n.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	n.db = database

	// Initialize YAML writer
	n.yamlWriter = yamlwriter.New(n.ConfigPath)

	// Load or generate RSA keys
	if err := n.loadOrGenerateKeys(); err != nil {
		n.db.Close()
		return fmt.Errorf("load keys: %w", err)
	}

	return nil
}

// Close shuts down the NVR subsystem.
func (n *NVR) Close() {
	if n.db != nil {
		n.db.Close()
	}
}

// IsSetupRequired returns true if no admin user has been created yet.
func (n *NVR) IsSetupRequired() bool {
	count, err := n.db.CountUsers()
	if err != nil {
		return true
	}
	return count == 0
}

// DB returns the database handle.
func (n *NVR) DB() *db.DB {
	return n.db
}

// JWKSJSON returns the JWKS JSON document for the NVR's public key.
func (n *NVR) JWKSJSON() []byte {
	return n.jwksJSON
}

// PrivateKey returns the RSA private key for signing JWTs.
func (n *NVR) PrivateKey() *rsa.PrivateKey {
	return n.privateKey
}

func (n *NVR) loadOrGenerateKeys() error {
	encKey := crypto.DeriveKey(n.JWTSecret, "nvr-rsa-key-encryption")

	// Try to load existing key
	encPrivPEM, err := n.db.GetConfig("rsa_private_key")
	if err == nil && encPrivPEM != "" {
		encBytes, err := base64.StdEncoding.DecodeString(encPrivPEM)
		if err != nil {
			return fmt.Errorf("decode stored key: %w", err)
		}
		privPEM, err := crypto.Decrypt(encKey, encBytes)
		if err != nil {
			return fmt.Errorf("decrypt stored key: %w", err)
		}
		n.privateKey, err = crypto.ParseRSAPrivateKey(privPEM)
		if err != nil {
			return fmt.Errorf("parse stored key: %w", err)
		}

		pubPEM, err := n.db.GetConfig("rsa_public_key")
		if err != nil {
			return fmt.Errorf("get public key: %w", err)
		}
		n.jwksJSON, err = crypto.JWKSFromPublicKey([]byte(pubPEM))
		if err != nil {
			return fmt.Errorf("generate JWKS: %w", err)
		}
		return nil
	}

	// Generate new key pair
	privPEM, pubPEM, err := crypto.GenerateRSAKeyPair()
	if err != nil {
		return err
	}

	n.privateKey, err = crypto.ParseRSAPrivateKey(privPEM)
	if err != nil {
		return err
	}

	n.jwksJSON, err = crypto.JWKSFromPublicKey(pubPEM)
	if err != nil {
		return err
	}

	// Encrypt and store private key
	encPriv, err := crypto.Encrypt(encKey, privPEM)
	if err != nil {
		return fmt.Errorf("encrypt private key: %w", err)
	}
	if err := n.db.SetConfig("rsa_private_key", base64.StdEncoding.EncodeToString(encPriv)); err != nil {
		return fmt.Errorf("store private key: %w", err)
	}
	if err := n.db.SetConfig("rsa_public_key", string(pubPEM)); err != nil {
		return fmt.Errorf("store public key: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Run NVR tests**

```bash
go test ./internal/nvr/ -v
```

Expected: All tests PASS

- [ ] **Step 5: Integrate NVR into Core**

Read `internal/core/core.go` and add NVR initialization. In the Core struct, add:

```go
nvr *nvr.NVR
```

In the initialization section (after record cleaner, before API), add following the existing pattern:

```go
if p.conf.NVR {
    p.nvr = &nvr.NVR{
        DatabasePath: p.conf.NVRDatabase,
        JWTSecret:    p.conf.NVRJWTSecret,
        ConfigPath:   p.confPath,
    }
    if err := p.nvr.Initialize(); err != nil {
        return err
    }
}
```

In the close/cleanup section, add:

```go
if p.nvr != nil {
    p.nvr.Close()
}
```

- [ ] **Step 6: Add NVR config section to mediamtx.yml**

Add at the end of the global config section (before `pathDefaults`):

```yaml
###############################################
# NVR settings

# Enable NVR functionality (camera management UI, ONVIF, recording timeline).
nvr: no
# Path to the NVR SQLite database.
nvrDatabase: ~/.mediamtx/nvr.db
# Secret key for encrypting stored credentials. Auto-generated on first run if empty.
nvrJWTSecret: ""
```

- [ ] **Step 7: Run existing core tests to verify nothing breaks**

```bash
go test ./internal/core/ -v -count=1 -timeout 120s
```

Expected: All existing tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/nvr.go internal/nvr/nvr_test.go internal/core/core.go internal/conf/conf.go mediamtx.yml
git commit -m "feat(nvr): add NVR subsystem with core integration"
```

---

## Task 9: JWT Auth Middleware & Auth Endpoints

**Files:**
- Create: `internal/nvr/api/middleware.go`
- Create: `internal/nvr/api/auth.go`
- Create: `internal/nvr/api/jwks.go`
- Create: `internal/nvr/api/middleware_test.go`
- Create: `internal/nvr/api/auth_test.go`

- [ ] **Step 1: Write failing tests for JWT middleware**

Create `internal/nvr/api/middleware_test.go`:

```go
package api

import (
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
)

func testKeyPair(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	privPEM, _, err := crypto.GenerateRSAKeyPair()
	require.NoError(t, err)
	key, err := crypto.ParseRSAPrivateKey(privPEM)
	require.NoError(t, err)
	return key
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "nvr-signing-key"
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func TestMiddlewareValidToken(t *testing.T) {
	key := testKeyPair(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	m := &Middleware{PrivateKey: key}
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		c.JSON(200, gin.H{"user_id": userID})
	})

	token := signTestToken(t, key, jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
}

func TestMiddlewareNoToken(t *testing.T) {
	key := testKeyPair(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	m := &Middleware{PrivateKey: key}
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 401, w.Code)
}

func TestMiddlewareExpiredToken(t *testing.T) {
	key := testKeyPair(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	m := &Middleware{PrivateKey: key}
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {})

	token := signTestToken(t, key, jwt.MapClaims{
		"sub": "user-123",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 401, w.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/nvr/api/ -v -run TestMiddleware
```

Expected: FAIL

- [ ] **Step 3: Implement JWT middleware**

Create `internal/nvr/api/middleware.go`:

```go
package api

import (
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Middleware struct {
	PrivateKey *rsa.PrivateKey
}

func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ""

		// Check Authorization header
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenString = strings.TrimPrefix(auth, "Bearer ")
		}

		// Check query parameter (for SSE)
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return &m.PrivateKey.PublicKey, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		c.Set("user_id", claims["sub"])
		c.Set("role", claims["role"])
		if perms, ok := claims["camera_permissions"]; ok {
			c.Set("camera_permissions", perms)
		}

		c.Next()
	}
}
```

- [ ] **Step 4: Write failing tests for auth endpoints**

Create `internal/nvr/api/auth_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSetupEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newTestDB(t)
	key := testKeyPair(t)

	auth := &AuthHandler{
		DB:         database,
		PrivateKey: key,
	}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", auth.Setup)

	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "securepass123",
	})

	req := httptest.NewRequest("POST", "/api/nvr/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// Verify user was created
	user, err := database.GetUserByUsername("admin")
	require.NoError(t, err)
	require.Equal(t, "admin", user.Role)
}

func TestLoginEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newTestDB(t)
	key := testKeyPair(t)

	auth := &AuthHandler{
		DB:         database,
		PrivateKey: key,
	}

	// Create user via setup first
	router := gin.New()
	router.POST("/api/nvr/auth/setup", auth.Setup)
	router.POST("/api/nvr/auth/login", auth.Login)

	// Setup
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "testpass"})
	req := httptest.NewRequest("POST", "/api/nvr/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Login
	body, _ = json.Marshal(map[string]string{"username": "admin", "password": "testpass"})
	req = httptest.NewRequest("POST", "/api/nvr/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotEmpty(t, resp["access_token"])

	// Should have refresh token cookie
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			found = true
			require.True(t, c.HttpOnly)
			require.Equal(t, http.SameSiteStrictMode, c.SameSite)
		}
	}
	require.True(t, found)
}
```

- [ ] **Step 5: Implement auth endpoints**

Create `internal/nvr/api/auth.go`:

```go
package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/matthewhartstonge/argon2"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type AuthHandler struct {
	DB         *db.DB
	PrivateKey *rsa.PrivateKey
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Setup(c *gin.Context) {
	// Only allow setup if no users exist
	count, err := h.DB.CountUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	hash := hashPassword(req.Password)
	user := &db.User{
		Username:          req.Username,
		PasswordHash:      hash,
		Role:              "admin",
		CameraPermissions: `"*"`,
	}
	if err := h.DB.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "username": user.Username})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !verifyPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Issue access JWT
	accessToken, err := h.issueAccessToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	// Issue refresh token
	refreshRaw := generateRandomToken()
	refreshHash := sha256Hash(refreshRaw)
	err = h.DB.CreateRefreshToken(&db.RefreshToken{
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create refresh token"})
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", refreshRaw, 7*24*60*60, "/api/nvr/auth", "", false, true)

	c.JSON(http.StatusOK, gin.H{"access_token": accessToken})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshRaw, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	refreshHash := sha256Hash(refreshRaw)
	token, err := h.DB.GetRefreshToken(refreshHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	if token.RevokedAt != nil || token.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "expired or revoked token"})
		return
	}

	user, err := h.DB.GetUser(token.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	accessToken, err := h.issueAccessToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": accessToken})
}

func (h *AuthHandler) Revoke(c *gin.Context) {
	refreshRaw, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
		return
	}

	refreshHash := sha256Hash(refreshRaw)
	h.DB.RevokeRefreshToken(refreshHash)

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", "", -1, "/api/nvr/auth", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) issueAccessToken(user *db.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":                    user.ID,
		"username":               user.Username,
		"role":                   user.Role,
		"camera_permissions":     user.CameraPermissions,
		"mediamtx_permissions":   buildMediaMTXPermissions(user),
		"exp":                    time.Now().Add(15 * time.Minute).Unix(),
		"iat":                    time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "nvr-signing-key"
	return token.SignedString(h.PrivateKey)
}

func buildMediaMTXPermissions(user *db.User) []map[string]string {
	// Admin gets full access, viewer gets read + playback on their cameras
	if user.Role == "admin" {
		return []map[string]string{
			{"action": "publish"},
			{"action": "read"},
			{"action": "playback"},
			{"action": "api"},
			{"action": "metrics"},
			{"action": "pprof"},
		}
	}
	return []map[string]string{
		{"action": "read"},
		{"action": "playback"},
	}
}

func hashPassword(password string) string {
	// Use matthewhartstonge/argon2 to match the existing codebase pattern
	// (see internal/conf/credential.go)
	cfg := argon2.DefaultConfig()
	encoded, err := cfg.HashEncoded([]byte(password))
	if err != nil {
		panic("argon2 hash failed: " + err.Error())
	}
	return string(encoded)
}

func verifyPassword(password, encoded string) bool {
	ok, err := argon2.VerifyEncoded([]byte(password), []byte(encoded))
	return ok && err == nil
}

func generateRandomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 6: Implement JWKS endpoint**

Create `internal/nvr/api/jwks.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type JWKSHandler struct {
	JWKSJSON []byte
}

func (h *JWKSHandler) ServeJWKS(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", h.JWKSJSON)
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/nvr/api/ -v
```

Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/middleware.go internal/nvr/api/auth.go internal/nvr/api/jwks.go internal/nvr/api/middleware_test.go internal/nvr/api/auth_test.go
git commit -m "feat(nvr): add JWT auth middleware, auth endpoints, and JWKS handler"
```

---

## Task 10: Camera, Recording, User, and System API Endpoints

**Files:**
- Create: `internal/nvr/api/cameras.go`
- Create: `internal/nvr/api/recordings.go`
- Create: `internal/nvr/api/users.go`
- Create: `internal/nvr/api/system.go`
- Create: `internal/nvr/api/cameras_test.go`
- Create: `internal/nvr/api/recordings_test.go`
- Create: `internal/nvr/api/users_test.go`

- [ ] **Step 1: Write failing test for camera list endpoint**

Create `internal/nvr/api/cameras_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

func TestCameraList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newTestDB(t)

	// Create test cameras
	for _, name := range []string{"Front Door", "Garage"} {
		database.CreateCamera(&db.Camera{
			Name: name, RTSPURL: "rtsp://1.2.3.4/s", MediaMTXPath: "nvr/" + name,
		})
	}

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")
	os.WriteFile(yamlPath, []byte("paths:\n  all_others:\n"), 0644)

	h := &CameraHandler{DB: database, YAMLWriter: yamlwriter.New(yamlPath)}

	router := gin.New()
	router.GET("/cameras", h.List)

	req := httptest.NewRequest("GET", "/cameras", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp []*db.Camera
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Len(t, resp, 2)
}

func TestCameraCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newTestDB(t)

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "mediamtx.yml")
	os.WriteFile(yamlPath, []byte("paths:\n  all_others:\n"), 0644)

	h := &CameraHandler{DB: database, YAMLWriter: yamlwriter.New(yamlPath)}

	router := gin.New()
	router.POST("/cameras", h.Create)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "Front Door",
		"rtsp_url": "rtsp://192.168.1.100:554/stream1",
	})

	req := httptest.NewRequest("POST", "/cameras", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// Verify YAML was updated
	data, _ := os.ReadFile(yamlPath)
	require.Contains(t, string(data), "nvr/front-door")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/nvr/api/ -v -run TestCamera
```

Expected: FAIL

- [ ] **Step 3: Implement camera endpoints**

Create `internal/nvr/api/cameras.go`:

```go
package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

var pathSanitizer = regexp.MustCompile(`[^a-zA-Z0-9\-]`)

type CameraHandler struct {
	DB         *db.DB
	YAMLWriter *yamlwriter.Writer
}

type createCameraRequest struct {
	Name              string `json:"name" binding:"required"`
	RTSPURL           string `json:"rtsp_url" binding:"required"`
	ONVIFEndpoint     string `json:"onvif_endpoint"`
	ONVIFUsername      string `json:"onvif_username"`
	ONVIFPassword     string `json:"onvif_password"`
	ONVIFProfileToken string `json:"onvif_profile_token"`
	PTZCapable        bool   `json:"ptz_capable"`
	Record            bool   `json:"record"`
}

func (h *CameraHandler) List(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cameras"})
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}
	c.JSON(http.StatusOK, cameras)
}

func (h *CameraHandler) Get(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	c.JSON(http.StatusOK, cam)
}

func (h *CameraHandler) Create(c *gin.Context) {
	var req createCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	pathName := "nvr/" + sanitizePath(req.Name)

	cam := &db.Camera{
		Name:              req.Name,
		RTSPURL:           req.RTSPURL,
		ONVIFEndpoint:     req.ONVIFEndpoint,
		ONVIFUsername:      req.ONVIFUsername,
		ONVIFPassword:     req.ONVIFPassword,
		ONVIFProfileToken: req.ONVIFProfileToken,
		PTZCapable:        req.PTZCapable,
		MediaMTXPath:      pathName,
	}

	if err := h.DB.CreateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create camera"})
		return
	}

	// Write path to YAML
	pathConfig := map[string]interface{}{
		"source": req.RTSPURL,
	}
	if req.Record {
		pathConfig["record"] = true
	}
	if err := h.YAMLWriter.AddPath(pathName, pathConfig); err != nil {
		// Rollback camera creation
		h.DB.DeleteCamera(cam.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update config"})
		return
	}

	c.JSON(http.StatusCreated, cam)
}

func (h *CameraHandler) Update(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	var req createCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cam.Name = req.Name
	cam.RTSPURL = req.RTSPURL
	cam.ONVIFEndpoint = req.ONVIFEndpoint
	cam.ONVIFUsername = req.ONVIFUsername
	cam.ONVIFPassword = req.ONVIFPassword
	cam.ONVIFProfileToken = req.ONVIFProfileToken
	cam.PTZCapable = req.PTZCapable

	if err := h.DB.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update camera"})
		return
	}

	// Update YAML path
	pathConfig := map[string]interface{}{
		"source": req.RTSPURL,
	}
	if req.Record {
		pathConfig["record"] = true
	}
	h.YAMLWriter.AddPath(cam.MediaMTXPath, pathConfig)

	c.JSON(http.StatusOK, cam)
}

func (h *CameraHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	if err := h.DB.DeleteCamera(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete camera"})
		return
	}

	h.YAMLWriter.RemovePath(cam.MediaMTXPath)

	c.JSON(http.StatusOK, gin.H{"message": "camera deleted"})
}

func sanitizePath(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = pathSanitizer.ReplaceAllString(s, "")
	if s == "" {
		s = "camera"
	}
	return s
}
```

- [ ] **Step 4: Implement recording endpoints**

Create `internal/nvr/api/recordings.go`:

```go
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type RecordingHandler struct {
	DB *db.DB
}

func (h *RecordingHandler) Query(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id required"})
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time"})
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time"})
		return
	}

	recs, err := h.DB.QueryRecordings(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	if recs == nil {
		recs = []*db.Recording{}
	}
	c.JSON(http.StatusOK, recs)
}

func (h *RecordingHandler) Download(c *gin.Context) {
	// Serve the recording file directly
	// The implementer should look up the recording by ID,
	// verify the user has permission to the camera, then serve the file
	id := c.Param("id")
	_ = id
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not yet implemented"})
}

func (h *RecordingHandler) Export(c *gin.Context) {
	// Export a clip by concatenating/remuxing segments between start and end
	// This requires ffmpeg-style remuxing or using MediaMTX's existing playback muxer
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not yet implemented"})
}

func (h *RecordingHandler) Timeline(c *gin.Context) {
	cameraID := c.Query("camera_id")
	dateStr := c.Query("date")

	if cameraID == "" || dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id and date required"})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	start := date
	end := date.Add(24 * time.Hour)

	ranges, err := h.DB.GetTimeline(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	if ranges == nil {
		ranges = []db.TimeRange{}
	}
	c.JSON(http.StatusOK, ranges)
}
```

- [ ] **Step 5: Implement user management endpoints**

Create `internal/nvr/api/users.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type UserHandler struct {
	DB *db.DB
}

type createUserRequest struct {
	Username          string `json:"username" binding:"required"`
	Password          string `json:"password" binding:"required"`
	Role              string `json:"role" binding:"required"`
	CameraPermissions string `json:"camera_permissions"`
}

func (h *UserHandler) requireAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return false
	}
	return true
}

func (h *UserHandler) List(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	users, err := h.DB.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	if users == nil {
		users = []*db.User{}
	}
	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) Get(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	id := c.Param("id")
	user, err := h.DB.GetUser(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) Create(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.CameraPermissions == "" {
		req.CameraPermissions = `"*"`
	}

	user := &db.User{
		Username:          req.Username,
		PasswordHash:      hashPassword(req.Password),
		Role:              req.Role,
		CameraPermissions: req.CameraPermissions,
	}

	if err := h.DB.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) Update(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	id := c.Param("id")

	existing, err := h.DB.GetUser(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	existing.Username = req.Username
	existing.Role = req.Role
	if req.CameraPermissions != "" {
		existing.CameraPermissions = req.CameraPermissions
	}
	if req.Password != "" {
		existing.PasswordHash = hashPassword(req.Password)
		h.DB.RevokeAllUserTokens(id) // Force re-login on password change
	}

	if err := h.DB.UpdateUser(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

func (h *UserHandler) Delete(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	id := c.Param("id")

	// Prevent self-deletion
	userID, _ := c.Get("user_id")
	if userID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}

	if err := h.DB.DeleteUser(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
```

- [ ] **Step 6: Implement system endpoints**

Create `internal/nvr/api/system.go`:

```go
package api

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	Version   string
	StartedAt time.Time
}

func (h *SystemHandler) Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":  h.Version,
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
		"uptime":   time.Since(h.StartedAt).String(),
	})
}

func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SystemHandler) Storage(c *gin.Context) {
	// TODO: compute actual disk usage from recording paths
	c.JSON(http.StatusOK, gin.H{
		"total_bytes": 0,
		"used_bytes":  0,
		"free_bytes":  0,
	})
}

func (h *SystemHandler) Events(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	c.Writer.WriteString("event: connected\ndata: {}\n\n")
	c.Writer.Flush()

	// Keep connection open until client disconnects
	<-c.Request.Context().Done()
}
```

- [ ] **Step 7: Run all API tests**

```bash
go test ./internal/nvr/api/ -v
```

Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/recordings.go internal/nvr/api/users.go internal/nvr/api/system.go internal/nvr/api/cameras_test.go internal/nvr/api/recordings_test.go internal/nvr/api/users_test.go
git commit -m "feat(nvr): add camera, recording, user, and system API endpoints"
```

---

## Task 11: API Router & Core Wiring

**Files:**
- Create: `internal/nvr/api/router.go`
- Modify: `internal/nvr/nvr.go`
- Modify: `internal/core/core.go`
- Modify: `internal/api/api.go`

- [ ] **Step 1: Implement the router**

Create `internal/nvr/api/router.go`:

```go
package api

import (
	"crypto/rsa"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

type RouterConfig struct {
	DB         *db.DB
	PrivateKey *rsa.PrivateKey
	JWKSJSON   []byte
	YAMLWriter *yamlwriter.Writer
	Version    string
}

// RegisterRoutes registers all NVR API routes on the given gin engine.
func RegisterRoutes(engine *gin.Engine, cfg *RouterConfig) {
	auth := &AuthHandler{DB: cfg.DB, PrivateKey: cfg.PrivateKey}
	cameras := &CameraHandler{DB: cfg.DB, YAMLWriter: cfg.YAMLWriter}
	recordings := &RecordingHandler{DB: cfg.DB}
	users := &UserHandler{DB: cfg.DB}
	system := &SystemHandler{Version: cfg.Version}
	jwks := &JWKSHandler{JWKSJSON: cfg.JWKSJSON}
	middleware := &Middleware{PrivateKey: cfg.PrivateKey}

	nvr := engine.Group("/api/nvr")

	// Public routes (no auth required)
	nvr.POST("/auth/login", auth.Login)
	nvr.POST("/auth/setup", auth.Setup)
	nvr.GET("/.well-known/jwks.json", jwks.ServeJWKS)
	nvr.GET("/system/health", system.Health)

	// Protected routes
	protected := nvr.Group("")
	protected.Use(middleware.Handler())

	protected.POST("/auth/refresh", auth.Refresh)
	protected.POST("/auth/revoke", auth.Revoke)

	protected.GET("/cameras", cameras.List)
	protected.POST("/cameras", cameras.Create)
	protected.GET("/cameras/:id", cameras.Get)
	protected.PUT("/cameras/:id", cameras.Update)
	protected.DELETE("/cameras/:id", cameras.Delete)

	protected.GET("/recordings", recordings.Query)
	protected.GET("/recordings/:id/download", recordings.Download)
	protected.POST("/recordings/export", recordings.Export)
	protected.GET("/timeline", recordings.Timeline)

	protected.POST("/cameras/:id/ptz", cameras.PTZCommand)
	protected.GET("/cameras/:id/ptz/presets", cameras.PTZPresets)
	protected.GET("/cameras/:id/settings", cameras.GetSettings)
	protected.PUT("/cameras/:id/settings", cameras.UpdateSettings)

	protected.GET("/users", users.List)
	protected.POST("/users", users.Create)
	protected.GET("/users/:id", users.Get)
	protected.PUT("/users/:id", users.Update)
	protected.DELETE("/users/:id", users.Delete)

	protected.GET("/system/info", system.Info)
	protected.GET("/system/storage", system.Storage)
	protected.GET("/system/events", system.Events)
}
```

- [ ] **Step 2: Update NVR subsystem to expose router registration**

Add to `internal/nvr/nvr.go`:

```go
// RegisterRoutes registers NVR API routes on the given gin engine.
func (n *NVR) RegisterRoutes(engine *gin.Engine, version string) {
	api.RegisterRoutes(engine, &api.RouterConfig{
		DB:         n.db,
		PrivateKey: n.privateKey,
		JWKSJSON:   n.jwksJSON,
		YAMLWriter: n.yamlWriter,
		Version:    version,
	})
}
```

Add the import for the api package.

- [ ] **Step 3: Wire NVR routes into the existing API server**

Read `internal/api/api.go` to understand how the gin router is created. The NVR routes need to be registered on the same gin engine.

Approach: Add an optional NVR field to the API struct. If set, call its RegisterRoutes method during Initialize().

In `internal/api/api.go`, add to the API struct:

```go
NVRRouter func(*gin.Engine) // Optional: registers NVR routes if NVR is enabled
```

In the `Initialize()` method, after existing route registration and before `httpServer` creation:

```go
if a.NVRRouter != nil {
    a.NVRRouter(router)
}
```

In `internal/core/core.go`, when creating the API, set `NVRRouter` if NVR is enabled:

```go
if p.nvr != nil {
    apiConfig.NVRRouter = func(engine *gin.Engine) {
        p.nvr.RegisterRoutes(engine, p.conf.Version)
    }
}
```

(Adapt `apiConfig` to however the API struct is populated in core.go.)

- [ ] **Step 4: Run tests**

```bash
go test ./internal/nvr/... -v
go test ./internal/api/ -v -count=1
go test ./internal/core/ -v -count=1 -timeout 120s
```

Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/router.go internal/nvr/nvr.go internal/core/core.go internal/api/api.go
git commit -m "feat(nvr): wire NVR API routes into existing gin server"
```

---

## Task 12: React UI Scaffold

**Files:**
- Create: `ui/` (React project)
- Create: `internal/nvr/ui/embed.go`
- Create: `internal/nvr/ui/dist/.gitkeep`
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Scaffold React project**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
npm create vite@latest ui -- --template react-ts
cd ui && npm install
npm install react-router-dom
```

- [ ] **Step 2: Configure Vite for embedding**

Update `ui/vite.config.ts`:

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/nvr/ui/dist',
    emptyOutDir: true,
  },
})
```

- [ ] **Step 3: Create go:embed file**

Create `internal/nvr/ui/embed.go`:

```go
package ui

import "embed"

//go:embed all:dist
var DistFS embed.FS
```

Create `internal/nvr/ui/dist/.gitkeep` (empty file so the directory is tracked).

- [ ] **Step 4: Add .gitignore entries**

Add to `.gitignore`:

```
internal/nvr/ui/dist/*
!internal/nvr/ui/dist/.gitkeep
ui/node_modules/
```

- [ ] **Step 5: Add Makefile targets**

Add to `Makefile`:

```makefile
nvr-ui:
	cd ui && npm ci && npm run build
```

- [ ] **Step 6: Set up React Router with page stubs**

Replace `ui/src/App.tsx`:

```tsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<div>Login</div>} />
        <Route path="/setup" element={<div>Setup</div>} />
        <Route path="/live" element={<div>Live View</div>} />
        <Route path="/cameras" element={<div>Camera Management</div>} />
        <Route path="/recordings" element={<div>Recordings</div>} />
        <Route path="/settings" element={<div>Settings</div>} />
        <Route path="/users" element={<div>User Management</div>} />
        <Route path="/" element={<Navigate to="/live" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
```

- [ ] **Step 7: Build and verify embed works**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
make nvr-ui
go build ./...
```

Expected: Build succeeds with embedded UI

- [ ] **Step 8: Commit**

```bash
git add ui/ internal/nvr/ui/embed.go internal/nvr/ui/dist/.gitkeep Makefile .gitignore
git commit -m "feat(nvr): scaffold React UI with Vite, configure go:embed"
```

---

## Task 13: ONVIF Discovery & Device Management

**Files:**
- Create: `internal/nvr/onvif/discovery.go`
- Create: `internal/nvr/onvif/device.go`
- Create: `internal/nvr/onvif/media.go`
- Create: `internal/nvr/onvif/imaging.go`
- Create: `internal/nvr/onvif/ptz.go`
- Create: `internal/nvr/onvif/onvif_test.go`
- Modify: `go.mod`

- [ ] **Step 1: Add kerberos-io/onvif dependency**

```bash
go get github.com/kerberos-io/onvif
```

- [ ] **Step 2: Implement discovery**

Create `internal/nvr/onvif/discovery.go`:

```go
package onvif

import (
	"sync"

	"github.com/google/uuid"
	goonvif "github.com/kerberos-io/onvif"
	wsdiscovery "github.com/kerberos-io/onvif/ws-discovery"
)

type DiscoveredDevice struct {
	XAddr        string `json:"xaddr"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
}

type ScanStatus string

const (
	ScanStatusScanning ScanStatus = "scanning"
	ScanStatusComplete ScanStatus = "complete"
)

type ScanResult struct {
	ScanID  string             `json:"scan_id"`
	Status  ScanStatus         `json:"status"`
	Devices []DiscoveredDevice `json:"devices"`
}

type Discovery struct {
	mu      sync.Mutex
	current *ScanResult
}

func NewDiscovery() *Discovery {
	return &Discovery{}
}

func (d *Discovery) StartScan() (string, error) {
	d.mu.Lock()
	if d.current != nil && d.current.Status == ScanStatusScanning {
		d.mu.Unlock()
		return "", ErrScanInProgress
	}

	scanID := uuid.New().String()
	d.current = &ScanResult{
		ScanID: scanID,
		Status: ScanStatusScanning,
	}
	d.mu.Unlock()

	go d.runScan(scanID)
	return scanID, nil
}

func (d *Discovery) GetStatus() *ScanResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil {
		return nil
	}
	// Return a copy
	result := *d.current
	return &result
}

func (d *Discovery) GetResults() []DiscoveredDevice {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil {
		return nil
	}
	return d.current.Devices
}

func (d *Discovery) runScan(scanID string) {
	devices := wsdiscovery.SendProbe("", nil, []string{"dn:NetworkVideoTransmitter"}, map[string]string{"dn": "http://www.onvif.org/ver10/network/wsdl"})

	var discovered []DiscoveredDevice
	for _, xaddr := range devices {
		dev, err := goonvif.NewDevice(goonvif.DeviceParams{Xaddr: xaddr})
		if err != nil {
			continue
		}

		info, err := dev.GetDeviceInformation()
		if err != nil {
			discovered = append(discovered, DiscoveredDevice{XAddr: xaddr})
			continue
		}

		discovered = append(discovered, DiscoveredDevice{
			XAddr:        xaddr,
			Manufacturer: info.Manufacturer,
			Model:        info.Model,
			Firmware:     info.FirmwareVersion,
		})
	}

	d.mu.Lock()
	if d.current != nil && d.current.ScanID == scanID {
		d.current.Status = ScanStatusComplete
		d.current.Devices = discovered
	}
	d.mu.Unlock()
}
```

Note: The exact `kerberos-io/onvif` API may differ from what's shown above. The implementer should check the library's actual API and adapt. The key patterns (discovery probe, device info fetch) are correct but method signatures may need adjustment.

Create `internal/nvr/onvif/errors.go`:

```go
package onvif

import "errors"

var ErrScanInProgress = errors.New("scan already in progress")
```

- [ ] **Step 3: Implement device management stubs**

Create `internal/nvr/onvif/device.go`:

```go
package onvif

import (
	goonvif "github.com/kerberos-io/onvif"
)

type Device struct {
	dev *goonvif.Device
}

func NewDevice(xaddr, username, password string) (*Device, error) {
	dev, err := goonvif.NewDevice(goonvif.DeviceParams{
		Xaddr:    xaddr,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	return &Device{dev: dev}, nil
}

func (d *Device) GetStreamURI(profileToken string) (string, error) {
	// Use the ONVIF media service to get stream URI
	// Adapt based on kerberos-io/onvif API
	return "", nil // Placeholder — implement based on actual library API
}
```

Create `internal/nvr/onvif/media.go`, `internal/nvr/onvif/imaging.go`, `internal/nvr/onvif/ptz.go` as similar stubs. These will wrap the kerberos-io/onvif library calls for media profile management, imaging settings, and PTZ control respectively.

The implementer should:
1. Read the `kerberos-io/onvif` library documentation and examples
2. Implement each wrapper based on the actual ONVIF service SOAP calls
3. Test against real or simulated ONVIF cameras

- [ ] **Step 4: Write unit tests for discovery state machine**

Create `internal/nvr/onvif/onvif_test.go`:

```go
package onvif

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryInitialState(t *testing.T) {
	d := NewDiscovery()
	require.Nil(t, d.GetStatus())
	require.Nil(t, d.GetResults())
}

func TestDiscoveryConcurrentScanRejected(t *testing.T) {
	d := NewDiscovery()

	// Start first scan
	d.mu.Lock()
	d.current = &ScanResult{ScanID: "test", Status: ScanStatusScanning}
	d.mu.Unlock()

	// Second scan should fail
	_, err := d.StartScan()
	require.ErrorIs(t, err, ErrScanInProgress)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/nvr/onvif/ -v
```

Expected: State machine tests PASS

- [ ] **Step 6: Add ONVIF discovery endpoints to camera handler**

Add to `internal/nvr/api/cameras.go` (or create a separate `discover.go`):

```go
func (h *CameraHandler) Discover(c *gin.Context) {
	scanID, err := h.Discovery.StartScan()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"scan_id": scanID})
}

func (h *CameraHandler) DiscoverStatus(c *gin.Context) {
	status := h.Discovery.GetStatus()
	if status == nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle"})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *CameraHandler) DiscoverResults(c *gin.Context) {
	results := h.Discovery.GetResults()
	if results == nil {
		results = []onvif.DiscoveredDevice{}
	}
	c.JSON(http.StatusOK, results)
}
```

Add the Discovery field to CameraHandler and wire up routes in router.go.

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/onvif/ go.mod go.sum
git commit -m "feat(nvr): add ONVIF discovery, device management, and PTZ stubs"
```

---

## Task 14: Recording Hook Integration

**Files:**
- Modify: `internal/nvr/nvr.go`
- Modify: `internal/core/core.go`

- [ ] **Step 1: Add recording callback method to NVR**

Add to `internal/nvr/nvr.go`:

```go
// OnSegmentComplete matches the recorder.OnSegmentCompleteFunc signature:
//   func(path string, duration time.Duration)
// where `path` is the file path of the completed segment.
// We derive camera identity from the file path (which contains the mediamtx path name),
// file size via os.Stat, and format from the file extension.
func (n *NVR) OnSegmentComplete(filePath string, duration time.Duration) {
	// Extract the mediamtx path name from the file path
	// Recording paths follow the pattern: {recordPath}/{pathName}/{timestamp}.{ext}
	// We search for a camera whose mediamtx_path is a substring of the file path
	cameras, err := n.db.ListCameras()
	if err != nil {
		return
	}

	var cam *db.Camera
	for _, c := range cameras {
		if strings.Contains(filePath, c.MediaMTXPath) {
			cam = c
			break
		}
	}
	if cam == nil {
		return // Not an NVR-managed camera
	}

	// Get file size
	var fileSize int64
	if info, err := os.Stat(filePath); err == nil {
		fileSize = info.Size()
	}

	// Determine format from extension
	format := "fmp4"
	if strings.HasSuffix(filePath, ".ts") {
		format = "mpegts"
	}

	n.db.InsertRecording(&db.Recording{
		CameraID:   cam.ID,
		StartTime:  time.Now().Add(-duration).UTC(),
		EndTime:    time.Now().UTC(),
		DurationMs: duration.Milliseconds(),
		FilePath:   filePath,
		FileSize:   fileSize,
		Format:     format,
	})
}

// OnSegmentDelete is called by the record cleaner when a segment is removed.
func (n *NVR) OnSegmentDelete(filePath string) {
	n.db.DeleteRecordingByPath(filePath)
}
```

- [ ] **Step 2: Wire callbacks into Core**

In `internal/core/core.go` (or `internal/core/path.go` where recorders are created), wrap the existing `OnSegmentComplete` callback to also notify the NVR:

```go
// In the section where a Recorder is created for a path:
if p.nvr != nil {
    originalOnComplete := recorder.OnSegmentComplete
    recorder.OnSegmentComplete = func(filePath string, duration time.Duration) {
        if originalOnComplete != nil {
            originalOnComplete(filePath, duration)
        }
        p.nvr.OnSegmentComplete(filePath, duration)
    }
}
```

For the record cleaner:

```go
if p.nvr != nil {
    p.recordCleaner.OnSegmentDelete = p.nvr.OnSegmentDelete
}
```

Note: The exact wiring depends on how `core.go` creates the recorder and cleaner instances. The implementer should read the current creation points and adapt. The recorder callbacks are set in `internal/core/path.go` when creating recorder instances.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/nvr/ -v
go test ./internal/core/ -v -count=1 -timeout 120s
```

Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go internal/core/core.go
git commit -m "feat(nvr): wire recording segment callbacks to NVR database"
```

---

## Task 15: UI Serving & Catchall Route

**Files:**
- Modify: `internal/nvr/api/router.go`
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Add static file serving to the router**

Update `internal/nvr/api/router.go` to serve the embedded React app:

```go
import (
	"io/fs"
	"net/http"

	nvrui "github.com/bluenviron/mediamtx/internal/nvr/ui"
)

// In RegisterRoutes, add after API routes:

// Serve embedded React UI
distFS, err := fs.Sub(nvrui.DistFS, "dist")
if err == nil {
    fileServer := http.FileServer(http.FS(distFS))
    engine.NoRoute(func(c *gin.Context) {
        // Try to serve static file first
        path := c.Request.URL.Path
        f, err := distFS.Open(path[1:]) // strip leading /
        if err == nil {
            f.Close()
            fileServer.ServeHTTP(c.Writer, c.Request)
            return
        }
        // Fallback to index.html for client-side routing
        c.Request.URL.Path = "/"
        fileServer.ServeHTTP(c.Writer, c.Request)
    })
}
```

- [ ] **Step 2: Build UI and test**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
make nvr-ui
go build ./...
```

Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(nvr): serve embedded React UI with SPA fallback routing"
```

---

## Task 16: React UI — Auth Flow & API Client

**Files:**
- Create: `ui/src/api/client.ts`
- Create: `ui/src/auth/context.tsx`
- Create: `ui/src/hooks/useAuth.ts`
- Create: `ui/src/pages/Login.tsx`
- Create: `ui/src/pages/Setup.tsx`
- Modify: `ui/src/App.tsx`

- [ ] **Step 1: Create API client with JWT refresh**

Create `ui/src/api/client.ts`:

```ts
let accessToken: string | null = null

export function setAccessToken(token: string | null) {
  accessToken = token
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  const headers = new Headers(options.headers)
  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`)
  }
  headers.set('Content-Type', 'application/json')

  let res = await fetch(`/api/nvr${path}`, { ...options, headers, credentials: 'include' })

  if (res.status === 401 && accessToken) {
    // Try refresh
    const refreshRes = await fetch('/api/nvr/auth/refresh', {
      method: 'POST',
      credentials: 'include',
    })
    if (refreshRes.ok) {
      const data = await refreshRes.json()
      accessToken = data.access_token
      headers.set('Authorization', `Bearer ${accessToken}`)
      res = await fetch(`/api/nvr${path}`, { ...options, headers, credentials: 'include' })
    } else {
      accessToken = null
      window.location.href = '/login'
    }
  }

  return res
}
```

- [ ] **Step 2: Create auth context**

Create `ui/src/auth/context.tsx`:

```tsx
import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { setAccessToken, apiFetch } from '../api/client'

interface AuthState {
  isAuthenticated: boolean
  isLoading: boolean
  user: { id: string; username: string; role: string } | null
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  setupRequired: boolean
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [user, setUser] = useState<AuthState['user']>(null)
  const [setupRequired, setSetupRequired] = useState(false)

  useEffect(() => {
    // Try to refresh token on mount
    fetch('/api/nvr/auth/refresh', { method: 'POST', credentials: 'include' })
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setAccessToken(data.access_token)
          setIsAuthenticated(true)
          // Decode JWT to get user info (payload is base64)
          const payload = JSON.parse(atob(data.access_token.split('.')[1]))
          setUser({ id: payload.sub, username: payload.username, role: payload.role })
        } else {
          // Check if setup is needed
          const healthRes = await fetch('/api/nvr/system/health')
          if (healthRes.status === 503) {
            setSetupRequired(true)
          }
        }
      })
      .catch(() => {})
      .finally(() => setIsLoading(false))
  }, [])

  const login = async (username: string, password: string) => {
    const res = await fetch('/api/nvr/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
      credentials: 'include',
    })
    if (!res.ok) throw new Error('Invalid credentials')
    const data = await res.json()
    setAccessToken(data.access_token)
    const payload = JSON.parse(atob(data.access_token.split('.')[1]))
    setUser({ id: payload.sub, username: payload.username, role: payload.role })
    setIsAuthenticated(true)
  }

  const logout = async () => {
    await apiFetch('/auth/revoke', { method: 'POST' })
    setAccessToken(null)
    setUser(null)
    setIsAuthenticated(false)
  }

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, user, login, logout, setupRequired }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
```

- [ ] **Step 3: Create Login and Setup pages**

Create `ui/src/pages/Login.tsx`:

```tsx
import { useState, FormEvent } from 'react'
import { useAuth } from '../auth/context'
import { useNavigate } from 'react-router-dom'

export default function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const { login } = useAuth()
  const navigate = useNavigate()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      await login(username, password)
      navigate('/live')
    } catch {
      setError('Invalid credentials')
    }
  }

  return (
    <div style={{ maxWidth: 400, margin: '100px auto', padding: 20 }}>
      <h1>MediaMTX NVR</h1>
      <form onSubmit={handleSubmit}>
        <div>
          <label>Username</label>
          <input type="text" value={username} onChange={e => setUsername(e.target.value)} required />
        </div>
        <div>
          <label>Password</label>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
        </div>
        {error && <p style={{ color: 'red' }}>{error}</p>}
        <button type="submit">Login</button>
      </form>
    </div>
  )
}
```

Create `ui/src/pages/Setup.tsx`:

```tsx
import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/context'

export default function Setup() {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const { login } = useAuth()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (password !== confirm) {
      setError('Passwords do not match')
      return
    }
    const res = await fetch('/api/nvr/auth/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })
    if (!res.ok) {
      setError('Setup failed')
      return
    }
    await login(username, password)
    navigate('/live')
  }

  return (
    <div style={{ maxWidth: 400, margin: '100px auto', padding: 20 }}>
      <h1>MediaMTX NVR Setup</h1>
      <p>Create your admin account to get started.</p>
      <form onSubmit={handleSubmit}>
        <div>
          <label>Username</label>
          <input type="text" value={username} onChange={e => setUsername(e.target.value)} required />
        </div>
        <div>
          <label>Password</label>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
        </div>
        <div>
          <label>Confirm Password</label>
          <input type="password" value={confirm} onChange={e => setConfirm(e.target.value)} required />
        </div>
        {error && <p style={{ color: 'red' }}>{error}</p>}
        <button type="submit">Create Admin Account</button>
      </form>
    </div>
  )
}
```

- [ ] **Step 4: Update App.tsx with auth routing**

```tsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/context'
import Login from './pages/Login'
import Setup from './pages/Setup'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, setupRequired } = useAuth()
  if (isLoading) return <div>Loading...</div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/setup" element={<Setup />} />
      <Route path="/live" element={<ProtectedRoute><div>Live View</div></ProtectedRoute>} />
      <Route path="/cameras" element={<ProtectedRoute><div>Camera Management</div></ProtectedRoute>} />
      <Route path="/recordings" element={<ProtectedRoute><div>Recordings</div></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><div>Settings</div></ProtectedRoute>} />
      <Route path="/users" element={<ProtectedRoute><div>User Management</div></ProtectedRoute>} />
      <Route path="/" element={<Navigate to="/live" replace />} />
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}
```

- [ ] **Step 5: Build and verify**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build
```

Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add ui/src/
git commit -m "feat(nvr): add auth flow, API client with JWT refresh, login and setup pages"
```

---

## Task 17: React UI — Live View with Camera Grid

**Files:**
- Create: `ui/src/pages/LiveView.tsx`
- Create: `ui/src/components/CameraGrid.tsx`
- Create: `ui/src/components/PlayerCell.tsx`
- Create: `ui/src/hooks/useCameras.ts`

- [ ] **Step 1: Create camera data hook**

Create `ui/src/hooks/useCameras.ts`:

```ts
import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

export interface Camera {
  id: string
  name: string
  rtsp_url: string
  mediamtx_path: string
  status: string
  ptz_capable: boolean
}

export function useCameras() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    const res = await apiFetch('/cameras')
    if (res.ok) setCameras(await res.json())
    setLoading(false)
  }

  useEffect(() => { refresh() }, [])

  return { cameras, loading, refresh }
}
```

- [ ] **Step 2: Create player cell component**

Create `ui/src/components/PlayerCell.tsx`:

```tsx
import { useRef, useEffect } from 'react'
import { Camera } from '../hooks/useCameras'

interface Props {
  camera: Camera
  onSelect?: () => void
}

export default function PlayerCell({ camera, onSelect }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    // Use MediaMTX's existing WebRTC or HLS endpoints for playback
    // WebRTC: /api/v3/whep/${camera.mediamtx_path}
    // HLS: /hls/${camera.mediamtx_path}/index.m3u8
    // For v1, start with HLS as it's simpler
    if (videoRef.current) {
      videoRef.current.src = `/${camera.mediamtx_path}`
    }
  }, [camera.mediamtx_path])

  return (
    <div onClick={onSelect} style={{
      position: 'relative',
      background: '#000',
      aspectRatio: '16/9',
      cursor: 'pointer',
    }}>
      <video ref={videoRef} autoPlay muted playsInline style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
      <div style={{
        position: 'absolute', bottom: 4, left: 4,
        background: 'rgba(0,0,0,0.6)', color: '#fff',
        padding: '2px 8px', fontSize: 12, borderRadius: 4,
      }}>
        {camera.name}
        <span style={{ marginLeft: 8, color: camera.status === 'online' ? '#4f4' : '#f44' }}>
          {camera.status}
        </span>
      </div>
    </div>
  )
}
```

Note: The actual stream playback integration depends on which MediaMTX endpoint is used (HLS vs WebRTC). The implementer should integrate with `hls.js` for HLS playback or MediaMTX's WebRTC WHEP endpoint. This stub shows the component structure.

- [ ] **Step 3: Create camera grid component**

Create `ui/src/components/CameraGrid.tsx`:

```tsx
import { Camera } from '../hooks/useCameras'
import PlayerCell from './PlayerCell'

interface Props {
  cameras: Camera[]
  layout: number // grid columns (1, 2, 3, 4)
  onSelectCamera?: (camera: Camera) => void
}

export default function CameraGrid({ cameras, layout, onSelectCamera }: Props) {
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: `repeat(${layout}, 1fr)`,
      gap: 4,
      width: '100%',
    }}>
      {cameras.map(cam => (
        <PlayerCell
          key={cam.id}
          camera={cam}
          onSelect={() => onSelectCamera?.(cam)}
        />
      ))}
    </div>
  )
}
```

- [ ] **Step 4: Create Live View page**

Create `ui/src/pages/LiveView.tsx`:

```tsx
import { useState } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import CameraGrid from '../components/CameraGrid'

export default function LiveView() {
  const { cameras, loading } = useCameras()
  const [layout, setLayout] = useState(2)
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  if (loading) return <div>Loading cameras...</div>
  if (cameras.length === 0) return <div>No cameras configured. Go to Camera Management to add cameras.</div>

  if (selectedCamera) {
    return (
      <div>
        <button onClick={() => setSelectedCamera(null)}>Back to Grid</button>
        <h2>{selectedCamera.name}</h2>
        <div style={{ maxWidth: '100%', aspectRatio: '16/9' }}>
          {/* Full-size player */}
          <video autoPlay muted playsInline style={{ width: '100%', height: '100%' }} />
        </div>
      </div>
    )
  }

  return (
    <div>
      <div style={{ marginBottom: 8 }}>
        <span>Layout: </span>
        {[1, 2, 3, 4].map(n => (
          <button key={n} onClick={() => setLayout(n)}
            style={{ fontWeight: layout === n ? 'bold' : 'normal', marginRight: 4 }}>
            {n}x{n}
          </button>
        ))}
      </div>
      <CameraGrid cameras={cameras} layout={layout} onSelectCamera={setSelectedCamera} />
    </div>
  )
}
```

- [ ] **Step 5: Wire into App.tsx**

Replace the Live View stub route with the actual component:

```tsx
import LiveView from './pages/LiveView'
// ...
<Route path="/live" element={<ProtectedRoute><LiveView /></ProtectedRoute>} />
```

- [ ] **Step 6: Build and verify**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build
```

Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add ui/src/
git commit -m "feat(nvr): add live view with multi-camera grid and layout selector"
```

---

## Task 18: React UI — Camera Management Page

**Files:**
- Create: `ui/src/pages/CameraManagement.tsx`

- [ ] **Step 1: Implement camera management page**

Create `ui/src/pages/CameraManagement.tsx`:

```tsx
import { useState, FormEvent } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import { apiFetch } from '../api/client'

export default function CameraManagement() {
  const { cameras, loading, refresh } = useCameras()
  const [showAdd, setShowAdd] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<any[]>([])

  const handleDiscover = async () => {
    setDiscovering(true)
    const res = await apiFetch('/cameras/discover', { method: 'POST' })
    if (!res.ok) {
      setDiscovering(false)
      return
    }

    // Poll for results
    const poll = setInterval(async () => {
      const statusRes = await apiFetch('/cameras/discover/status')
      if (statusRes.ok) {
        const data = await statusRes.json()
        if (data.status === 'complete') {
          clearInterval(poll)
          const resultsRes = await apiFetch('/cameras/discover/results')
          if (resultsRes.ok) setDiscovered(await resultsRes.json())
          setDiscovering(false)
        }
      }
    }, 2000)
  }

  const handleAddCamera = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const formData = new FormData(e.currentTarget)
    const res = await apiFetch('/cameras', {
      method: 'POST',
      body: JSON.stringify({
        name: formData.get('name'),
        rtsp_url: formData.get('rtsp_url'),
        record: formData.get('record') === 'on',
      }),
    })
    if (res.ok) {
      setShowAdd(false)
      refresh()
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this camera?')) return
    await apiFetch(`/cameras/${id}`, { method: 'DELETE' })
    refresh()
  }

  if (loading) return <div>Loading...</div>

  return (
    <div>
      <h1>Camera Management</h1>

      <div style={{ marginBottom: 16 }}>
        <button onClick={() => setShowAdd(!showAdd)}>Add Camera</button>
        <button onClick={handleDiscover} disabled={discovering} style={{ marginLeft: 8 }}>
          {discovering ? 'Scanning...' : 'Discover ONVIF Cameras'}
        </button>
      </div>

      {discovered.length > 0 && (
        <div style={{ marginBottom: 16, padding: 12, border: '1px solid #ccc' }}>
          <h3>Discovered Cameras</h3>
          {discovered.map((d, i) => (
            <div key={i} style={{ marginBottom: 8 }}>
              <strong>{d.manufacturer} {d.model}</strong> — {d.xaddr}
              <button onClick={() => {
                setShowAdd(true)
                // Pre-fill form (simplified — actual implementation would set form state)
              }} style={{ marginLeft: 8 }}>Add</button>
            </div>
          ))}
        </div>
      )}

      {showAdd && (
        <form onSubmit={handleAddCamera} style={{ marginBottom: 16, padding: 12, border: '1px solid #ccc' }}>
          <h3>Add Camera</h3>
          <div><label>Name</label><input name="name" required /></div>
          <div><label>RTSP URL</label><input name="rtsp_url" required /></div>
          <div><label><input name="record" type="checkbox" /> Enable Recording</label></div>
          <button type="submit">Add</button>
        </form>
      )}

      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th>Name</th><th>Status</th><th>RTSP URL</th><th>PTZ</th><th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {cameras.map(cam => (
            <tr key={cam.id}>
              <td>{cam.name}</td>
              <td style={{ color: cam.status === 'online' ? 'green' : 'red' }}>{cam.status}</td>
              <td>{cam.rtsp_url}</td>
              <td>{cam.ptz_capable ? 'Yes' : 'No'}</td>
              <td>
                <button onClick={() => handleDelete(cam.id)}>Delete</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

- [ ] **Step 2: Wire into App.tsx and build**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build
```

Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/CameraManagement.tsx ui/src/App.tsx
git commit -m "feat(nvr): add camera management page with ONVIF discovery"
```

---

## Task 19: React UI — Recordings Browser with Timeline

**Files:**
- Create: `ui/src/pages/Recordings.tsx`
- Create: `ui/src/components/Timeline.tsx`
- Create: `ui/src/hooks/useRecordings.ts`

- [ ] **Step 1: Create recordings data hook**

Create `ui/src/hooks/useRecordings.ts`:

```ts
import { useState } from 'react'
import { apiFetch } from '../api/client'

export interface TimeRange {
  start: string
  end: string
}

export function useTimeline(cameraId: string | null, date: string) {
  const [ranges, setRanges] = useState<TimeRange[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    if (!cameraId || !date) return
    setLoading(true)
    const res = await apiFetch(`/timeline?camera_id=${cameraId}&date=${date}`)
    if (res.ok) setRanges(await res.json())
    setLoading(false)
  }

  return { ranges, loading, load }
}
```

- [ ] **Step 2: Create timeline component**

Create `ui/src/components/Timeline.tsx`:

```tsx
import { TimeRange } from '../hooks/useRecordings'

interface Props {
  ranges: TimeRange[]
  date: string
  onSeek?: (time: Date) => void
}

export default function Timeline({ ranges, date, onSeek }: Props) {
  const dayStart = new Date(date + 'T00:00:00Z')
  const dayMs = 24 * 60 * 60 * 1000

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const pct = (e.clientX - rect.left) / rect.width
    const time = new Date(dayStart.getTime() + pct * dayMs)
    onSeek?.(time)
  }

  return (
    <div onClick={handleClick} style={{
      position: 'relative', width: '100%', height: 40,
      background: '#222', borderRadius: 4, cursor: 'crosshair',
      overflow: 'hidden',
    }}>
      {ranges.map((r, i) => {
        const start = new Date(r.start).getTime() - dayStart.getTime()
        const end = new Date(r.end).getTime() - dayStart.getTime()
        const left = (start / dayMs) * 100
        const width = ((end - start) / dayMs) * 100
        return (
          <div key={i} style={{
            position: 'absolute', top: 0, bottom: 0,
            left: `${left}%`, width: `${width}%`,
            background: '#4a9eff', opacity: 0.7,
          }} />
        )
      })}
      {/* Hour markers */}
      {Array.from({ length: 24 }, (_, h) => (
        <div key={h} style={{
          position: 'absolute', top: 0, bottom: 0,
          left: `${(h / 24) * 100}%`,
          borderLeft: '1px solid rgba(255,255,255,0.2)',
          fontSize: 9, color: '#888', paddingLeft: 2,
        }}>
          {h}:00
        </div>
      ))}
    </div>
  )
}
```

- [ ] **Step 3: Create recordings page**

Create `ui/src/pages/Recordings.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { useCameras } from '../hooks/useCameras'
import { useTimeline } from '../hooks/useRecordings'
import Timeline from '../components/Timeline'

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(null)
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const { ranges, loading: timelineLoading, load } = useTimeline(selectedCamera, date)

  useEffect(() => {
    if (selectedCamera && date) load()
  }, [selectedCamera, date])

  if (camerasLoading) return <div>Loading...</div>

  return (
    <div>
      <h1>Recordings</h1>

      <div style={{ marginBottom: 16 }}>
        <select value={selectedCamera || ''} onChange={e => setSelectedCamera(e.target.value || null)}>
          <option value="">Select Camera</option>
          {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <input type="date" value={date} onChange={e => setDate(e.target.value)} style={{ marginLeft: 8 }} />
      </div>

      {selectedCamera && (
        <>
          <Timeline ranges={ranges} date={date} onSeek={(time) => {
            console.log('Seek to:', time)
            // TODO: integrate with playback
          }} />
          {timelineLoading && <p>Loading timeline...</p>}
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Wire into App.tsx and build**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build
```

Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/Recordings.tsx ui/src/components/Timeline.tsx ui/src/hooks/useRecordings.ts ui/src/App.tsx
git commit -m "feat(nvr): add recordings browser with timeline visualization"
```

---

## Task 20: React UI — Settings & User Management Pages

**Files:**
- Create: `ui/src/pages/Settings.tsx`
- Create: `ui/src/pages/UserManagement.tsx`

- [ ] **Step 1: Create Settings page**

Create `ui/src/pages/Settings.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

export default function Settings() {
  const [systemInfo, setSystemInfo] = useState<any>(null)

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  return (
    <div>
      <h1>Settings</h1>

      <h2>System Information</h2>
      {systemInfo ? (
        <table>
          <tbody>
            <tr><td>Version</td><td>{systemInfo.version}</td></tr>
            <tr><td>Platform</td><td>{systemInfo.platform}</td></tr>
            <tr><td>Uptime</td><td>{systemInfo.uptime}</td></tr>
          </tbody>
        </table>
      ) : <p>Loading...</p>}
    </div>
  )
}
```

- [ ] **Step 2: Create User Management page**

Create `ui/src/pages/UserManagement.tsx`:

```tsx
import { useState, useEffect, FormEvent } from 'react'
import { apiFetch } from '../api/client'

interface User {
  id: string
  username: string
  role: string
  camera_permissions: string
}

export default function UserManagement() {
  const [users, setUsers] = useState<User[]>([])
  const [showAdd, setShowAdd] = useState(false)

  const refresh = async () => {
    const res = await apiFetch('/users')
    if (res.ok) setUsers(await res.json())
  }

  useEffect(() => { refresh() }, [])

  const handleAdd = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const formData = new FormData(e.currentTarget)
    await apiFetch('/users', {
      method: 'POST',
      body: JSON.stringify({
        username: formData.get('username'),
        password: formData.get('password'),
        role: formData.get('role'),
      }),
    })
    setShowAdd(false)
    refresh()
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this user?')) return
    await apiFetch(`/users/${id}`, { method: 'DELETE' })
    refresh()
  }

  return (
    <div>
      <h1>User Management</h1>
      <button onClick={() => setShowAdd(!showAdd)}>Add User</button>

      {showAdd && (
        <form onSubmit={handleAdd} style={{ margin: '16px 0', padding: 12, border: '1px solid #ccc' }}>
          <div><label>Username</label><input name="username" required /></div>
          <div><label>Password</label><input name="password" type="password" required /></div>
          <div>
            <label>Role</label>
            <select name="role">
              <option value="viewer">Viewer</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          <button type="submit">Create</button>
        </form>
      )}

      <table style={{ width: '100%', marginTop: 16 }}>
        <thead><tr><th>Username</th><th>Role</th><th>Actions</th></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td>{u.username}</td>
              <td>{u.role}</td>
              <td><button onClick={() => handleDelete(u.id)}>Delete</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

- [ ] **Step 3: Wire all pages into App.tsx with navigation**

Update `ui/src/App.tsx` to include a navigation bar and all page imports. Add a simple nav component at the top of protected routes.

- [ ] **Step 4: Build and verify**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build
```

Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add ui/src/
git commit -m "feat(nvr): add settings and user management pages"
```

---

## Task 21: End-to-End Build Verification

**Files:**
- Modify: `scripts/binaries.mk`

- [ ] **Step 1: Update binaries.mk to include UI build**

Read `scripts/binaries.mk` and add the UI build step before the Go compilation. Since the build uses Docker, the Node.js build needs to happen in a Docker stage or as a pre-step.

Add a Docker build stage for the UI:

```dockerfile
FROM node:20-alpine AS build-ui
WORKDIR /app
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build
```

Then in the Go build stage, copy the UI output:

```dockerfile
COPY --from=build-ui /app/../internal/nvr/ui/dist/ /s/internal/nvr/ui/dist/
```

- [ ] **Step 2: Run full build**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
make nvr-ui
go build -o tmp/mediamtx ./
```

Expected: Single binary builds successfully

- [ ] **Step 3: Smoke test**

```bash
# Start with NVR enabled
cat > /tmp/mediamtx-test.yml << 'EOF'
nvr: yes
nvrDatabase: /tmp/mediamtx-nvr-test.db
paths:
  all_others:
EOF

./tmp/mediamtx /tmp/mediamtx-test.yml &
PID=$!

# Wait for startup
sleep 2

# Test health endpoint
curl -s http://localhost:9997/api/nvr/system/health
# Expected: {"status":"ok"}

# Test JWKS endpoint
curl -s http://localhost:9997/api/nvr/.well-known/jwks.json
# Expected: {"keys":[...]}

# Test setup endpoint
curl -s -X POST http://localhost:9997/api/nvr/auth/setup \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"testpass"}'
# Expected: {"id":"...","username":"admin"}

# Test login
curl -s -X POST http://localhost:9997/api/nvr/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"testpass"}'
# Expected: {"access_token":"..."}

# Cleanup
kill $PID
rm /tmp/mediamtx-nvr-test.db /tmp/mediamtx-test.yml
```

- [ ] **Step 4: Commit build system changes**

```bash
git add scripts/binaries.mk Makefile
git commit -m "feat(nvr): update build system to include React UI in binary"
```

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -count=1 -timeout 300s
```

Expected: All tests PASS

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "feat(nvr): complete NVR v1 implementation"
```
