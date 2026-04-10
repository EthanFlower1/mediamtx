package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
//	GET /api/nvr/audit?limit=50&offset=0&user_id=...&action=...&resource_type=...&q=...&from=YYYY-MM-DD&to=YYYY-MM-DD
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

	params := db.AuditQueryParams{
		Limit:        limit,
		Offset:       offset,
		UserID:       c.Query("user_id"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		Search:       c.Query("q"),
	}

	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			params.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			// Include the entire "to" day.
			params.To = t.Add(24*time.Hour - time.Millisecond)
		}
	}

	entries, total, err := h.DB.QueryAuditLogAdvanced(params)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query audit log", err)
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

// Export streams audit log entries as CSV, JSON, or PDF within a date range.
// Admin only.
//
//	GET /api/nvr/audit/export?format=csv&from=2026-01-01&to=2026-04-01&user_id=...&action=...&resource_type=...&q=...
func (h *AuditHandler) Export(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	format := c.DefaultQuery("format", "json")
	if format != "csv" && format != "json" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be csv or json"})
		return
	}

	fromStr := c.Query("from")
	toStr := c.Query("to")

	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query parameters are required (YYYY-MM-DD)"})
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date, use YYYY-MM-DD"})
		return
	}

	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date, use YYYY-MM-DD"})
		return
	}
	// Include the entire "to" day.
	to = to.Add(24*time.Hour - time.Millisecond)

	userID := c.Query("user_id")
	action := c.Query("action")
	resourceType := c.Query("resource_type")
	search := c.Query("q")

	entries, err := h.DB.QueryAuditLogByDateRange(from, to, userID, action)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query audit log for export", err)
		return
	}
	if entries == nil {
		entries = []*db.AuditEntry{}
	}

	// Apply additional filters that QueryAuditLogByDateRange doesn't support.
	if resourceType != "" || search != "" {
		filtered := make([]*db.AuditEntry, 0, len(entries))
		for _, e := range entries {
			if resourceType != "" && e.ResourceType != resourceType {
				continue
			}
			if search != "" {
				q := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(e.Username), q) &&
					!strings.Contains(strings.ToLower(e.ResourceType), q) &&
					!strings.Contains(strings.ToLower(e.ResourceID), q) &&
					!strings.Contains(strings.ToLower(e.Details), q) &&
					!strings.Contains(strings.ToLower(e.IPAddress), q) {
					continue
				}
			}
			filtered = append(filtered, e)
		}
		entries = filtered
	}

	filename := fmt.Sprintf("audit-log-%s-to-%s", fromStr, toStr)

	switch format {
	case "csv":
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, filename))
		if err := db.WriteAuditCSV(c.Writer, entries); err != nil {
			log.Printf("audit export: CSV write error: %v", err)
		}
	case "json":
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, filename))
		enc := json.NewEncoder(c.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			log.Printf("audit export: JSON write error: %v", err)
		}
	}
}

// GetRetention returns the current audit log retention settings.
// Admin only.
//
//	GET /api/nvr/audit/retention
func (h *AuditHandler) GetRetention(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	generalDays := 90
	securityDays := 365

	if v, err := h.DB.GetConfig("audit_retention_days"); err == nil {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			generalDays = n
		}
	}
	if v, err := h.DB.GetConfig("audit_security_retention_days"); err == nil {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			securityDays = n
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"general_retention_days":  generalDays,
		"security_retention_days": securityDays,
	})
}

// UpdateRetention updates audit log retention settings.
// Admin only.
//
//	PUT /api/nvr/audit/retention
func (h *AuditHandler) UpdateRetention(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req struct {
		GeneralRetentionDays  *int `json:"general_retention_days"`
		SecurityRetentionDays *int `json:"security_retention_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.GeneralRetentionDays != nil {
		if *req.GeneralRetentionDays < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "general_retention_days must be at least 1"})
			return
		}
		if err := h.DB.SetConfig("audit_retention_days", strconv.Itoa(*req.GeneralRetentionDays)); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save retention setting", err)
			return
		}
	}

	if req.SecurityRetentionDays != nil {
		if *req.SecurityRetentionDays < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "security_retention_days must be at least 1"})
			return
		}
		if err := h.DB.SetConfig("audit_security_retention_days", strconv.Itoa(*req.SecurityRetentionDays)); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to save retention setting", err)
			return
		}
	}

	// Return updated settings.
	h.GetRetention(c)
}
