// Package plans is the single source of truth for the Kaivue Cloud v1 plan
// catalog (KAI-363). It defines the four pricing tiers (Free, Starter,
// Professional, Enterprise), the per-feature add-on catalog, and a pure
// Resolve function that flattens a (tier, add-ons) pair into an Entitlements
// view consumable by every other cloud subsystem.
//
// # Design rules
//
//   - Pure data + lookups. No database, no Stripe SDK, no HTTP. KAI-361
//     (Stripe Connect) and KAI-362 (billing schema) consume this package; this
//     package depends on neither.
//   - Money is always int64 cents. Never floats. Never strings.
//   - Determinism: AllAddOns and similar list helpers return results in stable
//     ID-sorted order so callers (UIs, billing exports, snapshot tests) get
//     reproducible output.
//   - Validation: Resolve rejects unknown add-ons and enforces a small set of
//     tier-compatibility rules so that, e.g., a Free-tier tenant cannot
//     unlock Professional-only AI add-ons without first upgrading.
//
// # Consumers
//
// Other cloud packages should import this package and call LookupPlan / Resolve
// rather than re-deriving plan data. There must be exactly one place where the
// numbers in the pricing table live, and that place is catalog.go.
package plans
