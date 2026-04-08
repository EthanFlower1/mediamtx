package watchlist

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FakeRepository is an in-memory Repository implementation for tests and
// local development. It is NOT safe for production use (no persistence).
type FakeRepository struct {
	mu         sync.Mutex
	watchlists map[string]*Watchlist // key: id
	entries    map[string]*PlateEntry // key: id
}

// NewFakeRepository creates an empty FakeRepository.
func NewFakeRepository() *FakeRepository {
	return &FakeRepository{
		watchlists: make(map[string]*Watchlist),
		entries:    make(map[string]*PlateEntry),
	}
}

func (r *FakeRepository) ListWatchlists(_ context.Context, tenantID string) ([]Watchlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Watchlist
	for _, wl := range r.watchlists {
		if wl.TenantID == tenantID {
			out = append(out, *wl)
		}
	}
	return out, nil
}

func (r *FakeRepository) GetWatchlist(_ context.Context, tenantID, watchlistID string) (*Watchlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wl, ok := r.watchlists[watchlistID]
	if !ok || wl.TenantID != tenantID {
		return nil, fmt.Errorf("%w: watchlist %q", ErrNotFound, watchlistID)
	}
	// Populate entries.
	wlCopy := *wl
	for _, e := range r.entries {
		if e.WatchlistID == watchlistID {
			wlCopy.Entries = append(wlCopy.Entries, *e)
		}
	}
	return &wlCopy, nil
}

func (r *FakeRepository) CreateWatchlist(_ context.Context, wl Watchlist) (*Watchlist, error) {
	if wl.TenantID == "" || wl.Name == "" {
		return nil, fmt.Errorf("%w: tenantID and name required", ErrInvalidInput)
	}
	wl.ID = uuid.New().String()
	wl.CreatedAt = time.Now().UTC()
	wl.UpdatedAt = wl.CreatedAt
	if wl.RetentionDays == 0 {
		wl.RetentionDays = DefaultRetentionDays
	}
	r.mu.Lock()
	r.watchlists[wl.ID] = &wl
	r.mu.Unlock()
	return &wl, nil
}

func (r *FakeRepository) UpdateWatchlist(_ context.Context, wl Watchlist) (*Watchlist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.watchlists[wl.ID]
	if !ok || existing.TenantID != wl.TenantID {
		return nil, fmt.Errorf("%w: watchlist %q", ErrNotFound, wl.ID)
	}
	existing.Name = wl.Name
	existing.Type = wl.Type
	existing.RetentionDays = wl.RetentionDays
	existing.UpdatedAt = time.Now().UTC()
	return existing, nil
}

func (r *FakeRepository) DeleteWatchlist(_ context.Context, tenantID, watchlistID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	wl, ok := r.watchlists[watchlistID]
	if !ok || wl.TenantID != tenantID {
		return fmt.Errorf("%w: watchlist %q", ErrNotFound, watchlistID)
	}
	delete(r.watchlists, watchlistID)
	for id, e := range r.entries {
		if e.WatchlistID == watchlistID {
			delete(r.entries, id)
		}
	}
	return nil
}

func (r *FakeRepository) AddEntry(_ context.Context, tenantID string, entry PlateEntry) (*PlateEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wl, ok := r.watchlists[entry.WatchlistID]
	if !ok || wl.TenantID != tenantID {
		return nil, fmt.Errorf("%w: watchlist %q", ErrNotFound, entry.WatchlistID)
	}
	entry.ID = uuid.New().String()
	entry.CreatedAt = time.Now().UTC()
	r.entries[entry.ID] = &entry
	return &entry, nil
}

func (r *FakeRepository) RemoveEntry(_ context.Context, tenantID, watchlistID, entryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	wl, ok := r.watchlists[watchlistID]
	if !ok || wl.TenantID != tenantID {
		return fmt.Errorf("%w: watchlist %q", ErrNotFound, watchlistID)
	}
	e, ok := r.entries[entryID]
	if !ok || e.WatchlistID != watchlistID {
		return fmt.Errorf("%w: entry %q", ErrNotFound, entryID)
	}
	delete(r.entries, entryID)
	return nil
}

func (r *FakeRepository) ListEntries(_ context.Context, tenantID, watchlistID string) ([]PlateEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wl, ok := r.watchlists[watchlistID]
	if !ok || wl.TenantID != tenantID {
		return nil, fmt.Errorf("%w: watchlist %q", ErrNotFound, watchlistID)
	}
	var out []PlateEntry
	now := time.Now()
	for _, e := range r.entries {
		if e.WatchlistID != watchlistID {
			continue
		}
		if e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

func (r *FakeRepository) AllActivePlates(_ context.Context, tenantID string) ([]activeEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var out []activeEntry
	for _, e := range r.entries {
		wl, ok := r.watchlists[e.WatchlistID]
		if !ok || wl.TenantID != tenantID {
			continue
		}
		if e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, activeEntry{
			WatchlistID: e.WatchlistID,
			EntryID:     e.ID,
			PlateText:   e.PlateText,
			Type:        wl.Type,
		})
	}
	return out, nil
}
