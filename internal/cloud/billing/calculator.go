package billing

import "fmt"

// DefaultCalculator is the canonical implementation of the KAI-362 invoice
// calculator. It is a zero-value struct because the math is pure: there is no
// state to thread, and tests instantiate it via `var c billing.DefaultCalculator`.
type DefaultCalculator struct{}

// CalculateInvoiceAmount applies the billing-mode-specific math:
//
// Direct mode:
//
//	customer_charge   = baseAmount
//	platform_revenue  = baseAmount
//	integrator_payout = 0
//
// Via-integrator mode:
//
//	wholesale_price   = baseAmount * (1 - wholesale_discount_percent / 100)
//	customer_charge   = wholesale_price * (1 + markup_percent / 100)
//	platform_revenue  = wholesale_price
//	integrator_payout = customer_charge - wholesale_price
//
// Validation rules (returned as sentinel errors from billing.go):
//   - baseAmount must be >= 0
//   - tenant.BillingMode must be one of the recognised enum values
//   - via_integrator requires both an integrator and a relationship
//   - wholesale_discount_percent and markup_percent must be in 0..100
func (DefaultCalculator) CalculateInvoiceAmount(
	tenant Tenant,
	integrator *Integrator,
	relationship *Relationship,
	baseAmount float64,
) (InvoiceAmounts, error) {
	if baseAmount < 0 {
		return InvoiceAmounts{}, fmt.Errorf("%w: got %v", ErrNegativeAmount, baseAmount)
	}
	if !tenant.BillingMode.Valid() {
		return InvoiceAmounts{}, fmt.Errorf("%w: %q", ErrInvalidBillingMode, tenant.BillingMode)
	}

	switch tenant.BillingMode {
	case BillingModeDirect:
		return InvoiceAmounts{
			CustomerCharge:   baseAmount,
			IntegratorPayout: 0,
			PlatformRevenue:  baseAmount,
		}, nil

	case BillingModeViaIntegrator:
		if integrator == nil {
			return InvoiceAmounts{}, ErrMissingIntegrator
		}
		if relationship == nil {
			return InvoiceAmounts{}, ErrMissingRelationship
		}
		if integrator.WholesaleDiscountPercent < 0 || integrator.WholesaleDiscountPercent > 100 {
			return InvoiceAmounts{}, fmt.Errorf("%w: wholesale_discount_percent=%v",
				ErrPercentOutOfRange, integrator.WholesaleDiscountPercent)
		}
		if relationship.MarkupPercent < 0 || relationship.MarkupPercent > 100 {
			return InvoiceAmounts{}, fmt.Errorf("%w: markup_percent=%v",
				ErrPercentOutOfRange, relationship.MarkupPercent)
		}

		wholesale := baseAmount * (1 - integrator.WholesaleDiscountPercent/100.0)
		customerCharge := wholesale * (1 + relationship.MarkupPercent/100.0)
		integratorPayout := customerCharge - wholesale

		return InvoiceAmounts{
			CustomerCharge:   customerCharge,
			IntegratorPayout: integratorPayout,
			PlatformRevenue:  wholesale,
		}, nil
	}

	// Unreachable: Valid() above guards the enum.
	return InvoiceAmounts{}, fmt.Errorf("%w: %q", ErrInvalidBillingMode, tenant.BillingMode)
}

// CalculateInvoiceAmount is a convenience top-level function that delegates to
// the zero-value DefaultCalculator. Most call sites don't need an interface;
// the Calculator interface exists for the few that want to inject a fake.
func CalculateInvoiceAmount(
	tenant Tenant,
	integrator *Integrator,
	relationship *Relationship,
	baseAmount float64,
) (InvoiceAmounts, error) {
	return DefaultCalculator{}.CalculateInvoiceAmount(tenant, integrator, relationship, baseAmount)
}
