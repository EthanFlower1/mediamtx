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
)

func setupRecordingTest(t *testing.T) (*RecordingHandler, *db.DB, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := &RecordingHandler{DB: database}
	cleanup := func() { database.Close() }
	return handler, database, tmpDir, cleanup
}

func insertTestRecording(t *testing.T, database *db.DB, tmpDir string) *db.Recording {
	t.Helper()

	cam := &db.Camera{
		Name:    "TestCam",
		RTSPURL: "rtsp://192.168.1.1/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	// Create a real file on disk so download can serve it.
	filePath := filepath.Join(tmpDir, "test-recording.mp4")
	if err := os.WriteFile(filePath, []byte("fake-mp4-data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rec := &db.Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T10:00:00.000Z",
		EndTime:    "2025-01-15T10:05:00.000Z",
		DurationMs: 300000,
		FilePath:   filePath,
		FileSize:   13,
	}
	if err := database.InsertRecording(rec); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	return rec
}

func TestRecordingDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	rec := insertTestRecording(t, database, tmpDir)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	handler.Download(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Check Content-Disposition header.
	disp := w.Header().Get("Content-Disposition")
	if disp == "" {
		t.Fatal("expected Content-Disposition header")
	}

	// Verify the body contains our test data.
	if w.Body.String() != "fake-mp4-data" {
		t.Fatalf("expected body %q, got %q", "fake-mp4-data", w.Body.String())
	}

	_ = rec // used via insert
}

func TestRecordingDownloadNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupRecordingTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "999"}}

	handler.Download(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRecordingDownloadInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupRecordingTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-number"}}

	handler.Download(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestRecordingDownloadFileMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	cam := &db.Camera{
		Name:    "MissingCam",
		RTSPURL: "rtsp://192.168.1.2/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	rec := &db.Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T10:00:00.000Z",
		EndTime:    "2025-01-15T10:05:00.000Z",
		DurationMs: 300000,
		FilePath:   filepath.Join(tmpDir, "nonexistent.mp4"),
		FileSize:   100,
	}
	if err := database.InsertRecording(rec); err != nil {
		t.Fatalf("insert recording: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	handler.Download(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestRecordingExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	rec := insertTestRecording(t, database, tmpDir)

	body := `{"camera_id":"` + rec.CameraID + `","start":"2025-01-15T00:00:00Z","end":"2025-01-15T23:59:59Z"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Export(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	count, ok := resp["count"].(float64)
	if !ok || count != 1 {
		t.Fatalf("expected count 1, got %v", resp["count"])
	}

	segments, ok := resp["segments"].([]interface{})
	if !ok || len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %v", resp["segments"])
	}

	seg := segments[0].(map[string]interface{})
	if seg["url"] == nil || seg["url"] == "" {
		t.Fatal("expected non-empty url in segment")
	}
}

func TestRecordingExportEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	_ = insertTestRecording(t, database, tmpDir)

	// Query a date range with no recordings.
	body := `{"camera_id":"some-camera","start":"2024-01-01T00:00:00Z","end":"2024-01-01T23:59:59Z"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Export(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	count := resp["count"].(float64)
	if count != 0 {
		t.Fatalf("expected count 0, got %v", count)
	}
}

func TestRecordingExportMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupRecordingTest(t)
	defer cleanup()

	body := `{}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Export(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestRecordingQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	rec := insertTestRecording(t, database, tmpDir)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/?camera_id="+rec.CameraID+"&start=2025-01-15T00:00:00Z&end=2025-01-15T23:59:59Z", nil)

	handler.Query(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var recordings []*db.Recording
	if err := json.Unmarshal(w.Body.Bytes(), &recordings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(recordings) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recordings))
	}
}

func TestRecordingQueryCrossDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupRecordingTest(t)
	defer cleanup()

	cam := &db.Camera{
		Name:    "CrossDateCam",
		RTSPURL: "rtsp://192.168.1.3/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	// Insert recordings across multiple days.
	for _, r := range []struct {
		start, end, path string
	}{
		{"2025-01-14T22:00:00.000Z", "2025-01-14T22:05:00.000Z", filepath.Join(tmpDir, "a.mp4")},
		{"2025-01-15T10:00:00.000Z", "2025-01-15T10:05:00.000Z", filepath.Join(tmpDir, "b.mp4")},
		{"2025-01-16T08:00:00.000Z", "2025-01-16T08:05:00.000Z", filepath.Join(tmpDir, "c.mp4")},
	} {
		if err := database.InsertRecording(&db.Recording{
			CameraID:  cam.ID,
			StartTime: r.start,
			EndTime:   r.end,
			FilePath:  r.path,
		}); err != nil {
			t.Fatalf("insert recording: %v", err)
		}
	}

	// Query across all three days.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/?camera_id="+cam.ID+"&start=2025-01-14T00:00:00Z&end=2025-01-17T00:00:00Z", nil)

	handler.Query(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var recordings []*db.Recording
	if err := json.Unmarshal(w.Body.Bytes(), &recordings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(recordings) != 3 {
		t.Fatalf("expected 3 recordings across dates, got %d", len(recordings))
	}
}
