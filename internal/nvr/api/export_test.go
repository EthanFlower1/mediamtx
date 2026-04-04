package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupExportTest(t *testing.T) (*ExportHandler, *db.DB, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := &ExportHandler{
		DB:             database,
		RecordingsPath: tmpDir,
		ExportsPath:    filepath.Join(tmpDir, "exports"),
	}
	cleanup := func() { database.Close() }
	return handler, database, tmpDir, cleanup
}

func createExportTestCamera(t *testing.T, database *db.DB) *db.Camera {
	t.Helper()
	cam := &db.Camera{
		Name:    "TestCam",
		RTSPURL: "rtsp://192.168.1.1/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}
	return cam
}

func TestExportCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupExportTest(t)
	defer cleanup()

	cam := createExportTestCamera(t, database)

	body, _ := json.Marshal(CreateExportRequest{
		CameraID: cam.ID,
		Start:    "2025-01-01T00:00:00Z",
		End:      "2025-01-01T01:00:00Z",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")

	handler.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var job db.ExportJob
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if job.Status != "pending" {
		t.Fatalf("expected status pending, got %s", job.Status)
	}
}

func TestExportCreateBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupExportTest(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"camera_id": "test"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")

	handler.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestExportList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupExportTest(t)
	defer cleanup()

	cam := createExportTestCamera(t, database)

	// Create two jobs.
	job1 := &db.ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	job2 := &db.ExportJob{CameraID: cam.ID, StartTime: "2025-01-02T00:00:00Z", EndTime: "2025-01-02T01:00:00Z"}
	if err := database.CreateExportJob(job1); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := database.CreateExportJob(job2); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var jobs []*db.ExportJob
	if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestExportGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupExportTest(t)
	defer cleanup()

	cam := createExportTestCamera(t, database)

	job := &db.ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	if err := database.CreateExportJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	handler.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var got db.ExportJob
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ID != job.ID {
		t.Fatalf("expected job ID %s, got %s", job.ID, got.ID)
	}
}

func TestExportGetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupExportTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	handler.Get(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestExportDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupExportTest(t)
	defer cleanup()

	cam := createExportTestCamera(t, database)

	job := &db.ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	if err := database.CreateExportJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	handler.Delete(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Verify the job is deleted.
	_, err := database.GetExportJob(job.ID)
	if err != db.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestExportDeleteNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, cleanup := setupExportTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	handler.Delete(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestExportCreateForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupExportTest(t)
	defer cleanup()

	cam := createExportTestCamera(t, database)

	body, _ := json.Marshal(CreateExportRequest{
		CameraID: cam.ID,
		Start:    "2025-01-01T00:00:00Z",
		End:      "2025-01-01T01:00:00Z",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// Set restricted camera permissions (wrong camera).
	c.Set("camera_permissions", `["other-camera-id"]`)

	handler.Create(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}
