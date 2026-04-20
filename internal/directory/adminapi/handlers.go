package adminapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

// Handlers groups admin API HTTP handlers.
type Handlers struct {
	Store  *Store
	Logger *slog.Logger
}

func genID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- Users ------------------------------------------------------------------

func (h *Handlers) UsersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.listUsers(w, r)
		case http.MethodPost:
			h.createUser(w, r)
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func (h *Handlers) UserByIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, http.StatusBadRequest, "id required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			u, err := h.Store.GetUser(r.Context(), id)
			if err != nil {
				jsonErr(w, http.StatusNotFound, "user not found")
				return
			}
			jsonOK(w, u)
		case http.MethodPut:
			var u User
			if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			u.ID = id
			if err := h.Store.UpdateUser(r.Context(), u); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "update", "user", id)
			jsonOK(w, map[string]string{"status": "updated"})
		case http.MethodDelete:
			if err := h.Store.DeleteUser(r.Context(), id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "delete", "user", id)
			jsonOK(w, map[string]string{"status": "deleted"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.ListUsers(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"items": users})
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
		RoleID   string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if payload.Username == "" || payload.Password == "" {
		jsonErr(w, http.StatusBadRequest, "username and password required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "password hash failed")
		return
	}

	u := User{
		ID:           genID(),
		Username:     payload.Username,
		PasswordHash: string(hash),
		RoleID:       payload.RoleID,
	}
	if err := h.Store.CreateUser(r.Context(), u); err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "create", "user", u.ID)
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": u.ID, "username": u.Username})
}

// --- Roles ------------------------------------------------------------------

func (h *Handlers) RolesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			roles, err := h.Store.ListRoles(r.Context())
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": roles})
		case http.MethodPost:
			var role Role
			if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			role.ID = genID()
			if err := h.Store.CreateRole(r.Context(), role); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "create", "role", role.ID)
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": role.ID, "name": role.Name})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.DeleteRole(r.Context(), id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "delete", "role", id)
			jsonOK(w, map[string]string{"status": "deleted"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// --- Recording Schedules ----------------------------------------------------

func (h *Handlers) SchedulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := h.Store.ListSchedules(r.Context())
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": items})
		case http.MethodPost:
			var rs RecordingSchedule
			if err := json.NewDecoder(r.Body).Decode(&rs); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			rs.ID = genID()
			if err := h.Store.CreateSchedule(r.Context(), rs); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "create", "recording_schedule", rs.ID)
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": rs.ID})
		case http.MethodPut:
			var rs RecordingSchedule
			if err := json.NewDecoder(r.Body).Decode(&rs); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if rs.ID == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.UpdateSchedule(r.Context(), rs); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "update", "recording_schedule", rs.ID)
			jsonOK(w, map[string]string{"status": "updated"})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.DeleteSchedule(r.Context(), id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "delete", "recording_schedule", id)
			jsonOK(w, map[string]string{"status": "deleted"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// --- Retention Policies -----------------------------------------------------

func (h *Handlers) RetentionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := h.Store.ListRetention(r.Context())
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": items})
		case http.MethodPost:
			var rp RetentionPolicy
			if err := json.NewDecoder(r.Body).Decode(&rp); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			rp.ID = genID()
			if err := h.Store.CreateRetention(r.Context(), rp); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "create", "retention_policy", rp.ID)
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": rp.ID})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.DeleteRetention(r.Context(), id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "delete", "retention_policy", id)
			jsonOK(w, map[string]string{"status": "deleted"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// --- Alert Rules ------------------------------------------------------------

func (h *Handlers) AlertRulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := h.Store.ListAlertRules(r.Context())
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": items})
		case http.MethodPost:
			var ar AlertRule
			if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			ar.ID = genID()
			if err := h.Store.CreateAlertRule(r.Context(), ar); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "create", "alert_rule", ar.ID)
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": ar.ID})
		case http.MethodPut:
			var ar AlertRule
			if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if ar.ID == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.UpdateAlertRule(r.Context(), ar); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "update", "alert_rule", ar.ID)
			jsonOK(w, map[string]string{"status": "updated"})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				jsonErr(w, http.StatusBadRequest, "id required")
				return
			}
			if err := h.Store.DeleteAlertRule(r.Context(), id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "delete", "alert_rule", id)
			jsonOK(w, map[string]string{"status": "deleted"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// --- Audit Log --------------------------------------------------------------

func (h *Handlers) AuditHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			limit := 100
			if l := r.URL.Query().Get("limit"); l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 {
					limit = n
				}
			}
			entries, err := h.Store.ListAudit(r.Context(), limit)
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": entries})
		case http.MethodPost:
			// Accept audit events from recorders.
			var e AuditEntry
			if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := h.Store.InsertAudit(r.Context(), e); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "recorded"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// --- Export Jobs -------------------------------------------------------------

func (h *Handlers) ExportJobsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jobs, err := h.Store.ListExportJobs(r.Context())
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]any{"items": jobs})
		case http.MethodPost:
			var j ExportJob
			if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if j.RecorderID == "" || j.CameraID == "" {
				jsonErr(w, http.StatusBadRequest, "recorder_id and camera_id required")
				return
			}
			j.ID = genID()
			if j.Format == "" {
				j.Format = "mp4"
			}
			if err := h.Store.CreateExportJob(r.Context(), j); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			h.audit(r, "create", "export_job", j.ID)
			// TODO: dispatch to recorder via internal API
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": j.ID, "status": "pending"})
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func (h *Handlers) ExportJobByIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, http.StatusBadRequest, "id required")
			return
		}
		j, err := h.Store.GetExportJob(r.Context(), id)
		if err != nil {
			jsonErr(w, http.StatusNotFound, "export job not found")
			return
		}
		jsonOK(w, j)
	}
}

// --- Audit helper -----------------------------------------------------------

func (h *Handlers) audit(r *http.Request, action, resourceType, resourceID string) {
	_ = h.Store.InsertAudit(r.Context(), AuditEntry{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    r.RemoteAddr,
	})
}
