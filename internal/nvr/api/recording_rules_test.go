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

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

func setupRuleTest(t *testing.T) (*RecordingRuleHandler, *db.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	yamlPath := filepath.Join(tmpDir, "mediamtx.yml")

	if err := os.WriteFile(yamlPath, []byte("paths:\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	sched := scheduler.New(database, yamlwriter.New(yamlPath))
	handler := &RecordingRuleHandler{DB: database, Scheduler: sched}

	cleanup := func() {
		database.Close()
	}

	return handler, database, cleanup
}

func makeRuleRequest(method, url string, body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, url, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return w, c
}

// createTestCamera is a helper that inserts a camera directly into the DB and
// returns its ID for use in recording-rule tests.
func createTestCamera(t *testing.T, database *db.DB) string {
	t.Helper()
	cam := &db.Camera{
		Name:         "Test Camera",
		RTSPURL:      "rtsp://192.168.1.1/stream",
		MediaMTXPath: "nvr/test-camera",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}
	return cam.ID
}

const validRuleBody = `{
	"name": "Weekday Schedule",
	"mode": "always",
	"days": [1, 2, 3, 4, 5],
	"start_time": "08:00",
	"end_time": "18:00"
}`

func TestRecordingRuleCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	w, c := makeRuleRequest(http.MethodPost, "/", validRuleBody)
	c.Params = gin.Params{{Key: "id", Value: cameraID}}

	handler.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var rule db.RecordingRule
	if err := json.Unmarshal(w.Body.Bytes(), &rule); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if rule.CameraID != cameraID {
		t.Fatalf("expected camera_id %q, got %q", cameraID, rule.CameraID)
	}
	if rule.Mode != "always" {
		t.Fatalf("expected mode %q, got %q", "always", rule.Mode)
	}
}

func TestRecordingRuleCreateInvalidMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	body := `{
		"name": "Bad Mode Rule",
		"mode": "invalid",
		"days": [1],
		"start_time": "08:00",
		"end_time": "18:00"
	}`
	w, c := makeRuleRequest(http.MethodPost, "/", body)
	c.Params = gin.Params{{Key: "id", Value: cameraID}}

	handler.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestRecordingRuleCreateCameraNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, cleanup := setupRuleTest(t)
	defer cleanup()

	w, c := makeRuleRequest(http.MethodPost, "/", validRuleBody)
	c.Params = gin.Params{{Key: "id", Value: "non-existent-camera-id"}}

	handler.Create(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRecordingRuleList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	// Create a rule first.
	wCreate, cCreate := makeRuleRequest(http.MethodPost, "/", validRuleBody)
	cCreate.Params = gin.Params{{Key: "id", Value: cameraID}}
	handler.Create(cCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201 on create, got %d", wCreate.Code)
	}

	// Now list.
	w, c := makeRuleRequest(http.MethodGet, "/", "")
	c.Params = gin.Params{{Key: "id", Value: cameraID}}

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rules []*db.RecordingRule
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestRecordingRuleListEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	w, c := makeRuleRequest(http.MethodGet, "/", "")
	c.Params = gin.Params{{Key: "id", Value: cameraID}}

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rules []*db.RecordingRule
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rules))
	}
}

func TestRecordingRuleUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	// Create a rule to update.
	wCreate, cCreate := makeRuleRequest(http.MethodPost, "/", validRuleBody)
	cCreate.Params = gin.Params{{Key: "id", Value: cameraID}}
	handler.Create(cCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201 on create, got %d", wCreate.Code)
	}

	var created db.RecordingRule
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}

	updateBody := `{
		"name": "Updated Schedule",
		"mode": "events",
		"days": [0, 6],
		"start_time": "00:00",
		"end_time": "23:59"
	}`
	w, c := makeRuleRequest(http.MethodPut, "/", updateBody)
	c.Params = gin.Params{{Key: "id", Value: created.ID}}

	handler.Update(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var updated db.RecordingRule
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if updated.Name != "Updated Schedule" {
		t.Fatalf("expected name %q, got %q", "Updated Schedule", updated.Name)
	}
	if updated.Mode != "events" {
		t.Fatalf("expected mode %q, got %q", "events", updated.Mode)
	}
}

func TestRecordingRuleUpdateNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, cleanup := setupRuleTest(t)
	defer cleanup()

	updateBody := `{
		"name": "Ghost Rule",
		"mode": "always",
		"days": [1],
		"start_time": "09:00",
		"end_time": "17:00"
	}`
	w, c := makeRuleRequest(http.MethodPut, "/", updateBody)
	c.Params = gin.Params{{Key: "id", Value: "non-existent-rule-id"}}

	handler.Update(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRecordingRuleDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	// Create a rule to delete.
	wCreate, cCreate := makeRuleRequest(http.MethodPost, "/", validRuleBody)
	cCreate.Params = gin.Params{{Key: "id", Value: cameraID}}
	handler.Create(cCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201 on create, got %d", wCreate.Code)
	}

	var created db.RecordingRule
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}

	w, c := makeRuleRequest(http.MethodDelete, "/", "")
	c.Params = gin.Params{{Key: "id", Value: created.ID}}

	handler.Delete(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestRecordingRuleDeleteNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, cleanup := setupRuleTest(t)
	defer cleanup()

	w, c := makeRuleRequest(http.MethodDelete, "/", "")
	c.Params = gin.Params{{Key: "id", Value: "non-existent-rule-id"}}

	handler.Delete(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRecordingRuleStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, cleanup := setupRuleTest(t)
	defer cleanup()

	cameraID := createTestCamera(t, database)

	w, c := makeRuleRequest(http.MethodGet, "/", "")
	c.Params = gin.Params{{Key: "id", Value: cameraID}}

	handler.Status(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// With no rules evaluated, the scheduler returns the default "off" state.
	effectiveMode, ok := resp["effective_mode"]
	if !ok {
		t.Fatal("expected 'effective_mode' key in response")
	}
	if effectiveMode != "off" {
		t.Fatalf("expected effective_mode %q, got %q", "off", effectiveMode)
	}
}
