package watchlist

import (
	"context"
	"sync"
)

// MatcherRegistry manages a Matcher per tenant, creating them on-demand. It
// provides the cache-invalidation entry point called by the River job queue
// (KAI-234) when a watchlist is mutated on the cloud.
type MatcherRegistry struct {
	repo Repository

	mu       sync.Mutex
	matchers map[string]*Matcher // keyed by tenantID
}

// NewMatcherRegistry creates a new registry backed by repo.
func NewMatcherRegistry(repo Repository) *MatcherRegistry {
	return &MatcherRegistry{
		repo:     repo,
		matchers: make(map[string]*Matcher),
	}
}

// GetMatcher returns the Matcher for tenantID. If one does not exist, it is
// created and its cache rebuilt before returning. The returned Matcher is
// safe to use until InvalidateCache is called.
func (r *MatcherRegistry) GetMatcher(ctx context.Context, tenantID string) (*Matcher, error) {
	r.mu.Lock()
	m, ok := r.matchers[tenantID]
	if !ok {
		m = NewMatcher(tenantID, r.repo)
		r.matchers[tenantID] = m
	}
	r.mu.Unlock()

	// RebuildCache is idempotent and can be called concurrently from multiple
	// goroutines; the Matcher's internal RWMutex protects the filter swap.
	if err := m.RebuildCache(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// InvalidateCache forces a cache rebuild for tenantID. Called by the River
// job consumer when the cloud API mutates a watchlist. If no Matcher exists
// for the tenant, this is a no-op.
func (r *MatcherRegistry) InvalidateCache(ctx context.Context, tenantID string) error {
	r.mu.Lock()
	m, ok := r.matchers[tenantID]
	r.mu.Unlock()

	if !ok {
		return nil
	}
	return m.RebuildCache(ctx)
}
