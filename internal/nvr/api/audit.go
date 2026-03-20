package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// AuditLogger provides audit logging capabilities to handlers.
type AuditLogger struct {
	DB *db.DB
}

// logAction records an audit entry from the current request context.
func (a *AuditLogger) logAction(c *gin.Context, action, resourceType, resourceID, details string) {
	if a.DB == nil {
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	uid, _ := userID.(string)
	uname, _ := username.(string)
	if uname == "" {
		uname = "unknown"
	}

	entry := &db.AuditEntry{
		UserID:       uid,
		Username:     uname,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    c.ClientIP(),
	}

	if err := a.DB.InsertAuditEntry(entry); err != nil {
		log.Printf("audit: failed to log action %s on %s/%s: %v", action, resourceType, resourceID, err)
	}
}

// logLoginAttempt records a login attempt (success or failure).
func (a *AuditLogger) logLoginAttempt(c *gin.Context, userID, username string, success bool) {
	if a.DB == nil {
		return
	}

	action := "login"
	details := "Login successful"
	if !success {
		action = "login_failed"
		details = "Login failed for username " + username
	}

	entry := &db.AuditEntry{
		UserID:       userID,
		Username:     username,
		Action:       action,
		ResourceType: "system",
		ResourceID:   "",
		Details:      details,
		IPAddress:    c.ClientIP(),
	}

	if err := a.DB.InsertAuditEntry(entry); err != nil {
		log.Printf("audit: failed to log login attempt: %v", err)
	}
}

// AuditHandler implements the audit log query endpoint.
type AuditHandler struct {
	DB *db.DB
}

// List returns paginated audit log entries. Admin only.
//
//	GET /api/nvr/audit?limit=50&offset=0&user_id=...&action=...
func (h *AuditHandler) List(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	limit := 50
	offset := 0

	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	userID := c.Query("user_id")
	action := c.Query("action")

	entries, total, err := h.DB.QueryAuditLog(limit, offset, userID, action)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query audit log"})
		return
	}

	if entries == nil {
		entries = []*db.AuditEntry{}
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
	})
}
