package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// BulkExportHandler implements HTTP endpoints for bulk export operations.
type BulkExportHandler struct {
	DB             *db.DB
	RecordingsPath string
	ExportPath     string // directory where zip files are written
}

// BulkExportItemRequest describes a single camera/time-range for bulk export.
type BulkExportItemRequest struct {
	CameraID  string `json:"camera_id" binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

// BulkExportRequest is the JSON body for POST /exports/bulk.
type BulkExportRequest struct {
	Items []BulkExportItemRequest `json:"items" binding:"required,min=1"`
}

// ManifestEntry describes a single file inside the exported zip.
type ManifestEntry struct {
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	FileName   string `json:"file_name"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	FileSize   int64  `json:"file_size"`
	SHA256     string `json:"sha256"`
}

// Manifest is the top-level manifest.json included in every bulk export zip.
type Manifest struct {
	ExportID  string          `json:"export_id"`
	CreatedAt string          `json:"created_at"`
	Items     int             `json:"items"`
	Files     []ManifestEntry `json:"files"`
}

// Create handles POST /exports/bulk. It validates the request, creates a job,
// runs the export synchronously, and returns the job status.
func (h *BulkExportHandler) Create(c *gin.Context) {
	var req BulkExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items array with camera_id, start_time, and end_time is required"})
		return
	}

	if len(req.Items) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maximum 100 items per bulk export"})
		return
	}

	// Validate times and camera permissions.
	for i, item := range req.Items {
		if !hasCameraPermission(c, item.CameraID) {
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("no permission for camera %s", item.CameraID)})
			return
		}
		if _, err := time.Parse(time.RFC3339, item.StartTime); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("item %d: invalid start_time, expected RFC3339", i)})
			return
		}
		if _, err := time.Parse(time.RFC3339, item.EndTime); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("item %d: invalid end_time, expected RFC3339", i)})
			return
		}
	}

	// Resolve camera names.
	dbItems := make([]*db.BulkExportItem, len(req.Items))
	for i, item := range req.Items {
		cameraName := item.CameraID // fallback
		cam, err := h.DB.GetCamera(item.CameraID)
		if err == nil {
			cameraName = cam.Name
		}
		dbItems[i] = &db.BulkExportItem{
			CameraID:   item.CameraID,
			CameraName: cameraName,
			StartTime:  item.StartTime,
			EndTime:    item.EndTime,
		}
	}

	// Create the job in DB.
	job := &db.BulkExportJob{}
	if err := h.DB.CreateBulkExportJob(job, dbItems); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create export job", err)
		return
	}

	// Run the export in a goroutine so the client gets the job ID immediately.
	go h.runExport(job.ID, dbItems)

	c.JSON(http.StatusAccepted, gin.H{
		"job_id": job.ID,
		"status": "pending",
		"total":  len(req.Items),
	})
}

// Status handles GET /exports/bulk/:id and returns job progress.
func (h *BulkExportHandler) Status(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetBulkExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	items, err := h.DB.GetBulkExportItems(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export items", err)
		return
	}

	if items == nil {
		items = []*db.BulkExportItem{}
	}

	c.JSON(http.StatusOK, gin.H{
		"job":   job,
		"items": items,
	})
}

// Download handles GET /exports/bulk/:id/download and serves the completed zip.
func (h *BulkExportHandler) Download(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetBulkExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	if job.Status != "completed" {
		c.JSON(http.StatusConflict, gin.H{"error": "export not yet completed", "status": job.Status})
		return
	}

	if job.ZipPath == nil || *job.ZipPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "zip file path not set"})
		return
	}

	if _, err := os.Stat(*job.ZipPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "zip file not found on disk"})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.FileAttachment(*job.ZipPath, filepath.Base(*job.ZipPath))
}

// List handles GET /exports/bulk and returns recent export jobs.
func (h *BulkExportHandler) List(c *gin.Context) {
	jobs, err := h.DB.ListBulkExportJobs(50)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list export jobs", err)
		return
	}
	if jobs == nil {
		jobs = []*db.BulkExportJob{}
	}
	c.JSON(http.StatusOK, jobs)
}

// Delete handles DELETE /exports/bulk/:id. Removes the job, items, and zip file.
func (h *BulkExportHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetBulkExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	// Remove zip file from disk if present.
	if job.ZipPath != nil && *job.ZipPath != "" {
		os.Remove(*job.ZipPath)
	}

	if err := h.DB.DeleteBulkExportJob(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete export job", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// runExport processes all items, collects recording files, and builds the zip.
func (h *BulkExportHandler) runExport(jobID string, items []*db.BulkExportItem) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in runExport for job %s: %v\n%s", jobID, r, debug.Stack())
			errMsg := fmt.Sprintf("internal error: %v", r)
			h.DB.CompleteBulkExportJob(jobID, "failed", nil, &errMsg)
		}
	}()

	// Mark job as processing.
	_ = h.DB.CompleteBulkExportJob(jobID, "processing", nil, nil)

	exportDir := h.ExportPath
	if exportDir == "" {
		exportDir = filepath.Join(h.RecordingsPath, ".exports")
	}
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		errMsg := fmt.Sprintf("failed to create export directory: %v", err)
		h.DB.CompleteBulkExportJob(jobID, "failed", nil, &errMsg)
		return
	}

	zipPath := filepath.Join(exportDir, fmt.Sprintf("export-%s.zip", jobID))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create zip file: %v", err)
		h.DB.CompleteBulkExportJob(jobID, "failed", nil, &errMsg)
		return
	}

	zipWriter := zip.NewWriter(zipFile)
	var manifestFiles []ManifestEntry

	for _, item := range items {
		start, _ := time.Parse(time.RFC3339, item.StartTime)
		end, _ := time.Parse(time.RFC3339, item.EndTime)

		recordings, err := h.DB.QueryRecordings(item.CameraID, start, end)
		if err != nil {
			errMsg := fmt.Sprintf("failed to query recordings: %v", err)
			if updErr := h.DB.UpdateBulkExportItemStatus(item.ID, "failed", 0, 0, &errMsg); updErr != nil {
				log.Printf("bulk export job %s: failed to update item %s progress: %v", jobID, item.ID, updErr)
			}
			continue
		}

		// Sanitize camera name for use as folder name.
		folderName := sanitizeFolderName(item.CameraName)
		if folderName == "" {
			folderName = item.CameraID
		}

		var itemBytes int64
		var itemFileCount int

		for _, rec := range recordings {
			if _, statErr := os.Stat(rec.FilePath); os.IsNotExist(statErr) {
				continue
			}

			baseName := filepath.Base(rec.FilePath)
			entryPath := filepath.Join(folderName, baseName)

			// Open source file.
			src, err := os.Open(rec.FilePath)
			if err != nil {
				continue
			}

			// Compute SHA-256 while writing to zip.
			hasher := sha256.New()

			w, err := zipWriter.Create(entryPath)
			if err != nil {
				src.Close()
				continue
			}

			multiWriter := io.MultiWriter(w, hasher)
			n, err := io.Copy(multiWriter, src)
			src.Close()
			if err != nil {
				continue
			}

			itemBytes += n
			itemFileCount++

			manifestFiles = append(manifestFiles, ManifestEntry{
				CameraID:   item.CameraID,
				CameraName: item.CameraName,
				FileName:   entryPath,
				StartTime:  rec.StartTime,
				EndTime:    rec.EndTime,
				FileSize:   n,
				SHA256:     hex.EncodeToString(hasher.Sum(nil)),
			})
		}

		if updErr := h.DB.UpdateBulkExportItemStatus(item.ID, "completed", itemFileCount, itemBytes, nil); updErr != nil {
			log.Printf("bulk export job %s: failed to update item %s progress: %v", jobID, item.ID, updErr)
		}
	}

	// Write manifest.json into the zip.
	manifest := Manifest{
		ExportID:  jobID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     len(items),
		Files:     manifestFiles,
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	if w, err := zipWriter.Create("manifest.json"); err == nil {
		w.Write(manifestBytes)
	}

	// Close zip.
	if err := zipWriter.Close(); err != nil {
		zipFile.Close()
		errMsg := fmt.Sprintf("failed to finalize zip: %v", err)
		h.DB.CompleteBulkExportJob(jobID, "failed", nil, &errMsg)
		return
	}
	zipFile.Close()

	h.DB.CompleteBulkExportJob(jobID, "completed", &zipPath, nil)
}

// sanitizeFolderName replaces characters that are problematic in file paths.
func sanitizeFolderName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return strings.TrimSpace(replacer.Replace(name))
}
