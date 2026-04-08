package pairing

import (
	"encoding/json"
	"errors"
	"net/http"
)

// GenerateRequest is the JSON body for POST /api/v1/pairing/tokens.
type GenerateRequest struct {
	// SuggestedRoles is the list of roles the operator intends for the Recorder,
	// e.g. ["recorder"]. If omitted, defaults to ["recorder"].
	SuggestedRoles []string `json:"suggested_roles"`
	// CloudTenantBinding optionally ties the enrollment to a cloud-managed
	// tenant. Leave empty for air-gapped sites.
	CloudTenantBinding string `json:"cloud_tenant_binding,omitempty"`
}

// GenerateResponse is the JSON response from POST /api/v1/pairing/tokens.
type GenerateResponse struct {
	// TokenID is the human-readable UUID shown in the admin UI for audit
	// and for manual revocation.
	TokenID string `json:"token_id"`
	// Token is the opaque bearer credential that must be passed to
	// `mediamtx pair <token>` (KAI-244 CLI) within the TTL window.
	Token string `json:"token"`
	// ExpiresIn is a human-readable expiry hint, e.g. "15m0s".
	ExpiresIn string `json:"expires_in"`
}

// GenerateHandler returns an http.HandlerFunc for POST /api/v1/pairing/tokens.
//
// Authorization: the caller MUST gate this handler with the Casbin enforcer
// checking ActionRecorderPair (permissions.ActionRecorderPair) before routing
// to this handler. The handler trusts that auth + authz have already passed.
//
// userIDFromCtx extracts the authenticated UserID from the request context.
// The caller provides this so the pairing package does not couple to the auth
// package's context key.
func GenerateHandler(svc *Service, userIDFromCtx func(r *http.Request) (UserID, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"code":"METHOD_NOT_ALLOWED","message":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		uid, ok := userIDFromCtx(r)
		if !ok || uid == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthenticated", "missing user id in context")
			return
		}

		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, http.ErrBodyReadAfterClose) {
			// Tolerate a missing body; default roles apply.
			req = GenerateRequest{}
		}

		result, err := svc.Generate(r.Context(), uid, req.SuggestedRoles, req.CloudTenantBinding)
		if err != nil {
			svc.log.Error("pairing: generate handler error", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "internal", "failed to generate pairing token")
			return
		}

		resp := GenerateResponse{
			TokenID:   result.TokenID,
			Token:     result.Encoded,
			ExpiresIn: TokenTTL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// writeJSONError writes a minimal JSON error envelope.
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
