package revocation

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// AuditSink receives audit events for token revocation actions.
// Declared in-package to avoid cross-tree import of internal/cloud/audit.
type AuditSink interface {
	RecordRevocation(ctx context.Context, event AuditEvent)
}

// AuditEvent captures forensic details of a revocation action.
type AuditEvent struct {
	Timestamp   time.Time
	Action      string // "revoke_recorder_tokens" or "revoke_all_tokens"
	RecorderID  string
	TenantID    string
	RevokedBy   string
	TokenCount  int64
	Reason      string
	RemoteAddr  string
	UserAgent   string
}

// Notifier pushes revocation events to connected Recorders. The
// implementation is wired when the RecorderControl server-side lands
// (KAI-142). Until then, the nopNotifier is used and revocation takes
// effect on the Recorder's next token refresh / reconnect.
type Notifier interface {
	NotifyRevocation(ctx context.Context, recorderID string) error
}

type nopNotifier struct{}

func (nopNotifier) NotifyRevocation(context.Context, string) error { return nil }

// DefaultNotifier is a no-op until KAI-142 wires the push path.
var DefaultNotifier Notifier = nopNotifier{}

// RevokeRequest is the JSON body for the revoke-tokens endpoint.
type RevokeRequest struct {
	// Reason is a human-readable justification for the audit log.
	Reason string `json:"reason"`
	// TokenJTIs is the list of specific token JTIs to revoke. If empty,
	// the handler revokes by recorder_id pattern (all tokens whose
	// subject matches the recorder).
	TokenJTIs []string `json:"token_jtis,omitempty"`
	// ExpiresAt is the latest expiry among the tokens being revoked.
	// Required when TokenJTIs is provided; the handler uses it for
	// blocklist GC eligibility.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// RevokeResponse is the JSON response from the revoke-tokens endpoint.
type RevokeResponse struct {
	RevokedCount int64  `json:"revoked_count"`
	RecorderID   string `json:"recorder_id"`
	Message      string `json:"message"`
}

// HandlerConfig configures the revocation HTTP handler.
type HandlerConfig struct {
	Store    *Store
	Notifier Notifier
	Audit    AuditSink
	Log      *slog.Logger
}

// RevokeTokensHandler returns an http.HandlerFunc that handles
// POST /api/v1/admin/recorders/{recorder_id}/revoke-tokens.
//
// The caller is responsible for:
//   - Routing (extracting recorder_id from the URL path)
//   - Authentication (verifying the caller is an admin)
//   - Passing recorder_id and tenant_id extracted from the auth context
func RevokeTokensHandler(cfg HandlerConfig) func(w http.ResponseWriter, r *http.Request, recorderID, tenantID, adminUserID string) {
	if cfg.Notifier == nil {
		cfg.Notifier = DefaultNotifier
	}

	return func(w http.ResponseWriter, r *http.Request, recorderID, tenantID, adminUserID string) {
		ctx := r.Context()
		log := cfg.Log.With(
			slog.String("recorder_id", recorderID),
			slog.String("admin_user_id", adminUserID),
		)

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req RevokeRequest
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
				return
			}
		}

		if recorderID == "" {
			http.Error(w, `{"error":"recorder_id is required"}`, http.StatusBadRequest)
			return
		}

		now := time.Now()
		reason := req.Reason
		if reason == "" {
			reason = "admin force-revocation"
		}

		var revokedCount int64

		if len(req.TokenJTIs) > 0 {
			// Explicit JTI list — revoke specific tokens.
			expiresAt := now.Add(24 * time.Hour) // default: assume 24h TTL
			if req.ExpiresAt != nil {
				expiresAt = *req.ExpiresAt
			}

			tokens := make([]RevokedToken, len(req.TokenJTIs))
			for i, jti := range req.TokenJTIs {
				tokens[i] = RevokedToken{
					JTI:        jti,
					RecorderID: recorderID,
					TenantID:   tenantID,
					RevokedBy:  adminUserID,
					Reason:     reason,
					RevokedAt:  now,
					ExpiresAt:  expiresAt,
				}
			}

			n, err := cfg.Store.RevokeBatch(ctx, tokens)
			if err != nil {
				log.ErrorContext(ctx, "revocation: batch insert failed",
					slog.String("error", err.Error()))
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			revokedCount = n
		} else {
			// No explicit JTIs — this is a "revoke all for recorder" operation.
			// In a full implementation, this would query the IdP for active
			// sessions/tokens for this recorder subject and blocklist them all.
			// For now, we insert a sentinel row that the middleware checks
			// against the token's recorder_id claim (not just JTI).
			sentinel := RevokedToken{
				JTI:        "revoke-all:" + recorderID + ":" + now.Format(time.RFC3339Nano),
				RecorderID: recorderID,
				TenantID:   tenantID,
				RevokedBy:  adminUserID,
				Reason:     reason,
				RevokedAt:  now,
				ExpiresAt:  now.Add(30 * 24 * time.Hour), // 30-day sentinel
			}
			if err := cfg.Store.Revoke(ctx, sentinel); err != nil {
				log.ErrorContext(ctx, "revocation: sentinel insert failed",
					slog.String("error", err.Error()))
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			revokedCount = 1
		}

		// Notify the connected Recorder (best-effort).
		if err := cfg.Notifier.NotifyRevocation(ctx, recorderID); err != nil {
			log.WarnContext(ctx, "revocation: notifier failed (recorder may be offline)",
				slog.String("error", err.Error()))
			// Non-fatal: the Recorder will be rejected on next auth attempt.
		}

		// Audit.
		if cfg.Audit != nil {
			cfg.Audit.RecordRevocation(ctx, AuditEvent{
				Timestamp:  now,
				Action:     "revoke_recorder_tokens",
				RecorderID: recorderID,
				TenantID:   tenantID,
				RevokedBy:  adminUserID,
				TokenCount: revokedCount,
				Reason:     reason,
				RemoteAddr: r.RemoteAddr,
				UserAgent:  r.UserAgent(),
			})
		}

		log.InfoContext(ctx, "revocation: tokens revoked",
			slog.Int64("count", revokedCount),
			slog.String("reason", reason))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RevokeResponse{
			RevokedCount: revokedCount,
			RecorderID:   recorderID,
			Message:      "tokens revoked",
		})
	}
}

// IsRecorderRevoked checks whether a "revoke-all" sentinel exists for
// the given recorder_id. This is called from auth middleware when a
// token's JTI is not individually blocklisted — to catch the blanket
// revocation case.
func (s *Store) IsRecorderRevoked(ctx context.Context, recorderID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM revoked_tokens
		 WHERE recorder_id = ? AND jti LIKE 'revoke-all:%'
		   AND expires_at > CURRENT_TIMESTAMP
		 LIMIT 1`,
		recorderID,
	).Scan(&exists)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
