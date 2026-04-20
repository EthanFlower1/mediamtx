package recorderapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/recordercontrol"
)

// Handlers groups the HTTP handlers for recorder management.
type Handlers struct {
	Store    *Store
	RCStore  *recordercontrol.Store
	EventBus *recordercontrol.EventBus
	Logger   *slog.Logger
}

// --- Registration & Heartbeat -----------------------------------------------

// RegisterHandler handles POST /api/v1/recorders/register.
// Called by recorders on startup in managed mode.
func (h *Handlers) RegisterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			RecorderID string `json:"recorder_id"`
			Hostname   string `json:"hostname"`
			ListenAddr string `json:"listen_addr"`
			Version    string `json:"version"`
			OS         string `json:"os"`
			Arch       string `json:"arch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if payload.RecorderID == "" {
			http.Error(w, `{"error":"recorder_id is required"}`, http.StatusBadRequest)
			return
		}

		rec := RecorderRow{
			ID:              payload.RecorderID,
			Name:            payload.Hostname,
			Hostname:        payload.Hostname,
			InternalAPIAddr: payload.ListenAddr,
			HealthStatus:    "healthy",
			LastCheckinAt:   time.Now().UTC(),
		}
		if err := h.Store.UpsertRecorder(r.Context(), rec); err != nil {
			h.Logger.Error("register recorder failed", "error", err)
			http.Error(w, `{"error":"registration failed"}`, http.StatusInternalServerError)
			return
		}

		// Generate a service token for this recorder.
		token, err := h.Store.SetServiceToken(r.Context(), payload.RecorderID)
		if err != nil {
			h.Logger.Error("generate service token failed", "error", err)
			http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
			return
		}

		h.Logger.Info("recorder registered",
			"recorder_id", payload.RecorderID,
			"hostname", payload.Hostname,
			"version", payload.Version,
		)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":        "registered",
			"recorder_id":   payload.RecorderID,
			"service_token": token,
		})
	}
}

// HeartbeatHandler handles POST /api/v1/recorders/heartbeat.
// Called periodically by recorders in managed mode.
func (h *Handlers) HeartbeatHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			RecorderID  string  `json:"recorder_id"`
			CameraCount int     `json:"camera_count"`
			DiskUsedPct float64 `json:"disk_used_pct"`
			UptimeSec   int64   `json:"uptime_sec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		status := "healthy"
		if payload.DiskUsedPct > 90 {
			status = "degraded"
		}

		if err := h.Store.UpdateHeartbeat(r.Context(), payload.RecorderID, status); err != nil {
			h.Logger.Warn("heartbeat update failed", "recorder_id", payload.RecorderID, "error", err)
			http.Error(w, `{"error":"heartbeat failed"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListRecordersHandler handles GET /api/v1/recorders.
func (h *Handlers) ListRecordersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := h.Store.ListRecorders(r.Context())
		if err != nil {
			http.Error(w, `{"error":"failed to list recorders"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": recs})
	}
}

// --- Camera CRUD (Priority 2) -----------------------------------------------

// CameraCreatePayload is the request body for creating a camera assignment.
type CameraCreatePayload struct {
	CameraID    string `json:"camera_id"`
	RecorderID  string `json:"recorder_id"`
	Name        string `json:"name"`
	StreamURL   string `json:"stream_url"`
	RecordMode  string `json:"record_mode"` // "always", "events", "off"
}

// CreateCameraHandler handles POST /api/v1/cameras.
func (h *Handlers) CreateCameraHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var p CameraCreatePayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if p.CameraID == "" || p.RecorderID == "" {
			http.Error(w, `{"error":"camera_id and recorder_id required"}`, http.StatusBadRequest)
			return
		}

		configJSON, _ := json.Marshal(map[string]string{
			"stream_url":  p.StreamURL,
			"record_mode": p.RecordMode,
		})

		row := recordercontrol.CameraRow{
			CameraID:      p.CameraID,
			RecorderID:    p.RecorderID,
			Name:          p.Name,
			ConfigJSON:    string(configJSON),
			ConfigVersion: 1,
		}
		if err := h.RCStore.InsertCamera(r.Context(), row); err != nil {
			h.Logger.Error("create camera failed", "error", err)
			http.Error(w, `{"error":"create camera failed"}`, http.StatusInternalServerError)
			return
		}

		// Notify the recorder via EventBus so StreamAssignments pushes it.
		h.EventBus.Publish(p.RecorderID, recordercontrol.BusEvent{
			Kind:   recordercontrol.EventKindCameraAdded,
			Camera: &row,
		})

		h.Logger.Info("camera created", "camera_id", p.CameraID, "recorder_id", p.RecorderID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "created",
			"camera_id": p.CameraID,
		})
	}
}

// ListCamerasHandler handles GET /api/v1/cameras?recorder_id=...
func (h *Handlers) ListCamerasHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recorderID := r.URL.Query().Get("recorder_id")
		if recorderID == "" {
			http.Error(w, `{"error":"recorder_id query param required"}`, http.StatusBadRequest)
			return
		}

		cams, err := h.RCStore.ListCamerasForRecorder(r.Context(), recorderID)
		if err != nil {
			http.Error(w, `{"error":"list cameras failed"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": cams})
	}
}

// DeleteCameraHandler handles DELETE /api/v1/cameras?camera_id=...
func (h *Handlers) DeleteCameraHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			http.Error(w, `{"error":"camera_id query param required"}`, http.StatusBadRequest)
			return
		}

		// Look up recorder before deleting so we can notify.
		cam, err := h.RCStore.GetCamera(r.Context(), cameraID)
		if err != nil {
			http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
			return
		}

		if _, err := h.RCStore.DeleteCamera(r.Context(), cameraID); err != nil {
			http.Error(w, `{"error":"delete camera failed"}`, http.StatusInternalServerError)
			return
		}

		h.EventBus.Publish(cam.RecorderID, recordercontrol.BusEvent{
			Kind: recordercontrol.EventKindCameraRemoved,
			Removal: &recordercontrol.RemovalPayload{
				CameraID: cameraID,
				Reason:   "admin_deleted",
			},
		})

		h.Logger.Info("camera deleted", "camera_id", cameraID, "recorder_id", cam.RecorderID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}
