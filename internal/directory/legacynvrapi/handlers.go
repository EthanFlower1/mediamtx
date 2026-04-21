// Package legacynvrapi registers the /api/nvr/... compatibility routes that
// the Flutter client expects. The Flutter ApiClient uses $serverUrl/api/nvr as
// its base URL, so every endpoint the app calls arrives on these paths.
//
// Design:
//   - All routes are registered on the caller's *http.ServeMux via Register.
//   - Top-priority endpoints (cameras CRUD, users, system info, notifications,
//     groups, schedule templates) are fully implemented against the directory DB.
//   - All other endpoints return a structured NOT_IMPLEMENTED JSON response so
//     the client gets a proper error instead of an HTML 404.
package legacynvrapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// Handlers holds the dependencies for the legacy /api/nvr/* compatibility layer.
type Handlers struct {
	// DB is the directory SQLite database. Required for cameras, users,
	// notifications, groups, and schedule-template endpoints.
	DB *dirdb.DB
}

// Register mounts all /api/nvr/... routes on mux.
// The existing auth routes (/api/nvr/auth/...) and /api/nvr/system/health are
// registered separately in boot.go and take precedence — Go's ServeMux uses the
// longest matching prefix so the more-specific patterns win automatically.
func (h *Handlers) Register(mux *http.ServeMux) {
	// --- CAMERAS -------------------------------------------------------
	mux.HandleFunc("/api/nvr/cameras", h.cameras)
	mux.HandleFunc("/api/nvr/cameras/", h.camerasSubrouter)

	// --- USERS ---------------------------------------------------------
	mux.HandleFunc("/api/nvr/users", h.users)
	mux.HandleFunc("/api/nvr/users/", h.usersSubrouter)

	// --- AUDIT LOG -----------------------------------------------------
	mux.HandleFunc("/api/nvr/audit", h.notImplemented)

	// --- AUTH ----------------------------------------------------------
	// /api/nvr/auth/login, /api/nvr/auth/refresh, /api/nvr/auth/revoke are
	// registered in boot.go — do NOT re-register them here.
	// Catch password-change and any other auth sub-routes.
	mux.HandleFunc("/api/nvr/auth/", h.notImplemented)

	// --- SYSTEM --------------------------------------------------------
	// /api/nvr/system/health is registered in boot.go — do NOT re-register.
	mux.HandleFunc("/api/nvr/system/info", h.systemInfo)
	mux.HandleFunc("/api/nvr/system/", h.notImplemented)

	// --- RECORDINGS & EXPORTS ------------------------------------------
	mux.HandleFunc("/api/nvr/recordings", h.notImplemented)
	mux.HandleFunc("/api/nvr/recordings/", h.notImplemented)
	mux.HandleFunc("/api/nvr/exports", h.notImplemented)
	mux.HandleFunc("/api/nvr/exports/", h.notImplemented)

	// --- NOTIFICATIONS -------------------------------------------------
	mux.HandleFunc("/api/nvr/notifications/unread-count", h.notificationsUnreadCount)
	mux.HandleFunc("/api/nvr/notifications/mark-read", h.notificationsMarkRead)
	mux.HandleFunc("/api/nvr/notifications/mark-all-read", h.notificationsMarkAllRead)
	mux.HandleFunc("/api/nvr/notifications", h.notifications)

	// --- CAMERA GROUPS -------------------------------------------------
	mux.HandleFunc("/api/nvr/camera-groups", h.cameraGroups)
	mux.HandleFunc("/api/nvr/camera-groups/", h.notImplemented)

	// --- TOURS ---------------------------------------------------------
	mux.HandleFunc("/api/nvr/tours", h.notImplemented)

	// --- SCHEDULE TEMPLATES --------------------------------------------
	mux.HandleFunc("/api/nvr/schedule-templates", h.scheduleTemplates)

	// --- BOOKMARKS -----------------------------------------------------
	mux.HandleFunc("/api/nvr/bookmarks", h.notImplemented)

	// --- SEARCH --------------------------------------------------------
	mux.HandleFunc("/api/nvr/search", h.notImplemented)

	// --- SCREENSHOTS ---------------------------------------------------
	mux.HandleFunc("/api/nvr/screenshots", h.notImplemented)
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// notImplemented returns the agreed-upon stub JSON for endpoints not yet wired up.
func (h *Handlers) notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "NOT_IMPLEMENTED",
		"message": "this endpoint is not yet available in directory mode",
	})
}

// pathID extracts the last non-empty segment of a URL path, e.g.
// "/api/nvr/cameras/abc-123" → "abc-123".
func pathID(path string) string {
	path = strings.TrimSuffix(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

// -----------------------------------------------------------------------
// CAMERAS
// -----------------------------------------------------------------------

// cameras handles /api/nvr/cameras (collection).
func (h *Handlers) cameras(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "database not available",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.camerasListHandler(w, r)
	case http.MethodPost:
		h.cameraCreateHandler(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// camerasSubrouter dispatches /api/nvr/cameras/{id} and sub-resource routes.
func (h *Handlers) camerasSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code": "DB_UNAVAILABLE", "message": "database not available",
		})
		return
	}

	// Strip prefix to get the sub-path after /api/nvr/cameras/
	sub := strings.TrimPrefix(r.URL.Path, "/api/nvr/cameras/")
	sub = strings.TrimSuffix(sub, "/")

	// Special collection actions that have no camera ID.
	switch sub {
	case "discover":
		h.notImplemented(w, r)
		return
	case "discover/results":
		h.notImplemented(w, r)
		return
	case "probe":
		h.notImplemented(w, r)
		return
	}

	// Everything else starts with {id} optionally followed by a sub-resource.
	parts := strings.SplitN(sub, "/", 2)
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing camera id"})
		return
	}

	if len(parts) == 1 {
		// /api/nvr/cameras/{id}
		switch r.Method {
		case http.MethodGet:
			h.cameraGetHandler(w, r, id)
		case http.MethodPut:
			h.cameraUpdateHandler(w, r, id)
		case http.MethodDelete:
			h.cameraDeleteHandler(w, r, id)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
		return
	}

	subResource := parts[1]
	switch subResource {
	case "streams":
		h.cameraStreamsHandler(w, r, id)
	case "ai":
		if r.Method == http.MethodPut {
			h.cameraAIUpdateHandler(w, r, id)
		} else {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	case "refresh", "storage-estimate", "screenshot":
		h.notImplemented(w, r)
	default:
		h.notImplemented(w, r)
	}
}

func (h *Handlers) camerasListHandler(w http.ResponseWriter, _ *http.Request) {
	cams, err := h.DB.ListCameras()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list cameras: " + err.Error(),
		})
		return
	}
	if cams == nil {
		cams = []*dirdb.Camera{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": cams})
}

type cameraCreateRequest struct {
	Name          string `json:"name"`
	ONVIFEndpoint string `json:"onvif_endpoint"`
	ONVIFUsername string `json:"onvif_username"`
	ONVIFPassword string `json:"onvif_password"`
	RTSPURL       string `json:"rtsp_url"`
	MediaMTXPath  string `json:"mediamtx_path"`
	Tags          string `json:"tags"`
	Status        string `json:"status"`
	StoragePath   string `json:"storage_path"`
	RetentionDays int    `json:"retention_days"`
}

func (h *Handlers) cameraCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req cameraCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	cam := &dirdb.Camera{
		Name:          req.Name,
		ONVIFEndpoint: req.ONVIFEndpoint,
		ONVIFUsername: req.ONVIFUsername,
		ONVIFPassword: req.ONVIFPassword,
		RTSPURL:       req.RTSPURL,
		MediaMTXPath:  req.MediaMTXPath,
		Tags:          req.Tags,
		Status:        req.Status,
		StoragePath:   req.StoragePath,
		RetentionDays: req.RetentionDays,
	}

	if err := h.DB.CreateCamera(cam); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create camera: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusCreated, cam)
}

func (h *Handlers) cameraGetHandler(w http.ResponseWriter, _ *http.Request, id string) {
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *Handlers) cameraUpdateHandler(w http.ResponseWriter, r *http.Request, id string) {
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Decode into a generic map so we can do partial updates without
	// overwriting fields the client didn't send.
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if v, ok := patch["name"].(string); ok && v != "" {
		cam.Name = v
	}
	if v, ok := patch["onvif_endpoint"].(string); ok {
		cam.ONVIFEndpoint = v
	}
	if v, ok := patch["onvif_username"].(string); ok {
		cam.ONVIFUsername = v
	}
	if v, ok := patch["onvif_password"].(string); ok && v != "" {
		cam.ONVIFPassword = v
	}
	if v, ok := patch["rtsp_url"].(string); ok {
		cam.RTSPURL = v
	}
	if v, ok := patch["mediamtx_path"].(string); ok {
		cam.MediaMTXPath = v
	}
	if v, ok := patch["tags"].(string); ok {
		cam.Tags = v
	}
	if v, ok := patch["status"].(string); ok {
		cam.Status = v
	}
	if v, ok := patch["storage_path"].(string); ok {
		cam.StoragePath = v
	}
	if v, ok := patch["retention_days"].(float64); ok {
		cam.RetentionDays = int(v)
	}
	if v, ok := patch["sub_stream_url"].(string); ok {
		cam.SubStreamURL = v
	}
	if v, ok := patch["ai_enabled"].(bool); ok {
		cam.AIEnabled = v
	}
	if v, ok := patch["audio_transcode"].(bool); ok {
		cam.AudioTranscode = v
	}

	if err := h.DB.UpdateCamera(cam); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *Handlers) cameraDeleteHandler(w http.ResponseWriter, _ *http.Request, id string) {
	if err := h.DB.DeleteCamera(id); err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) cameraStreamsHandler(w http.ResponseWriter, _ *http.Request, cameraID string) {
	streams, err := h.DB.ListCameraStreams(cameraID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if streams == nil {
		streams = []*dirdb.CameraStream{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": streams})
}

type aiUpdateRequest struct {
	AIEnabled    bool    `json:"ai_enabled"`
	AIStreamID   string  `json:"ai_stream_id"`
	AIConfidence float64 `json:"ai_confidence"`
	AITrackTimeout int   `json:"ai_track_timeout"`
}

func (h *Handlers) cameraAIUpdateHandler(w http.ResponseWriter, r *http.Request, id string) {
	var req aiUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if err := h.DB.UpdateCameraAIConfig(id, req.AIEnabled, req.AIStreamID, req.AIConfidence, req.AITrackTimeout); err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cam, err := h.DB.GetCamera(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

// -----------------------------------------------------------------------
// USERS
// -----------------------------------------------------------------------

func (h *Handlers) users(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.usersListHandler(w, r)
	case http.MethodPost:
		h.userCreateHandler(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) usersSubrouter(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	id := pathID(r.URL.Path)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing user id"})
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.userUpdateHandler(w, r, id)
	case http.MethodDelete:
		h.userDeleteHandler(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) usersListHandler(w http.ResponseWriter, _ *http.Request) {
	users, err := h.DB.ListUsers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if users == nil {
		users = []*dirdb.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

type userCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	RoleID   string `json:"role_id"`
}

func (h *Handlers) userCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req userCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	u := &dirdb.User{
		Username: req.Username,
		RoleID:   req.RoleID,
		// PasswordHash: hashing is intentionally omitted here — this endpoint
		// is a compatibility stub. Production user creation goes through the
		// auth API which properly hashes the password via bcrypt.
		PasswordHash: req.Password,
	}
	if err := h.DB.CreateUser(u); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (h *Handlers) userUpdateHandler(w http.ResponseWriter, r *http.Request, id string) {
	u, err := h.DB.GetUser(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if v, ok := patch["username"].(string); ok && v != "" {
		u.Username = v
	}
	if v, ok := patch["role_id"].(string); ok {
		u.RoleID = v
	}

	if err := h.DB.UpdateUser(u); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *Handlers) userDeleteHandler(w http.ResponseWriter, _ *http.Request, id string) {
	if err := h.DB.DeleteUser(id); err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -----------------------------------------------------------------------
// SYSTEM INFO
// -----------------------------------------------------------------------

var bootTime = time.Now()

func (h *Handlers) systemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": "1.0.0",
		"mode":    "directory",
		"uptime":  int64(time.Since(bootTime).Seconds()),
	})
}

// -----------------------------------------------------------------------
// NOTIFICATIONS
// -----------------------------------------------------------------------

func (h *Handlers) notifications(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Use a placeholder user ID since auth middleware is not yet enforced here.
	userID := userIDFromRequest(r)
	filter := dirdb.NotificationFilter{
		UserID: userID,
		Limit:  50,
	}
	items, _, err := h.DB.ListNotifications(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*dirdb.Notification{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) notificationsUnreadCount(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	userID := userIDFromRequest(r)
	count, err := h.DB.UnreadNotificationCount(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (h *Handlers) notificationsMarkRead(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID := userIDFromRequest(r)
	if err := h.DB.MarkNotificationsRead(userID, body.IDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) notificationsMarkAllRead(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	userID := userIDFromRequest(r)
	count, err := h.DB.MarkAllNotificationsRead(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"marked": count})
}

// userIDFromRequest attempts to extract the user ID from the Authorization
// header (bare bearer token) or falls back to a static placeholder.
// Real auth middleware should be applied upstream once JWT validation is wired in.
func userIDFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		// Return the token as a stand-in user ID for now — the notification
		// read-state table uses this as a partitioning key so the value just
		// needs to be stable per-user session.
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return "anonymous"
}

// -----------------------------------------------------------------------
// CAMERA GROUPS
// -----------------------------------------------------------------------

func (h *Handlers) cameraGroups(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := h.DB.ListGroups()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if groups == nil {
			groups = []dirdb.CameraGroup{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": groups})

	case http.MethodPost:
		var body struct {
			Name      string   `json:"name"`
			CameraIDs []string `json:"camera_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		g, err := h.DB.CreateGroup(body.Name, body.CameraIDs)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, g)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// -----------------------------------------------------------------------
// SCHEDULE TEMPLATES
// -----------------------------------------------------------------------

func (h *Handlers) scheduleTemplates(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "DB_UNAVAILABLE", "message": "database not available"})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	templates, err := h.DB.ListScheduleTemplates()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if templates == nil {
		templates = []*dirdb.ScheduleTemplate{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": templates})
}
