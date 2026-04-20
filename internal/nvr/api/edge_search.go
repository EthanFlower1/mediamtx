package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	nvrCrypto "github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// EdgeSearchHandler implements HTTP endpoints for ONVIF Profile G
// edge search operations (recordings and events stored on camera SD cards).
type EdgeSearchHandler struct {
	DB            *db.DB
	EncryptionKey []byte // AES-256 key for decrypting ONVIF credentials
}

// Recordings handles GET /api/nvr/edge-search/recordings?camera_id=X&start=...&end=...&recording_token=...
//
// Query parameters:
//   - camera_id: camera ID (required)
//   - start: start time in RFC3339 format (optional)
//   - end: end time in RFC3339 format (optional)
//   - recording_token: filter to a specific recording token (optional)
func (h *EdgeSearchHandler) Recordings(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	filter, err := parseEdgeSearchFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)

	recordings, err := onvif.FindRecordingsFiltered(cam.ONVIFEndpoint, cam.ONVIFUsername, password, filter)
	if err != nil {
		nvrLogError("edge-search", fmt.Sprintf("failed to search recordings for camera %s", cameraID), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to search recordings on device"})
		return
	}

	if recordings == nil {
		recordings = []onvif.EdgeRecording{}
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":  cameraID,
		"count":      len(recordings),
		"recordings": recordings,
	})
}

// Events handles GET /api/nvr/edge-search/events?camera_id=X&start=...&end=...&recording_token=...&event_type=...
//
// Query parameters:
//   - camera_id: camera ID (required)
//   - start: start time in RFC3339 format (optional)
//   - end: end time in RFC3339 format (optional)
//   - recording_token: filter to events from a specific recording (optional)
//   - event_type: ONVIF topic expression filter (optional, e.g. "tns1:VideoSource/MotionAlarm")
func (h *EdgeSearchHandler) Events(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id query parameter is required"})
		return
	}

	cam, err := h.DB.GetCamera(cameraID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	filter, err := parseEdgeSearchFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)

	events, err := onvif.FindEvents(cam.ONVIFEndpoint, cam.ONVIFUsername, password, filter)
	if err != nil {
		nvrLogError("edge-search", fmt.Sprintf("failed to search events for camera %s", cameraID), err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to search events on device"})
		return
	}

	if events == nil {
		events = []onvif.EdgeEvent{}
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": cameraID,
		"count":     len(events),
		"events":    events,
	})
}

// parseEdgeSearchFilter extracts EdgeSearchFilter fields from query parameters.
func parseEdgeSearchFilter(c *gin.Context) (*onvif.EdgeSearchFilter, error) {
	filter := &onvif.EdgeSearchFilter{}
	hasFilter := false

	if s := c.Query("start"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid 'start' time format, use RFC3339")
		}
		filter.StartTime = &parsed
		hasFilter = true
	}

	if e := c.Query("end"); e != "" {
		parsed, err := time.Parse(time.RFC3339, e)
		if err != nil {
			return nil, fmt.Errorf("invalid 'end' time format, use RFC3339")
		}
		filter.EndTime = &parsed
		hasFilter = true
	}

	if rt := c.Query("recording_token"); rt != "" {
		filter.RecordingToken = rt
		hasFilter = true
	}

	if et := c.Query("event_type"); et != "" {
		filter.EventType = et
		hasFilter = true
	}

	if !hasFilter {
		return nil, nil
	}
	return filter, nil
}

func (h *EdgeSearchHandler) decryptPassword(stored string) string {
	if len(h.EncryptionKey) == 0 || stored == "" {
		return stored
	}
	if !strings.HasPrefix(stored, "enc:") {
		return stored
	}
	ct, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil {
		return ""
	}
	pt, err := nvrCrypto.Decrypt(h.EncryptionKey, ct)
	if err != nil {
		return ""
	}
	return string(pt)
}
