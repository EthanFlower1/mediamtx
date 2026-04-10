// Package portal implements the customer-facing billing portal API (KAI-367).
//
// It exposes read-only billing data (usage summary, invoice history, plan
// details) to the React customer admin UI (KAI-329) and the Flutter app
// (KAI-305). All endpoints are tenant-scoped — the tenant_id is extracted
// from the authenticated request context, never from query parameters.
//
// # Dependencies (interface-only, seam-safe)
//
//   - UsageReader  — reads metering aggregates (KAI-364)
//   - PlanReader   — reads plan catalog + tenant's current plan (KAI-363)
//   - InvoiceReader — reads Stripe invoice history (KAI-361)
//   - PortalSessionCreator — creates Stripe billing portal sessions
//
// Production adapters are wired at startup. Tests inject fakes.
package portal
