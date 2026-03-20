package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
)

var timeFormatRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// RecordingRuleHandler implements HTTP endpoints for recording rule management.
type RecordingRuleHandler struct {
	DB        *db.DB
	Scheduler *scheduler.Scheduler
	Audit     *AuditLogger
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

// validateRuleRequest performs field-level validation on a recording rule request.
func validateRuleRequest(req *recordingRuleRequest) error {
	if len(req.Name) > 100 {
		return fmt.Errorf("name must be 100 characters or fewer")
	}
	if req.Mode != "always" && req.Mode != "events" {
		return fmt.Errorf("mode must be 'always' or 'events'")
	}
	for _, d := range req.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("days values must be between 0 and 6")
		}
	}
	if !timeFormatRe.MatchString(req.StartTime) {
		return fmt.Errorf("start_time must be in HH:MM format (00:00-23:59)")
	}
	if !timeFormatRe.MatchString(req.EndTime) {
		return fmt.Errorf("end_time must be in HH:MM format (00:00-23:59)")
	}
	if req.PostEventSeconds < 0 || req.PostEventSeconds > 3600 {
		return fmt.Errorf("post_event_seconds must be between 0 and 3600")
	}
	return nil
}

// List returns all recording rules for a camera.
func (h *RecordingRuleHandler) List(c *gin.Context) {
	cameraID := c.Param("id")

	if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to look up camera for rules", err)
		return
	}

	rules, err := h.DB.ListRecordingRules(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list recording rules", err)
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
		apiError(c, http.StatusInternalServerError, "failed to look up camera for rule creation", err)
		return
	}

	var req recordingRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := validateRuleRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default post_event_seconds to 30 for events mode.
	if req.PostEventSeconds == 0 && req.Mode == "events" {
		req.PostEventSeconds = 30
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
		apiError(c, http.StatusInternalServerError, "failed to create recording rule", err)
		return
	}

	nvrLogInfo("rules", fmt.Sprintf("Created recording rule %q for camera %s", rule.Name, cameraID))

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "recording_rule", rule.ID, fmt.Sprintf("Created rule %q for camera %s", rule.Name, cameraID))
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve recording rule", err)
		return
	}

	var req recordingRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := validateRuleRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		apiError(c, http.StatusInternalServerError, "failed to update recording rule", err)
		return
	}

	nvrLogInfo("rules", fmt.Sprintf("Updated recording rule %q (id=%s)", existing.Name, existing.ID))

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "recording_rule", existing.ID, fmt.Sprintf("Updated rule %q", existing.Name))
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
		apiError(c, http.StatusInternalServerError, "failed to delete recording rule", err)
		return
	}

	nvrLogInfo("rules", fmt.Sprintf("Deleted recording rule (id=%s)", id))

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "recording_rule", id, "Deleted recording rule")
	}

	c.JSON(http.StatusOK, gin.H{"message": "recording rule deleted"})
}

// Status returns the scheduler's current state for a camera.
func (h *RecordingRuleHandler) Status(c *gin.Context) {
	cameraID := c.Param("id")

	if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to look up camera for recording status", err)
		return
	}

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
