package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/bluenviron/mediamtx/internal/recorder/backchannel"
	nvrCrypto "github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// --- WebSocket message types ---

type wsMessage struct {
	Type string `json:"type"`
}

type wsSessionStarted struct {
	Type       string `json:"type"`
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
}

type wsError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// BackchannelHandler implements HTTP/WebSocket endpoints for audio backchannel.
type BackchannelHandler struct {
	DB            *db.DB
	Manager       *backchannel.Manager
	EncryptionKey []byte
}

// decryptPassword decrypts a stored password. If the value does not have
// the "enc:" prefix it is returned unchanged (plaintext / legacy value).
func (h *BackchannelHandler) decryptPassword(stored string) string {
	if len(h.EncryptionKey) == 0 || stored == "" {
		return stored
	}
	if !strings.HasPrefix(stored, "enc:") {
		return stored
	}
	ct, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil {
		nvrLogWarn("backchannel", "failed to decode encrypted ONVIF password")
		return ""
	}
	pt, err := nvrCrypto.Decrypt(h.EncryptionKey, ct)
	if err != nil {
		nvrLogWarn("backchannel", "failed to decrypt ONVIF password")
		return ""
	}
	return string(pt)
}

// wsUpgrader upgrades HTTP connections to WebSocket.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocket handles the intercom WebSocket endpoint for a camera.
// Clients send text messages with {"type":"start"} or {"type":"stop"}, and
// binary messages containing raw audio payload.
func (h *BackchannelHandler) WebSocket(c *gin.Context) {
	id := c.Param("id")

	// Fetch and validate camera.
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}
	if !cam.SupportsAudioBackchannel {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera does not support audio backchannel"})
		return
	}

	// Upgrade to WebSocket.
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		nvrLogError("backchannel", "WebSocket upgrade failed", err)
		return
	}
	defer conn.Close()

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			// Client disconnected — stop any active session.
			if h.Manager != nil {
				_ = h.Manager.StopSession(id)
			}
			return
		}

		switch msgType {
		case websocket.TextMessage:
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("[NVR] [WARN] [backchannel] failed to parse WS text message: %v", err)
				continue
			}

			switch msg.Type {
			case "start":
				if h.Manager == nil {
					sendWSError(conn, "backchannel manager not initialised")
					continue
				}
				info, err := h.Manager.StartSession(context.Background(), id)
				if err != nil {
					sendWSError(conn, err.Error())
					continue
				}
				resp := wsSessionStarted{
					Type:       "session_started",
					Codec:      info.Codec,
					SampleRate: info.SampleRate,
					Bitrate:    info.Bitrate,
				}
				if err := conn.WriteJSON(resp); err != nil {
					nvrLogError("backchannel", "failed to write session_started message", err)
					return
				}

			case "stop":
				if h.Manager != nil {
					_ = h.Manager.StopSession(id)
				}
				if err := conn.WriteJSON(wsMessage{Type: "session_stopped"}); err != nil {
					nvrLogError("backchannel", "failed to write session_stopped message", err)
					return
				}

			default:
				nvrLogWarn("backchannel", "unknown WS message type: "+msg.Type)
			}

		case websocket.BinaryMessage:
			if h.Manager == nil {
				continue
			}
			if err := h.Manager.SendAudio(id, data); err != nil {
				nvrLogWarn("backchannel", "SendAudio: "+err.Error())
			}
		}
	}
}

// Info returns backchannel capability info for a camera, including the
// negotiated codec and decoder options.
func (h *BackchannelHandler) Info(c *gin.Context) {
	id := c.Param("id")

	if h.DB == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}

	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusOK, gin.H{"has_backchannel": false})
		return
	}

	xaddr := cam.ONVIFEndpoint
	user := cam.ONVIFUsername
	pass := h.decryptPassword(cam.ONVIFPassword)

	caps, err := onvif.GetAudioCapabilities(xaddr, user, pass)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get audio capabilities", err)
		return
	}

	if !caps.HasBackchannel {
		c.JSON(http.StatusOK, gin.H{"has_backchannel": false})
		return
	}

	// Gather richer decoder info.
	decoderCfgs, err := onvif.GetAudioDecoderConfigs(xaddr, user, pass)
	if err != nil {
		nvrLogError("backchannel", "failed to get audio decoder configs", err)
	}

	var decoderToken string
	if len(decoderCfgs) > 0 {
		decoderToken = decoderCfgs[0].Token
	}

	opts, err := onvif.GetAudioDecoderOpts(xaddr, user, pass, decoderToken)
	if err != nil {
		nvrLogError("backchannel", "failed to get audio decoder options", err)
	}

	negotiated := onvif.NegotiateCodec(opts)

	c.JSON(http.StatusOK, gin.H{
		"has_backchannel":  true,
		"audio_outputs":    caps.AudioOutputs,
		"negotiated_codec": negotiated,
		"decoder_options":  opts,
	})
}

// GetAudioOutputs returns the list of audio output tokens for a camera.
func (h *BackchannelHandler) GetAudioOutputs(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	tokens, err := onvif.GetAudioOutputs(cam.ONVIFEndpoint, user, pass)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get audio outputs", err)
		return
	}
	if tokens == nil {
		tokens = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

// GetAudioOutputConfigs returns all audio output configurations for a camera.
func (h *BackchannelHandler) GetAudioOutputConfigs(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	cfgs, err := onvif.GetAudioOutputConfigs(cam.ONVIFEndpoint, user, pass)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get audio output configs", err)
		return
	}
	if cfgs == nil {
		cfgs = []*onvif.AudioOutputConfig{}
	}
	c.JSON(http.StatusOK, cfgs)
}

// UpdateAudioOutputConfig updates an audio output configuration on a camera.
func (h *BackchannelHandler) UpdateAudioOutputConfig(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	token := c.Param("token")

	var cfg onvif.AudioOutputConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		apiError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	cfg.Token = token

	if err := onvif.SetAudioOutputConfig(cam.ONVIFEndpoint, user, pass, &cfg); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update audio output config", err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// GetAudioDecoderConfigs returns all audio decoder configurations for a camera.
func (h *BackchannelHandler) GetAudioDecoderConfigs(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	cfgs, err := onvif.GetAudioDecoderConfigs(cam.ONVIFEndpoint, user, pass)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get audio decoder configs", err)
		return
	}
	if cfgs == nil {
		cfgs = []*onvif.AudioDecoderConfig{}
	}
	c.JSON(http.StatusOK, cfgs)
}

// UpdateAudioDecoderConfig updates an audio decoder configuration on a camera.
func (h *BackchannelHandler) UpdateAudioDecoderConfig(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	token := c.Param("token")

	var cfg onvif.AudioDecoderConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		apiError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	cfg.Token = token

	if err := onvif.SetAudioDecoderConfig(cam.ONVIFEndpoint, user, pass, &cfg); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update audio decoder config", err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// GetAudioDecoderOptions returns the audio decoder options for a given token.
func (h *BackchannelHandler) GetAudioDecoderOptions(c *gin.Context) {
	cam, user, pass, ok := h.loadCamera(c)
	if !ok {
		return
	}
	token := c.Param("token")

	opts, err := onvif.GetAudioDecoderOpts(cam.ONVIFEndpoint, user, pass, token)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get audio decoder options", err)
		return
	}
	c.JSON(http.StatusOK, opts)
}

// --- helpers ---

// loadCamera fetches and validates a camera by ":id" param.
// It writes an error response and returns ok=false on failure.
func (h *BackchannelHandler) loadCamera(c *gin.Context) (cam *db.Camera, user, pass string, ok bool) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return nil, "", "", false
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return nil, "", "", false
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return nil, "", "", false
	}
	return cam, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), true
}

// sendWSError writes a JSON error message over the WebSocket connection.
func sendWSError(conn *websocket.Conn, msg string) {
	err := conn.WriteJSON(wsError{Type: "error", Message: msg})
	if err != nil {
		log.Printf("[NVR] [WARN] [backchannel] failed to send WS error: %v", err)
	}
}
