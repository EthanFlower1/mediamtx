package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// integrationEnv holds all shared state for an integration test scenario.
type integrationEnv struct {
	TmpDir     string
	DB         *db.DB
	YAMLPath   string
	YAMLWriter *yamlwriter.Writer
	Scheduler  *scheduler.Scheduler

	CameraHandler *CameraHandler
	StreamHandler *StreamHandler
	RuleHandler   *RecordingRuleHandler
}

// setupIntegration creates a temp directory, SQLite database, YAML config file,
// yamlwriter, scheduler, and all three API handlers wired together.
func setupIntegration(t *testing.T) *integrationEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	yamlPath := filepath.Join(tmpDir, "mediamtx.yml")

	require.NoError(t, os.WriteFile(yamlPath, []byte("paths:\n"), 0o644))

	database, err := db.Open(dbPath)
	require.NoError(t, err)

	writer := yamlwriter.New(yamlPath)
	sched := scheduler.New(database, writer, nil, nil, "")

	env := &integrationEnv{
		TmpDir:     tmpDir,
		DB:         database,
		YAMLPath:   yamlPath,
		YAMLWriter: writer,
		Scheduler:  sched,
		CameraHandler: &CameraHandler{
			DB:         database,
			YAMLWriter: writer,
		},
		StreamHandler: &StreamHandler{
			DB: database,
		},
		RuleHandler: &RecordingRuleHandler{
			DB:        database,
			Scheduler: sched,
		},
	}

	t.Cleanup(func() {
		database.Close()
	})

	return env
}

// readYAML reads the current contents of the YAML config file.
func (e *integrationEnv) readYAML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(e.YAMLPath)
	require.NoError(t, err)
	return string(data)
}

// apiCall creates a gin test context with the given method, URL, and JSON body,
// then returns the recorder and context.
func apiCall(method, url, body string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, url, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return w, c
}

// createCamera calls the Camera Create API and returns the decoded response.
func (e *integrationEnv) createCamera(t *testing.T, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	w, c := apiCall(http.MethodPost, "/cameras", body)
	e.CameraHandler.Create(c)

	var resp map[string]interface{}
	if w.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	}
	return w, resp
}

// createStream calls the Stream Create API for the given camera ID and returns
// the decoded stream.
func (e *integrationEnv) createStream(t *testing.T, cameraID, body string) (*httptest.ResponseRecorder, *db.CameraStream) {
	t.Helper()
	w, c := apiCall(http.MethodPost, "/cameras/"+cameraID+"/streams", body)
	c.Params = gin.Params{{Key: "id", Value: cameraID}}
	e.StreamHandler.Create(c)

	var stream db.CameraStream
	if w.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stream))
	}
	return w, &stream
}

// createRule calls the RecordingRule Create API for the given camera ID and
// returns the decoded rule.
func (e *integrationEnv) createRule(t *testing.T, cameraID, body string) (*httptest.ResponseRecorder, *db.RecordingRule) {
	t.Helper()
	w, c := apiCall(http.MethodPost, "/cameras/"+cameraID+"/recording-rules", body)
	c.Params = gin.Params{{Key: "id", Value: cameraID}}
	e.RuleHandler.Create(c)

	var rule db.RecordingRule
	if w.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	}
	return w, &rule
}

// flushScheduler starts the scheduler, waits for its initial 5s delay + one
// eval cycle + 500ms write coalesce, then stops it. Safe to call multiple
// times because Stop() reinitializes the stop channel.
func (e *integrationEnv) flushScheduler(t *testing.T) {
	t.Helper()
	e.Scheduler.Start()
	time.Sleep(6 * time.Second)
	e.Scheduler.Stop()
}

// ---------------------------------------------------------------------------
// Test 1: Camera creation writes YAML
// ---------------------------------------------------------------------------

func TestIntegration_CameraCreationWritesYAML(t *testing.T) {
	env := setupIntegration(t)

	body := `{"name":"Garage","rtsp_url":"rtsp://192.168.1.50/stream"}`
	w, resp := env.createCamera(t, body)

	// --- API response checks ---
	require.Equal(t, http.StatusCreated, w.Code, "expected 201 Created, body: %s", w.Body.String())
	camID, ok := resp["id"].(string)
	require.True(t, ok && camID != "", "response must include a non-empty id")

	expectedPath := "nvr/" + camID + "/main"
	assert.Equal(t, expectedPath, resp["mediamtx_path"], "mediamtx_path should be nvr/<id>/main")

	// --- DB state checks ---
	cam, err := env.DB.GetCamera(camID)
	require.NoError(t, err)
	assert.Equal(t, "Garage", cam.Name)
	assert.Equal(t, "rtsp://192.168.1.50/stream", cam.RTSPURL)
	assert.Equal(t, expectedPath, cam.MediaMTXPath)

	// --- YAML file checks ---
	yamlContent := env.readYAML(t)
	assert.Contains(t, yamlContent, expectedPath+":")
	assert.Contains(t, yamlContent, "record: true")
	assert.Contains(t, yamlContent, "source: rtsp://192.168.1.50/stream")
}

// ---------------------------------------------------------------------------
// Test 2: Camera with streams — DB ordering and role resolution
// ---------------------------------------------------------------------------

func TestIntegration_CameraWithStreamsDBState(t *testing.T) {
	env := setupIntegration(t)

	// Create a camera first.
	camBody := `{"name":"Driveway","rtsp_url":"rtsp://192.168.1.60/main"}`
	wCam, camResp := env.createCamera(t, camBody)
	require.Equal(t, http.StatusCreated, wCam.Code, "camera create failed: %s", wCam.Body.String())
	camID := camResp["id"].(string)

	// Create two streams: a low-res sub-stream first, then a high-res main stream.
	subBody := `{
		"name": "Sub Stream",
		"rtsp_url": "rtsp://192.168.1.60/sub",
		"width": 640,
		"height": 480,
		"roles": "mobile,ai_detection"
	}`
	wSub, sub := env.createStream(t, camID, subBody)
	require.Equal(t, http.StatusCreated, wSub.Code, "sub stream create failed: %s", wSub.Body.String())

	mainBody := `{
		"name": "Main Stream",
		"rtsp_url": "rtsp://192.168.1.60/main",
		"width": 1920,
		"height": 1080,
		"roles": "live_view,recording"
	}`
	wMain, main := env.createStream(t, camID, mainBody)
	require.Equal(t, http.StatusCreated, wMain.Code, "main stream create failed: %s", wMain.Body.String())

	// --- DB ordering check: ListCameraStreams returns highest resolution first ---
	streams, err := env.DB.ListCameraStreams(camID)
	require.NoError(t, err)
	require.Len(t, streams, 2, "expected 2 streams")

	// First stream should be 1920x1080 (main), second 640x480 (sub).
	assert.Equal(t, main.ID, streams[0].ID, "highest-res stream should be first")
	assert.Equal(t, 1920, streams[0].Width)
	assert.Equal(t, 1080, streams[0].Height)

	assert.Equal(t, sub.ID, streams[1].ID, "lower-res stream should be second")
	assert.Equal(t, 640, streams[1].Width)
	assert.Equal(t, 480, streams[1].Height)

	// --- Role checks ---
	assert.True(t, streams[0].HasRole("live_view"), "main stream should have live_view role")
	assert.True(t, streams[0].HasRole("recording"), "main stream should have recording role")
	assert.False(t, streams[0].HasRole("mobile"), "main stream should not have mobile role")

	assert.True(t, streams[1].HasRole("mobile"), "sub stream should have mobile role")
	assert.True(t, streams[1].HasRole("ai_detection"), "sub stream should have ai_detection role")
	assert.False(t, streams[1].HasRole("live_view"), "sub stream should not have live_view role")

	// --- ResolveStreamURL should pick the right stream for each role ---
	liveURL, err := env.DB.ResolveStreamURL(camID, "live_view")
	require.NoError(t, err)
	assert.Equal(t, "rtsp://192.168.1.60/main", liveURL, "live_view should resolve to main stream URL")

	mobileURL, err := env.DB.ResolveStreamURL(camID, "mobile")
	require.NoError(t, err)
	assert.Equal(t, "rtsp://192.168.1.60/sub", mobileURL, "mobile should resolve to sub stream URL")
}

// ---------------------------------------------------------------------------
// Test 3: Recording rule with stream_id — full DB field verification
// ---------------------------------------------------------------------------

func TestIntegration_RecordingRuleWithStreamID(t *testing.T) {
	env := setupIntegration(t)

	// Create camera.
	camBody := `{"name":"Backyard","rtsp_url":"rtsp://192.168.1.70/main"}`
	wCam, camResp := env.createCamera(t, camBody)
	require.Equal(t, http.StatusCreated, wCam.Code, "camera create failed: %s", wCam.Body.String())
	camID := camResp["id"].(string)

	// Create a stream to target with the rule.
	streamBody := `{
		"name": "Recording Stream",
		"rtsp_url": "rtsp://192.168.1.70/sub",
		"width": 1280,
		"height": 720,
		"roles": "recording"
	}`
	wStream, stream := env.createStream(t, camID, streamBody)
	require.Equal(t, http.StatusCreated, wStream.Code, "stream create failed: %s", wStream.Body.String())

	// Create a recording rule that targets this specific stream.
	ruleBody := `{
		"name": "Night Recording",
		"mode": "always",
		"days": [0, 1, 2, 3, 4, 5, 6],
		"start_time": "22:00",
		"end_time": "06:00",
		"stream_id": "` + stream.ID + `"
	}`
	wRule, rule := env.createRule(t, camID, ruleBody)
	require.Equal(t, http.StatusCreated, wRule.Code, "rule create failed: %s", wRule.Body.String())

	// --- Verify all DB fields via direct lookup ---
	dbRule, err := env.DB.GetRecordingRule(rule.ID)
	require.NoError(t, err)

	assert.Equal(t, rule.ID, dbRule.ID)
	assert.Equal(t, camID, dbRule.CameraID)
	assert.Equal(t, stream.ID, dbRule.StreamID, "stream_id should be persisted")
	assert.Equal(t, "Night Recording", dbRule.Name)
	assert.Equal(t, "always", dbRule.Mode)
	assert.Equal(t, "[0,1,2,3,4,5,6]", dbRule.Days)
	assert.Equal(t, "22:00", dbRule.StartTime)
	assert.Equal(t, "06:00", dbRule.EndTime)
	assert.Equal(t, 0, dbRule.PostEventSeconds, "always mode should not default post_event_seconds")
	assert.True(t, dbRule.Enabled, "rule should default to enabled")
	assert.NotEmpty(t, dbRule.CreatedAt)
	assert.NotEmpty(t, dbRule.UpdatedAt)

	// --- Verify the rule shows up in list for this camera ---
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, rule.ID, rules[0].ID)
	assert.Equal(t, stream.ID, rules[0].StreamID)
}

// ---------------------------------------------------------------------------
// Test 4: Scheduler evaluates rules and updates YAML
// ---------------------------------------------------------------------------

func TestIntegration_SchedulerEvaluatesRulesAndUpdatesYAML(t *testing.T) {
	env := setupIntegration(t)

	// Create camera.
	camBody := `{"name":"Front Door","rtsp_url":"rtsp://192.168.1.80/main"}`
	wCam, camResp := env.createCamera(t, camBody)
	require.Equal(t, http.StatusCreated, wCam.Code, "camera create failed: %s", wCam.Body.String())
	camID := camResp["id"].(string)

	// Create an "always" rule that matches the current time.
	now := time.Now()
	dayStr := strconv.Itoa(int(now.Weekday()))
	ruleBody := `{
		"name": "Always Record",
		"mode": "always",
		"days": [` + dayStr + `],
		"start_time": "00:00",
		"end_time": "23:59"
	}`
	wRule, rule := env.createRule(t, camID, ruleBody)
	require.Equal(t, http.StatusCreated, wRule.Code, "rule create failed: %s", wRule.Body.String())

	// Verify EvaluateRules returns ModeAlways for this rule at current time.
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	mode, activeIDs := scheduler.EvaluateRules(rules, now)
	assert.Equal(t, scheduler.ModeAlways, mode, "expected ModeAlways from EvaluateRules")
	assert.Contains(t, activeIDs, rule.ID, "active IDs should contain our rule")

	// Flush scheduler to trigger full evaluation and YAML write.
	env.flushScheduler(t)

	// Verify YAML contains record: true for the camera path.
	yamlContent := env.readYAML(t)
	assert.Contains(t, yamlContent, "record: true", "YAML should contain record: true after scheduler flush")
}

// ---------------------------------------------------------------------------
// Test 5: Disabling a rule turns off recording
// ---------------------------------------------------------------------------

func TestIntegration_DisableRuleTurnsOffRecording(t *testing.T) {
	env := setupIntegration(t)

	// Create camera.
	camBody := `{"name":"Side Yard","rtsp_url":"rtsp://192.168.1.90/main"}`
	wCam, camResp := env.createCamera(t, camBody)
	require.Equal(t, http.StatusCreated, wCam.Code, "camera create failed: %s", wCam.Body.String())
	camID := camResp["id"].(string)

	// Create an "always" rule matching now.
	now := time.Now()
	dayStr := strconv.Itoa(int(now.Weekday()))
	ruleBody := `{
		"name": "Always Record",
		"mode": "always",
		"days": [` + dayStr + `],
		"start_time": "00:00",
		"end_time": "23:59"
	}`
	wRule, rule := env.createRule(t, camID, ruleBody)
	require.Equal(t, http.StatusCreated, wRule.Code, "rule create failed: %s", wRule.Body.String())

	// Flush scheduler — YAML should have record: true.
	env.flushScheduler(t)
	yamlContent := env.readYAML(t)
	assert.Contains(t, yamlContent, "record: true", "YAML should have record: true initially")

	// Update rule via API to set enabled: false.
	updateBody := `{
		"name": "Always Record",
		"mode": "always",
		"days": [` + dayStr + `],
		"start_time": "00:00",
		"end_time": "23:59",
		"enabled": false
	}`
	wUpdate, cUpdate := apiCall(http.MethodPut, "/recording-rules/"+rule.ID, updateBody)
	cUpdate.Params = gin.Params{{Key: "id", Value: rule.ID}}
	env.RuleHandler.Update(cUpdate)
	require.Equal(t, http.StatusOK, wUpdate.Code, "rule update failed: %s", wUpdate.Body.String())

	// Verify DB shows rule disabled.
	dbRule, err := env.DB.GetRecordingRule(rule.ID)
	require.NoError(t, err)
	assert.False(t, dbRule.Enabled, "rule should be disabled after update")

	// Flush scheduler again — YAML should now have record: false.
	env.flushScheduler(t)
	yamlContent = env.readYAML(t)
	assert.Contains(t, yamlContent, "record: false", "YAML should have record: false after disabling rule")
}

// ---------------------------------------------------------------------------
// Test 6: Deleting a rule cleans up DB and YAML
// ---------------------------------------------------------------------------

func TestIntegration_DeleteRuleCleansUpDBAndYAML(t *testing.T) {
	env := setupIntegration(t)

	// Create camera.
	camBody := `{"name":"Backyard Cam","rtsp_url":"rtsp://192.168.1.100/main"}`
	wCam, camResp := env.createCamera(t, camBody)
	require.Equal(t, http.StatusCreated, wCam.Code, "camera create failed: %s", wCam.Body.String())
	camID := camResp["id"].(string)

	// Create an "always" rule matching now.
	now := time.Now()
	dayStr := strconv.Itoa(int(now.Weekday()))
	ruleBody := `{
		"name": "Always Record",
		"mode": "always",
		"days": [` + dayStr + `],
		"start_time": "00:00",
		"end_time": "23:59"
	}`
	wRule, rule := env.createRule(t, camID, ruleBody)
	require.Equal(t, http.StatusCreated, wRule.Code, "rule create failed: %s", wRule.Body.String())

	// Flush scheduler — YAML should have record: true.
	env.flushScheduler(t)
	yamlContent := env.readYAML(t)
	assert.Contains(t, yamlContent, "record: true", "YAML should have record: true initially")

	// Delete rule via API.
	wDelete, cDelete := apiCall(http.MethodDelete, "/recording-rules/"+rule.ID, "")
	cDelete.Params = gin.Params{{Key: "id", Value: rule.ID}}
	env.RuleHandler.Delete(cDelete)
	require.Equal(t, http.StatusOK, wDelete.Code, "rule delete failed: %s", wDelete.Body.String())

	// Verify rule is gone from DB.
	_, err := env.DB.GetRecordingRule(rule.ID)
	assert.Error(t, err, "GetRecordingRule should return error for deleted rule")

	// Verify ListRecordingRules returns empty.
	rules, err := env.DB.ListRecordingRules(camID)
	require.NoError(t, err)
	assert.Empty(t, rules, "ListRecordingRules should return empty after deletion")

	// Flush scheduler — YAML should now have record: false.
	env.flushScheduler(t)
	yamlContent = env.readYAML(t)
	assert.Contains(t, yamlContent, "record: false", "YAML should have record: false after deleting rule")
}
