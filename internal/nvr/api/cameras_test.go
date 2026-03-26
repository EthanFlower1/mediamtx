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
