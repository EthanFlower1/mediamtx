package impersonation

import (
	"context"
	"sync"
)

// SessionStore persists impersonation sessions. Production will use an
// RDS-backed implementation; tests use InMemorySessionStore.
type SessionStore interface {
	PutSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, sessionID string) (Session, bool, error)
	UpdateSession(ctx context.Context, s Session) error
	ListActiveSessions(ctx context.Context, tenantID string) ([]Session, error)
}

// GrantStore persists authorization grants. Production will use an
// RDS-backed implementation; tests use InMemoryGrantStore.
type GrantStore interface {
	PutGrant(ctx context.Context, g AuthorizationGrant) error
	GetGrant(ctx context.Context, grantID string) (AuthorizationGrant, bool, error)
	UpdateGrant(ctx context.Context, g AuthorizationGrant) error
	ListGrantsForTenant(ctx context.Context, tenantID string) ([]AuthorizationGrant, error)
}

// NotificationSender sends notifications to customer admins about
// impersonation events. Implementations bridge to the notifications
// package (KAI-376).
type NotificationSender interface {
	// NotifyImpersonationStart tells the customer admin(s) that an
	// impersonation session has begun.
	NotifyImpersonationStart(ctx context.Context, session Session) error

	// NotifyImpersonationEnd tells the customer admin(s) that an
	// impersonation session has ended.
	NotifyImpersonationEnd(ctx context.Context, session Session) error
}

// --- In-memory implementations for testing --------------------------------

// InMemorySessionStore is a concurrency-safe in-memory SessionStore.
type InMemorySessionStore struct {
	mu    sync.RWMutex
	items map[string]Session
}

// NewInMemorySessionStore returns an empty store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{items: make(map[string]Session)}
}

// PutSession inserts a session.
func (s *InMemorySessionStore) PutSession(_ context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sess.SessionID] = sess
	return nil
}

// GetSession returns the session for id, or ok=false.
func (s *InMemorySessionStore) GetSession(_ context.Context, sessionID string) (Session, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.items[sessionID]
	return sess, ok, nil
}

// UpdateSession replaces the session record.
func (s *InMemorySessionStore) UpdateSession(_ context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sess.SessionID] = sess
	return nil
}

// ListActiveSessions returns sessions with StatusActive for the given tenant.
func (s *InMemorySessionStore) ListActiveSessions(_ context.Context, tenantID string) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Session
	for _, sess := range s.items {
		if sess.ImpersonatedTenantID == tenantID && sess.Status == StatusActive {
			out = append(out, sess)
		}
	}
	return out, nil
}

// InMemoryGrantStore is a concurrency-safe in-memory GrantStore.
type InMemoryGrantStore struct {
	mu    sync.RWMutex
	items map[string]AuthorizationGrant
}

// NewInMemoryGrantStore returns an empty store.
func NewInMemoryGrantStore() *InMemoryGrantStore {
	return &InMemoryGrantStore{items: make(map[string]AuthorizationGrant)}
}

// PutGrant inserts a grant.
func (s *InMemoryGrantStore) PutGrant(_ context.Context, g AuthorizationGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[g.GrantID] = g
	return nil
}

// GetGrant returns the grant for id, or ok=false.
func (s *InMemoryGrantStore) GetGrant(_ context.Context, grantID string) (AuthorizationGrant, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.items[grantID]
	return g, ok, nil
}

// UpdateGrant replaces the grant record.
func (s *InMemoryGrantStore) UpdateGrant(_ context.Context, g AuthorizationGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[g.GrantID] = g
	return nil
}

// ListGrantsForTenant returns all grants for the given tenant.
func (s *InMemoryGrantStore) ListGrantsForTenant(_ context.Context, tenantID string) ([]AuthorizationGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []AuthorizationGrant
	for _, g := range s.items {
		if g.TenantID == tenantID {
			out = append(out, g)
		}
	}
	return out, nil
}

// NoopNotificationSender is a test stub that records calls.
type NoopNotificationSender struct {
	mu         sync.Mutex
	StartCalls []Session
	EndCalls   []Session
}

// NewNoopNotificationSender returns a new stub.
func NewNoopNotificationSender() *NoopNotificationSender {
	return &NoopNotificationSender{}
}

// NotifyImpersonationStart records the call.
func (n *NoopNotificationSender) NotifyImpersonationStart(_ context.Context, s Session) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.StartCalls = append(n.StartCalls, s)
	return nil
}

// NotifyImpersonationEnd records the call.
func (n *NoopNotificationSender) NotifyImpersonationEnd(_ context.Context, s Session) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.EndCalls = append(n.EndCalls, s)
	return nil
}

// GetStartCalls returns a copy of start notification calls.
func (n *NoopNotificationSender) GetStartCalls() []Session {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Session, len(n.StartCalls))
	copy(out, n.StartCalls)
	return out
}

// GetEndCalls returns a copy of end notification calls.
func (n *NoopNotificationSender) GetEndCalls() []Session {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Session, len(n.EndCalls))
	copy(out, n.EndCalls)
	return out
}

