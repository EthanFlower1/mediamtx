package api

import (
	"crypto/rsa"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	nvrui "github.com/bluenviron/mediamtx/internal/nvr/ui"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

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
	}

	savedClipHandler := &SavedClipHandler{
		DB: cfg.DB,
	}

	auditHandler := &AuditHandler{
		DB: cfg.DB,
	}

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

	// Camera discovery.
	protected.POST("/cameras/discover", cameraHandler.Discover)
	protected.GET("/cameras/discover/status", cameraHandler.DiscoverStatus)
	protected.GET("/cameras/discover/results", cameraHandler.DiscoverResults)
	protected.POST("/cameras/probe", cameraHandler.Probe)

	// Camera PTZ & settings.
	protected.POST("/cameras/:id/ptz", cameraHandler.PTZCommand)
	protected.GET("/cameras/:id/ptz/presets", cameraHandler.PTZPresets)
	protected.GET("/cameras/:id/ptz/capabilities", cameraHandler.PTZCapabilities)
	protected.GET("/cameras/:id/settings", cameraHandler.GetSettings)
	protected.PUT("/cameras/:id/settings", cameraHandler.UpdateSettings)
	protected.PUT("/cameras/:id/retention", cameraHandler.UpdateRetention)

	// Relay outputs.
	protected.GET("/cameras/:id/relay-outputs", cameraHandler.GetRelayOutputs)
	protected.POST("/cameras/:id/relay-outputs/:token/state", cameraHandler.SetRelayOutputState)

	// Audio capabilities.
	protected.GET("/cameras/:id/audio/capabilities", cameraHandler.AudioCapabilities)

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

	// Motion events.
	protected.GET("/cameras/:id/motion-events", recordingHandler.MotionEvents)

	// Serve event thumbnails.
	protected.GET("/thumbnails/*filepath", func(c *gin.Context) {
		fp := c.Param("filepath")
		thumbDir := "./thumbnails"
		if cfg.RecordingsPath != "" {
			thumbDir = filepath.Join(filepath.Dir(cfg.RecordingsPath), "thumbnails")
		}
		fullPath := filepath.Join(thumbDir, fp)
		// Prevent path traversal.
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(thumbDir)) {
			c.Status(http.StatusForbidden)
			return
		}
		c.File(fullPath)
	})

	// Saved clips.
	protected.GET("/saved-clips", savedClipHandler.List)
	protected.POST("/saved-clips", savedClipHandler.Create)
	protected.DELETE("/saved-clips/:id", savedClipHandler.Delete)

	// Recording rules.
	protected.GET("/cameras/:id/recording-rules", ruleHandler.List)
	protected.POST("/cameras/:id/recording-rules", ruleHandler.Create)
	protected.PUT("/recording-rules/:id", ruleHandler.Update)
	protected.DELETE("/recording-rules/:id", ruleHandler.Delete)
	protected.GET("/cameras/:id/recording-status", ruleHandler.Status)

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

	// Audit log (admin only).
	protected.GET("/audit", auditHandler.List)

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
