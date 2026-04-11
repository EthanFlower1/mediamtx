package openpath

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory EventStore for tests and development.
type MemoryStore struct {
	mu     sync.Mutex
	events []DoorEvent
	clips  []CorrelatedClip
}

// NewMemoryStore returns a ready-to-use in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// SaveDoorEvent persists a door event. Duplicate IDs are silently ignored.
func (m *MemoryStore) SaveDoorEvent(_ context.Context, ev DoorEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.events {
		if existing.ID == ev.ID {
			return nil // idempotent
		}
	}
	m.events = append(m.events, ev)
	return nil
}

// SaveCorrelatedClip persists a correlation record.
func (m *MemoryStore) SaveCorrelatedClip(_ context.Context, clip CorrelatedClip) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clips = append(m.clips, clip)
	return nil
}

// ListDoorEvents returns events for a tenant in a time range, newest first.
func (m *MemoryStore) ListDoorEvents(_ context.Context, tenantID string, from, to time.Time) ([]DoorEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []DoorEvent
	for _, ev := range m.events {
		if ev.TenantID == tenantID && !ev.Timestamp.Before(from) && !ev.Timestamp.After(to) {
			result = append(result, ev)
		}
	}
	// Reverse for descending order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// Events returns all stored events (test helper).
func (m *MemoryStore) Events() []DoorEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DoorEvent, len(m.events))
	copy(out, m.events)
	return out
}

// Clips returns all stored correlation records (test helper).
func (m *MemoryStore) Clips() []CorrelatedClip {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CorrelatedClip, len(m.clips))
	copy(out, m.clips)
	return out
}
