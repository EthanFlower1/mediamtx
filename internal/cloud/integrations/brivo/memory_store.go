package brivo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// MemoryTokenStore — in-memory TokenStore for tests
// ---------------------------------------------------------------------------

// MemoryTokenStore is an in-memory TokenStore suitable for unit tests.
type MemoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]TokenPair // keyed by tenantID
}

// NewMemoryTokenStore constructs a MemoryTokenStore.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{tokens: make(map[string]TokenPair)}
}

func (s *MemoryTokenStore) StoreToken(_ context.Context, tenantID string, token TokenPair) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tenantID] = token
	return nil
}

func (s *MemoryTokenStore) GetToken(_ context.Context, tenantID string) (TokenPair, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.tokens[tenantID]
	if !ok {
		return TokenPair{}, ErrNotConnected
	}
	return tok, nil
}

func (s *MemoryTokenStore) DeleteToken(_ context.Context, tenantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, tenantID)
	return nil
}

// ---------------------------------------------------------------------------
// MemoryConnectionStore — in-memory ConnectionStore for tests
// ---------------------------------------------------------------------------

// MemoryConnectionStore is an in-memory ConnectionStore.
type MemoryConnectionStore struct {
	mu    sync.RWMutex
	conns map[string]Connection // keyed by tenantID
}

// NewMemoryConnectionStore constructs a MemoryConnectionStore.
func NewMemoryConnectionStore() *MemoryConnectionStore {
	return &MemoryConnectionStore{conns: make(map[string]Connection)}
}

func (s *MemoryConnectionStore) Upsert(_ context.Context, conn Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[conn.TenantID] = conn
	return nil
}

func (s *MemoryConnectionStore) Get(_ context.Context, tenantID string) (*Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, ok := s.conns[tenantID]
	if !ok {
		return nil, ErrNotConnected
	}
	return &conn, nil
}

func (s *MemoryConnectionStore) Delete(_ context.Context, tenantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, tenantID)
	return nil
}

func (s *MemoryConnectionStore) UpdateSyncTime(_ context.Context, tenantID string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, ok := s.conns[tenantID]
	if !ok {
		return ErrNotConnected
	}
	conn.LastSyncAt = &t
	conn.UpdatedAt = t
	s.conns[tenantID] = conn
	return nil
}

// ---------------------------------------------------------------------------
// MemoryMappingStore — in-memory MappingStore for tests
// ---------------------------------------------------------------------------

// MemoryMappingStore is an in-memory MappingStore.
type MemoryMappingStore struct {
	mu       sync.RWMutex
	mappings []DoorCameraMapping
}

// NewMemoryMappingStore constructs a MemoryMappingStore.
func NewMemoryMappingStore() *MemoryMappingStore {
	return &MemoryMappingStore{}
}

func (s *MemoryMappingStore) Set(_ context.Context, m DoorCameraMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Upsert by ID.
	for i, existing := range s.mappings {
		if existing.ID == m.ID && existing.TenantID == m.TenantID {
			s.mappings[i] = m
			return nil
		}
	}
	s.mappings = append(s.mappings, m)
	return nil
}

func (s *MemoryMappingStore) ListByTenant(_ context.Context, tenantID string) ([]DoorCameraMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []DoorCameraMapping
	for _, m := range s.mappings {
		if m.TenantID == tenantID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *MemoryMappingStore) ListByDoor(_ context.Context, tenantID, brivoDoorID string) ([]DoorCameraMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []DoorCameraMapping
	for _, m := range s.mappings {
		if m.TenantID == tenantID && m.BrivoDoorID == brivoDoorID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *MemoryMappingStore) Delete(_ context.Context, tenantID, mappingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.mappings {
		if m.TenantID == tenantID && m.ID == mappingID {
			s.mappings = append(s.mappings[:i], s.mappings[i+1:]...)
			return nil
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// MemoryEventLog — in-memory EventLog for tests
// ---------------------------------------------------------------------------

// MemoryEventLog is an in-memory EventLog.
type MemoryEventLog struct {
	mu     sync.RWMutex
	events []DoorEvent
}

// NewMemoryEventLog constructs a MemoryEventLog.
func NewMemoryEventLog() *MemoryEventLog {
	return &MemoryEventLog{}
}

func (s *MemoryEventLog) Append(_ context.Context, event DoorEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *MemoryEventLog) ListByTenant(_ context.Context, tenantID string, from, to time.Time, limit int) ([]DoorEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []DoorEvent
	for _, e := range s.events {
		if e.TenantID == tenantID &&
			!e.OccurredAt.Before(from) &&
			!e.OccurredAt.After(to) {
			out = append(out, e)
		}
	}

	// Sort by occurred_at descending.
	sort.Slice(out, func(i, j int) bool {
		return out[i].OccurredAt.After(out[j].OccurredAt)
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
