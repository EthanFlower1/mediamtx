package pairing

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
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

// ---------------------------------------------------------------------------
// POST /api/v1/pairing/check-in (KAI-430)
// ---------------------------------------------------------------------------

// CheckInDisk represents a single disk in the Recorder's hardware snapshot.
type CheckInDisk struct {
	Device    string `json:"device"`
	SizeBytes int64  `json:"size_bytes"`
	Model     string `json:"model,omitempty"`
}

// CheckInNIC represents a network interface in the Recorder's hardware snapshot.
type CheckInNIC struct {
	Name string `json:"name"`
	MAC  string `json:"mac,omitempty"`
}

// CheckInHardware is the hardware snapshot sent by the Recorder during check-in.
// All fields are informational; only cpu_cores and ram_bytes are validated as
// non-zero to guard against truncated payloads.
type CheckInHardware struct {
	CPUModel string        `json:"cpu_model"`
	CPUCores int           `json:"cpu_cores"`
	RAMBytes int64         `json:"ram_bytes"`
	Disks    []CheckInDisk `json:"disks"`
	NICs     []CheckInNIC  `json:"nics"`
	GPU      string        `json:"gpu,omitempty"`
}

// CheckInRequest is the JSON body for POST /api/v1/pairing/check-in.
type CheckInRequest struct {
	Hardware     CheckInHardware `json:"hardware"`
	DevicePubkey string          `json:"device_pubkey"`
	OSRelease    string          `json:"os_release"`
}

// CheckInResponse is the JSON body returned by POST /api/v1/pairing/check-in
// on success (HTTP 200).
type CheckInResponse struct {
	RecorderID        string `json:"recorder_id"`
	TenantID          string `json:"tenant_id"`
	DirectoryEndpoint string `json:"directory_endpoint"`
	NextStepHint      string `json:"next_step_hint"`
}

// CheckInHandler returns an http.HandlerFunc for:
//
//	POST /api/v1/pairing/check-in
//
// This is step 2 of the 9-step Recorder join sequence (KAI-244). The Recorder
// presents its PairingToken as a Bearer credential, supplies its hardware
// snapshot and device public key, and receives a fresh recorder_id plus the
// address of the Directory's Connect-Go endpoint.
//
// Authorization: PairingToken extracted from Authorization: Bearer header.
// No Casbin enforcement — the token IS the authorization.
//
// svc is the pairing Service (provides Decode key and Redeem).
// recorderStore is the repository that persists the new recorder row.
// log may be nil (falls back to slog.Default()).
func CheckInHandler(svc *Service, recorderStore *RecorderStore, log *slog.Logger) http.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		// 1. Extract Bearer token.
		rawToken := extractBearer(r)
		if rawToken == "" {
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing or malformed Authorization header")
			return
		}

		// 2. Decode + verify signature + check expiry.
		verifyKey := svc.VerifyPublicKeyForDecode()
		var decodeErr error
		pt, decodeErr := Decode(rawToken, verifyKey)
		if decodeErr != nil {
			log.Warn("pairing/check-in: token decode failed", "error", decodeErr)
			writeJSONError(w, http.StatusUnauthorized, "INVALID_TOKEN", "token invalid or expired")
			return
		}

		// 3. Parse and validate body.
		var req CheckInRequest
		if bodyErr := json.NewDecoder(r.Body).Decode(&req); bodyErr != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.DevicePubkey == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "device_pubkey is required")
			return
		}
		if req.Hardware.CPUCores == 0 {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "hardware.cpu_cores must be non-zero")
			return
		}
		if req.Hardware.RAMBytes == 0 {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "hardware.ram_bytes must be non-zero")
			return
		}

		// 4. Cross-tenant check: if the token carries a cloud tenant binding,
		// reject any attempt to present the token while impersonating a different
		// tenant. (The tenant_id in the response is taken from the token itself,
		// so there is nothing to spoof here — but guard against future request
		// fields that might carry a tenant override.)
		// Currently there is no tenant field in CheckInRequest; this comment and
		// check are intentionally preserved as a seam for future additions.

		// 5. Atomic single-use enforcement.
		redeemErr := svc.Redeem(r.Context(), pt.TokenID)
		if redeemErr != nil {
			switch {
			case errors.Is(redeemErr, ErrAlreadyRedeemed):
				log.Warn("pairing/check-in: token already redeemed", "token_id", pt.TokenID)
				writeJSONError(w, http.StatusGone, "ALREADY_REDEEMED", "pairing token has already been used")
			case errors.Is(redeemErr, ErrTokenExpired):
				log.Warn("pairing/check-in: token expired at redeem", "token_id", pt.TokenID)
				writeJSONError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "pairing token has expired")
			case errors.Is(redeemErr, ErrNotFound):
				// Token decoded fine but is not in the DB — treat as tampered/invalid.
				log.Warn("pairing/check-in: token not found in store", "token_id", pt.TokenID)
				writeJSONError(w, http.StatusUnauthorized, "INVALID_TOKEN", "token not recognized by this directory")
			default:
				log.Error("pairing/check-in: redeem error", "token_id", pt.TokenID, "error", redeemErr)
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "redeem failed")
			}
			return
		}

		// 6. Persist the new Recorder.
		recorderID := uuid.NewString()
		hwJSON := marshalHardware(req.Hardware)
		row := RecorderRow{
			RecorderID:   recorderID,
			TenantID:     pt.CloudTenantBinding,
			DevicePubkey: req.DevicePubkey,
			OSRelease:    req.OSRelease,
			HardwareJSON: hwJSON,
			TokenID:      pt.TokenID,
		}
		insertErr := recorderStore.Insert(r.Context(), row)
		if insertErr != nil {
			log.Error("pairing/check-in: recorder insert failed",
				"token_id", pt.TokenID, "recorder_id", recorderID, "error", insertErr)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "failed to register recorder")
			return
		}

		log.Info("pairing/check-in: recorder enrolled",
			"recorder_id", recorderID,
			"token_id", pt.TokenID,
			"tenant_id", pt.CloudTenantBinding,
			"os_release", req.OSRelease,
		)

		// 7. Return the recorder identity.
		resp := CheckInResponse{
			RecorderID:        recorderID,
			TenantID:          pt.CloudTenantBinding,
			DirectoryEndpoint: pt.DirectoryEndpoint,
			NextStepHint:      "enroll-with-stepca",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// extractBearer returns the token string from an "Authorization: Bearer <tok>"
// header, or "" if the header is absent or malformed.
func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	tok := strings.TrimSpace(auth[len(prefix):])
	return tok
}
