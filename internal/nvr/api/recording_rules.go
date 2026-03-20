package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
)

// RecordingRuleHandler implements HTTP endpoints for recording rule management.
type RecordingRuleHandler struct {
	DB        *db.DB
	Scheduler *scheduler.Scheduler
}

// recordingRuleRequest is the JSON body for creating or updating a recording rule.
type recordingRuleRequest struct {
	Name             string `json:"name" binding:"required"`
	Mode             string `json:"mode" binding:"required"`
	Days             []int  `json:"days" binding:"required"`
	StartTime        string `json:"start_time" binding:"required"`
	EndTime          string `json:"end_time" binding:"required"`
	PostEventSeconds int    `json:"post_event_seconds"`
	Enabled          *bool  `json:"enabled"`
}

// List returns all recording rules for a camera.
func (h *RecordingRuleHandler) List(c *gin.Context) {
	cameraID := c.Param("id")

	rules, err := h.DB.ListRecordingRules(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if rules == nil {
		rules = []*db.RecordingRule{}
	}
	c.JSON(http.StatusOK, rules)
}

// Create creates a new recording rule for a camera.
func (h *RecordingRuleHandler) Create(c *gin.Context) {
	cameraID := c.Param("id")

	// Validate camera exists.
	_, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var req recordingRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate mode.
	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	// Marshal days to JSON string.
	daysJSON, err := json.Marshal(req.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid days"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := &db.RecordingRule{
		CameraID:         cameraID,
		Name:             req.Name,
		Mode:             req.Mode,
		Days:             string(daysJSON),
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		PostEventSeconds: req.PostEventSeconds,
		Enabled:          enabled,
	}

	if err := h.DB.CreateRecordingRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create recording rule"})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// Update updates an existing recording rule.
func (h *RecordingRuleHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetRecordingRule(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording rule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var req recordingRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate mode.
	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	// Marshal days to JSON string.
	daysJSON, err := json.Marshal(req.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid days"})
		return
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	existing.Name = req.Name
	existing.Mode = req.Mode
	existing.Days = string(daysJSON)
	existing.StartTime = req.StartTime
	existing.EndTime = req.EndTime
	existing.PostEventSeconds = req.PostEventSeconds
	existing.Enabled = enabled

	if err := h.DB.UpdateRecordingRule(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update recording rule"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a recording rule.
func (h *RecordingRuleHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.DeleteRecordingRule(id); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording rule not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete recording rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "recording rule deleted"})
}

// Status returns the scheduler's current state for a camera.
func (h *RecordingRuleHandler) Status(c *gin.Context) {
	cameraID := c.Param("id")

	if h.Scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scheduler not available"})
		return
	}

	state := h.Scheduler.GetCameraState(cameraID)
	if state == nil {
		// Return a default "off" state if no rules have been evaluated for this camera.
		c.JSON(http.StatusOK, gin.H{
			"effective_mode": "off",
			"recording":      false,
			"motion_state":   "idle",
			"active_rules":   []string{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"effective_mode": state.EffectiveMode,
		"recording":      state.Recording,
		"motion_state":   state.MotionState,
		"active_rules":   state.ActiveRules,
	})
}
