package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// GetRecordingConfig returns the recording configuration for a specific recording on the camera.
//
//	GET /cameras/:id/recording-control/config?token=X
func (h *CameraHandler) GetRecordingConfig(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Query("token")

	if recordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for recording config", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	config, err := onvif.GetRecordingConfiguration(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password, recordingToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to get recording config for camera %s token %s", id, recordingToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get recording configuration from device"})
		return
	}

	c.JSON(http.StatusOK, config)
}

// CreateEdgeRecording creates a new recording container on the camera's edge storage.
//
//	POST /cameras/:id/recording-control/recordings
func (h *CameraHandler) CreateEdgeRecording(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Source               onvif.RecordingSource `json:"source"`
		MaximumRetentionTime string                `json:"maximum_retention_time"`
		Content              string                `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for create recording", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	token, err := onvif.CreateRecording(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password,
		req.Source, req.MaximumRetentionTime, req.Content)
	if err != nil {
		nvrLogError("recording-control", fmt.Sprintf("failed to create recording on camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create recording on device"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"recording_token": token})
}

// DeleteEdgeRecording deletes a recording container from the camera's edge storage.
//
//	DELETE /cameras/:id/recording-control/recordings/:token
func (h *CameraHandler) DeleteEdgeRecording(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for delete recording", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	err = onvif.DeleteRecording(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password, recordingToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to delete recording on camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete recording on device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "recording deleted"})
}

// CreateEdgeRecordingJob creates a recording job on the camera's edge storage.
//
//	POST /cameras/:id/recording-control/jobs
func (h *CameraHandler) CreateEdgeRecordingJob(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		RecordingToken string `json:"recording_token"`
		Mode           string `json:"mode"`
		Priority       int    `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.RecordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token is required"})
		return
	}
	if req.Mode == "" {
		req.Mode = "Active"
	}
	if req.Priority == 0 {
		req.Priority = 1
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for create recording job", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	jobConfig, err := onvif.CreateRecordingJob(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password,
		req.RecordingToken, req.Mode, req.Priority)
	if err != nil {
		nvrLogError("recording-control", fmt.Sprintf("failed to create recording job on camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create recording job on device"})
		return
	}

	c.JSON(http.StatusCreated, jobConfig)
}

// DeleteEdgeRecordingJob deletes a recording job from the camera.
//
//	DELETE /cameras/:id/recording-control/jobs/:token
func (h *CameraHandler) DeleteEdgeRecordingJob(c *gin.Context) {
	id := c.Param("id")
	jobToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for delete recording job", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	err = onvif.DeleteRecordingJob(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password, jobToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to delete recording job on camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete recording job on device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "recording job deleted"})
}

// GetEdgeRecordingJobState returns the state of a recording job on the camera.
//
//	GET /cameras/:id/recording-control/jobs/:token/state
func (h *CameraHandler) GetEdgeRecordingJobState(c *gin.Context) {
	id := c.Param("id")
	jobToken := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for recording job state", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)
	state, err := onvif.GetRecordingJobState(
		cam.ONVIFEndpoint, cam.ONVIFUsername, password, jobToken)
	if err != nil {
		nvrLogError("recording-control",
			fmt.Sprintf("failed to get recording job state for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get recording job state from device"})
		return
	}

	c.JSON(http.StatusOK, state)
}
