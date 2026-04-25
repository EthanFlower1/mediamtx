package managed

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
	"github.com/bluenviron/mediamtx/internal/recorder/scheduler"
	"github.com/bluenviron/mediamtx/internal/recorder/storage"
)

// InternalAPI serves the service-to-service endpoints that the Directory
// calls to query and control this recorder.
type InternalAPI struct {
	DB             *db.DB
	Scheduler      *scheduler.Scheduler
	StorageManager *storage.Manager
	RecordingsPath string
	ServiceToken   string
	RecorderID     string

	server *http.Server
	addr   string
}

// Start binds and begins serving the internal API. Call Shutdown to stop.
func (a *InternalAPI) Start(listenAddr string) error {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Service auth middleware — every request must carry the shared token.
	engine.Use(a.serviceAuth())

	v1 := engine.Group("/internal/v1")

	// Health / info
	v1.GET("/health", a.handleHealth)

	// Recordings queries
	v1.GET("/recordings", a.handleRecordings)
	v1.GET("/timeline", a.handleTimeline)

	// Events queries
	v1.GET("/events", a.handleEvents)

	// Camera status
	v1.GET("/cameras", a.handleCameras)

	// Config push from Directory
	v1.POST("/config/cameras", a.handlePushCameras)
	v1.POST("/config/schedule", a.handlePushSchedule)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("managed api listen on %s: %w", listenAddr, err)
	}
	a.addr = ln.Addr().String()

	a.server = &http.Server{Handler: engine}
	go func() {
		if err := a.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[managed] internal API error: %v", err)
		}
	}()

	log.Printf("[managed] internal API listening on %s", a.addr)
	return nil
}

// Addr returns the resolved listen address (useful when binding to :0).
func (a *InternalAPI) Addr() string {
	return a.addr
}

// Shutdown gracefully stops the internal API server.
func (a *InternalAPI) Shutdown() {
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.server.Shutdown(ctx)
	}
}

// serviceAuth validates the shared service token on every request.
func (a *InternalAPI) serviceAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing service token"})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != a.ServiceToken {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid service token"})
			return
		}
		c.Next()
	}
}

// --- Handlers ---------------------------------------------------------------
// These are thin wrappers that query the existing NVR database and return JSON.
// The Directory fans out these calls across recorders and merges results.

func (a *InternalAPI) handleHealth(c *gin.Context) {
	diskTotal, diskFree := diskStats(a.RecordingsPath)
	cameraCount := 0
	if a.DB != nil {
		if cams, err := a.DB.ListCameras(); err == nil {
			cameraCount = len(cams)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"recorder_id":   a.RecorderID,
		"camera_count":  cameraCount,
		"disk_total_gb": float64(diskTotal) / (1 << 30),
		"disk_free_gb":  float64(diskFree) / (1 << 30),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *InternalAPI) handleRecordings(c *gin.Context) {
	cameraID := c.Query("camera_id")
	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time (use RFC3339)"})
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time (use RFC3339)"})
		return
	}

	recordings, err := a.DB.QueryRecordings(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": recordings})
}

func (a *InternalAPI) handleTimeline(c *gin.Context) {
	cameraID := c.Query("camera_id")
	dateStr := c.Query("date") // YYYY-MM-DD

	if cameraID == "" || dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id and date are required"})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date (use YYYY-MM-DD)"})
		return
	}

	start := date
	end := date.Add(24 * time.Hour)

	blocks, err := a.DB.GetTimeline(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"blocks": blocks})
}

func (a *InternalAPI) handleEvents(c *gin.Context) {
	cameraID := c.Query("camera_id")
	startStr := c.Query("start")
	endStr := c.Query("end")
	eventType := c.Query("type")

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time (use RFC3339)"})
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time (use RFC3339)"})
		return
	}

	var eventTypes []string
	if eventType != "" {
		eventTypes = strings.Split(eventType, ",")
	}

	events, err := a.DB.QueryEvents(cameraID, start, end, eventTypes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events})
}

func (a *InternalAPI) handleCameras(c *gin.Context) {
	cams, err := a.DB.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type cameraInfo struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		RTSPPath string `json:"rtsp_path"`
	}
	items := make([]cameraInfo, 0, len(cams))
	for _, cam := range cams {
		items = append(items, cameraInfo{
			ID:       cam.ID,
			Name:     cam.Name,
			Status:   cam.Status,
			RTSPPath: cam.MediaMTXPath,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (a *InternalAPI) handlePushCameras(c *gin.Context) {
	var payload struct {
		Cameras []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			StreamURL  string `json:"stream_url"`
			RecordMode string `json:"record_mode"` // "always", "events", "off"
		} `json:"cameras"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[managed] received %d camera assignments from Directory", len(payload.Cameras))
	// TODO: reconcile with local camera table and scheduler config.
	// This is the integration point where Directory-pushed config
	// merges with the existing camera and recording infrastructure.

	c.JSON(http.StatusOK, gin.H{"accepted": len(payload.Cameras)})
}

func (a *InternalAPI) handlePushSchedule(c *gin.Context) {
	var payload struct {
		Rules []struct {
			CameraID string `json:"camera_id"`
			Mode     string `json:"mode"` // "always", "events", "off"
			Schedule []struct {
				Days  []string `json:"days"`
				Start string   `json:"start"` // "HH:MM"
				End   string   `json:"end"`   // "HH:MM"
				Mode  string   `json:"mode"`
			} `json:"schedule,omitempty"`
		} `json:"rules"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[managed] received %d schedule rules from Directory", len(payload.Rules))
	// TODO: convert to local recording rules and apply via scheduler.
	// The scheduler already evaluates rules every 30 seconds — these
	// get stored as externally-managed rules in the DB.

	c.JSON(http.StatusOK, gin.H{"accepted": len(payload.Rules)})
}
