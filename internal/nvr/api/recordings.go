package api

import (
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording file not found on disk"})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.FileAttachment(rec.FilePath, filepath.Base(rec.FilePath))
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
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
