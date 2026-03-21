package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SavedClipHandler implements HTTP endpoints for saved clip management.
type SavedClipHandler struct {
	DB *db.DB
}

// CreateSavedClipRequest is the JSON body for creating a saved clip.
type CreateSavedClipRequest struct {
	CameraID  string `json:"camera_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
	Tags      string `json:"tags"`
	Notes     string `json:"notes"`
}

// List returns saved clips, optionally filtered by camera_id query parameter.
func (h *SavedClipHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")

	clips, err := h.DB.ListSavedClips(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list saved clips", err)
		return
	}

	if clips == nil {
		clips = []*db.SavedClip{}
	}

	c.JSON(http.StatusOK, clips)
}

// Create saves a new clip bookmark.
func (h *SavedClipHandler) Create(c *gin.Context) {
	var req CreateSavedClipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id, name, start_time, and end_time are required"})
		return
	}

	clip := &db.SavedClip{
		CameraID:  req.CameraID,
		Name:      req.Name,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Tags:      req.Tags,
		Notes:     req.Notes,
	}

	if err := h.DB.CreateSavedClip(clip); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save clip", err)
		return
	}

	c.JSON(http.StatusCreated, clip)
}

// Delete removes a saved clip by its ID.
func (h *SavedClipHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.DB.DeleteSavedClip(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "saved clip not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete saved clip", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
