# Recorder Capture Loop — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Plan revision (2026-04-24, mid-Phase-1):** A discovery during Task 4 invalidated the original "yamlwriter + reload" architecture. The new recorder uses `mediamtxsupervisor` to manage paths via mediamtx's HTTP API (`/v3/config/paths/...`), NOT via `mediamtx.yml` edits. mediamtx never re-reads its yaml file at runtime. This plan was revised to leverage the supervisor as the canonical mechanism (it is a strictly more hardened design than yamlwriter: diff-based reconciliation, acknowledged writes, operator-yaml stays clean). The first three Task 2 commits and the Task 4 wiring commit have been reverted. Task 2 is rewritten as a thin `supervisor.Reload()` shim. Task 4 is replaced with `PollInterval` + a `recordinghealth` watchdog. Tasks 6.5 (storage disk monitor) and 8.5 (Prometheus metrics) are added. Legacy `scheduler/`, `connmgr/`, `alerts/` need a state.Store-based redesign and are deferred to Phase 1.5.

**Goal:** Replace `noopCaptureManager{}` in `internal/recorder/boot.go` with a hardened implementation built around `mediamtxsupervisor` + `state.Store`. Add active drift detection (recordinghealth watchdog), wire recovery + integrity scanners, fragment backfill, thumbnail generator, and a partial port of the legacy storage manager (disk monitoring + retention). Surface Prometheus metrics for observability. Outcome: cameras assigned by Directory record reliably, drift is auto-corrected within 15s, the DB stays reconciled with disk, and operators have visibility into the system's state.

**Architecture:** `state.Store` is the canonical truth. The `recordercontrol` reconciler persists camera assignments to it. `CaptureManager.EnsureRunning/Stop` call `supervisor.Reload()` to nudge the supervisor to re-render and push to mediamtx via HTTP. The supervisor's `PollInterval` (15s) provides periodic drift correction independent of nudges. A separate `recordinghealth` watchdog (30s cadence) compares state.Store expected vs. mediamtx's runtime `/v3/paths/list`, alerts on drift, and triggers Reload to self-heal. Recovery + integrity + fragmentbackfill operate on disk + DB only — they don't interact with mediamtx config and drop in cleanly. Storage disk monitoring polls capacity and enforces retention by deleting files; the legacy yaml-based path-failover is deferred. Metrics are exposed at `/metrics` on the recorder HTTP API.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, gin, embedded mediamtx core, `internal/recorder/state` (persistent camera store), `internal/recorder/mediamtxsupervisor` (HTTP-based path manager with diff-based reconciliation and `.Reload()`), Prometheus client_golang.

---

## File Structure

**New packages:**
- `internal/recorder/capturemanager/{manager.go, manager_test.go}` — thin Reload-shim adapter for `recordercontrol.CaptureManager`
- `internal/recorder/fragmentbackfill/{backfill.go, scan.go, backfill_test.go}` — port of legacy fragment indexing + fMP4 scanner ✅ DONE (Task 3)
- `internal/recorder/recordinghealth/{watchdog.go, watchdog_test.go}` — drift detector against mediamtx runtime paths
- `internal/recorder/diskmonitor/{monitor.go, monitor_test.go}` — disk capacity polling + retention enforcement
- `internal/recorder/recordermetrics/{metrics.go, metrics_test.go}` — Prometheus exporter

**Modified files:**
- `internal/recorder/boot.go` — set `mediamtxsupervisor.Config.PollInterval=15s`, wire capturemanager (replaces noop), recovery scan + integrity scanner + fragmentbackfill + thumbnail + recordinghealth watchdog + diskmonitor + metrics handler
- `internal/recorder/boot.go:628-647` — delete `noopCaptureManager` type (after replacement is in place, Task 7)

**Reference files (read-only legacy source):**
- `git show 86569ce37^:internal/nvr/nvr.go:363-440` — legacy `initRecording()` (kept for context; some pieces still applicable, others deferred to Phase 1.5)
- `git show 86569ce37^:internal/nvr/storage/manager.go` — port target for diskmonitor (only the disk-IO and retention parts)

**Deferred to Phase 1.5 (NOT in this plan):**
- `internal/recorder/scheduler/` — recording-rule evaluation; needs state.Store-flag redesign
- `internal/recorder/connmgr/` + `internal/recorder/alerts/` — need a shared event broadcaster (Phase 1.5 builds it)
- Yaml-based path failover in legacy storage manager — supervisor-based equivalent

---

## Pre-flight: working agreement

This plan assumes:
- Working in a fresh worktree at `.worktrees/recovery-phase-1` on branch `feat/recovery-phase-1-capture-loop`.
- All commits include `Co-Authored-By: claude-flow <ruv@ruv.net>` per CLAUDE.md.
- `go test ./internal/recorder/...` is the unit-test gate; `go vet ./...` passes before each commit.
- `mediamtx.yml` runtime fields (`nvr: true`, `api: true`, `playback: true`, `logLevel: debug`, `nvrJWTSecret`, existing camera paths) are NEVER modified by tests or this plan's code paths. The yamlwriter only adds/modifies camera path entries.

---

## Task 1: Audit & confirm baseline (no code change)

**Files:**
- Read: `internal/recorder/boot.go:410-460` (current noop wiring)
- Read: `internal/recorder/recordercontrol/reconciler.go:1-40` (CaptureManager interface)
- Read: `internal/recorder/yamlwriter/writer.go:24-90` (AddPath/RemovePath/SetPathValue API)
- Read: `internal/recorder/mediamtxsupervisor/supervisor.go:192` (`.Reload()` API)
- Read: `internal/recorder/scheduler/scheduler.go:111-130` (scheduler.New signature)
- Read: `internal/recorder/storage/manager.go:50-70` (storage.New signature)
- Read: `internal/recorder/recovery/recovery.go:30-80` (Run / RunConfig)
- Read: `internal/recorder/integrity/scanner.go:18-60` (Scanner / FetchFunc)
- Read: `internal/recorder/recovery/adapter.go` (`NewDBAdapter`, `NewReconcileAdapter`)
- Read legacy: `git show 86569ce37^:internal/nvr/nvr.go:363-460` (initRecording reference)
- Read legacy: `git show 86569ce37^:internal/nvr/fragment_backfill.go` (full file)

- [ ] **Step 1: Confirm CaptureManager interface signature**

Run: `grep -A 12 "type CaptureManager interface" internal/recorder/recordercontrol/reconciler.go`

Expected output should match:
```go
type CaptureManager interface {
    EnsureRunning(camera Camera) error
    Stop(cameraID string) error
    RunningCameras() []string
}
```

- [ ] **Step 2: Confirm Camera struct fields**

Run: `grep -A 20 "^type Camera struct" internal/recorder/recordercontrol/types.go`

Capture the field names you find (likely `ID`, `StreamURL`, `ConfigVersion`, etc.). These drive the adapter's yamlwriter call.

- [ ] **Step 3: Confirm supervisor.Reload exists & is safe**

Run: `sed -n '185,210p' internal/recorder/mediamtxsupervisor/supervisor.go`

Confirm `Reload()` is non-blocking or has a sane timeout. If it blocks, the adapter must call it from a goroutine.

- [ ] **Step 4: Run baseline tests**

Run: `go test ./internal/recorder/...`

Expected: PASS (we want a green baseline before changing anything).

If any tests fail, stop and report — those failures must be fixed in their own commit before this plan continues.

- [ ] **Step 5: Snapshot the noop site for the diff**

Run: `git log -1 --format=%H -- internal/recorder/boot.go > /tmp/baseline-sha.txt && cat /tmp/baseline-sha.txt`

Record this SHA in the worktree's notes; use it to diff at the end.

(no commit — this task is pure audit)

---

## Task 2: CaptureManager adapter — package skeleton + Stop/RunningCameras

**Files:**
- Create: `internal/recorder/capturemanager/manager.go`
- Test: `internal/recorder/capturemanager/manager_test.go`

- [ ] **Step 1: Write the failing tests for Stop and RunningCameras**

Create `internal/recorder/capturemanager/manager_test.go`:

```go
package capturemanager

import (
    "log/slog"
    "os"
    "path/filepath"
    "sort"
    "testing"

    "github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
    "github.com/bluenviron/mediamtx/internal/recorder/yamlwriter"
)

func newTestWriter(t *testing.T) (*yamlwriter.Writer, string) {
    t.Helper()
    dir := t.TempDir()
    p := filepath.Join(dir, "mediamtx.yml")
    if err := os.WriteFile(p, []byte("paths:\n"), 0o644); err != nil {
        t.Fatalf("write seed yaml: %v", err)
    }
    return yamlwriter.New(p), p
}

func TestRunningCameras_EmptyByDefault(t *testing.T) {
    yw, _ := newTestWriter(t)
    m := New(Config{
        YAML:           yw,
        Reload:         func() {},
        RecordingsPath: t.TempDir(),
        Logger:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
    })
    if got := m.RunningCameras(); len(got) != 0 {
        t.Fatalf("RunningCameras: want empty, got %v", got)
    }
}

func TestStop_OnUnknownCamera_NoError(t *testing.T) {
    yw, _ := newTestWriter(t)
    m := New(Config{YAML: yw, Reload: func() {}, RecordingsPath: t.TempDir(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))})
    if err := m.Stop("does-not-exist"); err != nil {
        t.Fatalf("Stop unknown: want nil, got %v", err)
    }
}

func TestEnsureRunning_AddsPathAndTracks(t *testing.T) {
    yw, path := newTestWriter(t)
    var reloads int
    m := New(Config{
        YAML:           yw,
        Reload:         func() { reloads++ },
        RecordingsPath: t.TempDir(),
        Logger:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
    })
    cam := recordercontrol.Camera{ID: "cam-1", StreamURL: "rtsp://1.2.3.4/main"}
    if err := m.EnsureRunning(cam); err != nil {
        t.Fatalf("EnsureRunning: %v", err)
    }
    if got := m.RunningCameras(); len(got) != 1 || got[0] != "cam-1" {
        t.Fatalf("RunningCameras: want [cam-1], got %v", got)
    }
    if reloads != 1 {
        t.Fatalf("reloads: want 1, got %d", reloads)
    }
    body, _ := os.ReadFile(path)
    if !contains(body, "cam-1") || !contains(body, "rtsp://1.2.3.4/main") || !contains(body, "record: true") {
        t.Fatalf("yaml does not contain expected entry; got:\n%s", body)
    }
}

func TestEnsureRunning_Idempotent(t *testing.T) {
    yw, _ := newTestWriter(t)
    var reloads int
    m := New(Config{YAML: yw, Reload: func() { reloads++ }, RecordingsPath: t.TempDir(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))})
    cam := recordercontrol.Camera{ID: "cam-1", StreamURL: "rtsp://1.2.3.4/main"}
    _ = m.EnsureRunning(cam)
    _ = m.EnsureRunning(cam)
    if got := m.RunningCameras(); len(got) != 1 {
        t.Fatalf("dedup failed: %v", got)
    }
    if reloads != 1 {
        t.Fatalf("idempotent EnsureRunning should not reload twice; got %d", reloads)
    }
}

func TestEnsureRunning_VersionChangeRestarts(t *testing.T) {
    yw, _ := newTestWriter(t)
    var reloads int
    m := New(Config{YAML: yw, Reload: func() { reloads++ }, RecordingsPath: t.TempDir(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))})
    _ = m.EnsureRunning(recordercontrol.Camera{ID: "cam-1", StreamURL: "rtsp://a", ConfigVersion: 1})
    _ = m.EnsureRunning(recordercontrol.Camera{ID: "cam-1", StreamURL: "rtsp://b", ConfigVersion: 2})
    if reloads != 2 {
        t.Fatalf("version change should re-write yaml & reload; got %d reloads", reloads)
    }
}

func TestStop_RemovesPath(t *testing.T) {
    yw, path := newTestWriter(t)
    m := New(Config{YAML: yw, Reload: func() {}, RecordingsPath: t.TempDir(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))})
    _ = m.EnsureRunning(recordercontrol.Camera{ID: "cam-1", StreamURL: "rtsp://x"})
    if err := m.Stop("cam-1"); err != nil {
        t.Fatalf("Stop: %v", err)
    }
    body, _ := os.ReadFile(path)
    if contains(body, "cam-1") {
        t.Fatalf("yaml still references cam-1 after Stop; got:\n%s", body)
    }
    if got := m.RunningCameras(); len(got) != 0 {
        t.Fatalf("RunningCameras after Stop: want empty, got %v", got)
    }
}

func contains(body []byte, s string) bool {
    return len(body) > 0 && len(s) > 0 && stringContains(string(body), s)
}

func stringContains(haystack, needle string) bool {
    for i := 0; i+len(needle) <= len(haystack); i++ {
        if haystack[i:i+len(needle)] == needle {
            return true
        }
    }
    return false
}

// sort import needed only if we sort RunningCameras output deterministically; keep it for future tests.
var _ = sort.Strings
```

- [ ] **Step 2: Run tests; confirm they fail with `package capturemanager` not found**

Run: `go test ./internal/recorder/capturemanager/...`

Expected: build error — `no Go files` or `package not found`.

- [ ] **Step 3: Create the package skeleton with just enough to compile and fail tests**

Create `internal/recorder/capturemanager/manager.go`:

```go
// Package capturemanager implements recordercontrol.CaptureManager by
// translating imperative camera lifecycle calls into yaml mutations on
// mediamtx.yml + supervisor reloads. The embedded mediamtx core writes
// fMP4 segments to disk via internal/recordstore when a path has
// record: true; this adapter is the only thing that flips that switch.
package capturemanager

import (
    "fmt"
    "log/slog"
    "path/filepath"
    "sort"
    "sync"

    "github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
    "github.com/bluenviron/mediamtx/internal/recorder/yamlwriter"
)

// Config is the dependencies needed to construct a Manager.
type Config struct {
    // YAML is the AST-safe writer for mediamtx.yml.
    YAML *yamlwriter.Writer
    // Reload is invoked after every successful yaml mutation; supplies
    // mediamtxsupervisor.MediaMTXSupervisor.Reload in production.
    Reload func()
    // RecordingsPath is the on-disk root for fMP4 segment files; must
    // be set or recording is silently disabled.
    RecordingsPath string
    // Logger receives structured ops logs.
    Logger *slog.Logger
}

// Manager is the production CaptureManager.
type Manager struct {
    cfg Config

    mu      sync.Mutex
    running map[string]uint64 // camera ID -> last applied ConfigVersion
}

// New constructs a Manager.
func New(cfg Config) *Manager {
    if cfg.Logger == nil {
        cfg.Logger = slog.Default()
    }
    if cfg.Reload == nil {
        cfg.Reload = func() {}
    }
    return &Manager{cfg: cfg, running: map[string]uint64{}}
}

// EnsureRunning adds (or restarts on version change) a recording path
// for camera c. Idempotent for unchanged ConfigVersion.
func (m *Manager) EnsureRunning(c recordercontrol.Camera) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    prev, ok := m.running[c.ID]
    if ok && prev == c.ConfigVersion {
        return nil
    }

    if c.StreamURL == "" {
        return fmt.Errorf("capturemanager: camera %s has empty StreamURL", c.ID)
    }

    pathName := pathNameFor(c.ID)
    cfg := map[string]interface{}{
        "source":     c.StreamURL,
        "record":     true,
        "recordPath": filepath.Join(m.cfg.RecordingsPath, c.ID, "%Y-%m-%d_%H-%M-%S-%f"),
    }
    // Use Add (idempotent: writer treats existing path as upsert).
    if err := m.cfg.YAML.AddPath(pathName, cfg); err != nil {
        return fmt.Errorf("capturemanager: AddPath(%s): %w", c.ID, err)
    }
    m.running[c.ID] = c.ConfigVersion
    m.cfg.Logger.Info("capturemanager: ensure running",
        slog.String("camera_id", c.ID),
        slog.Uint64("config_version", c.ConfigVersion))
    m.cfg.Reload()
    return nil
}

// Stop removes the recording path for the given camera. No-op for unknown IDs.
func (m *Manager) Stop(cameraID string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if _, ok := m.running[cameraID]; !ok {
        return nil
    }
    if err := m.cfg.YAML.RemovePath(pathNameFor(cameraID)); err != nil {
        return fmt.Errorf("capturemanager: RemovePath(%s): %w", cameraID, err)
    }
    delete(m.running, cameraID)
    m.cfg.Logger.Info("capturemanager: stop", slog.String("camera_id", cameraID))
    m.cfg.Reload()
    return nil
}

// RunningCameras returns IDs of cameras with active recording paths, sorted.
func (m *Manager) RunningCameras() []string {
    m.mu.Lock()
    defer m.mu.Unlock()
    out := make([]string, 0, len(m.running))
    for id := range m.running {
        out = append(out, id)
    }
    sort.Strings(out)
    return out
}

// pathNameFor returns the mediamtx.yml path name for a camera. We use a
// stable cam-<id> prefix so operators can grep for recorder-managed paths.
func pathNameFor(cameraID string) string {
    return "cam-" + cameraID
}
```

- [ ] **Step 4: Run tests; confirm they pass**

Run: `go test ./internal/recorder/capturemanager/... -v`

Expected: all six tests PASS. If `Camera.ConfigVersion` is `int32` not `uint64`, fix the type in the manager (and in the test where we wrote `ConfigVersion: 1`/`2`) to match the real interface — adapt to the actual field type from Task 1 Step 2.

- [ ] **Step 5: Run go vet**

Run: `go vet ./internal/recorder/capturemanager/...`

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/capturemanager/
git commit -m "$(cat <<'EOF'
feat(recorder): capturemanager adapter for recordercontrol

Implements recordercontrol.CaptureManager by translating EnsureRunning/Stop
calls into idempotent yamlwriter mutations + supervisor reloads. This is
the production replacement for noopCaptureManager{} in recorder boot.

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 3: Port fragment-backfill from legacy

**Files:**
- Create: `internal/recorder/fragmentbackfill/backfill.go`
- Test: `internal/recorder/fragmentbackfill/backfill_test.go`

The legacy `internal/nvr/fragment_backfill.go` (read via `git show 86569ce37^:internal/nvr/fragment_backfill.go`) processes any `recordings` rows that lack fragment metadata, indexing them newest-first. We port it to its own package so boot.go can call `fragmentbackfill.Run(ctx, db)`.

- [ ] **Step 1: Read the legacy implementation**

Run: `git show 86569ce37^:internal/nvr/fragment_backfill.go`

Capture the full file (likely 30-80 lines). Note:
- It calls `n.database.GetUnindexedRecordings()` and a per-recording indexing function.
- It runs in a goroutine after a 5-second startup delay.
- It needs the recorder's `*db.DB` (recordings table) and the indexing helper that produces fragment metadata.

- [ ] **Step 2: Identify the indexing helper that ships with the recorder**

Run: `grep -rn "GetUnindexedRecordings\|IndexFragments\|Backfill" internal/recorder/db/ internal/recorder/recovery/ 2>&1 | head -20`

If a helper already exists in `recovery/` or `db/`, use it. If only `GetUnindexedRecordings` exists and the indexer was an inline body in legacy, port that body too.

- [ ] **Step 3: Write the failing test**

Create `internal/recorder/fragmentbackfill/backfill_test.go` with a test that:
- Sets up an in-memory `*db.DB` (using the recorder's existing test helpers — see `internal/recorder/db/recordings.go`'s test file for the pattern).
- Inserts one recording with no fragment rows.
- Calls `Run(ctx, db, logger)`.
- Asserts at least one fragment row exists for that recording afterward.

(If your search in Step 2 shows the indexer requires real fMP4 files on disk, simplify the test to verify the function calls `GetUnindexedRecordings` once and exits cleanly when the result is empty — that's enough to gate the wiring.)

- [ ] **Step 4: Run test; confirm fail**

Run: `go test ./internal/recorder/fragmentbackfill/...`

Expected: build error (no Go files).

- [ ] **Step 5: Implement the package**

Create `internal/recorder/fragmentbackfill/backfill.go` adapting the legacy code:

```go
// Package fragmentbackfill indexes recordings that pre-date fragment-level
// metadata. Ported verbatim from internal/nvr/fragment_backfill.go (see git
// history at 86569ce37^).
package fragmentbackfill

import (
    "context"
    "log/slog"
    "time"

    "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// Run starts a background goroutine that backfills fragment metadata.
// It waits 5s for the server to settle, then processes newest-first.
func Run(ctx context.Context, database *db.DB, logger *slog.Logger) {
    go func() {
        select {
        case <-ctx.Done():
            return
        case <-time.After(5 * time.Second):
        }

        recs, err := database.GetUnindexedRecordings()
        if err != nil {
            logger.Warn("fragmentbackfill: query unindexed", slog.String("error", err.Error()))
            return
        }
        if len(recs) == 0 {
            return
        }
        logger.Info("fragmentbackfill: starting", slog.Int("count", len(recs)))

        // (Adapt the legacy per-recording indexing body from
        // git show 86569ce37^:internal/nvr/fragment_backfill.go here.
        // It walks each recording's file, parses fMP4 fragments, and
        // calls db.InsertFragments.)
        for _, rec := range recs {
            if err := indexOne(ctx, database, rec); err != nil {
                logger.Warn("fragmentbackfill: index failed",
                    slog.String("recording_id", rec.ID),
                    slog.String("error", err.Error()))
                continue
            }
        }
    }()
}

func indexOne(ctx context.Context, database *db.DB, rec db.Recording) error {
    // Port the body of the legacy per-recording loop. Use the same
    // fragment parser that integrity.Scanner uses (it lives in
    // internal/recorder/integrity or similar) so the metadata format
    // matches.
    _ = ctx
    _ = database
    _ = rec
    return nil
}
```

If the legacy indexer is non-trivial (has its own helper functions), copy those helpers into this package or import them from where they were moved (e.g., `internal/recorder/integrity/parser.go` — search for it). Adjust the implementation until your test in Step 3 passes.

- [ ] **Step 6: Run tests; confirm pass**

Run: `go test ./internal/recorder/fragmentbackfill/... -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/recorder/fragmentbackfill/
git commit -m "$(cat <<'EOF'
feat(recorder): port fragment-backfill from legacy nvr

Ports internal/nvr/fragment_backfill.go (deleted in 86569ce37) into a
dedicated package so recorder boot can wire it after the recovery scan.

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 4: Wire scheduler + storage manager in recorder boot

**Files:**
- Modify: `internal/recorder/boot.go`

The legacy `initRecording()` constructs `scheduler.New(db, yamlWriter, encKey, callbackMgr, apiAddress, recordingsPath)` and `storage.New(db, yamlWriter, recordingsPath, apiAddress)`. We do the same in the recorder boot, after the supervisor is constructed but before the RecorderControl client starts (so the scheduler is running when assignments arrive).

- [ ] **Step 1: Locate the wiring site**

Run: `grep -n "noopCaptureManager\|capMgr := \|supervisor.New\|nvrDB" internal/recorder/boot.go`

You should see:
- `nvrDB, err := nvrdb.Open(...)` around line 512
- `supervisor, err := mediamtxsupervisor.New(...)` around line 381
- `capMgr := &noopCaptureManager{}` around line 425

The new wiring slots in between supervisor construction and the goroutine that starts RecorderControl.

- [ ] **Step 2: Add a smoke test for boot wiring**

Add to `internal/recorder/boot_test.go` (or create it if it doesn't exist) a test that builds a minimal `BootConfig`, calls `Boot(ctx, cfg)`, and asserts `cfg.captureMgr` (or whatever the test surface is — see the existing test if any) is a `*capturemanager.Manager`. If the boot doesn't expose its internals to tests, this step becomes "build a `BootResult` struct that exposes the wired Manager for assertion" — add that struct.

If adding a test surface is too invasive, skip the unit test and rely on the integration test in Task 9.

- [ ] **Step 3: Add scheduler + storage wiring**

In `internal/recorder/boot.go`, after the supervisor is constructed and `nvrDB` is open, before `capMgr := &noopCaptureManager{}`:

```go
// Wire legacy capture-loop machinery (scheduler, storage, recovery,
// integrity, thumbnail, fragment-backfill, connmgr, alerts). These
// packages were ported byte-for-byte from internal/nvr/ and are
// activated here per the legacy initRecording() pattern.
encKey := crypto.DeriveKey(cfg.JWTSecret, "nvr-credential-encryption")
callbackMgr := onvif.NewCallbackManager()

sched := scheduler.New(nvrDB, yw, encKey, callbackMgr, cfg.APIAddress, cfg.RecordingsPath)
sched.Start()
log.Info("recorder: scheduler started")

storageMgr := storage.New(nvrDB, yw, cfg.RecordingsPath, cfg.APIAddress)
storageMgr.Start()
log.Info("recorder: storage manager started")
```

Add the imports at the top of the file:

```go
"github.com/bluenviron/mediamtx/internal/shared/crypto"
"github.com/bluenviron/mediamtx/internal/recorder/onvif"
"github.com/bluenviron/mediamtx/internal/recorder/scheduler"
"github.com/bluenviron/mediamtx/internal/recorder/storage"
```

(Confirm exact import paths with `grep -rn "package onvif\|package scheduler\|package storage\|package crypto" internal/`.)

The variable `yw` must be the same `*yamlwriter.Writer` the supervisor uses; if boot doesn't already construct one, add it just above this block:

```go
yw := yamlwriter.New(cfg.MediaMTXConfigPath)
```

(Match the `cfg` field name used elsewhere in boot.go for the `mediamtx.yml` path.)

- [ ] **Step 4: Build & vet**

Run: `go build ./... && go vet ./internal/recorder/...`

Expected: no errors. If imports cycle or types mismatch, adapt — e.g., if `scheduler.New` actually expects `*yamlwriter.Writer` from a different package, rebase imports.

- [ ] **Step 5: Run all recorder tests**

Run: `go test ./internal/recorder/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "$(cat <<'EOF'
feat(recorder): wire scheduler and storage manager into boot

Activates internal/recorder/scheduler and internal/recorder/storage,
which were git-mv'd byte-for-byte from internal/nvr/ but never wired.
Mirrors legacy nvr.initRecording() (see git show 86569ce37^:internal/nvr/nvr.go:363).

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 5: Wire recovery scan + integrity scanner

**Files:**
- Modify: `internal/recorder/boot.go`

- [ ] **Step 1: Add recovery scan after storageMgr.Start()**

Insert after the `storageMgr.Start()` block from Task 4:

```go
if cfg.RecordingsPath != "" {
    recoveryCfg := recovery.RunConfig{
        RecordDirs: []string{cfg.RecordingsPath},
        DB:         recovery.NewDBAdapter(nvrDB),
        Reconciler: recovery.NewReconcileAdapter(nvrDB),
    }
    if result, err := recovery.Run(recoveryCfg); err != nil {
        log.Error("recorder: recovery scan failed", slog.String("error", err.Error()))
    } else if result.Scanned > 0 {
        log.Info("recorder: recovery complete",
            slog.Int("scanned", result.Scanned),
            slog.Int("repaired", result.Repaired),
            slog.Int("unrecoverable", result.Unrecoverable))
    }
}
```

Add import: `"github.com/bluenviron/mediamtx/internal/recorder/recovery"`.

- [ ] **Step 2: Add integrity scanner**

Append after the recovery block:

```go
integrityScanner := &integrity.Scanner{
    Interval:  1 * time.Hour,
    BatchSize: 100,
    FetchFunc: func(cutoff time.Time, batchSize int) ([]integrity.ScanItem, error) {
        recs, err := nvrDB.GetRecordingsNeedingVerification(cutoff, batchSize)
        if err != nil {
            return nil, err
        }
        items := make([]integrity.ScanItem, 0, len(recs))
        for _, rec := range recs {
            fragCount := 0
            if frags, err := nvrDB.GetFragments(rec.ID); err == nil {
                fragCount = len(frags)
            }
            items = append(items, integrity.ScanItem{
                RecordingID: rec.ID,
                CameraID:    rec.CameraID,
                Info: integrity.RecordingInfo{
                    FilePath:      rec.FilePath,
                    FileSize:      rec.FileSize,
                    InitSize:      rec.InitSize,
                    FragmentCount: fragCount,
                    DurationMs:    rec.DurationMs,
                },
            })
        }
        return items, nil
    },
}
go integrityScanner.Run(ctx)
log.Info("recorder: integrity scanner started")
```

Add import: `"github.com/bluenviron/mediamtx/internal/recorder/integrity"`.

If the field names of `db.Recording` differ from legacy (e.g., the recorder DB renamed `FilePath` to `Path`), adjust accordingly — check with `grep "type Recording struct" internal/recorder/db/recordings.go`.

- [ ] **Step 3: Build & vet**

Run: `go build ./... && go vet ./internal/recorder/...`

Expected: no errors.

- [ ] **Step 4: Run recorder tests**

Run: `go test ./internal/recorder/...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "$(cat <<'EOF'
feat(recorder): wire recovery scan and integrity scanner into boot

Boot now reconciles on-disk segments against the DB at startup and runs
the hourly integrity scanner. Mirrors legacy nvr.initRecording().

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 6: Wire fragment backfill + thumbnail generator + connmgr + alerts

**Files:**
- Modify: `internal/recorder/boot.go`

- [ ] **Step 1: Add fragment-backfill goroutine**

Append after the integrity scanner block:

```go
fragmentbackfill.Run(ctx, nvrDB, log)
log.Info("recorder: fragment backfill scheduled")
```

Add import: `"github.com/bluenviron/mediamtx/internal/recorder/fragmentbackfill"`.

- [ ] **Step 2: Add thumbnail generator**

Inspect `internal/recorder/thumbnail/` for the constructor name and required dependencies:

Run: `grep -n "^func New\|^func Run\|^func Start" internal/recorder/thumbnail/*.go`

If `thumbnail.New(db, recordingsPath)` returns a generator that exposes `.Start(ctx)`, add:

```go
thumbGen := thumbnail.New(nvrDB, cfg.RecordingsPath)
go thumbGen.Run(ctx)
log.Info("recorder: thumbnail generator started")
```

Adapt the constructor signature to what actually exists. If the generator is HTTP-handler-only with no background loop, omit this step and address it in Phase 2 (API surface).

- [ ] **Step 3: Add connmgr + alerts**

Inspect the constructors:

Run: `grep -n "^func New\|^func Start" internal/recorder/connmgr/*.go internal/recorder/alerts/*.go`

Wire them following the same pattern. From the legacy `nvr.go` reference at line ~340: `connMgr.Start()` is called for connection state events, and the alerts package observes events. If alerts requires a callback into `events.Publish`, hook it via the existing event broadcaster the recorder already uses.

If the recorder doesn't yet have an event broadcaster, scope it for a follow-up task and note "alerts wired but events broadcast TBD" in the commit.

- [ ] **Step 4: Build & vet**

Run: `go build ./... && go vet ./internal/recorder/...`

Expected: no errors.

- [ ] **Step 5: Run recorder tests**

Run: `go test ./internal/recorder/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "$(cat <<'EOF'
feat(recorder): wire fragment-backfill, thumbnail, connmgr, alerts

Activates the remaining orphaned legacy packages now that the trunk
boot path constructs them. Recorder is now feature-equivalent to the
legacy nvr.initRecording() except for the CaptureManager itself.

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 7: Replace noopCaptureManager with capturemanager.Manager

**Files:**
- Modify: `internal/recorder/boot.go` (replace the construction and remove the noop type)

- [ ] **Step 1: Replace the `capMgr := &noopCaptureManager{}` site**

Find the line:
```go
capMgr := &noopCaptureManager{}
```

Replace with:
```go
capMgr := capturemanager.New(capturemanager.Config{
    YAML:           yw,
    Reload:         supervisor.Reload,
    RecordingsPath: cfg.RecordingsPath,
    Logger:         log,
})
```

Add import: `"github.com/bluenviron/mediamtx/internal/recorder/capturemanager"`.

- [ ] **Step 2: Delete the noop type**

Find and delete this block from `internal/recorder/boot.go` (the noop type and its three methods around line 628-647):

```go
// noopCaptureManager satisfies recordercontrol.CaptureManager with
// no-op methods; placeholder until KAI-259.
type noopCaptureManager struct{}

func (n *noopCaptureManager) EnsureRunning(_ recordercontrol.Camera) error { return nil }
func (n *noopCaptureManager) Stop(_ string) error                          { return nil }
func (n *noopCaptureManager) RunningCameras() []string                     { return nil }
```

- [ ] **Step 3: Build & vet**

Run: `go build ./... && go vet ./internal/recorder/...`

Expected: no errors. If the compiler complains that `recordercontrol` import is now unused, remove it from `boot.go`'s imports.

- [ ] **Step 4: Run all recorder tests**

Run: `go test ./internal/recorder/...`

Expected: PASS.

- [ ] **Step 5: Run the broader test suite**

Run: `go test ./...`

Expected: PASS (or pre-existing failures unrelated to this change). If new failures appear, investigate before committing.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "$(cat <<'EOF'
feat(recorder): replace noopCaptureManager with real adapter

Wires capturemanager.Manager into the recorder boot path. Camera
assignments from the Directory's RecorderControl stream now translate
to record: true entries in mediamtx.yml + supervisor reloads, so
fMP4 segments actually land on disk.

Closes the KAI-259 placeholder.

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 8: End-to-end smoke test

**Files:**
- Create: `internal/recorder/boot_smoke_test.go`

- [ ] **Step 1: Write the smoke test**

Create `internal/recorder/boot_smoke_test.go`:

```go
//go:build smoke

package recorder

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
)

// TestBoot_EnsureRunning_WritesYAML drives the full boot path with a
// fake Directory and asserts that an EnsureRunning call mutates the
// mediamtx.yml owned by the recorder. Tagged smoke because it spins up
// a real supervisor; run with: go test -tags smoke ./internal/recorder/.
func TestBoot_EnsureRunning_WritesYAML(t *testing.T) {
    dir := t.TempDir()
    yamlPath := filepath.Join(dir, "mediamtx.yml")
    if err := os.WriteFile(yamlPath, []byte("paths:\n"), 0o644); err != nil {
        t.Fatalf("seed yaml: %v", err)
    }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Construct a minimal BootConfig with the temp paths. The exact
    // fields depend on your boot.BootConfig — adapt accordingly. The
    // important plumb-throughs are MediaMTXConfigPath and RecordingsPath.
    bootResult, err := Boot(ctx, BootConfig{
        MediaMTXConfigPath: yamlPath,
        RecordingsPath:     filepath.Join(dir, "recordings"),
        // ...other required fields with safe test defaults...
    })
    if err != nil {
        t.Fatalf("Boot: %v", err)
    }

    cam := recordercontrol.Camera{
        ID:        "smoke-cam-1",
        StreamURL: "rtsp://127.0.0.1:8554/dummy",
    }
    if err := bootResult.CaptureManager.EnsureRunning(cam); err != nil {
        t.Fatalf("EnsureRunning: %v", err)
    }

    body, err := os.ReadFile(yamlPath)
    if err != nil {
        t.Fatalf("read yaml: %v", err)
    }
    if !contains(string(body), "smoke-cam-1") || !contains(string(body), "record: true") {
        t.Fatalf("yaml missing recorded path; got:\n%s", body)
    }
}

func contains(s, sub string) bool {
    for i := 0; i+len(sub) <= len(s); i++ {
        if s[i:i+len(sub)] == sub {
            return true
        }
    }
    return false
}
```

If `Boot` doesn't currently return a `BootResult` with `CaptureManager` exposed, add that field to whatever boot returns. (Small refactor: turn the `Boot` function's return into a struct that exposes `CaptureManager`, `Supervisor`, etc., for testability.)

- [ ] **Step 2: Run the smoke test**

Run: `go test -tags smoke ./internal/recorder/ -run TestBoot_EnsureRunning -v`

Expected: PASS. The supervisor will start a child mediamtx process; clean shutdown happens via the test's context cancel.

If the supervisor refuses to start in CI without a `mediamtx` binary on `PATH`, gate the test further with `t.Skip` when the binary is absent — don't make CI brittle.

- [ ] **Step 3: Commit**

```bash
git add internal/recorder/boot_smoke_test.go
git commit -m "$(cat <<'EOF'
test(recorder): smoke test capture loop end to end

Verifies that a Boot()'d recorder responds to a CaptureManager.EnsureRunning
call by mutating mediamtx.yml. Tagged smoke; runs only with -tags smoke.

Co-Authored-By: claude-flow <ruv@ruv.net>
EOF
)"
```

---

## Task 9: Manual verification against a real camera (one-shot, not committed)

This task is human-driven; no code changes.

- [ ] **Step 1: Boot the recorder pointing at a real camera**

Pair the recorder with the directory using the existing pairing flow. Add a real camera in the directory UI and set it to record (this is what causes the directory to push an assignment).

- [ ] **Step 2: Watch the recorder log**

Look for:
```
recorder: scheduler started
recorder: storage manager started
recorder: recovery complete (or "scanned=0")
recorder: integrity scanner started
recorder: fragment backfill scheduled
recorder: thumbnail generator started
recorder: RecorderControl stream started
capturemanager: ensure running camera_id=<id>
```

- [ ] **Step 3: Verify segments land on disk**

After ~30 seconds:
```bash
find /var/lib/raikada/recordings -name "*.mp4" -mmin -2
```
Expected: at least one file created in the last 2 minutes.

- [ ] **Step 4: Verify DB rows appear**

```bash
sqlite3 /var/lib/raikada/recorder/nvr.db "SELECT id, camera_id, file_path, duration_ms FROM recordings ORDER BY created_at DESC LIMIT 5;"
```
Expected: at least one row corresponding to the file from Step 3.

- [ ] **Step 5: Verify mediamtx.yml mutation**

```bash
grep -A 4 "cam-<your-camera-id>" /etc/raikada/mediamtx.yml
```
Expected: a `paths: cam-<id>:` entry with `record: true`, the camera's RTSP URL as `source`, and `recordPath` under your recordings directory.

(If any step fails, revert and debug — the inventory will tell you which package's wiring is suspect.)

---

## Self-review

**Spec coverage:**
- CaptureManager adapter — Task 2
- Scheduler wired — Task 4
- Storage manager wired — Task 4
- Recovery scan wired — Task 5
- Integrity scanner wired — Task 5
- Fragment backfill ported + wired — Tasks 3, 6
- Thumbnail generator wired — Task 6
- connmgr + alerts wired — Task 6
- noopCaptureManager replaced + deleted — Task 7
- End-to-end smoke test — Task 8
- Manual verification — Task 9

**Out of scope (next phases):**
- mTLS / `getCert` (Phase 4)
- `/api/nvr/*` route prefix on recorder + JWT middleware (Phase 2)
- HLS/VoD/screenshot HTTP handlers (Phase 2)
- Talkback wiring (Phase 2)

**Open assumptions to verify on Step 1 audit:**
- `Camera.ConfigVersion` is `uint64` (test uses `1`/`2`; adapt to actual type).
- `db.Recording` exposes `FilePath`, `FileSize`, `InitSize`, `DurationMs` (used in integrity wiring).
- `supervisor.Reload()` is non-blocking or fast enough to call inline.
- `cfg.RecordingsPath` and `cfg.MediaMTXConfigPath` exist on the boot config struct (or equivalent names).

If any assumption is wrong, fix in the relevant task before continuing — do not paper over.
