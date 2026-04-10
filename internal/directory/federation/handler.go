package federation

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// InviteRequest is the JSON body for POST /api/v1/federation/invite.
type InviteRequest struct {
	// No fields required — the founding admin just clicks the button.
	// Future: optional PeerName hint, custom TTL, etc.
}

// InviteResponse is the JSON response from POST /api/v1/federation/invite.
type InviteResponse struct {
	TokenID    string `json:"token_id"`
	Token      string `json:"token"`
	PeerSiteID string `json:"peer_site_id"`
	ExpiresIn  string `json:"expires_in"`
	ExpiresAt  string `json:"expires_at"`
}

// InviteHandler returns an http.HandlerFunc for POST /api/v1/federation/invite.
//
// Authorization: the caller MUST gate this with appropriate admin authz before
// routing to this handler. The handler trusts that auth + authz have passed.
//
// userIDFromCtx extracts the authenticated user ID from the request context.
func InviteHandler(svc *Service, userIDFromCtx func(r *http.Request) (string, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		uid, ok := userIDFromCtx(r)
		if !ok || uid == "" {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing user id in context")
			return
		}

		result, err := svc.Invite(r.Context(), uid)
		if err != nil {
			svc.log.Error("federation: invite handler error", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate federation invite token")
			return
		}

		resp := InviteResponse{
			TokenID:    result.TokenID,
			Token:      result.FEDToken,
			PeerSiteID: result.PeerSiteID,
			ExpiresIn:  TokenTTL.String(),
			ExpiresAt:  result.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// JoinHTTPRequest is the JSON body for POST /api/v1/federation/join.
type JoinHTTPRequest struct {
	// Token is the full "FED-v1...." string.
	Token string `json:"token"`
	// PeerEndpoint is the joining Directory's reachable endpoint.
	PeerEndpoint string `json:"peer_endpoint"`
	// PeerName is the human-readable name of the joining site.
	PeerName string `json:"peer_name"`
	// PeerJWKS is the JSON-encoded JWKS of the joining Directory's signing keys.
	PeerJWKS string `json:"peer_jwks"`
	// PeerSiteID is the site ID claimed by the joining Directory. Must match
	// the pre-allocated ID in the token.
	PeerSiteID string `json:"peer_site_id"`
}

// JoinHTTPResponse is the JSON response from POST /api/v1/federation/join.
type JoinHTTPResponse struct {
	FoundingSiteID   string `json:"founding_site_id"`
	FoundingEndpoint string `json:"founding_endpoint"`
	FoundingJWKS     string `json:"founding_jwks"`
	CAFingerprint    string `json:"ca_fingerprint"`
	CARootPEM        string `json:"ca_root_pem"`
}

// JoinHandler returns an http.HandlerFunc for POST /api/v1/federation/join.
//
// This endpoint is unauthenticated (the token IS the authorization), but the
// token is single-use and TTL-bound.
//
// verifyKey is the ed25519 public key for verifying enrollment tokens. In
// production this is derived via federation.DerivePeerTokenVerifyKey from the
// federation CA root key.
func JoinHandler(svc *Service, verifyKey ed25519.PublicKey, log *slog.Logger) http.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var req JoinHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}

		if req.Token == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "token is required")
			return
		}
		if req.PeerEndpoint == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "peer_endpoint is required")
			return
		}
		if req.PeerJWKS == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "peer_jwks is required")
			return
		}

		joinReq := JoinRequest{
			FEDToken:     req.Token,
			PeerEndpoint: req.PeerEndpoint,
			PeerName:     req.PeerName,
			PeerJWKS:     req.PeerJWKS,
			PeerSiteID:   req.PeerSiteID,
		}

		result, err := svc.Join(r.Context(), joinReq, verifyKey)
		if err != nil {
			// Map known errors to appropriate HTTP codes.
			switch {
			case errors.Is(err, ErrAlreadyRedeemed):
				log.Warn("federation/join: token already redeemed", "error", err)
				writeJSONError(w, http.StatusConflict, "TOKEN_ALREADY_USED",
					"this federation token has already been used")
			case errors.Is(err, ErrTokenExpired):
				log.Warn("federation/join: token expired", "error", err)
				writeJSONError(w, http.StatusGone, "TOKEN_EXPIRED",
					"this federation token has expired")
			case errors.Is(err, ErrNotFound):
				log.Warn("federation/join: token not found", "error", err)
				writeJSONError(w, http.StatusNotFound, "TOKEN_NOT_FOUND",
					"federation token not found")
			default:
				log.Error("federation/join: handshake error", "error", err)
				writeJSONError(w, http.StatusUnauthorized, "INVALID_TOKEN",
					"federation token is invalid")
			}
			return
		}

		resp := JoinHTTPResponse{
			FoundingSiteID:   result.FoundingSiteID,
			FoundingEndpoint: result.FoundingEndpoint,
			FoundingJWKS:     result.FoundingJWKS,
			CAFingerprint:    result.CAFingerprint,
			CARootPEM:        result.CARootPEM,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// writeJSONError writes a minimal JSON error envelope.
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
