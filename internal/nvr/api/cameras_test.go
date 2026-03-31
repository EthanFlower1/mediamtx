package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

func setupCameraTest(t *testing.T) (*CameraHandler, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	yamlPath := filepath.Join(tmpDir, "mediamtx.yml")

	// Write a minimal YAML config with a paths key.
	if err := os.WriteFile(yamlPath, []byte("paths:\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := &CameraHandler{
		DB:         database,
		YAMLWriter: yamlwriter.New(yamlPath),
	}

	cleanup := func() {
		database.Close()
	}

	return handler, cleanup
}

func TestCameraList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var cameras []db.Camera
	if err := json.Unmarshal(w.Body.Bytes(), &cameras); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cameras) != 0 {
		t.Fatalf("expected 0 cameras, got %d", len(cameras))
	}
}

func TestCameraCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	body := `{"name": "Front Door", "rtsp_url": "rtsp://192.168.1.100/stream"}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var cam db.Camera
	if err := json.Unmarshal(w.Body.Bytes(), &cam); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cam.Name != "Front Door" {
		t.Fatalf("expected name %q, got %q", "Front Door", cam.Name)
	}
	// Path should now be nvr/<camera-id>/main (ID-based naming convention).
	expectedPath := "nvr/" + cam.ID + "/main"
	if cam.MediaMTXPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, cam.MediaMTXPath)
	}
	if cam.ID == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Front Door", "front-door"},
		{"My Camera 123", "my-camera-123"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "specialchars"},
		{"", "camera"},
		{"---dashes---", "dashes"},
	}

	for _, tt := range tests {
		got := sanitizePath(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

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

func TestUpdateCameraStoragePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, cleanup := setupCameraTest(t)
	defer cleanup()

	nasDir := t.TempDir()

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

func TestDetections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a camera and motion event with detections.
	cam := &db.Camera{
		ID:   "cam1",
		Name: "Test Cam",
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	evt := &db.MotionEvent{
		CameraID:  "cam1",
		StartedAt: "2026-03-24T10:00:00Z",
		EventType: "ai_detection",
		ObjectClass: "person",
		Confidence:  0.95,
	}
	require.NoError(t, handler.DB.InsertMotionEvent(evt))

	det := &db.Detection{
		MotionEventID: evt.ID,
		FrameTime:     "2026-03-24T10:00:01Z",
		Class:         "person",
		Confidence:    0.95,
		BoxX:          0.1,
		BoxY:          0.2,
		BoxW:          0.3,
		BoxH:          0.4,
	}
	require.NoError(t, handler.DB.InsertDetection(det))

	// Query with matching time range.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c.Request = httptest.NewRequest(http.MethodGet,
		"/?start=2026-03-24T09:59:00Z&end=2026-03-24T10:01:00Z", nil)

	handler.Detections(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var detections []db.Detection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detections))
	assert.Len(t, detections, 1)
	assert.Equal(t, "person", detections[0].Class)
	assert.InDelta(t, 0.1, detections[0].BoxX, 0.001)

	// Query with non-overlapping time range returns empty.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c2.Request = httptest.NewRequest(http.MethodGet,
		"/?start=2026-03-24T11:00:00Z&end=2026-03-24T12:00:00Z", nil)

	handler.Detections(c2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var empty []db.Detection
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &empty))
	assert.Empty(t, empty)

	// Missing params returns 400.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.Detections(c3)

	assert.Equal(t, http.StatusBadRequest, w3.Code)
}
