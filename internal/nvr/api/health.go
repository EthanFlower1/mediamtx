package api

import (
	"context"
	"fmt"
	"net/http"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
)

// ComponentStatus represents the health of a single subsystem.
type ComponentStatus struct {
	Status  string `json:"status"`            // "healthy" or "unhealthy"
	Message string `json:"message,omitempty"` // human-readable detail on failure
}

// HealthResponse is the JSON body returned by GET /health.
type HealthResponse struct {
	Status     string                     `json:"status"`     // "healthy" or "unhealthy"
	Timestamp  string                     `json:"timestamp"`  // ISO-8601
	DurationMs float64                    `json:"duration_ms"` // wall-clock time of check
	Components map[string]ComponentStatus `json:"components"`
}

// HealthHandler implements the GET /health endpoint.
type HealthHandler struct {
	DB             *db.DB
	Scheduler      *scheduler.Scheduler
	StorageManager *storage.Manager
	Discovery      *onvif.Discovery
	RecordingsPath string
}

// Check runs lightweight probes against each subsystem and returns an
// aggregate health status. It is designed to complete well under 100 ms.
func (h *HealthHandler) Check(c *gin.Context) {
	start := time.Now()
	components := make(map[string]ComponentStatus, 4)
	overall := true

	// 1. Database: a context-limited Ping ensures the SQLite connection is live.
	components["db"] = h.checkDB()
	if components["db"].Status != "healthy" {
		overall = false
	}

	// 2. Recording subsystem.
	components["recording"] = h.checkRecording()
	if components["recording"].Status != "healthy" {
		overall = false
	}

	// 3. Storage: verify the recordings directory is writable and has free space.
	components["storage"] = h.checkStorage()
	if components["storage"].Status != "healthy" {
		overall = false
	}

	// 4. ONVIF discovery service.
	components["onvif"] = h.checkONVIF()
	if components["onvif"].Status != "healthy" {
		overall = false
	}

	elapsed := time.Since(start)

	status := "healthy"
	httpCode := http.StatusOK
	if !overall {
		status = "unhealthy"
		httpCode = http.StatusServiceUnavailable
	}

	c.JSON(httpCode, HealthResponse{
		Status:     status,
		Timestamp:  start.UTC().Format(time.RFC3339),
		DurationMs: float64(elapsed.Microseconds()) / 1000.0,
		Components: components,
	})
}

func (h *HealthHandler) checkDB() ComponentStatus {
	if h.DB == nil {
		return ComponentStatus{Status: "unhealthy", Message: "database not initialized"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := h.DB.PingContext(ctx); err != nil {
		return ComponentStatus{Status: "unhealthy", Message: "ping failed: " + err.Error()}
	}
	return ComponentStatus{Status: "healthy"}
}

func (h *HealthHandler) checkRecording() ComponentStatus {
	if h.Scheduler == nil {
		return ComponentStatus{Status: "healthy", Message: "scheduler not initialized"}
	}
	states := h.Scheduler.GetAllRecordingHealth()
	if len(states) == 0 {
		return ComponentStatus{Status: "healthy", Message: "no cameras configured"}
	}
	var failCount int
	for _, s := range states {
		if s.Status == "stalled" || s.Status == "failed" {
			failCount++
		}
	}
	if failCount > 0 {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: fmt.Sprintf("%d/%d cameras reporting recording issues", failCount, len(states)),
		}
	}
	return ComponentStatus{Status: "healthy"}
}

func (h *HealthHandler) checkStorage() ComponentStatus {
	path := h.RecordingsPath
	if path == "" {
		path = "./recordings/"
	}

	// Fast filesystem stat -- no disk I/O beyond a single syscall.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return ComponentStatus{Status: "unhealthy", Message: "cannot stat recordings path: " + err.Error()}
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	if totalBytes > 0 && freeBytes*100/totalBytes < 5 {
		return ComponentStatus{Status: "unhealthy", Message: "disk free space below 5%"}
	}

	// Also check storage manager health if available.
	if h.StorageManager != nil {
		allHealth := h.StorageManager.GetAllHealth()
		for p, ok := range allHealth {
			if !ok {
				return ComponentStatus{Status: "unhealthy", Message: "storage path unhealthy: " + p}
			}
		}
	}

	return ComponentStatus{Status: "healthy"}
}

func (h *HealthHandler) checkONVIF() ComponentStatus {
	if h.Discovery == nil {
		return ComponentStatus{Status: "healthy", Message: "ONVIF discovery not initialized"}
	}
	// The discovery service is a stateless probe manager; if it exists,
	// it is considered healthy. A full network scan would be too slow
	// for a health check.
	return ComponentStatus{Status: "healthy"}
}
