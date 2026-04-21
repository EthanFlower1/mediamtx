# Legacydb Elimination — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate `internal/shared/legacydb/` by migrating all 26 consuming files to use `recorder/db` or `directory/db`, then delete legacydb entirely.

**Architecture:** The legacydb package is a transitional copy of the old monolithic NVR database. All recorder-side types and methods already exist in `recorder/db`. The missing piece is `directory/db` Camera CRUD — once that's added, every legacydb import can be replaced with the correct role-specific package. After all imports are switched, legacydb is deleted and depguard enforces the boundary in CI.

**Tech Stack:** Go 1.22+, SQLite via `modernc.org/sqlite`, Gin HTTP router, golangci-lint with depguard

**Spec:** `docs/superpowers/specs/2026-04-20-directory-recorder-consolidation-design.md`

---

## File Map

| File | Change | What |
|------|--------|------|
| `internal/directory/db/cameras.go` | Create | Camera type + 16 CRUD/update methods |
| `internal/directory/db/cameras_test.go` | Create | Camera CRUD tests |
| `internal/directory/cameraapi/handler.go` | Modify | Switch `legacydb` → `dirdb "internal/directory/db"` |
| `internal/directory/cameraapi/handler_test.go` | Modify | Same import switch |
| `internal/directory/boot.go` | Modify | Switch `legacydb` → `dirdb` for camera DB |
| `internal/directory/webhook/dispatcher.go` | Modify | Switch `legacydb` → `dirdb` |
| `internal/directory/webhook/dispatcher_test.go` | Modify | Same |
| `internal/recorder/recordingapi/handler.go` | Modify | Switch `legacydb` → `recdb "internal/recorder/db"` |
| `internal/recorder/detectionapi/handler.go` | Modify | Same |
| `internal/recorder/boot.go` | Modify | Switch `legacydb` → `recdb` |
| `internal/recorder/scheduler/scheduler.go` | Modify | Same |
| `internal/recorder/scheduler/detection.go` | Modify | Same |
| `internal/recorder/scheduler/detection_evaluator.go` | Modify | Same |
| `internal/recorder/ai/pipeline.go` | Modify | Same |
| `internal/recorder/ai/publisher.go` | Modify | Same |
| `internal/recorder/ai/search.go` | Modify | Same |
| `internal/recorder/ai/forensic/executor.go` | Modify | Same |
| `internal/recorder/alerts/email.go` | Modify | Same |
| `internal/recorder/alerts/evaluator.go` | Modify | Same |
| `internal/recorder/storage/manager.go` | Modify | Same |
| `internal/recorder/connmgr/connmgr.go` | Modify | Same |
| `internal/recorder/managed/internal_api.go` | Modify | Same |
| `internal/recorder/thumbnail/generator.go` | Modify | Same |
| `internal/recorder/recovery/adapter.go` | Modify | Same |
| `internal/shared/legacydb/` | Delete | Entire directory removed |
| `.golangci.yml` | Modify | Add depguard rules |
| `.github/workflows/lint.yml` | Create/Modify | Run golangci-lint in CI |

---

## Task 1: Add Camera type and CRUD to `directory/db`

**Files:**
- Create: `internal/directory/db/cameras.go`
- Create: `internal/directory/db/cameras_test.go`

- [ ] **Step 1: Write failing test for Camera CRUD**

Create `internal/directory/db/cameras_test.go`:

```go
package db_test

import (
	"context"
	"path/filepath"
	"testing"

	db "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(context.Background(), filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestCreateAndGetCamera(t *testing.T) {
	d := openTestDB(t)
	cam := &db.Camera{Name: "Front Door", RTSPURL: "rtsp://192.168.1.10/stream1"}
	err := d.CreateCamera(cam)
	require.NoError(t, err)
	require.NotEmpty(t, cam.ID)

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, "Front Door", got.Name)
	require.Equal(t, "rtsp://192.168.1.10/stream1", got.RTSPURL)
}

func TestListCameras(t *testing.T) {
	d := openTestDB(t)
	err := d.CreateCamera(&db.Camera{Name: "Cam1"})
	require.NoError(t, err)
	err = d.CreateCamera(&db.Camera{Name: "Cam2"})
	require.NoError(t, err)

	cams, err := d.ListCameras()
	require.NoError(t, err)
	require.Len(t, cams, 2)
}

func TestUpdateCamera(t *testing.T) {
	d := openTestDB(t)
	cam := &db.Camera{Name: "Original"}
	err := d.CreateCamera(cam)
	require.NoError(t, err)

	cam.Name = "Updated"
	err = d.UpdateCamera(cam)
	require.NoError(t, err)

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated", got.Name)
}

func TestDeleteCamera(t *testing.T) {
	d := openTestDB(t)
	cam := &db.Camera{Name: "ToDelete"}
	err := d.CreateCamera(cam)
	require.NoError(t, err)

	err = d.DeleteCamera(cam.ID)
	require.NoError(t, err)

	_, err = d.GetCamera(cam.ID)
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestGetCamera_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetCamera("nonexistent")
	require.ErrorIs(t, err, db.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/db/... -v -run TestCreateAndGetCamera
```

Expected: FAIL — `db.Camera` undefined.

- [ ] **Step 3: Create `internal/directory/db/cameras.go`**

Copy the Camera struct and all 16 methods from `internal/shared/legacydb/cameras.go`. Change the package declaration from `package legacydb` to `package db`. The file should contain:

```go
package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Camera represents a camera record in the database.
type Camera struct {
	ID                       string  `json:"id"`
	Name                     string  `json:"name"`
	ONVIFEndpoint            string  `json:"onvif_endpoint"`
	ONVIFUsername             string  `json:"onvif_username"`
	ONVIFPassword            string  `json:"-"`
	ONVIFProfileToken        string  `json:"onvif_profile_token"`
	RTSPURL                  string  `json:"rtsp_url"`
	PTZCapable               bool    `json:"ptz_capable"`
	MediaMTXPath             string  `json:"mediamtx_path"`
	Status                   string  `json:"status"`
	Tags                     string  `json:"tags"`
	RetentionDays            int     `json:"retention_days"`
	EventRetentionDays       int     `json:"event_retention_days"`
	DetectionRetentionDays   int     `json:"detection_retention_days"`
	SupportsPTZ              bool    `json:"supports_ptz"`
	SupportsImaging          bool    `json:"supports_imaging"`
	SupportsEvents           bool    `json:"supports_events"`
	SupportsRelay            bool    `json:"supports_relay"`
	SupportsAudioBackchannel bool    `json:"supports_audio_backchannel"`
	SnapshotURI              string  `json:"snapshot_uri,omitempty"`
	SupportsMedia2           bool    `json:"supports_media2"`
	SupportsAnalytics        bool    `json:"supports_analytics"`
	SupportsEdgeRecording    bool    `json:"supports_edge_recording"`
	ServiceCapabilities      string  `json:"service_capabilities,omitempty"`
	MotionTimeoutSeconds     int     `json:"motion_timeout_seconds"`
	SubStreamURL             string  `json:"sub_stream_url,omitempty"`
	AIEnabled                bool    `json:"ai_enabled"`
	AIStreamID               string  `json:"ai_stream_id,omitempty"`
	AITrackTimeout           int     `json:"ai_track_timeout"`
	AIConfidence             float64 `json:"ai_confidence"`
	AudioTranscode           bool    `json:"audio_transcode"`
	RecordingStreamID        string  `json:"recording_stream_id,omitempty"`
	StoragePath              string  `json:"storage_path"`
	QuotaBytes               int64   `json:"quota_bytes"`
	QuotaWarningPercent      int     `json:"quota_warning_percent"`
	QuotaCriticalPercent     int     `json:"quota_critical_percent"`
	SupportedEventTopics     string  `json:"supported_event_topics"`
	DeviceID                 string  `json:"device_id,omitempty"`
	ChannelIndex             *int    `json:"channel_index,omitempty"`
	MulticastEnabled         bool    `json:"multicast_enabled"`
	MulticastAddress         string  `json:"multicast_address"`
	MulticastPort            int     `json:"multicast_port"`
	MulticastTTL             int     `json:"multicast_ttl"`
	ConfidenceThresholds     string  `json:"confidence_thresholds,omitempty"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}
```

Then copy ALL 16 method implementations verbatim from `internal/shared/legacydb/cameras.go` (lines 68-528), changing only the receiver to use `*DB` from the directory/db package (which has the same name, so no change needed).

Verify: the `cameras` table already exists in the directory DB via migration `0015_admin_tables`. If it doesn't include all Camera columns, check and add a new migration for missing columns.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/directory/db/... -v -run TestCamera
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Verify full build**

```bash
go build ./internal/directory/db/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/directory/db/cameras.go internal/directory/db/cameras_test.go
git commit -m "feat: add Camera type and CRUD methods to directory/db

Copied from legacydb/cameras.go. Enables cameraapi to use
directory/db directly instead of legacydb."
```

---

## Task 2: Switch Directory packages from legacydb to directory/db

**Files:**
- Modify: `internal/directory/cameraapi/handler.go`
- Modify: `internal/directory/cameraapi/handler_test.go`
- Modify: `internal/directory/boot.go`
- Modify: `internal/directory/webhook/dispatcher.go`
- Modify: `internal/directory/webhook/dispatcher_test.go`

- [ ] **Step 1: Update cameraapi handler import**

In `internal/directory/cameraapi/handler.go`, replace:

```go
dirdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```

with:

```go
dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
```

No other code changes needed — the alias `dirdb` stays the same, and the types (`dirdb.DB`, `dirdb.Camera`, `dirdb.ErrNotFound`) now resolve to `directory/db`.

- [ ] **Step 2: Update cameraapi test import**

In `internal/directory/cameraapi/handler_test.go`, same replacement:

```go
dirdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```
→
```go
dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
```

- [ ] **Step 3: Update directory boot.go**

In `internal/directory/boot.go`, replace:

```go
nvrdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```

with:

```go
dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
```

Then update all references from `nvrdb.` to `dirdb.` — or keep the alias as `nvrdb` if there are many callsites. Whichever minimizes the diff.

- [ ] **Step 4: Update webhook dispatcher and test**

In `internal/directory/webhook/dispatcher.go` and `dispatcher_test.go`, replace the legacydb import with `directory/db`. Check what types/methods it uses and verify they exist in `directory/db`.

- [ ] **Step 5: Verify build and tests**

```bash
go build ./internal/directory/...
go test ./internal/directory/cameraapi/... -v -count=1
go test ./internal/directory/webhook/... -v -count=1
```

Expected: clean build, all tests pass.

- [ ] **Step 6: Verify no legacydb imports remain in directory/**

```bash
grep -rl 'internal/shared/legacydb' internal/directory/ --include='*.go'
```

Expected: zero results.

- [ ] **Step 7: Commit**

```bash
git add internal/directory/
git commit -m "refactor: switch directory/ packages from legacydb to directory/db"
```

---

## Task 3: Add missing types to recorder/db for recorder packages

**Files:**
- Modify: `internal/recorder/db/` — add any types referenced by recorder packages that don't exist yet

- [ ] **Step 1: Audit which legacydb types recorder packages use**

```bash
grep -rn 'legacydb\.' internal/recorder/ --include='*.go' | \
  sed 's/.*legacydb\.\([A-Za-z]*\).*/\1/' | sort -u
```

This shows every type/function accessed via legacydb. Check each one against `internal/recorder/db/`:

```bash
grep -rn 'type ' internal/recorder/db/*.go | grep -v test
grep -rn 'func ' internal/recorder/db/*.go | grep -v test | head -40
```

- [ ] **Step 2: Add any missing types to recorder/db**

For each type found in Step 1 that doesn't exist in `recorder/db`, copy it from `legacydb`. Common ones to check:
- `Camera` — recorder packages (scheduler, alerts, AI) reference camera config. The recorder shouldn't own the Camera type (that's directory), but it needs a local representation. Check if `internal/recorder/state/types.go` already has `CameraConfig` that can substitute. If so, create type aliases or adapter functions.
- `AlertRule` — used by `alerts/evaluator.go`
- `SMTPConfig` — used by `alerts/email.go`
- `Detection` — used by AI packages
- Any other struct types

For types that are truly directory-owned (Camera, AlertRule, SMTPConfig), create a minimal local type in `internal/recorder/types.go` that has only the fields the recorder needs. Don't copy the full 45-field Camera struct — use the existing `CameraConfig` from `recorder/state/types.go` where possible.

- [ ] **Step 3: Verify build**

```bash
go build ./internal/recorder/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/recorder/
git commit -m "feat: add missing types to recorder/db for legacydb migration"
```

---

## Task 4: Switch recorder API packages from legacydb to recorder/db

**Files:**
- Modify: `internal/recorder/recordingapi/handler.go`
- Modify: `internal/recorder/detectionapi/handler.go`

- [ ] **Step 1: Update recordingapi handler import**

In `internal/recorder/recordingapi/handler.go`, replace:

```go
recdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```

with:

```go
recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
```

All method calls use the `recdb` alias — `recdb.DB`, `recdb.Recording`, `recdb.Bookmark`, etc. Verify these types exist in `recorder/db`.

- [ ] **Step 2: Update detectionapi handler import**

Same replacement in `internal/recorder/detectionapi/handler.go`:

```go
recdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```
→
```go
recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/recorder/recordingapi/... ./internal/recorder/detectionapi/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/recorder/recordingapi/ internal/recorder/detectionapi/
git commit -m "refactor: switch recorder API packages from legacydb to recorder/db"
```

---

## Task 5: Switch recorder boot.go from legacydb to recorder/db

**Files:**
- Modify: `internal/recorder/boot.go`

- [ ] **Step 1: Update boot.go import**

In `internal/recorder/boot.go`, replace:

```go
nvrdb "github.com/bluenviron/mediamtx/internal/shared/legacydb"
```

with:

```go
recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
```

Update all `nvrdb.` references to `recdb.` — this includes:
- `nvrdb.Open()` → `recdb.Open()` 
- Any `*nvrdb.DB` field types → `*recdb.DB`
- The RecorderServer struct field

- [ ] **Step 2: Verify build**

```bash
go build ./internal/recorder/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "refactor: switch recorder boot.go from legacydb to recorder/db"
```

---

## Task 6: Switch remaining recorder packages from legacydb

**Files:**
- Modify: `internal/recorder/scheduler/scheduler.go`
- Modify: `internal/recorder/scheduler/detection.go`
- Modify: `internal/recorder/scheduler/detection_evaluator.go`
- Modify: `internal/recorder/scheduler/detection_test.go`
- Modify: `internal/recorder/scheduler/scheduler_test.go`
- Modify: `internal/recorder/ai/pipeline.go`
- Modify: `internal/recorder/ai/publisher.go`
- Modify: `internal/recorder/ai/search.go`
- Modify: `internal/recorder/ai/forensic/executor.go`
- Modify: `internal/recorder/alerts/email.go`
- Modify: `internal/recorder/alerts/evaluator.go`
- Modify: `internal/recorder/storage/manager.go`
- Modify: `internal/recorder/storage/sync_test.go`
- Modify: `internal/recorder/storage/integration_test.go`
- Modify: `internal/recorder/connmgr/connmgr.go`
- Modify: `internal/recorder/managed/internal_api.go`
- Modify: `internal/recorder/thumbnail/generator.go`
- Modify: `internal/recorder/recovery/adapter.go`

- [ ] **Step 1: Batch-replace all legacydb imports in recorder/**

```bash
find internal/recorder/ -name '*.go' -exec grep -l 'internal/shared/legacydb' {} \; | while read f; do
  sed -i '' 's|github.com/bluenviron/mediamtx/internal/shared/legacydb|github.com/bluenviron/mediamtx/internal/recorder/db|g' "$f"
done
```

- [ ] **Step 2: Fix any alias mismatches**

Some files use `nvrdb` or `db` as the alias for legacydb. After the import path change, verify the alias still makes sense. Where the alias was `nvrdb`, consider renaming to `recdb` for clarity — but only if it doesn't cause a large diff.

- [ ] **Step 3: Handle type mismatches**

Some files may reference `legacydb.Camera` or `legacydb.AlertRule` — types that might not exist in `recorder/db`. For each:
- If the type exists in `recorder/db` under the same name → no change needed
- If it exists under a different name → update the reference
- If it doesn't exist → add it to `recorder/db` (from Task 3) or use an interface

- [ ] **Step 4: Verify build**

```bash
go build ./internal/recorder/...
```

Fix any compilation errors. Common issues:
- Missing type → add to `recorder/db`
- Missing method → copy from legacydb
- Different method signature → adapt the caller

- [ ] **Step 5: Run existing tests**

```bash
go test ./internal/recorder/... -count=1 -timeout 120s 2>&1 | grep -E 'ok|FAIL'
```

- [ ] **Step 6: Verify no legacydb imports remain in recorder/**

```bash
grep -rl 'internal/shared/legacydb' internal/recorder/ --include='*.go'
```

Expected: zero results.

- [ ] **Step 7: Commit**

```bash
git add internal/recorder/
git commit -m "refactor: switch all recorder packages from legacydb to recorder/db

Updated 18 files across scheduler, ai, alerts, storage, connmgr,
managed, thumbnail, and recovery packages."
```

---

## Task 7: Delete legacydb

**Files:**
- Delete: `internal/shared/legacydb/` (entire directory, ~70 files)

- [ ] **Step 1: Verify zero imports remain**

```bash
grep -rl 'internal/shared/legacydb' --include='*.go' .
```

Expected: zero results. If any remain, fix them first.

- [ ] **Step 2: Delete legacydb**

```bash
git rm -r internal/shared/legacydb/
```

- [ ] **Step 3: Verify full build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 4: Run key tests**

```bash
go test ./internal/directory/... ./internal/recorder/db/... ./internal/recorder/recordingapi/... ./internal/recorder/detectionapi/... -count=1 2>&1 | grep -E 'ok|FAIL'
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: delete internal/shared/legacydb/ — all consumers migrated

All 26 files now use role-specific packages:
- directory/ → directory/db
- recorder/ → recorder/db"
```

---

## Task 8: Add depguard linter rules and CI enforcement

**Files:**
- Modify: `.golangci.yml`
- Create or Modify: `.github/workflows/lint.yml` (if not already present)

- [ ] **Step 1: Update `.golangci.yml` with depguard rules**

Read the existing `.golangci.yml` to understand its structure, then add:

```yaml
linters:
  enable:
    - depguard

linters-settings:
  depguard:
    rules:
      directory-boundary:
        list-mode: lax
        files:
          - "internal/directory/**/*.go"
        deny:
          - pkg: "github.com/bluenviron/mediamtx/internal/recorder"
            desc: "directory must not import recorder — use internal/shared"
          - pkg: "github.com/bluenviron/mediamtx/internal/shared/legacydb"
            desc: "legacydb is deleted — use directory/db"
      recorder-boundary:
        list-mode: lax
        files:
          - "internal/recorder/**/*.go"
        deny:
          - pkg: "github.com/bluenviron/mediamtx/internal/directory"
            desc: "recorder must not import directory — use internal/shared"
          - pkg: "github.com/bluenviron/mediamtx/internal/shared/legacydb"
            desc: "legacydb is deleted — use recorder/db"
```

- [ ] **Step 2: Run linter locally**

```bash
golangci-lint run ./internal/directory/... ./internal/recorder/... --enable depguard
```

Expected: no violations.

- [ ] **Step 3: Add CI workflow (if not present)**

Check if `.github/workflows/` has a lint workflow. If not, create `.github/workflows/lint.yml`:

```yaml
name: Lint
on: [push, pull_request]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: ./internal/...
```

If a lint workflow already exists, verify it includes `depguard` in the enabled linters.

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml .github/workflows/
git commit -m "chore: add depguard rules enforcing directory/recorder boundary

Prevents cross-imports between directory and recorder packages.
Also blocks any re-introduction of legacydb."
```

---

## Task 9: Final verification

**Files:** None — verification only.

- [ ] **Step 1: Full build**

```bash
go build ./...
```

- [ ] **Step 2: Verify no legacydb directory**

```bash
ls internal/shared/legacydb/ 2>&1
```

Expected: "No such file or directory"

- [ ] **Step 3: Verify no legacydb imports**

```bash
grep -r 'internal/shared/legacydb' --include='*.go' .
```

Expected: zero matches.

- [ ] **Step 4: Verify no cross-boundary imports**

```bash
grep -r '"github.com/bluenviron/mediamtx/internal/recorder' internal/directory/ --include='*.go'
grep -r '"github.com/bluenviron/mediamtx/internal/directory' internal/recorder/ --include='*.go'
```

Expected: zero matches for both.

- [ ] **Step 5: Run full test suite**

```bash
go test ./internal/directory/... ./internal/recorder/... ./internal/shared/... -count=1 -timeout 300s 2>&1 | grep -E 'ok|FAIL'
```

Expected: all pass.

- [ ] **Step 6: Commit verification**

```bash
git log --oneline -10
```

Verify the commit history tells a clear story.
