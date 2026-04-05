package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ExportHandler implements HTTP endpoints and background processing for clip exports.
type ExportHandler struct {
	DB             *db.DB
	RecordingsPath string // base path for recording files
	ExportsPath    string // directory where exported files are written

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	startedAt map[string]time.Time // job ID -> time processing started
}

// Start begins the background export queue processor. It processes up to
// maxConcurrent jobs simultaneously.
func (h *ExportHandler) Start(maxConcurrent int) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.stopCh = make(chan struct{})
	h.startedAt = make(map[string]time.Time)
	h.mu.Unlock()

	if maxConcurrent < 1 {
		maxConcurrent = 2
	}

	// Ensure exports directory exists.
	if err := os.MkdirAll(h.ExportsPath, 0o755); err != nil {
		log.Printf("[NVR] [EXPORT] failed to create exports directory: %v", err)
	}

	sem := make(chan struct{}, maxConcurrent)

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-h.stopCh:
				return
			case <-ticker.C:
				pending, err := h.DB.GetPendingExportJobs()
				if err != nil {
					log.Printf("[NVR] [EXPORT] failed to get pending jobs: %v", err)
					continue
				}
				for _, job := range pending {
					select {
					case sem <- struct{}{}:
						go func(j *db.ExportJob) {
							defer func() { <-sem }()
							h.processJob(j)
						}(job)
					default:
						// All workers busy, wait for next tick.
					}
				}
			}
		}
	}()
}

// Stop stops the background export queue processor.
func (h *ExportHandler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	close(h.stopCh)
}

// processJob executes a single export job: queries recordings, concatenates
// segments into a single output file, and updates progress.
func (h *ExportHandler) processJob(job *db.ExportJob) {
	// Mark as processing and record start time for ETA calculation.
	if err := h.DB.UpdateExportJobStatus(job.ID, "processing", 0, ""); err != nil {
		log.Printf("[NVR] [EXPORT] failed to update job %s to processing: %v", job.ID, err)
		return
	}
	h.mu.Lock()
	h.startedAt[job.ID] = time.Now()
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.startedAt, job.ID)
		h.mu.Unlock()
	}()

	start, err := time.Parse(time.RFC3339, job.StartTime)
	if err != nil {
		h.failJob(job.ID, fmt.Sprintf("invalid start time: %v", err))
		return
	}
	end, err := time.Parse(time.RFC3339, job.EndTime)
	if err != nil {
		h.failJob(job.ID, fmt.Sprintf("invalid end time: %v", err))
		return
	}

	recordings, err := h.DB.QueryRecordingsBestQuality(job.CameraID, start, end)
	if err != nil {
		h.failJob(job.ID, fmt.Sprintf("failed to query recordings: %v", err))
		return
	}
	if len(recordings) == 0 {
		h.failJob(job.ID, "no recordings found in the specified time range")
		return
	}

	// Check for cancellation.
	if h.isCancelled(job.ID) {
		return
	}

	// Determine output file path.
	outputName := fmt.Sprintf("%s_%s_%s.mp4",
		job.CameraID,
		start.Format("20060102T150405Z"),
		end.Format("20060102T150405Z"),
	)
	outputPath := filepath.Join(h.ExportsPath, outputName)

	// Concatenate recording segments into a single file.
	err = h.concatenateSegments(job.ID, recordings, outputPath)
	if err != nil {
		// Clean up partial file.
		os.Remove(outputPath)
		h.failJob(job.ID, fmt.Sprintf("concatenation failed: %v", err))
		return
	}

	// Update output path and mark as completed.
	if err := h.DB.UpdateExportJobOutput(job.ID, outputPath); err != nil {
		log.Printf("[NVR] [EXPORT] failed to set output path for job %s: %v", job.ID, err)
	}
	if err := h.DB.UpdateExportJobStatus(job.ID, "completed", 100, ""); err != nil {
		log.Printf("[NVR] [EXPORT] failed to mark job %s as completed: %v", job.ID, err)
	}

	log.Printf("[NVR] [EXPORT] job %s completed: %s", job.ID, outputPath)
}

// concatenateSegments uses ffmpeg's concat demuxer to merge recording segments
// into a single valid MP4 file. It writes a concat list file, then runs ffmpeg
// with "-c copy" so no re-encoding occurs. Progress is updated after ffmpeg
// completes (since ffmpeg handles the actual muxing atomically).
func (h *ExportHandler) concatenateSegments(jobID string, recordings []*db.Recording, outputPath string) error {
	// Filter out segments whose files don't exist on disk.
	var validPaths []string
	for _, rec := range recordings {
		if _, err := os.Stat(rec.FilePath); err != nil {
			log.Printf("[NVR] [EXPORT] job %s: skipping missing segment %s: %v", jobID, rec.FilePath, err)
			continue
		}
		validPaths = append(validPaths, rec.FilePath)
	}
	if len(validPaths) == 0 {
		return fmt.Errorf("no valid segment files found on disk")
	}

	// Write the ffmpeg concat list file.
	concatListPath := outputPath + ".concat.txt"
	defer os.Remove(concatListPath)

	var listContent strings.Builder
	for _, p := range validPaths {
		// Escape single quotes in paths for ffmpeg concat format.
		escaped := strings.ReplaceAll(p, "'", "'\\''")
		fmt.Fprintf(&listContent, "file '%s'\n", escaped)
	}
	if err := os.WriteFile(concatListPath, []byte(listContent.String()), 0o644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	// Update progress to indicate concatenation is starting.
	_ = h.DB.UpdateExportJobStatus(jobID, "processing", 10, "")

	if h.isCancelled(jobID) {
		return fmt.Errorf("job cancelled")
	}

	// Run ffmpeg concat demuxer.
	cmd := exec.Command("ffmpeg",
		"-y",             // overwrite output
		"-f", "concat",   // concat demuxer
		"-safe", "0",     // allow absolute paths
		"-i", concatListPath,
		"-c", "copy",     // no re-encoding
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat failed: %w\noutput: %s", err, string(output))
	}

	// Update progress to 95% (completed will be set by caller).
	_ = h.DB.UpdateExportJobStatus(jobID, "processing", 95, "")

	return nil
}

// isCancelled checks if a job has been cancelled by reading the current DB status.
func (h *ExportHandler) isCancelled(jobID string) bool {
	job, err := h.DB.GetExportJob(jobID)
	if err != nil {
		return false
	}
	return job.Status == "cancelled"
}

// failJob marks an export job as failed with an error message.
func (h *ExportHandler) failJob(jobID, errMsg string) {
	log.Printf("[NVR] [EXPORT] job %s failed: %s", jobID, errMsg)
	_ = h.DB.UpdateExportJobStatus(jobID, "failed", 0, errMsg)
}

// CreateExportRequest is the JSON body for creating an export job.
type CreateExportRequest struct {
	CameraID string `json:"camera_id" binding:"required"`
	Start    string `json:"start" binding:"required"`
	End      string `json:"end" binding:"required"`
}

// Create queues a new export job.
// POST /api/nvr/exports
func (h *ExportHandler) Create(c *gin.Context) {
	var req CreateExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id, start, and end are required"})
		return
	}

	if !requireCameraPermission(c, req.CameraID, db.PermExport) {
		return
	}

	// Validate time formats.
	if _, err := time.Parse(time.RFC3339, req.Start); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time, expected RFC3339"})
		return
	}
	if _, err := time.Parse(time.RFC3339, req.End); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time, expected RFC3339"})
		return
	}

	job := &db.ExportJob{
		CameraID:  req.CameraID,
		StartTime: req.Start,
		EndTime:   req.End,
	}

	if err := h.DB.CreateExportJob(job); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create export job", err)
		return
	}

	c.JSON(http.StatusCreated, job)
}

// List returns export jobs, optionally filtered by camera_id and status query parameters.
// GET /api/nvr/exports
func (h *ExportHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")
	status := c.Query("status")

	jobs, err := h.DB.ListExportJobs(cameraID, status)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list export jobs", err)
		return
	}

	if jobs == nil {
		jobs = []*db.ExportJob{}
	}

	c.JSON(http.StatusOK, jobs)
}

// ExportJobResponse extends ExportJob with a computed ETA field.
type ExportJobResponse struct {
	*db.ExportJob
	ETASeconds *float64 `json:"eta_seconds,omitempty"`
}

// Get returns a single export job by ID, including computed ETA for in-progress jobs.
// GET /api/nvr/exports/:id
func (h *ExportHandler) Get(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	resp := ExportJobResponse{ExportJob: job}

	// Compute ETA for in-progress jobs.
	if job.Status == "processing" && job.Progress > 0 {
		h.mu.Lock()
		started, ok := h.startedAt[job.ID]
		h.mu.Unlock()
		if ok {
			elapsed := time.Since(started).Seconds()
			remaining := elapsed / job.Progress * (100 - job.Progress)
			resp.ETASeconds = &remaining
		}
	}

	c.JSON(http.StatusOK, resp)
}

// Delete cancels a pending/processing export job or deletes a completed/failed one.
// DELETE /api/nvr/exports/:id
func (h *ExportHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	// If the job is still pending or processing, cancel it first.
	if job.Status == "pending" || job.Status == "processing" {
		if err := h.DB.UpdateExportJobStatus(id, "cancelled", job.Progress, "cancelled by user"); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to cancel export job", err)
			return
		}
	}

	// Clean up the output file if it exists.
	if job.OutputPath != "" {
		os.Remove(job.OutputPath)
	}

	if err := h.DB.DeleteExportJob(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete export job", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// Download serves the exported file for a completed export job.
// GET /api/nvr/exports/:id/download
func (h *ExportHandler) Download(c *gin.Context) {
	id := c.Param("id")

	job, err := h.DB.GetExportJob(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get export job", err)
		return
	}

	if job.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "export is not yet completed"})
		return
	}

	if _, err := os.Stat(job.OutputPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "export file not found on disk"})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.FileAttachment(job.OutputPath, filepath.Base(job.OutputPath))
}
