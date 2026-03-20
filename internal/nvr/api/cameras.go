package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// CameraHandler implements HTTP endpoints for camera management.
type CameraHandler struct {
	DB         *db.DB
	YAMLWriter *yamlwriter.Writer
	Discovery  *onvif.Discovery // may be nil
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

// List returns all cameras as a JSON array.
func (h *CameraHandler) List(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}
	c.JSON(http.StatusOK, cameras)
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

// PTZCommand is a stub endpoint for PTZ control.
func (h *CameraHandler) PTZCommand(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "PTZ control not implemented"})
}

// PTZPresets is a stub endpoint for PTZ presets.
func (h *CameraHandler) PTZPresets(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "PTZ presets not implemented"})
}

// GetSettings is a stub endpoint for camera settings retrieval.
func (h *CameraHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "camera settings not implemented"})
}

// UpdateSettings is a stub endpoint for camera settings update.
func (h *CameraHandler) UpdateSettings(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "camera settings not implemented"})
}
