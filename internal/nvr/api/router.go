package api

import (
	"crypto/rsa"
	"io/fs"
	"net/http"
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
	DB           *db.DB
	PrivateKey   *rsa.PrivateKey
	JWKSJSON     []byte
	YAMLWriter   *yamlwriter.Writer
	Version      string
	Discovery    *onvif.Discovery
	Scheduler    *scheduler.Scheduler
	SetupChecker SetupChecker
}

// RegisterRoutes registers all NVR API routes on the given gin engine.
func RegisterRoutes(engine *gin.Engine, cfg *RouterConfig) {
	authHandler := &AuthHandler{
		DB:         cfg.DB,
		PrivateKey: cfg.PrivateKey,
	}

	cameraHandler := &CameraHandler{
		DB:         cfg.DB,
		YAMLWriter: cfg.YAMLWriter,
		Discovery:  cfg.Discovery,
		Scheduler:  cfg.Scheduler,
	}

	recordingHandler := &RecordingHandler{
		DB: cfg.DB,
	}

	userHandler := &UserHandler{
		DB: cfg.DB,
	}

	ruleHandler := &RecordingRuleHandler{
		DB:        cfg.DB,
		Scheduler: cfg.Scheduler,
	}

	systemHandler := &SystemHandler{
		Version:      cfg.Version,
		StartedAt:    time.Now(),
		SetupChecker: cfg.SetupChecker,
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

	// Camera PTZ & settings.
	protected.POST("/cameras/:id/ptz", cameraHandler.PTZCommand)
	protected.GET("/cameras/:id/ptz/presets", cameraHandler.PTZPresets)
	protected.GET("/cameras/:id/settings", cameraHandler.GetSettings)
	protected.PUT("/cameras/:id/settings", cameraHandler.UpdateSettings)

	// Recordings.
	protected.GET("/recordings", recordingHandler.Query)
	protected.GET("/recordings/:id/download", recordingHandler.Download)
	protected.POST("/recordings/export", recordingHandler.Export)
	protected.GET("/timeline", recordingHandler.Timeline)

	// Recording rules.
	protected.GET("/cameras/:id/recording-rules", ruleHandler.List)
	protected.POST("/cameras/:id/recording-rules", ruleHandler.Create)
	protected.PUT("/recording-rules/:id", ruleHandler.Update)
	protected.DELETE("/recording-rules/:id", ruleHandler.Delete)
	protected.GET("/cameras/:id/recording-status", ruleHandler.Status)

	// Users.
	protected.GET("/users", userHandler.List)
	protected.POST("/users", userHandler.Create)
	protected.GET("/users/:id", userHandler.Get)
	protected.PUT("/users/:id", userHandler.Update)
	protected.DELETE("/users/:id", userHandler.Delete)

	// System.
	protected.GET("/system/info", systemHandler.Info)
	protected.GET("/system/storage", systemHandler.Storage)
	protected.GET("/system/events", systemHandler.Events)

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
