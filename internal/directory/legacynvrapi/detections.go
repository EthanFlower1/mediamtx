package legacynvrapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// -----------------------------------------------------------------------
// DETECTION ZONES  —  /api/nvr/cameras/{id}/zones
// -----------------------------------------------------------------------

// cameraZonesHandler dispatches zone collection requests for a specific camera.
// Called from camerasSubrouter when subResource == "zones".
func (h *Handlers) cameraZonesHandler(w http.ResponseWriter, r *http.Request, cameraID string) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}
	switch r.Method {
	case http.MethodGet:
		zones, err := h.RecDB.ListDetectionZones(cameraID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if zones == nil {
			zones = []*recdb.DetectionZone{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": zones})

	case http.MethodPost:
		var z recdb.DetectionZone
		if err := readJSON(r, &z); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		z.CameraID = cameraID
		if err := h.RecDB.CreateDetectionZone(&z); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, z)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// zonesSubrouter handles /api/nvr/zones/{zoneId}  (PUT, DELETE).
func (h *Handlers) zonesSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}

	id := pathID(r.URL.Path)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing zone id"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		existing, err := h.RecDB.GetDetectionZone(id)
		if err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "zone not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var patch recdb.DetectionZone
		if err := readJSON(r, &patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		// Merge patch into existing record.
		existing.ID = id
		if patch.Name != "" {
			existing.Name = patch.Name
		}
		if len(patch.Points) > 0 {
			existing.Points = patch.Points
		}
		if len(patch.ClassFilter) > 0 {
			existing.ClassFilter = patch.ClassFilter
		}
		existing.Enabled = patch.Enabled
		if err := h.RecDB.UpdateDetectionZone(existing); err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "zone not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		if err := h.RecDB.DeleteDetectionZone(id); err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "zone not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -----------------------------------------------------------------------
// RECORDING RULES  —  /api/nvr/cameras/{id}/recording-rules
// -----------------------------------------------------------------------

// cameraRecordingRulesHandler dispatches recording-rule collection requests.
// Called from camerasSubrouter when subResource == "recording-rules".
func (h *Handlers) cameraRecordingRulesHandler(w http.ResponseWriter, r *http.Request, cameraID string) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules, err := h.RecDB.ListRecordingRules(cameraID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if rules == nil {
			rules = []*recdb.RecordingRule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": rules})

	case http.MethodPost:
		var rule recdb.RecordingRule
		if err := readJSON(r, &rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		rule.CameraID = cameraID
		if err := h.RecDB.CreateRecordingRule(&rule); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rule)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// recordingRulesSubrouter handles /api/nvr/recording-rules/{ruleId}  (PUT, DELETE).
func (h *Handlers) recordingRulesSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}

	id := pathID(r.URL.Path)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing rule id"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		existing, err := h.RecDB.GetRecordingRule(id)
		if err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording rule not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var patch recdb.RecordingRule
		if err := readJSON(r, &patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		// Merge patch into existing record, preserving ID and camera.
		existing.ID = id
		if patch.Name != "" {
			existing.Name = patch.Name
		}
		if patch.StreamID != "" {
			existing.StreamID = patch.StreamID
		}
		if patch.TemplateID != "" {
			existing.TemplateID = patch.TemplateID
		}
		if patch.Mode != "" {
			existing.Mode = patch.Mode
		}
		if patch.Days != "" {
			existing.Days = patch.Days
		}
		if patch.StartTime != "" {
			existing.StartTime = patch.StartTime
		}
		if patch.EndTime != "" {
			existing.EndTime = patch.EndTime
		}
		if patch.PostEventSeconds != 0 {
			existing.PostEventSeconds = patch.PostEventSeconds
		}
		existing.Enabled = patch.Enabled
		if err := h.RecDB.UpdateRecordingRule(existing); err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording rule not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		if err := h.RecDB.DeleteRecordingRule(id); err != nil {
			if errors.Is(err, recdb.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording rule not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -----------------------------------------------------------------------
// TRACKING  —  /api/nvr/detections/{id}/track, /api/nvr/tracks
// -----------------------------------------------------------------------

// detectionsSubrouter handles /api/nvr/detections/{id}/track  (POST).
func (h *Handlers) detectionsSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}

	// Expect path: /api/nvr/detections/{id}/track
	sub := strings.TrimPrefix(r.URL.Path, "/api/nvr/detections/")
	sub = strings.TrimSuffix(sub, "/")
	parts := strings.SplitN(sub, "/", 2)

	if len(parts) != 2 || parts[1] != "track" {
		h.notImplemented(w, r)
		return
	}

	detectionIDStr := parts[0]
	detectionID, err := strconv.ParseInt(detectionIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid detection id"})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Label     string          `json:"label"`
		Status    string          `json:"status"`
		Sightings []recdb.Sighting `json:"sightings"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	track := &recdb.Track{
		Label:       body.Label,
		Status:      body.Status,
		DetectionID: detectionID,
	}
	if err := h.RecDB.InsertTrack(track); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for i := range body.Sightings {
		s := body.Sightings[i]
		s.TrackID = track.ID
		if err := h.RecDB.InsertSighting(&s); err != nil {
			// Continue inserting remaining sightings — partial success is acceptable.
			continue
		}
		body.Sightings[i] = s
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"track":    track,
		"sightings": body.Sightings,
	})
}

// tracks handles GET /api/nvr/tracks  (collection).
func (h *Handlers) tracks(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}

	items, err := h.RecDB.ListTracks(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*recdb.TrackWithSightings{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// tracksSubrouter handles GET /api/nvr/tracks/{id}
func (h *Handlers) tracksSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}

	idStr := pathID(r.URL.Path)
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing track id"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid track id"})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tw, err := h.RecDB.GetTrackWithSightings(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tw)
}

// -----------------------------------------------------------------------
// SEARCH  —  GET /api/nvr/search
// -----------------------------------------------------------------------

// searchDetections handles GET /api/nvr/search
// Query params: camera_id, start (RFC3339), end (RFC3339), type, limit
func (h *Handlers) searchDetections(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "recorder database not available",
		})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	cameraID := q.Get("camera_id")

	start, end, err := parseTimeRange(q.Get("start"), q.Get("end"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	detections, err := h.RecDB.QueryDetectionsByTimeRange(cameraID, start, end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if detections == nil {
		detections = []*recdb.Detection{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": detections})
}
