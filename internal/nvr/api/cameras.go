package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	nvrCrypto "github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// pathItem represents a single entry from the MediaMTX paths list API.
type pathItem struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// CameraHandler implements HTTP endpoints for camera management.
type CameraHandler struct {
	DB            *db.DB
	YAMLWriter    *yamlwriter.Writer
	Discovery     *onvif.Discovery      // may be nil
	APIAddress    string                // MediaMTX API address, e.g. "127.0.0.1:9997"
	Scheduler     *scheduler.Scheduler  // may be nil
	Audit         *AuditLogger
	EncryptionKey []byte                // AES-256 key for encrypting ONVIF credentials at rest
	AIRestarter   AIPipelineRestarter   // may be nil
	StorageMgr    *storage.Manager      // may be nil
}

// cameraResponse wraps db.Camera and adds a storage_status field.
type cameraResponse struct {
	db.Camera
	StorageStatus string `json:"storage_status"`
}

// cameraWithStreams extends cameraResponse to include stream records.
type cameraWithStreams struct {
	cameraResponse
	Streams []*db.CameraStream `json:"streams"`
}

func (h *CameraHandler) buildCameraResponse(cam *db.Camera) cameraResponse {
	status := "default"
	if h.StorageMgr != nil {
		status = h.StorageMgr.StorageStatus(cam)
	}
	return cameraResponse{Camera: *cam, StorageStatus: status}
}

func (h *CameraHandler) buildCameraWithStreams(cam *db.Camera) cameraWithStreams {
	resp := h.buildCameraResponse(cam)
	streams, err := h.DB.ListCameraStreams(cam.ID)
	if err != nil {
		nvrLogWarn("cameras", "failed to list streams for camera "+cam.ID+": "+err.Error())
		streams = []*db.CameraStream{}
	}
	if streams == nil {
		streams = []*db.CameraStream{}
	}
	return cameraWithStreams{cameraResponse: resp, Streams: streams}
}

// encryptPassword encrypts a plaintext password for storage. If no encryption
// key is configured or the password is empty, the plaintext is returned as-is.
func (h *CameraHandler) encryptPassword(plaintext string) string {
	if len(h.EncryptionKey) == 0 || plaintext == "" {
		return plaintext
	}
	ct, err := nvrCrypto.Encrypt(h.EncryptionKey, []byte(plaintext))
	if err != nil {
		nvrLogWarn("cameras", "failed to encrypt ONVIF password, storing plaintext")
		return plaintext
	}
	return "enc:" + base64.StdEncoding.EncodeToString(ct)
}

// decryptPassword decrypts a stored password. If the value does not have
// the "enc:" prefix it is returned unchanged (plaintext / legacy value).
func (h *CameraHandler) decryptPassword(stored string) string {
	if len(h.EncryptionKey) == 0 || stored == "" {
		return stored
	}
	if !strings.HasPrefix(stored, "enc:") {
		return stored
	}
	ct, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil {
		nvrLogWarn("cameras", "failed to decode encrypted ONVIF password")
		return ""
	}
	pt, err := nvrCrypto.Decrypt(h.EncryptionKey, ct)
	if err != nil {
		nvrLogWarn("cameras", "failed to decrypt ONVIF password")
		return ""
	}
	return string(pt)
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
	StoragePath       string `json:"storage_path"`
	Profiles          []struct {
		Name         string `json:"name"`
		RTSPURL      string `json:"rtsp_url"`
		ProfileToken string `json:"profile_token"`
		VideoCodec   string `json:"video_codec"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
	} `json:"profiles"`
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
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}

	// Batch-fetch all path statuses in a single HTTP call instead of N+1 queries.
	statuses := h.getPathStatuses()
	for _, cam := range cameras {
		if statuses == nil {
			cam.Status = "unknown" // API unreachable
		} else if cam.MediaMTXPath != "" {
			if s, ok := statuses[cam.MediaMTXPath]; ok {
				cam.Status = s
			} else {
				cam.Status = "disconnected"
			}
		}
	}

	responses := make([]cameraWithStreams, 0, len(cameras))
	for _, cam := range cameras {
		responses = append(responses, h.buildCameraWithStreams(cam))
	}
	c.JSON(http.StatusOK, responses)
}

// getPathStatuses fetches all paths from MediaMTX in one call and returns a
// map of path name to status string ("online" or "disconnected").
// Returns nil when the MediaMTX API is unreachable so callers can distinguish
// "API down" from "all cameras disconnected" (empty map).
func (h *CameraHandler) getPathStatuses() map[string]string {
	addr := h.APIAddress
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	url := fmt.Sprintf("http://%s/v3/paths/list", addr)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		nvrLogWarn("cameras", "failed to fetch path statuses from MediaMTX: "+err.Error())
		return nil // API unreachable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		nvrLogWarn("cameras", fmt.Sprintf("MediaMTX paths list returned status %d", resp.StatusCode))
		return nil // API error
	}

	var result struct {
		Items []pathItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		nvrLogWarn("cameras", "failed to decode MediaMTX paths list: "+err.Error())
		return nil // API returned invalid data
	}

	statuses := make(map[string]string, len(result.Items))
	for _, item := range result.Items {
		if item.Ready {
			statuses[item.Name] = "online"
		} else {
			statuses[item.Name] = "disconnected"
		}
	}
	return statuses
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	// Enrich with live status from MediaMTX (same as List).
	statuses := h.getPathStatuses()
	if statuses == nil {
		cam.Status = "unknown"
	} else if cam.MediaMTXPath != "" {
		if s, ok := statuses[cam.MediaMTXPath]; ok {
			cam.Status = s
		} else {
			cam.Status = "disconnected"
		}
	}

	c.JSON(http.StatusOK, h.buildCameraWithStreams(cam))
}

// Create creates a new camera in the database and writes its path to the YAML config.
func (h *CameraHandler) Create(c *gin.Context) {
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate storage path if provided.
	if req.StoragePath != "" {
		if !filepath.IsAbs(req.StoragePath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path must be an absolute path"})
			return
		}
		// Verify the path is writable by attempting to create a temp file.
		testFile := filepath.Join(req.StoragePath, ".nvr_write_check")
		if f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0o600); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path is not writable: " + err.Error()})
			return
		} else {
			f.Close()
			os.Remove(testFile)
		}
	}

	// Generate camera ID early so it can be used in path naming.
	camID := uuid.New().String()
	pathName := "nvr/" + camID + "/main"

	storagePath := req.StoragePath
	if storagePath == "" {
		storagePath = "./recordings"
	}
	recordPath := storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"

	cam := &db.Camera{
		ID:                camID,
		Name:              req.Name,
		ONVIFEndpoint:     req.ONVIFEndpoint,
		ONVIFUsername:      req.ONVIFUsername,
		ONVIFPassword:     h.encryptPassword(req.ONVIFPassword),
		ONVIFProfileToken: req.ONVIFProfileToken,
		RTSPURL:           req.RTSPURL,
		PTZCapable:        req.PTZCapable,
		MediaMTXPath:      pathName,
		Tags:              req.Tags,
		StoragePath:       req.StoragePath,
	}

	if err := h.DB.CreateCamera(cam); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create camera", err)
		return
	}

	// Auto-create stream records from the provided profiles.
	if len(req.Profiles) > 0 {
		for i, p := range req.Profiles {
			var roles string
			switch {
			case len(req.Profiles) == 1:
				roles = "live_view,recording,ai_detection,mobile"
			case i == 0:
				roles = "live_view"
			case i == len(req.Profiles)-1:
				roles = "recording,ai_detection,mobile"
			default:
				roles = ""
			}
			stream := &db.CameraStream{
				CameraID:     cam.ID,
				Name:         p.Name,
				RTSPURL:      p.RTSPURL,
				ProfileToken: p.ProfileToken,
				VideoCodec:   p.VideoCodec,
				Width:        p.Width,
				Height:       p.Height,
				Roles:        roles,
			}
			if err := h.DB.CreateCameraStream(stream); err != nil {
				nvrLogWarn("cameras", fmt.Sprintf("failed to create stream for camera %s: %v", cam.ID, err))
			}
		}
	}

	// Write the path to YAML config.
	yamlConfig := map[string]interface{}{
		"source":     cam.RTSPURL,
		"record":     true,
		"recordPath": recordPath,
	}
	if err := h.YAMLWriter.AddPath(pathName, yamlConfig); err != nil {
		// Rollback: delete the camera from DB.
		_ = h.DB.DeleteCamera(cam.ID)
		apiError(c, http.StatusInternalServerError, "failed to write config", err)
		return
	}

	nvrLogInfo("cameras", fmt.Sprintf("Created camera %q (id=%s, path=%s)", cam.Name, cam.ID, pathName))

	// Populate ONVIF capability flags in the background if the camera has an ONVIF endpoint.
	if cam.ONVIFEndpoint != "" {
		go func(camCopy db.Camera) {
			done := make(chan struct{})
			var result *onvif.ProbeResult
			var probeErr error
			go func() {
				defer close(done)
				result, probeErr = onvif.ProbeDeviceFull(camCopy.ONVIFEndpoint, camCopy.ONVIFUsername, h.decryptPassword(camCopy.ONVIFPassword))
			}()
			select {
			case <-done:
				// probe completed
			case <-time.After(15 * time.Second):
				log.Printf("[NVR] [WARN] [cameras] ONVIF probe timed out for camera %s", camCopy.ID)
				return
			}
			if probeErr != nil {
				nvrLogWarn("cameras", fmt.Sprintf("background probe failed for camera %s: %v", camCopy.ID, probeErr))
				return
			}
			camCopy.SupportsPTZ = result.Capabilities.PTZ
			camCopy.SupportsImaging = result.Capabilities.Imaging
			camCopy.SupportsEvents = result.Capabilities.Events
			camCopy.SupportsRelay = result.Capabilities.DeviceIO
			camCopy.SupportsAudioBackchannel = result.Capabilities.AudioBackchannel
			camCopy.SupportsMedia2 = result.Capabilities.Media2
			camCopy.SupportsAnalytics = result.Capabilities.Analytics
			camCopy.SupportsEdgeRecording = result.Capabilities.Recording && result.Capabilities.Replay
			camCopy.SnapshotURI = result.SnapshotURI
			if err := h.DB.UpdateCamera(&camCopy); err != nil {
				nvrLogWarn("cameras", fmt.Sprintf("failed to store capabilities for camera %s: %v", camCopy.ID, err))
			} else {
				nvrLogInfo("cameras", fmt.Sprintf("Populated capabilities for camera %q (id=%s)", camCopy.Name, camCopy.ID))
			}
		}(*cam)
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "camera", cam.ID, fmt.Sprintf("Created camera %q", cam.Name))
	}

	c.JSON(http.StatusCreated, h.buildCameraWithStreams(cam))
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for update", err)
		return
	}

	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate storage path if provided.
	if req.StoragePath != "" {
		if !filepath.IsAbs(req.StoragePath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path must be an absolute path"})
			return
		}
		req.StoragePath = filepath.Clean(req.StoragePath)
		testFile := filepath.Join(req.StoragePath, ".nvr_write_test")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path is not writable: " + err.Error()})
			return
		}
		os.Remove(testFile)
	}

	// Only update fields that are actually provided (non-zero) to avoid
	// wiping out fields the client didn't send.
	existing.Name = req.Name
	if req.ONVIFEndpoint != "" || existing.ONVIFEndpoint != "" {
		existing.ONVIFEndpoint = req.ONVIFEndpoint
	}
	if req.ONVIFUsername != "" {
		existing.ONVIFUsername = req.ONVIFUsername
	}
	if req.ONVIFPassword != "" {
		existing.ONVIFPassword = h.encryptPassword(req.ONVIFPassword)
	}
	if req.ONVIFProfileToken != "" {
		existing.ONVIFProfileToken = req.ONVIFProfileToken
	}
	if req.RTSPURL != "" {
		existing.RTSPURL = req.RTSPURL
	}
	existing.PTZCapable = req.PTZCapable
	if req.Tags != "" || existing.Tags != "" {
		existing.Tags = req.Tags
	}

	// Detect storage path change before updating existing.StoragePath.
	storagePathChanged := req.StoragePath != existing.StoragePath
	existing.StoragePath = req.StoragePath

	// The mediamtx_path is immutable after creation — renaming a camera must
	// not change the stream path, as that would break live connections and
	// recording associations.
	stablePath := existing.MediaMTXPath

	if err := h.DB.UpdateCamera(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update camera", err)
		return
	}

	// Build YAML config update — always refresh source, and include recordPath
	// if the storage path changed.
	yamlConfig := map[string]interface{}{
		"source": existing.RTSPURL,
	}
	if storagePathChanged {
		storagePath := existing.StoragePath
		if storagePath == "" {
			storagePath = "./recordings"
		}
		yamlConfig["recordPath"] = storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
	}
	if err := h.YAMLWriter.AddPath(stablePath, yamlConfig); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to write config", err)
		return
	}

	nvrLogInfo("cameras", fmt.Sprintf("Updated camera %q (id=%s)", existing.Name, existing.ID))

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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for deletion", err)
		return
	}

	if err := h.DB.DeleteCamera(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete camera", err)
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

	nvrLogInfo("cameras", fmt.Sprintf("Deleted camera %q (id=%s)", cam.Name, id))

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
		apiError(c, http.StatusInternalServerError, "failed to start ONVIF discovery scan", err)
		return
	}

	nvrLogInfo("discovery", fmt.Sprintf("Started ONVIF discovery scan (id=%s)", scanID))
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

// DiscoverResults returns the devices found by the most recent scan,
// annotated with ExistingCameraID for devices already added as cameras.
func (h *CameraHandler) DiscoverResults(c *gin.Context) {
	if h.Discovery == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "ONVIF discovery not available"})
		return
	}

	results := h.Discovery.GetResults()

	cameras, err := h.DB.ListCameras()
	if err == nil {
		endpointToID := make(map[string]string)
		for _, cam := range cameras {
			if cam.ONVIFEndpoint != "" {
				endpointToID[cam.ONVIFEndpoint] = cam.ID
			}
		}
		for i := range results {
			if id, ok := endpointToID[results[i].XAddr]; ok {
				results[i].ExistingCameraID = id
			}
		}
	}

	c.JSON(http.StatusOK, results)
}

// probeRequest is the JSON body for probing an ONVIF device with credentials.
type probeRequest struct {
	XAddr    string `json:"xaddr" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Probe connects to an ONVIF device with credentials and returns its profiles,
// capabilities, and snapshot URI.
func (h *CameraHandler) Probe(c *gin.Context) {
	var req probeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := onvif.ProbeDeviceFull(req.XAddr, req.Username, req.Password)
	if err != nil {
		nvrLogError("discovery", fmt.Sprintf("ONVIF probe failed for %s", req.XAddr), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable or probe failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"profiles":     result.Profiles,
		"capabilities": result.Capabilities,
		"snapshot_uri":  result.SnapshotURI,
	})
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
		apiError(c, http.StatusInternalServerError, "failed to update retention policy", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera after retention update", err)
		return
	}

	c.JSON(http.StatusOK, cam)
}

// motionTimeoutRequest is the JSON body for updating a camera's motion timeout.
type motionTimeoutRequest struct {
	MotionTimeoutSeconds int `json:"motion_timeout_seconds"`
}

// UpdateMotionTimeout updates the motion_timeout_seconds field for a specific camera.
//
//	PUT /cameras/:id/motion-timeout
func (h *CameraHandler) UpdateMotionTimeout(c *gin.Context) {
	id := c.Param("id")

	var req motionTimeoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.MotionTimeoutSeconds < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "motion_timeout_seconds must be >= 1"})
		return
	}

	if err := h.DB.UpdateCameraMotionTimeout(id, req.MotionTimeoutSeconds); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update motion timeout", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera after motion timeout update", err)
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for PTZ", err)
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

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("ptz", fmt.Sprintf("failed to connect to ONVIF device for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable"})
		return
	}

	profileToken := cam.ONVIFProfileToken
	if profileToken == "" {
		profileToken = "000"
	}

	switch req.Action {
	case "move":
		if err := ctrl.ContinuousMove(profileToken, req.Pan, req.Tilt, req.Zoom); err != nil {
			nvrLogError("ptz", "PTZ move failed", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PTZ move failed"})
			return
		}
	case "stop":
		if err := ctrl.Stop(profileToken); err != nil {
			nvrLogError("ptz", "PTZ stop failed", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PTZ stop failed"})
			return
		}
	case "preset":
		if req.PresetToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "preset_token is required for preset action"})
			return
		}
		if err := ctrl.GotoPreset(profileToken, req.PresetToken); err != nil {
			nvrLogError("ptz", "PTZ goto preset failed", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PTZ goto preset failed"})
			return
		}
	case "home":
		if err := ctrl.GotoHome(profileToken); err != nil {
			nvrLogError("ptz", "PTZ goto home failed", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PTZ goto home failed"})
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for PTZ presets", err)
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

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("ptz", fmt.Sprintf("failed to connect to ONVIF device for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable"})
		return
	}

	profileToken := cam.ONVIFProfileToken
	if profileToken == "" {
		profileToken = "000"
	}

	presets, err := ctrl.GetPresets(profileToken)
	if err != nil {
		nvrLogError("ptz", "failed to get PTZ presets", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get presets from device"})
		return
	}

	if presets == nil {
		presets = []onvif.PTZPreset{}
	}

	c.JSON(http.StatusOK, gin.H{"presets": presets})
}

// PTZCapabilities returns the PTZ node capabilities for a camera.
//
//	GET /cameras/:id/ptz/capabilities
func (h *CameraHandler) PTZCapabilities(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for PTZ capabilities", err)
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

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("ptz", fmt.Sprintf("failed to connect to ONVIF device for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable"})
		return
	}

	nodes, err := ctrl.GetNodes()
	if err != nil {
		nvrLogError("ptz", "failed to get PTZ nodes", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get PTZ capabilities from device"})
		return
	}

	if nodes == nil {
		nodes = []onvif.PTZNode{}
	}

	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera settings", err)
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

	settings, err := onvif.GetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), videoSourceToken)
	if err != nil {
		if errors.Is(err, onvif.ErrImagingNotSupported) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "This camera does not support ONVIF image settings. Use the camera's web interface instead."})
			return
		}
		nvrLogError("imaging", fmt.Sprintf("failed to get imaging settings for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable or imaging query failed"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// GetRelayOutputs returns the list of relay outputs for a camera.
//
//	GET /cameras/:id/relay-outputs
func (h *CameraHandler) GetRelayOutputs(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for relay outputs", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	outputs, err := onvif.GetRelayOutputs(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("relay", fmt.Sprintf("failed to get relay outputs for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get relay outputs from device"})
		return
	}

	if outputs == nil {
		outputs = []onvif.RelayOutput{}
	}

	c.JSON(http.StatusOK, gin.H{"relay_outputs": outputs})
}

// relayStateRequest is the JSON body for toggling a relay output.
type relayStateRequest struct {
	Active bool `json:"active"`
}

// SetRelayOutputState toggles the state of a relay output on a camera.
//
//	POST /cameras/:id/relay-outputs/:token/state
func (h *CameraHandler) SetRelayOutputState(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for relay state", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var req relayStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := onvif.SetRelayOutputState(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token, req.Active); err != nil {
		nvrLogError("relay", fmt.Sprintf("failed to set relay output state for camera %s token %s", id, token), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to set relay output state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// AudioCapabilities returns the audio capabilities for a camera.
//
//	GET /cameras/:id/audio/capabilities
func (h *CameraHandler) AudioCapabilities(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for audio capabilities", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	caps, err := onvif.GetAudioCapabilities(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		nvrLogError("audio", fmt.Sprintf("failed to get audio capabilities for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get audio capabilities from device"})
		return
	}

	c.JSON(http.StatusOK, caps)
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
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for settings update", err)
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

	if err := onvif.SetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), videoSourceToken, settings); err != nil {
		nvrLogError("imaging", fmt.Sprintf("failed to apply imaging settings for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ONVIF device unreachable or settings update failed"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// --- Analytics Rule Configuration API ---

// analyticsConfigToken returns the analytics configuration token for a camera.
// Uses the ONVIF profile token if set, or "000" as default.
func analyticsConfigToken(cam *db.Camera) string {
	if cam.ONVIFProfileToken != "" {
		return cam.ONVIFProfileToken
	}
	return "000"
}

// GetAnalyticsRules returns all analytics rules for a camera.
//
//	GET /cameras/:id/analytics/rules
func (h *CameraHandler) GetAnalyticsRules(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for analytics rules", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	rules, err := onvif.GetRules(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), analyticsConfigToken(cam))
	if err != nil {
		nvrLogError("analytics", fmt.Sprintf("failed to get analytics rules for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get analytics rules from device"})
		return
	}

	if rules == nil {
		rules = []onvif.AnalyticsRule{}
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// CreateAnalyticsRule creates a new analytics rule on the camera.
//
//	POST /cameras/:id/analytics/rules
func (h *CameraHandler) CreateAnalyticsRule(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for analytics rule creation", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var rule onvif.AnalyticsRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if rule.Name == "" || rule.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and type are required"})
		return
	}

	if err := onvif.CreateRule(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), analyticsConfigToken(cam), rule); err != nil {
		nvrLogError("analytics", fmt.Sprintf("failed to create analytics rule for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create analytics rule on device"})
		return
	}

	nvrLogInfo("analytics", fmt.Sprintf("Created analytics rule %q on camera %s", rule.Name, id))
	c.JSON(http.StatusCreated, gin.H{"status": "ok", "rule": rule})
}

// UpdateAnalyticsRule modifies an existing analytics rule on the camera.
//
//	PUT /cameras/:id/analytics/rules/:name
func (h *CameraHandler) UpdateAnalyticsRule(c *gin.Context) {
	id := c.Param("id")
	ruleName := c.Param("name")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for analytics rule update", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var rule onvif.AnalyticsRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Use the URL parameter name if not specified in the body.
	if rule.Name == "" {
		rule.Name = ruleName
	}

	if rule.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type is required"})
		return
	}

	if err := onvif.ModifyRule(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), analyticsConfigToken(cam), rule); err != nil {
		nvrLogError("analytics", fmt.Sprintf("failed to update analytics rule %q for camera %s", ruleName, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update analytics rule on device"})
		return
	}

	nvrLogInfo("analytics", fmt.Sprintf("Updated analytics rule %q on camera %s", ruleName, id))
	c.JSON(http.StatusOK, gin.H{"status": "ok", "rule": rule})
}

// DeleteAnalyticsRule deletes an analytics rule from the camera.
//
//	DELETE /cameras/:id/analytics/rules/:name
func (h *CameraHandler) DeleteAnalyticsRule(c *gin.Context) {
	id := c.Param("id")
	ruleName := c.Param("name")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for analytics rule deletion", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	if err := onvif.DeleteRule(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), analyticsConfigToken(cam), ruleName); err != nil {
		nvrLogError("analytics", fmt.Sprintf("failed to delete analytics rule %q for camera %s", ruleName, id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete analytics rule on device"})
		return
	}

	nvrLogInfo("analytics", fmt.Sprintf("Deleted analytics rule %q from camera %s", ruleName, id))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetAnalyticsModules returns all analytics modules for a camera.
//
//	GET /cameras/:id/analytics/modules
func (h *CameraHandler) GetAnalyticsModules(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for analytics modules", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	modules, err := onvif.GetAnalyticsModules(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), analyticsConfigToken(cam))
	if err != nil {
		nvrLogError("analytics", fmt.Sprintf("failed to get analytics modules for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get analytics modules from device"})
		return
	}

	if modules == nil {
		modules = []onvif.AnalyticsModule{}
	}

	c.JSON(http.StatusOK, gin.H{"modules": modules})
}

// EdgeRecordings lists recordings stored on the camera's edge storage (SD card).
//
//	GET /cameras/:id/edge-recordings
func (h *CameraHandler) EdgeRecordings(c *gin.Context) {
	id := c.Param("id")

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for edge recordings", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)

	summary, err := onvif.GetRecordingSummary(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		nvrLogError("edge-recordings", fmt.Sprintf("failed to get recording summary for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get recording summary from device"})
		return
	}

	recordings, err := onvif.FindRecordings(cam.ONVIFEndpoint, cam.ONVIFUsername, password)
	if err != nil {
		nvrLogError("edge-recordings", fmt.Sprintf("failed to find recordings for camera %s", id), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to find recordings on device"})
		return
	}

	if recordings == nil {
		recordings = []onvif.EdgeRecording{}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":    summary,
		"recordings": recordings,
	})
}

// EdgePlayback returns the replay RTSP URI for a specific recording on the camera's edge storage.
//
//	GET /cameras/:id/edge-recordings/playback?recording_token=X
func (h *CameraHandler) EdgePlayback(c *gin.Context) {
	id := c.Param("id")
	recordingToken := c.Query("recording_token")

	if recordingToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for edge playback", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	replayUri, err := onvif.GetReplayUri(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), recordingToken)
	if err != nil {
		nvrLogError("edge-recordings", fmt.Sprintf("failed to get replay URI for camera %s token %s", id, recordingToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get replay URI from device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"replay_uri": replayUri})
}

// EdgeImport returns the replay URI for importing an edge recording.
// In v1 this simply returns the URI; future versions may download and re-mux locally.
//
//	POST /cameras/:id/edge-recordings/import
func (h *CameraHandler) EdgeImport(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		RecordingToken string `json:"recording_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recording_token is required"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera for edge import", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	replayUri, err := onvif.GetReplayUri(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.RecordingToken)
	if err != nil {
		nvrLogError("edge-recordings", fmt.Sprintf("failed to get replay URI for import camera %s token %s", id, req.RecordingToken), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get replay URI from device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"replay_uri": replayUri,
		"message":    "Use this RTSP URI to stream or record the edge recording. Local import is planned for a future version.",
	})
}

// aiConfigRequest is the JSON body for updating AI configuration on a camera.
type aiConfigRequest struct {
	AIEnabled    bool   `json:"ai_enabled"`
	SubStreamURL string `json:"sub_stream_url"`
}

// UpdateAIConfig handles PUT /cameras/:id/ai — updates AI-specific configuration
// for a camera (ai_enabled flag and sub-stream URL for AI processing).
func (h *CameraHandler) UpdateAIConfig(c *gin.Context) {
	id := c.Param("id")

	var req aiConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.DB.UpdateCameraAIConfig(id, req.AIEnabled, req.SubStreamURL); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update AI config", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera after AI config update", err)
		return
	}

	if h.AIRestarter != nil {
		h.AIRestarter.RestartAIPipeline(id)
	}

	c.JSON(http.StatusOK, cam)
}

type audioTranscodeRequest struct {
	AudioTranscode bool `json:"audio_transcode"`
}

// UpdateAudioTranscode handles PUT /cameras/:id/audio-transcode — toggles
// the audio transcode (live re-encoding) flag for a camera.
func (h *CameraHandler) UpdateAudioTranscode(c *gin.Context) {
	id := c.Param("id")

	var req audioTranscodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.DB.UpdateCameraAudioTranscode(id, req.AudioTranscode); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update audio transcode setting", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera after update", err)
		return
	}

	c.JSON(http.StatusOK, cam)
}

// LatestDetections returns the most recent detections (last 2 seconds) for a
// camera. The frontend polls this endpoint to render bounding box overlays.
//
// GET /api/nvr/cameras/:id/detections/latest
func (h *CameraHandler) LatestDetections(c *gin.Context) {
	id := c.Param("id")
	since := time.Now().Add(-2 * time.Second)

	detections, err := h.DB.GetRecentDetections(id, since)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recent detections", err)
		return
	}
	if detections == nil {
		detections = []*db.Detection{}
	}
	c.JSON(http.StatusOK, detections)
}
