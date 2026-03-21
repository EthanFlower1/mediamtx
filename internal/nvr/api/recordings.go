package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// hasCameraPermission checks whether the authenticated user has permission to
// access the given camera. It reads camera_permissions from the gin context
// (set by JWT middleware). "*" grants access to all cameras; otherwise the value
// is expected to be a JSON array of allowed camera IDs.
func hasCameraPermission(c *gin.Context, cameraID string) bool {
	permsRaw, exists := c.Get("camera_permissions")
	if !exists {
		return false
	}
	perms, ok := permsRaw.(string)
	if !ok {
		return false
	}
	if perms == "*" || perms == "" {
		return true
	}
	var ids []string
	if err := json.Unmarshal([]byte(perms), &ids); err != nil {
		return false
	}
	for _, id := range ids {
		if id == cameraID {
			return true
		}
	}
	return false
}

// RecordingHandler implements HTTP endpoints for recording queries.
type RecordingHandler struct {
	DB *db.DB
}

// Query returns recordings matching the given camera_id and time range.
// Query params: camera_id (required), start (RFC3339), end (RFC3339).
func (h *RecordingHandler) Query(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time, expected RFC3339"})
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time, expected RFC3339"})
		return
	}

	recordings, err := h.DB.QueryRecordings(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
		return
	}

	if recordings == nil {
		recordings = []*db.Recording{}
	}

	c.JSON(http.StatusOK, recordings)
}

// Timeline returns time ranges of recordings for a camera on a given date.
// Query params: camera_id (required), date (YYYY-MM-DD, required).
func (h *RecordingHandler) Timeline(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	dateStr := c.Query("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
		return
	}

	start := date
	end := date.Add(24 * time.Hour)

	ranges, err := h.DB.GetTimeline(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query timeline", err)
		return
	}

	if ranges == nil {
		ranges = []db.TimeRange{}
	}

	c.JSON(http.StatusOK, ranges)
}

// Download serves a recording file as a download attachment.
// Path param: id (recording ID, integer).
func (h *RecordingHandler) Download(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
		return
	}

	rec, err := h.DB.GetRecording(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve recording", err)
		return
	}

	if !hasCameraPermission(c, rec.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording file not found on disk"})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.FileAttachment(rec.FilePath, filepath.Base(rec.FilePath))
}

// MotionEvents returns motion events for a camera on a given date.
// Path param: id (camera ID). Query param: date (YYYY-MM-DD).
func (h *RecordingHandler) MotionEvents(c *gin.Context) {
	cameraID := c.Param("id")

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	dateStr := c.Query("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
		return
	}

	start := date
	end := date.Add(24 * time.Hour)

	events, err := h.DB.QueryMotionEvents(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query motion events", err)
		return
	}

	if events == nil {
		events = []*db.MotionEvent{}
	}

	c.JSON(http.StatusOK, events)
}

// CleanupRequest is the JSON body for the Cleanup endpoint.
type CleanupRequest struct {
	CameraID string `json:"camera_id" binding:"required"`
	Before   string `json:"before" binding:"required"`
}

// Cleanup deletes recordings older than a given time for a camera.
// It removes both DB records and files from disk.
// DELETE body: { camera_id, before (RFC3339) }.
func (h *RecordingHandler) Cleanup(c *gin.Context) {
	var req CleanupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id and before are required"})
		return
	}

	if !hasCameraPermission(c, req.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	before, err := time.Parse(time.RFC3339, req.Before)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before time, expected RFC3339"})
		return
	}

	paths, err := h.DB.DeleteRecordingsByDateRange(req.CameraID, before)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete recordings from database", err)
		return
	}

	var bytesFreed int64
	var filesDeleted int
	for _, p := range paths {
		info, statErr := os.Stat(p)
		if statErr == nil {
			bytesFreed += info.Size()
		}
		if rmErr := os.Remove(p); rmErr == nil {
			filesDeleted++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"deleted_count": len(paths),
		"files_removed": filesDeleted,
		"bytes_freed":   bytesFreed,
	})
}

// ExportRequest is the JSON body for the Export endpoint.
type ExportRequest struct {
	CameraID string `json:"camera_id" binding:"required"`
	Start    string `json:"start" binding:"required"`
	End      string `json:"end" binding:"required"`
}

// Export returns a list of download URLs for recording segments within a time range.
// POST body: { camera_id, start (RFC3339), end (RFC3339) }.
func (h *RecordingHandler) Export(c *gin.Context) {
	var req ExportRequest
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

	recordings, err := h.DB.QueryRecordings(req.CameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings for export", err)
		return
	}

	if recordings == nil {
		recordings = []*db.Recording{}
	}

	downloadURLs := make([]gin.H, 0, len(recordings))
	for _, rec := range recordings {
		downloadURLs = append(downloadURLs, gin.H{
			"id":         rec.ID,
			"start_time": rec.StartTime,
			"end_time":   rec.EndTime,
			"file_size":  rec.FileSize,
			"url":        fmt.Sprintf("/api/nvr/recordings/%d/download", rec.ID),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"segments": downloadURLs,
		"count":    len(recordings),
	})
}
