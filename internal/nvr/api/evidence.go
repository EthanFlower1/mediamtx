package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// EvidenceHandler implements chain-of-custody evidence export endpoints.
type EvidenceHandler struct {
	DB             *db.DB
	Audit          *AuditLogger
	RecordingsPath string
	ExportPath     string // directory where evidence zip files are written
}

// EvidenceExportRequest is the JSON body for creating an evidence export.
type EvidenceExportRequest struct {
	CameraID string `json:"camera_id" binding:"required"`
	Start    string `json:"start" binding:"required"`
	End      string `json:"end" binding:"required"`
	Notes    string `json:"notes"`
}

// ChainOfCustodyMetadata is the sidecar JSON included in the evidence zip.
type ChainOfCustodyMetadata struct {
	ExportID     string   `json:"export_id"`
	CameraID     string   `json:"camera_id"`
	CameraName   string   `json:"camera_name"`
	SerialNumber string   `json:"serial_number,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	StartTime    string   `json:"start_time"`
	EndTime      string   `json:"end_time"`
	ExportedBy   string   `json:"exported_by"`
	ExportedAt   string   `json:"exported_at"`
	Notes        string   `json:"notes,omitempty"`
	Files        []string `json:"files"`
	FileHashes   map[string]string `json:"file_hashes"`
}

// Create handles POST /exports/evidence — creates a chain-of-custody evidence export.
func (h *EvidenceHandler) Create(c *gin.Context) {
	var req EvidenceExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id, start, and end are required"})
		return
	}

	if !hasCameraPermission(c, req.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time, expected RFC3339"})
		return
	}

	end, err := time.Parse(time.RFC3339, req.End)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time, expected RFC3339"})
		return
	}

	// Get camera info for metadata.
	cam, err := h.DB.GetCamera(req.CameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}

	// Get device info for serial number if available.
	var deviceInfo *db.Device
	if cam.DeviceID != "" {
		deviceInfo, _ = h.DB.GetDevice(cam.DeviceID)
	}

	// Query recordings in the time range.
	recordings, err := h.DB.QueryRecordings(req.CameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
		return
	}

	if len(recordings) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no recordings found in the specified time range"})
		return
	}

	// Extract user info from context.
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = "unknown"
	}

	exportID := uuid.New().String()
	exportedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Ensure export directory exists.
	exportDir := h.ExportPath
	if exportDir == "" {
		exportDir = filepath.Join(h.RecordingsPath, ".evidence-exports")
	}
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create export directory", err)
		return
	}

	// Build the evidence zip.
	zipName := fmt.Sprintf("evidence_%s_%s.zip", cam.Name, exportID[:8])
	// Sanitize camera name for filename.
	zipName = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, zipName)
	zipPath := filepath.Join(exportDir, zipName)

	packageHash, metadata, err := h.buildEvidenceZip(zipPath, exportID, cam, deviceInfo, recordings, usernameStr, exportedAt, req)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create evidence package", err)
		return
	}

	// Persist the export record.
	export := &db.EvidenceExport{
		ID:         exportID,
		CameraID:   req.CameraID,
		CameraName: cam.Name,
		StartTime:  req.Start,
		EndTime:    req.End,
		ExportedBy: usernameStr,
		ExportedAt: exportedAt,
		SHA256Hash: packageHash,
		ZipPath:    zipPath,
		Notes:      req.Notes,
	}
	if err := h.DB.CreateEvidenceExport(export); err != nil {
		// Clean up the zip on DB failure.
		os.Remove(zipPath)
		apiError(c, http.StatusInternalServerError, "failed to save export record", err)
		return
	}

	// Log audit entry.
	h.Audit.logAction(c, "evidence_export", "recording", exportID,
		fmt.Sprintf("Evidence export for camera %s (%s) from %s to %s, %d segments, hash=%s",
			cam.Name, req.CameraID, req.Start, req.End, len(recordings), packageHash))

	c.JSON(http.StatusCreated, gin.H{
		"export":   export,
		"metadata": metadata,
		"download": fmt.Sprintf("/api/nvr/exports/evidence/%s/download", exportID),
	})
}

// buildEvidenceZip creates the tamper-evident evidence package.
// It returns the SHA-256 hash of the complete zip, the metadata, and any error.
func (h *EvidenceHandler) buildEvidenceZip(
	zipPath string,
	exportID string,
	cam *db.Camera,
	device *db.Device,
	recordings []*db.Recording,
	username, exportedAt string,
	req EvidenceExportRequest,
) (string, *ChainOfCustodyMetadata, error) {

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", nil, fmt.Errorf("create zip file: %w", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)

	fileHashes := make(map[string]string)
	var fileNames []string

	// Add each recording segment to the zip with its individual hash.
	for _, rec := range recordings {
		if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
			continue // skip missing files
		}

		baseName := filepath.Base(rec.FilePath)
		entryName := fmt.Sprintf("segments/%s", baseName)
		fileNames = append(fileNames, entryName)

		hash, err := addFileToZip(zw, entryName, rec.FilePath)
		if err != nil {
			zw.Close()
			return "", nil, fmt.Errorf("add recording to zip: %w", err)
		}
		fileHashes[entryName] = hash
	}

	// Build metadata.
	metadata := &ChainOfCustodyMetadata{
		ExportID:   exportID,
		CameraID:   cam.ID,
		CameraName: cam.Name,
		StartTime:  req.Start,
		EndTime:    req.End,
		ExportedBy: username,
		ExportedAt: exportedAt,
		Notes:      req.Notes,
		Files:      fileNames,
		FileHashes: fileHashes,
	}

	if device != nil {
		metadata.SerialNumber = device.ID
		metadata.Manufacturer = device.Manufacturer
		metadata.Model = device.Model
	}

	// Write metadata sidecar JSON.
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		zw.Close()
		return "", nil, fmt.Errorf("marshal metadata: %w", err)
	}

	metaWriter, err := zw.Create("chain_of_custody.json")
	if err != nil {
		zw.Close()
		return "", nil, fmt.Errorf("create metadata entry: %w", err)
	}
	if _, err := metaWriter.Write(metadataJSON); err != nil {
		zw.Close()
		return "", nil, fmt.Errorf("write metadata: %w", err)
	}

	// Write individual file hash manifest.
	var manifestLines []string
	for _, name := range fileNames {
		manifestLines = append(manifestLines, fmt.Sprintf("%s  %s", fileHashes[name], name))
	}
	manifest := strings.Join(manifestLines, "\n") + "\n"

	manifestWriter, err := zw.Create("manifest.sha256")
	if err != nil {
		zw.Close()
		return "", nil, fmt.Errorf("create manifest entry: %w", err)
	}
	if _, err := manifestWriter.Write([]byte(manifest)); err != nil {
		zw.Close()
		return "", nil, fmt.Errorf("write manifest: %w", err)
	}

	if err := zw.Close(); err != nil {
		return "", nil, fmt.Errorf("close zip: %w", err)
	}

	// Compute SHA-256 of the entire zip for tamper detection.
	packageHash, err := hashFile(zipPath)
	if err != nil {
		return "", nil, fmt.Errorf("hash zip: %w", err)
	}

	// Write the package hash as a sidecar .sha256 file alongside the zip.
	sha256Path := zipPath + ".sha256"
	hashContent := fmt.Sprintf("%s  %s\n", packageHash, filepath.Base(zipPath))
	if err := os.WriteFile(sha256Path, []byte(hashContent), 0o644); err != nil {
		return "", nil, fmt.Errorf("write sha256 file: %w", err)
	}

	return packageHash, metadata, nil
}

// addFileToZip adds a file to the zip writer and returns its SHA-256 hash.
func addFileToZip(zw *zip.Writer, entryName, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w, err := zw.Create(entryName)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	tee := io.TeeReader(f, hasher)

	if _, err := io.Copy(w, tee); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// hashFile computes the SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// List returns evidence exports, optionally filtered by camera_id.
// GET /exports/evidence?camera_id=...
func (h *EvidenceHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")

	exports, err := h.DB.ListEvidenceExports(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list evidence exports", err)
		return
	}

	if exports == nil {
		exports = []*db.EvidenceExport{}
	}

	c.JSON(http.StatusOK, exports)
}

// Download serves the evidence zip file for download.
// GET /exports/evidence/:id/download
func (h *EvidenceHandler) Download(c *gin.Context) {
	id := c.Param("id")

	export, err := h.DB.GetEvidenceExport(id)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "evidence export not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get evidence export", err)
		return
	}

	if !hasCameraPermission(c, export.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	if _, err := os.Stat(export.ZipPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evidence zip file not found on disk"})
		return
	}

	// Log download in audit trail.
	h.Audit.logAction(c, "evidence_download", "recording", id,
		fmt.Sprintf("Downloaded evidence export %s for camera %s", id, export.CameraName))

	c.Header("Content-Type", "application/zip")
	c.FileAttachment(export.ZipPath, filepath.Base(export.ZipPath))
}
