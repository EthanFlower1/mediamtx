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
}

// SignupResponse is the JSON body returned on successful signup.
type SignupResponse struct {
	TenantID string `json:"tenant_id"`
	APIKey   string `json:"api_key"`
	Message  string `json:"message"`
}

// SignupHandler returns an http.Handler that handles POST /api/v1/signup.
//
// Flow:
//  1. Parse JSON body
//  2. Validate company_name and email are non-empty
//  3. Check email not already registered (409 Conflict)
//  4. Create tenant
//  5. Create API key
//  6. Return 201 with tenant_id, api_key, and message
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

		// Validate required fields.
		if req.CompanyName == "" || req.Email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "company_name and email are required",
			})
			return
		}

		// Check for duplicate email.
		if _, err := store.GetTenantByEmail(req.Email); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "email already registered",
			})
			return
		}

		// Create tenant.
		tenantID, err := store.CreateTenant(req.CompanyName, req.Email)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create tenant",
			})
			return
		}

		// Create API key.
		apiKey, err := store.CreateAPIKey(tenantID, "default")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to create API key",
			})
			return
		}

		writeJSON(w, http.StatusCreated, SignupResponse{
			TenantID: tenantID,
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
