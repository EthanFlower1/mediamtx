package zitadel

import (
	"context"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// auditEmit best-effort records one audit entry. If the Recorder is nil or
// Record returns an error, we swallow the failure — the adapter's primary
// responsibility is authentication, not audit durability. The audit package
// tests cover the Recorder reliability guarantees separately.
func (a *Adapter) auditEmit(
	ctx context.Context,
	tenant auth.TenantRef,
	actorUserID auth.UserID,
	action, resourceType, resourceID string,
	result audit.Result,
) {
	if a.cfg.AuditRecorder == nil {
		return
	}
	entry := audit.Entry{
		TenantID:     string(tenant.ID),
		ActorUserID:  string(actorUserID),
		ActorAgent:   audit.AgentCloud,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		Timestamp:    a.now(),
	}
	// Fill required fields fail-safes so audit.Validate doesn't reject
	// the entry on partially-authenticated events (e.g. a failed login
	// has no actor).
	if entry.TenantID == "" {
		entry.TenantID = "unknown"
	}
	if entry.ActorUserID == "" {
		entry.ActorUserID = "unauthenticated"
	}
	_ = a.cfg.AuditRecorder.Record(ctx, entry)
}

// now returns the current time via the injectable clock (for deterministic
// tests) or time.Now in production.
func (a *Adapter) now() time.Time {
	if a.cfg.Now != nil {
		return a.cfg.Now()
	}
	return time.Now()
}
