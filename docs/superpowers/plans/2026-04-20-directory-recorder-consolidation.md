# Directory/Recorder Architecture Consolidation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose `internal/nvr/` (63K LOC, 176 files) into `internal/directory/`, `internal/recorder/`, and `internal/shared/`, removing legacy mode and establishing clean control-plane/data-plane separation.

**Architecture:** The single monolithic NVR package is split along responsibility boundaries: Directory (control plane — cameras, users, auth, schedules, federation), Recorder (data plane — capture, store, serve, detect), and Shared (cross-cutting utilities). Legacy mode is removed; default empty `mode` becomes all-in-one. The Flutter client gains Recorder endpoint routing for direct data-plane access.

**Tech Stack:** Go 1.22+, SQLite via `modernc.org/sqlite`, Gin HTTP router, Connect-Go protos, Flutter/Dart with Riverpod, Freezed

**Spec:** `docs/superpowers/specs/2026-04-20-directory-recorder-consolidation-design.md`

---

## Phase 1: Move Shared Utility Packages (6 packages, ~3,700 LOC)

These are self-contained utility packages with zero NVR-internal dependencies. Pure file moves with import path updates.

### Task 1: Move `nvr/syscheck/` → `shared/syscheck/`

**Files:**
- Move: `internal/nvr/syscheck/*.go` (4 files, 458 LOC) → `internal/shared/syscheck/`
- Modify: any files importing `internal/nvr/syscheck` (update import paths)

- [ ] **Step 1: Create destination and move files**

```bash
mkdir -p internal/shared/syscheck
git mv internal/nvr/syscheck/*.go internal/shared/syscheck/
```

- [ ] **Step 2: Update package declaration in all moved files**

In each `.go` file in `internal/shared/syscheck/`, the package declaration should already be `package syscheck` — verify this is correct. No change needed if already `syscheck`.

- [ ] **Step 3: Update all import paths**

Find all files importing the old path and update:

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/syscheck"' internal/ | head -20
```

For each file found, replace:
```
"github.com/bluenviron/mediamtx/internal/nvr/syscheck"
```
with:
```
"github.com/bluenviron/mediamtx/internal/shared/syscheck"
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./internal/shared/syscheck/...
go build ./internal/...
```

Expected: clean compilation, no errors.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/shared/syscheck/... -v
```

Expected: all tests pass (or no tests exist — either is fine for utility packages).

- [ ] **Step 6: Commit**

```bash
git add -A internal/shared/syscheck/ internal/nvr/syscheck/
git commit -m "refactor: move nvr/syscheck to shared/syscheck

Part of NVR decomposition. Pure file move with import path updates."
```

---

### Task 2: Move `nvr/logmgr/` → `shared/logmgr/`

**Files:**
- Move: `internal/nvr/logmgr/*.go` (2 files, 614 LOC) → `internal/shared/logmgr/`

- [ ] **Step 1: Create destination and move files**

```bash
mkdir -p internal/shared/logmgr
git mv internal/nvr/logmgr/*.go internal/shared/logmgr/
```

- [ ] **Step 2: Update all import paths**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/logmgr"' internal/
```

For each file, replace `internal/nvr/logmgr` with `internal/shared/logmgr`.

- [ ] **Step 3: Verify compilation and run tests**

```bash
go build ./internal/shared/logmgr/...
go build ./internal/...
go test ./internal/shared/logmgr/... -v
```

Expected: clean build, tests pass.

- [ ] **Step 4: Commit**

```bash
git add -A internal/shared/logmgr/ internal/nvr/logmgr/
git commit -m "refactor: move nvr/logmgr to shared/logmgr"
```

---

### Task 3: Move `nvr/diagnostics/` → `shared/diagnostics/`

**Files:**
- Move: `internal/nvr/diagnostics/*.go` (7 files, 1,596 LOC) → `internal/shared/diagnostics/`

- [ ] **Step 1: Create destination and move files**

```bash
mkdir -p internal/shared/diagnostics
git mv internal/nvr/diagnostics/*.go internal/shared/diagnostics/
```

- [ ] **Step 2: Update all import paths**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/diagnostics"' internal/
```

Replace `internal/nvr/diagnostics` with `internal/shared/diagnostics` in each file.

- [ ] **Step 3: Update internal imports within diagnostics files**

Check if any diagnostics files import other nvr packages (e.g., `nvr/db`). If so, note these — they will be updated in later tasks when those packages move. For now, leave cross-nvr imports as-is if they still compile.

- [ ] **Step 4: Verify compilation and run tests**

```bash
go build ./internal/shared/diagnostics/...
go build ./internal/...
go test ./internal/shared/diagnostics/... -v
```

- [ ] **Step 5: Commit**

```bash
git add -A internal/shared/diagnostics/ internal/nvr/diagnostics/
git commit -m "refactor: move nvr/diagnostics to shared/diagnostics"
```

---

### Task 4: Move `nvr/metrics/` → `shared/metrics/`

**Files:**
- Move: `internal/nvr/metrics/*.go` (1 file, 225 LOC) → `internal/shared/metrics/`

- [ ] **Step 1: Move files**

```bash
mkdir -p internal/shared/metrics
git mv internal/nvr/metrics/*.go internal/shared/metrics/
```

- [ ] **Step 2: Update import paths, verify build, commit**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/metrics"' internal/
# Update each file's import path
go build ./internal/...
git add -A internal/shared/metrics/ internal/nvr/metrics/
git commit -m "refactor: move nvr/metrics to shared/metrics"
```

---

### Task 5: Move `nvr/updater/` → `shared/updater/`

**Files:**
- Move: `internal/nvr/updater/*.go` (1 file, 509 LOC) → `internal/shared/updater/`

- [ ] **Step 1: Move files**

```bash
mkdir -p internal/shared/updater
git mv internal/nvr/updater/*.go internal/shared/updater/
```

- [ ] **Step 2: Update import paths, verify build, commit**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/updater"' internal/
# Update each file's import path
go build ./internal/...
git add -A internal/shared/updater/ internal/nvr/updater/
git commit -m "refactor: move nvr/updater to shared/updater"
```

---

### Task 6: Move `nvr/hwaccel/` → `shared/hwaccel/`

**Files:**
- Move: `internal/nvr/hwaccel/*.go` (1 file, 338 LOC) → `internal/shared/hwaccel/`

- [ ] **Step 1: Move files**

```bash
mkdir -p internal/shared/hwaccel
git mv internal/nvr/hwaccel/*.go internal/shared/hwaccel/
```

- [ ] **Step 2: Update import paths, verify build, commit**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/hwaccel"' internal/
# Update each file's import path
go build ./internal/...
git add -A internal/shared/hwaccel/ internal/nvr/hwaccel/
git commit -m "refactor: move nvr/hwaccel to shared/hwaccel"
```

---

### Task 7: Consolidate `nvr/crypto/` → `shared/auth/`

**Files:**
- Move: `internal/nvr/crypto/*.go` (3 files, 474 LOC)
- Modify: `internal/shared/auth/` (existing package — merge crypto functions)

- [ ] **Step 1: Audit existing shared/auth for overlap**

```bash
ls internal/shared/auth/
grep -r 'func ' internal/shared/auth/*.go | head -20
grep -r 'func ' internal/nvr/crypto/*.go
```

Compare function names. If there's overlap (e.g., both have TLS helpers), keep the `shared/auth` version and update callers of the nvr/crypto version to use it.

- [ ] **Step 2: Move non-overlapping files**

For each file in `nvr/crypto/` that has no equivalent in `shared/auth/`:

```bash
git mv internal/nvr/crypto/keys.go internal/shared/auth/crypto_keys.go
git mv internal/nvr/crypto/encrypt.go internal/shared/auth/crypto_encrypt.go
git mv internal/nvr/crypto/tls.go internal/shared/auth/crypto_tls.go
```

Update the `package` declaration in each moved file to `package auth`.

- [ ] **Step 3: Update all import paths**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/crypto"' internal/
```

Replace `internal/nvr/crypto` with `internal/shared/auth` in each file. Update any renamed function references if the merge required deduplication.

- [ ] **Step 4: Verify compilation and run tests**

```bash
go build ./internal/shared/auth/...
go build ./internal/...
go test ./internal/shared/auth/... -v
```

- [ ] **Step 5: Commit**

```bash
git add -A internal/shared/auth/ internal/nvr/crypto/
git commit -m "refactor: consolidate nvr/crypto into shared/auth"
```

---

### Task 8: Move `nvr/api/middleware.go` → `shared/middleware/`

**Files:**
- Move: `internal/nvr/api/middleware.go` (92 LOC) → `internal/shared/middleware/middleware.go`

- [ ] **Step 1: Create destination and move**

```bash
mkdir -p internal/shared/middleware
cp internal/nvr/api/middleware.go internal/shared/middleware/middleware.go
```

- [ ] **Step 2: Update package declaration**

In `internal/shared/middleware/middleware.go`, change:
```go
package api
```
to:
```go
package middleware
```

- [ ] **Step 3: Update the Middleware struct's dependencies**

The Middleware struct (line 16 of original) references `*db.DB` from `nvr/db`. Change the field type to use an interface instead:

```go
// SessionUpdater is implemented by any database that can update session activity.
type SessionUpdater interface {
    UpdateSessionActivity(ctx context.Context, sessionID string) error
}

type Middleware struct {
    PublicKey  *rsa.PublicKey
    Sessions  SessionUpdater // was *db.DB
}
```

- [ ] **Step 4: Update callers to use the new import path**

```bash
grep -rl 'nvr/api.*Middleware' internal/
```

For each caller, update the import and adapt to use the interface.

- [ ] **Step 5: Verify build and commit**

```bash
go build ./internal/shared/middleware/...
go build ./internal/...
git add internal/shared/middleware/
git commit -m "refactor: extract nvr/api/middleware to shared/middleware

Uses SessionUpdater interface to decouple from nvr/db."
```

---

## Phase 2: Move Recorder-Specific Packages (14 packages, ~23,800 LOC)

These packages belong exclusively to the Recorder. Self-contained moves with import path updates.

### Task 9: Move `nvr/onvif/` → `recorder/onvif/`

**Files:**
- Move: `internal/nvr/onvif/*.go` (26 files, 8,866 LOC) → `internal/recorder/onvif/`

- [ ] **Step 1: Move files**

```bash
mkdir -p internal/recorder/onvif
git mv internal/nvr/onvif/*.go internal/recorder/onvif/
```

- [ ] **Step 2: Update all import paths**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/onvif"' internal/
```

Replace `internal/nvr/onvif` with `internal/recorder/onvif` in each file.

- [ ] **Step 3: Verify compilation and run tests**

```bash
go build ./internal/recorder/onvif/...
go build ./internal/...
go test ./internal/recorder/onvif/... -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add -A internal/recorder/onvif/ internal/nvr/onvif/
git commit -m "refactor: move nvr/onvif to recorder/onvif"
```

---

### Task 10: Move `nvr/ai/` → `recorder/ai/`

**Files:**
- Move: `internal/nvr/ai/*.go` and `internal/nvr/ai/audio/*.go` and `internal/nvr/ai/forensic/*.go` (30 files, 5,881 LOC) → `internal/recorder/ai/`

- [ ] **Step 1: Move files preserving subdirectory structure**

```bash
mkdir -p internal/recorder/ai/audio internal/recorder/ai/forensic
git mv internal/nvr/ai/*.go internal/recorder/ai/
git mv internal/nvr/ai/audio/*.go internal/recorder/ai/audio/
git mv internal/nvr/ai/forensic/*.go internal/recorder/ai/forensic/
```

- [ ] **Step 2: Update all import paths**

```bash
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/ai' internal/
```

Replace `internal/nvr/ai` with `internal/recorder/ai` (handles `ai`, `ai/audio`, `ai/forensic`).

- [ ] **Step 3: Verify compilation and run tests**

```bash
go build ./internal/recorder/ai/...
go build ./internal/...
go test ./internal/recorder/ai/... -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add -A internal/recorder/ai/ internal/nvr/ai/
git commit -m "refactor: move nvr/ai to recorder/ai"
```

---

### Task 11: Move remaining Recorder packages (12 packages)

**Files:**
- Move: `internal/nvr/{scheduler,driver,storage,backchannel,recovery,integrity,thumbnail,alerts,connmgr,managed,yamlwriter}/*.go` → corresponding `internal/recorder/` subdirectories

- [ ] **Step 1: Move all 12 packages**

```bash
for pkg in scheduler driver storage backchannel recovery integrity thumbnail alerts connmgr managed yamlwriter; do
  mkdir -p "internal/recorder/$pkg"
  git mv "internal/nvr/$pkg/"*.go "internal/recorder/$pkg/"
done
```

- [ ] **Step 2: Absorb root-level recorder files**

```bash
# recovery_adapter.go → recorder/recovery/
git mv internal/nvr/recovery_adapter.go internal/recorder/recovery/adapter.go

# fragment_backfill.go → recorder/ root
git mv internal/nvr/fragment_backfill.go internal/recorder/fragment_backfill.go
```

Update package declarations in `adapter.go` to `package recovery` and `fragment_backfill.go` to `package recorder`.

- [ ] **Step 3: Update all import paths**

```bash
for pkg in scheduler driver storage backchannel recovery integrity thumbnail alerts connmgr managed yamlwriter; do
  grep -rl "\"github.com/bluenviron/mediamtx/internal/nvr/$pkg\"" internal/ | while read f; do
    sed -i '' "s|internal/nvr/$pkg|internal/recorder/$pkg|g" "$f"
  done
done
```

- [ ] **Step 4: Handle cross-references between moved packages**

Some of these packages import each other (e.g., `scheduler` may import `storage`). Since they all moved to `internal/recorder/`, the import paths need to reflect the new location. Run:

```bash
grep -r '"github.com/bluenviron/mediamtx/internal/nvr/' internal/recorder/
```

Update any remaining `nvr/` references within the moved packages.

- [ ] **Step 5: Verify compilation and run tests**

```bash
go build ./internal/recorder/...
go build ./internal/...
go test ./internal/recorder/... -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add -A internal/recorder/ internal/nvr/
git commit -m "refactor: move 12 recorder-specific packages from nvr/ to recorder/

Moved: scheduler, driver, storage, backchannel, recovery, integrity,
thumbnail, alerts, connmgr, managed, yamlwriter.
Also absorbed recovery_adapter.go and fragment_backfill.go."
```

---

### Task 12: Move `nvr/webhook/` → `directory/webhook/`

**Files:**
- Move: `internal/nvr/webhook/*.go` (1 file, 358 LOC) → `internal/directory/webhook/`

- [ ] **Step 1: Move, update imports, verify, commit**

```bash
mkdir -p internal/directory/webhook
git mv internal/nvr/webhook/*.go internal/directory/webhook/
grep -rl '"github.com/bluenviron/mediamtx/internal/nvr/webhook"' internal/
# Update each file's import path
go build ./internal/...
git add -A internal/directory/webhook/ internal/nvr/webhook/
git commit -m "refactor: move nvr/webhook to directory/webhook"
```

---

## Phase 3: Split the Database Layer (10,317 LOC)

### Task 13: Map NVR DB tables to Directory vs. Recorder ownership

**Files:**
- Read: `internal/nvr/db/migrations.go`
- Read: `internal/directory/db/migrations/` (all files)
- Read: `internal/recorder/state/store.go`

This is a planning/mapping step — no code changes.

- [ ] **Step 1: Catalog every NVR table and its owner**

Read `internal/nvr/db/migrations.go` and classify each table:

**Directory-owned tables** (these go into `internal/directory/db/`):
- `cameras`, `camera_streams`, `devices` — camera management
- `users`, `roles`, `user_camera_permissions` — identity
- `refresh_tokens`, `api_keys`, `api_key_audit_log` — auth tokens
- `recording_rules` — schedules (maps to existing `recording_schedules`)
- `config` — system config
- `camera_groups`, `camera_group_members` — organization
- `webhook_configs`, `webhook_deliveries` — webhook system
- `alert_rules`, `alerts` — alerting (maps to existing `alert_rules`)
- `smtp_config` — notification config
- `notification_preferences`, `notification_quiet_hours`, `escalation_rules`, `notifications`, `notification_read_state` — notifications
- `federations`, `federation_peers` — enterprise
- `integration_configs` — integrations
- `upgrade_migrations` — system
- `update_history` — system

**Recorder-owned tables** (these go into `internal/recorder/db/`):
- `recordings` — segment metadata
- `saved_clips`, `clips`, `bookmarks` — user clips
- `tracks`, `tours` — playback
- `detections`, `detection_events`, `detection_zones`, `detection_schedules` — AI
- `motion_events` — motion detection
- `screenshots` — snapshots
- `storage_quotas` — local storage
- `connection_events`, `queued_commands` — sync state
- `export_jobs`, `evidence_exports`, `bulk_export_jobs`, `bulk_export_items` — export
- `cross_camera_tracks`, `cross_camera_sightings` — tracking
- `pending_syncs` — offline queue

- [ ] **Step 2: Document which tables already exist in directory/recorder DBs**

Check existing directory DB migrations for overlap:

```bash
grep -r 'CREATE TABLE' internal/directory/db/migrations/
```

Note any tables that exist in both NVR and Directory schemas. The Directory version wins — NVR columns not present in Directory get added via new migration.

- [ ] **Step 3: Commit mapping document (optional)**

If useful, save the mapping as a comment in the migration files for reference.

---

### Task 14: Create Recorder DB package (`internal/recorder/db/`)

**Files:**
- Create: `internal/recorder/db/db.go`
- Create: `internal/recorder/db/migrations.go`
- Create: `internal/recorder/db/*_store.go` files for each table group
- Reference: `internal/nvr/db/db.go` (lines 14-105), `internal/nvr/db/recordings.go`, etc.

- [ ] **Step 1: Write failing test for DB open and migrate**

Create `internal/recorder/db/db_test.go`:

```go
package db_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    db "github.com/bluenviron/mediamtx/internal/recorder/db"
    "github.com/stretchr/testify/require"
)

func TestOpenAndMigrate(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "recorder.db")

    rdb, err := db.Open(context.Background(), path)
    require.NoError(t, err)
    defer rdb.Close()

    // Verify a recorder-owned table exists
    var count int
    err = rdb.QueryRowContext(context.Background(),
        "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='recordings'").Scan(&count)
    require.NoError(t, err)
    require.Equal(t, 1, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/recorder/db/... -v -run TestOpenAndMigrate
```

Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Create `internal/recorder/db/db.go`**

```go
package db

import (
    "context"
    "database/sql"
    "fmt"

    _ "modernc.org/sqlite"
)

// DB wraps a SQLite connection for the Recorder's local database.
type DB struct {
    *sql.DB
    path string
}

// Open creates or opens the Recorder database at path and runs migrations.
func Open(ctx context.Context, path string) (*DB, error) {
    dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)", path)
    sqlDB, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, fmt.Errorf("open recorder db: %w", err)
    }
    sqlDB.SetMaxOpenConns(2)

    if err := sqlDB.PingContext(ctx); err != nil {
        sqlDB.Close()
        return nil, fmt.Errorf("ping recorder db: %w", err)
    }

    rdb := &DB{DB: sqlDB, path: path}
    if err := rdb.migrate(ctx); err != nil {
        sqlDB.Close()
        return nil, fmt.Errorf("migrate recorder db: %w", err)
    }
    return rdb, nil
}

// Path returns the file path of the database.
func (d *DB) Path() string { return d.path }
```

- [ ] **Step 4: Create `internal/recorder/db/migrations.go`**

Move the recorder-owned table schemas from `internal/nvr/db/migrations.go`. Copy the exact CREATE TABLE statements for recorder-owned tables identified in Task 13, wrapping them in a single migration:

```go
package db

import (
    "context"
    "fmt"
)

var migrations = []struct {
    version int
    sql     string
}{
    {1, `
CREATE TABLE IF NOT EXISTS recordings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    duration REAL,
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    status TEXT DEFAULT 'recording',
    thumbnail_path TEXT,
    has_audio INTEGER DEFAULT 0,
    codec TEXT DEFAULT '',
    resolution TEXT DEFAULT '',
    fps REAL DEFAULT 0,
    bitrate INTEGER DEFAULT 0,
    segment_count INTEGER DEFAULT 0,
    error_message TEXT,
    metadata TEXT
);
CREATE INDEX IF NOT EXISTS idx_recordings_camera_time ON recordings(camera_id, start_time);
CREATE INDEX IF NOT EXISTS idx_recordings_status ON recordings(status);

CREATE TABLE IF NOT EXISTS saved_clips (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    file_path TEXT,
    file_size INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    notes TEXT
);

CREATE TABLE IF NOT EXISTS bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    track_type TEXT NOT NULL DEFAULT 'video',
    encoding TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tours (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    dwell_seconds INTEGER NOT NULL DEFAULT 10,
    transition TEXT NOT NULL DEFAULT 'cut',
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS detections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    type TEXT NOT NULL,
    confidence REAL NOT NULL,
    bbox_x REAL, bbox_y REAL, bbox_w REAL, bbox_h REAL,
    metadata TEXT,
    track_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_detections_camera_time ON detections(camera_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_detections_type ON detections(type);

CREATE TABLE IF NOT EXISTS detection_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    zone_id INTEGER,
    event_type TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    end_time DATETIME,
    detection_count INTEGER DEFAULT 1,
    max_confidence REAL DEFAULT 0,
    metadata TEXT,
    thumbnail_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS detection_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    zone_type TEXT NOT NULL DEFAULT 'include',
    points TEXT NOT NULL,
    detection_types TEXT NOT NULL DEFAULT '[]',
    min_confidence REAL DEFAULT 0.5,
    enabled INTEGER DEFAULT 1,
    color TEXT DEFAULT '#FF0000',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS detection_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    detection_type TEXT NOT NULL DEFAULT 'all',
    schedule_type TEXT NOT NULL DEFAULT 'always',
    days_of_week TEXT DEFAULT '[]',
    start_time TEXT,
    end_time TEXT,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS motion_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    intensity REAL DEFAULT 0,
    regions TEXT,
    metadata TEXT
);
CREATE INDEX IF NOT EXISTS idx_motion_camera_time ON motion_events(camera_id, start_time);

CREATE TABLE IF NOT EXISTS screenshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    width INTEGER DEFAULT 0,
    height INTEGER DEFAULT 0,
    source TEXT DEFAULT 'manual',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS storage_quotas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT,
    max_size_gb REAL DEFAULT 0,
    max_days INTEGER DEFAULT 30,
    max_recordings INTEGER DEFAULT 0,
    priority INTEGER DEFAULT 5,
    action TEXT DEFAULT 'delete_oldest',
    enabled INTEGER DEFAULT 1,
    current_size_gb REAL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS connection_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    details TEXT,
    source_ip TEXT,
    duration_ms INTEGER
);

CREATE TABLE IF NOT EXISTS pending_syncs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    attempts INTEGER DEFAULT 0,
    last_attempt_at DATETIME,
    error_message TEXT
);
CREATE INDEX IF NOT EXISTS idx_pending_syncs_type ON pending_syncs(sync_type);

CREATE TABLE IF NOT EXISTS export_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    format TEXT NOT NULL DEFAULT 'mp4',
    status TEXT NOT NULL DEFAULT 'pending',
    file_path TEXT,
    file_size INTEGER DEFAULT 0,
    progress REAL DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS evidence_exports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    case_number TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cameras TEXT NOT NULL DEFAULT '[]',
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    include_metadata INTEGER DEFAULT 1,
    include_detections INTEGER DEFAULT 1,
    format TEXT DEFAULT 'zip',
    status TEXT DEFAULT 'pending',
    file_path TEXT,
    file_size INTEGER DEFAULT 0,
    hash_sha256 TEXT,
    chain_of_custody TEXT DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS cross_camera_tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id TEXT NOT NULL UNIQUE,
    detection_type TEXT NOT NULL,
    first_seen DATETIME NOT NULL,
    last_seen DATETIME NOT NULL,
    camera_count INTEGER DEFAULT 1,
    total_sightings INTEGER DEFAULT 1,
    metadata TEXT,
    embedding BLOB,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cross_camera_sightings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id TEXT NOT NULL,
    camera_id TEXT NOT NULL,
    detection_id INTEGER,
    timestamp DATETIME NOT NULL,
    confidence REAL NOT NULL,
    bbox TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (track_id) REFERENCES cross_camera_tracks(track_id)
);
`},
}

func (d *DB) migrate(ctx context.Context) error {
    _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )`)
    if err != nil {
        return fmt.Errorf("create schema_migrations: %w", err)
    }

    for _, m := range migrations {
        var exists int
        err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&exists)
        if err != nil {
            return fmt.Errorf("check migration %d: %w", m.version, err)
        }
        if exists > 0 {
            continue
        }
        tx, err := d.BeginTx(ctx, nil)
        if err != nil {
            return fmt.Errorf("begin migration %d: %w", m.version, err)
        }
        if _, err := tx.ExecContext(ctx, m.sql); err != nil {
            tx.Rollback()
            return fmt.Errorf("exec migration %d: %w", m.version, err)
        }
        if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
            tx.Rollback()
            return fmt.Errorf("record migration %d: %w", m.version, err)
        }
        if err := tx.Commit(); err != nil {
            return fmt.Errorf("commit migration %d: %w", m.version, err)
        }
    }
    return nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/recorder/db/... -v -run TestOpenAndMigrate
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/db/
git commit -m "feat: create recorder/db package with recorder-owned schema

Extracted recorder-owned tables from nvr/db into dedicated package."
```

---

### Task 15: Create CRUD store files for Recorder DB

**Files:**
- Create: `internal/recorder/db/recordings.go` — from `internal/nvr/db/recordings.go` (684 LOC)
- Create: `internal/recorder/db/detections.go` — from `internal/nvr/db/detections.go` + `detection_events.go`
- Create: `internal/recorder/db/clips.go` — from `internal/nvr/db/clips.go` + `saved_clips.go` + `bookmarks.go`
- Create: `internal/recorder/db/export.go` — from `internal/nvr/db/export_jobs.go` + `evidence_exports.go`
- Create: `internal/recorder/db/storage.go` — from `internal/nvr/db/quota.go` + `storage_estimate.go` + `retention.go`
- Create: `internal/recorder/db/motion.go` — from `internal/nvr/db/motion_events.go`
- Create: `internal/recorder/db/sync.go` — from `internal/nvr/db/pending_syncs.go` + `connection_events.go`

- [ ] **Step 1: Copy each NVR DB store file to recorder/db**

For each file, copy from `internal/nvr/db/` to `internal/recorder/db/`, changing the package declaration from `package db` (nvr) to `package db` (recorder). Since both are `package db` this is a direct copy:

```bash
for f in recordings.go detections.go detection_events.go clips.go saved_clips.go bookmarks.go \
         export_jobs.go evidence_exports.go quota.go storage_estimate.go retention.go \
         motion_events.go pending_syncs.go connection_events.go screenshots.go \
         tracks.go tours.go maintenance.go; do
  if [ -f "internal/nvr/db/$f" ]; then
    cp "internal/nvr/db/$f" "internal/recorder/db/$f"
  fi
done
```

- [ ] **Step 2: Update DB type references**

In each copied file, the receiver type should be `*DB` matching the new `recorder/db.DB` struct. Since both the old and new package name their struct `DB`, no receiver changes should be needed. Verify:

```bash
grep 'func (d \*DB)' internal/recorder/db/*.go | head -20
```

- [ ] **Step 3: Remove references to directory-owned tables**

If any copied file references a table that belongs to Directory (e.g., `cameras`, `users`), remove those functions. The recorder/db package should only contain functions for recorder-owned tables.

- [ ] **Step 4: Verify compilation and run tests**

```bash
go build ./internal/recorder/db/...
go test ./internal/recorder/db/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/recorder/db/
git commit -m "feat: add CRUD stores to recorder/db

Copied from nvr/db, scoped to recorder-owned tables only."
```

---

### Task 16: Absorb NVR tables into Directory DB

**Files:**
- Modify: `internal/directory/db/migrations/` — add new migration for NVR tables not yet in Directory
- Reference: `internal/nvr/db/migrations.go` — source schemas

- [ ] **Step 1: Identify tables missing from Directory DB**

```bash
# Tables already in directory DB:
grep -r 'CREATE TABLE' internal/directory/db/migrations/ | grep -o 'CREATE TABLE[^(]*' | sort

# Tables that should be in directory DB (from spec):
# cameras, camera_streams, devices, users, roles, user_camera_permissions,
# refresh_tokens, api_keys, api_key_audit_log, recording_rules, config,
# camera_groups, camera_group_members, webhook_configs, webhook_deliveries,
# alert_rules, alerts, smtp_config, notification_preferences,
# notification_quiet_hours, escalation_rules, notifications,
# notification_read_state, federations, federation_peers,
# integration_configs, upgrade_migrations, update_history
```

- [ ] **Step 2: Create new migration file for missing tables**

Create `internal/directory/db/migrations/0016_nvr_absorbed_tables.sql` (or next version number) with CREATE TABLE IF NOT EXISTS for each table missing from the Directory DB. Copy the exact schema from `nvr/db/migrations.go`.

- [ ] **Step 3: Copy NVR DB store files for Directory-owned tables**

```bash
for f in cameras.go camera_streams.go devices.go users.go roles.go \
         api_keys.go webhooks.go retention.go alerts.go \
         notification_prefs.go federation.go integrations.go \
         config.go stats.go; do
  if [ -f "internal/nvr/db/$f" ]; then
    cp "internal/nvr/db/$f" "internal/directory/db/$f"
  fi
done
```

Update package declarations to match the directory/db package name.

- [ ] **Step 4: Verify compilation and run tests**

```bash
go build ./internal/directory/db/...
go test ./internal/directory/db/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/directory/db/
git commit -m "feat: absorb NVR directory-owned tables into directory/db

Added missing table schemas and CRUD stores from nvr/db."
```

---

## Phase 4: Split the API Layer (21,532 LOC)

### Task 17: Create `directory/cameraapi/` package

**Files:**
- Create: `internal/directory/cameraapi/handler.go`
- Create: `internal/directory/cameraapi/handler_test.go`
- Reference: `internal/nvr/api/cameras.go` (5,230 LOC) — extract Directory-side CRUD

- [ ] **Step 1: Write failing test**

Create `internal/directory/cameraapi/handler_test.go`:

```go
package cameraapi_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/bluenviron/mediamtx/internal/directory/cameraapi"
    "github.com/stretchr/testify/require"
)

func TestRegisterRoutes(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := cameraapi.NewHandler(nil) // nil DB for route registration test
    h.Register(r)

    // Verify routes are registered
    routes := r.Routes()
    paths := make(map[string]bool)
    for _, route := range routes {
        paths[route.Method+" "+route.Path] = true
    }
    require.True(t, paths["GET /api/v1/cameras"])
    require.True(t, paths["POST /api/v1/cameras"])
    require.True(t, paths["PUT /api/v1/cameras/:id"])
    require.True(t, paths["DELETE /api/v1/cameras/:id"])
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/cameraapi/... -v -run TestRegisterRoutes
```

Expected: FAIL.

- [ ] **Step 3: Create handler with route registration**

Create `internal/directory/cameraapi/handler.go`:

```go
package cameraapi

import (
    "github.com/gin-gonic/gin"
    dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// Handler serves camera management CRUD endpoints.
type Handler struct {
    db *dirdb.DB
}

// NewHandler creates a camera API handler.
func NewHandler(db *dirdb.DB) *Handler {
    return &Handler{db: db}
}

// Register mounts camera routes on the router.
func (h *Handler) Register(r gin.IRouter) {
    g := r.Group("/api/v1/cameras")
    g.GET("", h.list)
    g.POST("", h.create)
    g.GET("/:id", h.get)
    g.PUT("/:id", h.update)
    g.DELETE("/:id", h.delete)
}
```

Then copy the handler method implementations from `internal/nvr/api/cameras.go`, extracting only the Directory-side operations (list, create, get, update, delete cameras). Rename the receiver from whatever it was in nvr/api to `*Handler`. Update DB references to use `dirdb.DB`.

- [ ] **Step 4: Run test**

```bash
go test ./internal/directory/cameraapi/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/cameraapi/
git commit -m "feat: create directory/cameraapi package

Extracted camera management CRUD from nvr/api/cameras.go."
```

---

### Task 18: Absorb NVR admin endpoints into `directory/adminapi/`

**Files:**
- Modify: `internal/directory/adminapi/handlers.go` — add user, role, auth, federation, branding, audit, apikey, notification endpoints
- Reference: `internal/nvr/api/auth.go`, `users.go`, `roles.go`, `apikeys.go`, `sessions.go`, `federation.go`, `branding.go`, `audit.go`, `notification_prefs.go`, `notifications.go`, `alerts.go`, `webhooks.go`

- [ ] **Step 1: Audit existing adminapi handlers**

```bash
grep 'func ' internal/directory/adminapi/handlers.go | head -30
```

Identify which endpoints already exist vs. what needs to be added from nvr/api.

- [ ] **Step 2: Copy handler functions from nvr/api for missing endpoints**

For each nvr/api file that maps to adminapi (auth, users, roles, apikeys, sessions, federation, branding, audit, notification_prefs, notifications, alerts, webhooks), copy the handler functions into new files in `internal/directory/adminapi/`:

```bash
# Create per-concern files to keep things organized
cp internal/nvr/api/auth.go internal/directory/adminapi/auth.go
cp internal/nvr/api/users.go internal/directory/adminapi/users_handlers.go
cp internal/nvr/api/roles.go internal/directory/adminapi/roles_handlers.go
cp internal/nvr/api/apikeys.go internal/directory/adminapi/apikeys.go
cp internal/nvr/api/federation.go internal/directory/adminapi/federation.go
cp internal/nvr/api/branding.go internal/directory/adminapi/branding.go
cp internal/nvr/api/audit.go internal/directory/adminapi/audit_handlers.go
cp internal/nvr/api/notification_prefs.go internal/directory/adminapi/notification_prefs.go
cp internal/nvr/api/alerts.go internal/directory/adminapi/alerts_handlers.go
cp internal/nvr/api/webhooks.go internal/directory/adminapi/webhooks_handlers.go
```

- [ ] **Step 3: Update package declarations and DB references**

Change `package api` to `package adminapi` in each file. Update DB references from `*db.DB` (nvr) to `*dirdb.DB` (directory). Update receiver types to match the existing adminapi Handler pattern.

- [ ] **Step 4: Add route registration for new endpoints**

Update the `Register()` method in adminapi to include routes for all absorbed endpoints, following the pattern already established in the existing adminapi code.

- [ ] **Step 5: Verify compilation and run tests**

```bash
go build ./internal/directory/adminapi/...
go test ./internal/directory/adminapi/... -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/directory/adminapi/
git commit -m "feat: absorb NVR admin endpoints into directory/adminapi

Added: auth, users, roles, apikeys, federation, branding, audit,
notifications, alerts, webhooks handlers."
```

---

### Task 19: Create `recorder/recordingapi/` package

**Files:**
- Create: `internal/recorder/recordingapi/handler.go`
- Create: `internal/recorder/recordingapi/handler_test.go`
- Reference: `internal/nvr/api/recordings.go`, `hls.go`, `export.go`, `bulk_export.go`, `evidence.go`, `bookmarks.go`, `tours.go`, `tracks.go`, `saved_clips.go`, `screenshots.go`, `thumbnails.go`, `recording_control.go`, `backchannel.go`, `recording_health.go`, `stats.go`, `storage.go`, `quota.go`

- [ ] **Step 1: Write failing test for route registration**

Create `internal/recorder/recordingapi/handler_test.go`:

```go
package recordingapi_test

import (
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/bluenviron/mediamtx/internal/recorder/recordingapi"
    "github.com/stretchr/testify/require"
)

func TestRegisterRoutes(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := recordingapi.NewHandler(nil)
    h.Register(r)

    routes := r.Routes()
    paths := make(map[string]bool)
    for _, route := range routes {
        paths[route.Method+" "+route.Path] = true
    }
    require.True(t, paths["GET /api/v1/recordings"])
    require.True(t, paths["GET /api/v1/timeline"])
    require.True(t, paths["GET /api/v1/health"])
    require.True(t, paths["POST /api/v1/export"])
}
```

- [ ] **Step 2: Create handler with implementations**

Create `internal/recorder/recordingapi/handler.go`:

```go
package recordingapi

import (
    "github.com/gin-gonic/gin"
    recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

type Handler struct {
    db *recdb.DB
}

func NewHandler(db *recdb.DB) *Handler {
    return &Handler{db: db}
}

func (h *Handler) Register(r gin.IRouter) {
    r.GET("/api/v1/recordings", h.listRecordings)
    r.GET("/api/v1/recordings/:id", h.getRecording)
    r.GET("/api/v1/recordings/:id/hls", h.serveHLS)
    r.GET("/api/v1/timeline", h.getTimeline)
    r.GET("/api/v1/health", h.getHealth)
    r.POST("/api/v1/export", h.createExport)
    r.GET("/api/v1/export/:id", h.getExportStatus)
    r.GET("/api/v1/bookmarks", h.listBookmarks)
    r.POST("/api/v1/bookmarks", h.createBookmark)
    r.GET("/api/v1/diagnostics", h.getDiagnostics)
}
```

Copy handler method bodies from the corresponding nvr/api files. Update receiver types, DB references, and import paths.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/recorder/recordingapi/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/recorder/recordingapi/
git commit -m "feat: create recorder/recordingapi package

Extracted recording data-plane endpoints from nvr/api."
```

---

### Task 20: Create `recorder/detectionapi/` package

**Files:**
- Create: `internal/recorder/detectionapi/handler.go`
- Reference: `internal/nvr/api/detection_zones.go`, `detection_schedule.go`, `detection_events.go`, `ai_metrics.go`, `forensic_search.go`, `edge_search.go`

- [ ] **Step 1: Create handler with route registration and implementations**

Create `internal/recorder/detectionapi/handler.go`:

```go
package detectionapi

import (
    "github.com/gin-gonic/gin"
    recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

type Handler struct {
    db *recdb.DB
}

func NewHandler(db *recdb.DB) *Handler {
    return &Handler{db: db}
}

func (h *Handler) Register(r gin.IRouter) {
    r.GET("/api/v1/detections", h.listDetections)
    r.GET("/api/v1/detection-zones", h.listZones)
    r.POST("/api/v1/detection-zones", h.createZone)
    r.PUT("/api/v1/detection-zones/:id", h.updateZone)
    r.DELETE("/api/v1/detection-zones/:id", h.deleteZone)
    r.GET("/api/v1/detection-schedules", h.listSchedules)
    r.POST("/api/v1/detection-schedules", h.createSchedule)
    r.GET("/api/v1/ai/metrics", h.getAIMetrics)
    r.POST("/api/v1/forensic-search", h.forensicSearch)
}
```

Copy handler method bodies from corresponding nvr/api files.

- [ ] **Step 2: Verify build and commit**

```bash
go build ./internal/recorder/detectionapi/...
git add internal/recorder/detectionapi/
git commit -m "feat: create recorder/detectionapi package

Extracted AI/detection endpoints from nvr/api."
```

---

### Task 21: Create `shared/systemapi/` package

**Files:**
- Create: `internal/shared/systemapi/handler.go`
- Reference: `internal/nvr/api/system.go`, `health.go`, `diagnostics.go`, `updates.go`, `log_config.go`, `logging.go`, `security.go`, `tls.go`, `sizing.go`

- [ ] **Step 1: Create handler**

Create `internal/shared/systemapi/handler.go`:

```go
package systemapi

import (
    "github.com/gin-gonic/gin"
)

type Handler struct{}

func NewHandler() *Handler {
    return &Handler{}
}

func (h *Handler) Register(r gin.IRouter) {
    r.GET("/api/v1/system/health", h.health)
    r.GET("/api/v1/system/status", h.status)
    r.GET("/api/v1/system/diagnostics", h.diagnostics)
    r.GET("/api/v1/system/updates", h.checkUpdates)
    r.GET("/api/v1/system/metrics", h.metrics)
    r.PUT("/api/v1/system/logging", h.setLogLevel)
}
```

Copy handler implementations from nvr/api system files.

- [ ] **Step 2: Verify build and commit**

```bash
go build ./internal/shared/systemapi/...
git add internal/shared/systemapi/
git commit -m "feat: create shared/systemapi package

Extracted system endpoints from nvr/api for use by both Directory and Recorder."
```

---

## Phase 5: Update Boot Sequences and Core Dispatch

### Task 22: Update Recorder boot.go to use new packages

**Files:**
- Modify: `internal/recorder/boot.go` (lines 160-440)

- [ ] **Step 1: Add HTTP server to Recorder boot sequence**

The Recorder currently has no HTTP server (it only runs streaming clients). Add an HTTP server that registers `recordingapi`, `detectionapi`, and `systemapi` routes.

In `internal/recorder/boot.go`, after the streaming clients are started (around line 420), add:

```go
// --- Stage: HTTP API Server ---
recRouter := gin.New()
recRouter.Use(gin.Recovery())

recordingapi.NewHandler(recDB).Register(recRouter)
detectionapi.NewHandler(recDB).Register(recRouter)
systemapi.NewHandler().Register(recRouter)

recHTTPServer := &http.Server{
    Addr:    cfg.APIAddress,
    Handler: recRouter,
}
go func() {
    if err := recHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Error("recorder HTTP server failed", "error", err)
    }
}()
```

- [ ] **Step 2: Update imports**

Add imports for the new packages:

```go
import (
    "github.com/bluenviron/mediamtx/internal/recorder/recordingapi"
    "github.com/bluenviron/mediamtx/internal/recorder/detectionapi"
    "github.com/bluenviron/mediamtx/internal/shared/systemapi"
    recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)
```

- [ ] **Step 3: Open recorder/db during boot**

In the boot sequence, after the state store is opened (around line 205), also open the recorder DB:

```go
rdb, err := recdb.Open(ctx, cfg.recorderDBPath())
if err != nil {
    return nil, fmt.Errorf("open recorder db: %w", err)
}
```

Add `recorderDBPath()` to the config defaults:

```go
func (c *BootConfig) recorderDBPath() string {
    return filepath.Join(c.stateDir(), "recorder.db")
}
```

- [ ] **Step 4: Update RecorderServer struct to hold new resources**

Add fields to the `RecorderServer` struct (line 160):

```go
type RecorderServer struct {
    // ... existing fields ...
    recDB      *recdb.DB
    httpServer *http.Server
}
```

Update `Shutdown()` to close the HTTP server and recorder DB.

- [ ] **Step 5: Verify compilation**

```bash
go build ./internal/recorder/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/
git commit -m "feat: add HTTP API server to Recorder boot sequence

Recorder now exposes recordingapi, detectionapi, and systemapi
endpoints for direct client access."
```

---

### Task 23: Update Directory boot.go to use new API packages

**Files:**
- Modify: `internal/directory/boot.go` (lines 310-390, route registration section)

- [ ] **Step 1: Add new API package route registration**

In the HTTP route registration section (around line 310), add registration for the new packages alongside existing routes:

```go
// Camera management (new — from nvr/api/cameras.go)
cameraapi.NewHandler(dirDB).Register(ginRouter)

// Admin API (existing — now with absorbed NVR endpoints)
// Already registered, but verify all new routes are included

// System API (new — shared endpoints)
systemapi.NewHandler().Register(ginRouter)
```

- [ ] **Step 2: Update imports**

```go
import (
    "github.com/bluenviron/mediamtx/internal/directory/cameraapi"
    "github.com/bluenviron/mediamtx/internal/shared/systemapi"
)
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/directory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/directory/boot.go
git commit -m "feat: register cameraapi and systemapi in Directory boot"
```

---

### Task 24: Remove legacy mode from Core dispatch

**Files:**
- Modify: `internal/core/core.go` (lines 29, 138-139, 212-219, 321-337)
- Modify: `internal/shared/runtime/mode.go` (lines 22-41, 102-157)
- Modify: `internal/conf/runtime_mode.go` (lines 16-29)

- [ ] **Step 1: Remove NVR import from core.go**

In `internal/core/core.go`, remove line 29:

```go
// DELETE this import:
"github.com/bluenviron/mediamtx/internal/nvr"
```

- [ ] **Step 2: Update mode dispatch — empty string becomes all-in-one**

In `internal/shared/runtime/mode.go`, update the `Dispatch` function (line 102+):

Change the legacy case to route to all-in-one:

```go
func Dispatch(mode Mode, hooks DispatchHooks) error {
    switch mode {
    case ModeLegacy, ModeAllInOne:
        // Empty/legacy mode now boots all-in-one
        return startAllInOne(hooks)
    case ModeDirectory:
        return hooks.StartDirectory()
    case ModeRecorder:
        return hooks.StartRecorder()
    default:
        return fmt.Errorf("unknown mode: %q", mode)
    }
}

func startAllInOne(hooks DispatchHooks) error {
    if err := hooks.StartDirectory(); err != nil {
        return fmt.Errorf("start directory: %w", err)
    }
    if err := hooks.StartRecorder(); err != nil {
        return fmt.Errorf("start recorder: %w", err)
    }
    if hooks.AutoPair != nil {
        if err := hooks.AutoPair(); err != nil {
            return fmt.Errorf("auto-pair: %w", err)
        }
    }
    return nil
}
```

- [ ] **Step 3: Update core.go wiring — legacy mode wires both booters**

In `internal/core/core.go` (lines 212-219), update so that empty mode also wires both:

```go
mode := p.conf.Mode.Runtime()
if mode == kairuntime.ModeLegacy || mode == kairuntime.ModeDirectory || mode == kairuntime.ModeAllInOne {
    p.directoryBooter = directory.NewBooter()
}
if mode == kairuntime.ModeLegacy || mode == kairuntime.ModeRecorder || mode == kairuntime.ModeAllInOne {
    p.recorderBooter = &recorderboot.Booter{}
}
```

- [ ] **Step 4: Remove all NVR initialization from core.go**

Find and remove any code in core.go that references the `nvr` package — the old `StartOptions`, `AllOptions()`, `DirectoryOptions()`, `RecorderOptions()` patterns. These are replaced by the booter dispatch.

- [ ] **Step 5: Verify compilation**

```bash
go build ./internal/core/...
go build ./internal/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/core/core.go internal/shared/runtime/mode.go internal/conf/runtime_mode.go
git commit -m "refactor: remove legacy mode — empty mode now boots all-in-one

Default config (no mode set) now starts directory+recorder in-process."
```

---

### Task 25: Create legacy DB migration utility

**Files:**
- Create: `internal/shared/migration/legacy.go`
- Create: `internal/shared/migration/legacy_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/shared/migration/legacy_test.go`:

```go
package migration_test

import (
    "context"
    "database/sql"
    "os"
    "path/filepath"
    "testing"

    "github.com/bluenviron/mediamtx/internal/shared/migration"
    "github.com/stretchr/testify/require"
    _ "modernc.org/sqlite"
)

func TestMigrateLegacyNVRDB(t *testing.T) {
    dir := t.TempDir()
    nvrPath := filepath.Join(dir, "nvr.db")

    // Create a minimal legacy nvr.db with a cameras table
    nvrDB, err := sql.Open("sqlite", nvrPath)
    require.NoError(t, err)
    _, err = nvrDB.Exec(`
        CREATE TABLE cameras (id TEXT PRIMARY KEY, name TEXT);
        INSERT INTO cameras VALUES ('cam-1', 'Front Door');
        CREATE TABLE recordings (id INTEGER PRIMARY KEY, camera_id TEXT, file_path TEXT);
        INSERT INTO recordings VALUES (1, 'cam-1', '/data/rec1.mp4');
    `)
    require.NoError(t, err)
    nvrDB.Close()

    // Run migration
    err = migration.MigrateLegacyDB(context.Background(), dir)
    require.NoError(t, err)

    // Verify directory.db has cameras
    dirPath := filepath.Join(dir, "directory.db")
    require.FileExists(t, dirPath)

    // Verify nvr.db was backed up
    require.FileExists(t, filepath.Join(dir, "nvr.db.backup"))
    require.NoFileExists(t, nvrPath)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/shared/migration/... -v -run TestMigrateLegacyNVRDB
```

Expected: FAIL.

- [ ] **Step 3: Implement migration utility**

Create `internal/shared/migration/legacy.go`:

```go
package migration

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "os"
    "path/filepath"

    _ "modernc.org/sqlite"
)

// directoryTables are tables that belong to the Directory database.
var directoryTables = []string{
    "cameras", "camera_streams", "devices",
    "users", "roles", "user_camera_permissions",
    "refresh_tokens", "api_keys", "api_key_audit_log",
    "recording_rules", "config",
    "camera_groups", "camera_group_members",
    "webhook_configs", "webhook_deliveries",
    "alert_rules", "alerts", "smtp_config",
    "notification_preferences", "notification_quiet_hours",
    "escalation_rules", "notifications", "notification_read_state",
    "federations", "federation_peers",
    "integration_configs", "upgrade_migrations", "update_history",
}

// recorderTables are tables that belong to the Recorder database.
var recorderTables = []string{
    "recordings", "saved_clips", "clips", "bookmarks",
    "tracks", "tours",
    "detections", "detection_events", "detection_zones", "detection_schedules",
    "motion_events", "screenshots",
    "storage_quotas", "connection_events", "pending_syncs",
    "export_jobs", "evidence_exports",
    "bulk_export_jobs", "bulk_export_items",
    "cross_camera_tracks", "cross_camera_sightings",
    "queued_commands",
}

// MigrateLegacyDB splits a legacy nvr.db into directory.db and recorder.db.
// It is idempotent — if directory.db already exists, it returns nil.
func MigrateLegacyDB(ctx context.Context, dataDir string) error {
    nvrPath := filepath.Join(dataDir, "nvr.db")
    dirPath := filepath.Join(dataDir, "directory.db")

    // Skip if no legacy DB or already migrated
    if _, err := os.Stat(nvrPath); os.IsNotExist(err) {
        return nil
    }
    if _, err := os.Stat(dirPath); err == nil {
        return nil // already migrated
    }

    slog.Info("migrating legacy NVR database", "path", nvrPath)

    srcDB, err := sql.Open("sqlite", nvrPath+"?mode=ro")
    if err != nil {
        return fmt.Errorf("open legacy db: %w", err)
    }
    defer srcDB.Close()

    // Get list of tables that actually exist in the legacy DB
    existingTables, err := listTables(ctx, srcDB)
    if err != nil {
        return fmt.Errorf("list tables: %w", err)
    }
    tableSet := make(map[string]bool, len(existingTables))
    for _, t := range existingTables {
        tableSet[t] = true
    }

    // Copy directory-owned tables
    if err := copyTables(ctx, srcDB, dirPath, directoryTables, tableSet); err != nil {
        return fmt.Errorf("copy directory tables: %w", err)
    }

    // Copy recorder-owned tables
    recPath := filepath.Join(dataDir, "recorder.db")
    if err := copyTables(ctx, srcDB, recPath, recorderTables, tableSet); err != nil {
        return fmt.Errorf("copy recorder tables: %w", err)
    }

    srcDB.Close()

    // Backup original
    backupPath := filepath.Join(dataDir, "nvr.db.backup")
    if err := os.Rename(nvrPath, backupPath); err != nil {
        return fmt.Errorf("backup legacy db: %w", err)
    }

    slog.Info("legacy NVR database migration complete",
        "directory_db", dirPath,
        "recorder_db", recPath,
        "backup", backupPath)

    return nil
}

func listTables(ctx context.Context, db *sql.DB) ([]string, error) {
    rows, err := db.QueryContext(ctx,
        "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var tables []string
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return nil, err
        }
        tables = append(tables, name)
    }
    return tables, rows.Err()
}

func copyTables(ctx context.Context, src *sql.DB, dstPath string, tables []string, existing map[string]bool) error {
    dst, err := sql.Open("sqlite", dstPath)
    if err != nil {
        return err
    }
    defer dst.Close()

    for _, table := range tables {
        if !existing[table] {
            continue
        }

        // Get CREATE TABLE statement
        var createSQL string
        err := src.QueryRowContext(ctx,
            "SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&createSQL)
        if err != nil {
            return fmt.Errorf("get schema for %s: %w", table, err)
        }

        if _, err := dst.ExecContext(ctx, createSQL); err != nil {
            return fmt.Errorf("create table %s: %w", table, err)
        }

        // Copy data using ATTACH
        if _, err := dst.ExecContext(ctx, fmt.Sprintf("ATTACH DATABASE '%s' AS src", src)); err != nil {
            // Fallback: row-by-row copy if ATTACH fails
            if err := copyTableRows(ctx, src, dst, table); err != nil {
                return fmt.Errorf("copy rows for %s: %w", table, err)
            }
            continue
        }
        if _, err := dst.ExecContext(ctx, fmt.Sprintf("INSERT INTO main.%s SELECT * FROM src.%s", table, table)); err != nil {
            return fmt.Errorf("copy data for %s: %w", table, err)
        }
        dst.ExecContext(ctx, "DETACH DATABASE src")
    }
    return nil
}

func copyTableRows(ctx context.Context, src, dst *sql.DB, table string) error {
    rows, err := src.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
    if err != nil {
        return err
    }
    defer rows.Close()

    cols, err := rows.Columns()
    if err != nil {
        return err
    }

    placeholders := ""
    for i := range cols {
        if i > 0 {
            placeholders += ","
        }
        placeholders += "?"
    }

    for rows.Next() {
        vals := make([]any, len(cols))
        ptrs := make([]any, len(cols))
        for i := range vals {
            ptrs[i] = &vals[i]
        }
        if err := rows.Scan(ptrs...); err != nil {
            return err
        }
        if _, err := dst.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s VALUES (%s)", table, placeholders), vals...); err != nil {
            return err
        }
    }
    return rows.Err()
}
```

- [ ] **Step 4: Run test**

```bash
go test ./internal/shared/migration/... -v
```

Expected: PASS.

- [ ] **Step 5: Wire migration into Core.New()**

In `internal/core/core.go`, before mode dispatch, add:

```go
import "github.com/bluenviron/mediamtx/internal/shared/migration"

// In New(), before dispatchRuntimeMode():
if err := migration.MigrateLegacyDB(p.ctx, p.conf.NVRDirectoryDataDir); err != nil {
    return nil, fmt.Errorf("legacy db migration: %w", err)
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/shared/migration/ internal/core/core.go
git commit -m "feat: add legacy NVR database migration utility

Splits nvr.db into directory.db + recorder.db on first boot.
Original backed up to nvr.db.backup."
```

---

## Phase 6: Delete `internal/nvr/` and Clean Up

### Task 26: Remove all remaining `internal/nvr/` references

**Files:**
- Delete: `internal/nvr/` (entire directory)
- Modify: any files still importing from `internal/nvr/`

- [ ] **Step 1: Find any remaining nvr imports**

```bash
grep -r '"github.com/bluenviron/mediamtx/internal/nvr' internal/ --include='*.go'
```

For each file found, either:
- Update the import to the new location (if the package was moved)
- Remove the import and the code using it (if the functionality was absorbed)

- [ ] **Step 2: Verify build compiles without nvr**

```bash
go build ./internal/...
```

Fix any remaining compilation errors from the import removal.

- [ ] **Step 3: Delete `internal/nvr/`**

```bash
git rm -r internal/nvr/
```

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/... -count=1 -timeout 300s
```

Fix any test failures.

- [ ] **Step 5: Verify no remaining nvr references anywhere**

```bash
grep -r 'internal/nvr' . --include='*.go'
```

Expected: zero results.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: delete internal/nvr/ — decomposition complete

All 63K LOC moved to internal/directory/, internal/recorder/,
and internal/shared/. Legacy mode removed."
```

---

### Task 27: Add depguard linter rule

**Files:**
- Modify: `.golangci.yml` or equivalent linter config

- [ ] **Step 1: Add import boundary enforcement**

Add depguard rules to prevent directory↔recorder imports:

```yaml
linters:
  enable:
    - depguard

linters-settings:
  depguard:
    rules:
      directory-no-recorder:
        files:
          - "internal/directory/**"
        deny:
          - pkg: "github.com/bluenviron/mediamtx/internal/recorder"
            desc: "directory must not import recorder — use internal/shared"
      recorder-no-directory:
        files:
          - "internal/recorder/**"
        deny:
          - pkg: "github.com/bluenviron/mediamtx/internal/directory"
            desc: "recorder must not import directory — use internal/shared"
```

- [ ] **Step 2: Run linter**

```bash
golangci-lint run ./internal/...
```

Expected: no violations.

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "chore: add depguard rule enforcing directory/recorder boundary"
```

---

## Phase 7: Flutter Client Updates

### Task 28: Enrich Camera model with Recorder endpoint

**Files:**
- Modify: `clients/flutter/lib/models/camera.dart` (lines 7-42)

- [ ] **Step 1: Add Recorder routing fields to Camera model**

In `clients/flutter/lib/models/camera.dart`, add three fields to the Freezed class:

```dart
@freezed
class Camera with _$Camera {
  const factory Camera({
    required String id,
    required String name,
    // ... existing fields ...
    String? recorderId,          // ADD: which Recorder owns this camera
    String? recorderEndpoint,    // ADD: direct URL to Recorder's API
    String? directoryId,         // ADD: which Directory (for federation)
  }) = _Camera;

  factory Camera.fromJson(Map<String, dynamic> json) => _$CameraFromJson(json);
}
```

- [ ] **Step 2: Run code generation**

```bash
cd clients/flutter && dart run build_runner build --delete-conflicting-outputs
```

- [ ] **Step 3: Verify compilation**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
cd clients/flutter
git add lib/models/camera.dart lib/models/camera.freezed.dart lib/models/camera.g.dart
git commit -m "feat: add recorderId, recorderEndpoint, directoryId to Camera model"
```

---

### Task 29: Implement `HttpCameraDirectoryClient`

**Files:**
- Modify: `clients/flutter/lib/cameras/camera_directory_client.dart` (lines 120-147)

- [ ] **Step 1: Write test for listCameras**

Create `clients/flutter/test/cameras/http_camera_directory_client_test.dart`:

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/cameras/camera_directory_client.dart';

void main() {
  group('HttpCameraDirectoryClient', () {
    test('listCameras returns cameras with recorder endpoints', () async {
      // This is an integration test stub — will need a mock HTTP server
      // For now, verify the class compiles and has the right method signatures
      final client = HttpCameraDirectoryClient(
        baseUrl: 'http://localhost:9997',
        tokenProvider: () async => 'test-token',
      );
      expect(client, isA<CameraDirectoryClient>());
    });
  });
}
```

- [ ] **Step 2: Implement listCameras and watchStatus**

Replace the stub implementation in `camera_directory_client.dart` (lines 120-147):

```dart
class HttpCameraDirectoryClient implements CameraDirectoryClient {
  HttpCameraDirectoryClient({
    required this.baseUrl,
    required this.tokenProvider,
  });

  final String baseUrl;
  final Future<String> Function() tokenProvider;

  @override
  Future<List<Camera>> listCameras() async {
    final token = await tokenProvider();
    final uri = Uri.parse('$baseUrl/api/v1/cameras');
    final response = await http.get(uri, headers: {
      'Authorization': 'Bearer $token',
    });
    if (response.statusCode != 200) {
      throw Exception('Failed to list cameras: ${response.statusCode}');
    }
    final List<dynamic> body = jsonDecode(response.body);
    return body.map((json) => Camera.fromJson(json as Map<String, dynamic>)).toList();
  }

  @override
  Stream<CameraStatusEvent> watchStatus() {
    // Server-sent events stream from Directory
    final controller = StreamController<CameraStatusEvent>.broadcast();
    _connectStatusStream(controller);
    return controller.stream;
  }

  Future<void> _connectStatusStream(StreamController<CameraStatusEvent> controller) async {
    final token = await tokenProvider();
    final uri = Uri.parse('$baseUrl/api/v1/cameras/watch');
    final request = http.Request('GET', uri);
    request.headers['Authorization'] = 'Bearer $token';
    request.headers['Accept'] = 'text/event-stream';

    try {
      final response = await http.Client().send(request);
      response.stream
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .where((line) => line.startsWith('data: '))
          .map((line) => line.substring(6))
          .map((data) => CameraStatusEvent.fromJson(
              jsonDecode(data) as Map<String, dynamic>))
          .listen(
            controller.add,
            onError: controller.addError,
            onDone: controller.close,
          );
    } catch (e) {
      controller.addError(e);
      controller.close();
    }
  }
}
```

- [ ] **Step 3: Run tests**

```bash
cd clients/flutter && flutter test test/cameras/
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/cameras/camera_directory_client.dart
git add clients/flutter/test/cameras/
git commit -m "feat: implement HttpCameraDirectoryClient with listCameras and watchStatus"
```

---

### Task 30: Add Recorder endpoint routing to live view and playback

**Files:**
- Modify: `clients/flutter/lib/cameras/camera_status_notifier.dart`
- Modify: live view and playback providers (varies by current structure)

- [ ] **Step 1: Update CameraStatusNotifier to track recorder endpoints**

In `camera_status_notifier.dart`, add `recorderEndpoint` to `CameraStatus`:

```dart
class CameraStatus {
  CameraStatus({
    required this.cameraId,
    required this.state,
    required this.lastUpdated,
    required this.sourceConnectionId,
    this.reason,
    this.recorderEndpoint,  // ADD
  });

  final String cameraId;
  final CameraOnlineState state;
  final DateTime lastUpdated;
  final String sourceConnectionId;
  final String? reason;
  final String? recorderEndpoint;  // ADD

  CameraStatus copyWith({
    CameraOnlineState? state,
    DateTime? lastUpdated,
    String? reason,
    String? recorderEndpoint,  // ADD
  }) {
    return CameraStatus(
      cameraId: cameraId,
      state: state ?? this.state,
      lastUpdated: lastUpdated ?? this.lastUpdated,
      sourceConnectionId: sourceConnectionId,
      reason: reason ?? this.reason,
      recorderEndpoint: recorderEndpoint ?? this.recorderEndpoint,  // ADD
    );
  }
}
```

- [ ] **Step 2: Create a RecorderClient provider**

Create `clients/flutter/lib/providers/recorder_client_provider.dart`:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Resolves the correct Recorder API endpoint for a given camera.
///
/// In all-in-one mode, this returns the same URL as the Directory.
/// In multi-server mode, this returns the Recorder's direct endpoint.
String resolveRecorderEndpoint(String? recorderEndpoint, String directoryEndpoint) {
  return recorderEndpoint ?? directoryEndpoint;
}
```

- [ ] **Step 3: Verify Flutter analyzes clean**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/cameras/camera_status_notifier.dart
git add clients/flutter/lib/providers/recorder_client_provider.dart
git commit -m "feat: add recorder endpoint routing to Flutter client

CameraStatus now carries recorderEndpoint for direct data-plane access.
resolveRecorderEndpoint falls back to Directory URL for all-in-one mode."
```

---

## Phase 8: Verification

### Task 31: End-to-end build and test verification

**Files:** None (verification only)

- [ ] **Step 1: Full Go build**

```bash
go build ./...
```

Expected: clean build, zero errors, zero nvr imports.

- [ ] **Step 2: Full Go test suite**

```bash
go test ./internal/... -count=1 -timeout 600s
```

Expected: all tests pass.

- [ ] **Step 3: Verify no nvr references remain**

```bash
grep -r 'internal/nvr' . --include='*.go' --include='*.md' | grep -v 'nvr.db.backup' | grep -v CHANGELOG | grep -v 'design.md' | grep -v 'plan'
```

Expected: zero results (except documentation referencing the migration).

- [ ] **Step 4: Flutter build verification**

```bash
cd clients/flutter && flutter analyze && flutter test
```

Expected: clean analysis, all tests pass.

- [ ] **Step 5: Verify depguard boundary**

```bash
golangci-lint run ./internal/directory/... ./internal/recorder/...
```

Expected: no cross-boundary import violations.

- [ ] **Step 6: Commit any test fixes**

```bash
git add -A
git commit -m "fix: resolve test failures from NVR decomposition"
```

- [ ] **Step 7: Final verification commit**

```bash
git log --oneline -20
```

Verify the commit history tells a clear story of the decomposition.
