package permissions

import (
	"sync"
)

// PolicyRule is a single Casbin "p" line: (sub, obj, act, eft).
// Eft must be "allow" or "deny" — empty means "allow".
type PolicyRule struct {
	Sub string
	Obj string
	Act string
	Eft string
}

// effect normalizes an empty Eft to "allow".
func (r PolicyRule) effect() string {
	if r.Eft == "" {
		return "allow"
	}
	return r.Eft
}

// GroupingRule is a single Casbin "g" line: (subject, role).
// The second element is the role (e.g. "role:admin@tenant-A"), the first is
// either another role or a concrete subject string.
type GroupingRule struct {
	Subject string
	Role    string
}

// PolicyStore persists Casbin policies. The in-memory implementation in this
// package is the seam KAI-216 will replace with a real DB adapter; keep the
// surface narrow so the swap is drop-in.
//
// All methods MUST be safe for concurrent use.
type PolicyStore interface {
	// LoadAll returns the complete set of rules. The enforcer calls this
	// at startup and whenever ReloadPolicy is invoked.
	LoadAll() ([]PolicyRule, []GroupingRule, error)

	// AddPolicy appends a single policy rule. Returning nil means the rule
	// is now persisted and will survive a reload.
	AddPolicy(rule PolicyRule) error

	// RemovePolicy removes the matching rule (by exact field match).
	// Returns nil even if no match — removal is idempotent.
	RemovePolicy(rule PolicyRule) error

	// AddGrouping appends a role-assignment rule.
	AddGrouping(rule GroupingRule) error

	// RemoveGrouping removes a role assignment.
	RemoveGrouping(rule GroupingRule) error

	// ListPolicies returns a snapshot of all policy rules.
	ListPolicies() []PolicyRule

	// ListGroupings returns a snapshot of all grouping rules.
	ListGroupings() []GroupingRule
}

// InMemoryStore is a process-local PolicyStore. It is the only implementation
// this ticket ships; the DB-backed implementation will land with KAI-216.
type InMemoryStore struct {
	mu        sync.RWMutex
	policies  []PolicyRule
	groupings []GroupingRule
}

// NewInMemoryStore returns an empty in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

// LoadAll returns copies of the current policy and grouping slices.
func (s *InMemoryStore) LoadAll() ([]PolicyRule, []GroupingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := make([]PolicyRule, len(s.policies))
	copy(p, s.policies)
	g := make([]GroupingRule, len(s.groupings))
	copy(g, s.groupings)
	return p, g, nil
}

// AddPolicy appends a rule (deduplicated).
func (s *InMemoryStore) AddPolicy(rule PolicyRule) error {
	rule.Eft = rule.effect()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.policies {
		if existing == rule {
			return nil
		}
	}
	s.policies = append(s.policies, rule)
	return nil
}

// RemovePolicy is idempotent.
func (s *InMemoryStore) RemovePolicy(rule PolicyRule) error {
	rule.Eft = rule.effect()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.policies[:0]
	for _, existing := range s.policies {
		if existing == rule {
			continue
		}
		out = append(out, existing)
	}
	s.policies = out
	return nil
}

// AddGrouping appends a grouping rule (deduplicated).
func (s *InMemoryStore) AddGrouping(rule GroupingRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.groupings {
		if existing == rule {
			return nil
		}
	}
	s.groupings = append(s.groupings, rule)
	return nil
}

// RemoveGrouping is idempotent.
func (s *InMemoryStore) RemoveGrouping(rule GroupingRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.groupings[:0]
	for _, existing := range s.groupings {
		if existing == rule {
			continue
		}
		out = append(out, existing)
	}
	s.groupings = out
	return nil
}

// ListPolicies returns a snapshot copy.
func (s *InMemoryStore) ListPolicies() []PolicyRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PolicyRule, len(s.policies))
	copy(out, s.policies)
	return out
}

// ListGroupings returns a snapshot copy.
func (s *InMemoryStore) ListGroupings() []GroupingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupingRule, len(s.groupings))
	copy(out, s.groupings)
	return out
}
