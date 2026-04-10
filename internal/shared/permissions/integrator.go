package permissions

import (
	"context"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// IntegratorRelationship describes a single row of the sub-reseller chain.
// This mirrors the shape KAI-228 (customer_integrator_relationships) and
// KAI-229 (sub_reseller_hierarchy) will write into RDS; the resolver only
// needs the four fields below.
//
// ScopedActions is the set of actions this link is allowed to perform on
// CustomerTenant. An empty/nil slice means "inherit whatever the parent
// granted" — callers that want explicit-only semantics should pass a
// non-nil slice.
type IntegratorRelationship struct {
	// IntegratorUserID is the user in the integrator tenant.
	IntegratorUserID auth.UserID
	// CustomerTenant is the customer tenant being granted access.
	CustomerTenant auth.TenantRef
	// ParentIntegrator, if non-empty, points at the parent reseller record
	// whose permissions this one narrows.
	ParentIntegrator auth.IntegratorRelationshipRef
	// ScopedActions is the explicit allowlist for this link.
	ScopedActions []string
}

// IntegratorRelationshipStore is the seam the resolver walks. KAI-228 will
// ship a RDS-backed implementation; tests and KAI-225 ship an in-memory one.
type IntegratorRelationshipStore interface {
	// LookupRelationship returns the relationship for (integratorUserID,
	// customerTenant). ok=false means no such relationship exists and the
	// caller should deny.
	LookupRelationship(ctx context.Context, integratorUserID auth.UserID, customerTenant auth.TenantRef) (IntegratorRelationship, bool, error)

	// LookupParent resolves a parent reference. ok=false means the chain
	// terminates (i.e. this is a root integrator).
	LookupParent(ctx context.Context, ref auth.IntegratorRelationshipRef) (IntegratorRelationship, bool, error)
}

// ResolveIntegratorScope walks the parent_integrator chain and intersects
// ScopedActions at every step. The child NEVER broadens the parent —
// returning an action in the final set requires that every link in the chain
// allows it.
//
// Returns:
//   - allowed: the intersected action set (may be empty → no access)
//   - found:   whether a direct relationship exists at all
//   - err:     only for store errors
func ResolveIntegratorScope(
	ctx context.Context,
	store IntegratorRelationshipStore,
	integratorUserID auth.UserID,
	customerTenant auth.TenantRef,
) (allowed []string, found bool, err error) {
	if store == nil {
		return nil, false, fmt.Errorf("permissions: relationship store is nil")
	}
	rel, ok, err := store.LookupRelationship(ctx, integratorUserID, customerTenant)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	// Start with this link's scope.
	current := toSet(rel.ScopedActions)
	cursor := rel.ParentIntegrator

	// Walk parents and intersect. Bound the walk to prevent cycles.
	const maxDepth = 32
	for depth := 0; cursor != "" && depth < maxDepth; depth++ {
		parent, ok, err := store.LookupParent(ctx, cursor)
		if err != nil {
			return nil, true, err
		}
		if !ok {
			break
		}
		current = intersect(current, toSet(parent.ScopedActions))
		if len(current) == 0 {
			// Narrowed to empty — stop walking, the answer cannot grow.
			return nil, true, nil
		}
		cursor = parent.ParentIntegrator
	}

	// Deterministic output order.
	out := make([]string, 0, len(current))
	for a := range current {
		out = append(out, a)
	}
	return out, true, nil
}

// toSet converts a slice to a set; an empty slice becomes an empty map (which
// intersect treats as "no grants").
func toSet(actions []string) map[string]struct{} {
	m := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		if a == "" {
			continue
		}
		m[a] = struct{}{}
	}
	return m
}

// intersect returns a ∩ b. Either side containing the wildcard "*" is treated
// as "match everything the other side has".
func intersect(a, b map[string]struct{}) map[string]struct{} {
	if _, ok := a["*"]; ok {
		return copySet(b)
	}
	if _, ok := b["*"]; ok {
		return copySet(a)
	}
	out := make(map[string]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func copySet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

// InMemoryRelationshipStore is a test/bootstrap implementation of
// IntegratorRelationshipStore. Keys for direct lookup are "<user>|<tenant>".
type InMemoryRelationshipStore struct {
	Direct  map[string]IntegratorRelationship
	Parents map[auth.IntegratorRelationshipRef]IntegratorRelationship
}

// NewInMemoryRelationshipStore creates an empty store.
func NewInMemoryRelationshipStore() *InMemoryRelationshipStore {
	return &InMemoryRelationshipStore{
		Direct:  make(map[string]IntegratorRelationship),
		Parents: make(map[auth.IntegratorRelationshipRef]IntegratorRelationship),
	}
}

// LookupRelationship implements IntegratorRelationshipStore.
func (s *InMemoryRelationshipStore) LookupRelationship(
	_ context.Context, uid auth.UserID, tenant auth.TenantRef,
) (IntegratorRelationship, bool, error) {
	rel, ok := s.Direct[relationshipKey(uid, tenant)]
	return rel, ok, nil
}

// LookupParent implements IntegratorRelationshipStore.
func (s *InMemoryRelationshipStore) LookupParent(
	_ context.Context, ref auth.IntegratorRelationshipRef,
) (IntegratorRelationship, bool, error) {
	rel, ok := s.Parents[ref]
	return rel, ok, nil
}

// PutDirect inserts a direct relationship.
func (s *InMemoryRelationshipStore) PutDirect(rel IntegratorRelationship) {
	s.Direct[relationshipKey(rel.IntegratorUserID, rel.CustomerTenant)] = rel
}

// PutParent inserts a parent relationship addressable by ref.
func (s *InMemoryRelationshipStore) PutParent(ref auth.IntegratorRelationshipRef, rel IntegratorRelationship) {
	s.Parents[ref] = rel
}

func relationshipKey(uid auth.UserID, tenant auth.TenantRef) string {
	return fmt.Sprintf("%s|%s|%s", uid, tenant.Type, tenant.ID)
}
