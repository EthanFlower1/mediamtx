package api

import (
	"net/http"
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

// Download is a stub endpoint for recording download.
func (h *RecordingHandler) Download(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "recording download not implemented"})
}

// Export is a stub endpoint for recording export.
func (h *RecordingHandler) Export(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "recording export not implemented"})
}
