package api

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/thumbnail"
)

// ThumbnailHandler implements HTTP endpoints for timeline thumbnail strips.
type ThumbnailHandler struct {
	DB             *db.DB
	RecordingsPath string
}

// ThumbnailResponse is the JSON envelope returned by the List endpoint.
type ThumbnailResponse struct {
	CameraID   string              `json:"camera_id"`
	Start      string              `json:"start"`
	End        string              `json:"end"`
	Thumbnails []ThumbnailListItem `json:"thumbnails"`
}

// ThumbnailListItem is a single thumbnail entry in the list response.
type ThumbnailListItem struct {
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
}

// List handles GET /cameras/:id/thumbnails?start=...&end=...
// It returns thumbnail metadata for the requested time range.
func (h *ThumbnailHandler) List(c *gin.Context) {
	cameraID := c.Param("id")

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

	thumbs, err := thumbnail.ListThumbnails(h.RecordingsPath, cameraID, start, end)
	if err != nil {
		nvrLogError("thumbnails", "failed to list thumbnails", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list thumbnails"})
		return
	}

	items := make([]ThumbnailListItem, 0, len(thumbs))
	for _, t := range thumbs {
		items = append(items, ThumbnailListItem{
			Timestamp: t.Timestamp.UTC().Format(time.RFC3339),
			URL:       "/api/nvr/cameras/" + cameraID + "/thumbnails/" + t.Filename,
		})
	}

	c.JSON(http.StatusOK, ThumbnailResponse{
		CameraID:   cameraID,
		Start:      start.UTC().Format(time.RFC3339),
		End:        end.UTC().Format(time.RFC3339),
		Thumbnails: items,
	})
}

// Serve handles GET /cameras/:id/thumbnails/:filename
// It serves the individual thumbnail JPEG file.
func (h *ThumbnailHandler) Serve(c *gin.Context) {
	cameraID := c.Param("id")

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	filename := c.Param("filename")

	// Prevent directory traversal.
	filename = filepath.Base(filename)

	dir := thumbnail.ThumbnailDir(h.RecordingsPath, cameraID)
	fullPath := filepath.Join(dir, filename)

	c.File(fullPath)
}
