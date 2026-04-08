package pairing

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// --- Request / response types -----------------------------------------------

// PendingRequestBody is the JSON body for POST /api/v1/pairing/request
// sent by a Recorder that found the Directory via mDNS.
type PendingRequestBody struct {
	// RecorderHostname is the Recorder's self-reported hostname.
	RecorderHostname string `json:"recorder_hostname"`
	// RequestedRoles is the list of roles the Recorder suggests for itself.
	// Defaults to ["recorder"] if omitted.
	RequestedRoles []string `json:"requested_roles,omitempty"`
	// Note is an optional human-readable hint, e.g. "camera-room-b unit 3".
	Note string `json:"note,omitempty"`
}

// PendingRequestResponse is the JSON body returned by POST /api/v1/pairing/request.
// The Recorder should long-poll GET /api/v1/pairing/request/{id}/token until
// the admin approves or the request expires.
type PendingRequestResponse struct {
	// ID is the pending request UUID. Used to poll for approval.
	ID string `json:"id"`
	// ExpiresIn is a human-readable TTL, e.g. "5m0s".
	ExpiresIn string `json:"expires_in"`
	// Message advises the Recorder to wait for admin approval.
	Message string `json:"message"`
}

// PendingListResponse is the JSON body returned by GET /api/v1/pairing/pending.
type PendingListResponse struct {
	Requests []*PendingRequestView `json:"requests"`
}

// PendingRequestView is the admin-facing view of a pending request.
type PendingRequestView struct {
	ID               string   `json:"id"`
	RecorderHostname string   `json:"recorder_hostname"`
	RecorderIP       string   `json:"recorder_ip"`
	RequestedRoles   []string `json:"requested_roles"`
	Status           string   `json:"status"`
	Note             string   `json:"note,omitempty"`
	ExpiresAt        string   `json:"expires_at"`
	CreatedAt        string   `json:"created_at"`
}

// ApproveRequest is the JSON body for POST /api/v1/pairing/pending/{id}/approve.
type ApproveRequest struct {
	// SuggestedRoles overrides the Recorder's requested roles. If empty,
	// the Recorder's requested_roles are used.
	SuggestedRoles []string `json:"suggested_roles,omitempty"`
	// CloudTenantBinding is forwarded to Service.Generate.
	CloudTenantBinding string `json:"cloud_tenant_binding,omitempty"`
}

// ApproveResponse is the JSON body returned by approve and also polled by
// GET /api/v1/pairing/request/{id}/token once the admin has approved.
type ApproveResponse struct {
	// Token is the opaque pairing token the Recorder passes to Run().
	Token string `json:"token"`
	// TokenID is the UUID for admin audit.
	TokenID string `json:"token_id"`
	// ExpiresIn is the token TTL, e.g. "15m0s".
	ExpiresIn string `json:"expires_in"`
}

// --- Handlers ---------------------------------------------------------------

// RequestPairingHandler returns an http.HandlerFunc for:
//
//	POST /api/v1/pairing/request
//
// Called by a Recorder that discovered the Directory via mDNS. No auth is
// required on this endpoint — it only creates a pending row, it does not
// issue a token. Admin approval (ApprovePendingHandler) is the gate.
//
// Rate-limiting should be applied at the router level; this handler trusts
// that basic transport-layer protections are in place.
func RequestPairingHandler(svc *Service, pendingStore *PendingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var body PendingRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if body.RecorderHostname == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "recorder_hostname is required")
			return
		}

		// Extract source IP for the admin's benefit.
		srcIP := r.RemoteAddr
		if idx := strings.LastIndex(srcIP, ":"); idx >= 0 {
			srcIP = srcIP[:idx]
		}
		srcIP = strings.Trim(srcIP, "[]") // strip IPv6 brackets

		req, err := pendingStore.Create(r.Context(), body.RecorderHostname, srcIP, body.Note, body.RequestedRoles)
		if err != nil {
			svc.log.Error("pairing/pending: create failed", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "failed to create pairing request")
			return
		}

		svc.log.Info("pairing/pending: new request",
			"id", req.ID,
			"hostname", req.RecorderHostname,
			"src_ip", srcIP,
		)

		resp := PendingRequestResponse{
			ID:        req.ID,
			ExpiresIn: PendingRequestTTL.String(),
			Message:   "Pairing request submitted. Waiting for admin approval.",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ListPendingHandler returns an http.HandlerFunc for:
//
//	GET /api/v1/pairing/pending
//
// Authorization: admin-only. The caller MUST gate this with the Casbin
// enforcer before routing here.
func ListPendingHandler(pendingStore *PendingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}

		reqs, err := pendingStore.ListPending(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "failed to list requests")
			return
		}

		views := make([]*PendingRequestView, 0, len(reqs))
		for _, req := range reqs {
			views = append(views, &PendingRequestView{
				ID:               req.ID,
				RecorderHostname: req.RecorderHostname,
				RecorderIP:       req.RecorderIP,
				RequestedRoles:   req.RequestedRoles,
				Status:           string(req.Status),
				Note:             req.RequestNote,
				ExpiresAt:        req.ExpiresAt.Format("2006-01-02T15:04:05Z"),
				CreatedAt:        req.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		resp := PendingListResponse{Requests: views}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ApprovePendingHandler returns an http.HandlerFunc for:
//
//	POST /api/v1/pairing/pending/{id}/approve
//
// The handler mints a PairingToken (via Service.Generate) and stores it in
// the pending request row. The waiting Recorder will retrieve it via
// PollTokenHandler.
//
// Authorization: admin-only. Caller MUST gate with Casbin before routing here.
// userIDFromCtx extracts the authenticated UserID from the request context.
func ApprovePendingHandler(
	svc *Service,
	pendingStore *PendingStore,
	userIDFromCtx func(r *http.Request) (UserID, bool),
	idFromPath func(r *http.Request) string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		uid, ok := userIDFromCtx(r)
		if !ok || uid == "" {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing user id")
			return
		}

		id := idFromPath(r)
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "missing request id in path")
			return
		}

		// Fetch the pending request to get the Recorder's suggested roles.
		pending, err := pendingStore.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrPendingNotFound) {
				writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "pending request not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "lookup failed")
			return
		}
		if pending.Status != PendingStatusPending {
			writeJSONError(w, http.StatusConflict, "ALREADY_DECIDED", "request already "+string(pending.Status))
			return
		}

		var body ApproveRequest
		_ = json.NewDecoder(r.Body).Decode(&body) // body is optional

		roles := body.SuggestedRoles
		if len(roles) == 0 {
			roles = pending.RequestedRoles
		}

		// Mint the pairing token via the existing KAI-243 service.
		result, err := svc.Generate(r.Context(), uid, roles, body.CloudTenantBinding)
		if err != nil {
			svc.log.Error("pairing/pending: generate token for approve", "id", id, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "failed to mint token")
			return
		}

		// Record the approval in the pending request.
		if err := pendingStore.Approve(r.Context(), id, result.TokenID, uid); err != nil {
			// If this fails after token generation, log it but still return the
			// token — the Recorder can still use it and the token is tracked in
			// pairing_tokens.
			svc.log.Error("pairing/pending: record approval", "id", id, "error", err)
		}

		svc.log.Info("pairing/pending: approved",
			"request_id", id,
			"token_id", result.TokenID,
			"decided_by", string(uid),
		)

		resp := ApproveResponse{
			Token:     result.Encoded,
			TokenID:   result.TokenID,
			ExpiresIn: TokenTTL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// DenyPendingHandler returns an http.HandlerFunc for:
//
//	POST /api/v1/pairing/pending/{id}/deny
//
// Authorization: admin-only. Caller MUST gate with Casbin.
func DenyPendingHandler(
	svc *Service,
	pendingStore *PendingStore,
	userIDFromCtx func(r *http.Request) (UserID, bool),
	idFromPath func(r *http.Request) string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		uid, ok := userIDFromCtx(r)
		if !ok || uid == "" {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing user id")
			return
		}

		id := idFromPath(r)
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "missing request id in path")
			return
		}

		if err := pendingStore.Deny(r.Context(), id, uid); err != nil {
			switch {
			case errors.Is(err, ErrPendingNotFound):
				writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "pending request not found")
			case errors.Is(err, ErrPendingAlreadyDecided):
				writeJSONError(w, http.StatusConflict, "ALREADY_DECIDED", "request already decided")
			case errors.Is(err, ErrPendingExpired):
				writeJSONError(w, http.StatusGone, "EXPIRED", "request has expired")
			default:
				svc.log.Error("pairing/pending: deny", "id", id, "error", err)
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "deny failed")
			}
			return
		}

		svc.log.Info("pairing/pending: denied", "request_id", id, "decided_by", string(uid))
		w.WriteHeader(http.StatusNoContent)
	}
}

// PollTokenHandler returns an http.HandlerFunc for:
//
//	GET /api/v1/pairing/request/{id}/token
//
// Called by the Recorder at ~5 s intervals after submitting a pairing request.
// Returns 202 Accepted while pending, 200 OK with ApproveResponse once
// approved, 403 Forbidden if denied, and 410 Gone if expired.
//
// No auth required — the request ID is the bearer of the pending claim.
func PollTokenHandler(
	pendingStore *PendingStore,
	tokenStore *Store,
	idFromPath func(r *http.Request) string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}

		id := idFromPath(r)
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "missing request id in path")
			return
		}

		req, err := pendingStore.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrPendingNotFound) {
				writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "pending request not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "lookup failed")
			return
		}

		switch req.Status {
		case PendingStatusPending:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "pending",
				"message": "Waiting for admin approval.",
			})

		case PendingStatusApproved:
			// Fetch the actual token blob from pairing_tokens.
			stored, err := tokenStore.Get(r.Context(), req.TokenID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "token not found after approval")
				return
			}
			resp := ApproveResponse{
				Token:     stored.EncodedBlob,
				TokenID:   stored.TokenID,
				ExpiresIn: TokenTTL.String(),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case PendingStatusDenied:
			writeJSONError(w, http.StatusForbidden, "DENIED", "pairing request was denied by admin")

		case PendingStatusExpired:
			writeJSONError(w, http.StatusGone, "EXPIRED", "pairing request expired without admin decision")

		default:
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "unknown request status")
		}
	}
}
