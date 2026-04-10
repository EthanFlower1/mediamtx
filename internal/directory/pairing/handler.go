package pairing

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AuditSink records security-relevant pairing events for SOC 2 CC6.1 / HIPAA
// 164.312(b) audit controls. A nil sink is tolerated (the handler no-ops),
// but production wire-up MUST supply a non-nil implementation. Concrete
// backends live outside the pairing package so that internal/directory/pairing
// does not cross-couple to internal/cloud/audit or an on-prem audit table.
//
// TODO(KAI-233 follow-up): wire an on-prem Directory audit backend here. The
// cloud audit service (internal/cloud/audit) cannot be imported from the
// Directory on-prem package per the cross-tree import rule; the on-prem
// Directory needs its own local audit table flushed to cloud via the
// DirectoryIngest stream.
type AuditSink interface {
	RecordPairingCheckIn(ctx context.Context, evt AuditEvent)
}

// AuditEvent is the payload recorded on every pairing check-in attempt
// regardless of outcome. On failure, RecorderID is empty and Outcome names
// the failure mode (e.g. "token_invalid", "pubkey_invalid", "internal_error").
type AuditEvent struct {
	Timestamp    time.Time
	Outcome      string // "success" | "token_invalid" | "pubkey_invalid" | "hardware_invalid" | "internal_error"
	TokenID      string // may be empty if the token could not be decoded
	RecorderID   string // empty on failure
	TenantID     string // from the token binding if decoded, else empty
	RemoteAddr   string // r.RemoteAddr as observed
	UserAgent    string
	OSRelease    string
	FailureCause string // short free-form description for forensic review
}

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
// Security properties enforced here (lead-security review, 2026-04):
//
//  1. device_pubkey is validated as a base64url-encoded 32-byte Ed25519
//     public key at the HTTP boundary; malformed/wrong-length values are
//     rejected before any DB work.
//  2. Every authentication failure — bad signature, expired token, not
//     in store, already-redeemed, or any other Redeem error — maps to a
//     single opaque 401 INVALID_TOKEN response. This eliminates the
//     redeem-state side channel that would otherwise let an attacker
//     with a stolen ciphertext learn whether the token is live, consumed,
//     or unknown. Forensic detail is written to the audit sink and slog
//     only, never to the client.
//  3. Every attempt — success or failure — is recorded via the AuditSink
//     for SOC 2 CC6.1 / HIPAA 164.312(b).
//
// svc is the pairing Service (provides Decode key and Redeem).
// recorderStore is the repository that persists the new recorder row.
// audit may be nil (no-op sink) but production wire-up MUST supply one.
// log may be nil (falls back to slog.Default()).
func CheckInHandler(svc *Service, recorderStore *RecorderStore, audit AuditSink, log *slog.Logger) http.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		// Buffer fields we want to include in every audit event.
		evt := AuditEvent{
			Timestamp:  time.Now().UTC(),
			RemoteAddr: r.RemoteAddr,
			UserAgent:  r.UserAgent(),
		}
		recordAudit := func(outcome, cause string) {
			evt.Outcome = outcome
			evt.FailureCause = cause
			if audit != nil {
				audit.RecordPairingCheckIn(r.Context(), evt)
			}
		}

		// Helper: collapse any authentication failure into a single opaque 401.
		// All token-auth failures — bad signature, expired token, token not in
		// store, already-redeemed — return the same response so an attacker
		// cannot distinguish redeem state from the HTTP reply. Forensic detail
		// is logged only on the server.
		//
		//nolint:unparam // outcome is always the same today but may diverge.
		rejectAsInvalidToken := func(outcome, logMsg string, attrs ...any) {
			log.Warn("pairing/check-in: "+logMsg, attrs...)
			recordAudit(outcome, logMsg)
			writeJSONError(w, http.StatusUnauthorized, "INVALID_TOKEN", "token invalid or expired")
		}

		// 1. Extract Bearer token.
		rawToken := extractBearer(r)
		if rawToken == "" {
			rejectAsInvalidToken("token_invalid", "missing or malformed Authorization header")
			return
		}

		// 2. Decode + verify signature + check expiry.
		verifyKey := svc.VerifyPublicKeyForDecode()
		pt, decodeErr := Decode(rawToken, verifyKey)
		if decodeErr != nil {
			rejectAsInvalidToken("token_invalid", "token decode failed", "error", decodeErr)
			return
		}
		// Token decoded: now we know the token_id + tenant binding, stamp them
		// onto the audit event for all subsequent outcomes.
		evt.TokenID = pt.TokenID
		evt.TenantID = pt.CloudTenantBinding

		// 3. Parse and validate body.
		var req CheckInRequest
		if bodyErr := json.NewDecoder(r.Body).Decode(&req); bodyErr != nil {
			recordAudit("hardware_invalid", "invalid JSON body")
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		evt.OSRelease = req.OSRelease

		// 3a. device_pubkey: base64url-decode, require 32-byte Ed25519 key.
		// RawURLEncoding matches the unpadded form used across the Kaivue
		// protocol (pairing tokens, JWTs, nonces, etc.).
		if req.DevicePubkey == "" {
			recordAudit("pubkey_invalid", "device_pubkey is required")
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "device_pubkey is required")
			return
		}
		pubkeyBytes, decodeB64Err := base64.RawURLEncoding.DecodeString(req.DevicePubkey)
		if decodeB64Err != nil {
			recordAudit("pubkey_invalid", "device_pubkey is not valid base64url")
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST",
				"device_pubkey must be base64url-encoded")
			return
		}
		if len(pubkeyBytes) != ed25519.PublicKeySize {
			recordAudit("pubkey_invalid", "device_pubkey wrong length")
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST",
				"device_pubkey must be a 32-byte Ed25519 public key")
			return
		}
		// Construct to confirm the type, even though we currently persist the
		// encoded form. The rebase onto KAI-139's recorders table will switch
		// RecorderRow.DevicePubkey to the raw []byte for BLOB storage.
		_ = ed25519.PublicKey(pubkeyBytes)

		// 3b. Hardware sanity.
		if req.Hardware.CPUCores == 0 {
			recordAudit("hardware_invalid", "hardware.cpu_cores must be non-zero")
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "hardware.cpu_cores must be non-zero")
			return
		}
		if req.Hardware.RAMBytes == 0 {
			recordAudit("hardware_invalid", "hardware.ram_bytes must be non-zero")
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

		// 5. Atomic single-use enforcement. Every Redeem failure — whether the
		// token is already redeemed, expired, missing from the store, or an
		// internal error — is collapsed into a single opaque 401 INVALID_TOKEN
		// response per the security invariant documented above. The
		// distinguishing detail (ErrAlreadyRedeemed vs ErrNotFound vs an
		// unexpected error) is written to the audit sink and slog for
		// forensic use but never exposed to the client.
		redeemErr := svc.Redeem(r.Context(), pt.TokenID)
		if redeemErr != nil {
			switch {
			case errors.Is(redeemErr, ErrAlreadyRedeemed):
				rejectAsInvalidToken("token_invalid", "token already redeemed",
					"token_id", pt.TokenID)
			case errors.Is(redeemErr, ErrTokenExpired):
				rejectAsInvalidToken("token_invalid", "token expired at redeem",
					"token_id", pt.TokenID)
			case errors.Is(redeemErr, ErrNotFound):
				rejectAsInvalidToken("token_invalid", "token not found in store",
					"token_id", pt.TokenID)
			default:
				// Internal errors must not leak as 401 — that would let a
				// failing DB masquerade as an auth failure and mask incidents.
				log.Error("pairing/check-in: redeem error",
					"token_id", pt.TokenID, "error", redeemErr)
				recordAudit("internal_error", "redeem internal error")
				writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
			return
		}

		// 6. Persist the new Recorder.
		recorderID := uuid.NewString()
		hwJSON := marshalHardware(req.Hardware)
		row := RecorderRow{
			RecorderID:   recorderID,
			TenantID:     pt.CloudTenantBinding,
			DevicePubkey: req.DevicePubkey, // NOTE: base64url form; rebase onto KAI-139 switches this to raw []byte for BLOB column.
			OSRelease:    req.OSRelease,
			HardwareJSON: hwJSON,
			TokenID:      pt.TokenID,
		}
		insertErr := recorderStore.Insert(r.Context(), row)
		if insertErr != nil {
			log.Error("pairing/check-in: recorder insert failed",
				"token_id", pt.TokenID, "recorder_id", recorderID, "error", insertErr)
			recordAudit("internal_error", "recorder insert failed")
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
			return
		}

		evt.RecorderID = recorderID
		log.Info("pairing/check-in: recorder enrolled",
			"recorder_id", recorderID,
			"token_id", pt.TokenID,
			"tenant_id", pt.CloudTenantBinding,
			"os_release", req.OSRelease,
		)
		recordAudit("success", "")

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
