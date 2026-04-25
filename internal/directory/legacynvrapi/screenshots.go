package legacynvrapi

import (
	"errors"
	"net/http"
	"strconv"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// -----------------------------------------------------------------------
// SCREENSHOTS
// -----------------------------------------------------------------------

// screenshotsCollection handles GET /api/nvr/screenshots
// Query params: camera_id, page, per_page, sort ("asc"/"desc")
func (h *Handlers) screenshotsCollection(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	cameraID := q.Get("camera_id")
	sort := q.Get("sort")

	page := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	perPage := 20
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	// Support "limit" as an alias for per_page used by Flutter client.
	if q.Get("per_page") == "" {
		if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
			perPage = v
		}
	}

	items, total, err := h.RecDB.ListScreenshots(cameraID, sort, page, perPage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*recdb.Screenshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"screenshots": items,
		"total":       total,
		"page":        page,
	})
}

// screenshotsSubrouter handles /api/nvr/screenshots/{id}  (DELETE).
func (h *Handlers) screenshotsSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	idStr := pathID(r.URL.Path)
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing screenshot id"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid screenshot id"})
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if err := h.RecDB.DeleteScreenshot(id); err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "screenshot not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// cameraScreenshotHandler handles POST /api/nvr/cameras/{id}/screenshot
// Currently returns NOT_IMPLEMENTED — requires ONVIF snapshot or frame grab.
func (h *Handlers) cameraScreenshotHandler(w http.ResponseWriter, r *http.Request, _ string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "NOT_IMPLEMENTED",
		"message": "screenshot capture requires ONVIF or frame-grab support, not yet available",
	})
}
