package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"sync"
)

// MemoryRecorder is an in-memory Recorder used by tests and development
// scaffolding. It enforces the same tenant-scoping rules as the SQL
// implementation so tests can rely on identical behavior.
type MemoryRecorder struct {
	mu      sync.RWMutex
	entries []Entry
}

// NewMemoryRecorder returns an empty in-memory Recorder.
func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{}
}

// Record validates and appends the entry. If ID is empty, a random one is
// assigned so callers can omit it.
func (m *MemoryRecorder) Record(_ context.Context, entry Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = newID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

// Query returns entries matching filter. It enforces Seam #4 by rejecting
// empty TenantID and by filtering in-memory on TenantID before any other
// predicate.
func (m *MemoryRecorder) Query(_ context.Context, filter QueryFilter) ([]Entry, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []Entry
	for _, e := range m.entries {
		if !matchesTenant(e, filter) {
			continue
		}
		if !matchesFilter(e, filter) {
			continue
		}
		out = append(out, e)
	}
	// Newest-first ordering matches the SQL implementation so pagination
	// semantics are portable between the two.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].ID > out[j].ID
		}
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	if filter.Cursor != "" {
		idx := -1
		for i, e := range out {
			if e.ID == filter.Cursor {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, ErrNotFound
		}
		out = out[idx+1:]
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// Export dispatches to the shared writer.
func (m *MemoryRecorder) Export(ctx context.Context, filter QueryFilter, format ExportFormat, w io.Writer) error {
	return exportEntries(ctx, m, filter, format, w)
}

func matchesTenant(e Entry, f QueryFilter) bool {
	if e.TenantID == f.TenantID {
		return true
	}
	if f.IncludeImpersonatedTenant && e.ImpersonatedTenantID != nil && *e.ImpersonatedTenantID == f.TenantID {
		return true
	}
	return false
}

func matchesFilter(e Entry, f QueryFilter) bool {
	if f.ActorUserID != "" && e.ActorUserID != f.ActorUserID {
		return false
	}
	if f.ActionPattern != "" {
		ok, _ := path.Match(f.ActionPattern, e.Action)
		if !ok {
			return false
		}
	}
	if f.ResourceType != "" && e.ResourceType != f.ResourceType {
		return false
	}
	if f.Result != "" && e.Result != f.Result {
		return false
	}
	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}
	return true
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read failure is a process-wide problem; panicking here makes
		// the audit path impossible to silently corrupt.
		panic(fmt.Sprintf("audit: rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}
