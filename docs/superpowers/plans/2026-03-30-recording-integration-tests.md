# Recording Schedule Integration Tests

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add integration tests that verify the full chain: API creates camera + streams → creates recording rules per stream → scheduler evaluates rules → YAML config is updated correctly → DB state is accurate.

**Architecture:** A single integration test file that exercises the real DB, real scheduler, and real YAML writer together. No mocks. Each test creates a camera with streams, assigns recording rules, triggers scheduler evaluation, then verifies DB state and YAML file contents. Uses the existing `setupRuleTest` pattern extended with stream and YAML verification helpers.

**Tech Stack:** Go `testing`, `httptest`, `gin`, `testify`, SQLite (via `db.Open`), real `yamlwriter.Writer`, real `scheduler.Scheduler`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/nvr/api/integration_test.go` | Create | End-to-end integration tests for camera config + recording schedules |

---

### Task 1: Integration test setup helpers and camera-with-streams test

**Files:**
- Create: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Create the test file with setup helpers and first test**

Create `internal/nvr/api/integration_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// integrationEnv holds all components for an integration test.
type integrationEnv struct {
	DB            *db.DB
	YAMLWriter    *yamlwriter.Writer
	Scheduler     *scheduler.Scheduler
	CameraHandler *CameraHandler
	RuleHandler   *RecordingRuleHandler
	StreamHandler *StreamHandler
	YAMLPath      string
}

func setupIntegration(t *testing.T) *integrationEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	yamlPath := filepath.Join(tmpDir, "mediamtx.yml")

	require.NoError(t, os.WriteFile(yamlPath, []byte("paths:\n"), 0o644))

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	writer := yamlwriter.New(yamlPath)
	sched := scheduler.New(database, writer, nil, nil, "")

	return &integrationEnv{
		DB:         database,
		YAMLWriter: writer,
		Scheduler:  sched,
		CameraHandler: &CameraHandler{
			DB:         database,
			YAMLWriter: writer,
		},
		RuleHandler: &RecordingRuleHandler{
			DB:        database,
			Scheduler: sched,
		},
		StreamHandler: &StreamHandler{
			DB: database,
		},
		YAMLPath: yamlPath,
	}
}

// readYAML returns the current YAML config file contents.
func (e *integrationEnv) readYAML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(e.YAMLPath)
	require.NoError(t, err)
	return string(data)
}

// apiCall is a helper that builds a gin context, calls a handler, and returns the recorder.
func apiCall(t *testing.T, method, body string, params gin.Params) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("camera_permissions", "*")
	return w, c
}

// createCamera creates a camera via the API and returns the camera ID and MediaMTX path.
func (e *integrationEnv) createCamera(t *testing.T, name, rtspURL string) (string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "rtsp_url": rtspURL})
	w, c := apiCall(t, http.MethodPost, string(body), nil)

	e.CameraHandler.Create(c)
	require.Equal(t, http.StatusCreated, w.Code, "create camera: %s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp["id"].(string), resp["mediamtx_path"].(string)
}

// createStream creates a stream for a camera via DB and returns the stream ID.
func (e *integrationEnv) createStream(t *testing.T, cameraID, name, rtspURL, roles string, width, height int) string {
	t.Helper()
	stream := &db.CameraStream{
		CameraID: cameraID,
		Name:     name,
		RTSPURL:  rtspURL,
		Roles:    roles,
		Width:    width,
		Height:   height,
	}
	require.NoError(t, e.DB.CreateCameraStream(stream))
	return stream.ID
}

// createRule creates a recording rule via the API and returns the rule ID.
func (e *integrationEnv) createRule(t *testing.T, cameraID string, ruleJSON string) string {
	t.Helper()
	w, c := apiCall(t, http.MethodPost, ruleJSON, gin.Params{{Key: "id", Value: cameraID}})

	e.RuleHandler.Create(c)
	require.Equal(t, http.StatusCreated, w.Code, "create rule: %s", w.Body.String())

	var rule db.RecordingRule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	return rule.ID
}

// flushScheduler triggers a scheduler evaluation and waits for YAML writes to flush.
func (e *integrationEnv) flushScheduler(t *testing.T) {
	t.Helper()
	// Export-safe: call evaluate via the public method that the tick loop uses.
	// Since evaluate() is unexported, we call it indirectly by starting and
	// immediately stopping the scheduler, which triggers one evaluation.
	// Alternative: use the EvaluateRules function + manual queueWrite.
	//
	// For integration tests we directly invoke the unexported evaluate() since
	// this test file is in the same package (api) — but the scheduler is in a
	// different package. Instead, we replicate the core logic:
	// 1. Read rules and cameras from DB
	// 2. Call scheduler.EvaluateRules (exported) to get mode
	// 3. Call yamlWriter.SetPathValue directly
	//
	// Actually, the simplest approach: start the scheduler, let it run one tick,
	// then stop it. The initial evaluation delay is 5 seconds so we need to
	// account for that, OR we can just call the exported test helper.
	//
	// Best approach: since setupRuleTest already creates a scheduler, and the
	// scheduler's evaluate() reads from DB and writes to YAML, we trigger it
	// by starting the scheduler and waiting for the first eval + flush.
	go e.Scheduler.Run()
	// Wait for initial delay (5s) + one eval cycle + write coalesce (500ms)
	time.Sleep(6 * time.Second)
	e.Scheduler.Stop()
}

// --- Test: Camera creation writes correct YAML path entry ---

func TestIntegration_CameraCreationWritesYAML(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Front Door", "rtsp://192.168.1.100:554/stream1")

	// Verify DB state.
	cam, err := env.DB.GetCamera(camID)
	require.NoError(t, err)
	assert.Equal(t, "Front Door", cam.Name)
	assert.Equal(t, "rtsp://192.168.1.100:554/stream1", cam.RTSPURL)
	assert.Equal(t, "nvr/"+camID+"/main", mtxPath)

	// Verify YAML has the path entry with correct source and record=true.
	yaml := env.readYAML(t)
	assert.Contains(t, yaml, "  "+mtxPath+":")
	assert.Contains(t, yaml, "source: rtsp://192.168.1.100:554/stream1")
	assert.Contains(t, yaml, "record: true")
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_CameraCreationWritesYAML -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for camera creation YAML verification"
```

---

### Task 2: Test camera with streams — DB correctness

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for camera + streams DB state**

Append to `integration_test.go`:

```go
func TestIntegration_CameraWithStreamsDBState(t *testing.T) {
	env := setupIntegration(t)

	camID, _ := env.createCamera(t, "Backyard", "rtsp://192.168.1.101:554/main")

	// Create two streams: main (high-res) and sub (low-res).
	mainStreamID := env.createStream(t, camID, "Main Stream",
		"rtsp://192.168.1.101:554/main", "live_view,recording", 1920, 1080)
	subStreamID := env.createStream(t, camID, "Sub Stream",
		"rtsp://192.168.1.101:554/sub", "ai_detection,mobile", 640, 480)

	// Verify streams are in DB.
	streams, err := env.DB.ListCameraStreams(camID)
	require.NoError(t, err)
	assert.Len(t, streams, 2)

	// Streams are ordered by resolution descending (largest first).
	assert.Equal(t, mainStreamID, streams[0].ID)
	assert.Equal(t, "Main Stream", streams[0].Name)
	assert.Equal(t, 1920, streams[0].Width)
	assert.True(t, streams[0].HasRole("recording"))
	assert.True(t, streams[0].HasRole("live_view"))

	assert.Equal(t, subStreamID, streams[1].ID)
	assert.Equal(t, "Sub Stream", streams[1].Name)
	assert.Equal(t, 640, streams[1].Width)
	assert.True(t, streams[1].HasRole("ai_detection"))
	assert.True(t, streams[1].HasRole("mobile"))

	// Verify stream resolution by role.
	recURL, err := env.DB.ResolveStreamURL(camID, "recording")
	require.NoError(t, err)
	assert.Equal(t, "rtsp://192.168.1.101:554/main", recURL)

	aiURL, err := env.DB.ResolveStreamURL(camID, "ai_detection")
	require.NoError(t, err)
	assert.Equal(t, "rtsp://192.168.1.101:554/sub", aiURL)
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_CameraWithStreamsDBState -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for camera streams DB state and role resolution"
```

---

### Task 3: Test recording rule creation stores correct DB data

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for recording rules with stream_id**

Append to `integration_test.go`:

```go
func TestIntegration_RecordingRuleWithStreamID(t *testing.T) {
	env := setupIntegration(t)

	camID, _ := env.createCamera(t, "Garage", "rtsp://192.168.1.102:554/main")
	mainStreamID := env.createStream(t, camID, "Main",
		"rtsp://192.168.1.102:554/main", "live_view,recording", 1920, 1080)
	subStreamID := env.createStream(t, camID, "Sub",
		"rtsp://192.168.1.102:554/sub", "ai_detection,mobile", 640, 480)

	// Create rule for main stream — always record weekdays.
	mainRuleID := env.createRule(t, camID, `{
		"name": "Main Weekday",
		"mode": "always",
		"days": [1, 2, 3, 4, 5],
		"start_time": "08:00",
		"end_time": "18:00",
		"stream_id": "`+mainStreamID+`"
	}`)

	// Create rule for sub stream — events only on weekends.
	subRuleID := env.createRule(t, camID, `{
		"name": "Sub Weekend Motion",
		"mode": "events",
		"days": [0, 6],
		"start_time": "00:00",
		"end_time": "23:59",
		"stream_id": "`+subStreamID+`",
		"post_event_seconds": 60
	}`)

	// Verify main stream rule in DB.
	mainRule, err := env.DB.GetRecordingRule(mainRuleID)
	require.NoError(t, err)
	assert.Equal(t, camID, mainRule.CameraID)
	assert.Equal(t, mainStreamID, mainRule.StreamID)
	assert.Equal(t, "always", mainRule.Mode)
	assert.Equal(t, "[1,2,3,4,5]", mainRule.Days)
	assert.Equal(t, "08:00", mainRule.StartTime)
	assert.Equal(t, "18:00", mainRule.EndTime)
	assert.True(t, mainRule.Enabled)

	// Verify sub stream rule in DB.
	subRule, err := env.DB.GetRecordingRule(subRuleID)
	require.NoError(t, err)
	assert.Equal(t, camID, subRule.CameraID)
	assert.Equal(t, subStreamID, subRule.StreamID)
	assert.Equal(t, "events", subRule.Mode)
	assert.Equal(t, "[0,6]", subRule.Days)
	assert.Equal(t, 60, subRule.PostEventSeconds)

	// Verify listing rules by camera returns both.
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	assert.Len(t, rules, 2)

	// Verify enabled rules query includes both.
	allEnabled, err := env.DB.ListAllEnabledRecordingRules()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allEnabled), 2)
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_RecordingRuleWithStreamID -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for recording rules with stream_id DB state"
```

---

### Task 4: Test scheduler evaluation updates YAML per-stream

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add scheduler evaluation test**

This test verifies the scheduler reads rules from DB, evaluates them correctly, and writes the right `record:` values to YAML. Since `evaluate()` is unexported and in a different package, we use `EvaluateRules` (exported) to verify logic, and `flushScheduler` to verify the full YAML write chain.

Append to `integration_test.go`:

```go
func TestIntegration_SchedulerEvaluatesRulesAndUpdatesYAML(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Driveway", "rtsp://192.168.1.103:554/main")

	// Create an "always" rule for the default stream that matches right now.
	now := time.Now()
	dayOfWeek := int(now.Weekday()) // 0=Sunday matches our days format
	startTime := "00:00"
	endTime := "23:59"

	env.createRule(t, camID, `{
		"name": "24/7 Recording",
		"mode": "always",
		"days": [`+itoa(dayOfWeek)+`],
		"start_time": "`+startTime+`",
		"end_time": "`+endTime+`"
	}`)

	// Verify the rule evaluates to ModeAlways using the exported function.
	rules, err := env.DB.ListAllEnabledRecordingRules()
	require.NoError(t, err)

	// Filter to this camera's rules.
	var camRules []*db.RecordingRule
	for _, r := range rules {
		if r.CameraID == camID {
			camRules = append(camRules, r)
		}
	}
	require.Len(t, camRules, 1)

	mode, activeIDs := scheduler.EvaluateRules(camRules, now)
	assert.Equal(t, scheduler.ModeAlways, mode)
	assert.Len(t, activeIDs, 1)

	// Trigger full scheduler evaluation → YAML write.
	env.flushScheduler(t)

	// Verify YAML has record: true for the camera path.
	yaml := env.readYAML(t)
	assert.Contains(t, yaml, mtxPath+":")
	// The path should have record: true (it was already true from camera creation,
	// and the scheduler should keep it true).
	assert.Contains(t, yaml, "record: true")
}

func itoa(n int) string {
	return strings.TrimRight(strings.TrimLeft(
		strings.Replace(
			strings.Replace(
				string(rune('0'+n)), "\x00", "", -1),
			"\x00", "", -1),
		"\x00"), "\x00")
}
```

Actually, let's use `strconv.Itoa` instead. Update the imports to include `"strconv"` and replace `itoa` with `strconv.Itoa`.

Replace the `itoa` helper and update the test to use `strconv.Itoa`:

```go
func TestIntegration_SchedulerEvaluatesRulesAndUpdatesYAML(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Driveway", "rtsp://192.168.1.103:554/main")

	// Create an "always" rule for the default stream that matches right now.
	now := time.Now()
	dayOfWeek := int(now.Weekday()) // 0=Sunday matches our days format

	env.createRule(t, camID, `{
		"name": "24/7 Recording",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// Verify the rule evaluates to ModeAlways using the exported function.
	rules, err := env.DB.ListAllEnabledRecordingRules()
	require.NoError(t, err)

	var camRules []*db.RecordingRule
	for _, r := range rules {
		if r.CameraID == camID {
			camRules = append(camRules, r)
		}
	}
	require.Len(t, camRules, 1)

	mode, activeIDs := scheduler.EvaluateRules(camRules, now)
	assert.Equal(t, scheduler.ModeAlways, mode)
	assert.Len(t, activeIDs, 1)

	// Trigger full scheduler evaluation → YAML write.
	env.flushScheduler(t)

	// Verify YAML still has record: true for the camera path.
	yaml := env.readYAML(t)
	assert.Contains(t, yaml, mtxPath+":")
	assert.Contains(t, yaml, "record: true")
}
```

Add `"strconv"` to the imports at the top of the file.

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_SchedulerEvaluatesRulesAndUpdatesYAML -v -count=1 -timeout=30s`
Expected: PASS (note: this test takes ~6s due to scheduler startup delay)

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for scheduler evaluation and YAML update"
```

---

### Task 5: Test disabling a rule turns off recording in YAML

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for rule disable → YAML record=false**

Append to `integration_test.go`:

```go
func TestIntegration_DisableRuleTurnsOffRecording(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Side Gate", "rtsp://192.168.1.104:554/main")

	now := time.Now()
	dayOfWeek := int(now.Weekday())

	ruleID := env.createRule(t, camID, `{
		"name": "Always On",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// First evaluation — rule is active, should be recording.
	env.flushScheduler(t)
	yaml1 := env.readYAML(t)
	assert.Contains(t, yaml1, mtxPath+":")
	assert.Contains(t, yaml1, "record: true")

	// Disable the rule via API.
	updateBody := `{
		"name": "Always On",
		"mode": "always",
		"days": [` + strconv.Itoa(dayOfWeek) + `],
		"start_time": "00:00",
		"end_time": "23:59",
		"enabled": false
	}`
	w, c := apiCall(t, http.MethodPut, updateBody, gin.Params{{Key: "id", Value: ruleID}})
	env.RuleHandler.Update(c)
	require.Equal(t, http.StatusOK, w.Code, "update rule: %s", w.Body.String())

	// Verify rule is disabled in DB.
	rule, err := env.DB.GetRecordingRule(ruleID)
	require.NoError(t, err)
	assert.False(t, rule.Enabled)

	// Re-evaluate scheduler — no active rules, recording should stop.
	env.flushScheduler(t)
	yaml2 := env.readYAML(t)
	assert.Contains(t, yaml2, mtxPath+":")
	assert.Contains(t, yaml2, "record: false")
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_DisableRuleTurnsOffRecording -v -count=1 -timeout=30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for rule disable turning off YAML recording"
```

---

### Task 6: Test deleting a rule and verifying cleanup

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for rule deletion + DB/YAML cleanup**

Append to `integration_test.go`:

```go
func TestIntegration_DeleteRuleCleansUpDBAndYAML(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Porch", "rtsp://192.168.1.105:554/main")

	now := time.Now()
	dayOfWeek := int(now.Weekday())

	ruleID := env.createRule(t, camID, `{
		"name": "Temp Rule",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// Evaluate — recording should be on.
	env.flushScheduler(t)
	yaml1 := env.readYAML(t)
	assert.Contains(t, yaml1, "record: true")

	// Delete the rule via API.
	w, c := apiCall(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: ruleID}})
	env.RuleHandler.Delete(c)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify rule is gone from DB.
	_, err := env.DB.GetRecordingRule(ruleID)
	assert.Error(t, err) // Should be ErrNotFound.

	// Verify listing returns empty.
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	assert.Len(t, rules, 0)

	// Re-evaluate — no rules, recording should stop.
	env.flushScheduler(t)
	yaml2 := env.readYAML(t)
	assert.Contains(t, yaml2, mtxPath+":")
	assert.Contains(t, yaml2, "record: false")
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_DeleteRuleCleansUpDBAndYAML -v -count=1 -timeout=30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for rule deletion DB and YAML cleanup"
```

---

### Task 7: Test multiple rules per camera — union logic

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for always-wins-over-events union logic**

Append to `integration_test.go`:

```go
func TestIntegration_MultipleRulesUnionLogic(t *testing.T) {
	env := setupIntegration(t)

	camID, _ := env.createCamera(t, "Lobby", "rtsp://192.168.1.106:554/main")

	now := time.Now()
	dayOfWeek := int(now.Weekday())

	// Rule 1: events mode.
	env.createRule(t, camID, `{
		"name": "Events Rule",
		"mode": "events",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// Rule 2: always mode (same camera, same time window).
	env.createRule(t, camID, `{
		"name": "Always Rule",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// Evaluate rules — "always" should win over "events".
	rules, err := env.DB.ListAllEnabledRecordingRules()
	require.NoError(t, err)

	var camRules []*db.RecordingRule
	for _, r := range rules {
		if r.CameraID == camID {
			camRules = append(camRules, r)
		}
	}
	require.Len(t, camRules, 2)

	mode, activeIDs := scheduler.EvaluateRules(camRules, now)
	assert.Equal(t, scheduler.ModeAlways, mode, "always should win over events")
	assert.Len(t, activeIDs, 2, "both rules should be active")
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_MultipleRulesUnionLogic -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for multiple rules union logic (always wins)"
```

---

### Task 8: Test per-stream rules create separate YAML paths

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for per-stream YAML paths**

Append to `integration_test.go`:

```go
func TestIntegration_PerStreamRulesCreateSeparateYAMLPaths(t *testing.T) {
	env := setupIntegration(t)

	camID, mtxPath := env.createCamera(t, "Warehouse", "rtsp://192.168.1.107:554/main")
	subStreamID := env.createStream(t, camID, "Sub Stream",
		"rtsp://192.168.1.107:554/sub", "recording", 640, 480)

	now := time.Now()
	dayOfWeek := int(now.Weekday())

	// Rule for default stream (no stream_id).
	env.createRule(t, camID, `{
		"name": "Main Always",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	// Rule for sub stream (with stream_id).
	env.createRule(t, camID, `{
		"name": "Sub Always",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59",
		"stream_id": "`+subStreamID+`"
	}`)

	// Verify rules are stored with correct stream_ids.
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	streamIDs := map[string]bool{}
	for _, r := range rules {
		streamIDs[r.StreamID] = true
	}
	assert.True(t, streamIDs[""], "should have a rule with empty stream_id (default)")
	assert.True(t, streamIDs[subStreamID], "should have a rule with sub stream_id")

	// Trigger scheduler — should create a separate YAML path for the sub-stream.
	env.flushScheduler(t)
	yaml := env.readYAML(t)

	// Main path should exist.
	assert.Contains(t, yaml, mtxPath+":")

	// Sub-stream path should be created: <mtxPath>~<prefix>.
	prefix := subStreamID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	subPath := mtxPath + "~" + prefix
	assert.Contains(t, yaml, subPath+":", "sub-stream path should exist in YAML")
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_PerStreamRulesCreateSeparateYAMLPaths -v -count=1 -timeout=30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for per-stream recording YAML path creation"
```

---

### Task 9: Test recording status API reflects scheduler state

**Files:**
- Modify: `internal/nvr/api/integration_test.go`

- [ ] **Step 1: Add test for status API endpoint**

Append to `integration_test.go`:

```go
func TestIntegration_StatusAPIReflectsSchedulerState(t *testing.T) {
	env := setupIntegration(t)

	camID, _ := env.createCamera(t, "Reception", "rtsp://192.168.1.108:554/main")

	// Status with no rules should be "off".
	w1, c1 := apiCall(t, http.MethodGet, "", gin.Params{{Key: "id", Value: camID}})
	env.RuleHandler.Status(c1)
	require.Equal(t, http.StatusOK, w1.Code)

	var status1 map[string]interface{}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &status1))
	assert.Equal(t, "off", status1["effective_mode"])
	assert.Equal(t, false, status1["recording"])

	// Create a matching always rule and evaluate.
	now := time.Now()
	dayOfWeek := int(now.Weekday())

	env.createRule(t, camID, `{
		"name": "Always On",
		"mode": "always",
		"days": [`+strconv.Itoa(dayOfWeek)+`],
		"start_time": "00:00",
		"end_time": "23:59"
	}`)

	env.flushScheduler(t)

	// Status should now reflect "always" mode and recording=true.
	w2, c2 := apiCall(t, http.MethodGet, "", gin.Params{{Key: "id", Value: camID}})
	env.RuleHandler.Status(c2)
	require.Equal(t, http.StatusOK, w2.Code)

	var status2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &status2))
	assert.Equal(t, "always", status2["effective_mode"])
	assert.Equal(t, true, status2["recording"])
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration_StatusAPIReflectsSchedulerState -v -count=1 -timeout=30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/integration_test.go
git commit -m "test: add integration test for recording status API reflecting scheduler state"
```

---

### Task 10: Run all integration tests together

- [ ] **Step 1: Run the full integration test suite**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestIntegration -v -count=1 -timeout=120s`
Expected: All TestIntegration_* tests pass.

- [ ] **Step 2: Run ALL existing tests to check for regressions**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1 -timeout=120s`
Expected: All packages pass (no regressions).

- [ ] **Step 3: Final commit if any fixups needed**

If any tests required fixups, commit them now.
