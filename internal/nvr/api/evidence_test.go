package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupEvidenceTest(t *testing.T) (*EvidenceHandler, *db.DB, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	exportDir := filepath.Join(tmpDir, "exports")

	handler := &EvidenceHandler{
		DB:             database,
		Audit:          &AuditLogger{DB: database},
		RecordingsPath: tmpDir,
		ExportPath:     exportDir,
	}

	cleanup := func() { database.Close() }
	return handler, database, tmpDir, cleanup
}

func insertTestCameraAndRecording(t *testing.T, database *db.DB, tmpDir string) (*db.Camera, *db.Recording) {
	t.Helper()

	cam := &db.Camera{
		Name:    "Lobby Camera",
		RTSPURL: "rtsp://192.168.1.100/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	filePath := filepath.Join(tmpDir, "test-segment.mp4")
	if err := os.WriteFile(filePath, []byte("fake-video-content-for-evidence"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rec := &db.Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-06-15T10:00:00.000Z",
		EndTime:    "2025-06-15T10:05:00.000Z",
		DurationMs: 300000,
		FilePath:   filePath,
		FileSize:   int64(len("fake-video-content-for-evidence")),
	}
	if err := database.InsertRecording(rec); err != nil {
		t.Fatalf("insert recording: %v", err)
	}

	return cam, rec
}

func TestEvidenceExportCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupEvidenceTest(t)
	defer cleanup()

	cam, _ := insertTestCameraAndRecording(t, database, tmpDir)

	body := `{"camera_id":"` + cam.ID + `","start":"2025-06-15T09:00:00Z","end":"2025-06-15T11:00:00Z","notes":"Incident #1234"}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")
	c.Set("username", "admin")
	c.Set("user_id", "user-1")

	handler.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Verify export record is in response.
	export, ok := resp["export"].(map[string]interface{})
	if !ok {
		t.Fatal("expected export in response")
	}
	if export["camera_id"] != cam.ID {
		t.Errorf("expected camera_id %s, got %v", cam.ID, export["camera_id"])
	}
	if export["sha256_hash"] == "" {
		t.Error("expected non-empty sha256_hash")
	}

	// Verify metadata is in response.
	metadata, ok := resp["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if metadata["camera_name"] != "Lobby Camera" {
		t.Errorf("expected camera_name 'Lobby Camera', got %v", metadata["camera_name"])
	}
	if metadata["exported_by"] != "admin" {
		t.Errorf("expected exported_by 'admin', got %v", metadata["exported_by"])
	}

	// Verify download URL is in response.
	downloadURL, ok := resp["download"].(string)
	if !ok || !strings.Contains(downloadURL, "/exports/evidence/") {
		t.Errorf("expected download URL, got %v", downloadURL)
	}

	// Verify zip file was created and is valid.
	zipPath := export["zip_path"].(string)
	verifyEvidenceZip(t, zipPath)

	// Verify .sha256 sidecar file exists.
	sha256Path := zipPath + ".sha256"
	sha256Content, err := os.ReadFile(sha256Path)
	if err != nil {
		t.Fatalf("read sha256 file: %v", err)
	}
	if !strings.Contains(string(sha256Content), export["sha256_hash"].(string)) {
		t.Error("sha256 sidecar file does not contain the package hash")
	}

	// Verify the hash in the sidecar matches the actual zip hash.
	actualHash := computeFileHash(t, zipPath)
	if !strings.Contains(string(sha256Content), actualHash) {
		t.Errorf("sha256 sidecar hash does not match actual file hash: sidecar=%s, actual=%s",
			strings.TrimSpace(string(sha256Content)), actualHash)
	}

	// Verify audit log entry was created.
	entries, _, err := database.QueryAuditLog(10, 0, "", "evidence_export")
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected audit log entry for evidence_export")
	}
}

func TestEvidenceExportNoRecordings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupEvidenceTest(t)
	defer cleanup()

	cam := &db.Camera{
		Name:    "Empty Camera",
		RTSPURL: "rtsp://192.168.1.200/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	body := `{"camera_id":"` + cam.ID + `","start":"2025-06-15T09:00:00Z","end":"2025-06-15T11:00:00Z"}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")
	c.Set("username", "admin")

	handler.Create(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestEvidenceExportForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupEvidenceTest(t)
	defer cleanup()

	cam := &db.Camera{
		Name:    "Restricted Camera",
		RTSPURL: "rtsp://192.168.1.201/stream",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	body := `{"camera_id":"` + cam.ID + `","start":"2025-06-15T09:00:00Z","end":"2025-06-15T11:00:00Z"}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", `["other-camera-id"]`)

	handler.Create(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestEvidenceExportList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, _, cleanup := setupEvidenceTest(t)
	defer cleanup()

	// Insert a test export record directly.
	export := &db.EvidenceExport{
		CameraID:   "cam-1",
		CameraName: "Test Camera",
		StartTime:  "2025-06-15T10:00:00.000Z",
		EndTime:    "2025-06-15T10:05:00.000Z",
		ExportedBy: "admin",
		SHA256Hash: "abc123",
		ZipPath:    "/tmp/test.zip",
	}

	// Need a camera for FK constraint.
	cam := &db.Camera{ID: "cam-1", Name: "Test Camera", RTSPURL: "rtsp://test/stream"}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	if err := database.CreateEvidenceExport(export); err != nil {
		t.Fatalf("create export: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?camera_id=cam-1", nil)
	c.Set("camera_permissions", "*")

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var exports []*db.EvidenceExport
	if err := json.Unmarshal(w.Body.Bytes(), &exports); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].CameraName != "Test Camera" {
		t.Errorf("expected camera_name 'Test Camera', got %s", exports[0].CameraName)
	}
}

func TestEvidenceExportDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupEvidenceTest(t)
	defer cleanup()

	cam := &db.Camera{ID: "cam-dl", Name: "Download Camera", RTSPURL: "rtsp://test/stream"}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("create camera: %v", err)
	}

	// Create a fake zip file.
	zipPath := filepath.Join(tmpDir, "evidence_test.zip")
	if err := os.WriteFile(zipPath, []byte("PK-fake-zip"), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	export := &db.EvidenceExport{
		ID:         "export-dl-1",
		CameraID:   "cam-dl",
		CameraName: "Download Camera",
		StartTime:  "2025-06-15T10:00:00.000Z",
		EndTime:    "2025-06-15T10:05:00.000Z",
		ExportedBy: "admin",
		SHA256Hash: "fakehash",
		ZipPath:    zipPath,
	}
	if err := database.CreateEvidenceExport(export); err != nil {
		t.Fatalf("create export: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "export-dl-1"}}
	c.Set("camera_permissions", "*")
	c.Set("username", "admin")
	c.Set("user_id", "user-1")

	handler.Download(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected Content-Type application/zip, got %s", ct)
	}
}

func TestEvidenceZipContents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, database, tmpDir, cleanup := setupEvidenceTest(t)
	defer cleanup()

	cam, _ := insertTestCameraAndRecording(t, database, tmpDir)

	body := `{"camera_id":"` + cam.ID + `","start":"2025-06-15T09:00:00Z","end":"2025-06-15T11:00:00Z","notes":"Test evidence"}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("camera_permissions", "*")
	c.Set("username", "officer_jones")
	c.Set("user_id", "user-2")

	handler.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	export := resp["export"].(map[string]interface{})
	zipPath := export["zip_path"].(string)

	// Open and verify zip contents.
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	fileNames := make(map[string]bool)
	for _, f := range zr.File {
		fileNames[f.Name] = true
	}

	// Must contain chain_of_custody.json.
	if !fileNames["chain_of_custody.json"] {
		t.Error("zip missing chain_of_custody.json")
	}

	// Must contain manifest.sha256.
	if !fileNames["manifest.sha256"] {
		t.Error("zip missing manifest.sha256")
	}

	// Must contain at least one segment.
	hasSegment := false
	for name := range fileNames {
		if strings.HasPrefix(name, "segments/") {
			hasSegment = true
			break
		}
	}
	if !hasSegment {
		t.Error("zip missing segments")
	}

	// Verify chain_of_custody.json content.
	for _, f := range zr.File {
		if f.Name == "chain_of_custody.json" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open metadata: %v", err)
			}
			var metadata ChainOfCustodyMetadata
			if err := json.NewDecoder(rc).Decode(&metadata); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
			rc.Close()

			if metadata.CameraName != "Lobby Camera" {
				t.Errorf("metadata camera_name: expected 'Lobby Camera', got %q", metadata.CameraName)
			}
			if metadata.ExportedBy != "officer_jones" {
				t.Errorf("metadata exported_by: expected 'officer_jones', got %q", metadata.ExportedBy)
			}
			if metadata.Notes != "Test evidence" {
				t.Errorf("metadata notes: expected 'Test evidence', got %q", metadata.Notes)
			}
			if len(metadata.FileHashes) == 0 {
				t.Error("metadata file_hashes should not be empty")
			}
		}
	}
}

// verifyEvidenceZip checks that a zip file is valid and contains the required files.
func verifyEvidenceZip(t *testing.T, zipPath string) {
	t.Helper()

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip %s: %v", zipPath, err)
	}
	defer zr.Close()

	hasMetadata := false
	hasManifest := false
	for _, f := range zr.File {
		if f.Name == "chain_of_custody.json" {
			hasMetadata = true
		}
		if f.Name == "manifest.sha256" {
			hasManifest = true
		}
	}

	if !hasMetadata {
		t.Error("zip missing chain_of_custody.json")
	}
	if !hasManifest {
		t.Error("zip missing manifest.sha256")
	}
}

// computeFileHash returns the SHA-256 hex digest of a file.
func computeFileHash(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file for hashing: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash file: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
