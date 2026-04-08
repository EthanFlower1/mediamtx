// Package fixtures provides deterministic seed data for integration and unit
// tests of the cloud control plane schema (KAI-218). The IDs are stable so
// Casbin policy fixtures (KAI-225) and audit log tests (KAI-233) can reference
// the same tenants.
package fixtures

import (
	"context"
	"fmt"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// Stable seed IDs. Treat as test-only constants.
const (
	IntegratorAcmeID        = "11111111-1111-4111-8111-111111111111"
	CustomerTenantBetaID    = "22222222-2222-4222-8222-222222222222"
	CustomerTenantGammaID   = "33333333-3333-4333-8333-333333333333"
	UserAcmeAdminID         = "44444444-4444-4444-8444-444444444444"
	UserBetaOperatorID      = "55555555-5555-4555-8555-555555555555"
	DirectoryBetaSiteOneID  = "66666666-6666-4666-8666-666666666666"
)

// Seed inserts the canonical test fixture set: one integrator, two customer
// tenants (one billed via the integrator, one direct), two users (one
// integrator staff, one customer staff), and one on-prem directory.
func Seed(ctx context.Context, d *clouddb.DB) error {
	region := clouddb.DefaultRegion

	// 1) Root integrator.
	if err := d.InsertIntegrator(ctx, clouddb.Integrator{
		ID:                       IntegratorAcmeID,
		DisplayName:              "Acme Security Integrators",
		LegalName:                strPtr("Acme Security LLC"),
		ContactEmail:             strPtr("ops@acme-security.test"),
		BillingMode:              "via_integrator",
		WholesaleDiscountPercent: 20.00,
		Status:                   "active",
		Region:                   region,
	}); err != nil {
		return fmt.Errorf("insert integrator: %w", err)
	}

	// 2) Two customer tenants — one billed via Acme, one direct.
	if err := d.InsertCustomerTenant(ctx, clouddb.CustomerTenant{
		ID:               CustomerTenantBetaID,
		DisplayName:      "Beta Warehouse Co",
		BillingMode:      "via_integrator",
		HomeIntegratorID: strPtr(IntegratorAcmeID),
		Status:           "active",
		Region:           region,
	}); err != nil {
		return fmt.Errorf("insert beta: %w", err)
	}
	if err := d.InsertCustomerTenant(ctx, clouddb.CustomerTenant{
		ID:          CustomerTenantGammaID,
		DisplayName: "Gamma Retail Inc",
		BillingMode: "direct",
		Status:      "active",
		Region:      region,
	}); err != nil {
		return fmt.Errorf("insert gamma: %w", err)
	}

	// 3) Relationship: Acme manages Beta with a 15% markup.
	if err := d.InsertCustomerIntegratorRelationship(
		ctx,
		clouddb.TenantRef{Type: clouddb.TenantCustomerTenant, ID: CustomerTenantBetaID, Region: region},
		clouddb.CustomerIntegratorRelationship{
			IntegratorID:      IntegratorAcmeID,
			ScopedPermissions: `{"sites":["*"],"cameras":["*"]}`,
			RoleTemplate:      "full_management",
			MarkupPercent:     15.00,
			Status:            "active",
		},
	); err != nil {
		return fmt.Errorf("insert relationship: %w", err)
	}

	// 4) Two users — one integrator staff, one customer staff.
	if err := d.InsertUser(
		ctx,
		clouddb.TenantRef{Type: clouddb.TenantIntegrator, ID: IntegratorAcmeID, Region: region},
		clouddb.User{
			ID:          UserAcmeAdminID,
			Email:       "admin@acme-security.test",
			DisplayName: strPtr("Acme Admin"),
			Status:      "active",
		},
	); err != nil {
		return fmt.Errorf("insert acme admin: %w", err)
	}
	if err := d.InsertUser(
		ctx,
		clouddb.TenantRef{Type: clouddb.TenantCustomerTenant, ID: CustomerTenantBetaID, Region: region},
		clouddb.User{
			ID:          UserBetaOperatorID,
			Email:       "ops@beta-warehouse.test",
			DisplayName: strPtr("Beta Operator"),
			Status:      "active",
		},
	); err != nil {
		return fmt.Errorf("insert beta operator: %w", err)
	}

	// 5) One on-prem directory for Beta.
	if err := d.InsertOnPremDirectory(
		ctx,
		clouddb.TenantRef{Type: clouddb.TenantCustomerTenant, ID: CustomerTenantBetaID, Region: region},
		clouddb.OnPremDirectory{
			ID:              DirectoryBetaSiteOneID,
			DisplayName:     "Beta HQ Directory",
			SiteLabel:       "beta-hq-01",
			DeploymentMode:  "cloud_connected",
			SoftwareVersion: strPtr("1.0.0"),
			Status:          "online",
		},
	); err != nil {
		return fmt.Errorf("insert directory: %w", err)
	}

	return nil
}

func strPtr(s string) *string { return &s }
