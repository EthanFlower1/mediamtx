package billing_test

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/billing"
	"github.com/bluenviron/mediamtx/internal/cloud/tenants"
)

// TestBillingModeStringDriftGuard pins the string values of the billing-package
// BillingMode type to the untyped-string constants in internal/cloud/tenants.
//
// Why this test exists:
//
// The tenants package (KAI-227) introduced `BillingModeDirect = "direct"` and
// `BillingModeViaIntegrator = "via_integrator"` as untyped string constants
// used by CreateCustomerTenantSpec and the provisioning invariant checks. The
// billing package (KAI-362) re-declares the same values as a typed wrapper
// `BillingMode string` so the calculator can take a strongly-typed argument
// without importing tenants (which would risk a future import cycle once
// tenants wires invoice generation).
//
// If a future change renames or retypes either constant, the database rows
// persisted with the old string would silently stop matching the enum check
// in Valid(), the calculator would fall through to ErrInvalidBillingMode, and
// invoices would start failing in production. This test fails the build the
// moment the two drift apart so the author is forced to update both sides in
// the same PR (or add a migration).
func TestBillingModeStringDriftGuard(t *testing.T) {
	cases := []struct {
		name    string
		billing billing.BillingMode
		tenant  string
	}{
		{"direct", billing.BillingModeDirect, tenants.BillingModeDirect},
		{"via_integrator", billing.BillingModeViaIntegrator, tenants.BillingModeViaIntegrator},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if string(c.billing) != c.tenant {
				t.Fatalf("billing.%s = %q, tenants.%s = %q — string values drifted; "+
					"update both packages in the same PR (and add a DB migration if "+
					"the column value on disk needs rewriting)",
					c.name, string(c.billing), c.name, c.tenant)
			}
			if !c.billing.Valid() {
				t.Fatalf("billing.%s is not Valid() — Valid()'s switch is out of sync with the constants", c.name)
			}
		})
	}
}
