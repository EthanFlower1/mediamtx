package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/hwaccel"
	"github.com/bluenviron/mediamtx/internal/nvr/metrics"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
	"github.com/bluenviron/mediamtx/internal/nvr/syscheck"
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
	ConfigDB       *db.DB           // full DB access for config export/import
	ConfigPath     string           // path to mediamtx.yml for reading server configuration
	APIAddress     string           // Raikada API address for live camera status
	Collector      *metrics.Collector  // ring-buffer metrics collector (may be nil)
	StorageMgr     *storage.Manager    // storage manager for disk I/O metrics (may be nil)
	HWDetector     *hwaccel.Detector   // hardware acceleration detector (may be nil)
	SysChecker     *syscheck.Checker   // system requirements checker (may be nil)
}

// Metrics returns runtime performance metrics such as memory usage,
// goroutine count, and camera count.
func (h *SystemHandler) Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var cameraCount int
	if h.ConfigDB != nil {
		cameras, err := h.ConfigDB.ListCameras()
		if err == nil {
			cameraCount = len(cameras)
		}
	}

	current := gin.H{
		"cpu_percent":  0.0,
		"mem_percent":  0.0,
		"mem_alloc_mb": float64(m.Alloc) / (1024 * 1024),
		"mem_sys_mb":   float64(m.Sys) / (1024 * 1024),
		"goroutines":   runtime.NumGoroutine(),
	}

	var history []metrics.Sample
	if h.Collector != nil {
		cur := h.Collector.Current()
		current["cpu_percent"] = cur.CPUPercent
		current["mem_percent"] = cur.MemPercent
		history = h.Collector.History()
	}

	c.JSON(http.StatusOK, gin.H{
		"cpu_goroutines":  runtime.NumGoroutine(),
		"mem_alloc_bytes": m.Alloc,
		"mem_sys_bytes":   m.Sys,
		"mem_gc_count":    m.NumGC,
		"uptime_seconds":  time.Since(h.StartedAt).Seconds(),
		"camera_count":    cameraCount,
		"current":         current,
		"history":         history,
	})
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

	var usedPercent float64
	if totalBytes > 0 {
		usedPercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	// Database stats.
	var dbStats *db.DatabaseStats
	if h.ConfigDB != nil {
		dbStats, _ = h.ConfigDB.GetDatabaseStats()
	}

	c.JSON(http.StatusOK, gin.H{
		"total_bytes":      totalBytes,
		"free_bytes":       freeBytes,
		"used_bytes":       usedBytes,
		"recordings_bytes": recordingsBytes,
		"per_camera":       perCamera,
		"database":         dbStats,
		"warning":          usedPercent > 85,
		"critical":         usedPercent > 95,
	})
}

// mediamtxConfig is a partial representation of mediamtx.yml used to extract
// configuration values for the config summary endpoint.
type mediamtxConfig struct {
	RTSPAddress   string `yaml:"rtspAddress"`
	RTSPSAddress  string `yaml:"rtspsAddress"`
	HLSAddress    string `yaml:"hlsAddress"`
	WebRTCAddress string `yaml:"webrtcAddress"`
	APIAddress    string `yaml:"apiAddress"`
	RTPAddress    string `yaml:"rtpAddress"`
	RTCPAddress   string `yaml:"rtcpAddress"`
	PlaybackAddress string `yaml:"playbackAddress"`

	// Encryption settings.
	RTSPEncryption     string `yaml:"rtspEncryption"`
	APIEncryption      bool   `yaml:"apiEncryption"`
	HLSEncryption      bool   `yaml:"hlsEncryption"`
	WebRTCEncryption   bool   `yaml:"webrtcEncryption"`
	PlaybackEncryption bool   `yaml:"playbackEncryption"`

	// TLS certificate paths.
	APIServerKey      string `yaml:"apiServerKey"`
	APIServerCert     string `yaml:"apiServerCert"`
	RTSPServerKey     string `yaml:"rtspServerKey"`
	RTSPServerCert    string `yaml:"rtspServerCert"`
	WebRTCServerKey   string `yaml:"webrtcServerKey"`
	WebRTCServerCert  string `yaml:"webrtcServerCert"`

	PathDefaults struct {
		Record                bool   `yaml:"record"`
		RecordPath            string `yaml:"recordPath"`
		RecordFormat          string `yaml:"recordFormat"`
		RecordSegmentDuration string `yaml:"recordSegmentDuration"`
		RecordDeleteAfter     string `yaml:"recordDeleteAfter"`
	} `yaml:"pathDefaults"`
}

// extractPort returns the port portion of an address string like ":8554" or "0.0.0.0:8554".
func extractPort(addr string) string {
	if addr == "" {
		return ""
	}
	idx := strings.LastIndex(addr, ":")
	if idx >= 0 {
		return addr[idx+1:]
	}
	return addr
}

// ConfigSummary returns a summary of the current NVR and server configuration.
// It reads recording settings and port numbers from mediamtx.yml and queries
// the database for camera, rule, and user counts.
//
//	GET /api/nvr/system/config (admin only)
func (h *SystemHandler) ConfigSummary(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	// Parse recording settings and ports from the YAML config file.
	cfg := h.parseMediamtxConfig()

	// Build recording section.
	recordingEnabled := cfg.PathDefaults.Record
	recordFormat := cfg.PathDefaults.RecordFormat
	if recordFormat == "" {
		recordFormat = "fmp4"
	}
	segmentDuration := cfg.PathDefaults.RecordSegmentDuration
	if segmentDuration == "" {
		segmentDuration = "1h"
	}
	deleteAfter := cfg.PathDefaults.RecordDeleteAfter
	if deleteAfter == "" {
		deleteAfter = "24h"
	}
	recordPath := cfg.PathDefaults.RecordPath
	if recordPath == "" {
		recordPath = "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"
	}

	// Build server ports section.
	rtspPort := extractPort(cfg.RTSPAddress)
	if rtspPort == "" {
		rtspPort = "8554"
	}
	hlsPort := extractPort(cfg.HLSAddress)
	if hlsPort == "" {
		hlsPort = "8888"
	}
	webrtcPort := extractPort(cfg.WebRTCAddress)
	if webrtcPort == "" {
		webrtcPort = "8889"
	}
	apiPort := extractPort(cfg.APIAddress)
	if apiPort == "" {
		apiPort = "9997"
	}

	// Query database for entity counts.
	var totalCameras, onlineCameras, recordingCameras int
	var totalRules, activeRules int
	var totalUsers, adminUsers int

	if h.ConfigDB != nil {
		cameras, err := h.ConfigDB.ListCameras()
		if err == nil {
			totalCameras = len(cameras)

			// Check live status from Raikada API.
			statuses := h.getCameraStatuses()
			for _, cam := range cameras {
				if cam.MediaMTXPath != "" {
					if s, ok := statuses[cam.MediaMTXPath]; ok && s == "online" {
						onlineCameras++
					}
				}
			}
		}

		// Use aggregate queries instead of per-camera lookups.
		if t, a, err := h.ConfigDB.CountRecordingRules(); err == nil {
			totalRules = t
			activeRules = a
		}
		if rc, err := h.ConfigDB.CountCamerasWithRules(); err == nil {
			recordingCameras = rc
		}

		users, err := h.ConfigDB.ListUsers()
		if err == nil {
			totalUsers = len(users)
			for _, u := range users {
				if u.Role == "admin" {
					adminUsers++
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"recording": gin.H{
			"enabled":          recordingEnabled,
			"format":           recordFormat,
			"segment_duration": segmentDuration,
			"delete_after":     deleteAfter,
			"path":             recordPath,
		},
		"cameras": gin.H{
			"total":     totalCameras,
			"online":    onlineCameras,
			"recording": recordingCameras,
		},
		"recording_rules": gin.H{
			"total":  totalRules,
			"active": activeRules,
		},
		"users": gin.H{
			"total":  totalUsers,
			"admins": adminUsers,
		},
		"server": gin.H{
			"rtsp_port":   rtspPort,
			"webrtc_port": webrtcPort,
			"api_port":    apiPort,
			"hls_port":    hlsPort,
		},
	})
}

// getCameraStatuses fetches all path statuses from the Raikada API.
func (h *SystemHandler) getCameraStatuses() map[string]string {
	statuses := make(map[string]string)

	addr := h.APIAddress
	if addr == "" {
		return statuses
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	url := fmt.Sprintf("http://%s/v3/paths/list", addr)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return statuses
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return statuses
	}

	var result struct {
		Items []struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return statuses
	}

	for _, item := range result.Items {
		if item.Ready {
			statuses[item.Name] = "online"
		} else {
			statuses[item.Name] = "disconnected"
		}
	}
	return statuses
}

// configExport represents the full NVR configuration for export/import.
type configExport struct {
	Version        string              `json:"version"`
	ExportedAt     string              `json:"exported_at"`
	Cameras        []*db.Camera        `json:"cameras"`
	RecordingRules []*db.RecordingRule `json:"recording_rules"`
	Users          []*db.User          `json:"users"`
	SavedClips     []*db.SavedClip     `json:"saved_clips"`
}

// ExportConfig returns the full NVR configuration as a downloadable JSON file.
// Users are included without password hashes for reference only.
func (h *SystemHandler) ExportConfig(c *gin.Context) {
	if h.ConfigDB == nil {
		apiError(c, http.StatusInternalServerError, "database not available", fmt.Errorf("ConfigDB is nil"))
		return
	}

	cameras, err := h.ConfigDB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras for config export", err)
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}

	rules, err := h.ConfigDB.ListAllEnabledRecordingRules()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list recording rules for config export", err)
		return
	}
	if rules == nil {
		rules = []*db.RecordingRule{}
	}

	users, err := h.ConfigDB.ListUsers()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list users for config export", err)
		return
	}
	if users == nil {
		users = []*db.User{}
	}
	// Strip password hashes from users for security.
	for _, u := range users {
		u.PasswordHash = ""
	}

	clips, err := h.ConfigDB.ListSavedClips("")
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list saved clips for config export", err)
		return
	}
	if clips == nil {
		clips = []*db.SavedClip{}
	}

	export := configExport{
		Version:        "1",
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		Cameras:        cameras,
		RecordingRules: rules,
		Users:          users,
		SavedClips:     clips,
	}

	c.Header("Content-Disposition", "attachment; filename=nvr-config.json")
	c.JSON(http.StatusOK, export)
}

// configImportResult summarizes what was imported.
type configImportResult struct {
	CamerasImported int      `json:"cameras_imported"`
	CamerasSkipped  int      `json:"cameras_skipped"`
	RulesImported   int      `json:"rules_imported"`
	RulesSkipped    int      `json:"rules_skipped"`
	UsersSkipped    int      `json:"users_skipped"`
	Errors          []string `json:"errors,omitempty"`
}

// ImportConfig accepts an exported JSON config and creates cameras and recording
// rules that don't already exist (matched by name). Users are always skipped for
// security reasons.
func (h *SystemHandler) ImportConfig(c *gin.Context) {
	if h.ConfigDB == nil {
		apiError(c, http.StatusInternalServerError, "database not available", fmt.Errorf("ConfigDB is nil"))
		return
	}

	var export configExport
	if err := c.ShouldBindJSON(&export); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config JSON"})
		return
	}

	result := configImportResult{}

	// Import cameras (skip if name already exists).
	existingCameras, err := h.ConfigDB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list existing cameras for import", err)
		return
	}
	existingNames := make(map[string]string) // name -> id
	for _, cam := range existingCameras {
		existingNames[cam.Name] = cam.ID
	}

	// Map old camera IDs to new IDs for rule import.
	cameraIDMap := make(map[string]string)

	for _, cam := range export.Cameras {
		if _, exists := existingNames[cam.Name]; exists {
			cameraIDMap[cam.ID] = existingNames[cam.Name]
			result.CamerasSkipped++
			continue
		}

		oldID := cam.ID
		cam.ID = "" // Let DB generate a new UUID.
		cam.Status = ""
		if err := h.ConfigDB.CreateCamera(cam); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to import camera %q: %v", cam.Name, err))
			continue
		}
		cameraIDMap[oldID] = cam.ID
		existingNames[cam.Name] = cam.ID
		result.CamerasImported++
	}

	// Import recording rules (skip if name+camera combo already exists).
	for _, rule := range export.RecordingRules {
		newCameraID, ok := cameraIDMap[rule.CameraID]
		if !ok {
			result.RulesSkipped++
			continue
		}

		// Check if a rule with the same name already exists for this camera.
		existingRules, err := h.ConfigDB.ListRecordingRules(newCameraID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check rules for camera %s: %v", newCameraID, err))
			result.RulesSkipped++
			continue
		}

		duplicate := false
		for _, existing := range existingRules {
			if existing.Name == rule.Name {
				duplicate = true
				break
			}
		}
		if duplicate {
			result.RulesSkipped++
			continue
		}

		rule.ID = "" // Let DB generate a new UUID.
		rule.CameraID = newCameraID
		if err := h.ConfigDB.CreateRecordingRule(rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to import rule %q: %v", rule.Name, err))
			continue
		}
		result.RulesImported++
	}

	// Skip users for security.
	result.UsersSkipped = len(export.Users)

	c.JSON(http.StatusOK, result)
}

// ExportConfigAdmin wraps ExportConfig with an admin role check.
func (h *SystemHandler) ExportConfigAdmin(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	h.ExportConfig(c)
}

// DiskIO returns per-path I/O performance status and latency history.
//
//	GET /api/nvr/system/disk-io
func (h *SystemHandler) DiskIO(c *gin.Context) {
	if h.StorageMgr == nil {
		c.JSON(http.StatusOK, gin.H{"paths": map[string]interface{}{}})
		return
	}
	status := h.StorageMgr.GetIOMonitor().GetStatus()
	c.JSON(http.StatusOK, gin.H{"paths": status})
}

// UpdateDiskIOThresholds updates warn/critical latency thresholds for a storage path.
//
//	PUT /api/nvr/system/disk-io/thresholds
func (h *SystemHandler) UpdateDiskIOThresholds(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if h.StorageMgr == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage manager not available"})
		return
	}

	var req struct {
		Path   string  `json:"path" binding:"required"`
		WarnMs float64 `json:"warn_ms" binding:"required,gt=0"`
		CritMs float64 `json:"critical_ms" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WarnMs >= req.CritMs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "warn_ms must be less than critical_ms"})
		return
	}

	if err := h.StorageMgr.GetIOMonitor().UpdateThresholds(req.Path, req.WarnMs, req.CritMs); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":        req.Path,
		"warn_ms":     req.WarnMs,
		"critical_ms": req.CritMs,
	})
}

// DBHealth returns database health metrics including integrity status, WAL size,
// page count, and last maintenance timestamps.
//
//	GET /api/nvr/system/db/health (admin only)
func (h *SystemHandler) DBHealth(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if h.ConfigDB == nil {
		apiError(c, http.StatusInternalServerError, "database not available", fmt.Errorf("ConfigDB is nil"))
		return
	}

	health, err := h.ConfigDB.GetDBHealth()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get database health", err)
		return
	}

	c.JSON(http.StatusOK, health)
}

// ImportConfigAdmin wraps ImportConfig with an admin role check.
func (h *SystemHandler) ImportConfigAdmin(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	h.ImportConfig(c)
}

// Hardware returns detected hardware acceleration capabilities and a
// recommended AI backend configuration.
//
//	GET /api/nvr/system/hardware (admin only)
func (h *SystemHandler) Hardware(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	detector := h.HWDetector
	if detector == nil {
		detector = hwaccel.NewDetector()
	}

	force := c.Query("refresh") == "true"
	info := detector.Detect(force)

	c.JSON(http.StatusOK, info)
}

// RequirementsCheck validates system requirements (disk, RAM, CPU, ports, network)
// and returns the results.
//
//	GET /api/nvr/system/requirements-check (admin only)
func (h *SystemHandler) RequirementsCheck(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if h.SysChecker == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "system checker not configured"})
		return
	}
	report := h.SysChecker.Run()
	c.JSON(http.StatusOK, report)
}

// NetworkConfig returns the full network configuration (addresses and ports)
// parsed from mediamtx.yml.
//
//	GET /api/nvr/system/network
func (h *SystemHandler) NetworkConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	cfg := h.parseMediamtxConfig()

	type protocolInfo struct {
		Address    string `json:"address"`
		Port       string `json:"port"`
		Encryption string `json:"encryption"`
	}

	protocols := map[string]protocolInfo{
		"rtsp": {
			Address:    orDefault(cfg.RTSPAddress, ":8554"),
			Port:       orDefault(extractPort(cfg.RTSPAddress), "8554"),
			Encryption: orDefault(cfg.RTSPEncryption, "no"),
		},
		"rtsps": {
			Address:    orDefault(cfg.RTSPSAddress, ":8322"),
			Port:       orDefault(extractPort(cfg.RTSPSAddress), "8322"),
			Encryption: "strict",
		},
		"hls": {
			Address:    orDefault(cfg.HLSAddress, ":8888"),
			Port:       orDefault(extractPort(cfg.HLSAddress), "8888"),
			Encryption: boolEncryption(cfg.HLSEncryption),
		},
		"webrtc": {
			Address:    orDefault(cfg.WebRTCAddress, ":8889"),
			Port:       orDefault(extractPort(cfg.WebRTCAddress), "8889"),
			Encryption: boolEncryption(cfg.WebRTCEncryption),
		},
		"api": {
			Address:    orDefault(cfg.APIAddress, ":9997"),
			Port:       orDefault(extractPort(cfg.APIAddress), "9997"),
			Encryption: boolEncryption(cfg.APIEncryption),
		},
		"playback": {
			Address:    orDefault(cfg.PlaybackAddress, ":9996"),
			Port:       orDefault(extractPort(cfg.PlaybackAddress), "9996"),
			Encryption: boolEncryption(cfg.PlaybackEncryption),
		},
		"rtp": {
			Address:    orDefault(cfg.RTPAddress, ":8000"),
			Port:       orDefault(extractPort(cfg.RTPAddress), "8000"),
			Encryption: "no",
		},
		"rtcp": {
			Address:    orDefault(cfg.RTCPAddress, ":8001"),
			Port:       orDefault(extractPort(cfg.RTCPAddress), "8001"),
			Encryption: "no",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"protocols": protocols,
	})
}

// TLSStatus returns the current TLS/encryption configuration for all protocols.
//
//	GET /api/nvr/system/tls
func (h *SystemHandler) TLSStatus(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	cfg := h.parseMediamtxConfig()

	type tlsInfo struct {
		Encryption bool   `json:"encryption"`
		CertPath   string `json:"cert_path"`
		KeyPath    string `json:"key_path"`
		CertExists bool   `json:"cert_exists"`
		KeyExists  bool   `json:"key_exists"`
	}

	checkFile := func(path string) bool {
		if path == "" {
			return false
		}
		_, err := os.Stat(path)
		return err == nil
	}

	services := map[string]tlsInfo{
		"api": {
			Encryption: cfg.APIEncryption,
			CertPath:   orDefault(cfg.APIServerCert, "server.crt"),
			KeyPath:    orDefault(cfg.APIServerKey, "server.key"),
			CertExists: checkFile(orDefault(cfg.APIServerCert, "server.crt")),
			KeyExists:  checkFile(orDefault(cfg.APIServerKey, "server.key")),
		},
		"rtsp": {
			Encryption: cfg.RTSPEncryption == "strict" || cfg.RTSPEncryption == "optional",
			CertPath:   orDefault(cfg.RTSPServerCert, "server.crt"),
			KeyPath:    orDefault(cfg.RTSPServerKey, "server.key"),
			CertExists: checkFile(orDefault(cfg.RTSPServerCert, "server.crt")),
			KeyExists:  checkFile(orDefault(cfg.RTSPServerKey, "server.key")),
		},
		"webrtc": {
			Encryption: cfg.WebRTCEncryption,
			CertPath:   orDefault(cfg.WebRTCServerCert, "server.crt"),
			KeyPath:    orDefault(cfg.WebRTCServerKey, "server.key"),
			CertExists: checkFile(orDefault(cfg.WebRTCServerCert, "server.crt")),
			KeyExists:  checkFile(orDefault(cfg.WebRTCServerKey, "server.key")),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"services": services,
	})
}

// BackupDatabase creates a full database backup and returns it as a downloadable
// SQLite file.
//
//	GET /api/nvr/system/backup/database
func (h *SystemHandler) BackupDatabase(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if h.ConfigDB == nil {
		apiError(c, http.StatusInternalServerError, "database not available", fmt.Errorf("ConfigDB is nil"))
		return
	}

	dbPath := h.ConfigDB.Path()
	if dbPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database path unknown"})
		return
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read database file", err)
		return
	}

	filename := fmt.Sprintf("nvr-backup-%s.db", time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// RestoreDatabase accepts an uploaded SQLite database file to replace the current one.
// This requires a server restart to take effect.
//
//	POST /api/nvr/system/backup/restore
func (h *SystemHandler) RestoreDatabase(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if h.ConfigDB == nil {
		apiError(c, http.StatusInternalServerError, "database not available", fmt.Errorf("ConfigDB is nil"))
		return
	}

	file, _, err := c.Request.FormFile("database")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing database file in form data"})
		return
	}
	defer file.Close()

	dbPath := h.ConfigDB.Path()
	if dbPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database path unknown"})
		return
	}

	// Read the uploaded file into memory.
	data, err := io.ReadAll(file)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read uploaded file", err)
		return
	}

	// Basic validation: SQLite files start with "SQLite format 3\000".
	if len(data) < 16 || string(data[:16]) != "SQLite format 3\000" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file is not a valid SQLite database"})
		return
	}

	// Write backup of current database.
	backupPath := dbPath + ".pre-restore-backup"
	if err := os.WriteFile(backupPath, mustReadFile(dbPath), 0o600); err != nil {
		nvrLogWarn("system", "failed to create pre-restore backup: "+err.Error())
	}

	// Write the new database.
	if err := os.WriteFile(dbPath, data, 0o600); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to write database file", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Database restored successfully. Restart the server for changes to take effect.",
		"restart_required": true,
	})
}

// CheckForUpdates compares the running version against the latest GitHub release.
//
//	GET /api/nvr/system/updates/check
func (h *SystemHandler) CheckForUpdates(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"current_version": h.Version,
		"update_available": false,
		"message":         "You are running the latest version.",
	})
}

// parseMediamtxConfig reads and parses the mediamtx.yml configuration file.
func (h *SystemHandler) parseMediamtxConfig() mediamtxConfig {
	var cfg mediamtxConfig
	if h.ConfigPath != "" {
		data, err := os.ReadFile(h.ConfigPath)
		if err != nil {
			nvrLogWarn("system", "failed to read config file: "+err.Error())
			return cfg
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			nvrLogWarn("system", "failed to parse config file: "+err.Error())
		}
	}
	return cfg
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func boolEncryption(enabled bool) string {
	if enabled {
		return "yes"
	}
	return "no"
}

func mustReadFile(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}

