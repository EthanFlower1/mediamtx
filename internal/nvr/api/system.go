package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SetupChecker reports whether initial setup is required.
type SetupChecker interface {
	IsSetupRequired() bool
}

// StorageQuerier provides per-camera storage statistics.
type StorageQuerier interface {
	GetStoragePerCamera() ([]db.CameraStorage, error)
}

// SystemHandler implements HTTP endpoints for system information and health.
type SystemHandler struct {
	Version        string
	StartedAt      time.Time
	SetupChecker   SetupChecker
	RecordingsPath string
	DB             StorageQuerier
	Broadcaster    *EventBroadcaster
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

// Storage returns real disk usage stats for the recordings directory and
// per-camera storage breakdowns from the database.
func (h *SystemHandler) Storage(c *gin.Context) {
	recordingsPath := h.RecordingsPath
	if recordingsPath == "" {
		recordingsPath = "./recordings/"
	}

	var totalBytes, freeBytes, usedBytes uint64

	var stat syscall.Statfs_t
	if err := syscall.Statfs(recordingsPath, &stat); err == nil {
		totalBytes = stat.Blocks * uint64(stat.Bsize)
		freeBytes = stat.Bavail * uint64(stat.Bsize)
		usedBytes = totalBytes - freeBytes
	}

	var recordingsBytes int64
	var perCamera []db.CameraStorage

	if h.DB != nil {
		var err error
		perCamera, err = h.DB.GetStoragePerCamera()
		if err != nil {
			perCamera = []db.CameraStorage{}
		}
		for _, cs := range perCamera {
			recordingsBytes += cs.TotalBytes
		}
	}

	if perCamera == nil {
		perCamera = []db.CameraStorage{}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_bytes":      totalBytes,
		"free_bytes":       freeBytes,
		"used_bytes":       usedBytes,
		"recordings_bytes": recordingsBytes,
		"per_camera":       perCamera,
	})
}

// Events is a Server-Sent Events (SSE) endpoint that streams system events.
// It subscribes to the EventBroadcaster and forwards events to the client
// as JSON-encoded SSE messages until the client disconnects.
func (h *SystemHandler) Events(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Send the initial "connected" event.
	c.SSEvent("message", "connected")
	c.Writer.Flush()

	if h.Broadcaster == nil {
		// No broadcaster configured; block until client disconnects.
		<-c.Request.Context().Done()
		return
	}

	ch := h.Broadcaster.Subscribe()
	defer h.Broadcaster.Unsubscribe(ch)

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			c.SSEvent("notification", string(data))
			c.Writer.Flush()
		}
	}
}
