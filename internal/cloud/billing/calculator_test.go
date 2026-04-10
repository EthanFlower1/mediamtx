package billing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/billing"
)

const epsilon = 1e-9

func approxEqual(a, b float64) bool { return math.Abs(a-b) < epsilon }

func ptrString(s string) *string { return &s }

func TestCalculateInvoiceAmount_TableDriven(t *testing.T) {
	type tc struct {
		name             string
		tenant           billing.Tenant
		integrator       *billing.Integrator
		relationship     *billing.Relationship
		baseAmount       float64
		wantCustomer     float64
		wantIntegrator   float64
		wantPlatform     float64
		wantErrSentinel  error
	}

	cases := []tc{
		{
			name:           "direct mode keeps everything on the platform",
			tenant:         billing.Tenant{ID: "t1", BillingMode: billing.BillingModeDirect},
			baseAmount:     100,
			wantCustomer:   100,
			wantIntegrator: 0,
			wantPlatform:   100,
		},
		{
			name:           "direct mode at zero is a no-op",
			tenant:         billing.Tenant{ID: "t1", BillingMode: billing.BillingModeDirect},
			baseAmount:     0,
			wantCustomer:   0,
			wantIntegrator: 0,
			wantPlatform:   0,
		},
		{
			name: "via_integrator with 0/0 collapses to direct economics",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 0},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 0},
			baseAmount:     200,
			wantCustomer:   200,
			wantIntegrator: 0,
			wantPlatform:   200,
		},
		{
			name: "via_integrator with wholesale discount only (20% off, no markup)",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 20},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 0},
			baseAmount:     100,
			wantCustomer:   80, // wholesale = 80, no markup
			wantIntegrator: 0,
			wantPlatform:   80,
		},
		{
			name: "via_integrator with markup only (no wholesale discount)",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 0},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 25},
			baseAmount:     100,
			wantCustomer:   125, // wholesale = 100, +25% markup
			wantIntegrator: 25,
			wantPlatform:   100,
		},
		{
			name: "via_integrator with both wholesale and markup (canonical case)",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 20},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 50},
			baseAmount:     100,
			wantCustomer:   120, // wholesale=80, +50% markup => 120
			wantIntegrator: 40,
			wantPlatform:   80,
		},
		{
			name: "via_integrator with 100% wholesale discount yields zero everything",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 100},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 50},
			baseAmount:     500,
			wantCustomer:   0,
			wantIntegrator: 0,
			wantPlatform:   0,
		},
		{
			name: "via_integrator with 100% markup doubles the wholesale price",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:     &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 10},
			relationship:   &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 100},
			baseAmount:     1000,
			wantCustomer:   1800, // wholesale=900, +100% => 1800
			wantIntegrator: 900,
			wantPlatform:   900,
		},
		// ---------- error cases ----------
		{
			name:            "negative baseAmount rejected",
			tenant:          billing.Tenant{ID: "t1", BillingMode: billing.BillingModeDirect},
			baseAmount:      -1,
			wantErrSentinel: billing.ErrNegativeAmount,
		},
		{
			name:            "invalid billing mode rejected",
			tenant:          billing.Tenant{ID: "t1", BillingMode: billing.BillingMode("free")},
			baseAmount:      10,
			wantErrSentinel: billing.ErrInvalidBillingMode,
		},
		{
			name: "via_integrator without integrator rejected",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			relationship:    &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 0},
			baseAmount:      10,
			wantErrSentinel: billing.ErrMissingIntegrator,
		},
		{
			name: "via_integrator without relationship rejected",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:      &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 0},
			baseAmount:      10,
			wantErrSentinel: billing.ErrMissingRelationship,
		},
		{
			name: "wholesale_discount_percent > 100 rejected",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:      &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 101},
			relationship:    &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 0},
			baseAmount:      10,
			wantErrSentinel: billing.ErrPercentOutOfRange,
		},
		{
			name: "markup_percent > 100 rejected",
			tenant: billing.Tenant{
				ID: "t2", BillingMode: billing.BillingModeViaIntegrator,
				HomeIntegratorID: ptrString("int-1"),
			},
			integrator:      &billing.Integrator{ID: "int-1", WholesaleDiscountPercent: 0},
			relationship:    &billing.Relationship{CustomerTenantID: "t2", IntegratorID: "int-1", MarkupPercent: 150},
			baseAmount:      10,
			wantErrSentinel: billing.ErrPercentOutOfRange,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, err := billing.CalculateInvoiceAmount(c.tenant, c.integrator, c.relationship, c.baseAmount)
			if c.wantErrSentinel != nil {
				if err == nil {
					t.Fatalf("want sentinel %v, got nil err and %#v", c.wantErrSentinel, got)
				}
				if !errors.Is(err, c.wantErrSentinel) {
					t.Fatalf("want errors.Is(err, %v), got %v", c.wantErrSentinel, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !approxEqual(got.CustomerCharge, c.wantCustomer) {
				t.Errorf("CustomerCharge = %v, want %v", got.CustomerCharge, c.wantCustomer)
			}
			if !approxEqual(got.IntegratorPayout, c.wantIntegrator) {
				t.Errorf("IntegratorPayout = %v, want %v", got.IntegratorPayout, c.wantIntegrator)
			}
			if !approxEqual(got.PlatformRevenue, c.wantPlatform) {
				t.Errorf("PlatformRevenue = %v, want %v", got.PlatformRevenue, c.wantPlatform)
			}
			// Invariant: customer_charge == platform_revenue + integrator_payout
			if !approxEqual(got.CustomerCharge, got.PlatformRevenue+got.IntegratorPayout) {
				t.Errorf("invariant broken: %v != %v + %v",
					got.CustomerCharge, got.PlatformRevenue, got.IntegratorPayout)
			}
		})
	}
}

func TestLineItem_Total(t *testing.T) {
	li := billing.LineItem{Quantity: 3, UnitPrice: 12.5}
	if got := li.Total(); !approxEqual(got, 37.5) {
		t.Errorf("Total() = %v, want 37.5", got)
	}
}

func TestBillingMode_Valid(t *testing.T) {
	if !billing.BillingModeDirect.Valid() {
		t.Error("direct should be valid")
	}
	if !billing.BillingModeViaIntegrator.Valid() {
		t.Error("via_integrator should be valid")
	}
	if billing.BillingMode("nope").Valid() {
		t.Error("nope should be invalid")
	}
}
