// Package streams provides the stream URL minting endpoint for the Directory.
// When a client wants to view a live stream or play back recorded footage, it
// calls POST /api/v1/streams/request with the camera ID and protocol. The
// Directory resolves which Recorder owns the camera and returns a time-limited,
// signed URL that the client uses to connect directly to the Recorder's
// Raikada instance.
package streams

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Protocol enumerates the supported streaming protocols.
type Protocol string

const (
	ProtocolRTSP      Protocol = "rtsp"
	ProtocolWebRTC    Protocol = "webrtc"
	ProtocolHLS       Protocol = "hls"
	ProtocolHLSLowLat Protocol = "hls-ll"
)

var validProtocols = map[Protocol]bool{
	ProtocolRTSP:      true,
	ProtocolWebRTC:    true,
	ProtocolHLS:       true,
	ProtocolHLSLowLat: true,
}

// StreamRequest is the JSON body for POST /api/v1/streams/request.
type StreamRequest struct {
	CameraID string   `json:"camera_id"`
	Protocol Protocol `json:"protocol"`
}

// StreamResponse is the JSON body returned with the minted stream URL.
type StreamResponse struct {
	URL       string `json:"url"`
	Protocol  string `json:"protocol"`
	ExpiresAt string `json:"expires_at"`
	Token     string `json:"token"`
}

// CameraResolver looks up which Recorder serves a camera and returns
// the Recorder's base URL and the camera's stream path.
type CameraResolver interface {
	Resolve(cameraID string) (recorderBaseURL, streamPath string, err error)
}

// URLSigner generates HMAC-SHA256 signed tokens for stream URLs.
type URLSigner struct {
	secret []byte
	ttl    time.Duration
}

// NewURLSigner creates a signer with the given HMAC secret and token TTL.
func NewURLSigner(secret []byte, ttl time.Duration) *URLSigner {
	return &URLSigner{secret: secret, ttl: ttl}
}

// Sign produces a time-limited token for the given stream path.
func (s *URLSigner) Sign(streamPath string, now time.Time) (token string, expiresAt time.Time) {
	expiresAt = now.Add(s.ttl)

	// Token payload: nonce:expires_epoch:path
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	nonceHex := hex.EncodeToString(nonce)

	payload := fmt.Sprintf("%s:%d:%s", nonceHex, expiresAt.Unix(), streamPath)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	token = payload + ":" + sig
	return token, expiresAt
}

// Verify checks that a token is valid and not expired.
func (s *URLSigner) Verify(token string) (streamPath string, err error) {
	// Parse: nonce:expires:path:sig
	// Find the last colon (sig separator)
	lastColon := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon < 0 {
		return "", fmt.Errorf("streams: malformed token")
	}

	payload := token[:lastColon]
	sig := token[lastColon+1:]

	// Verify HMAC.
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", fmt.Errorf("streams: invalid signature")
	}

	// Parse expiry from payload: nonce:expires:path
	var nonce string
	var expires int64
	var path string
	// Split payload into parts.
	firstColon := -1
	secondColon := -1
	for i, c := range payload {
		if c == ':' {
			if firstColon < 0 {
				firstColon = i
			} else if secondColon < 0 {
				secondColon = i
				break
			}
		}
	}
	if firstColon < 0 || secondColon < 0 {
		return "", fmt.Errorf("streams: malformed token payload")
	}
	nonce = payload[:firstColon]
	_ = nonce // nonce is for uniqueness, not checked

	expiresStr := payload[firstColon+1 : secondColon]
	path = payload[secondColon+1:]

	_, err = fmt.Sscanf(expiresStr, "%d", &expires)
	if err != nil {
		return "", fmt.Errorf("streams: invalid expiry in token")
	}

	if time.Now().Unix() > expires {
		return "", fmt.Errorf("streams: token expired")
	}

	return path, nil
}

// Handler returns an http.HandlerFunc for POST /api/v1/streams/request.
func Handler(resolver CameraResolver, signer *URLSigner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var req StreamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.CameraID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "camera_id is required")
			return
		}
		if !validProtocols[req.Protocol] {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				fmt.Sprintf("invalid protocol %q; valid: rtsp, webrtc, hls, hls-ll", req.Protocol))
			return
		}

		baseURL, streamPath, err := resolver.Resolve(req.CameraID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "camera not found or no recorder assigned")
			return
		}

		token, expiresAt := signer.Sign(streamPath, time.Now())

		// Build the full URL based on protocol.
		var url string
		switch req.Protocol {
		case ProtocolRTSP:
			url = fmt.Sprintf("rtsp://%s/%s?token=%s", baseURL, streamPath, token)
		case ProtocolWebRTC:
			url = fmt.Sprintf("https://%s/webrtc/%s?token=%s", baseURL, streamPath, token)
		case ProtocolHLS:
			url = fmt.Sprintf("https://%s/hls/%s/index.m3u8?token=%s", baseURL, streamPath, token)
		case ProtocolHLSLowLat:
			url = fmt.Sprintf("https://%s/hls/%s/index.m3u8?token=%s&_HLS_msn=0", baseURL, streamPath, token)
		}

		resp := StreamResponse{
			URL:       url,
			Protocol:  string(req.Protocol),
			ExpiresAt: expiresAt.Format(time.RFC3339),
			Token:     token,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
