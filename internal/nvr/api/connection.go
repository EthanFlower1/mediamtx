package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/recorder/connmgr"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ConnectionHandler serves camera connection state and history.
type ConnectionHandler struct {
	DB      *db.DB
	ConnMgr *connmgr.Manager
}

// GetState returns the current connection state for a camera.
func (h *ConnectionHandler) GetState(c *gin.Context) {
	cameraID := c.Param("id")
	if h.ConnMgr == nil {
		c.JSON(http.StatusOK, gin.H{"camera_id": cameraID, "state": "unknown"})
		return
	}
	state := h.ConnMgr.GetState(cameraID)
	if state == nil {
		c.JSON(http.StatusOK, gin.H{"camera_id": cameraID, "state": "untracked"})
		return
	}

	resp := gin.H{
		"camera_id":   state.CameraID,
		"state":       state.State,
		"retry_count": state.RetryCount,
	}
	if state.LastError != "" {
		resp["last_error"] = state.LastError
	}
	if state.ConnectedAt != nil {
		resp["connected_at"] = state.ConnectedAt.Format("2006-01-02T15:04:05.000Z")
	}
	if state.LastAttempt != nil {
		resp["last_attempt"] = state.LastAttempt.Format("2006-01-02T15:04:05.000Z")
	}
	c.JSON(http.StatusOK, resp)
}

// GetAllStates returns connection states for all cameras.
func (h *ConnectionHandler) GetAllStates(c *gin.Context) {
	if h.ConnMgr == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}
	states := h.ConnMgr.GetAllStates()
	result := make([]gin.H, 0, len(states))
	for _, s := range states {
		entry := gin.H{
			"camera_id":   s.CameraID,
			"state":       s.State,
			"retry_count": s.RetryCount,
		}
		if s.LastError != "" {
			entry["last_error"] = s.LastError
		}
		if s.ConnectedAt != nil {
			entry["connected_at"] = s.ConnectedAt.Format("2006-01-02T15:04:05.000Z")
		}
		if s.LastAttempt != nil {
			entry["last_attempt"] = s.LastAttempt.Format("2006-01-02T15:04:05.000Z")
		}
		result = append(result, entry)
	}
	c.JSON(http.StatusOK, result)
}

// History returns the connection event log for a camera.
func (h *ConnectionHandler) History(c *gin.Context) {
	cameraID := c.Param("id")
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	events, err := h.DB.ListConnectionEvents(cameraID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if events == nil {
		events = []*db.ConnectionEvent{}
	}
	c.JSON(http.StatusOK, events)
}

// Summary returns aggregate connection statistics for a camera.
func (h *ConnectionHandler) Summary(c *gin.Context) {
	cameraID := c.Param("id")
	summary, err := h.DB.GetConnectionSummary(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// QueuedCommands returns the pending command queue for a camera.
func (h *ConnectionHandler) QueuedCommands(c *gin.Context) {
	cameraID := c.Param("id")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	cmds, err := h.DB.ListQueuedCommands(cameraID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cmds == nil {
		cmds = []*db.QueuedCommand{}
	}
	c.JSON(http.StatusOK, cmds)
}
