package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// StartReplaySession starts a replay session for a camera's edge recording.
// It returns the RTSP URI along with pre-formatted RTSP headers for Range,
// Scale, and Speed that the client should use when connecting.
//
//	POST /cameras/:id/replay/session
func (h *CameraHandler) StartReplaySession(c *gin.Context) {
	id := c.Param("id")

	var req onvif.ReplaySessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.RecordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for replay session", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	session, err := onvif.BuildReplaySessionFromRequest(cam.ONVIFEndpoint, cam.ONVIFUsername, password, &req)
	if err != nil {
		nvrLogError("replay", fmt.Sprintf("failed to build replay session for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to start replay session on device"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// GetReplayURI returns the RTSP replay URI for a recording on the camera.
// This is a lightweight alternative to StartReplaySession when the client
// only needs the URI without header construction.
//
//	GET /cameras/:id/replay/uri?recording_token=X
func (h *CameraHandler) GetReplayURI(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Query("recording_token")

	if recordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for replay URI", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	uri, err := onvif.GetReplayUri(cam.ONVIFEndpoint, cam.ONVIFUsername, password, recordingToken)
	if err != nil {
		nvrLogError("replay", fmt.Sprintf("failed to get replay URI for camera %s token %s", id, recordingToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get replay URI from device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"replay_uri": uri})
}

// GetReplayCapabilities returns the replay service capabilities for a camera.
//
//	GET /cameras/:id/replay/capabilities
func (h *CameraHandler) GetReplayCapabilities(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for replay capabilities", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	caps, err := onvif.GetReplayCapabilities(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		nvrLogError("replay", fmt.Sprintf("failed to get replay capabilities for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get replay capabilities from device"})
		return
	}

	c.JSON(http.StatusOK, caps)
}
