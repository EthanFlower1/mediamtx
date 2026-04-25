package legacynvrapi

import (
	"errors"
	"net/http"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// -----------------------------------------------------------------------
// TOURS  —  /api/nvr/tours
// -----------------------------------------------------------------------

// toursCollection handles GET /api/nvr/tours and POST /api/nvr/tours.
func (h *Handlers) toursCollection(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.RecDB.ListTours()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if items == nil {
			items = []recdb.Tour{}
		}
		writeJSON(w, http.StatusOK, items)

	case http.MethodPost:
		var body struct {
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			CameraIDs    []string `json:"camera_ids"`
			DwellSeconds int      `json:"dwell_seconds"`
			Transition   string   `json:"transition"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if body.DwellSeconds <= 0 {
			body.DwellSeconds = 10 // sensible default
		}
		tour, err := h.RecDB.CreateTour(body.Name, body.CameraIDs, body.DwellSeconds)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, tour)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// toursSubrouter handles /api/nvr/tours/{id}  (PUT, DELETE).
func (h *Handlers) toursSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	id := pathID(r.URL.Path)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing tour id"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		existing, err := h.RecDB.GetTour(id)
		if err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "tour not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var body struct {
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			CameraIDs    []string `json:"camera_ids"`
			DwellSeconds int      `json:"dwell_seconds"`
			Transition   string   `json:"transition"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		// Merge — keep existing values when patch field is zero/empty.
		name := existing.Name
		if body.Name != "" {
			name = body.Name
		}
		cameraIDs := existing.CameraIDs
		if len(body.CameraIDs) > 0 {
			cameraIDs = body.CameraIDs
		}
		dwellSeconds := existing.DwellSeconds
		if body.DwellSeconds > 0 {
			dwellSeconds = body.DwellSeconds
		}
		if err := h.RecDB.UpdateTour(id, name, cameraIDs, dwellSeconds); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		updated, err := h.RecDB.GetTour(id)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := h.RecDB.DeleteTour(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -----------------------------------------------------------------------
// SAVED CLIPS  —  /api/nvr/saved-clips
// -----------------------------------------------------------------------

// savedClips handles GET /api/nvr/saved-clips.
// Query param: camera_id (optional)
func (h *Handlers) savedClips(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cameraID := r.URL.Query().Get("camera_id")
	items, err := h.RecDB.ListSavedClips(cameraID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*recdb.SavedClip{}
	}
	writeJSON(w, http.StatusOK, items)
}
