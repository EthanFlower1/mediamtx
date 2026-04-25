package adminapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// NotificationPrefsHandlers groups notification preference handlers.
type NotificationPrefsHandlers struct {
	DB *directorydb.DB
}

// NotificationPrefsHandler handles GET/PUT for notification preferences.
//
//	GET /api/v1/admin/notification-preferences?user_id=<id>
//	PUT /api/v1/admin/notification-preferences?user_id=<id>
func (h *NotificationPrefsHandlers) NotificationPrefsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			jsonErr(w, http.StatusBadRequest, "user_id required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			prefs, err := h.DB.ListNotificationPreferences(userID)
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if prefs == nil {
				prefs = []*directorydb.NotificationPreference{}
			}
			jsonOK(w, map[string]any{"items": prefs})

		case http.MethodPut:
			var prefs []*directorydb.NotificationPreference
			if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			for _, p := range prefs {
				p.UserID = userID
			}
			if err := h.DB.BulkUpsertNotificationPreferences(prefs); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "updated"})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// EscalationRulesHandler handles GET/POST for escalation rules.
func (h *NotificationPrefsHandlers) EscalationRulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rules, err := h.DB.ListEscalationRules()
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if rules == nil {
				rules = []*directorydb.EscalationRule{}
			}
			jsonOK(w, map[string]any{"items": rules})

		case http.MethodPost:
			var rule directorydb.EscalationRule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := h.DB.CreateEscalationRule(&rule); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]int64{"id": rule.ID})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// EscalationRuleByIDHandler handles PUT/DELETE for escalation rules.
func (h *NotificationPrefsHandlers) EscalationRuleByIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			jsonErr(w, http.StatusBadRequest, "id required")
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			jsonErr(w, http.StatusBadRequest, "invalid id")
			return
		}
		switch r.Method {
		case http.MethodPut:
			var rule directorydb.EscalationRule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			rule.ID = id
			if err := h.DB.UpdateEscalationRule(&rule); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "updated"})

		case http.MethodDelete:
			if err := h.DB.DeleteEscalationRule(id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "deleted"})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
