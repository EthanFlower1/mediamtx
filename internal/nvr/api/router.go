package api

import (
	"crypto/rsa"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/metrics"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
	nvrui "github.com/bluenviron/mediamtx/internal/nvr/ui"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// AIPipelineRestarter can restart the AI detection pipeline for a camera.
type AIPipelineRestarter interface {
	RestartAIPipeline(cameraID string)
}

// RouterConfig holds the dependencies needed to register NVR API routes.
type RouterConfig struct {
	DB             *db.DB
	PrivateKey     *rsa.PrivateKey
	JWKSJSON       []byte
	YAMLWriter     *yamlwriter.Writer
	Version        string
	Discovery      *onvif.Discovery
	APIAddress     string
	Scheduler      *scheduler.Scheduler
	SetupChecker   SetupChecker
	RecordingsPath string
	Events          *EventBroadcaster
	CallbackManager *onvif.CallbackManager
	EncryptionKey   []byte // AES-256 key for ONVIF credential encryption
	ConfigPath     string // path to mediamtx.yml for reading server configuration
	Embedder        *ai.Embedder        // CLIP embedder for semantic search (may be nil)
	AIRestarter     AIPipelineRestarter // restart AI pipeline on camera settings change (may be nil)
	HLSHandler      *HLSHandler         // HLS VOD playback handler (may be nil)
	StorageManager  *storage.Manager    // storage health and sync manager (may be nil)
	Collector       *metrics.Collector  // ring-buffer metrics collector (may be nil)
}

// RegisterRoutes registers all NVR API routes on the given gin engine.
func RegisterRoutes(engine *gin.Engine, cfg *RouterConfig) {
	audit := &AuditLogger{DB: cfg.DB}

	authHandler := &AuthHandler{
		DB:         cfg.DB,
		PrivateKey: cfg.PrivateKey,
		Audit:      audit,
	}

	cameraHandler := &CameraHandler{
		DB:            cfg.DB,
		YAMLWriter:    cfg.YAMLWriter,
		Discovery:     cfg.Discovery,
		APIAddress:    cfg.APIAddress,
		Scheduler:     cfg.Scheduler,
		Audit:         audit,
		EncryptionKey: cfg.EncryptionKey,
		AIRestarter:   cfg.AIRestarter,
		StorageMgr:    cfg.StorageManager,
	}

	recordingHandler := &RecordingHandler{
		DB: cfg.DB,
	}

	userHandler := &UserHandler{
		DB:    cfg.DB,
		Audit: audit,
	}

	ruleHandler := &RecordingRuleHandler{
		DB:        cfg.DB,
		Scheduler: cfg.Scheduler,
		Audit:     audit,
	}

	systemHandler := &SystemHandler{
		Version:        cfg.Version,
		StartedAt:      time.Now(),
		SetupChecker:   cfg.SetupChecker,
		RecordingsPath: cfg.RecordingsPath,
		DB:             cfg.DB,
		Broadcaster:    cfg.Events,
		ConfigDB:       cfg.DB,
		ConfigPath:     cfg.ConfigPath,
		APIAddress:     cfg.APIAddress,
		Collector:      cfg.Collector,
	}

	savedClipHandler := &SavedClipHandler{
		DB: cfg.DB,
	}

	bookmarkHandler := &BookmarkHandler{
		DB: cfg.DB,
	}

	searchHandler := &SearchHandler{
		DB:       cfg.DB,
		Embedder: cfg.Embedder,
	}

	auditHandler := &AuditHandler{
		DB: cfg.DB,
	}

	streamHandler := &StreamHandler{DB: cfg.DB, APIAddress: cfg.APIAddress}

	screenshotHandler := &ScreenshotHandler{DB: cfg.DB, EncryptionKey: cfg.EncryptionKey}

	templateHandler := &ScheduleTemplateHandler{DB: cfg.DB}

	jwksHandler := &JWKSHandler{
		JWKSJSON: cfg.JWKSJSON,
	}

	middleware := &Middleware{
		PrivateKey: cfg.PrivateKey,
	}

	nvr := engine.Group("/api/nvr")

	// Public routes (no auth required).
	nvr.POST("/auth/login", authHandler.Login)
	nvr.POST("/auth/setup", authHandler.Setup)
	nvr.POST("/auth/refresh", authHandler.Refresh)
	nvr.POST("/auth/revoke", authHandler.Revoke)
	nvr.GET("/.well-known/jwks.json", jwksHandler.ServeJWKS)
	nvr.GET("/system/health", systemHandler.Health)

	// Serve event thumbnails as static files (public route for img tags).
	engine.Static("/thumbnails", "./thumbnails")
	engine.Static("/screenshots", "./screenshots")

	// ONVIF callback (no auth — camera POSTs notifications here).
	if cfg.CallbackManager != nil {
		nvr.POST("/onvif-callback/:cameraId", func(c *gin.Context) {
			cameraID := c.Param("cameraId")
			body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
			if err != nil {
				c.Status(http.StatusBadRequest)
				return
			}
			cfg.CallbackManager.HandleCallback(cameraID, body)
			c.Status(http.StatusOK)
		})
	}

	// Protected routes (JWT auth required).
	protected := nvr.Group("", middleware.Handler())

	// Cameras.
	protected.GET("/cameras", cameraHandler.List)
	protected.POST("/cameras", cameraHandler.Create)
	protected.GET("/cameras/:id", cameraHandler.Get)
	protected.PUT("/cameras/:id", cameraHandler.Update)
	protected.DELETE("/cameras/:id", cameraHandler.Delete)

	// Camera discovery and refresh.
	protected.POST("/cameras/discover", cameraHandler.Discover)
	protected.GET("/cameras/discover/status", cameraHandler.DiscoverStatus)
	protected.GET("/cameras/discover/results", cameraHandler.DiscoverResults)
	protected.POST("/cameras/probe", cameraHandler.Probe)
	protected.POST("/cameras/:id/refresh", cameraHandler.RefreshCapabilities)

	// Camera PTZ & settings.
	protected.POST("/cameras/:id/ptz", cameraHandler.PTZCommand)
	protected.GET("/cameras/:id/ptz/presets", cameraHandler.PTZPresets)
	protected.GET("/cameras/:id/ptz/capabilities", cameraHandler.PTZCapabilities)
	protected.GET("/cameras/:id/settings", cameraHandler.GetSettings)
	protected.PUT("/cameras/:id/settings", cameraHandler.UpdateSettings)
	protected.PUT("/cameras/:id/retention", cameraHandler.UpdateRetention)
	protected.PUT("/cameras/:id/motion-timeout", cameraHandler.UpdateMotionTimeout)

	// Relay outputs.
	protected.GET("/cameras/:id/relay-outputs", cameraHandler.GetRelayOutputs)
	protected.POST("/cameras/:id/relay-outputs/:token/state", cameraHandler.SetRelayOutputState)

	// Audio capabilities.
	protected.GET("/cameras/:id/audio/capabilities", cameraHandler.AudioCapabilities)

	// Edge recordings (camera SD card / Profile G).
	protected.GET("/cameras/:id/edge-recordings", cameraHandler.EdgeRecordings)
	protected.GET("/cameras/:id/edge-recordings/playback", cameraHandler.EdgePlayback)
	protected.POST("/cameras/:id/edge-recordings/import", cameraHandler.EdgeImport)

	// Camera AI configuration.
	protected.PUT("/cameras/:id/ai", cameraHandler.UpdateAIConfig)
	protected.PUT("/cameras/:id/audio-transcode", cameraHandler.UpdateAudioTranscode)

	// Real-time detections for live overlay.
	protected.GET("/cameras/:id/detections/latest", cameraHandler.LatestDetections)
	protected.GET("/cameras/:id/detections/stream", cfg.Events.StreamDetections)

	// Analytics rules and modules.
	protected.GET("/cameras/:id/analytics/rules", cameraHandler.GetAnalyticsRules)
	protected.POST("/cameras/:id/analytics/rules", cameraHandler.CreateAnalyticsRule)
	protected.PUT("/cameras/:id/analytics/rules/:name", cameraHandler.UpdateAnalyticsRule)
	protected.DELETE("/cameras/:id/analytics/rules/:name", cameraHandler.DeleteAnalyticsRule)
	protected.GET("/cameras/:id/analytics/modules", cameraHandler.GetAnalyticsModules)

	// Recordings.
	protected.GET("/recordings", recordingHandler.Query)
	protected.GET("/recordings/:id/download", recordingHandler.Download)
	protected.POST("/recordings/export", recordingHandler.Export)
	protected.DELETE("/recordings/cleanup", recordingHandler.Cleanup)
	protected.GET("/timeline", recordingHandler.Timeline)
	protected.GET("/timeline/intensity", recordingHandler.Intensity)

	// Motion events.
	protected.GET("/cameras/:id/motion-events", recordingHandler.MotionEvents)

	// Saved clips.
	protected.GET("/saved-clips", savedClipHandler.List)
	protected.POST("/saved-clips", savedClipHandler.Create)
	protected.DELETE("/saved-clips/:id", savedClipHandler.Delete)

	// Bookmarks.
	protected.GET("/bookmarks", bookmarkHandler.List)
	protected.POST("/bookmarks", bookmarkHandler.Create)
	protected.PUT("/bookmarks/:id", bookmarkHandler.Update)
	protected.DELETE("/bookmarks/:id", bookmarkHandler.Delete)

	// Screenshots.
	protected.POST("/cameras/:id/screenshot", screenshotHandler.Capture)
	protected.GET("/screenshots", screenshotHandler.List)
	protected.GET("/screenshots/:id/download", screenshotHandler.Download)
	protected.DELETE("/screenshots/:id", screenshotHandler.Delete)

	// Camera streams.
	protected.GET("/cameras/:id/streams", streamHandler.List)
	protected.POST("/cameras/:id/streams", streamHandler.Create)
	protected.PUT("/streams/:id", streamHandler.Update)
	protected.PUT("/streams/:id/roles", streamHandler.UpdateRoles)
	protected.DELETE("/streams/:id", streamHandler.Delete)

	// Recording rules.
	protected.GET("/cameras/:id/recording-rules", ruleHandler.List)
	protected.POST("/cameras/:id/recording-rules", ruleHandler.Create)
	protected.PUT("/recording-rules/:id", ruleHandler.Update)
	protected.DELETE("/recording-rules/:id", ruleHandler.Delete)
	protected.GET("/cameras/:id/recording-status", ruleHandler.Status)

	// Schedule templates.
	protected.GET("/schedule-templates", templateHandler.List)
	protected.POST("/schedule-templates", templateHandler.Create)
	protected.PUT("/schedule-templates/:id", templateHandler.Update)
	protected.DELETE("/schedule-templates/:id", templateHandler.Delete)

	// Stream schedule assignment.
	protected.PUT("/cameras/:id/stream-schedule", cameraHandler.AssignStreamSchedule)

	// Auth (protected).
	protected.PUT("/auth/password", userHandler.ChangePassword)

	// Users.
	protected.GET("/users", userHandler.List)
	protected.POST("/users", userHandler.Create)
	protected.GET("/users/:id", userHandler.Get)
	protected.PUT("/users/:id", userHandler.Update)
	protected.DELETE("/users/:id", userHandler.Delete)

	// System.
	protected.GET("/system/info", systemHandler.Info)
	protected.GET("/system/storage", systemHandler.Storage)
	protected.GET("/system/metrics", systemHandler.Metrics)
	protected.GET("/system/config", systemHandler.ConfigSummary)
	protected.GET("/system/config/export", systemHandler.ExportConfigAdmin)
	protected.POST("/system/config/import", systemHandler.ImportConfigAdmin)

	// HLS VoD playback.
	if cfg.HLSHandler != nil {
		protected.GET("/vod/:cameraId/playlist.m3u8", cfg.HLSHandler.ServePlaylist)
		protected.GET("/vod/thumbnail", cfg.HLSHandler.ServeThumbnail)
		// Segment serving is public (token is in URL from playlist).
		nvr.GET("/vod/segments/:id", cfg.HLSHandler.ServeSegment)
	}

	// Storage health and sync.
	if cfg.StorageManager != nil {
		storageHandler := &StorageHandler{DB: cfg.DB, Manager: cfg.StorageManager}
		protected.GET("/storage/status", storageHandler.Status)
		protected.GET("/storage/pending", storageHandler.Pending)
		protected.POST("/storage/sync/:camera_id", storageHandler.TriggerSync)
	}

	// AI semantic search.
	protected.GET("/search", searchHandler.Search)
	protected.POST("/search/backfill", searchHandler.Backfill)

	// Audit log (admin only).
	protected.GET("/audit", auditHandler.List)

	// Camera Groups.
	groupHandler := &GroupHandler{DB: cfg.DB, Audit: audit}
	protected.GET("/camera-groups", groupHandler.List)
	protected.POST("/camera-groups", groupHandler.Create)
	protected.GET("/camera-groups/:id", groupHandler.Get)
	protected.PUT("/camera-groups/:id", groupHandler.Update)
	protected.DELETE("/camera-groups/:id", groupHandler.Delete)

	// Tours.
	tourHandler := &TourHandler{DB: cfg.DB, Audit: audit}
	protected.GET("/tours", tourHandler.List)
	protected.POST("/tours", tourHandler.Create)
	protected.GET("/tours/:id", tourHandler.Get)
	protected.PUT("/tours/:id", tourHandler.Update)
	protected.DELETE("/tours/:id", tourHandler.Delete)

	// Serve embedded React UI.
	distFS, err := fs.Sub(nvrui.DistFS, "dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))
		engine.NoRoute(func(c *gin.Context) {
			// Try to serve static file first.
			path := c.Request.URL.Path
			if len(path) > 0 && path[0] == '/' {
				path = path[1:]
			}
			f, err := distFS.Open(path)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			// Fallback to index.html for client-side routing.
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}
}
