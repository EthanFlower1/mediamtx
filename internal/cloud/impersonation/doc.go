// Package impersonation implements the customer impersonation service (KAI-379).
//
// This package provides audited, scope-limited, customer-authorized
// impersonation for platform support. Two impersonation modes are supported:
//
//  1. Integrator impersonation: integrator staff impersonate their customers.
//     The access is audited and scope-limited by the existing
//     customer-integrator relationship (KAI-224). The integrator gets the
//     customer's permissions minus admin actions.
//
//  2. Platform support impersonation: platform support staff impersonate any
//     tenant, but ONLY with explicit customer authorization. The customer
//     admin grants a time-limited "support session" token that the platform
//     support agent presents to begin impersonation.
//
// Security invariants:
//
//   - Every impersonation session creates an audit log entry on start and end.
//   - All actions during an impersonation session are tagged with
//     impersonating_user and impersonated_tenant context.
//   - Customer admins are notified of impersonation start and end.
//   - Sessions auto-terminate after a configurable timeout (default 30 min).
//   - No impersonation without explicit customer authorization.
//   - Admin-level actions (users.create, users.delete, permissions.grant,
//     permissions.revoke, settings.edit, billing.change) are NEVER available
//     during impersonation sessions.
//
// Dependencies:
//
//   - audit.Recorder (KAI-233) for audit log entries
//   - auth.IdentityProvider for user verification
//   - permissions.IntegratorRelationshipStore for scope resolution (integrator mode)
//   - crosstenant.RelationshipStore for relationship validation (integrator mode)
//   - NotificationSender for customer notifications
package impersonation
