package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupBulkExportTest(t *testing.T) (*BulkExportHandler, *db.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	handler := &BulkExportHandler{
		DB:             database,
		RecordingsPath: tmpDir,
		ExportPath:     filepath.Join(tmpDir, "exports"),
	}
	return handler, database, tmpDir
}

func createTestCameraWithRecording(t *testing.T, database *db.DB, tmpDir, cameraName string) *db.Camera {
	t.Helper()

	cam := &db.Camera{
		Name:         cameraName,
		RTSPURL:      "rtsp://192.168.1.1/stream",
		MediaMTXPath: "test-" + cameraName,
	}
	require.NoError(t, database.CreateCamera(cam))

	filePath := filepath.Join(tmpDir, cam.ID+"-recording.mp4")
	require.NoError(t, os.WriteFile(filePath, []byte("fake-mp4-data-for-"+cameraName), 0o644))

	rec := &db.Recording{
		CameraID:   cam.ID,
		StartTime:  "2026-01-01T00:00:00.000Z",
		EndTime:    "2026-01-01T01:00:00.000Z",
		DurationMs: 3600000,
		FilePath:   filePath,
		FileSize:   int64(len("fake-mp4-data-for-" + cameraName)),
	}
	require.NoError(t, database.InsertRecording(rec))

	return cam
}

func TestBulkExportCreate(t *testing.T) {
	handler, database, tmpDir := setupBulkExportTest(t)

	cam := createTestCameraWithRecording(t, database, tmpDir, "Front Door")

	body, _ := json.Marshal(BulkExportRequest{
		Items: []BulkExportItemRequest{
			{
				CameraID:  cam.ID,
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T02:00:00Z",
			},
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")

	handler.Create(c)

	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["job_id"])
	require.Equal(t, "pending", resp["status"])
	require.Equal(t, float64(1), resp["total"])
}

func TestBulkExportCreateValidation(t *testing.T) {
	handler, _, _ := setupBulkExportTest(t)

	tests := []struct {
		name   string
		body   interface{}
		status int
	}{
		{
			name:   "empty items",
			body:   map[string]interface{}{"items": []interface{}{}},
			status: http.StatusBadRequest,
		},
		{
			name: "invalid start_time",
			body: BulkExportRequest{
				Items: []BulkExportItemRequest{
					{CameraID: "cam-1", StartTime: "not-a-time", EndTime: "2026-01-01T00:00:00Z"},
				},
			},
			status: http.StatusBadRequest,
		},
		{
			name: "no permission",
			body: BulkExportRequest{
				Items: []BulkExportItemRequest{
					{CameraID: "cam-1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
				},
			},
			status: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.name == "no permission" {
				// Set restricted permissions.
				c.Set("camera_permissions", `["cam-other"]`)
			} else {
				c.Set("camera_permissions", "*")
			}

			handler.Create(c)
			require.Equal(t, tt.status, w.Code)
		})
	}
}

func TestBulkExportStatus(t *testing.T) {
	handler, database, _ := setupBulkExportTest(t)

	job := &db.BulkExportJob{}
	items := []*db.BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}
	require.NoError(t, database.CreateBulkExportJob(job, items))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	handler.Status(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp["job"])
	require.NotNil(t, resp["items"])
}

func TestBulkExportStatusNotFound(t *testing.T) {
	handler, _, _ := setupBulkExportTest(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	handler.Status(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestBulkExportList(t *testing.T) {
	handler, database, _ := setupBulkExportTest(t)

	// Create a job.
	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, []*db.BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.List(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp []*db.BulkExportJob
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
}

func TestBulkExportDelete(t *testing.T) {
	handler, database, _ := setupBulkExportTest(t)

	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, []*db.BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	handler.Delete(c)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify deleted.
	_, err := database.GetBulkExportJob(job.ID)
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestBulkExportRunExportProducesValidZip(t *testing.T) {
	handler, database, tmpDir := setupBulkExportTest(t)

	cam := createTestCameraWithRecording(t, database, tmpDir, "Front Door")

	items := []*db.BulkExportItem{
		{CameraID: cam.ID, CameraName: "Front Door", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T02:00:00Z"},
	}
	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, items))

	// Run export synchronously for testing.
	handler.runExport(job.ID, items)

	// Wait briefly for DB updates (runExport is synchronous here).
	got, err := database.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.NotNil(t, got.ZipPath)

	// Open the zip and verify structure.
	reader, err := zip.OpenReader(*got.ZipPath)
	require.NoError(t, err)
	defer reader.Close()

	fileNames := make(map[string]bool)
	for _, f := range reader.File {
		fileNames[f.Name] = true
	}

	require.True(t, fileNames["manifest.json"], "zip should contain manifest.json")

	// Should have a file under "Front Door/" folder.
	hasRecording := false
	for name := range fileNames {
		if name != "manifest.json" {
			require.Contains(t, name, "Front Door/")
			hasRecording = true
		}
	}
	require.True(t, hasRecording, "zip should contain at least one recording file")

	// Verify manifest content.
	for _, f := range reader.File {
		if f.Name == "manifest.json" {
			rc, err := f.Open()
			require.NoError(t, err)
			var manifest Manifest
			require.NoError(t, json.NewDecoder(rc).Decode(&manifest))
			rc.Close()

			require.Equal(t, job.ID, manifest.ExportID)
			require.Equal(t, 1, manifest.Items)
			require.Len(t, manifest.Files, 1)
			require.NotEmpty(t, manifest.Files[0].SHA256)
			require.Equal(t, cam.ID, manifest.Files[0].CameraID)
			break
		}
	}
}

func TestBulkExportDownloadNotCompleted(t *testing.T) {
	handler, database, _ := setupBulkExportTest(t)

	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, []*db.BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	handler.Download(c)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestBulkExportMultipleCameras(t *testing.T) {
	handler, database, tmpDir := setupBulkExportTest(t)

	cam1 := createTestCameraWithRecording(t, database, tmpDir, "Front Door")
	cam2 := createTestCameraWithRecording(t, database, tmpDir, "Back Yard")

	items := []*db.BulkExportItem{
		{CameraID: cam1.ID, CameraName: "Front Door", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T02:00:00Z"},
		{CameraID: cam2.ID, CameraName: "Back Yard", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T02:00:00Z"},
	}
	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, items))

	handler.runExport(job.ID, items)

	got, err := database.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.NotNil(t, got.ZipPath)

	reader, err := zip.OpenReader(*got.ZipPath)
	require.NoError(t, err)
	defer reader.Close()

	// Should have files under two different camera folders plus manifest.
	folders := make(map[string]bool)
	for _, f := range reader.File {
		if f.Name == "manifest.json" {
			continue
		}
		parts := filepath.SplitList(f.Name)
		if len(parts) == 0 {
			parts = []string{filepath.Dir(f.Name)}
		}
		folders[filepath.Dir(f.Name)] = true
	}
	require.Len(t, folders, 2, "should have recordings in two camera folders")
}

func TestSanitizeFolderName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Front Door", "Front Door"},
		{"Camera/1", "Camera_1"},
		{"Camera:Main", "Camera_Main"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, sanitizeFolderName(tt.input))
		})
	}
}

// Ensure no panic when the recording has no cameras set up, simulating
// a scenario where camera resolution fails gracefully.
func TestBulkExportMissingCamera(t *testing.T) {
	handler, database, _ := setupBulkExportTest(t)

	// Create job with a camera that doesn't exist in the DB.
	items := []*db.BulkExportItem{
		{CameraID: "nonexistent-cam", CameraName: "nonexistent-cam", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}
	job := &db.BulkExportJob{}
	require.NoError(t, database.CreateBulkExportJob(job, items))

	// Should complete without panic, with zero files.
	handler.runExport(job.ID, items)

	got, err := database.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)

	// Item should be completed with 0 files.
	gotItems, err := database.GetBulkExportItems(job.ID)
	require.NoError(t, err)
	require.Len(t, gotItems, 1)
	require.Equal(t, "completed", gotItems[0].Status)
	require.Equal(t, 0, gotItems[0].FileCount)
}

// pollJobStatus waits for a background export to finish (used when Create runs async).
func pollJobStatus(t *testing.T, database *db.DB, jobID string, timeout time.Duration) *db.BulkExportJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := database.GetBulkExportJob(jobID)
		require.NoError(t, err)
		if job.Status == "completed" || job.Status == "failed" {
			return job
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for export job to complete")
	return nil
}
