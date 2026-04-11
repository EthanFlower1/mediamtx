package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupNotificationTest(t *testing.T) (*gin.Engine, *NotificationHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	events := NewEventBroadcaster()
	handler := &NotificationHandler{DB: d, Events: events}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("username", "testuser")
		c.Next()
	})
	engine.GET("/notifications", handler.ListNotifications)
	engine.GET("/notifications/unread-count", handler.UnreadCount)
	engine.POST("/notifications/mark-read", handler.MarkRead)
	engine.POST("/notifications/mark-unread", handler.MarkUnread)
	engine.POST("/notifications/mark-all-read", handler.MarkAllRead)
	engine.POST("/notifications/archive", handler.Archive)
	engine.POST("/notifications/restore", handler.Restore)
	engine.DELETE("/notifications", handler.Delete)

	return engine, handler
}

func seedNotification(t *testing.T, d *db.DB, typ, severity, camera, message string) string {
	t.Helper()
	n := &db.Notification{
		Type:     typ,
		Severity: severity,
		Camera:   camera,
		Message:  message,
	}
	err := d.InsertNotification(n)
	require.NoError(t, err)
	return n.ID
}

func TestListNotifications_Empty(t *testing.T) {
	engine, _ := setupNotificationTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["total"])
}

func TestListNotifications_WithData(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion on cam1")
	seedNotification(t, handler.DB, "camera_offline", "critical", "cam2", "Cam2 offline")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(2), resp["total"])
}

func TestListNotifications_FilterBySeverity(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")
	seedNotification(t, handler.DB, "camera_offline", "critical", "cam2", "Offline")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications?severity=critical", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp["total"])
}

func TestListNotifications_Search(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion detected on lobby")
	seedNotification(t, handler.DB, "camera_offline", "critical", "cam2", "Parking camera offline")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications?q=lobby", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp["total"])
}

func TestUnreadCount(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")
	seedNotification(t, handler.DB, "camera_offline", "critical", "cam2", "Offline")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(2), resp["count"])
}

func TestMarkRead(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	id := seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")

	body, _ := json.Marshal(markReadRequest{IDs: []string{id}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notifications/mark-read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify unread count is now 0.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
	engine.ServeHTTP(w2, req2)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["count"])
}

func TestMarkUnread(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	id := seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")

	// Mark read first.
	body, _ := json.Marshal(markReadRequest{IDs: []string{id}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notifications/mark-read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Then mark unread.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notifications/mark-unread", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	// Verify unread count is back to 1.
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
	engine.ServeHTTP(w3, req3)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp["count"])
}

func TestArchiveAndRestore(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	id := seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")

	// Archive.
	body, _ := json.Marshal(archiveRequest{IDs: []string{id}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notifications/archive", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Should not appear in non-archived list.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w2, req2)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["total"])

	// Should appear in archived list.
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notifications?archived=true", nil)
	engine.ServeHTTP(w3, req3)
	var resp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp2))
	require.Equal(t, float64(1), resp2["total"])

	// Restore.
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost, "/notifications/restore", bytes.NewReader(body))
	req4.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w4, req4)
	require.Equal(t, http.StatusOK, w4.Code)

	// Should be back in non-archived list.
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w5, req5)
	var resp3 map[string]interface{}
	require.NoError(t, json.Unmarshal(w5.Body.Bytes(), &resp3))
	require.Equal(t, float64(1), resp3["total"])
}

func TestMarkAllRead(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion 1")
	seedNotification(t, handler.DB, "motion", "warning", "cam2", "Motion 2")
	seedNotification(t, handler.DB, "camera_offline", "critical", "cam3", "Offline")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notifications/mark-all-read", nil)
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
	engine.ServeHTTP(w2, req2)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["count"])
}

func TestDeleteNotifications(t *testing.T) {
	engine, handler := setupNotificationTest(t)

	id := seedNotification(t, handler.DB, "motion", "warning", "cam1", "Motion")

	body, _ := json.Marshal(archiveRequest{IDs: []string{id}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Should be gone.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w2, req2)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["total"])
}

func TestIngestEvent_SkipsDetectionFrame(t *testing.T) {
	_, handler := setupNotificationTest(t)

	handler.IngestEvent(Event{
		Type:    "detection_frame",
		Camera:  "cam1",
		Message: "frame data",
	})

	// Should not have created a notification.
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("username", "testuser")
		c.Next()
	})
	engine.GET("/notifications", handler.ListNotifications)

	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["total"])
}

func TestIngestEvent_PersistsMotion(t *testing.T) {
	_, handler := setupNotificationTest(t)

	handler.IngestEvent(Event{
		Type:    "motion",
		Camera:  "cam1",
		Message: "Motion detected on cam1",
	})

	w := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("username", "testuser")
		c.Next()
	})
	engine.GET("/notifications", handler.ListNotifications)

	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	engine.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp["total"])
}

func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		eventType string
		expected  string
	}{
		{"camera_offline", "critical"},
		{"recording_failed", "critical"},
		{"recording_stalled", "critical"},
		{"motion", "warning"},
		{"tampering", "warning"},
		{"intrusion", "warning"},
		{"line_crossing", "warning"},
		{"loitering", "warning"},
		{"ai_detection", "warning"},
		{"object_count", "warning"},
		{"camera_online", "info"},
		{"recording_started", "info"},
		{"recording_recovered", "info"},
		{"recording_stopped", "info"},
		{"unknown_event", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			require.Equal(t, tt.expected, classifySeverity(tt.eventType))
		})
	}
}
