// Package relationships manages the many-to-many customer↔integrator
// relationship table (KAI-228) and the sub-reseller hierarchy traversal
// (KAI-229).
//
// # State machine
//
// A relationship begins life as either:
//   - "pending_acceptance"  — an integrator has requested access (integrator
//     initiates, customer must approve); or
//   - "active"              — a customer admin directly grants access (customer
//     initiates, relationship is immediately live).
//
// Legal transitions:
//
//	pending_acceptance → active   (customer approves the request)
//	pending_acceptance → revoked  (customer or integrator withdraws request)
//	active             → revoked  (either party revokes)
//
// # Tenant-scoping (Seam #4)
//
// Every query is customer-tenant-scoped. The service derives the tenant from
// the caller's verified auth.Claims — never from request body fields. A
// customer admin can only manage relationships for their own tenant; an
// integrator can only see its own side.
//
// # Sub-reseller hierarchy (KAI-229)
//
// Three-level integrator hierarchy (root, child, grandchild). A sub-reseller's
// effective permissions on a customer tenant are the intersection of its own
// scoped_permissions with every ancestor's scoped_permissions. Children can
// only narrow, never broaden.
//
// # Authorization (KAI-225)
//
// Every mutating call checks:
//   - relationships.read  — list/inspect
//   - relationships.write — update permissions/markup on existing relationship
//   - relationships.grant — create, approve, or revoke
//
// # Audit log (KAI-233)
//
// Every successful mutation emits an audit.Entry with the customer tenant as
// TenantID (not the integrator's own tenant).
package relationships
