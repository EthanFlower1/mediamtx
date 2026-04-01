# Recording Schedules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-camera recording schedules with always/events modes, ONVIF motion detection, and a schedule management UI.

**Architecture:** Recording rules are stored in SQLite. A background scheduler goroutine evaluates rules every 30 seconds and toggles recording per camera path via YAML config writes. For events mode, ONVIF PullPoint subscriptions detect motion, and a state machine manages recording start/stop with post-event buffer and hysteresis.

**Tech Stack:** Go (SQLite, Gin, use-go/onvif), React + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-20-recording-schedules-design.md`

---

## File Structure

### New files

| File                                       | Responsibility                                                |
| ------------------------------------------ | ------------------------------------------------------------- |
| `internal/nvr/db/recording_rules.go`       | RecordingRule struct + CRUD queries                           |
| `internal/nvr/db/recording_rules_test.go`  | DB layer tests                                                |
| `internal/nvr/api/recording_rules.go`      | HTTP handlers for rules CRUD + recording status               |
| `internal/nvr/api/recording_rules_test.go` | API handler tests                                             |
| `internal/nvr/scheduler/scheduler.go`      | Background evaluator, write coalescing, camera state tracking |
| `internal/nvr/scheduler/scheduler_test.go` | Scheduler evaluation logic tests                              |
| `internal/nvr/scheduler/motion.go`         | ONVIF PullPoint subscription + motion state machine           |
| `internal/nvr/scheduler/motion_test.go`    | Motion state machine tests                                    |
| `internal/nvr/onvif/events.go`             | Raw SOAP calls for ONVIF event service                        |
| `ui/src/components/RecordingRules.tsx`     | Recording rules UI component                                  |
| `ui/src/components/SchedulePreview.tsx`    | Weekly schedule grid visualization                            |
| `ui/src/hooks/useRecordingRules.ts`        | React hook for rules CRUD                                     |

### Modified files

| File                                     | Change                                                             |
| ---------------------------------------- | ------------------------------------------------------------------ |
| `internal/nvr/db/migrations.go`          | Add v2 migration for recording_rules table                         |
| `internal/nvr/api/router.go`             | Register recording rules endpoints, pass scheduler to RouterConfig |
| `internal/nvr/api/cameras.go`            | Call scheduler.RemoveCamera on camera delete                       |
| `internal/nvr/nvr.go`                    | Create/start/stop scheduler, pass to RegisterRoutes                |
| `internal/nvr/yamlwriter/writer.go`      | Add SetPathValue method                                            |
| `internal/nvr/yamlwriter/writer_test.go` | Test SetPathValue                                                  |
| `ui/src/pages/CameraManagement.tsx`      | Add recording rules section per camera                             |

---

### Task 1: Add `recording_rules` DB migration and struct

**Files:**

- Modify: `internal/nvr/db/migrations.go`
- Create: `internal/nvr/db/recording_rules.go`
- Create: `internal/nvr/db/recording_rules_test.go`

- [ ] **Step 1: Add v2 migration to migrations.go**

Add the recording_rules table as migration v2. In `internal/nvr/db/migrations.go`, add to the `migrations` slice:

```go
{
    version: 2,
    sql: `CREATE TABLE recording_rules (
        id TEXT PRIMARY KEY,
        camera_id TEXT NOT NULL,
        name TEXT NOT NULL,
        mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
        days TEXT NOT NULL,
        start_time TEXT NOT NULL,
        end_time TEXT NOT NULL,
        post_event_seconds INTEGER NOT NULL DEFAULT 30,
        enabled INTEGER NOT NULL DEFAULT 1,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
    );
    CREATE INDEX idx_recording_rules_camera ON recording_rules(camera_id);`,
},
```

- [ ] **Step 2: Create RecordingRule struct and CRUD in recording_rules.go**

Create `internal/nvr/db/recording_rules.go`:

```go
package db

import (
    "database/sql"
    "errors"
    "time"

    "github.com/google/uuid"
)

// RecordingRule defines a recording schedule rule for a camera.
type RecordingRule struct {
    ID               string `json:"id"`
    CameraID         string `json:"camera_id"`
    Name             string `json:"name"`
    Mode             string `json:"mode"` // "always" or "events"
    Days             string `json:"days"` // JSON array of day numbers 0-6
    StartTime        string `json:"start_time"` // "HH:MM"
    EndTime          string `json:"end_time"`   // "HH:MM"
    PostEventSeconds int    `json:"post_event_seconds"`
    Enabled          bool   `json:"enabled"`
    CreatedAt        string `json:"created_at"`
    UpdatedAt        string `json:"updated_at"`
}

func (d *DB) CreateRecordingRule(rule *RecordingRule) error {
    if rule.ID == "" {
        rule.ID = uuid.New().String()
    }
    now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    rule.CreatedAt = now
    rule.UpdatedAt = now

    _, err := d.Exec(`INSERT INTO recording_rules
        (id, camera_id, name, mode, days, start_time, end_time,
         post_event_seconds, enabled, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        rule.ID, rule.CameraID, rule.Name, rule.Mode, rule.Days,
        rule.StartTime, rule.EndTime, rule.PostEventSeconds,
        rule.Enabled, rule.CreatedAt, rule.UpdatedAt,
    )
    return err
}

func (d *DB) GetRecordingRule(id string) (*RecordingRule, error) {
    rule := &RecordingRule{}
    err := d.QueryRow(`SELECT id, camera_id, name, mode, days, start_time, end_time,
        post_event_seconds, enabled, created_at, updated_at
        FROM recording_rules WHERE id = ?`, id).Scan(
        &rule.ID, &rule.CameraID, &rule.Name, &rule.Mode, &rule.Days,
        &rule.StartTime, &rule.EndTime, &rule.PostEventSeconds,
        &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
    )
    if errors.Is(err, sql.ErrNoRows) {
        return nil, ErrNotFound
    }
    return rule, err
}

func (d *DB) ListRecordingRules(cameraID string) ([]*RecordingRule, error) {
    rows, err := d.Query(`SELECT id, camera_id, name, mode, days, start_time, end_time,
        post_event_seconds, enabled, created_at, updated_at
        FROM recording_rules WHERE camera_id = ? ORDER BY created_at`, cameraID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var rules []*RecordingRule
    for rows.Next() {
        rule := &RecordingRule{}
        if err := rows.Scan(&rule.ID, &rule.CameraID, &rule.Name, &rule.Mode,
            &rule.Days, &rule.StartTime, &rule.EndTime, &rule.PostEventSeconds,
            &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
            return nil, err
        }
        rules = append(rules, rule)
    }
    return rules, rows.Err()
}

func (d *DB) ListAllEnabledRecordingRules() ([]*RecordingRule, error) {
    rows, err := d.Query(`SELECT id, camera_id, name, mode, days, start_time, end_time,
        post_event_seconds, enabled, created_at, updated_at
        FROM recording_rules WHERE enabled = 1 ORDER BY camera_id, created_at`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var rules []*RecordingRule
    for rows.Next() {
        rule := &RecordingRule{}
        if err := rows.Scan(&rule.ID, &rule.CameraID, &rule.Name, &rule.Mode,
            &rule.Days, &rule.StartTime, &rule.EndTime, &rule.PostEventSeconds,
            &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
            return nil, err
        }
        rules = append(rules, rule)
    }
    return rules, rows.Err()
}

func (d *DB) UpdateRecordingRule(rule *RecordingRule) error {
    rule.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    res, err := d.Exec(`UPDATE recording_rules SET
        name = ?, mode = ?, days = ?, start_time = ?, end_time = ?,
        post_event_seconds = ?, enabled = ?, updated_at = ?
        WHERE id = ?`,
        rule.Name, rule.Mode, rule.Days, rule.StartTime, rule.EndTime,
        rule.PostEventSeconds, rule.Enabled, rule.UpdatedAt, rule.ID,
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

func (d *DB) DeleteRecordingRule(id string) error {
    res, err := d.Exec("DELETE FROM recording_rules WHERE id = ?", id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return ErrNotFound
    }
    return nil
}
```

- [ ] **Step 3: Write tests for recording_rules CRUD**

Create `internal/nvr/db/recording_rules_test.go`:

```go
package db_test

import (
    "path/filepath"
    "testing"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
    "github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *db.DB {
    t.Helper()
    d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    t.Cleanup(func() { d.Close() })
    return d
}

func createTestCamera(t *testing.T, d *db.DB) *db.Camera {
    t.Helper()
    cam := &db.Camera{Name: "test-cam", RTSPURL: "rtsp://test", MediaMTXPath: "nvr/test"}
    require.NoError(t, d.CreateCamera(cam))
    return cam
}

func TestRecordingRuleCRUD(t *testing.T) {
    d := newTestDB(t)
    cam := createTestCamera(t, d)

    rule := &db.RecordingRule{
        CameraID:         cam.ID,
        Name:             "Weeknight",
        Mode:             "events",
        Days:             "[1,2,3,4,5]",
        StartTime:        "18:00",
        EndTime:          "06:00",
        PostEventSeconds: 30,
        Enabled:          true,
    }

    // Create
    require.NoError(t, d.CreateRecordingRule(rule))
    require.NotEmpty(t, rule.ID)

    // Get
    got, err := d.GetRecordingRule(rule.ID)
    require.NoError(t, err)
    require.Equal(t, "Weeknight", got.Name)
    require.Equal(t, "events", got.Mode)

    // List
    rules, err := d.ListRecordingRules(cam.ID)
    require.NoError(t, err)
    require.Len(t, rules, 1)

    // Update
    got.Name = "Updated"
    require.NoError(t, d.UpdateRecordingRule(got))
    got2, _ := d.GetRecordingRule(rule.ID)
    require.Equal(t, "Updated", got2.Name)

    // Delete
    require.NoError(t, d.DeleteRecordingRule(rule.ID))
    _, err = d.GetRecordingRule(rule.ID)
    require.ErrorIs(t, err, db.ErrNotFound)
}

func TestRecordingRuleCascadeDelete(t *testing.T) {
    d := newTestDB(t)
    cam := createTestCamera(t, d)

    rule := &db.RecordingRule{
        CameraID: cam.ID, Name: "test", Mode: "always",
        Days: "[0,1,2,3,4,5,6]", StartTime: "00:00", EndTime: "23:59",
        Enabled: true,
    }
    require.NoError(t, d.CreateRecordingRule(rule))

    // Delete camera should cascade delete rules
    require.NoError(t, d.DeleteCamera(cam.ID))
    rules, err := d.ListRecordingRules(cam.ID)
    require.NoError(t, err)
    require.Len(t, rules, 0)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/nvr/db/ -v -run TestRecordingRule`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```
git add internal/nvr/db/migrations.go internal/nvr/db/recording_rules.go internal/nvr/db/recording_rules_test.go
git commit -m "feat(nvr): add recording_rules database table and CRUD queries"
```

---

### Task 2: Add `SetPathValue` to YAML writer

**Files:**

- Modify: `internal/nvr/yamlwriter/writer.go`
- Modify: `internal/nvr/yamlwriter/writer_test.go`

- [ ] **Step 1: Write failing test for SetPathValue**

Add to `internal/nvr/yamlwriter/writer_test.go`:

```go
func TestSetPathValue(t *testing.T) {
    tmpDir := t.TempDir()
    yamlPath := filepath.Join(tmpDir, "mediamtx.yml")

    initial := `paths:
  nvr/cam1:
    source: rtsp://192.168.1.100/stream
    record: false
`
    require.NoError(t, os.WriteFile(yamlPath, []byte(initial), 0o644))

    w := yamlwriter.New(yamlPath)
    require.NoError(t, w.SetPathValue("nvr/cam1", "record", true))

    data, err := os.ReadFile(yamlPath)
    require.NoError(t, err)
    content := string(data)

    // record should be true now
    require.Contains(t, content, "record: true")
    // source should be preserved
    require.Contains(t, content, "source: rtsp://192.168.1.100/stream")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nvr/yamlwriter/ -v -run TestSetPathValue`
Expected: FAIL — method does not exist.

- [ ] **Step 3: Implement SetPathValue**

Add to `internal/nvr/yamlwriter/writer.go`:

```go
// SetPathValue sets a single key within an existing path entry without
// overwriting other keys. This is used to toggle record: true/false.
func (w *Writer) SetPathValue(pathName, key string, value interface{}) error {
    w.mu.Lock()
    defer w.mu.Unlock()

    data, err := os.ReadFile(w.path)
    if err != nil {
        return fmt.Errorf("read config: %w", err)
    }

    content := string(data)
    lines := strings.Split(content, "\n")

    // Find the path entry line.
    pathPrefix := "  " + pathName + ":"
    pathIdx := -1
    for i, line := range lines {
        if strings.TrimRight(line, " ") == pathPrefix || strings.HasPrefix(line, pathPrefix+" ") {
            pathIdx = i
            break
        }
    }
    if pathIdx == -1 {
        return fmt.Errorf("path %q not found", pathName)
    }

    // Find the key line within the path's indented block.
    keyPrefix := "    " + key + ":"
    keyIdx := -1
    for i := pathIdx + 1; i < len(lines); i++ {
        line := lines[i]
        trimmed := strings.TrimSpace(line)
        // Stop if we hit another path entry or end of paths section.
        if trimmed != "" && !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "  #") {
            break
        }
        if strings.HasPrefix(line, keyPrefix) {
            keyIdx = i
            break
        }
    }

    valueStr := fmt.Sprintf("%v", value)
    newLine := fmt.Sprintf("    %s: %s", key, valueStr)

    if keyIdx != -1 {
        lines[keyIdx] = newLine
    } else {
        // Insert the key after the path entry line.
        after := make([]string, len(lines[pathIdx+1:]))
        copy(after, lines[pathIdx+1:])
        lines = append(lines[:pathIdx+1], newLine)
        lines = append(lines, after...)
    }

    return w.atomicWriteText(strings.Join(lines, "\n"))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/nvr/yamlwriter/ -v -run TestSetPathValue`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add internal/nvr/yamlwriter/writer.go internal/nvr/yamlwriter/writer_test.go
git commit -m "feat(nvr): add SetPathValue to YAML writer for toggling individual path keys"
```

---

### Task 3: Implement schedule evaluator

**Files:**

- Create: `internal/nvr/scheduler/scheduler.go`
- Create: `internal/nvr/scheduler/scheduler_test.go`

- [ ] **Step 1: Write failing test for schedule evaluation logic**

Create `internal/nvr/scheduler/scheduler_test.go`:

```go
package scheduler

import (
    "testing"
    "time"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
    "github.com/stretchr/testify/require"
)

func TestEvaluateRules(t *testing.T) {
    // Monday 20:00 local time
    now := time.Date(2026, 3, 23, 20, 0, 0, 0, time.Local) // Monday

    tests := []struct {
        name     string
        rules    []*db.RecordingRule
        expected EffectiveMode
    }{
        {
            name:     "no rules = off",
            rules:    nil,
            expected: ModeOff,
        },
        {
            name: "always rule matching",
            rules: []*db.RecordingRule{{
                Mode: "always", Days: "[1]", StartTime: "18:00", EndTime: "22:00", Enabled: true,
            }},
            expected: ModeAlways,
        },
        {
            name: "events rule matching",
            rules: []*db.RecordingRule{{
                Mode: "events", Days: "[1]", StartTime: "18:00", EndTime: "22:00", Enabled: true,
            }},
            expected: ModeEvents,
        },
        {
            name: "always wins over events (union)",
            rules: []*db.RecordingRule{
                {Mode: "events", Days: "[1]", StartTime: "18:00", EndTime: "22:00", Enabled: true},
                {Mode: "always", Days: "[1]", StartTime: "19:00", EndTime: "21:00", Enabled: true},
            },
            expected: ModeAlways,
        },
        {
            name: "rule not matching day",
            rules: []*db.RecordingRule{{
                Mode: "always", Days: "[0]", StartTime: "18:00", EndTime: "22:00", Enabled: true,
            }},
            expected: ModeOff, // Sunday rule, today is Monday
        },
        {
            name: "cross-midnight rule — evening portion",
            rules: []*db.RecordingRule{{
                Mode: "always", Days: "[1]", StartTime: "22:00", EndTime: "06:00", Enabled: true,
            }},
            expected: ModeOff, // 20:00 is before 22:00
        },
        {
            name: "cross-midnight rule — morning portion (yesterday started)",
            rules: []*db.RecordingRule{{
                Mode: "always", Days: "[0]", StartTime: "22:00", EndTime: "06:00", Enabled: true,
            }},
            // Now is Monday 20:00 — yesterday was Sunday (day 0) but 20:00 is not < 06:00
            expected: ModeOff,
        },
        {
            name: "disabled rule ignored",
            rules: []*db.RecordingRule{{
                Mode: "always", Days: "[1]", StartTime: "18:00", EndTime: "22:00", Enabled: false,
            }},
            expected: ModeOff,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := evaluateRules(tt.rules, now)
            require.Equal(t, tt.expected, result)
        })
    }
}

func TestRuleMatchesTime(t *testing.T) {
    // Tuesday 03:00 — test cross-midnight "yesterday" check
    now := time.Date(2026, 3, 24, 3, 0, 0, 0, time.Local) // Tuesday

    rule := &db.RecordingRule{
        Mode: "always", Days: "[1]", StartTime: "22:00", EndTime: "06:00", Enabled: true,
    }
    // Monday (day 1) 22:00-06:00 should match Tuesday 03:00
    require.True(t, ruleMatchesTime(rule, now))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nvr/scheduler/ -v -run TestEvaluateRules`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement scheduler.go**

Create `internal/nvr/scheduler/scheduler.go`:

```go
package scheduler

import (
    "encoding/json"
    "fmt"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
    "github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// EffectiveMode is the resolved recording mode for a camera.
type EffectiveMode string

const (
    ModeOff    EffectiveMode = "off"
    ModeAlways EffectiveMode = "always"
    ModeEvents EffectiveMode = "events"
)

// CameraState tracks the current recording state for a camera.
type CameraState struct {
    EffectiveMode EffectiveMode
    Recording     bool
    MotionState   string // "idle", "recording", "post_buffer", "hysteresis"
    ActiveRules   []string
}

// Scheduler evaluates recording rules and manages recording state.
type Scheduler struct {
    db         *db.DB
    yamlWriter *yamlwriter.Writer

    mu     sync.Mutex
    states map[string]*CameraState // camera ID -> state
    stopCh chan struct{}

    // Pending YAML writes, coalesced on a 500ms timer.
    pendingWrites   map[string]bool // path -> desired record state
    pendingWritesMu sync.Mutex
    writeTimer      *time.Timer
}

// New creates a new Scheduler.
func New(database *db.DB, writer *yamlwriter.Writer) *Scheduler {
    return &Scheduler{
        db:            database,
        yamlWriter:    writer,
        states:        make(map[string]*CameraState),
        stopCh:        make(chan struct{}),
        pendingWrites: make(map[string]bool),
    }
}

// Start begins the scheduler background loop.
// It defers the first evaluation by 5 seconds to avoid racing with MediaMTX startup.
func (s *Scheduler) Start() {
    go func() {
        // Wait for MediaMTX to finish initial config load.
        select {
        case <-time.After(5 * time.Second):
        case <-s.stopCh:
            return
        }

        s.evaluate()

        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                s.evaluate()
            case <-s.stopCh:
                return
            }
        }
    }()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
    close(s.stopCh)
}

// RemoveCamera cleans up state for a deleted camera.
func (s *Scheduler) RemoveCamera(cameraID string) {
    s.mu.Lock()
    delete(s.states, cameraID)
    s.mu.Unlock()
}

// GetCameraState returns the current state for a camera.
func (s *Scheduler) GetCameraState(cameraID string) *CameraState {
    s.mu.Lock()
    defer s.mu.Unlock()
    if st, ok := s.states[cameraID]; ok {
        cp := *st
        return &cp
    }
    return &CameraState{EffectiveMode: ModeOff, MotionState: "idle"}
}

// evaluate runs one evaluation cycle for all cameras.
func (s *Scheduler) evaluate() {
    rules, err := s.db.ListAllEnabledRecordingRules()
    if err != nil {
        return
    }

    // Group rules by camera.
    byCamera := make(map[string][]*db.RecordingRule)
    for _, r := range rules {
        byCamera[r.CameraID] = append(byCamera[r.CameraID], r)
    }

    // Also include cameras with no rules (to turn off recording).
    cameras, err := s.db.ListCameras()
    if err != nil {
        return
    }

    now := time.Now()

    s.mu.Lock()
    defer s.mu.Unlock()

    for _, cam := range cameras {
        cameraRules := byCamera[cam.ID]
        newMode := evaluateRules(cameraRules, now)

        state, exists := s.states[cam.ID]
        if !exists {
            state = &CameraState{EffectiveMode: ModeOff, MotionState: "idle"}
            s.states[cam.ID] = state
        }

        if newMode == state.EffectiveMode {
            continue
        }

        oldMode := state.EffectiveMode
        state.EffectiveMode = newMode

        // Collect active rule IDs.
        state.ActiveRules = nil
        for _, r := range cameraRules {
            if ruleMatchesTime(r, now) {
                state.ActiveRules = append(state.ActiveRules, r.ID)
            }
        }

        // Apply recording state changes.
        switch newMode {
        case ModeAlways:
            if !state.Recording {
                s.setRecording(cam.MediaMTXPath, true)
                state.Recording = true
            }
            state.MotionState = "idle"
        case ModeEvents:
            // In events mode, recording starts only on motion.
            if oldMode == ModeAlways && state.Recording {
                s.setRecording(cam.MediaMTXPath, false)
                state.Recording = false
            }
            state.MotionState = "idle"
            // TODO: Start ONVIF event subscription (Task 4)
        case ModeOff:
            if state.Recording {
                s.setRecording(cam.MediaMTXPath, false)
                state.Recording = false
            }
            state.MotionState = "idle"
            // TODO: Stop ONVIF event subscription (Task 4)
        }
    }
}

// setRecording queues a recording state change with write coalescing.
func (s *Scheduler) setRecording(path string, record bool) {
    s.pendingWritesMu.Lock()
    defer s.pendingWritesMu.Unlock()

    s.pendingWrites[path] = record

    // Reset the coalesce timer.
    if s.writeTimer != nil {
        s.writeTimer.Stop()
    }
    s.writeTimer = time.AfterFunc(500*time.Millisecond, s.flushWrites)
}

// flushWrites writes all pending recording state changes to YAML.
func (s *Scheduler) flushWrites() {
    s.pendingWritesMu.Lock()
    writes := make(map[string]bool, len(s.pendingWrites))
    for k, v := range s.pendingWrites {
        writes[k] = v
    }
    s.pendingWrites = make(map[string]bool)
    s.pendingWritesMu.Unlock()

    for path, record := range writes {
        _ = s.yamlWriter.SetPathValue(path, "record", record)
    }
}

// evaluateRules determines the effective mode from a set of rules at a given time.
func evaluateRules(rules []*db.RecordingRule, now time.Time) EffectiveMode {
    hasAlways := false
    hasEvents := false

    for _, r := range rules {
        if !r.Enabled {
            continue
        }
        if !ruleMatchesTime(r, now) {
            continue
        }
        switch r.Mode {
        case "always":
            hasAlways = true
        case "events":
            hasEvents = true
        }
    }

    if hasAlways {
        return ModeAlways
    }
    if hasEvents {
        return ModeEvents
    }
    return ModeOff
}

// ruleMatchesTime checks if a rule is active at the given time.
func ruleMatchesTime(r *db.RecordingRule, now time.Time) bool {
    if !r.Enabled {
        return false
    }

    todayDay := int(now.Weekday()) // 0=Sun
    yesterdayDay := (todayDay + 6) % 7

    days := parseDays(r.Days)
    startMin := parseTimeToMinutes(r.StartTime)
    endMin := parseTimeToMinutes(r.EndTime)
    nowMin := now.Hour()*60 + now.Minute()

    crossesMidnight := startMin > endMin || (startMin == endMin)

    if crossesMidnight {
        // Check: today in days AND now >= start (evening portion)
        if containsDay(days, todayDay) && nowMin >= startMin {
            return true
        }
        // Check: yesterday in days AND now < end (morning portion)
        if containsDay(days, yesterdayDay) && nowMin < endMin {
            return true
        }
        return false
    }

    // Same-day rule.
    if !containsDay(days, todayDay) {
        return false
    }
    return nowMin >= startMin && nowMin < endMin
}

func parseDays(daysJSON string) []int {
    var days []int
    _ = json.Unmarshal([]byte(daysJSON), &days)
    return days
}

func containsDay(days []int, day int) bool {
    for _, d := range days {
        if d == day {
            return true
        }
    }
    return false
}

func parseTimeToMinutes(t string) int {
    parts := strings.Split(t, ":")
    if len(parts) != 2 {
        return 0
    }
    h, _ := strconv.Atoi(parts[0])
    m, _ := strconv.Atoi(parts[1])
    return h*60 + m
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/nvr/scheduler/ -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```
git add internal/nvr/scheduler/
git commit -m "feat(nvr): add schedule evaluator with rule matching and write coalescing"
```

---

### Task 4: Implement ONVIF event subscription and motion state machine

**Files:**

- Create: `internal/nvr/onvif/events.go`
- Create: `internal/nvr/scheduler/motion.go`
- Create: `internal/nvr/scheduler/motion_test.go`

- [ ] **Step 1: Write failing test for motion state machine**

Create `internal/nvr/scheduler/motion_test.go`:

```go
package scheduler

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestMotionStateMachine(t *testing.T) {
    writes := make(map[string]bool)
    mockSetRecording := func(path string, record bool) {
        writes[path] = record
    }

    msm := newMotionStateMachine("nvr/cam1", 30, mockSetRecording)
    require.Equal(t, "idle", msm.State())

    // Motion detected -> recording
    msm.OnMotion(true)
    require.Equal(t, "recording", msm.State())
    require.True(t, writes["nvr/cam1"])

    // Motion stopped -> post_buffer
    msm.OnMotion(false)
    require.Equal(t, "post_buffer", msm.State())

    // Motion again during post_buffer -> back to recording
    msm.OnMotion(true)
    require.Equal(t, "recording", msm.State())

    // Motion stopped -> post_buffer -> timer expires -> hysteresis
    msm.OnMotion(false)
    msm.OnPostBufferExpired()
    require.Equal(t, "hysteresis", msm.State())

    // No motion during hysteresis -> idle
    msm.OnHysteresisExpired()
    require.Equal(t, "idle", msm.State())
    require.False(t, writes["nvr/cam1"])
}

func TestMotionRetriggerDuringHysteresis(t *testing.T) {
    writes := make(map[string]bool)
    mockSetRecording := func(path string, record bool) {
        writes[path] = record
    }

    msm := newMotionStateMachine("nvr/cam1", 30, mockSetRecording)
    msm.OnMotion(true)
    msm.OnMotion(false)
    msm.OnPostBufferExpired()
    require.Equal(t, "hysteresis", msm.State())

    // Motion during hysteresis -> back to recording (no YAML write needed since still recording)
    msm.OnMotion(true)
    require.Equal(t, "recording", msm.State())
}
```

- [ ] **Step 2: Implement motion state machine**

Create `internal/nvr/scheduler/motion.go`:

```go
package scheduler

import (
    "sync"
    "time"
)

type setRecordingFunc func(path string, record bool)

type motionStateMachine struct {
    mu               sync.Mutex
    path             string
    postEventSeconds int
    state            string // "idle", "recording", "post_buffer", "hysteresis"
    setRecording     setRecordingFunc
    postBufferTimer  *time.Timer
    hysteresisTimer  *time.Timer
}

func newMotionStateMachine(path string, postEventSeconds int, setRec setRecordingFunc) *motionStateMachine {
    return &motionStateMachine{
        path:             path,
        postEventSeconds: postEventSeconds,
        state:            "idle",
        setRecording:     setRec,
    }
}

func (m *motionStateMachine) State() string {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.state
}

func (m *motionStateMachine) OnMotion(detected bool) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if detected {
        m.cancelTimers()
        switch m.state {
        case "idle":
            m.state = "recording"
            m.setRecording(m.path, true)
        case "post_buffer", "hysteresis":
            m.state = "recording"
            // Already recording, no YAML write needed
        }
        // "recording" -> stay recording
    } else {
        if m.state == "recording" {
            m.state = "post_buffer"
            m.postBufferTimer = time.AfterFunc(
                time.Duration(m.postEventSeconds)*time.Second,
                m.OnPostBufferExpired,
            )
        }
    }
}

func (m *motionStateMachine) OnPostBufferExpired() {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.state != "post_buffer" {
        return
    }
    m.state = "hysteresis"
    m.hysteresisTimer = time.AfterFunc(10*time.Second, m.OnHysteresisExpired)
}

func (m *motionStateMachine) OnHysteresisExpired() {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.state != "hysteresis" {
        return
    }
    m.state = "idle"
    m.setRecording(m.path, false)
}

func (m *motionStateMachine) Stop() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.cancelTimers()
    if m.state != "idle" {
        m.state = "idle"
        m.setRecording(m.path, false)
    }
}

func (m *motionStateMachine) cancelTimers() {
    if m.postBufferTimer != nil {
        m.postBufferTimer.Stop()
        m.postBufferTimer = nil
    }
    if m.hysteresisTimer != nil {
        m.hysteresisTimer.Stop()
        m.hysteresisTimer = nil
    }
}
```

- [ ] **Step 3: Implement ONVIF event subscription**

Create `internal/nvr/onvif/events.go`:

```go
package onvif

import (
    "context"
    "encoding/xml"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    onviflib "github.com/use-go/onvif"
)

// MotionCallback is called when motion state changes.
type MotionCallback func(detected bool)

// EventSubscriber polls an ONVIF camera for motion events.
type EventSubscriber struct {
    dev      *onviflib.Device
    callback MotionCallback
    cancel   context.CancelFunc
}

// NewEventSubscriber creates a subscriber for the given camera.
func NewEventSubscriber(xaddr, username, password string, cb MotionCallback) (*EventSubscriber, error) {
    host := xaddrToHost(xaddr)
    if host == "" {
        host = xaddr
    }

    dev, err := onviflib.NewDevice(onviflib.DeviceParams{
        Xaddr:    host,
        Username: username,
        Password: password,
    })
    if err != nil {
        return nil, fmt.Errorf("connect to device: %w", err)
    }

    return &EventSubscriber{
        dev:      dev,
        callback: cb,
    }, nil
}

// Start begins polling for events. Blocks until ctx is cancelled.
func (s *EventSubscriber) Start(ctx context.Context) {
    ctx, s.cancel = context.WithCancel(ctx)

    for {
        s.subscribeAndPoll(ctx)

        select {
        case <-ctx.Done():
            return
        case <-time.After(5 * time.Second):
            // Retry after error
        }
    }
}

// Stop cancels the subscription.
func (s *EventSubscriber) Stop() {
    if s.cancel != nil {
        s.cancel()
    }
}

func (s *EventSubscriber) subscribeAndPoll(ctx context.Context) {
    // Create PullPoint subscription via raw SOAP.
    subRef, err := s.createPullPointSubscription()
    if err != nil {
        return
    }
    defer s.unsubscribe(subRef)

    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()

    renewTicker := time.NewTicker(48 * time.Second) // Renew at ~80% of 60s
    defer renewTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-renewTicker.C:
            _ = s.renewSubscription(subRef)
        case <-ticker.C:
            messages, err := s.pullMessages(subRef)
            if err != nil {
                return // Connection error, will retry in outer loop
            }
            for _, msg := range messages {
                if isMotionEvent(msg) {
                    s.callback(isMotionActive(msg))
                }
            }
        }
    }
}

// SOAP envelope for CreatePullPointSubscription.
func (s *EventSubscriber) createPullPointSubscription() (string, error) {
    soap := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <tev:CreatePullPointSubscription>
      <tev:InitialTerminationTime>PT60S</tev:InitialTerminationTime>
    </tev:CreatePullPointSubscription>
  </s:Body>
</s:Envelope>`

    resp, err := s.sendSOAP(s.dev.GetEndpoint("event"), soap)
    if err != nil {
        return "", err
    }

    // Parse the subscription reference address from response.
    var env struct {
        Body struct {
            Response struct {
                SubscriptionReference struct {
                    Address string `xml:"Address"`
                } `xml:"SubscriptionReference"`
            } `xml:"CreatePullPointSubscriptionResponse"`
        } `xml:"Body"`
    }
    if err := xml.Unmarshal(resp, &env); err != nil {
        return "", err
    }

    return env.Body.Response.SubscriptionReference.Address, nil
}

func (s *EventSubscriber) pullMessages(subRef string) ([]eventMessage, error) {
    soap := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Body>
    <tev:PullMessages>
      <tev:Timeout>PT1S</tev:Timeout>
      <tev:MessageLimit>100</tev:MessageLimit>
    </tev:PullMessages>
  </s:Body>
</s:Envelope>`

    resp, err := s.sendSOAP(subRef, soap)
    if err != nil {
        return nil, err
    }

    var env struct {
        Body struct {
            Response struct {
                Messages []eventMessage `xml:"NotificationMessage"`
            } `xml:"PullMessagesResponse"`
        } `xml:"Body"`
    }
    _ = xml.Unmarshal(resp, &env)
    return env.Body.Response.Messages, nil
}

func (s *EventSubscriber) renewSubscription(subRef string) error {
    soap := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:Renew>
      <wsnt:TerminationTime>PT60S</wsnt:TerminationTime>
    </wsnt:Renew>
  </s:Body>
</s:Envelope>`
    _, err := s.sendSOAP(subRef, soap)
    return err
}

func (s *EventSubscriber) unsubscribe(subRef string) {
    soap := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:Unsubscribe/>
  </s:Body>
</s:Envelope>`
    _, _ = s.sendSOAP(subRef, soap)
}

func (s *EventSubscriber) sendSOAP(endpoint, body string) ([]byte, error) {
    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Post(endpoint, "application/soap+xml; charset=utf-8", strings.NewReader(body))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return io.ReadAll(resp.Body)
}

type eventMessage struct {
    Topic   string `xml:"Topic"`
    Message struct {
        Data struct {
            SimpleItem []struct {
                Name  string `xml:"Name,attr"`
                Value string `xml:"Value,attr"`
            } `xml:"SimpleItem"`
        } `xml:"Data"`
    } `xml:"Message>Message"`
}

func isMotionEvent(msg eventMessage) bool {
    topic := strings.ToLower(msg.Topic)
    return strings.Contains(topic, "motion") || strings.Contains(topic, "cellmotion")
}

func isMotionActive(msg eventMessage) bool {
    for _, item := range msg.Message.Data.SimpleItem {
        if strings.EqualFold(item.Name, "IsMotion") || strings.EqualFold(item.Name, "State") {
            return item.Value == "true" || item.Value == "1"
        }
    }
    return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/nvr/scheduler/ -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```
git add internal/nvr/onvif/events.go internal/nvr/scheduler/motion.go internal/nvr/scheduler/motion_test.go
git commit -m "feat(nvr): add ONVIF event subscription and motion state machine"
```

---

### Task 5: Wire scheduler into NVR lifecycle and API

**Files:**

- Modify: `internal/nvr/nvr.go`
- Modify: `internal/nvr/api/router.go`
- Create: `internal/nvr/api/recording_rules.go`
- Modify: `internal/nvr/api/cameras.go`

- [ ] **Step 1: Add recording rules API handler**

Create `internal/nvr/api/recording_rules.go`:

```go
package api

import (
    "errors"
    "encoding/json"
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
    "github.com/bluenviron/mediamtx/internal/nvr/scheduler"
)

// RecordingRuleHandler implements HTTP endpoints for recording rules.
type RecordingRuleHandler struct {
    DB        *db.DB
    Scheduler *scheduler.Scheduler
}

type recordingRuleRequest struct {
    Name             string `json:"name" binding:"required"`
    Mode             string `json:"mode" binding:"required"`
    Days             []int  `json:"days" binding:"required"`
    StartTime        string `json:"start_time" binding:"required"`
    EndTime          string `json:"end_time" binding:"required"`
    PostEventSeconds int    `json:"post_event_seconds"`
    Enabled          *bool  `json:"enabled"`
}

func (h *RecordingRuleHandler) List(c *gin.Context) {
    cameraID := c.Param("id")
    rules, err := h.DB.ListRecordingRules(cameraID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
        return
    }
    if rules == nil {
        rules = []*db.RecordingRule{}
    }
    c.JSON(http.StatusOK, rules)
}

func (h *RecordingRuleHandler) Create(c *gin.Context) {
    cameraID := c.Param("id")

    // Verify camera exists.
    if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
        return
    }

    var req recordingRuleRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }

    if req.Mode != "always" && req.Mode != "events" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
        return
    }

    daysJSON, _ := json.Marshal(req.Days)
    enabled := true
    if req.Enabled != nil {
        enabled = *req.Enabled
    }

    rule := &db.RecordingRule{
        CameraID:         cameraID,
        Name:             req.Name,
        Mode:             req.Mode,
        Days:             string(daysJSON),
        StartTime:        req.StartTime,
        EndTime:          req.EndTime,
        PostEventSeconds: req.PostEventSeconds,
        Enabled:          enabled,
    }

    if err := h.DB.CreateRecordingRule(rule); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create rule"})
        return
    }

    c.JSON(http.StatusCreated, rule)
}

func (h *RecordingRuleHandler) Update(c *gin.Context) {
    id := c.Param("id")

    existing, err := h.DB.GetRecordingRule(id)
    if errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
        return
    }
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
        return
    }

    var req recordingRuleRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }

    daysJSON, _ := json.Marshal(req.Days)
    existing.Name = req.Name
    existing.Mode = req.Mode
    existing.Days = string(daysJSON)
    existing.StartTime = req.StartTime
    existing.EndTime = req.EndTime
    existing.PostEventSeconds = req.PostEventSeconds
    if req.Enabled != nil {
        existing.Enabled = *req.Enabled
    }

    if err := h.DB.UpdateRecordingRule(existing); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update rule"})
        return
    }

    c.JSON(http.StatusOK, existing)
}

func (h *RecordingRuleHandler) Delete(c *gin.Context) {
    id := c.Param("id")
    if err := h.DB.DeleteRecordingRule(id); errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
        return
    } else if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete rule"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

func (h *RecordingRuleHandler) Status(c *gin.Context) {
    cameraID := c.Param("id")
    if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
        return
    }

    state := h.Scheduler.GetCameraState(cameraID)
    c.JSON(http.StatusOK, gin.H{
        "effective_mode": state.EffectiveMode,
        "motion_state":   state.MotionState,
        "active_rules":   state.ActiveRules,
        "recording":      state.Recording,
    })
}
```

- [ ] **Step 2: Register routes in router.go**

Add to `RouterConfig`:

```go
Scheduler *scheduler.Scheduler
```

Add to `RegisterRoutes`:

```go
ruleHandler := &RecordingRuleHandler{
    DB:        cfg.DB,
    Scheduler: cfg.Scheduler,
}

protected.GET("/cameras/:id/recording-rules", ruleHandler.List)
protected.POST("/cameras/:id/recording-rules", ruleHandler.Create)
protected.PUT("/recording-rules/:id", ruleHandler.Update)
protected.DELETE("/recording-rules/:id", ruleHandler.Delete)
protected.GET("/cameras/:id/recording-status", ruleHandler.Status)
```

- [ ] **Step 3: Wire scheduler into NVR struct**

In `internal/nvr/nvr.go`:

- Add `scheduler *scheduler.Scheduler` field to NVR struct
- In `Initialize()`, after opening the database and yamlWriter, create and start the scheduler:
  ```go
  n.sched = scheduler.New(n.database, n.yamlWriter)
  n.sched.Start()
  ```
- In `Close()`, stop the scheduler:
  ```go
  if n.sched != nil {
      n.sched.Stop()
  }
  ```
- In `RegisterRoutes()`, pass `Scheduler: n.sched` in RouterConfig

- [ ] **Step 4: Call scheduler.RemoveCamera on camera delete**

In `internal/nvr/api/cameras.go` `Delete` handler, add after successful DB delete:

```go
if h.Scheduler != nil {
    h.Scheduler.RemoveCamera(id)
}
```

Add `Scheduler *scheduler.Scheduler` field to `CameraHandler` struct and pass it through `RouterConfig`.

- [ ] **Step 5: Build and verify**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 6: Commit**

```
git add internal/nvr/api/recording_rules.go internal/nvr/api/router.go internal/nvr/api/cameras.go internal/nvr/nvr.go
git commit -m "feat(nvr): wire recording schedule API, scheduler, and camera cleanup"
```

---

### Task 6: Build recording rules UI

**Files:**

- Create: `ui/src/hooks/useRecordingRules.ts`
- Create: `ui/src/components/RecordingRules.tsx`
- Create: `ui/src/components/SchedulePreview.tsx`
- Modify: `ui/src/pages/CameraManagement.tsx`

- [ ] **Step 1: Create useRecordingRules hook**

Create `ui/src/hooks/useRecordingRules.ts`:

```typescript
import { useState, useEffect, useCallback } from "react";
import { apiFetch } from "../api/client";

export interface RecordingRule {
  id: string;
  camera_id: string;
  name: string;
  mode: "always" | "events";
  days: string; // JSON array
  start_time: string;
  end_time: string;
  post_event_seconds: number;
  enabled: boolean;
}

export interface RecordingStatus {
  effective_mode: string;
  motion_state: string;
  active_rules: string[];
  recording: boolean;
}

export function useRecordingRules(cameraId: string | null) {
  const [rules, setRules] = useState<RecordingRule[]>([]);
  const [status, setStatus] = useState<RecordingStatus | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!cameraId) return;
    setLoading(true);
    const [rulesRes, statusRes] = await Promise.all([
      apiFetch(`/cameras/${cameraId}/recording-rules`),
      apiFetch(`/cameras/${cameraId}/recording-status`),
    ]);
    if (rulesRes.ok) setRules(await rulesRes.json());
    if (statusRes.ok) setStatus(await statusRes.json());
    setLoading(false);
  }, [cameraId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const createRule = async (rule: Omit<RecordingRule, "id" | "camera_id">) => {
    const res = await apiFetch(`/cameras/${cameraId}/recording-rules`, {
      method: "POST",
      body: JSON.stringify(rule),
    });
    if (res.ok) await refresh();
    return res.ok;
  };

  const updateRule = async (
    id: string,
    rule: Omit<RecordingRule, "id" | "camera_id">,
  ) => {
    const res = await apiFetch(`/recording-rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(rule),
    });
    if (res.ok) await refresh();
    return res.ok;
  };

  const deleteRule = async (id: string) => {
    const res = await apiFetch(`/recording-rules/${id}`, { method: "DELETE" });
    if (res.ok) await refresh();
    return res.ok;
  };

  return {
    rules,
    status,
    loading,
    refresh,
    createRule,
    updateRule,
    deleteRule,
  };
}
```

- [ ] **Step 2: Create SchedulePreview component**

Create `ui/src/components/SchedulePreview.tsx` — a 7×48 grid (30-min slots) that visualizes active rules per time slot. Color-coded: blue = always, amber = events, gray = no coverage. Uses the same `ruleMatchesTime` logic from the backend but in TypeScript.

- [ ] **Step 3: Create RecordingRules component**

Create `ui/src/components/RecordingRules.tsx` — the rules list + add/edit form:

- Shows rules as cards with name, mode badge, days, time range, enabled toggle
- Add Rule form: name, mode (Always/Events), day checkboxes with Weekdays/Weekends/Every Day shortcuts, time pickers, post-event buffer (events only)
- Edit and delete actions
- SchedulePreview at the bottom
- Live status indicator showing effective mode and motion state
- All styled with Tailwind using the nvr- color palette

- [ ] **Step 4: Add RecordingRules section to CameraManagement page**

In `ui/src/pages/CameraManagement.tsx`, add an expandable section per camera row or a detail panel that shows the `RecordingRules` component when a camera is selected.

- [ ] **Step 5: Build and verify**

Run from `ui/`: `npm run build`
Expected: Clean build.

- [ ] **Step 6: Commit**

```
git add ui/src/hooks/useRecordingRules.ts ui/src/components/RecordingRules.tsx ui/src/components/SchedulePreview.tsx ui/src/pages/CameraManagement.tsx
git commit -m "feat(nvr): add recording rules UI with schedule preview and status indicators"
```

---

### Task 7: Integration testing and polish

**Files:**

- Create: `internal/nvr/api/recording_rules_test.go`

- [ ] **Step 1: Write API handler tests**

Create `internal/nvr/api/recording_rules_test.go` testing:

- Create rule returns 201 with valid body
- Create rule returns 404 for non-existent camera
- Create rule returns 400 for invalid mode
- List rules returns empty array for camera with no rules
- Update rule returns 200
- Delete rule returns 200
- Status endpoint returns current effective mode

Follow the existing test pattern from `cameras_test.go`.

- [ ] **Step 2: Run all tests**

Run: `go test ./internal/nvr/... -v`
Expected: All tests pass.

- [ ] **Step 3: Build and run end-to-end**

```bash
cd ui && npm run build && cd ..
go generate ./...
go run .
```

Verify:

1. Server starts with no errors
2. Create a camera, add recording rules via UI
3. Rules appear in the list
4. Schedule preview shows correct coverage
5. Recording status updates as rules change

- [ ] **Step 4: Commit**

```
git add internal/nvr/api/recording_rules_test.go
git commit -m "test(nvr): add recording rules API integration tests"
```
