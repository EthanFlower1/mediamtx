package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// StreamHandler implements HTTP endpoints for camera stream management.
type StreamHandler struct {
	DB         *db.DB
	APIAddress string // MediaMTX API address for live track info
}

// streamRequest is the JSON body for creating or updating a camera stream.
type streamRequest struct {
	Name         string `json:"name" binding:"required"`
	RTSPURL      string `json:"rtsp_url" binding:"required"`
	ProfileToken string `json:"profile_token"`
	VideoCodec   string `json:"video_codec"`
	AudioCodec   string `json:"audio_codec"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Roles        string `json:"roles"`
}

// enrichedStream adds live track info to a DB stream.
type enrichedStream struct {
	*db.CameraStream
	LiveVideoCodec string `json:"live_video_codec,omitempty"`
	LiveAudioCodec string `json:"live_audio_codec,omitempty"`
	LiveWidth      int    `json:"live_width,omitempty"`
	LiveHeight     int    `json:"live_height,omitempty"`
}

// List returns all streams for a camera, enriched with live codec info.
func (h *StreamHandler) List(c *gin.Context) {
	cameraID := c.Param("id")

	streams, err := h.DB.ListCameraStreams(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list camera streams", err)
		return
	}
	if streams == nil {
		streams = []*db.CameraStream{}
	}

	cam, _ := h.DB.GetCamera(cameraID)
	tracksByPath := h.fetchLiveTracks()

	result := make([]enrichedStream, len(streams))
	for i, s := range streams {
		result[i] = enrichedStream{CameraStream: s}
		if cam == nil || tracksByPath == nil {
			continue
		}
		// Match stream to its MediaMTX path.
		path := cam.MediaMTXPath
		if s.ID != "" {
			prefix := s.ID
			if len(prefix) > 8 {
				prefix = prefix[:8]
			}
			altPath := cam.MediaMTXPath + "~" + prefix
			if _, ok := tracksByPath[altPath]; ok {
				path = altPath
			}
		}
		if tracks, ok := tracksByPath[path]; ok {
			for _, t := range tracks {
				codec := strings.ToUpper(string(t.Codec))
				if isVideoCodec(codec) {
					result[i].LiveVideoCodec = codec
					if props, ok := t.Props.(map[string]interface{}); ok {
						if w, ok := props["width"].(float64); ok {
							result[i].LiveWidth = int(w)
						}
						if h, ok := props["height"].(float64); ok {
							result[i].LiveHeight = int(h)
						}
					}
				} else if isAudioCodec(codec) {
					result[i].LiveAudioCodec = codec
				}
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

type liveTrack struct {
	Codec string      `json:"codec"`
	Props interface{} `json:"codecProps"`
}

func (h *StreamHandler) fetchLiveTracks() map[string][]liveTrack {
	if h.APIAddress == "" {
		return nil
	}
	addr := h.APIAddress
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/v3/paths/list", addr))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var result struct {
		Items []struct {
			Name    string      `json:"name"`
			Tracks2 []liveTrack `json:"tracks2"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	m := make(map[string][]liveTrack, len(result.Items))
	for _, item := range result.Items {
		m[item.Name] = item.Tracks2
	}
	return m
}

func isVideoCodec(c string) bool {
	switch c {
	case "H264", "H265", "AV1", "VP9", "VP8", "MJPEG":
		return true
	}
	return false
}

func isAudioCodec(c string) bool {
	switch c {
	case "OPUS", "MPEG4AUDIO", "G711", "AC3", "LPCM", "G722":
		return true
	}
	return false
}

// validateStreamURL checks that rawURL is a valid rtsp:// or rtsps:// URL.
func validateStreamURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("rtsp_url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "rtsp" && scheme != "rtsps" {
		return fmt.Errorf("URL scheme must be rtsp:// or rtsps://, got %q", u.Scheme)
	}
	return nil
}

// Create creates a new stream for a camera.
func (h *StreamHandler) Create(c *gin.Context) {
	cameraID := c.Param("id")

	// Verify camera exists.
	if _, err := h.DB.GetCamera(cameraID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to look up camera", err)
		return
	}

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if err := validateStreamURL(req.RTSPURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stream := &db.CameraStream{
		CameraID:     cameraID,
		Name:         req.Name,
		RTSPURL:      req.RTSPURL,
		ProfileToken: req.ProfileToken,
		VideoCodec:   req.VideoCodec,
		AudioCodec:   req.AudioCodec,
		Width:        req.Width,
		Height:       req.Height,
		Roles:        req.Roles,
	}

	if err := h.DB.CreateCameraStream(stream); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create camera stream", err)
		return
	}

	c.JSON(http.StatusCreated, stream)
}

// Update updates an existing stream.
func (h *StreamHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetCameraStream(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve stream", err)
		return
	}

	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	existing.Name = req.Name
	existing.RTSPURL = req.RTSPURL
	existing.ProfileToken = req.ProfileToken
	existing.VideoCodec = req.VideoCodec
	existing.AudioCodec = req.AudioCodec
	existing.Width = req.Width
	existing.Height = req.Height
	existing.Roles = req.Roles

	if err := h.DB.UpdateCameraStream(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update stream", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// UpdateRoles updates only the roles of an existing stream.
func (h *StreamHandler) UpdateRoles(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetCameraStream(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve stream", err)
		return
	}

	var req struct {
		Roles string `json:"roles" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	existing.Roles = req.Roles
	if err := h.DB.UpdateCameraStream(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update stream roles", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a stream by ID.
func (h *StreamHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.DeleteCameraStream(id); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete stream", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "stream deleted"})
}

// streamRetentionRequest is the JSON body for updating a stream's retention policy.
type streamRetentionRequest struct {
	RetentionDays      int `json:"retention_days"`
	EventRetentionDays int `json:"event_retention_days"`
}

// UpdateRetention updates retention settings for a specific stream.
func (h *StreamHandler) UpdateRetention(c *gin.Context) {
	id := c.Param("id")

	var req streamRetentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.RetentionDays < 0 || req.EventRetentionDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention days must be >= 0"})
		return
	}

	if err := h.DB.UpdateStreamRetention(id, req.RetentionDays, req.EventRetentionDays); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update stream retention", err)
		return
	}

	stream, err := h.DB.GetCameraStream(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve stream", err)
		return
	}

	c.JSON(http.StatusOK, stream)
}
