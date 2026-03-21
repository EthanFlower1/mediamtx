package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"

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
	ConfigDB       *db.DB  // full DB access for config export/import
	ConfigPath     string  // path to mediamtx.yml for reading server configuration
	APIAddress     string  // MediaMTX API address for live camera status
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

	c.JSON(http.StatusOK, gin.H{
		"cpu_goroutines":  runtime.NumGoroutine(),
		"mem_alloc_bytes": m.Alloc,
		"mem_sys_bytes":   m.Sys,
		"mem_gc_count":    m.NumGC,
		"uptime_seconds":  time.Since(h.StartedAt).Seconds(),
		"camera_count":    cameraCount,
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

	c.JSON(http.StatusOK, gin.H{
		"total_bytes":      totalBytes,
		"free_bytes":       freeBytes,
		"used_bytes":       usedBytes,
		"recordings_bytes": recordingsBytes,
		"per_camera":       perCamera,
		"warning":          usedPercent > 85,
		"critical":         usedPercent > 95,
	})
}

// mediamtxConfig is a partial representation of mediamtx.yml used to extract
// configuration values for the config summary endpoint.
type mediamtxConfig struct {
	RTSPAddress  string `yaml:"rtspAddress"`
	HLSAddress   string `yaml:"hlsAddress"`
	WebRTCAddress string `yaml:"webrtcAddress"`
	APIAddress   string `yaml:"apiAddress"`
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
	var cfg mediamtxConfig
	if h.ConfigPath != "" {
		data, err := os.ReadFile(h.ConfigPath)
		if err != nil {
			nvrLogWarn("system", "failed to read config file for summary: "+err.Error())
		} else {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				nvrLogWarn("system", "failed to parse config file for summary: "+err.Error())
			}
		}
	}

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

			// Check live status from MediaMTX API.
			statuses := h.getCameraStatuses()
			for _, cam := range cameras {
				if cam.MediaMTXPath != "" {
					if s, ok := statuses[cam.MediaMTXPath]; ok && s == "online" {
						onlineCameras++
					}
				}
			}

			// Count cameras that have at least one enabled recording rule.
			camerasWithRules := make(map[string]bool)
			for _, cam := range cameras {
				rules, err := h.ConfigDB.ListRecordingRules(cam.ID)
				if err == nil {
					for _, rule := range rules {
						totalRules++
						if rule.Enabled {
							activeRules++
							camerasWithRules[cam.ID] = true
						}
					}
				}
			}
			recordingCameras = len(camerasWithRules)
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

// getCameraStatuses fetches all path statuses from the MediaMTX API.
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
	resp, err := http.Get(url)
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

	export := configExport{
		Version:        "1",
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		Cameras:        cameras,
		RecordingRules: rules,
		Users:          users,
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

// ImportConfigAdmin wraps ImportConfig with an admin role check.
func (h *SystemHandler) ImportConfigAdmin(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	h.ImportConfig(c)
}

