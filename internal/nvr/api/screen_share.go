package api

// KAI-469: Screen-sharing session management API.
//
// Allows integrator staff to create, list, and end screen-share sessions
// with customer recorders. Sessions use WebRTC signalling by default but
// the transport is configurable (WebRTC or Rewind).

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ScreenShareHandler implements HTTP endpoints for screen-sharing sessions.
type ScreenShareHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// ScreenShareSessionRow is the JSON/DB shape for a screen-share session.
type ScreenShareSessionRow struct {
	SessionID       string  `json:"session_id"`
	IntegratorID    string  `json:"integrator_id"`
	CustomerID      string  `json:"customer_id"`
	CustomerName    string  `json:"customer_name"`
	RecorderID      string  `json:"recorder_id"`
	InitiatedBy     string  `json:"initiated_by"`
	Transport       string  `json:"transport"` // "webrtc" | "rewind"
	Status          string  `json:"status"`    // "pending" | "active" | "completed" | "failed" | "cancelled"
	SignallingURL   string  `json:"signalling_url"`
	LinkedTicketID  *string `json:"linked_ticket_id"`
	StartedAt       *string `json:"started_at_iso"`
	EndedAt         *string `json:"ended_at_iso"`
	DurationSeconds int     `json:"duration_seconds"`
	CreatedAt       string  `json:"created_at_iso"`
}

type initiateSessionRequest struct {
	IntegratorID string `json:"integrator_id" binding:"required"`
	CustomerID   string `json:"customer_id"   binding:"required"`
	RecorderID   string `json:"recorder_id"   binding:"required"`
	Transport    string `json:"transport"`
}

// Initiate creates a new screen-share session.
//
//	POST /api/nvr/screen-share/sessions
func (h *ScreenShareHandler) Initiate(c *gin.Context) {
	var req initiateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	transport := strings.ToLower(req.Transport)
	if transport == "" {
		transport = "webrtc"
	}
	if transport != "webrtc" && transport != "rewind" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transport must be 'webrtc' or 'rewind'"})
		return
	}

	userID, _ := c.Get("user_id")
	initiatedBy, _ := userID.(string)

	sessionID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	signallingURL := "wss://signal.kaivue.io/v1/sessions/" + sessionID

	row := ScreenShareSessionRow{
		SessionID:       sessionID,
		IntegratorID:    req.IntegratorID,
		CustomerID:      req.CustomerID,
		CustomerName:    "", // Resolved by DB lookup in production.
		RecorderID:      req.RecorderID,
		InitiatedBy:     initiatedBy,
		Transport:       transport,
		Status:          "pending",
		SignallingURL:   signallingURL,
		LinkedTicketID:  nil,
		StartedAt:       nil,
		EndedAt:         nil,
		DurationSeconds: 0,
		CreatedAt:       now,
	}

	data, _ := json.Marshal(row)
	if err := h.DB.SetConfig("screenshare_session_"+sessionID, string(data)); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create session", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "screen_share_initiate", "screen_share_session", sessionID,
			"Initiated screen share for customer "+req.CustomerID+" via "+transport)
	}

	c.JSON(http.StatusCreated, row)
}

// List returns screen-share sessions for an integrator, optionally filtered by customer.
//
//	GET /api/nvr/screen-share/sessions?integrator_id=...&customer_id=...
func (h *ScreenShareHandler) List(c *gin.Context) {
	integratorID := c.Query("integrator_id")
	customerID := c.Query("customer_id")

	if integratorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "integrator_id is required"})
		return
	}

	// In production this would be a proper DB query. For now we scan config keys.
	sessions := []ScreenShareSessionRow{}
	allConfigs, err := h.DB.ListConfigByPrefix("screenshare_session_")
	if err != nil {
		// If the method doesn't exist yet, return empty.
		c.JSON(http.StatusOK, gin.H{"sessions": sessions, "total": 0})
		return
	}

	for _, val := range allConfigs {
		var row ScreenShareSessionRow
		if err := json.Unmarshal([]byte(val), &row); err != nil {
			continue
		}
		if row.IntegratorID != integratorID {
			continue
		}
		if customerID != "" && row.CustomerID != customerID {
			continue
		}
		sessions = append(sessions, row)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// End terminates an active screen-share session.
//
//	POST /api/nvr/screen-share/sessions/:id/end
func (h *ScreenShareHandler) End(c *gin.Context) {
	sessionID := c.Param("id")

	val, err := h.DB.GetConfig("screenshare_session_" + sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var row ScreenShareSessionRow
	if err := json.Unmarshal([]byte(val), &row); err != nil {
		apiError(c, http.StatusInternalServerError, "corrupt session data", err)
		return
	}

	if row.Status != "pending" && row.Status != "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "session is already " + row.Status})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row.Status = "completed"
	row.EndedAt = &now
	if row.StartedAt != nil {
		if startedAt, err := time.Parse(time.RFC3339, *row.StartedAt); err == nil {
			row.DurationSeconds = int(time.Since(startedAt).Seconds())
		}
	}

	data, _ := json.Marshal(row)
	if err := h.DB.SetConfig("screenshare_session_"+sessionID, string(data)); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to end session", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "screen_share_end", "screen_share_session", sessionID,
			"Ended screen share session")
	}

	c.JSON(http.StatusOK, row)
}

// Get returns a single screen-share session.
//
//	GET /api/nvr/screen-share/sessions/:id
func (h *ScreenShareHandler) Get(c *gin.Context) {
	sessionID := c.Param("id")

	val, err := h.DB.GetConfig("screenshare_session_" + sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var row ScreenShareSessionRow
	if err := json.Unmarshal([]byte(val), &row); err != nil {
		apiError(c, http.StatusInternalServerError, "corrupt session data", err)
		return
	}

	c.JSON(http.StatusOK, row)
}
