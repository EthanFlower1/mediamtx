package timeline

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Handler returns an http.HandlerFunc for GET /api/v1/timeline.
//
// Query parameters:
//   - cameras: comma-separated camera IDs (required)
//   - start: RFC3339 start time (required)
//   - end: RFC3339 end time (required)
func Handler(assembler *Assembler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}

		q := r.URL.Query()

		camerasRaw := q.Get("cameras")
		if camerasRaw == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cameras parameter is required")
			return
		}
		cameras := splitAndTrim(camerasRaw)
		if len(cameras) == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cameras parameter must contain at least one camera ID")
			return
		}

		startStr := q.Get("start")
		if startStr == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "start parameter is required (RFC3339)")
			return
		}
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid start time: "+err.Error())
			return
		}

		endStr := q.Get("end")
		if endStr == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "end parameter is required (RFC3339)")
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid end time: "+err.Error())
			return
		}

		if !end.After(start) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "end must be after start")
			return
		}

		resp, err := assembler.Assemble(TimelineRequest{
			CameraIDs: cameras,
			Start:     start,
			End:       end,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to assemble timeline")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
