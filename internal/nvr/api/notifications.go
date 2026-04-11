package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// NotificationHandler serves the in-product notification center API.
type NotificationHandler struct {
	DB     *db.DB
	Events *EventBroadcaster
}

// notificationResponse is a single notification item returned by the API.
type notificationResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Camera    string    `json:"camera"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	ReadAt    *string   `json:"read_at"`
	Archived  bool      `json:"archived"`
}

// ListNotifications returns notifications with filtering, search, and pagination.
//
//	GET /api/nvr/notifications?camera=&type=&severity=&read=&archived=false&q=&limit=50&offset=0&since=&until=
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = "unknown"
	}

	filter := db.NotificationFilter{
		UserID:   usernameStr,
		Camera:   c.Query("camera"),
		Type:     c.Query("type"),
		Severity: c.Query("severity"),
		Query:    c.Query("q"),
		Archived: false, // default
	}

	if archived := c.Query("archived"); archived == "true" {
		filter.Archived = true
	}

	if read := c.Query("read"); read != "" {
		v := read == "true"
		filter.Read = &v
	}

	if since := c.Query("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = &t
		}
	}

	if until := c.Query("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filter.Until = &t
		}
	}

	filter.Limit = 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			filter.Limit = n
		}
	}

	filter.Offset = 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	notifications, total, err := h.DB.ListNotifications(filter)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list notifications", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"total":         total,
		"limit":         filter.Limit,
		"offset":        filter.Offset,
	})
}

// UnreadCount returns the total number of unread, non-archived notifications.
//
//	GET /api/nvr/notifications/unread-count
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr = "unknown"
	}

	count, err := h.DB.UnreadNotificationCount(usernameStr)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get unread count", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// markReadRequest is the JSON body for marking notifications read/unread.
type markReadRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

// MarkRead marks one or more notifications as read.
//
//	POST /api/nvr/notifications/mark-read
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.MarkNotificationsRead(usernameStr, req.IDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to mark notifications read", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notifications marked as read", "count": len(req.IDs)})
}

// MarkUnread marks one or more notifications as unread.
//
//	POST /api/nvr/notifications/mark-unread
func (h *NotificationHandler) MarkUnread(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.MarkNotificationsUnread(usernameStr, req.IDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to mark notifications unread", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notifications marked as unread", "count": len(req.IDs)})
}

// MarkAllRead marks all non-archived notifications as read for the current user.
//
//	POST /api/nvr/notifications/mark-all-read
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	count, err := h.DB.MarkAllNotificationsRead(usernameStr)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to mark all read", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "all notifications marked as read", "count": count})
}

// archiveRequest is the JSON body for archiving/restoring notifications.
type archiveRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

// Archive moves notifications to the archive.
//
//	POST /api/nvr/notifications/archive
func (h *NotificationHandler) Archive(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	var req archiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.ArchiveNotifications(usernameStr, req.IDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to archive notifications", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notifications archived", "count": len(req.IDs)})
}

// Restore moves notifications out of the archive.
//
//	POST /api/nvr/notifications/restore
func (h *NotificationHandler) Restore(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	var req archiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.RestoreNotifications(usernameStr, req.IDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to restore notifications", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notifications restored", "count": len(req.IDs)})
}

// Delete permanently removes notifications.
//
//	DELETE /api/nvr/notifications
func (h *NotificationHandler) Delete(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	var req archiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.DeleteNotifications(usernameStr, req.IDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete notifications", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notifications deleted", "count": len(req.IDs)})
}

// IngestEvent is called internally when the EventBroadcaster publishes an event.
// It persists the event as a notification in the database for all users.
func (h *NotificationHandler) IngestEvent(event Event) {
	if event.Type == "detection_frame" || event.Type == "connected" {
		return // Skip high-frequency detection frames and connection events
	}

	severity := classifySeverity(event.Type)

	notification := &db.Notification{
		Type:      event.Type,
		Severity:  severity,
		Camera:    event.Camera,
		Message:   event.Message,
		CreatedAt: time.Now().UTC(),
	}

	// Best-effort insert; do not block the event pipeline on DB errors.
	_ = h.DB.InsertNotification(notification)
}

// classifySeverity maps event types to severity levels.
func classifySeverity(eventType string) string {
	switch eventType {
	case "camera_offline", "recording_failed", "recording_stalled":
		return "critical"
	case "motion", "tampering", "intrusion", "line_crossing", "loitering":
		return "warning"
	case "camera_online", "recording_started", "recording_recovered":
		return "info"
	case "recording_stopped":
		return "info"
	case "ai_detection", "object_count":
		return "warning"
	default:
		return "info"
	}
}
