// Package watchlist implements per-tenant license plate watchlists with
// bloom-filter-backed hot-path matching.
//
// Architecture
//
// Each tenant has zero or more named Watchlists (allow / deny / alert type).
// A Watchlist contains a set of PlateEntry values, each holding a normalised
// plate text and optional metadata (label, expiry).
//
// Hot-path matching uses an in-memory bloom filter per tenant so that the
// common "plate not on any watchlist" case does not require a DB round-trip.
// The bloom filter is rebuilt whenever a watchlist is mutated via the CRUD
// service. Cache invalidation is triggered through the River job queue
// (KAI-234). The conservative false-positive rate (0.1 %) means an occasional
// spurious DB lookup; false negatives are impossible by design.
//
// Retention
//
// Every Watchlist carries a RetentionDays field (default 90). A background
// sweeper (not in this package) deletes reads older than the retention window.
// This satisfies the EU AI Act ancillary requirement for data minimisation
// even though LPR itself is not classified as high-risk.
//
// Multi-tenant isolation
//
// The Matcher is keyed by tenant_id everywhere. There is no shared state
// between tenant bloom filters. The Repository interface MUST scope every
// query by tenant_id.
package watchlist

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// WatchlistType controls the action taken when a plate matches.
type WatchlistType string

const (
	// TypeAllow means the plate is explicitly permitted (e.g., authorised
	// residents). The system takes no alert action on a match.
	TypeAllow WatchlistType = "allow"

	// TypeDeny means the plate should be blocked / access denied. The pipeline
	// fires a WatchlistMatchEvent and the notification service is invoked
	// (KAI-370 stub).
	TypeDeny WatchlistType = "deny"

	// TypeAlert means an informational alert is raised without blocking.
	TypeAlert WatchlistType = "alert"
)

// DefaultRetentionDays is the default lpr_reads retention for a watchlist.
const DefaultRetentionDays = 90

// Watchlist is the aggregate holding one named set of plates for a tenant.
type Watchlist struct {
	// ID is the opaque UUID primary key.
	ID string

	// TenantID scopes this watchlist to a single tenant. Every query MUST
	// filter by this field.
	TenantID string

	// Name is the human-readable label shown in the UI.
	Name string

	// Type controls the alert action on match.
	Type WatchlistType

	// Entries is the full plate entry set. Populated on full loads; may
	// be nil for index-only queries.
	Entries []PlateEntry

	// RetentionDays is the number of days lpr_reads matching this watchlist
	// are retained. 0 defaults to DefaultRetentionDays.
	RetentionDays int

	// CreatedAt / UpdatedAt are set by the persistence layer.
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PlateEntry is a single plate text entry within a Watchlist.
type PlateEntry struct {
	// ID is the opaque UUID primary key.
	ID string

	// WatchlistID is the parent watchlist FK.
	WatchlistID string

	// PlateText is the normalised plate text (upper-case, no separators).
	PlateText string

	// Label is an optional human-readable note (e.g. "suspect vehicle").
	Label string

	// ExpiresAt, when non-zero, marks the entry invalid after this time.
	// The bloom filter rebuild skips expired entries.
	ExpiresAt *time.Time

	// CreatedAt is set by the persistence layer.
	CreatedAt time.Time
}

// MatchResult is returned by Matcher.Match when a plate is found on a
// watchlist.
type MatchResult struct {
	// WatchlistID is the matched watchlist's ID.
	WatchlistID string

	// EntryID is the specific PlateEntry that matched.
	EntryID string

	// Type is the watchlist type for routing (allow / deny / alert).
	Type WatchlistType
}

// Sentinel errors.
var (
	// ErrNotFound is returned when a watchlist or entry does not exist.
	ErrNotFound = errors.New("watchlist: not found")

	// ErrInvalidInput is returned for malformed inputs.
	ErrInvalidInput = errors.New("watchlist: invalid input")

	// ErrTenantMismatch is returned when an operation attempts to cross
	// tenant boundaries (defence-in-depth over the Casbin policy layer).
	ErrTenantMismatch = errors.New("watchlist: tenant mismatch")
)

// Repository is the storage interface the Matcher and CRUD service operate
// against. Every method MUST scope results by tenantID.
//
// The concrete implementation is wired up against the cloud/db package by the
// apiserver registration layer (KAI-226 pattern). Unit tests substitute a
// fake in-memory repository.
type Repository interface {
	// ListWatchlists returns all watchlists for the tenant (entries omitted).
	ListWatchlists(ctx context.Context, tenantID string) ([]Watchlist, error)

	// GetWatchlist returns a single watchlist with its entries populated.
	GetWatchlist(ctx context.Context, tenantID, watchlistID string) (*Watchlist, error)

	// CreateWatchlist creates a new watchlist. The ID field of the supplied
	// struct is ignored and replaced by the generated UUID.
	CreateWatchlist(ctx context.Context, wl Watchlist) (*Watchlist, error)

	// UpdateWatchlist updates the name, type and retention of a watchlist.
	UpdateWatchlist(ctx context.Context, wl Watchlist) (*Watchlist, error)

	// DeleteWatchlist removes the watchlist and all its entries.
	DeleteWatchlist(ctx context.Context, tenantID, watchlistID string) error

	// AddEntry appends a PlateEntry to the watchlist.
	AddEntry(ctx context.Context, tenantID string, entry PlateEntry) (*PlateEntry, error)

	// RemoveEntry removes a single entry.
	RemoveEntry(ctx context.Context, tenantID, watchlistID, entryID string) error

	// ListEntries returns all non-expired entries for the watchlist.
	ListEntries(ctx context.Context, tenantID, watchlistID string) ([]PlateEntry, error)

	// AllActivePlates returns every non-expired plate text for the tenant
	// across all watchlists, with their associated watchlist ID and type.
	// Used exclusively to rebuild the bloom filter.
	AllActivePlates(ctx context.Context, tenantID string) ([]activeEntry, error)
}

// activeEntry is the minimal shape returned by Repository.AllActivePlates.
type activeEntry struct {
	WatchlistID string
	EntryID     string
	PlateText   string
	Type        WatchlistType
}

// Matcher provides bloom-filter-backed watchlist matching for a single tenant.
// Instances are created by the MatcherRegistry and must not be shared across
// tenants.
type Matcher struct {
	tenantID string
	repo     Repository

	mu     sync.RWMutex
	filter *bloomFilter  // rebuilt on InvalidateCache
	index  []activeEntry // full entry list for post-filter exact lookup
}

// NewMatcher creates a Matcher for tenantID. Call RebuildCache before using
// Match.
func NewMatcher(tenantID string, repo Repository) *Matcher {
	return &Matcher{tenantID: tenantID, repo: repo}
}

// RebuildCache (re)builds the bloom filter and in-memory exact-match index
// from the repository. Should be called at startup and after any watchlist
// mutation.
func (m *Matcher) RebuildCache(ctx context.Context) error {
	entries, err := m.repo.AllActivePlates(ctx, m.tenantID)
	if err != nil {
		return fmt.Errorf("watchlist: rebuild cache for tenant %q: %w", m.tenantID, err)
	}

	bf := newBloomFilter(len(entries)+1024, 0.001) // 0.1% FP rate
	for _, e := range entries {
		bf.Add(e.PlateText)
	}

	m.mu.Lock()
	m.filter = bf
	m.index = entries
	m.mu.Unlock()
	return nil
}

// Match checks whether normalised plate text appears on any watchlist for
// this tenant. Returns nil if no match is found or if the bloom filter has
// not been built yet (graceful degradation — falls back to DB lookup).
//
// The hot path is:
//  1. Bloom filter query — O(k) hash ops, no allocation.
//  2. If bloom reports possibly present, exact scan of m.index.
//  3. On index miss (false positive), return nil.
func (m *Matcher) Match(ctx context.Context, tenantID, plateText string) (*MatchResult, error) {
	if tenantID != m.tenantID {
		return nil, ErrTenantMismatch
	}

	m.mu.RLock()
	bf := m.filter
	index := m.index
	m.mu.RUnlock()

	if bf == nil {
		// Cache not yet built — perform direct DB lookup (slow path, only
		// during startup / first call).
		return m.dbLookup(ctx, plateText)
	}

	if !bf.Test(plateText) {
		return nil, nil // definitive miss
	}

	// Bloom reported possibly present — do exact scan.
	for _, e := range index {
		if e.PlateText == plateText {
			return &MatchResult{
				WatchlistID: e.WatchlistID,
				EntryID:     e.EntryID,
				Type:        e.Type,
			}, nil
		}
	}
	// False positive.
	return nil, nil
}

// dbLookup is the slow-path fallback when the cache is empty. It queries the
// repository directly. This should only occur during startup.
func (m *Matcher) dbLookup(ctx context.Context, plateText string) (*MatchResult, error) {
	entries, err := m.repo.AllActivePlates(ctx, m.tenantID)
	if err != nil {
		return nil, fmt.Errorf("watchlist: db lookup: %w", err)
	}
	for _, e := range entries {
		if e.PlateText == plateText {
			return &MatchResult{
				WatchlistID: e.WatchlistID,
				EntryID:     e.EntryID,
				Type:        e.Type,
			}, nil
		}
	}
	return nil, nil
}
