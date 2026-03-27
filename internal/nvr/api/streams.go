package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// StreamHandler implements HTTP endpoints for camera stream management.
type StreamHandler struct {
	DB *db.DB
}

// streamRequest is the JSON body for creating or updating a camera stream.
type streamRequest struct {
	Name         string `json:"name" binding:"required"`
	RTSPURL      string `json:"rtsp_url" binding:"required"`
	ProfileToken string `json:"profile_token"`
	VideoCodec   string `json:"video_codec"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Roles        string `json:"roles"`
}

// List returns all streams for a camera.
func (h *StreamHandler) List(c *gin.Context) {
	cameraID := c.Param("id")

	streams, err := h.DB.ListCameraStreams(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list camera streams", err)
		return
	}
	if streams == nil {
		streams = []*db.CameraStream{}
	}
	c.JSON(http.StatusOK, streams)
}

// Create creates a new stream for a camera.
func (h *StreamHandler) Create(c *gin.Context) {
	cameraID := c.Param("id")

	// Verify camera exists.
	if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to look up camera", err)
		return
	}

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	stream := &db.CameraStream{
		CameraID:     cameraID,
		Name:         req.Name,
		RTSPURL:      req.RTSPURL,
		ProfileToken: req.ProfileToken,
		VideoCodec:   req.VideoCodec,
		Width:        req.Width,
		Height:       req.Height,
		Roles:        req.Roles,
	}

	if err := h.DB.CreateCameraStream(stream); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create camera stream", err)
		return
	}

	c.JSON(http.StatusCreated, stream)
}

// Update updates an existing stream.
func (h *StreamHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetCameraStream(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve stream", err)
		return
	}

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	existing.Name = req.Name
	existing.RTSPURL = req.RTSPURL
	existing.ProfileToken = req.ProfileToken
	existing.VideoCodec = req.VideoCodec
	existing.Width = req.Width
	existing.Height = req.Height
	existing.Roles = req.Roles

	if err := h.DB.UpdateCameraStream(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update stream", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a stream by ID.
func (h *StreamHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.DeleteCameraStream(id); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete stream", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "stream deleted"})
}
