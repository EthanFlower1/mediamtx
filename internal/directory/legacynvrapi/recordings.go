package legacynvrapi

import (
	"net/http"
	"time"
)

// recordingsCollection handles GET /api/nvr/recordings
// Query params: camera_id, start (RFC3339), end (RFC3339)
func (h *Handlers) recordingsCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	q := r.URL.Query()
	cameraID := q.Get("camera_id")

	start, end, err := parseTimeRange(q.Get("start"), q.Get("end"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	recs, err := h.RecDB.GetRecordingsByFilter(cameraID, &start, &end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if recs == nil {
		recs = nil // let JSON encode as null — frontend handles both null and []
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": recs,
		"total": len(recs),
	})
}

// recordingsStats handles GET /api/nvr/recordings/stats
func (h *Handlers) recordingsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	stats, err := h.RecDB.GetStoragePerCamera()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": stats})
}

// cameraDetectionsHandler handles GET /api/nvr/cameras/{id}/detections
// Returns recordings that overlap the time range as a proxy for detection activity.
func (h *Handlers) cameraDetectionsHandler(w http.ResponseWriter, r *http.Request, cameraID string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	q := r.URL.Query()
	start, end, err := parseTimeRange(q.Get("start"), q.Get("end"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	recs, err := h.RecDB.QueryRecordings(cameraID, start, end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     recs,
		"camera_id": cameraID,
	})
}

// timelineMulti handles GET /api/nvr/timeline/multi
// Query params: camera_ids (comma-separated), start (RFC3339), end (RFC3339)
func (h *Handlers) timelineMulti(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	q := r.URL.Query()
	cameraIDsRaw := q.Get("camera_ids")
	cameraIDs := splitCommaStr(cameraIDsRaw)

	start, end, err := parseTimeRange(q.Get("start"), q.Get("end"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	type cameraTimeline struct {
		CameraID string `json:"camera_id"`
		Ranges   any    `json:"ranges"`
	}

	results := make([]cameraTimeline, 0, len(cameraIDs))
	for _, camID := range cameraIDs {
		ranges, tlErr := h.RecDB.GetTimeline(camID, start, end)
		if tlErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tlErr.Error()})
			return
		}
		results = append(results, cameraTimeline{CameraID: camID, Ranges: ranges})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

// parseTimeRange parses optional RFC3339 start/end strings.
// Defaults: start = now-24h, end = now.
func parseTimeRange(startStr, endStr string) (start, end time.Time, err error) {
	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return start, end, err
		}
	} else {
		start = time.Now().Add(-24 * time.Hour)
	}
	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return start, end, err
		}
	} else {
		end = time.Now()
	}
	return start, end, nil
}

// splitCommaStr splits a comma-separated string, dropping empty entries.
func splitCommaStr(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := trimSpaces(s[start:i])
			if part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	return result
}

func trimSpaces(s string) string {
	lo, hi := 0, len(s)
	for lo < hi && (s[lo] == ' ' || s[lo] == '\t') {
		lo++
	}
	for hi > lo && (s[hi-1] == ' ' || s[hi-1] == '\t') {
		hi--
	}
	return s[lo:hi]
}
