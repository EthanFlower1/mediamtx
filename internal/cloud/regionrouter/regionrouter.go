// Package regionrouter implements region-scoped URL routing for the Kaivue
// cloud control plane (KAI-230). It provides:
//
//   - Host-header parsing: "us-east-2.api.yourbrand.com" → "us-east-2"
//   - Allowlist validation against the canonical region table
//   - Tenant home-region lookup with an in-memory TTL cache (Redis stub;
//     real Redis wired in KAI-217 follow-up)
//   - Context helpers for downstream middleware and handlers
//
// Architectural seam #9: build for multi-region, ship single-region. v1 has
// only us-east-2; all the routing paths exist so adding a second region in
// v1.x is uncommenting a config line and running a DB migration.
//
// Package imports: internal/shared/* and internal/cloud/db/* only.
// Never import a concrete identity adapter from this package.
package regionrouter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// KnownRegions is the canonical allowlist for v1. Adding a region here (and
// deploying the matching apiserver instance + DNS entry via KAI-231) is all
// that is required to enable it.
var KnownRegions = []string{
	"us-east-2",
}

// DefaultRegion is the single region v1 ships with. Seam #9: do not hard-code
// this anywhere but here and internal/cloud/db.DefaultRegion.
const DefaultRegion = "us-east-2"

// BaseURLForRegion returns the canonical HTTPS base URL for a region.
// v1 has one region; the map grows as regions are added.
var BaseURLForRegion = map[string]string{
	"us-east-2": "https://us-east-2.api.yourbrand.com",
}

// ctxKey is a private type to prevent collisions with other packages.
type ctxKey int

const (
	ctxKeyRegion ctxKey = iota
	ctxKeyTenantHomeRegion
)

// WithRegion attaches the resolved region to ctx.
func WithRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, ctxKeyRegion, region)
}

// RegionFromContext retrieves the region injected by the middleware.
func RegionFromContext(ctx context.Context) (string, bool) {
	r, ok := ctx.Value(ctxKeyRegion).(string)
	return r, ok && r != ""
}

// WithTenantHomeRegion attaches the resolved home region to ctx.
func WithTenantHomeRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantHomeRegion, region)
}

// TenantHomeRegionFromContext retrieves the tenant's home region.
func TenantHomeRegionFromContext(ctx context.Context) (string, bool) {
	r, ok := ctx.Value(ctxKeyTenantHomeRegion).(string)
	return r, ok && r != ""
}

// -----------------------------------------------------------------------
// Host-header parsing
// -----------------------------------------------------------------------

// ParseRegionFromHost extracts the AWS region segment from a Kaivue API host.
// Expected format: "<region>.api.<domain>" e.g. "us-east-2.api.yourbrand.com".
//
// Returns ("", false) for any host that does not match the pattern, including
// direct IP access, localhost, or unknown subdomain patterns.
func ParseRegionFromHost(host string) (string, bool) {
	// Strip optional :port suffix.
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		// Ensure it is a port and not an IPv6 colon.
		if !strings.Contains(host[:i], "]") {
			host = host[:i]
		}
	}

	// Must have at least 3 labels: <region>.api.<domain>.
	parts := strings.SplitN(host, ".", 3)
	if len(parts) < 3 {
		return "", false
	}
	// Second segment must be "api" (or "api-int" for internal load-balancer
	// variants used in VPC-only deployments).
	second := strings.ToLower(parts[1])
	if second != "api" && second != "api-int" {
		return "", false
	}
	return parts[0], true
}

// -----------------------------------------------------------------------
// Region allowlist
// -----------------------------------------------------------------------

// IsAllowedRegion reports whether r is in the platform's canonical region
// table. The check is O(n) over a tiny slice — there will never be more than
// ~20 regions in the foreseeable future, so a map is unnecessary complexity.
func IsAllowedRegion(r string) bool {
	for _, known := range KnownRegions {
		if r == known {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Region resolver — combines parse + validate + context injection
// -----------------------------------------------------------------------

// Resolver validates a host header against the allowlist and injects the
// region into the request context. It is the single entry point called by the
// middleware.
type Resolver struct {
	// LocalRegion is the region this server instance serves.
	LocalRegion string
	// AllowedRegions overrides KnownRegions. Nil means use KnownRegions.
	AllowedRegions []string
}

// allowed returns the effective allowlist.
func (r *Resolver) allowed() []string {
	if r.AllowedRegions != nil {
		return r.AllowedRegions
	}
	return KnownRegions
}

// ResolveHost parses the host header and validates it. Returns the resolved
// region and true on success, or ("", false) for hosts that carry no region
// signal (e.g. direct IP, localhost). Returns an error for hosts that carry a
// region signal that is not in the allowlist.
func (r *Resolver) ResolveHost(host string) (region string, hasRegion bool, err error) {
	region, ok := ParseRegionFromHost(host)
	if !ok {
		return "", false, nil
	}
	for _, a := range r.allowed() {
		if region == a {
			return region, true, nil
		}
	}
	return "", true, fmt.Errorf("regionrouter: unknown region %q", region)
}

// -----------------------------------------------------------------------
// Tenant home-region lookup with TTL cache
// -----------------------------------------------------------------------

// TenantLookup is the abstraction the cache falls back to on a miss. It is
// satisfied by *db.DB via a small adapter in the middleware layer, keeping
// this package free of direct DB imports.
type TenantLookup interface {
	// LookupHomeRegion returns the home region for a tenant ID.
	// It must return ("", ErrTenantNotFound) for unknown tenants.
	LookupHomeRegion(ctx context.Context, tenantID string) (string, error)
}

// ErrTenantNotFound is returned by TenantLookup implementations when no
// tenant matches the given id.
var ErrTenantNotFound = fmt.Errorf("regionrouter: tenant not found")

// cacheEntry holds one cached lookup.
type cacheEntry struct {
	region    string
	expiresAt time.Time
}

// Cache is a thread-safe, TTL-keyed, tenant → home-region cache. It is the
// KAI-217 Redis stub: the interface is identical; swap the backing store for
// Redis without changing callers.
//
// Multi-tenant isolation: keys are tenant IDs. An entry for tenant A can never
// satisfy a lookup for tenant B because map keys are compared exactly.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	// now is injectable for testing.
	now func() time.Time
}

// NewCache constructs a Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// Get returns the cached region for tenantID, if present and not expired.
func (c *Cache) Get(tenantID string) (string, bool) {
	c.mu.RLock()
	e, ok := c.entries[tenantID]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if c.now().After(e.expiresAt) {
		c.mu.Lock()
		delete(c.entries, tenantID)
		c.mu.Unlock()
		return "", false
	}
	return e.region, true
}

// Set stores a region for tenantID with the configured TTL.
func (c *Cache) Set(tenantID, region string) {
	c.mu.Lock()
	c.entries[tenantID] = cacheEntry{
		region:    region,
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate removes a tenant's cached entry. Call this after a tenant region
// migration (future: KAI-231 follow-up).
func (c *Cache) Invalidate(tenantID string) {
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}

// TenantRegionResolver wraps a TenantLookup with a Cache. It is the
// authoritative answer to "what region does tenant X live in?"
//
// Multi-tenant guarantee: each LookupHomeRegion call is scoped to one tenantID.
// The cache key is the tenantID. There is no path by which tenant A's lookup
// can populate or read tenant B's entry.
type TenantRegionResolver struct {
	cache  *Cache
	lookup TenantLookup
}

// NewTenantRegionResolver constructs a TenantRegionResolver with the given
// cache and database-backed lookup.
func NewTenantRegionResolver(cache *Cache, lookup TenantLookup) *TenantRegionResolver {
	return &TenantRegionResolver{cache: cache, lookup: lookup}
}

// LookupHomeRegion returns the home region for tenantID, consulting the cache
// first and the backing store on a miss.
func (r *TenantRegionResolver) LookupHomeRegion(ctx context.Context, tenantID string) (string, error) {
	if region, ok := r.cache.Get(tenantID); ok {
		return region, nil
	}
	region, err := r.lookup.LookupHomeRegion(ctx, tenantID)
	if err != nil {
		return "", err
	}
	r.cache.Set(tenantID, region)
	return region, nil
}
