package legacynvrapi

import (
	"errors"
	"net/http"
	"strings"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// exportsCollection handles GET /api/nvr/exports and POST /api/nvr/exports.
func (h *Handlers) exportsCollection(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.exportsListHandler(w, r)
	case http.MethodPost:
		h.exportCreateHandler(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// exportsSubrouter handles /api/nvr/exports/{id} and /api/nvr/exports/{id}/download.
func (h *Handlers) exportsSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	// Strip prefix: /api/nvr/exports/{id}[/download]
	sub := strings.TrimPrefix(r.URL.Path, "/api/nvr/exports/")
	sub = strings.TrimSuffix(sub, "/")

	parts := strings.SplitN(sub, "/", 2)
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing export id"})
		return
	}

	if len(parts) == 2 && parts[1] == "download" {
		h.exportDownloadHandler(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.exportGetHandler(w, r, id)
	case http.MethodDelete:
		h.exportDeleteHandler(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// exportsListHandler handles GET /api/nvr/exports
// Optional query params: camera_id, status
func (h *Handlers) exportsListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cameraID := q.Get("camera_id")
	status := q.Get("status")

	jobs, err := h.RecDB.ListExportJobs(cameraID, status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": jobs})
}

type exportCreateRequest struct {
	CameraID  string `json:"camera_id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Format    string `json:"format"`
}

// exportCreateHandler handles POST /api/nvr/exports
func (h *Handlers) exportCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req exportCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.CameraID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "camera_id is required"})
		return
	}
	if req.StartTime == "" || req.EndTime == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "start_time and end_time are required"})
		return
	}

	job := &recdb.ExportJob{
		CameraID:  req.CameraID,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		// Format is stored in the notes field via OutputPath extension convention;
		// the DB schema does not have a format column so we embed it in Error
		// temporarily or ignore it — the worker reads recordings and picks format.
	}
	if err := h.RecDB.CreateExportJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// exportGetHandler handles GET /api/nvr/exports/{id}
func (h *Handlers) exportGetHandler(w http.ResponseWriter, _ *http.Request, id string) {
	job, err := h.RecDB.GetExportJob(id)
	if err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "export job not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// exportDeleteHandler handles DELETE /api/nvr/exports/{id}
func (h *Handlers) exportDeleteHandler(w http.ResponseWriter, _ *http.Request, id string) {
	if err := h.RecDB.DeleteExportJob(id); err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "export job not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// exportDownloadHandler handles GET /api/nvr/exports/{id}/download
func (h *Handlers) exportDownloadHandler(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	job, err := h.RecDB.GetExportJob(id)
	if err != nil {
		if errors.Is(err, recdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "export job not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if job.Status != "completed" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":  "export job is not completed",
			"status": job.Status,
		})
		return
	}

	if job.OutputPath == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "export job has no output file",
		})
		return
	}

	http.ServeFile(w, r, job.OutputPath)
}
