package permissions

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// FederationGrant describes one explicit grant from a receiving Directory to a
// peer Directory. Grants are always:
//   - Explicit: no implicit discovery; the receiving admin chooses exactly what
//     to share.
//   - Camera-level or resource-type-level: never tenant-wide wildcard.
//   - Action-scoped: the grant names exactly which actions the peer may perform.
//
// The subject in Casbin is always "federation:<PeerDirectoryID>". The object is
// always "<ReceivingTenantID>/<ResourceType>/<ResourceID>".
type FederationGrant struct {
	// PeerDirectoryID is the unique identifier of the requesting peer.
	PeerDirectoryID string

	// ReceivingTenant is the tenant that owns the resources being shared.
	ReceivingTenant auth.TenantRef

	// ResourceType is the type of resource being granted (e.g. "cameras").
	ResourceType string

	// ResourceID is the specific resource instance, or "*" for all instances
	// of the resource type. Note: wildcard grants should be used sparingly
	// and are restricted to the named resource type — they never span types.
	ResourceID string

	// Actions is the explicit set of actions the peer may perform on the
	// resource. Empty slices are rejected — a grant with no actions is a
	// no-op that could mask misconfiguration.
	Actions []string
}

// Validate checks that a FederationGrant is structurally well-formed and does
// not attempt to grant escalated privileges.
func (g FederationGrant) Validate() error {
	if g.PeerDirectoryID == "" {
		return fmt.Errorf("federation: peer directory id is required")
	}
	if strings.Contains(g.PeerDirectoryID, ":") || strings.Contains(g.PeerDirectoryID, "@") {
		return fmt.Errorf("federation: peer directory id must not contain ':' or '@'")
	}
	if g.ReceivingTenant.ID == "" {
		return fmt.Errorf("federation: receiving tenant is required")
	}
	if g.ResourceType == "" {
		return fmt.Errorf("federation: resource type is required")
	}
	if g.ResourceType == "*" {
		return fmt.Errorf("federation: wildcard resource type is forbidden for federation grants")
	}
	if strings.Contains(g.ResourceType, "/") {
		return fmt.Errorf("federation: resource type must not contain '/'")
	}
	if g.ResourceID == "" {
		return fmt.Errorf("federation: resource id is required (use '*' for type-wide)")
	}
	if len(g.Actions) == 0 {
		return fmt.Errorf("federation: at least one action is required")
	}
	for _, a := range g.Actions {
		if a == "" {
			return fmt.Errorf("federation: empty action in grant")
		}
		if a == "*" {
			return fmt.Errorf("federation: wildcard action '*' is forbidden for federation grants")
		}
		if !isFederationAllowedAction(a) {
			return fmt.Errorf("federation: action %q is not allowed for federation peers", a)
		}
	}
	return nil
}

// FederationAllowedActions is the closed set of actions a federation peer may
// be granted. Administrative, destructive, and privilege-escalation actions are
// excluded by design — a peer can never manage users, delete cameras, change
// billing, or grant permissions on the receiving site.
var FederationAllowedActions = map[string]bool{
	ActionViewThumbnails: true,
	ActionViewLive:       true,
	ActionViewPlayback:   true,
	ActionViewSnapshot:   true,
	ActionPTZControl:     true,
	ActionAudioTalkback:  true,
}

func isFederationAllowedAction(action string) bool {
	return FederationAllowedActions[action]
}

// FederationGrantManager provides the grant/revoke API for federation peers.
// It wraps the Enforcer and enforces all federation-specific invariants.
type FederationGrantManager struct {
	enforcer *Enforcer
}

// NewFederationGrantManager creates a manager backed by the given enforcer.
func NewFederationGrantManager(enforcer *Enforcer) *FederationGrantManager {
	return &FederationGrantManager{enforcer: enforcer}
}

// Grant creates explicit Casbin policy rules for a federation peer. Each
// (resource, action) pair becomes one PolicyRule. The enforcer is reloaded
// after all rules are written, making the grant effective immediately.
//
// Grant is idempotent — re-granting an existing permission is a no-op.
func (m *FederationGrantManager) Grant(_ context.Context, grant FederationGrant) error {
	if err := grant.Validate(); err != nil {
		return err
	}

	sub := NewFederationSubject(grant.PeerDirectoryID)
	obj := NewObject(grant.ReceivingTenant, grant.ResourceType, grant.ResourceID)

	for _, action := range grant.Actions {
		if err := m.enforcer.store.AddPolicy(PolicyRule{
			Sub: sub.String(),
			Obj: obj.String(),
			Act: action,
			Eft: "allow",
		}); err != nil {
			return fmt.Errorf("federation: add policy for %s/%s: %w", obj, action, err)
		}
	}

	return m.enforcer.ReloadPolicy()
}

// Revoke removes all policy rules matching the grant specification. The
// enforcer is reloaded after removal, making revocation effective immediately
// (within the same process — distributed reload is the responsibility of the
// caller's cache-invalidation layer).
//
// Revoke is idempotent — revoking a non-existent grant is a no-op.
func (m *FederationGrantManager) Revoke(_ context.Context, grant FederationGrant) error {
	// Validate everything except actions — we allow revoking with empty
	// actions to mean "revoke all actions for this resource".
	if grant.PeerDirectoryID == "" {
		return fmt.Errorf("federation: peer directory id is required for revoke")
	}
	if grant.ReceivingTenant.ID == "" {
		return fmt.Errorf("federation: receiving tenant is required for revoke")
	}
	if grant.ResourceType == "" {
		return fmt.Errorf("federation: resource type is required for revoke")
	}
	if grant.ResourceID == "" {
		return fmt.Errorf("federation: resource id is required for revoke")
	}

	sub := NewFederationSubject(grant.PeerDirectoryID)
	obj := NewObject(grant.ReceivingTenant, grant.ResourceType, grant.ResourceID)

	if len(grant.Actions) == 0 {
		// Revoke ALL actions for this (peer, resource) pair.
		return m.revokeAllActions(sub, obj)
	}

	for _, action := range grant.Actions {
		if err := m.enforcer.store.RemovePolicy(PolicyRule{
			Sub: sub.String(),
			Obj: obj.String(),
			Act: action,
			Eft: "allow",
		}); err != nil {
			return fmt.Errorf("federation: remove policy for %s/%s: %w", obj, action, err)
		}
	}

	return m.enforcer.ReloadPolicy()
}

// revokeAllActions removes every policy rule for (sub, obj) regardless of
// action. Used when the caller wants to fully revoke a peer's access to a
// resource without specifying individual actions.
func (m *FederationGrantManager) revokeAllActions(sub SubjectRef, obj ObjectRef) error {
	subStr := sub.String()
	objStr := obj.String()

	for _, p := range m.enforcer.store.ListPolicies() {
		if p.Sub == subStr && p.Obj == objStr {
			if err := m.enforcer.store.RemovePolicy(p); err != nil {
				return fmt.Errorf("federation: remove policy: %w", err)
			}
		}
	}
	return m.enforcer.ReloadPolicy()
}

// RevokePeer removes ALL policy rules for a given peer directory, across all
// tenants and resources. This is the nuclear option used when a federation
// peering relationship is terminated.
func (m *FederationGrantManager) RevokePeer(_ context.Context, peerDirectoryID string) error {
	if peerDirectoryID == "" {
		return fmt.Errorf("federation: peer directory id is required")
	}

	subStr := NewFederationSubject(peerDirectoryID).String()

	for _, p := range m.enforcer.store.ListPolicies() {
		if p.Sub == subStr {
			if err := m.enforcer.store.RemovePolicy(p); err != nil {
				return fmt.Errorf("federation: remove policy: %w", err)
			}
		}
	}
	// Also remove any grouping rules (federation peers shouldn't have them,
	// but defense in depth).
	for _, g := range m.enforcer.store.ListGroupings() {
		if g.Subject == subStr {
			if err := m.enforcer.store.RemoveGrouping(g); err != nil {
				return fmt.Errorf("federation: remove grouping: %w", err)
			}
		}
	}
	return m.enforcer.ReloadPolicy()
}

// ListPeerGrants returns all policy rules for a given peer directory,
// optionally filtered by tenant. This is the read side of the federation
// grant UI.
func (m *FederationGrantManager) ListPeerGrants(peerDirectoryID string, filterTenant *auth.TenantRef) []PolicyRule {
	subStr := NewFederationSubject(peerDirectoryID).String()
	var out []PolicyRule

	for _, p := range m.enforcer.store.ListPolicies() {
		if p.Sub != subStr {
			continue
		}
		if filterTenant != nil {
			// Object format is "<tenant_id>/<type>/<id>". Check prefix.
			if !strings.HasPrefix(p.Obj, filterTenant.ID+"/") {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// Enforcer exposes the underlying enforcer for direct Enforce calls. The
// caller is expected to build SubjectRef/ObjectRef and call Enforce on the
// returned enforcer — the FederationGrantManager only manages policy CRUD.
func (m *FederationGrantManager) Enforcer() *Enforcer {
	return m.enforcer
}
