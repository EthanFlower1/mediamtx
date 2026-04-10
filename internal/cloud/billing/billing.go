// Package billing implements the KAI-362 invoice-amount calculator and the
// billing-context store for the Kaivue cloud control plane.
//
// Billing flows in two modes:
//
//   - direct: the customer is invoiced directly by the platform; the integrator
//     (if any) participates only as a permissioned operator and earns nothing.
//   - via_integrator: the integrator buys the service from the platform at a
//     wholesale discount, applies a markup, and resells to the customer.
//
// The pure-function calculator in calculator.go is the canonical source of
// truth for the math; the Store in store.go is the minimal tenant-scoped
// persistence helper that loads the inputs and lets a tenant flip modes.
package billing

import "errors"

// BillingMode is the discriminator on customer_tenants.billing_mode.
type BillingMode string

const (
	// BillingModeDirect — customer pays the platform directly. The platform
	// keeps 100% of the invoice amount and the integrator (if linked at all)
	// earns 0.
	BillingModeDirect BillingMode = "direct"

	// BillingModeViaIntegrator — the home integrator is the seller-of-record.
	// Platform revenue is the wholesale price; the integrator pockets the
	// markup as payout.
	BillingModeViaIntegrator BillingMode = "via_integrator"
)

// Valid reports whether m is one of the recognised billing modes.
func (m BillingMode) Valid() bool {
	switch m {
	case BillingModeDirect, BillingModeViaIntegrator:
		return true
	}
	return false
}

// String implements fmt.Stringer.
func (m BillingMode) String() string { return string(m) }

// Tenant is the slim view of customer_tenants the billing calculator needs.
// It is intentionally NOT a re-export of clouddb.CustomerTenant — the
// calculator only depends on what it actually uses, so that future schema
// additions don't ripple into the calculator's signature.
type Tenant struct {
	ID               string
	BillingMode      BillingMode
	HomeIntegratorID *string
	Region           string
}

// Integrator is the slim view of integrators the calculator needs.
type Integrator struct {
	ID                       string
	WholesaleDiscountPercent float64 // 0..100
}

// Relationship is the slim view of customer_integrator_relationships the
// calculator needs.
type Relationship struct {
	CustomerTenantID string
	IntegratorID     string
	MarkupPercent    float64 // 0..100
}

// LineItem is a single billable charge before mode-specific math is applied.
// Calculator inputs may be a single LineItem or an aggregated baseAmount;
// LineItem exists so callers (KAI-364 metering, KAI-363 plan catalog) can
// describe what they're billing for in a structured way.
type LineItem struct {
	SKU         string
	Description string
	Quantity    float64
	UnitPrice   float64 // platform-list unit price, in the smallest currency unit's float
}

// Total returns Quantity * UnitPrice. No tax, no discounts — those layer on
// top in the calculator.
func (l LineItem) Total() float64 { return l.Quantity * l.UnitPrice }

// InvoiceAmounts is the calculator's output triple. All values are in the
// same currency unit as the input baseAmount; the calculator does not do
// rounding or currency conversion.
//
// Invariant: CustomerCharge == IntegratorPayout + PlatformRevenue (within
// floating-point tolerance). The Store and the test suite both assert this.
type InvoiceAmounts struct {
	CustomerCharge   float64 // what the end customer pays
	IntegratorPayout float64 // what the home integrator earns (0 in direct mode)
	PlatformRevenue  float64 // what the platform recognises as revenue
}

// Calculator is the abstract interface for "given a tenant context and a base
// amount, compute who gets paid what". The default implementation lives in
// calculator.go; tests use the same implementation since the math is pure.
type Calculator interface {
	CalculateInvoiceAmount(
		tenant Tenant,
		integrator *Integrator,
		relationship *Relationship,
		baseAmount float64,
	) (InvoiceAmounts, error)
}

// Sentinel errors. The Store and Calculator both surface these so the API
// layer (KAI-367 customer billing portal) can map them to HTTP statuses.
var (
	// ErrInvalidBillingMode is returned when a Tenant carries a BillingMode
	// outside the {direct, via_integrator} enum.
	ErrInvalidBillingMode = errors.New("billing: invalid billing mode")

	// ErrMissingIntegrator is returned when a tenant in via_integrator mode
	// has no home integrator (or the calculator was called without one).
	ErrMissingIntegrator = errors.New("billing: via_integrator mode requires an integrator")

	// ErrMissingRelationship is returned when a via_integrator tenant has no
	// relationship row recording the markup.
	ErrMissingRelationship = errors.New("billing: via_integrator mode requires a customer_integrator_relationships row")

	// ErrNegativeAmount is returned when baseAmount < 0.
	ErrNegativeAmount = errors.New("billing: baseAmount must be >= 0")

	// ErrPercentOutOfRange is returned when wholesale_discount_percent or
	// markup_percent is outside 0..100. This mirrors the SQL CHECK constraints
	// after migration 0015.
	ErrPercentOutOfRange = errors.New("billing: percent must be in 0..100")

	// ErrTenantNotFound is returned by the Store when a tenant lookup misses.
	ErrTenantNotFound = errors.New("billing: tenant not found")
)
