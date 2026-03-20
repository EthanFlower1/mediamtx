package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// CameraHandler implements HTTP endpoints for camera management.
type CameraHandler struct {
	DB         *db.DB
	YAMLWriter *yamlwriter.Writer
	Discovery  *onvif.Discovery      // may be nil
	APIAddress string                // MediaMTX API address, e.g. "127.0.0.1:9997"
	Scheduler  *scheduler.Scheduler  // may be nil
	Audit      *AuditLogger
}

// cameraRequest is the JSON body for creating or updating a camera.
type cameraRequest struct {
	Name              string `json:"name" binding:"required"`
	ONVIFEndpoint     string `json:"onvif_endpoint"`
	ONVIFUsername      string `json:"onvif_username"`
	ONVIFPassword     string `json:"onvif_password"`
	ONVIFProfileToken string `json:"onvif_profile_token"`
	RTSPURL           string `json:"rtsp_url"`
	PTZCapable        bool   `json:"ptz_capable"`
	Tags              string `json:"tags"`
}

var nonAlphanumericDash = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizePath converts a camera name to a safe MediaMTX path component.
// It lowercases, replaces spaces with dashes, and strips non-alphanumeric chars.
func sanitizePath(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumericDash.ReplaceAllString(s, "")
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "camera"
	}
	return s
}

// List returns all cameras as a JSON array with live status from MediaMTX.
func (h *CameraHandler) List(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}

	// Enrich with live status from MediaMTX API.
	for _, cam := range cameras {
		if cam.MediaMTXPath != "" {
			cam.Status = h.getPathStatus(cam.MediaMTXPath)
		}
	}

	c.JSON(http.StatusOK, cameras)
}

// getPathStatus checks if a MediaMTX path is online by querying the local API.
func (h *CameraHandler) getPathStatus(pathName string) string {
	addr := h.APIAddress
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	url := fmt.Sprintf("http://%s/v3/paths/get/%s", addr, pathName)
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return "disconnected"
	}
	defer resp.Body.Close()

	var result struct {
		Ready bool `json:"ready"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "disconnected"
	}
	if result.Ready {
		return "online"
	}
	return "disconnected"
}

// Get returns a single camera by ID.
func (h *CameraHandler) Get(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, cam)
}

// Create creates a new camera in the database and writes its path to the YAML config.
func (h *CameraHandler) Create(c *gin.Context) {
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	pathName := "nvr/" + sanitizePath(req.Name)

	cam := &db.Camera{
		Name:              req.Name,
		ONVIFEndpoint:     req.ONVIFEndpoint,
		ONVIFUsername:      req.ONVIFUsername,
		ONVIFPassword:     req.ONVIFPassword,
		ONVIFProfileToken: req.ONVIFProfileToken,
		RTSPURL:           req.RTSPURL,
		PTZCapable:        req.PTZCapable,
		MediaMTXPath:      pathName,
		Tags:              req.Tags,
	}

	if err := h.DB.CreateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create camera"})
		return
	}

	// Write the path to YAML config.
	yamlConfig := map[string]interface{}{
		"source": cam.RTSPURL,
	}
	if err := h.YAMLWriter.AddPath(pathName, yamlConfig); err != nil {
		// Rollback: delete the camera from DB.
		_ = h.DB.DeleteCamera(cam.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write config"})
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "camera", cam.ID, fmt.Sprintf("Created camera %q", cam.Name))
	}

	c.JSON(http.StatusCreated, cam)
}

// Update updates an existing camera in the database and YAML config.
func (h *CameraHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	oldPath := existing.MediaMTXPath
	newPath := "nvr/" + sanitizePath(req.Name)

	existing.Name = req.Name
	existing.ONVIFEndpoint = req.ONVIFEndpoint
	existing.ONVIFUsername = req.ONVIFUsername
	existing.ONVIFPassword = req.ONVIFPassword
	existing.ONVIFProfileToken = req.ONVIFProfileToken
	existing.RTSPURL = req.RTSPURL
	existing.PTZCapable = req.PTZCapable
	existing.MediaMTXPath = newPath
	existing.Tags = req.Tags

	if err := h.DB.UpdateCamera(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update camera"})
		return
	}

	// Remove old path if it changed.
	if oldPath != newPath && oldPath != "" {
		_ = h.YAMLWriter.RemovePath(oldPath)
	}

	// Write the updated path to YAML.
	yamlConfig := map[string]interface{}{
		"source": existing.RTSPURL,
	}
	if err := h.YAMLWriter.AddPath(newPath, yamlConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write config"})
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "camera", existing.ID, fmt.Sprintf("Updated camera %q", existing.Name))
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a camera from the database and YAML config.
func (h *CameraHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if err := h.DB.DeleteCamera(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete camera"})
		return
	}

	// Remove from YAML config.
	if cam.MediaMTXPath != "" {
		_ = h.YAMLWriter.RemovePath(cam.MediaMTXPath)
	}

	// Remove scheduler state for this camera.
	if h.Scheduler != nil {
		h.Scheduler.RemoveCamera(id)
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "camera", id, fmt.Sprintf("Deleted camera %q", cam.Name))
	}

	c.JSON(http.StatusOK, gin.H{"message": "camera deleted"})
}

// Discover triggers an ONVIF WS-Discovery scan.
func (h *CameraHandler) Discover(c *gin.Context) {
	if h.Discovery == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "ONVIF discovery not available"})
		return
	}

	scanID, err := h.Discovery.StartScan()
	if errors.Is(err, onvif.ErrScanInProgress) {
		c.JSON(http.StatusConflict, gin.H{"error": "scan already in progress"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start scan"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"scan_id": scanID})
}

// DiscoverStatus returns the current status of an ONVIF discovery scan.
func (h *CameraHandler) DiscoverStatus(c *gin.Context) {
	if h.Discovery == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "ONVIF discovery not available"})
		return
	}

	status := h.Discovery.GetStatus()
	if status == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no scan started"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// DiscoverResults returns the devices found by the most recent scan.
func (h *CameraHandler) DiscoverResults(c *gin.Context) {
	if h.Discovery == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "ONVIF discovery not available"})
		return
	}

	results := h.Discovery.GetResults()
	c.JSON(http.StatusOK, results)
}

// probeRequest is the JSON body for probing an ONVIF device with credentials.
type probeRequest struct {
	XAddr    string `json:"xaddr" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Probe connects to an ONVIF device with credentials and returns its profiles and stream URIs.
func (h *CameraHandler) Probe(c *gin.Context) {
	var req probeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	profiles, err := onvif.ProbeDevice(req.XAddr, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to probe device: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"profiles": profiles})
}

// retentionRequest is the JSON body for updating a camera's retention policy.
type retentionRequest struct {
	RetentionDays int `json:"retention_days"`
}

// UpdateRetention updates the retention policy for a specific camera.
func (h *CameraHandler) UpdateRetention(c *gin.Context) {
	id := c.Param("id")

	var req retentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.RetentionDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention_days must be >= 0"})
		return
	}

	if err := h.DB.UpdateCameraRetention(id, req.RetentionDays); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update retention"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve camera"})
		return
	}

	c.JSON(http.StatusOK, cam)
}

// ptzRequest is the JSON body for PTZ commands.
type ptzRequest struct {
	Action      string  `json:"action" binding:"required"` // "move", "stop", "preset", "home"
	Pan         float64 `json:"pan"`
	Tilt        float64 `json:"tilt"`
	Zoom        float64 `json:"zoom"`
	PresetToken string  `json:"preset_token"`
}

// PTZCommand handles PTZ control requests.
//
//	POST /cameras/:id/ptz
//	Body: {"action":"move","pan":0.5,"tilt":-0.3,"zoom":0.0}
//	   or {"action":"stop"}
//	   or {"action":"preset","preset_token":"1"}
//	   or {"action":"home"}
func (h *CameraHandler) PTZCommand(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if !cam.PTZCapable {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera is not PTZ-capable"})
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var req ptzRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to ONVIF device: " + err.Error()})
		return
	}

	profileToken := cam.ONVIFProfileToken
	if profileToken == "" {
		profileToken = "000"
	}

	switch req.Action {
	case "move":
		if err := ctrl.ContinuousMove(profileToken, req.Pan, req.Tilt, req.Zoom); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "PTZ move failed: " + err.Error()})
			return
		}
	case "stop":
		if err := ctrl.Stop(profileToken); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "PTZ stop failed: " + err.Error()})
			return
		}
	case "preset":
		if req.PresetToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "preset_token is required for preset action"})
			return
		}
		if err := ctrl.GotoPreset(profileToken, req.PresetToken); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "PTZ goto preset failed: " + err.Error()})
			return
		}
	case "home":
		if err := ctrl.GotoHome(profileToken); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "PTZ goto home failed: " + err.Error()})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action: " + req.Action})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PTZPresets returns the list of PTZ presets for a camera.
//
//	GET /cameras/:id/ptz/presets
func (h *CameraHandler) PTZPresets(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if !cam.PTZCapable {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera is not PTZ-capable"})
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to ONVIF device: " + err.Error()})
		return
	}

	profileToken := cam.ONVIFProfileToken
	if profileToken == "" {
		profileToken = "000"
	}

	presets, err := ctrl.GetPresets(profileToken)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get presets: " + err.Error()})
		return
	}

	if presets == nil {
		presets = []onvif.PTZPreset{}
	}

	c.JSON(http.StatusOK, gin.H{"presets": presets})
}

// GetSettings retrieves the current imaging settings from a camera via ONVIF.
func (h *CameraHandler) GetSettings(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	videoSourceToken := cam.ONVIFProfileToken
	if videoSourceToken == "" {
		videoSourceToken = "000"
	}

	settings, err := onvif.GetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword, videoSourceToken)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get imaging settings: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// imagingSettingsRequest is the JSON body for updating camera imaging settings.
type imagingSettingsRequest struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Sharpness  float64 `json:"sharpness"`
}

// UpdateSettings applies imaging settings to a camera via ONVIF.
func (h *CameraHandler) UpdateSettings(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var req imagingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	videoSourceToken := cam.ONVIFProfileToken
	if videoSourceToken == "" {
		videoSourceToken = "000"
	}

	settings := &onvif.ImagingSettings{
		Brightness: req.Brightness,
		Contrast:   req.Contrast,
		Saturation: req.Saturation,
		Sharpness:  req.Sharpness,
	}

	if err := onvif.SetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword, videoSourceToken, settings); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply imaging settings: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}
