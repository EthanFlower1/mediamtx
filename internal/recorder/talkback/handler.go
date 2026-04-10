package talkback

import (
	"encoding/json"
	"errors"
	"net/http"
)

// StartTalkbackRequest is the JSON body for POST /api/v1/talkback/start.
type StartTalkbackRequest struct {
	CameraID string `json:"camera_id"`
}

// StartTalkbackResponse is returned on success.
type StartTalkbackResponse struct {
	CameraID string `json:"camera_id"`
	Codec    string `json:"codec"`
	Message  string `json:"message"`
}

// StopTalkbackRequest is the JSON body for POST /api/v1/talkback/stop.
type StopTalkbackRequest struct {
	CameraID string `json:"camera_id"`
}

// UserIDExtractor pulls the authenticated user ID from the request context.
type UserIDExtractor func(r *http.Request) (string, bool)

// StartHandler returns an http.HandlerFunc for POST /api/v1/talkback/start.
func StartHandler(mgr *Manager, userID UserIDExtractor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		uid, ok := userID(r)
		if !ok || uid == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing user id")
			return
		}

		var req StartTalkbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.CameraID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "camera_id is required")
			return
		}

		sess, err := mgr.Start(r.Context(), StartRequest{
			CameraID: req.CameraID,
			UserID:   uid,
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrSessionExists):
				writeError(w, http.StatusConflict, "CONFLICT", "talkback session already active for this camera")
			case errors.Is(err, ErrBackchannelUnavailable):
				writeError(w, http.StatusUnprocessableEntity, "UNSUPPORTED", "camera does not support audio backchannel")
			default:
				writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to start talkback session")
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(StartTalkbackResponse{
			CameraID: sess.CameraID,
			Codec:    sess.codec,
			Message:  "talkback session started",
		})
	}
}

// StopHandler returns an http.HandlerFunc for POST /api/v1/talkback/stop.
func StopHandler(mgr *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var req StopTalkbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.CameraID == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "camera_id is required")
			return
		}

		if err := mgr.Stop(req.CameraID); err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "no active talkback session for this camera")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to stop talkback session")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ListHandler returns an http.HandlerFunc for GET /api/v1/talkback/sessions.
func ListHandler(mgr *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}

		sessions := mgr.ActiveSessions()
		if sessions == nil {
			sessions = []SessionInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
