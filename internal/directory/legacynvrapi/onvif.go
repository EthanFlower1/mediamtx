package legacynvrapi

// onvif.go — ONVIF device management endpoints for the legacy NVR API.
//
// Discovery:
//   POST  /api/nvr/cameras/discover          — Start WS-Discovery scan
//   GET   /api/nvr/cameras/discover/results  — Return accumulated results
//   POST  /api/nvr/cameras/probe             — Probe a specific device
//
// Per-camera device info (all require camera lookup from directory DB):
//   GET   /api/nvr/cameras/{id}/device-info
//   GET   /api/nvr/cameras/{id}/settings
//   PUT   /api/nvr/cameras/{id}/settings
//   GET   /api/nvr/cameras/{id}/relay-outputs
//   GET   /api/nvr/cameras/{id}/ptz/presets
//   GET   /api/nvr/cameras/{id}/ptz/status
//   POST  /api/nvr/cameras/{id}/ptz
//   GET   /api/nvr/cameras/{id}/audio/capabilities
//   GET   /api/nvr/cameras/{id}/media/profiles
//   GET   /api/nvr/cameras/{id}/media/video-sources
//   GET   /api/nvr/cameras/{id}/device/datetime
//   GET   /api/nvr/cameras/{id}/device/hostname
//   GET   /api/nvr/cameras/{id}/device/network/interfaces
//   GET   /api/nvr/cameras/{id}/device/network/protocols
//   GET   /api/nvr/cameras/{id}/device/users

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// discoveryState holds a shared Discovery engine and its mutex-protected results.
// It is lazily initialised the first time a discover request arrives.
var (
	discoveryMu     sync.Mutex
	sharedDiscovery *onvif.Discovery
)

func getDiscovery() *onvif.Discovery {
	discoveryMu.Lock()
	defer discoveryMu.Unlock()
	if sharedDiscovery == nil {
		sharedDiscovery = onvif.NewDiscovery()
	}
	return sharedDiscovery
}

// -----------------------------------------------------------------------
// Discovery
// -----------------------------------------------------------------------

// handleCameraDiscover handles POST /api/nvr/cameras/discover.
func (h *Handlers) handleCameraDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	d := getDiscovery()
	scanID, err := d.StartScan()
	if err != nil {
		// ErrScanInProgress — return current status instead of an error.
		result := d.GetStatus()
		writeJSON(w, http.StatusOK, result)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"scan_id": scanID,
		"status":  "scanning",
	})
}

// handleCameraDiscoverResults handles GET /api/nvr/cameras/discover/results.
func (h *Handlers) handleCameraDiscoverResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	d := getDiscovery()
	result := d.GetStatus()
	if result == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"scan_id": "",
			"status":  "idle",
			"devices": []any{},
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCameraProbe handles POST /api/nvr/cameras/probe.
func (h *Handlers) handleCameraProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Endpoint string `json:"endpoint"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if body.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
		return
	}

	result, err := onvif.ProbeDeviceFull(body.Endpoint, body.Username, body.Password)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// -----------------------------------------------------------------------
// Camera lookup helper
// -----------------------------------------------------------------------

// cameraForOnvif looks up the camera record from the directory DB.
// It writes the HTTP error response itself and returns nil on failure.
func (h *Handlers) cameraForOnvif(w http.ResponseWriter, id string) *dirdb.Camera {
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
			return nil
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return nil
	}
	return cam
}

// -----------------------------------------------------------------------
// Device-info
// -----------------------------------------------------------------------

// handleCameraDeviceInfo handles GET /api/nvr/cameras/{id}/device-info.
func (h *Handlers) handleCameraDeviceInfo(w http.ResponseWriter, _ *http.Request, id string) {
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	client, err := onvif.NewClient(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ONVIF connect: " + err.Error()})
		return
	}

	info, err := client.Dev.GetDeviceInformation(context.Background())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// -----------------------------------------------------------------------
// Imaging settings
// -----------------------------------------------------------------------

// handleCameraSettings handles GET/PUT /api/nvr/cameras/{id}/settings.
func (h *Handlers) handleCameraSettings(w http.ResponseWriter, r *http.Request, id string) {
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := onvif.GetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword, "")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, settings)

	case http.MethodPut:
		var req onvif.ImagingSettings
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		if err := onvif.SetImagingSettings(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword, "", &req); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -----------------------------------------------------------------------
// Relay outputs
// -----------------------------------------------------------------------

// handleCameraRelayOutputs handles GET /api/nvr/cameras/{id}/relay-outputs.
func (h *Handlers) handleCameraRelayOutputs(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	outputs, err := onvif.GetRelayOutputs(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if outputs == nil {
		outputs = []onvif.RelayOutput{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": outputs})
}

// -----------------------------------------------------------------------
// PTZ
// -----------------------------------------------------------------------

// handleCameraPTZPresets handles GET /api/nvr/cameras/{id}/ptz/presets.
func (h *Handlers) handleCameraPTZPresets(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	token := r.URL.Query().Get("profile_token")

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ONVIF connect: " + err.Error()})
		return
	}

	presets, err := ctrl.GetPresets(token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if presets == nil {
		presets = []onvif.PTZPreset{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": presets})
}

// handleCameraPTZStatus handles GET /api/nvr/cameras/{id}/ptz/status.
func (h *Handlers) handleCameraPTZStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	token := r.URL.Query().Get("profile_token")

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ONVIF connect: " + err.Error()})
		return
	}

	status, err := ctrl.GetStatus(token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// ptzMoveRequest is the body for POST /api/nvr/cameras/{id}/ptz.
type ptzMoveRequest struct {
	Mode         string  `json:"mode"` // "continuous" or "absolute" or "relative" or "stop" or "preset"
	ProfileToken string  `json:"profile_token"`
	PanSpeed     float64 `json:"pan_speed"`
	TiltSpeed    float64 `json:"tilt_speed"`
	ZoomSpeed    float64 `json:"zoom_speed"`
	PanPos       float64 `json:"pan_position"`
	TiltPos      float64 `json:"tilt_position"`
	ZoomPos      float64 `json:"zoom_position"`
	PresetToken  string  `json:"preset_token"`
}

// handleCameraPTZ handles POST /api/nvr/cameras/{id}/ptz.
func (h *Handlers) handleCameraPTZ(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	var req ptzMoveRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ONVIF connect: " + err.Error()})
		return
	}

	switch req.Mode {
	case "continuous":
		err = ctrl.ContinuousMove(req.ProfileToken, req.PanSpeed, req.TiltSpeed, req.ZoomSpeed)
	case "absolute":
		err = ctrl.AbsoluteMove(req.ProfileToken, req.PanPos, req.TiltPos, req.ZoomPos)
	case "relative":
		err = ctrl.RelativeMove(req.ProfileToken, req.PanPos, req.TiltPos, req.ZoomPos)
	case "stop":
		err = ctrl.Stop(req.ProfileToken)
	case "preset":
		err = ctrl.GotoPreset(req.ProfileToken, req.PresetToken)
	case "home":
		err = ctrl.GotoHome(req.ProfileToken)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown mode: " + req.Mode})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// -----------------------------------------------------------------------
// Audio capabilities
// -----------------------------------------------------------------------

// handleCameraAudioCapabilities handles GET /api/nvr/cameras/{id}/audio/capabilities.
func (h *Handlers) handleCameraAudioCapabilities(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	caps, err := onvif.GetAudioCapabilities(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, caps)
}

// -----------------------------------------------------------------------
// Media profiles and video sources
// -----------------------------------------------------------------------

// handleCameraMediaProfiles handles GET /api/nvr/cameras/{id}/media/profiles.
func (h *Handlers) handleCameraMediaProfiles(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	profiles, _, err := onvif.GetProfilesAuto(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if profiles == nil {
		profiles = []onvif.MediaProfile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": profiles})
}

// handleCameraMediaVideoSources handles GET /api/nvr/cameras/{id}/media/video-sources.
func (h *Handlers) handleCameraMediaVideoSources(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	sources, err := onvif.GetVideoSourcesList(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if sources == nil {
		sources = []*onvif.VideoSourceInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sources})
}

// -----------------------------------------------------------------------
// Device management sub-routes
// -----------------------------------------------------------------------

// handleCameraDeviceDatetime handles GET /api/nvr/cameras/{id}/device/datetime.
func (h *Handlers) handleCameraDeviceDatetime(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	info, err := onvif.GetSystemDateAndTime(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleCameraDeviceHostname handles GET /api/nvr/cameras/{id}/device/hostname.
func (h *Handlers) handleCameraDeviceHostname(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	info, err := onvif.GetDeviceHostname(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleCameraDeviceNetworkInterfaces handles GET /api/nvr/cameras/{id}/device/network/interfaces.
func (h *Handlers) handleCameraDeviceNetworkInterfaces(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	ifaces, err := onvif.GetNetworkInterfaces(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if ifaces == nil {
		ifaces = []*onvif.NetworkInterfaceInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ifaces})
}

// handleCameraDeviceNetworkProtocols handles GET /api/nvr/cameras/{id}/device/network/protocols.
func (h *Handlers) handleCameraDeviceNetworkProtocols(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	protos, err := onvif.GetNetworkProtocols(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if protos == nil {
		protos = []*onvif.NetworkProtocolInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": protos})
}

// handleCameraDeviceUsers handles GET /api/nvr/cameras/{id}/device/users.
func (h *Handlers) handleCameraDeviceUsers(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cam := h.cameraForOnvif(w, id)
	if cam == nil {
		return
	}

	users, err := onvif.GetDeviceUsers(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if users == nil {
		users = []*onvif.DeviceUser{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

// -----------------------------------------------------------------------
// ONVIF sub-route dispatcher (called from camerasSubrouter)
// -----------------------------------------------------------------------

// dispatchCameraONVIFSubroute routes camera ONVIF sub-paths.
// sub is the path after /api/nvr/cameras/{id}/, e.g. "ptz/presets".
func (h *Handlers) dispatchCameraONVIFSubroute(w http.ResponseWriter, r *http.Request, id, sub string) bool {
	switch {
	case sub == "device-info":
		h.handleCameraDeviceInfo(w, r, id)
		return true

	case sub == "settings":
		h.handleCameraSettings(w, r, id)
		return true

	case sub == "relay-outputs":
		h.handleCameraRelayOutputs(w, r, id)
		return true

	case sub == "ptz/presets":
		h.handleCameraPTZPresets(w, r, id)
		return true

	case sub == "ptz/status":
		h.handleCameraPTZStatus(w, r, id)
		return true

	case sub == "ptz":
		h.handleCameraPTZ(w, r, id)
		return true

	case sub == "audio/capabilities":
		h.handleCameraAudioCapabilities(w, r, id)
		return true

	case sub == "media/profiles":
		h.handleCameraMediaProfiles(w, r, id)
		return true

	case sub == "media/video-sources":
		h.handleCameraMediaVideoSources(w, r, id)
		return true

	case sub == "device/datetime":
		h.handleCameraDeviceDatetime(w, r, id)
		return true

	case sub == "device/hostname":
		h.handleCameraDeviceHostname(w, r, id)
		return true

	case strings.HasPrefix(sub, "device/network/interfaces"):
		h.handleCameraDeviceNetworkInterfaces(w, r, id)
		return true

	case strings.HasPrefix(sub, "device/network/protocols"):
		h.handleCameraDeviceNetworkProtocols(w, r, id)
		return true

	case sub == "device/users":
		h.handleCameraDeviceUsers(w, r, id)
		return true
	}

	return false
}
