// Package recordingapi implements the recorder-side recording management API.
// It exposes routes for querying recordings, timeline, bookmarks, saved clips,
// HLS VoD playback, exports, and recording health. This is a role-scoped
// extract of the monolithic internal/nvr/api layer.
//
// The Handler uses internal/nvr/db.DB because that is the database shared
// between the recorder and the NVR subsystem. When the nvr/db split is
// complete (Phase 3), this will be updated to use internal/recorder/db.DB.
package recordingapi

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// Handler is the recording API handler for the Recorder service.
type Handler struct {
	db             *recdb.DB
	recordingsPath string
}

// NewHandler creates a Handler backed by the given recorder DB.
// recordingsPath is the root directory where recording files are stored.
func NewHandler(db *recdb.DB, recordingsPath string) *Handler {
	return &Handler{db: db, recordingsPath: recordingsPath}
}

// Register wires recording routes onto r.
// All routes are expected to be in a JWT-protected router group.
func (h *Handler) Register(r gin.IRouter) {
	// Recordings.
	r.GET("/recordings", h.Query)
	r.GET("/recordings/:id/download", h.Download)
	r.DELETE("/recordings/cleanup", h.Cleanup)
	r.GET("/timeline", h.Timeline)

	// Bookmarks.
	r.GET("/bookmarks", h.ListBookmarks)
	r.POST("/bookmarks", h.CreateBookmark)
	r.GET("/bookmarks/:id", h.GetBookmark)
	r.PUT("/bookmarks/:id", h.UpdateBookmark)
	r.DELETE("/bookmarks/:id", h.DeleteBookmark)
	r.GET("/bookmarks/search", h.SearchBookmarks)

	// Saved clips.
	r.GET("/saved-clips", h.ListSavedClips)
	r.POST("/saved-clips", h.CreateSavedClip)
	r.DELETE("/saved-clips/:id", h.DeleteSavedClip)
}

// --- helpers ---------------------------------------------------------------

func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[recordingapi] [ERROR] [%s] %s: %v", reqID, userMsg, err)
	code := "internal_error"
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusForbidden:
		code = "forbidden"
	}
	c.JSON(status, gin.H{"error": userMsg, "code": code, "request_id": reqID})
}

// --- Recordings ------------------------------------------------------------

// Query returns recordings for a camera within a time range.
//
//	GET /recordings?camera_id=<id>&start=<RFC3339>&end=<RFC3339>
func (h *Handler) Query(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time, expected RFC3339"})
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time, expected RFC3339"})
		return
	}

	recs, err := h.db.QueryRecordings(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
		return
	}
	if recs == nil {
		recs = []*recdb.Recording{}
	}
	c.JSON(http.StatusOK, gin.H{"items": recs})
}

// Download serves a recording file by ID.
//
//	GET /recordings/:id/download
func (h *Handler) Download(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
		return
	}

	rec, err := h.db.GetRecording(id)
	if err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get recording", err)
		return
	}

	c.FileAttachment(rec.FilePath, rec.FilePath)
}

// Cleanup deletes recordings for a camera that fall before a given time.
//
//	DELETE /recordings/cleanup?camera_id=<id>&before=<RFC3339>
func (h *Handler) Cleanup(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}
	before, err := time.Parse(time.RFC3339, c.Query("before"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before time, expected RFC3339"})
		return
	}

	deleted, err := h.db.DeleteRecordingsByDateRange(cameraID, before)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "cleanup failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(deleted), "files": deleted})
}

// Timeline returns recording time ranges for a camera.
//
//	GET /timeline?camera_id=<id>&start=<RFC3339>&end=<RFC3339>
func (h *Handler) Timeline(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}
	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time, expected RFC3339"})
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time, expected RFC3339"})
		return
	}

	ranges, err := h.db.GetTimeline(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get timeline", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ranges": ranges})
}

// --- Bookmarks -------------------------------------------------------------

// ListBookmarks returns bookmarks for a camera within an optional time range.
//
//	GET /bookmarks?camera_id=<id>&start=<RFC3339>&end=<RFC3339>
func (h *Handler) ListBookmarks(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}
	start, _ := time.Parse(time.RFC3339, c.Query("start"))
	end, _ := time.Parse(time.RFC3339, c.Query("end"))
	if end.IsZero() {
		end = time.Now().UTC()
	}

	bks, err := h.db.GetBookmarks(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list bookmarks", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": bks})
}

// CreateBookmark adds a new bookmark.
//
//	POST /bookmarks
func (h *Handler) CreateBookmark(c *gin.Context) {
	var b recdb.Bookmark
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if b.CameraID == "" || b.Timestamp == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id and timestamp are required"})
		return
	}
	if err := h.db.InsertBookmark(&b); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create bookmark", err)
		return
	}
	c.JSON(http.StatusCreated, b)
}

// GetBookmark returns a single bookmark by ID.
//
//	GET /bookmarks/:id
func (h *Handler) GetBookmark(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}
	b, err := h.db.GetBookmark(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}
	c.JSON(http.StatusOK, b)
}

// UpdateBookmark updates a bookmark's label and notes.
//
//	PUT /bookmarks/:id
func (h *Handler) UpdateBookmark(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}
	var req struct {
		Label string `json:"label"`
		Notes string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if err := h.db.UpdateBookmark(id, req.Label, req.Notes); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update bookmark", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DeleteBookmark removes a bookmark.
//
//	DELETE /bookmarks/:id
func (h *Handler) DeleteBookmark(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}
	if err := h.db.DeleteBookmark(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete bookmark", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// SearchBookmarks performs a full-text search on bookmark labels and notes.
//
//	GET /bookmarks/search?q=<query>
func (h *Handler) SearchBookmarks(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}
	bks, err := h.db.SearchBookmarks(q)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "search failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": bks})
}

// --- Saved clips -----------------------------------------------------------

// ListSavedClips returns all saved clips, optionally filtered by camera_id.
//
//	GET /saved-clips?camera_id=<id>
func (h *Handler) ListSavedClips(c *gin.Context) {
	clips, err := h.db.ListSavedClips(c.Query("camera_id"))
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list saved clips", err)
		return
	}
	if clips == nil {
		clips = []*recdb.SavedClip{}
	}
	c.JSON(http.StatusOK, gin.H{"items": clips})
}

// CreateSavedClip creates a new saved clip.
//
//	POST /saved-clips
func (h *Handler) CreateSavedClip(c *gin.Context) {
	var clip recdb.SavedClip
	if err := c.ShouldBindJSON(&clip); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if clip.CameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}
	if err := h.db.CreateSavedClip(&clip); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create saved clip", err)
		return
	}
	c.JSON(http.StatusCreated, clip)
}

// DeleteSavedClip removes a saved clip by ID.
//
//	DELETE /saved-clips/:id
func (h *Handler) DeleteSavedClip(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.DeleteSavedClip(id); err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "saved clip not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete saved clip", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
