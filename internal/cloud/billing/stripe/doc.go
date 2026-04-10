// Package stripe implements the metering.UsageReporter interface for
// Stripe Connect marketplace billing (KAI-361).
//
// # Scope
//
// Kaivue uses Stripe Connect with the marketplace/facilitator model:
// Kaivue is the platform, each integrator (tenant) is a connected
// account. This package maps metering aggregates to Stripe's
// usage-record API for per-metric, per-subscription billing.
//
// # Design
//
// The package defines a [Client] interface that abstracts the Stripe
// HTTP surface. A production adapter (wired at startup) calls the
// real Stripe API via the official Go SDK; tests use a fake Client
// that captures calls into a slice. This keeps the package testable
// without network access and avoids a hard compile-time dependency on
// the Stripe SDK version.
//
// # Seams enforced
//
//   - Seam #3 (IdentityProvider firewall): zero Zitadel imports.
//   - Seam #4 (multi-tenant): reporter requires TenantID on every
//     aggregate; unknown tenants are rejected, not silently dropped.
//   - Seam #1 (package boundaries): depends inward on
//     internal/cloud/metering (UsageReporter interface), never the
//     other way around.
//
// # Metric → Stripe price mapping
//
// Each metering.Metric maps to a Stripe price id (metered usage-based
// pricing). The mapping is stored in the plan catalog (KAI-363)
// and injected via [Config.MetricPriceMap].
//
// # Connect onboarding
//
// The full KYC/onboarding flow is a follow-up. This v1 ships the
// usage-record adapter + tenant-to-connected-account resolver + the
// subscription-item lookup. The [AccountResolver] interface is the
// seam for the KYC module.
package stripe
