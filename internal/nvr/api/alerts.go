package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/alerts"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// AlertHandler implements HTTP endpoints for system alerts and SMTP configuration.
type AlertHandler struct {
	DB          *db.DB
	EmailSender *alerts.EmailSender
}

// --- SMTP Configuration ---

// GetSMTPConfig returns the current SMTP configuration (password masked).
//
//	GET /api/nvr/system/smtp/config (admin only)
func (h *AlertHandler) GetSMTPConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	cfg, err := h.DB.GetSMTPConfig()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get SMTP config", err)
		return
	}

	// Mask the password in the response.
	if cfg.Password != "" {
		cfg.Password = "********"
	}

	c.JSON(http.StatusOK, cfg)
}

// smtpConfigRequest is the JSON body for updating SMTP settings.
type smtpConfigRequest struct {
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromAddr   string `json:"from_address" binding:"required"`
	TLSEnabled *bool  `json:"tls_enabled"`
}

// UpdateSMTPConfig updates the SMTP server configuration.
//
//	POST /api/nvr/system/smtp/config (admin only)
func (h *AlertHandler) UpdateSMTPConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req smtpConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := &db.SMTPConfig{
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		FromAddr: req.FromAddr,
	}
	if req.TLSEnabled != nil {
		cfg.TLSEnabled = *req.TLSEnabled
	} else {
		cfg.TLSEnabled = true // default to TLS enabled
	}

	// If password is the mask string, keep the existing password.
	if req.Password == "********" || req.Password == "" {
		existing, err := h.DB.GetSMTPConfig()
		if err == nil {
			cfg.Password = existing.Password
		}
	}

	if err := h.DB.UpdateSMTPConfig(cfg); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update SMTP config", err)
		return
	}

	nvrLogInfo("alerts", "SMTP configuration updated")

	// Mask password in response.
	if cfg.Password != "" {
		cfg.Password = "********"
	}
	c.JSON(http.StatusOK, cfg)
}

// smtpTestRequest is the JSON body for sending a test email.
type smtpTestRequest struct {
	To string `json:"to" binding:"required"`
}

// TestSMTP sends a test email using the current SMTP configuration.
//
//	POST /api/nvr/system/smtp/test (admin only)
func (h *AlertHandler) TestSMTP(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req smtpTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg, err := h.DB.GetSMTPConfig()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get SMTP config", err)
		return
	}

	if cfg.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SMTP not configured"})
		return
	}

	if h.EmailSender == nil {
		apiError(c, http.StatusInternalServerError, "email sender not available", fmt.Errorf("EmailSender is nil"))
		return
	}

	if err := h.EmailSender.SendTestEmail(cfg, req.To); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "test email sent"})
}

// --- Alert Rules ---

// alertRuleRequest is the JSON body for creating/updating an alert rule.
type alertRuleRequest struct {
	Name            string  `json:"name" binding:"required"`
	RuleType        string  `json:"rule_type" binding:"required"`
	ThresholdValue  float64 `json:"threshold_value" binding:"required"`
	CameraID        string  `json:"camera_id"`
	Enabled         *bool   `json:"enabled"`
	NotifyEmail     *bool   `json:"notify_email"`
	CooldownMinutes int     `json:"cooldown_minutes"`
}

// ListAlertRules returns all alert rules.
//
//	GET /api/nvr/alert-rules (admin only)
func (h *AlertHandler) ListAlertRules(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	rules, err := h.DB.ListAlertRules()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list alert rules", err)
		return
	}
	if rules == nil {
		rules = []*db.AlertRule{}
	}
	c.JSON(http.StatusOK, rules)
}

// CreateAlertRule creates a new alert rule.
//
//	POST /api/nvr/alert-rules (admin only)
func (h *AlertHandler) CreateAlertRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req alertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate rule type.
	switch req.RuleType {
	case "disk_usage", "camera_offline", "recording_gap":
		// valid
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_type; must be disk_usage, camera_offline, or recording_gap"})
		return
	}

	rule := &db.AlertRule{
		Name:            req.Name,
		RuleType:        req.RuleType,
		ThresholdValue:  req.ThresholdValue,
		CameraID:        req.CameraID,
		Enabled:         true,
		NotifyEmail:     true,
		CooldownMinutes: 60,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.NotifyEmail != nil {
		rule.NotifyEmail = *req.NotifyEmail
	}
	if req.CooldownMinutes > 0 {
		rule.CooldownMinutes = req.CooldownMinutes
	}

	if err := h.DB.CreateAlertRule(rule); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create alert rule", err)
		return
	}

	nvrLogInfo("alerts", fmt.Sprintf("alert rule created: %s (%s)", rule.Name, rule.RuleType))
	c.JSON(http.StatusCreated, rule)
}

// UpdateAlertRule updates an existing alert rule.
//
//	PUT /api/nvr/alert-rules/:id (admin only)
func (h *AlertHandler) UpdateAlertRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	var req alertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate rule type.
	switch req.RuleType {
	case "disk_usage", "camera_offline", "recording_gap":
		// valid
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_type"})
		return
	}

	rule := &db.AlertRule{
		ID:              id,
		Name:            req.Name,
		RuleType:        req.RuleType,
		ThresholdValue:  req.ThresholdValue,
		CameraID:        req.CameraID,
		Enabled:         true,
		NotifyEmail:     true,
		CooldownMinutes: 60,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.NotifyEmail != nil {
		rule.NotifyEmail = *req.NotifyEmail
	}
	if req.CooldownMinutes > 0 {
		rule.CooldownMinutes = req.CooldownMinutes
	}

	if err := h.DB.UpdateAlertRule(rule); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "alert rule not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update alert rule", err)
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DeleteAlertRule deletes an alert rule.
//
//	DELETE /api/nvr/alert-rules/:id (admin only)
func (h *AlertHandler) DeleteAlertRule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	if err := h.DB.DeleteAlertRule(id); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "alert rule not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete alert rule", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert rule deleted"})
}

// --- Alerts ---

// ListAlerts returns alert history with optional filtering.
//
//	GET /api/nvr/alerts?acknowledged=true&limit=50
func (h *AlertHandler) ListAlerts(c *gin.Context) {
	var acknowledged *bool
	if ack := c.Query("acknowledged"); ack != "" {
		v := ack == "true"
		acknowledged = &v
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	alertList, err := h.DB.ListAlerts(acknowledged, limit)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list alerts", err)
		return
	}
	if alertList == nil {
		alertList = []*db.Alert{}
	}
	c.JSON(http.StatusOK, alertList)
}

// AcknowledgeAlert marks an alert as acknowledged.
//
//	POST /api/nvr/alerts/:id/acknowledge
func (h *AlertHandler) AcknowledgeAlert(c *gin.Context) {
	id := c.Param("id")

	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = "unknown"
	}

	if err := h.DB.AcknowledgeAlert(id, usernameStr); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "alert not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to acknowledge alert", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert acknowledged"})
}
