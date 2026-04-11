package impersonation

import "context"

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	sessionKey contextKey = iota
)

// ImpersonationContext holds the impersonation metadata that middleware
// attaches to every request made during an impersonation session. Downstream
// handlers and the audit middleware use this to tag actions with the
// impersonating_user and impersonated_tenant.
type ImpersonationContext struct {
	// SessionID is the active impersonation session.
	SessionID string

	// ImpersonatingUserID is the user performing the impersonation.
	ImpersonatingUserID string

	// ImpersonatingTenantID is the impersonator's own tenant.
	ImpersonatingTenantID string

	// ImpersonatedTenantID is the customer tenant being impersonated.
	ImpersonatedTenantID string

	// Mode is the impersonation mode (integrator or platform_support).
	Mode ImpersonationMode

	// ScopedPermissions is the set of actions allowed in this session.
	ScopedPermissions []string
}

// WithImpersonationContext attaches impersonation metadata to a context.
// The audit middleware (KAI-233) checks for this and automatically populates
// ImpersonatingUserID and ImpersonatedTenantID on every audit.Entry.
func WithImpersonationContext(ctx context.Context, ic *ImpersonationContext) context.Context {
	return context.WithValue(ctx, sessionKey, ic)
}

// FromContext extracts impersonation metadata from a context, or nil if
// the request is not part of an impersonation session.
func FromContext(ctx context.Context) *ImpersonationContext {
	if ic, ok := ctx.Value(sessionKey).(*ImpersonationContext); ok {
		return ic
	}
	return nil
}

// IsImpersonating reports whether the context carries impersonation metadata.
func IsImpersonating(ctx context.Context) bool {
	return FromContext(ctx) != nil
}
