package api

import (
	"crypto/rsa"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/backchannel"
	"github.com/bluenviron/mediamtx/internal/nvr/connmgr"
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
	QuarantineBase  string              // quarantine directory for corrupted recordings
	BackchannelMgr  *backchannel.Manager // backchannel audio session manager (may be nil)
	ConnManager     *connmgr.Manager    // camera connection resilience manager (may be nil)
	ExportsPath        string              // directory for exported clip files
	ExportMaxConcurrent int               // max concurrent export jobs (default 2)
}

// RegisterRoutes registers all NVR API routes on the given gin engine.
// It returns the ExportHandler so the caller can call Stop() on shutdown.
func RegisterRoutes(engine *gin.Engine, cfg *RouterConfig) *ExportHandler {
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

	var backchannelHandler *BackchannelHandler
	if cfg.BackchannelMgr != nil {
		backchannelHandler = &BackchannelHandler{
			DB:            cfg.DB,
			Manager:       cfg.BackchannelMgr,
			EncryptionKey: cfg.EncryptionKey,
		}
	}

	recordingHandler := &RecordingHandler{
		DB: cfg.DB,
	}

	quarantineBase := cfg.QuarantineBase
	if quarantineBase == "" {
		quarantineBase = filepath.Join(cfg.RecordingsPath, ".quarantine")
	}
	integrityHandler := &IntegrityHandler{
		DB:             cfg.DB,
		Events:         cfg.Events,
		RecordingsBase: cfg.RecordingsPath,
		QuarantineBase: quarantineBase,
	}

	statsHandler := &StatsHandler{
		DB: cfg.DB,
	}

	var healthHandler *RecordingHealthHandler
	if cfg.Scheduler != nil {
		healthHandler = &RecordingHealthHandler{
			DB:             cfg.DB,
			HealthProvider: cfg.Scheduler,
		}
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
		StorageMgr:     cfg.StorageManager,
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

	edgeSearchHandler := &EdgeSearchHandler{
		DB:            cfg.DB,
		EncryptionKey: cfg.EncryptionKey,
	}

	auditHandler := &AuditHandler{
		DB: cfg.DB,
	}

	bulkExportHandler := &BulkExportHandler{
		DB:             cfg.DB,
		RecordingsPath: cfg.RecordingsPath,
	}

	streamHandler := &StreamHandler{DB: cfg.DB, APIAddress: cfg.APIAddress}

	screenshotHandler := &ScreenshotHandler{DB: cfg.DB, EncryptionKey: cfg.EncryptionKey}

	thumbnailHandler := &ThumbnailHandler{
		DB:             cfg.DB,
		RecordingsPath: cfg.RecordingsPath,
	}

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
	protected.POST("/cameras/multi-channel", cameraHandler.CreateMultiChannel)
	protected.GET("/cameras/:id", cameraHandler.Get)
	protected.PUT("/cameras/:id", cameraHandler.Update)
	protected.DELETE("/cameras/:id", cameraHandler.Delete)

	// Camera discovery and refresh.
	protected.POST("/cameras/discover", cameraHandler.Discover)
	protected.GET("/cameras/discover/status", cameraHandler.DiscoverStatus)
	protected.GET("/cameras/discover/results", cameraHandler.DiscoverResults)
	protected.POST("/cameras/probe", cameraHandler.Probe)
	protected.POST("/cameras/:id/refresh", cameraHandler.RefreshCapabilities)
	protected.POST("/cameras/:id/rotate-credentials", cameraHandler.RotateCredentials)

	// Camera PTZ & settings.
	protected.POST("/cameras/:id/ptz", cameraHandler.PTZCommand)
	protected.GET("/cameras/:id/ptz/presets", cameraHandler.PTZPresets)
	protected.GET("/cameras/:id/ptz/capabilities", cameraHandler.PTZCapabilities)
	protected.GET("/cameras/:id/ptz/status", cameraHandler.PTZStatus)
	protected.GET("/cameras/:id/settings", cameraHandler.GetSettings)
	protected.PUT("/cameras/:id/settings", cameraHandler.UpdateSettings)
	protected.GET("/cameras/:id/settings/options", cameraHandler.GetImagingOptions)
	protected.GET("/cameras/:id/settings/status", cameraHandler.GetImagingStatus)
	protected.GET("/cameras/:id/settings/focus/move-options", cameraHandler.GetFocusMoveOptions)
	protected.POST("/cameras/:id/settings/focus/move", cameraHandler.MoveFocus)
	protected.POST("/cameras/:id/settings/focus/stop", cameraHandler.StopFocus)
	protected.PUT("/cameras/:id/retention", cameraHandler.UpdateRetention)
	protected.GET("/cameras/:id/storage-estimate", cameraHandler.StorageEstimate)
	protected.PUT("/cameras/:id/motion-timeout", cameraHandler.UpdateMotionTimeout)

	// Media configuration.
	protected.GET("/cameras/:id/media/profiles", cameraHandler.GetMediaProfiles)
	protected.POST("/cameras/:id/media/profiles", cameraHandler.CreateMediaProfile)
	protected.DELETE("/cameras/:id/media/profiles/:token", cameraHandler.DeleteMediaProfile)
	protected.GET("/cameras/:id/media/video-sources", cameraHandler.GetVideoSources)
	protected.GET("/cameras/:id/media/video-encoder/:token", cameraHandler.GetVideoEncoder)
	protected.PUT("/cameras/:id/media/video-encoder/:token", cameraHandler.UpdateVideoEncoder)
	protected.GET("/cameras/:id/media/video-encoder/:token/options", cameraHandler.GetVideoEncoderOptions)

	// Multicast streaming.
	protected.GET("/cameras/:id/multicast", cameraHandler.GetMulticast)
	protected.PUT("/cameras/:id/multicast", cameraHandler.UpdateMulticast)

	// Media2 configuration.
	protected.POST("/cameras/:id/media2/profiles", cameraHandler.CreateMedia2Profile)
	protected.DELETE("/cameras/:id/media2/profiles/:token", cameraHandler.DeleteMedia2Profile)
	protected.POST("/cameras/:id/media2/profiles/:token/configurations", cameraHandler.AddMedia2Configuration)
	protected.DELETE("/cameras/:id/media2/profiles/:token/configurations", cameraHandler.RemoveMedia2Configuration)
	protected.GET("/cameras/:id/media2/video-source-configs", cameraHandler.GetVideoSourceConfigs)
	protected.PUT("/cameras/:id/media2/video-source-configs/:token", cameraHandler.SetVideoSourceConfig)
	protected.GET("/cameras/:id/media2/video-source-configs/:token/options", cameraHandler.GetVideoSourceConfigOptions)
	protected.GET("/cameras/:id/media2/audio-source-configs", cameraHandler.GetAudioSourceConfigs)
	protected.PUT("/cameras/:id/media2/audio-source-configs/:token", cameraHandler.SetAudioSourceConfig)

	// Device info and service capabilities.
	protected.GET("/cameras/:id/device-info", cameraHandler.GetDeviceInfo)
	protected.GET("/cameras/:id/services", cameraHandler.GetServices)

	// Device management.
	protected.GET("/cameras/:id/device/datetime", cameraHandler.GetDeviceDateTime)
	protected.PUT("/cameras/:id/device/datetime", cameraHandler.SetDeviceDateTimeHandler)
	protected.GET("/cameras/:id/device/hostname", cameraHandler.GetDeviceHostnameHandler)
	protected.PUT("/cameras/:id/device/hostname", cameraHandler.SetDeviceHostnameHandler)
	protected.POST("/cameras/:id/device/reboot", cameraHandler.RebootDevice)
	protected.GET("/cameras/:id/device/scopes", cameraHandler.GetDeviceScopesHandler)
	protected.PUT("/cameras/:id/device/scopes", cameraHandler.SetDeviceScopesHandler)
	protected.POST("/cameras/:id/device/scopes", cameraHandler.AddDeviceScopesHandler)
	protected.DELETE("/cameras/:id/device/scopes", cameraHandler.RemoveDeviceScopesHandler)
	protected.GET("/cameras/:id/device/discovery-mode", cameraHandler.GetDiscoveryModeHandler)
	protected.PUT("/cameras/:id/device/discovery-mode", cameraHandler.SetDiscoveryModeHandler)
	protected.GET("/cameras/:id/device/system-log", cameraHandler.GetSystemLogHandler)
	protected.GET("/cameras/:id/device/support-info", cameraHandler.GetSystemSupportInfoHandler)
	protected.GET("/cameras/:id/device/network/interfaces", cameraHandler.GetNetworkInterfacesHandler)
	protected.PUT("/cameras/:id/device/network/interfaces/:token", cameraHandler.SetNetworkInterfaceHandler)
	protected.GET("/cameras/:id/device/network/protocols", cameraHandler.GetNetworkProtocolsHandler)
	protected.PUT("/cameras/:id/device/network/protocols", cameraHandler.SetNetworkProtocolsHandler)
	protected.GET("/cameras/:id/device/network/dns", cameraHandler.GetDNSConfigHandler)
	protected.PUT("/cameras/:id/device/network/dns", cameraHandler.SetDNSConfigHandler)
	protected.GET("/cameras/:id/device/network/ntp", cameraHandler.GetNTPConfigHandler)
	protected.PUT("/cameras/:id/device/network/ntp", cameraHandler.SetNTPConfigHandler)
	protected.GET("/cameras/:id/device/network/gateway", cameraHandler.GetNetworkDefaultGatewayHandler)
	protected.PUT("/cameras/:id/device/network/gateway", cameraHandler.SetNetworkDefaultGatewayHandler)
	protected.GET("/cameras/:id/device/users", cameraHandler.GetDeviceUsersHandler)
	protected.POST("/cameras/:id/device/users", cameraHandler.CreateDeviceUserHandler)
	protected.PUT("/cameras/:id/device/users/:username", cameraHandler.UpdateDeviceUserHandler)
	protected.DELETE("/cameras/:id/device/users/:username", cameraHandler.DeleteDeviceUserHandler)

	// Relay outputs.
	protected.GET("/cameras/:id/relay-outputs", cameraHandler.GetRelayOutputs)
	protected.POST("/cameras/:id/relay-outputs/:token/state", cameraHandler.SetRelayOutputState)

	// Audio.
	protected.GET("/cameras/:id/audio/capabilities", cameraHandler.AudioCapabilities)
	protected.GET("/cameras/:id/audio/sources", cameraHandler.AudioSources)
	protected.GET("/cameras/:id/audio/source-configs", cameraHandler.AudioSourceConfigs)
	protected.GET("/cameras/:id/audio/source-configs/compatible/:profileToken", cameraHandler.CompatibleAudioSourceConfigs)
	protected.GET("/cameras/:id/audio/source-configs/:token", cameraHandler.GetAudioSourceConfig)
	protected.PUT("/cameras/:id/audio/source-configs/:token", cameraHandler.UpdateAudioSourceConfig)
	protected.GET("/cameras/:id/audio/source-configs/:token/options", cameraHandler.AudioSourceConfigOptions)
	protected.POST("/cameras/:id/audio/source-configs/add", cameraHandler.AddAudioSourceToProfile)
	protected.POST("/cameras/:id/audio/source-configs/remove", cameraHandler.RemoveAudioSourceFromProfile)

	// Backchannel audio.
	if backchannelHandler != nil {
		protected.GET("/cameras/:id/audio/backchannel/ws", backchannelHandler.WebSocket)
		protected.GET("/cameras/:id/audio/backchannel/info", backchannelHandler.Info)
		protected.GET("/cameras/:id/audio/outputs", backchannelHandler.GetAudioOutputs)
		protected.GET("/cameras/:id/audio/output-configs", backchannelHandler.GetAudioOutputConfigs)
		protected.PUT("/cameras/:id/audio/output-configs/:token", backchannelHandler.UpdateAudioOutputConfig)
		protected.GET("/cameras/:id/audio/decoder-configs", backchannelHandler.GetAudioDecoderConfigs)
		protected.PUT("/cameras/:id/audio/decoder-configs/:token", backchannelHandler.UpdateAudioDecoderConfig)
		protected.GET("/cameras/:id/audio/decoder-options/:token", backchannelHandler.GetAudioDecoderOptions)
	}

	// Edge recordings (camera SD card / Profile G).
	protected.GET("/cameras/:id/edge-recordings", cameraHandler.EdgeRecordings)
	protected.GET("/cameras/:id/edge-recordings/playback", cameraHandler.EdgePlayback)
	protected.POST("/cameras/:id/edge-recordings/replay-session", cameraHandler.EdgeReplaySession)
	protected.POST("/cameras/:id/edge-recordings/import", cameraHandler.EdgeImport)

	// Replay control (Profile G — RTSP playback with Range/Scale/Speed).
	protected.POST("/cameras/:id/replay/session", cameraHandler.StartReplaySession)
	protected.GET("/cameras/:id/replay/uri", cameraHandler.GetReplayURI)
	protected.GET("/cameras/:id/replay/capabilities", cameraHandler.GetReplayCapabilities)

	// Recording control (Profile G — manage recordings and jobs on device).
	protected.GET("/cameras/:id/recording-control/config", cameraHandler.GetRecordingConfig)
	protected.POST("/cameras/:id/recording-control/recordings", cameraHandler.CreateEdgeRecording)
	protected.DELETE("/cameras/:id/recording-control/recordings/:token", cameraHandler.DeleteEdgeRecording)
	protected.POST("/cameras/:id/recording-control/jobs", cameraHandler.CreateEdgeRecordingJob)
	protected.DELETE("/cameras/:id/recording-control/jobs/:token", cameraHandler.DeleteEdgeRecordingJob)
	protected.GET("/cameras/:id/recording-control/jobs/:token/state", cameraHandler.GetEdgeRecordingJobState)

	// Track management (Profile G — manage tracks within recordings on device).
	protected.POST("/cameras/:id/recording-control/recordings/:token/tracks", cameraHandler.CreateEdgeTrack)
	protected.DELETE("/cameras/:id/recording-control/recordings/:token/tracks/:trackToken", cameraHandler.DeleteEdgeTrack)
	protected.GET("/cameras/:id/recording-control/tracks/:trackToken/config", cameraHandler.GetEdgeTrackConfig)

	// Camera AI configuration.
	protected.PUT("/cameras/:id/ai", cameraHandler.UpdateAIConfig)
	protected.PUT("/cameras/:id/audio-transcode", cameraHandler.UpdateAudioTranscode)

	// Real-time detections for live overlay.
	protected.GET("/cameras/:id/detections/latest", cameraHandler.LatestDetections)
	protected.GET("/cameras/:id/detections/stream", cfg.Events.StreamDetections)
	protected.GET("/cameras/:id/detections", cameraHandler.Detections)

	// Analytics rules and modules.
	protected.GET("/cameras/:id/analytics/rules", cameraHandler.GetAnalyticsRules)
	protected.POST("/cameras/:id/analytics/rules", cameraHandler.CreateAnalyticsRule)
	protected.PUT("/cameras/:id/analytics/rules/:name", cameraHandler.UpdateAnalyticsRule)
	protected.DELETE("/cameras/:id/analytics/rules/:name", cameraHandler.DeleteAnalyticsRule)
	protected.GET("/cameras/:id/analytics/modules", cameraHandler.GetAnalyticsModules)

	// Metadata configuration (Profile T).
	protected.GET("/cameras/:id/metadata/configurations", cameraHandler.GetMetadataConfigurations)
	protected.GET("/cameras/:id/metadata/configurations/:token", cameraHandler.GetMetadataConfiguration)
	protected.PUT("/cameras/:id/metadata/configurations/:token", cameraHandler.SetMetadataConfiguration)
	protected.POST("/cameras/:id/metadata/profile", cameraHandler.AddMetadataToProfile)
	protected.DELETE("/cameras/:id/metadata/profile/:profileToken", cameraHandler.RemoveMetadataFromProfile)

	// OSD (On-Screen Display) management.
	protected.GET("/cameras/:id/osd", cameraHandler.GetOSDs)
	protected.GET("/cameras/:id/osd/options", cameraHandler.GetOSDOptions)
	protected.POST("/cameras/:id/osd", cameraHandler.CreateOSD)
	protected.PUT("/cameras/:id/osd/:token", cameraHandler.SetOSD)
	protected.DELETE("/cameras/:id/osd/:token", cameraHandler.DeleteOSD)

	// Recordings.
	protected.GET("/recordings", recordingHandler.Query)
	protected.GET("/recordings/:id/download", recordingHandler.Download)
	protected.POST("/recordings/export", recordingHandler.Export)
	protected.DELETE("/recordings/cleanup", recordingHandler.Cleanup)
	protected.GET("/timeline", recordingHandler.Timeline)
	protected.GET("/timeline/multi", recordingHandler.MultiTimeline)
	protected.GET("/timeline/intensity", recordingHandler.Intensity)

	// Bulk export.
	protected.POST("/exports/bulk", bulkExportHandler.Create)
	protected.GET("/exports/bulk", bulkExportHandler.List)
	protected.GET("/exports/bulk/:id", bulkExportHandler.Status)
	protected.GET("/exports/bulk/:id/download", bulkExportHandler.Download)
	protected.DELETE("/exports/bulk/:id", bulkExportHandler.Delete)

	// Recording integrity.
	protected.GET("/recordings/integrity", recordingHandler.IntegritySummary)
	protected.POST("/recordings/verify", integrityHandler.Verify)
	protected.POST("/recordings/:id/quarantine", integrityHandler.Quarantine)
	protected.POST("/recordings/:id/unquarantine", integrityHandler.Unquarantine)

	// Recording statistics.
	protected.GET("/recordings/stats", statsHandler.GetStats)
	protected.GET("/recordings/stats/:camera_id/gaps", statsHandler.GetGaps)

	// Recording health.
	if healthHandler != nil {
		protected.GET("/recordings/health", healthHandler.List)
	}

	// Motion events.
	protected.GET("/cameras/:id/motion-events", recordingHandler.MotionEvents)
	protected.GET("/cameras/:id/events", recordingHandler.Events)
	protected.DELETE("/cameras/:id/events", cameraHandler.PurgeEvents)

	// Saved clips.
	protected.GET("/saved-clips", savedClipHandler.List)
	protected.POST("/saved-clips", savedClipHandler.Create)
	protected.DELETE("/saved-clips/:id", savedClipHandler.Delete)

	// Bookmarks.
	protected.GET("/bookmarks", bookmarkHandler.List)
	protected.GET("/bookmarks/search", bookmarkHandler.Search)
	protected.GET("/bookmarks/mine", bookmarkHandler.Mine)
	protected.GET("/bookmarks/:id", bookmarkHandler.Get)
	protected.POST("/bookmarks", bookmarkHandler.Create)
	protected.PUT("/bookmarks/:id", bookmarkHandler.Update)
	protected.DELETE("/bookmarks/:id", bookmarkHandler.Delete)

	// Screenshots.
	protected.POST("/cameras/:id/screenshot", screenshotHandler.Capture)
	protected.GET("/screenshots", screenshotHandler.List)
	protected.GET("/screenshots/:id/download", screenshotHandler.Download)
	protected.DELETE("/screenshots/:id", screenshotHandler.Delete)

	// Timeline thumbnails.
	protected.GET("/cameras/:id/thumbnails", thumbnailHandler.List)
	protected.GET("/cameras/:id/thumbnails/:filename", thumbnailHandler.Serve)

	// Camera streams.
	protected.GET("/cameras/:id/streams", streamHandler.List)
	protected.POST("/cameras/:id/streams", streamHandler.Create)
	protected.PUT("/streams/:id", streamHandler.Update)
	protected.PUT("/streams/:id/roles", streamHandler.UpdateRoles)
	protected.DELETE("/streams/:id", streamHandler.Delete)
	protected.PUT("/streams/:id/retention", streamHandler.UpdateRetention)
	protected.GET("/cameras/:id/stream-storage", streamHandler.GetStreamStorage)

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
	protected.GET("/system/disk-io", systemHandler.DiskIO)
	protected.PUT("/system/disk-io/thresholds", systemHandler.UpdateDiskIOThresholds)
	protected.GET("/system/config", systemHandler.ConfigSummary)
	protected.GET("/system/config/export", systemHandler.ExportConfigAdmin)
	protected.POST("/system/config/import", systemHandler.ImportConfigAdmin)

	// Branding customization.
	protected.GET("/system/branding", brandingHandler.Get)
	protected.PUT("/system/branding", brandingHandler.Update)
	protected.POST("/system/branding/logo", brandingHandler.UploadLogo)
	protected.DELETE("/system/branding/logo", brandingHandler.DeleteLogo)

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

	// Storage quotas.
	quotaHandler := &QuotaHandler{DB: cfg.DB}
	protected.GET("/quotas", quotaHandler.ListQuotas)
	protected.PUT("/quotas/global", quotaHandler.SetGlobalQuota)
	protected.GET("/quotas/status", quotaHandler.QuotaStatus)
	protected.PUT("/cameras/:id/quota", quotaHandler.SetCameraQuota)

	// AI semantic search.
	protected.GET("/search", searchHandler.Search)
	protected.POST("/search/backfill", searchHandler.Backfill)

	// Evidence exports.
	evidenceHandler := &EvidenceHandler{
		DB:             cfg.DB,
		Audit:          audit,
		RecordingsPath: cfg.RecordingsPath,
	}
	protected.POST("/exports/evidence", evidenceHandler.Create)
	protected.GET("/exports/evidence", evidenceHandler.List)
	protected.GET("/exports/evidence/:id/download", evidenceHandler.Download)

	// Edge search (ONVIF Profile G — search recordings and events on device).
	protected.GET("/edge-search/recordings", edgeSearchHandler.Recordings)
	protected.GET("/edge-search/events", edgeSearchHandler.Events)

	// Audit log (admin only).
	protected.GET("/audit", auditHandler.List)

	// Camera Groups.
	groupHandler := &GroupHandler{DB: cfg.DB, Audit: audit}
	protected.GET("/camera-groups", groupHandler.List)
	protected.POST("/camera-groups", groupHandler.Create)
	protected.GET("/camera-groups/:id", groupHandler.Get)
	protected.PUT("/camera-groups/:id", groupHandler.Update)
	protected.DELETE("/camera-groups/:id", groupHandler.Delete)

	// Devices.
	deviceHandler := &DeviceHandler{
		DB:         cfg.DB,
		YAMLWriter: cfg.YAMLWriter,
		Scheduler:  cfg.Scheduler,
	}
	protected.GET("/devices", deviceHandler.List)
	protected.GET("/devices/:id", deviceHandler.Get)
	protected.DELETE("/devices/:id", deviceHandler.Delete)

	// Tours.
	tourHandler := &TourHandler{DB: cfg.DB, Audit: audit}
	protected.GET("/tours", tourHandler.List)
	protected.POST("/tours", tourHandler.Create)
	protected.GET("/tours/:id", tourHandler.Get)
	protected.PUT("/tours/:id", tourHandler.Update)
	protected.DELETE("/tours/:id", tourHandler.Delete)

	// Camera connection resilience.
	// Export jobs.
	exportsPath := cfg.ExportsPath
	if exportsPath == "" {
		exportsPath = filepath.Join(cfg.RecordingsPath, "exports")
	}
	exportHandler := &ExportHandler{
		DB:             cfg.DB,
		RecordingsPath: cfg.RecordingsPath,
		ExportsPath:    exportsPath,
	}
	maxConcurrent := cfg.ExportMaxConcurrent
	if maxConcurrent < 1 {
		maxConcurrent = 2
	}
	exportHandler.Start(maxConcurrent)
	protected.POST("/exports", exportHandler.Create)
	protected.GET("/exports", exportHandler.List)
	protected.GET("/exports/:id", exportHandler.Get)
	protected.DELETE("/exports/:id", exportHandler.Delete)
	protected.GET("/exports/:id/download", exportHandler.Download)

	brandingHandler := &BrandingHandler{DB: cfg.DB}

	connHandler := &ConnectionHandler{DB: cfg.DB, ConnMgr: cfg.ConnManager}
	protected.GET("/cameras/:id/connection", connHandler.GetState)
	protected.GET("/cameras/:id/connection/history", connHandler.History)
	protected.GET("/cameras/:id/connection/summary", connHandler.Summary)
	protected.GET("/cameras/:id/connection/queue", connHandler.QueuedCommands)
	protected.GET("/connections", connHandler.GetAllStates)

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

	return exportHandler
}
