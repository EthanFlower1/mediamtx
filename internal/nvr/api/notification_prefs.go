package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// NotificationPrefsHandler implements the notification preferences, quiet hours,
// and escalation rule endpoints for the Customer Admin notifications page.
type NotificationPrefsHandler struct {
	DB *db.DB
}

// --- Notification Preference Matrix ---

// ListPreferences returns all notification preferences for the authenticated user.
//
//	GET /api/nvr/notification-preferences
func (h *NotificationPrefsHandler) ListPreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	if uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	prefs, err := h.DB.ListNotificationPreferences(uid)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list notification preferences", err)
		return
	}
	if prefs == nil {
		prefs = []*db.NotificationPreference{}
	}
	c.JSON(http.StatusOK, prefs)
}

// bulkPrefRequest is the JSON body for batch-updating preferences.
type bulkPrefRequest struct {
	Preferences []prefEntry `json:"preferences" binding:"required"`
}

type prefEntry struct {
	CameraID  string `json:"camera_id"`
	EventType string `json:"event_type" binding:"required"`
	Channel   string `json:"channel" binding:"required"`
	Enabled   bool   `json:"enabled"`
}

// validEventTypes are the event types the notification system supports.
var validEventTypes = map[string]bool{
	"motion":            true,
	"camera_offline":    true,
	"camera_online":     true,
	"recording_started": true,
	"recording_stopped": true,
	"disk_warning":      true,
	"disk_critical":     true,
}

// validChannels are the supported notification delivery channels.
var validChannels = map[string]bool{
	"email": true,
	"sms":   true,
	"push":  true,
	"slack": true,
	"teams": true,
}

// UpdatePreferences batch-upserts notification preferences for the authenticated user.
//
//	PUT /api/nvr/notification-preferences
func (h *NotificationPrefsHandler) UpdatePreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	if uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req bulkPrefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dbPrefs []*db.NotificationPreference
	for _, p := range req.Preferences {
		if !validEventTypes[p.EventType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid event_type: %s", p.EventType)})
			return
		}
		if !validChannels[p.Channel] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid channel: %s", p.Channel)})
			return
		}
		cameraID := p.CameraID
		if cameraID == "" {
			cameraID = "*"
		}
		dbPrefs = append(dbPrefs, &db.NotificationPreference{
			UserID:    uid,
			CameraID:  cameraID,
			EventType: p.EventType,
			Channel:   p.Channel,
			Enabled:   p.Enabled,
		})
	}

	if err := h.DB.BulkUpsertNotificationPreferences(dbPrefs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update notification preferences", err)
		return
	}

	nvrLogInfo("notifications", fmt.Sprintf("user %s updated %d notification preferences", uid, len(dbPrefs)))

	// Return updated list.
	prefs, err := h.DB.ListNotificationPreferences(uid)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list notification preferences", err)
		return
	}
	c.JSON(http.StatusOK, prefs)
}

// --- Quiet Hours ---

// GetQuietHours returns the quiet-hours configuration for the authenticated user.
//
//	GET /api/nvr/notification-preferences/quiet-hours
func (h *NotificationPrefsHandler) GetQuietHours(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	if uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	qh, err := h.DB.GetQuietHours(uid)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get quiet hours", err)
		return
	}
	c.JSON(http.StatusOK, qh)
}

type quietHoursRequest struct {
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
	Timezone  string `json:"timezone"`
	Days      string `json:"days"` // JSON array
}

// UpdateQuietHours updates the quiet-hours configuration for the authenticated user.
//
//	PUT /api/nvr/notification-preferences/quiet-hours
func (h *NotificationPrefsHandler) UpdateQuietHours(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	if uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req quietHoursRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	days := req.Days
	if days == "" {
		days = `["mon","tue","wed","thu","fri","sat","sun"]`
	}

	qh := &db.QuietHours{
		UserID:    uid,
		Enabled:   req.Enabled,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Timezone:  tz,
		Days:      days,
	}

	if err := h.DB.UpsertQuietHours(qh); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update quiet hours", err)
		return
	}

	nvrLogInfo("notifications", fmt.Sprintf("user %s updated quiet hours", uid))

	updated, err := h.DB.GetQuietHours(uid)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get quiet hours", err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

// --- Escalation Rules (admin only) ---

// ListEscalationRules returns all escalation rules.
//
//	GET /api/nvr/escalation-rules
func (h *NotificationPrefsHandler) ListEscalationRules(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	rules, err := h.DB.ListEscalationRules()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list escalation rules", err)
		return
	}
	if rules == nil {
		rules = []*db.EscalationRule{}
	}
	c.JSON(http.StatusOK, rules)
}

type escalationRuleRequest struct {
	Name                  string `json:"name" binding:"required"`
	EventType             string `json:"event_type" binding:"required"`
	CameraID              string `json:"camera_id"`
	Enabled               *bool  `json:"enabled"`
	DelayMinutes          int    `json:"delay_minutes"`
	RepeatCount           int    `json:"repeat_count"`
	RepeatIntervalMinutes int    `json:"repeat_interval_minutes"`
	EscalationChain       string `json:"escalation_chain"` // JSON array
}

// CreateEscalationRule creates a new escalation rule.
//
//	POST /api/nvr/escalation-rules
func (h *NotificationPrefsHandler) CreateEscalationRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req escalationRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validEventTypes[req.EventType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid event_type: %s", req.EventType)})
		return
	}

	cameraID := req.CameraID
	if cameraID == "" {
		cameraID = "*"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	delay := req.DelayMinutes
	if delay < 1 {
		delay = 5
	}
	repeatCount := req.RepeatCount
	if repeatCount < 1 {
		repeatCount = 3
	}
	repeatInterval := req.RepeatIntervalMinutes
	if repeatInterval < 1 {
		repeatInterval = 10
	}
	chain := req.EscalationChain
	if chain == "" {
		chain = "[]"
	}

	rule := &db.EscalationRule{
		Name:                  req.Name,
		EventType:             req.EventType,
		CameraID:              cameraID,
		Enabled:               enabled,
		DelayMinutes:          delay,
		RepeatCount:           repeatCount,
		RepeatIntervalMinutes: repeatInterval,
		EscalationChain:       chain,
	}

	if err := h.DB.CreateEscalationRule(rule); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create escalation rule", err)
		return
	}

	nvrLogInfo("notifications", fmt.Sprintf("escalation rule created: %s", rule.Name))
	c.JSON(http.StatusCreated, rule)
}

// UpdateEscalationRule updates an existing escalation rule.
//
//	PUT /api/nvr/escalation-rules/:id
func (h *NotificationPrefsHandler) UpdateEscalationRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req escalationRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validEventTypes[req.EventType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid event_type: %s", req.EventType)})
		return
	}

	cameraID := req.CameraID
	if cameraID == "" {
		cameraID = "*"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	chain := req.EscalationChain
	if chain == "" {
		chain = "[]"
	}

	rule := &db.EscalationRule{
		ID:                    id,
		Name:                  req.Name,
		EventType:             req.EventType,
		CameraID:              cameraID,
		Enabled:               enabled,
		DelayMinutes:          req.DelayMinutes,
		RepeatCount:           req.RepeatCount,
		RepeatIntervalMinutes: req.RepeatIntervalMinutes,
		EscalationChain:       chain,
	}

	if err := h.DB.UpdateEscalationRule(rule); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "escalation rule not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update escalation rule", err)
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DeleteEscalationRule removes an escalation rule.
//
//	DELETE /api/nvr/escalation-rules/:id
func (h *NotificationPrefsHandler) DeleteEscalationRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.DB.DeleteEscalationRule(id); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "escalation rule not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete escalation rule", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "escalation rule deleted"})
}
