// Package tenants is the cloud control-plane tenant provisioning service
// (KAI-227).
//
// The Service type exposes four high-level operations:
//
//   - CreateIntegrator — bootstrap a new reseller / MSP tenant
//   - CreateCustomerTenant — bootstrap a new end-customer tenant
//   - CreateSubReseller — nest a reseller under an existing parent
//   - InviteInitialAdmin — create the first admin user for a freshly
//     provisioned tenant
//
// Each provisioning call performs SIX steps in order and, on any failure,
// unwinds prior steps via compensating actions. See README.md for the full
// diagram.
//
//  1. Insert the tenant row into `integrators` / `customer_tenants` (via
//     KAI-218's DB wrapper, inside a single SQL transaction).
//  2. Create the Zitadel org via the TenantBootstrapper seam (seam #3).
//     This is the ONLY step that touches an external system; failure here
//     has to unwind step 1 (tx rollback).
//  3. Leave brand_config_id NULL — the brand_configs table lands in KAI-310.
//  4. Seed Casbin policies from roles.DefaultRoleTemplates (KAI-225).
//  5. Enqueue a welcome-email job via the JobEnqueuer seam (KAI-234).
//  6. Emit an audit Entry with action "tenant.provision" (KAI-233).
//
// ROLLBACK SEMANTICS
//
// Because we cannot run distributed transactions across Postgres and
// Zitadel, we compose two unwind strategies:
//
//   - The DB rows are written inside a *sql.Tx that is only committed after
//     the Zitadel org call succeeds. A DB failure AFTER the Zitadel call is
//     therefore compensated by an explicit DeleteOrg.
//   - Casbin policy writes that fail mid-seed are reverted via a local
//     undo-stack.
//   - The welcome-email job is enqueued last so any earlier failure leaves
//     no stray side effect.
//
// SUB-RESELLER DEPTH CAP
//
// The spec allows a maximum chain depth of 3 (per the v1 roadmap: NSC ->
// regional -> city). This matches the mental model of the v1 reseller
// motion and prevents pathological trees that the scope resolver would
// have to walk on every request. Depth is measured inclusive of the root:
// a root integrator is depth 1, its sub-reseller is depth 2, the
// grandchild is depth 3 and is the last level any API will provision.
//
// SEAMS TAKEN
//
//   - TenantBootstrapper — the identity provider side. Defined in this
//     package rather than internal/shared/auth so KAI-223 can land in
//     parallel without proto-locking the IdentityProvider interface.
//   - JobEnqueuer — the River seam (KAI-234). One-method interface.
//   - PermissionChecker — a narrow wrapper around *permissions.Enforcer
//     that lets tests swap in a fake that always allows / always denies.
//
// MULTI-TENANT SAFETY
//
// Every mutation goes through db.TenantRef, carries the caller's
// authenticated subject (never a body-supplied tenant id), and emits one
// audit entry per mutation — no more, no less. The integration with
// KAI-235's chaos test is left to the API surface (KAI-226); this package
// only promises that its own call graph is side-effect-free on any error
// return.
package tenants
