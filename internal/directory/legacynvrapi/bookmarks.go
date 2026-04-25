package legacynvrapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// bookmarksCollection handles GET /api/nvr/bookmarks and POST /api/nvr/bookmarks.
func (h *Handlers) bookmarksCollection(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.bookmarksListHandler(w, r)
	case http.MethodPost:
		h.bookmarkCreateHandler(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// bookmarksSubrouter handles /api/nvr/bookmarks/{id}
func (h *Handlers) bookmarksSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	idStr := pathID(r.URL.Path)
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing bookmark id"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bookmark id: " + err.Error()})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.bookmarkDeleteHandler(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// bookmarksListHandler handles GET /api/nvr/bookmarks
// Query params: camera_id, start (RFC3339), end (RFC3339), query (search term)
func (h *Handlers) bookmarksListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cameraID := q.Get("camera_id")
	searchQuery := q.Get("query")

	// If a text search query is provided, use SearchBookmarks.
	if searchQuery != "" {
		items, err := h.RecDB.SearchBookmarks(searchQuery)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}

	// Otherwise query by camera + time range.
	start, end, err := parseTimeRange(q.Get("start"), q.Get("end"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if cameraID == "" {
		// No camera filter — default to a wide time window.
		if q.Get("start") == "" {
			start = time.Now().Add(-30 * 24 * time.Hour)
		}
		if q.Get("end") == "" {
			end = time.Now()
		}
		// Return bookmarks for all cameras by querying with an empty cameraID.
		// GetBookmarks requires a cameraID; fall back to SearchBookmarks with empty query.
		items, err := h.RecDB.SearchBookmarks("")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}

	items, err := h.RecDB.GetBookmarks(cameraID, start, end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type bookmarkCreateRequest struct {
	CameraID  string `json:"camera_id"`
	Timestamp string `json:"timestamp"`
	Label     string `json:"label"`
	Notes     string `json:"notes"`
	CreatedBy string `json:"created_by"`
}

// bookmarkCreateHandler handles POST /api/nvr/bookmarks
func (h *Handlers) bookmarkCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req bookmarkCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.CameraID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "camera_id is required"})
		return
	}
	if req.Timestamp == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timestamp is required"})
		return
	}

	b := &recdb.Bookmark{
		CameraID:  req.CameraID,
		Timestamp: req.Timestamp,
		Label:     req.Label,
		Notes:     req.Notes,
		CreatedBy: req.CreatedBy,
	}
	if err := h.RecDB.InsertBookmark(b); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// bookmarkDeleteHandler handles DELETE /api/nvr/bookmarks/{id}
func (h *Handlers) bookmarkDeleteHandler(w http.ResponseWriter, _ *http.Request, id int64) {
	if err := h.RecDB.DeleteBookmark(id); err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "bookmark not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
