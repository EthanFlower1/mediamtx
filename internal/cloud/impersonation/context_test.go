package impersonation_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/impersonation"
)

func TestContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// No impersonation context initially.
	if impersonation.IsImpersonating(ctx) {
		t.Fatal("expected no impersonation context on bare context")
	}
	if impersonation.FromContext(ctx) != nil {
		t.Fatal("FromContext should return nil on bare context")
	}

	// Attach impersonation context.
	ic := &impersonation.ImpersonationContext{
		SessionID:             "sess-1",
		ImpersonatingUserID:   "alice",
		ImpersonatingTenantID: "int-acme",
		ImpersonatedTenantID:  "cust-widgets",
		Mode:                  impersonation.ModeIntegrator,
		ScopedPermissions:     []string{"view.live", "view.playback"},
	}
	ctx = impersonation.WithImpersonationContext(ctx, ic)

	if !impersonation.IsImpersonating(ctx) {
		t.Fatal("expected impersonation context to be present")
	}

	got := impersonation.FromContext(ctx)
	if got == nil {
		t.Fatal("FromContext should return non-nil")
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("session ID: got %q want %q", got.SessionID, "sess-1")
	}
	if got.ImpersonatingUserID != "alice" {
		t.Fatalf("impersonating user: got %q want %q", got.ImpersonatingUserID, "alice")
	}
	if got.ImpersonatedTenantID != "cust-widgets" {
		t.Fatalf("impersonated tenant: got %q want %q", got.ImpersonatedTenantID, "cust-widgets")
	}
	if got.Mode != impersonation.ModeIntegrator {
		t.Fatalf("mode: got %q want %q", got.Mode, impersonation.ModeIntegrator)
	}
	if len(got.ScopedPermissions) != 2 {
		t.Fatalf("scoped permissions: got %d want 2", len(got.ScopedPermissions))
	}
}
