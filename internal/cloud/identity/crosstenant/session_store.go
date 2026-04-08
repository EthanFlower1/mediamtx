package crosstenant

import (
	"context"
	"sync"
	"time"
)

// ScopedSessionRecord is what the ScopedSessionStore persists per mint.
type ScopedSessionRecord struct {
	SessionID        string
	IntegratorUserID string
	IntegratorTenant string
	CustomerTenantID string
	PermissionScope  []string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	Revoked          bool
}

// ScopedSessionStore tracks active scoped sessions so they can be revoked
// before their natural expiry. KAI-227 may later replace the in-memory impl
// with an RDS-backed one.
type ScopedSessionStore interface {
	Put(ctx context.Context, rec ScopedSessionRecord) error
	Get(ctx context.Context, sessionID string) (ScopedSessionRecord, bool, error)
	Revoke(ctx context.Context, sessionID string) error
}

// InMemorySessionStore is a concurrency-safe in-memory ScopedSessionStore.
// Tests use this directly; production wiring can swap it out.
type InMemorySessionStore struct {
	mu    sync.RWMutex
	items map[string]ScopedSessionRecord
}

// NewInMemorySessionStore returns an empty store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{items: make(map[string]ScopedSessionRecord)}
}

// Put inserts or replaces a session record.
func (s *InMemorySessionStore) Put(_ context.Context, rec ScopedSessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[rec.SessionID] = rec
	return nil
}

// Get returns the session record for id, or ok=false if not present.
func (s *InMemorySessionStore) Get(_ context.Context, sessionID string) (ScopedSessionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.items[sessionID]
	return rec, ok, nil
}

// Revoke marks the session as revoked. Idempotent: an unknown session is a
// no-op, matching auth.IdentityProvider.RevokeSession semantics.
func (s *InMemorySessionStore) Revoke(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.items[sessionID]; ok {
		rec.Revoked = true
		s.items[sessionID] = rec
	}
	return nil
}

// RelationshipRecord is the row shape the RelationshipStore returns. It is a
// thin wrapper around permissions.IntegratorRelationship plus a Revoked flag,
// which the permissions package does not carry (KAI-225 stores only the
// non-revoked rows).
type RelationshipRecord struct {
	IntegratorUserID string
	IntegratorTenant string
	CustomerTenantID string
	Revoked          bool
}

// RelationshipStore is the seam KAI-227 (tenant provisioning) owns. For
// KAI-224 we ship an in-memory stub; the real impl will query
// customer_integrator_relationships (KAI-218 schema).
type RelationshipStore interface {
	Lookup(ctx context.Context, integratorUserID, customerTenantID string) (RelationshipRecord, bool, error)
}

// InMemoryRelationshipStore is a trivial test stub.
type InMemoryRelationshipStore struct {
	mu    sync.RWMutex
	items map[string]RelationshipRecord
}

// NewInMemoryRelationshipStore constructs an empty store.
func NewInMemoryRelationshipStore() *InMemoryRelationshipStore {
	return &InMemoryRelationshipStore{items: make(map[string]RelationshipRecord)}
}

// Put inserts or replaces a relationship row.
func (s *InMemoryRelationshipStore) Put(rec RelationshipRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[relKey(rec.IntegratorUserID, rec.CustomerTenantID)] = rec
}

// Lookup implements RelationshipStore.
func (s *InMemoryRelationshipStore) Lookup(_ context.Context, integratorUserID, customerTenantID string) (RelationshipRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.items[relKey(integratorUserID, customerTenantID)]
	return rec, ok, nil
}

func relKey(user, tenant string) string {
	return user + "|" + tenant
}
