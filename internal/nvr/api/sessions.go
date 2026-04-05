package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SessionHandler implements HTTP endpoints for session management.
type SessionHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// SessionTimeoutConfig holds configurable session timeout settings.
type SessionTimeoutConfig struct {
	IdleTimeoutMinutes     int `json:"idle_timeout_minutes"`
	AbsoluteTimeoutMinutes int `json:"absolute_timeout_minutes"`
}

// defaultIdleTimeout is the default idle session timeout (30 minutes).
const defaultIdleTimeout = 30

// defaultAbsoluteTimeout is the default absolute session timeout (7 days = 10080 minutes).
const defaultAbsoluteTimeout = 10080

// List returns all active sessions. Admin sees all; non-admin sees only their own.
//
//	GET /api/nvr/sessions?user_id=...
func (h *SessionHandler) List(c *gin.Context) {
	role, _ := c.Get("role")
	currentUserID, _ := c.Get("user_id")
	uid, _ := currentUserID.(string)

	filterUserID := c.Query("user_id")

	// Non-admins can only see their own sessions.
	if role != "admin" {
		filterUserID = uid
	}

	sessions, err := h.DB.ListActiveSessions(filterUserID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list sessions", err)
		return
	}
	if sessions == nil {
		sessions = []*db.Session{}
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// Revoke force-logs out a specific session by revoking its refresh token.
//
//	DELETE /api/nvr/sessions/:id
func (h *SessionHandler) Revoke(c *gin.Context) {
	sessionID := c.Param("id")

	role, _ := c.Get("role")
	currentUserID, _ := c.Get("user_id")
	uid, _ := currentUserID.(string)

	// Verify the session exists and the user has permission.
	tok, err := h.DB.GetRefreshTokenByID(sessionID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve session", err)
		return
	}

	// Non-admins can only revoke their own sessions.
	if role != "admin" && tok.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required to revoke other users' sessions"})
		return
	}

	if err := h.DB.RevokeRefreshToken(sessionID); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to revoke session", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "session_revoke", "session", sessionID, "Force logout session for user "+tok.UserID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "session revoked"})
}

// RevokeAllForUser force-logs out all sessions for a specific user.
//
//	DELETE /api/nvr/users/:id/sessions
func (h *SessionHandler) RevokeAllForUser(c *gin.Context) {
	targetUserID := c.Param("id")

	role, _ := c.Get("role")
	currentUserID, _ := c.Get("user_id")
	uid, _ := currentUserID.(string)

	// Non-admins can only revoke their own sessions.
	if role != "admin" && targetUserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required to revoke other users' sessions"})
		return
	}

	if err := h.DB.RevokeAllUserTokens(targetUserID); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to revoke sessions", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "session_revoke_all", "user", targetUserID, "Force logout all sessions")
	}

	c.JSON(http.StatusOK, gin.H{"message": "all sessions revoked"})
}

// GetTimeout returns the current session timeout configuration.
//
//	GET /api/nvr/sessions/timeout
func (h *SessionHandler) GetTimeout(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	cfg := h.loadTimeoutConfig()
	c.JSON(http.StatusOK, cfg)
}

// SetTimeout updates the session timeout configuration.
//
//	PUT /api/nvr/sessions/timeout
func (h *SessionHandler) SetTimeout(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req SessionTimeoutConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.IdleTimeoutMinutes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "idle_timeout_minutes must be non-negative"})
		return
	}
	if req.AbsoluteTimeoutMinutes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "absolute_timeout_minutes must be non-negative"})
		return
	}

	data, _ := json.Marshal(req)
	if err := h.DB.SetConfig("session_timeout", string(data)); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save timeout config", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "config", "session_timeout",
			"Set idle="+strconv.Itoa(req.IdleTimeoutMinutes)+"m absolute="+strconv.Itoa(req.AbsoluteTimeoutMinutes)+"m")
	}

	c.JSON(http.StatusOK, req)
}

// loadTimeoutConfig reads timeout settings from the config table, falling back to defaults.
func (h *SessionHandler) loadTimeoutConfig() SessionTimeoutConfig {
	cfg := SessionTimeoutConfig{
		IdleTimeoutMinutes:     defaultIdleTimeout,
		AbsoluteTimeoutMinutes: defaultAbsoluteTimeout,
	}

	val, err := h.DB.GetConfig("session_timeout")
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal([]byte(val), &cfg)
	return cfg
}

// GetIdleTimeout returns the idle timeout as a duration, for use by middleware.
func (h *SessionHandler) GetIdleTimeout() time.Duration {
	cfg := h.loadTimeoutConfig()
	if cfg.IdleTimeoutMinutes <= 0 {
		return 0 // disabled
	}
	return time.Duration(cfg.IdleTimeoutMinutes) * time.Minute
}

// GetAbsoluteTimeout returns the absolute timeout as a duration.
func (h *SessionHandler) GetAbsoluteTimeout() time.Duration {
	cfg := h.loadTimeoutConfig()
	if cfg.AbsoluteTimeoutMinutes <= 0 {
		return 0 // disabled
	}
	return time.Duration(cfg.AbsoluteTimeoutMinutes) * time.Minute
}
