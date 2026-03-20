package api

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupChecker reports whether initial setup is required.
type SetupChecker interface {
	IsSetupRequired() bool
}

// SystemHandler implements HTTP endpoints for system information and health.
type SystemHandler struct {
	Version      string
	StartedAt    time.Time
	SetupChecker SetupChecker
}

// Info returns system version, platform, and uptime information.
func (h *SystemHandler) Info(c *gin.Context) {
	uptime := time.Since(h.StartedAt)

	c.JSON(http.StatusOK, gin.H{
		"version":  h.Version,
		"platform": fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"uptime":   uptime.String(),
	})
}

// Health returns 200 OK when running, or 503 Service Unavailable when setup is required.
func (h *SystemHandler) Health(c *gin.Context) {
	if h.SetupChecker != nil && h.SetupChecker.IsSetupRequired() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "setup_required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Storage is a stub endpoint that returns zeros for storage metrics.
func (h *SystemHandler) Storage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"total_bytes": 0,
		"used_bytes":  0,
		"free_bytes":  0,
	})
}

// Events is a Server-Sent Events (SSE) endpoint that streams system events.
// It sets the appropriate headers, sends a "connected" event, and blocks
// until the client disconnects.
func (h *SystemHandler) Events(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Send the initial "connected" event.
	c.SSEvent("message", "connected")
	c.Writer.Flush()

	// Block until the client disconnects.
	<-c.Request.Context().Done()
}
