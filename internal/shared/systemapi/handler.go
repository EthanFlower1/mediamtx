// Package systemapi implements the shared system-level API endpoints usable
// by both the Directory and Recorder services. Routes include health checks,
// system info, version, and logging configuration.
//
// This is a role-scoped extract of the monolithic internal/nvr/api layer
// (system.go, health.go portions, log_config.go, updates.go).
package systemapi

import (
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HealthChecker is implemented by any subsystem that can report its health.
type HealthChecker interface {
	// Healthy returns true if the subsystem is operating normally.
	Healthy() bool
	// Name returns the subsystem name for the health response.
	Name() string
}

// Handler is the system API handler shared by Directory and Recorder.
type Handler struct {
	version   string
	startedAt time.Time
	checkers  []HealthChecker
}

// NewHandler creates a Handler for the given service version.
// Optional HealthCheckers are probed by the health endpoint.
func NewHandler(version string, checkers ...HealthChecker) *Handler {
	return &Handler{
		version:   version,
		startedAt: time.Now(),
		checkers:  checkers,
	}
}

// Register wires system routes onto r.
// Public routes (health, version) should be called on an unauthenticated group;
// protected routes (info, metrics) on a JWT-protected group.
func (h *Handler) Register(r gin.IRouter) {
	// These are typically called on an unauthenticated group.
	r.GET("/system/health", h.Health)
	r.GET("/system/version", h.Version)

	// These typically require authentication.
	r.GET("/system/info", h.Info)
	r.GET("/system/metrics", h.Metrics)
}

// --- helpers ---------------------------------------------------------------

func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[systemapi] [ERROR] [%s] %s: %v", reqID, userMsg, err)
	c.JSON(status, gin.H{"error": userMsg, "request_id": reqID})
}

// --- Handlers --------------------------------------------------------------

// Health returns aggregate health across all registered HealthCheckers.
// Responds 200 "healthy" when all checks pass, 503 "unhealthy" otherwise.
//
//	GET /system/health
func (h *Handler) Health(c *gin.Context) {
	start := time.Now()
	components := make(map[string]any, len(h.checkers))
	overall := "healthy"

	for _, chk := range h.checkers {
		ok := chk.Healthy()
		status := "healthy"
		if !ok {
			status = "unhealthy"
			overall = "unhealthy"
		}
		components[chk.Name()] = gin.H{"status": status}
	}

	resp := gin.H{
		"status":      overall,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"duration_ms": float64(time.Since(start).Microseconds()) / 1000.0,
		"components":  components,
	}

	code := http.StatusOK
	if overall != "healthy" {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, resp)
}

// Version returns the service version string.
//
//	GET /system/version
func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": h.version,
	})
}

// Info returns basic system information: version, uptime, Go runtime.
//
//	GET /system/info
func (h *Handler) Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    h.version,
		"started_at": h.startedAt.UTC().Format(time.RFC3339),
		"uptime_sec": int64(time.Since(h.startedAt).Seconds()),
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"num_cpu":    runtime.NumCPU(),
	})
}

// Metrics returns lightweight runtime metrics.
//
//	GET /system/metrics
func (h *Handler) Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, gin.H{
		"goroutines":    runtime.NumGoroutine(),
		"heap_alloc":    m.HeapAlloc,
		"heap_sys":      m.HeapSys,
		"heap_objects":  m.HeapObjects,
		"gc_pause_ns":   m.PauseNs[(m.NumGC+255)%256],
		"num_gc":        m.NumGC,
		"uptime_sec":    int64(time.Since(h.startedAt).Seconds()),
	})
}
