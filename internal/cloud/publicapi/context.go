package publicapi

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ctxKey is a private type to avoid collisions with other packages.
type ctxKey int

const (
	ctxKeyClaims ctxKey = iota
	ctxKeyAPIKeyID
	ctxKeyTenantTier
)

// withPublicClaims attaches verified claims to the context.
func withPublicClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, claims)
}

// PublicClaimsFromContext retrieves claims from the context.
func PublicClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(ctxKeyClaims).(*auth.Claims)
	return c, ok && c != nil
}

// withAPIKeyID attaches the API key ID to the context.
func withAPIKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKeyID, id)
}

// APIKeyIDFromContext retrieves the API key ID if present.
func APIKeyIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKeyAPIKeyID).(string)
	return id, ok && id != ""
}

// withTenantTier attaches the tenant tier to the context.
func withTenantTier(ctx context.Context, tier TenantTier) context.Context {
	return context.WithValue(ctx, ctxKeyTenantTier, tier)
}

// TenantTierFromContext retrieves the tenant tier from the context.
func TenantTierFromContext(ctx context.Context) (TenantTier, bool) {
	t, ok := ctx.Value(ctxKeyTenantTier).(TenantTier)
	return t, ok
}
