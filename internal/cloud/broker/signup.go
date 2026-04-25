package broker

import (
	"encoding/json"
	"net/http"
	"strings"
)

// SignupRequest is the JSON body for POST /api/v1/signup.
type SignupRequest struct {
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name,omitempty"`
}

// SignupResponse is the JSON body returned on successful signup.
type SignupResponse struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	APIKey   string `json:"api_key"`
	Message  string `json:"message"`
}

// SignupHandler returns an http.Handler that handles POST /api/v1/signup.
//
// Flow: parse body → validate → check duplicate email → create tenant → create
// first user with bcrypt-hashed password → create default API key → return 201.
func SignupHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req SignupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		req.CompanyName = strings.TrimSpace(req.CompanyName)
		req.Email = strings.TrimSpace(req.Email)
		req.Name = strings.TrimSpace(req.Name)

		if req.CompanyName == "" || req.Email == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "company_name, email, and password are required",
			})
			return
		}
		if len(req.Password) < 12 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "password must be at least 12 characters",
			})
			return
		}

		if _, err := store.GetTenantByEmail(req.Email); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "email already registered",
			})
			return
		}

		tenantID, err := store.CreateTenant(req.CompanyName, req.Email)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create tenant",
			})
			return
		}

		userID, err := store.CreateUser(tenantID, req.Email, req.Name, req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create user",
			})
			return
		}

		apiKey, err := store.CreateAPIKey(tenantID, "default")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create API key",
			})
			return
		}

		writeJSON(w, http.StatusCreated, SignupResponse{
			TenantID: tenantID,
			UserID:   userID,
			APIKey:   apiKey,
			Message:  "Signup successful. Save your API key — it will not be shown again.",
		})
	})
}

// writeJSON is a small helper that writes a JSON response with the given
// status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
