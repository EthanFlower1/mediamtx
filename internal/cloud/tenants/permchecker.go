package tenants

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// PermissionChecker is the narrow Casbin seam the provisioning service
// depends on. Production code passes an adapter over *permissions.Enforcer;
// tests pass allowAllChecker / denyAllChecker.
//
// The check is always against the "platform" meta-tenant because creating
// an integrator or customer tenant is, by construction, cross-tenant. The
// concrete tenant string is the caller's own tenant id (platform staff) or
// the parent integrator id (sub-reseller creation).
type PermissionChecker interface {
	Enforce(ctx context.Context, subject permissions.SubjectRef, object permissions.ObjectRef, action string) (bool, error)
}

// enforcerAdapter wraps *permissions.Enforcer into PermissionChecker.
type enforcerAdapter struct{ e *permissions.Enforcer }

// NewEnforcerChecker wraps a production Enforcer as a PermissionChecker.
func NewEnforcerChecker(e *permissions.Enforcer) PermissionChecker {
	return enforcerAdapter{e: e}
}

func (a enforcerAdapter) Enforce(
	ctx context.Context,
	subject permissions.SubjectRef,
	object permissions.ObjectRef,
	action string,
) (bool, error) {
	return a.e.Enforce(ctx, subject, object, action)
}

// AllowAllChecker is a test helper that allows every action. Exposed so
// KAI-226 (API server) tests can reuse it without redefining.
type AllowAllChecker struct{}

// Enforce implements PermissionChecker.
func (AllowAllChecker) Enforce(
	_ context.Context,
	_ permissions.SubjectRef,
	_ permissions.ObjectRef,
	_ string,
) (bool, error) {
	return true, nil
}

// DenyAllChecker is a test helper that denies every action.
type DenyAllChecker struct{}

// Enforce implements PermissionChecker.
func (DenyAllChecker) Enforce(
	_ context.Context,
	_ permissions.SubjectRef,
	_ permissions.ObjectRef,
	_ string,
) (bool, error) {
	return false, nil
}

// platformTenant is the meta-tenant used as the object of every
// provisioning permission check. Not a real DB row — just the string
// namespace under which platform-level Casbin policies live.
func platformTenant() auth.TenantRef {
	return auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "platform"}
}
