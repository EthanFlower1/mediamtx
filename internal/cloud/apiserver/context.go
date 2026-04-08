// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ctxKey is a private type so external packages cannot accidentally collide
// with the context keys owned by this server.
type ctxKey int

const (
	ctxKeyTenant ctxKey = iota
	ctxKeyClaims
	ctxKeySubjectID
)

// withTenant attaches the resolved TenantRef to ctx. The tenant middleware
// runs BEFORE auth; if auth later replaces the tenant from verified claims,
// the claims win (that's the whole point of verifying the token).
func withTenant(ctx context.Context, tenant auth.TenantRef) context.Context {
	return context.WithValue(ctx, ctxKeyTenant, tenant)
}

// TenantFromContext returns the tenant attached to ctx. Callers that need
// strong authentication should use ClaimsFromContext instead — the tenant
// value can be set from an unauthenticated hint (e.g. subdomain) in the
// tenant middleware.
func TenantFromContext(ctx context.Context) (auth.TenantRef, bool) {
	t, ok := ctx.Value(ctxKeyTenant).(auth.TenantRef)
	return t, ok
}

// withClaims attaches verified token claims to ctx.
func withClaims(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, c)
}

// ClaimsFromContext returns the verified claims, if any. Handlers that
// require authentication should call this and return Unauthenticated if
// !ok — but in practice the auth middleware already refused unauthenticated
// requests to protected routes by the time the handler runs.
func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(ctxKeyClaims).(*auth.Claims)
	return c, ok && c != nil
}
