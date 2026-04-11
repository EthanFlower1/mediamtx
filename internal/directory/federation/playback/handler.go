package playback

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// streamURLRequest is the JSON body for POST /api/v1/federation/streams/request.
type streamURLRequest struct {
	CameraID          string `json:"camera_id"`
	RequestedKind     uint32 `json:"requested_kind"`
	PreferredProtocol string `json:"preferred_protocol"`
	ClientIP          string `json:"client_ip,omitempty"`
	MaxTTLSeconds     int32  `json:"max_ttl_seconds,omitempty"`
	// Playback fields (optional, for playback tokens).
	PlaybackStartRFC3339 string `json:"playback_start,omitempty"`
	PlaybackEndRFC3339   string `json:"playback_end,omitempty"`
}

// streamURLResponse is the JSON response for a successful delegation.
type streamURLResponse struct {
	URL         string `json:"url"`
	GrantedKind uint32 `json:"granted_kind"`
	PeerID      string `json:"peer_id"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// UserExtractor returns the authenticated user_id from the request.
// The Directory wires this to the JWT/session middleware.
type UserExtractor func(r *http.Request) (userID string, ok bool)

// protocolMap maps JSON string values to StreamProtocol enum values.
var protocolMap = map[string]kaivuev1.StreamProtocol{
	"webrtc": kaivuev1.StreamProtocol_STREAM_PROTOCOL_WEBRTC,
	"hls":    kaivuev1.StreamProtocol_STREAM_PROTOCOL_HLS,
	"rtsp":   kaivuev1.StreamProtocol_STREAM_PROTOCOL_RTSP,
	"mp4":    kaivuev1.StreamProtocol_STREAM_PROTOCOL_MP4,
	"jpeg":   kaivuev1.StreamProtocol_STREAM_PROTOCOL_JPEG,
}

// Handler returns an http.HandlerFunc for POST /api/v1/federation/streams/request.
// It delegates cross-site stream URL minting to the PlaybackDelegator.
func Handler(delegator *Delegator, extractUser UserExtractor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody("METHOD_NOT_ALLOWED", "use POST"))
			return
		}

		userID, ok := extractUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorBody("UNAUTHENTICATED", "valid authentication required"))
			return
		}

		var req streamURLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "invalid JSON body"))
			return
		}

		if req.CameraID == "" {
			writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "camera_id is required"))
			return
		}
		if req.RequestedKind == 0 {
			writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "requested_kind is required"))
			return
		}

		proto := kaivuev1.StreamProtocol_STREAM_PROTOCOL_UNSPECIFIED
		if req.PreferredProtocol != "" {
			p, found := protocolMap[req.PreferredProtocol]
			if !found {
				writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "invalid preferred_protocol; valid: webrtc, hls, rtsp, mp4, jpeg"))
				return
			}
			proto = p
		}

		var playbackRange *kaivuev1.PlaybackRange
		if req.PlaybackStartRFC3339 != "" && req.PlaybackEndRFC3339 != "" {
			startT, err := time.Parse(time.RFC3339, req.PlaybackStartRFC3339)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "invalid playback_start; use RFC3339"))
				return
			}
			endT, err := time.Parse(time.RFC3339, req.PlaybackEndRFC3339)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody("BAD_REQUEST", "invalid playback_end; use RFC3339"))
				return
			}
			playbackRange = &kaivuev1.PlaybackRange{
				Start: timestamppb.New(startT),
				End:   timestamppb.New(endT),
			}
		}

		delegateReq := DelegateRequest{
			CameraID:          req.CameraID,
			RequestedKind:     req.RequestedKind,
			PreferredProtocol: proto,
			PlaybackRange:     playbackRange,
			ClientIP:          req.ClientIP,
			MaxTTLSeconds:     req.MaxTTLSeconds,
			UserID:            userID,
		}

		resp, err := delegator.Delegate(r.Context(), delegateReq)
		if err != nil {
			handleDelegateError(w, err)
			return
		}

		out := streamURLResponse{
			URL:         resp.URL,
			GrantedKind: resp.GrantedKind,
			PeerID:      resp.PeerID,
		}
		if resp.Claims != nil && resp.Claims.ExpiresAt != nil {
			out.ExpiresAt = resp.Claims.ExpiresAt.AsTime().Format(time.RFC3339)
		}

		writeJSON(w, http.StatusCreated, out)
	}
}

// handleDelegateError maps Delegator sentinel errors to HTTP status codes.
func handleDelegateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCameraNotFound):
		writeJSON(w, http.StatusNotFound, errorBody("CAMERA_NOT_FOUND", err.Error()))
	case errors.Is(err, ErrPermissionDenied):
		writeJSON(w, http.StatusForbidden, errorBody("PERMISSION_DENIED", err.Error()))
	case errors.Is(err, ErrPeerUnreachable):
		writeJSON(w, http.StatusBadGateway, errorBody("PEER_UNREACHABLE", err.Error()))
	case errors.Is(err, ErrPeerInternal):
		writeJSON(w, http.StatusBadGateway, errorBody("PEER_ERROR", err.Error()))
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody("INTERNAL", "unexpected error"))
	}
}

func errorBody(code, message string) map[string]string {
	return map[string]string{"code": code, "message": message}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
