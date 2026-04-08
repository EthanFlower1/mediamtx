package regionrouter_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/regionrouter"
)

// -----------------------------------------------------------------------
// ParseRegionFromHost
// -----------------------------------------------------------------------

func TestParseRegionFromHost(t *testing.T) {
	tests := []struct {
		host      string
		wantRegion string
		wantOK    bool
	}{
		{"us-east-2.api.yourbrand.com", "us-east-2", true},
		{"eu-west-1.api.yourbrand.com", "eu-west-1", true},
		{"us-east-2.api.yourbrand.com:8443", "us-east-2", true},
		{"us-west-2.api-int.yourbrand.com", "us-west-2", true},
		// No region signal
		{"localhost", "", false},
		{"localhost:8080", "", false},
		{"192.168.1.1", "", false},
		{"api.yourbrand.com", "", false},
		// Only two labels — not enough
		{"us-east-2.yourbrand.com", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			got, ok := regionrouter.ParseRegionFromHost(tc.host)
			if ok != tc.wantOK {
				t.Fatalf("ParseRegionFromHost(%q): ok=%v, want %v", tc.host, ok, tc.wantOK)
			}
			if got != tc.wantRegion {
				t.Fatalf("ParseRegionFromHost(%q): region=%q, want %q", tc.host, got, tc.wantRegion)
			}
		})
	}
}

// -----------------------------------------------------------------------
// IsAllowedRegion
// -----------------------------------------------------------------------

func TestIsAllowedRegion(t *testing.T) {
	if !regionrouter.IsAllowedRegion("us-east-2") {
		t.Fatal("us-east-2 should be allowed")
	}
	if regionrouter.IsAllowedRegion("us-west-2") {
		t.Fatal("us-west-2 should not be allowed in v1")
	}
	if regionrouter.IsAllowedRegion("") {
		t.Fatal("empty string should not be allowed")
	}
}

// -----------------------------------------------------------------------
// Resolver
// -----------------------------------------------------------------------

func TestResolver_ResolveHost(t *testing.T) {
	r := &regionrouter.Resolver{
		LocalRegion:    "us-east-2",
		AllowedRegions: []string{"us-east-2", "eu-west-1"},
	}

	t.Run("valid local region", func(t *testing.T) {
		region, has, err := r.ResolveHost("us-east-2.api.yourbrand.com")
		if err != nil || !has || region != "us-east-2" {
			t.Fatalf("got (%q, %v, %v)", region, has, err)
		}
	})

	t.Run("valid non-local allowed region", func(t *testing.T) {
		region, has, err := r.ResolveHost("eu-west-1.api.yourbrand.com")
		if err != nil || !has || region != "eu-west-1" {
			t.Fatalf("got (%q, %v, %v)", region, has, err)
		}
	})

	t.Run("unknown region returns error", func(t *testing.T) {
		_, _, err := r.ResolveHost("ap-southeast-1.api.yourbrand.com")
		if err == nil {
			t.Fatal("expected error for unknown region")
		}
	})

	t.Run("no region in host returns false", func(t *testing.T) {
		_, has, err := r.ResolveHost("localhost:8080")
		if err != nil || has {
			t.Fatalf("got (has=%v, err=%v), want (false, nil)", has, err)
		}
	})
}

// -----------------------------------------------------------------------
// Cache
// -----------------------------------------------------------------------

func TestCache_HitAndMiss(t *testing.T) {
	cache := regionrouter.NewCache(10 * time.Second)

	if _, ok := cache.Get("tenant-a"); ok {
		t.Fatal("cold cache should miss")
	}

	cache.Set("tenant-a", "us-east-2")

	region, ok := cache.Get("tenant-a")
	if !ok || region != "us-east-2" {
		t.Fatalf("expected hit for tenant-a, got (%q, %v)", region, ok)
	}
}

func TestCache_Expiry(t *testing.T) {
	var nowMu sync.Mutex
	now := time.Now()
	cache := &regionrouter.Cache{}
	*cache = *regionrouter.NewCache(100 * time.Millisecond)
	// Inject controllable clock via exported field on a test-only cache wrapper.
	// Since Cache.now is unexported we use the public TTL-based expiry test
	// via real time with a very short TTL.
	shortCache := regionrouter.NewCache(50 * time.Millisecond)
	_ = now
	_ = nowMu

	shortCache.Set("tenant-x", "us-east-2")
	region, ok := shortCache.Get("tenant-x")
	if !ok || region != "us-east-2" {
		t.Fatal("expected hit immediately after set")
	}

	time.Sleep(75 * time.Millisecond)
	_, ok = shortCache.Get("tenant-x")
	if ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestCache_Invalidate(t *testing.T) {
	cache := regionrouter.NewCache(10 * time.Second)
	cache.Set("tenant-b", "us-east-2")
	cache.Invalidate("tenant-b")
	if _, ok := cache.Get("tenant-b"); ok {
		t.Fatal("expected miss after invalidation")
	}
}

// TestCache_MultiTenantIsolation verifies that tenant A's cache entry cannot
// satisfy a lookup for tenant B — the canonical multi-tenant isolation test
// for the cache layer.
func TestCache_MultiTenantIsolation(t *testing.T) {
	cache := regionrouter.NewCache(10 * time.Second)
	cache.Set("tenant-a", "us-east-2")

	_, ok := cache.Get("tenant-b")
	if ok {
		t.Fatal("ISOLATION VIOLATION: tenant-b lookup returned tenant-a's cached region")
	}
}

// -----------------------------------------------------------------------
// TenantRegionResolver
// -----------------------------------------------------------------------

type stubLookup struct {
	mu   sync.Mutex
	data map[string]string
	hits map[string]int
}

func newStubLookup(data map[string]string) *stubLookup {
	return &stubLookup{data: data, hits: make(map[string]int)}
}

func (s *stubLookup) LookupHomeRegion(_ context.Context, tenantID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[tenantID]++
	if r, ok := s.data[tenantID]; ok {
		return r, nil
	}
	return "", regionrouter.ErrTenantNotFound
}

func TestTenantRegionResolver_HappyPath(t *testing.T) {
	stub := newStubLookup(map[string]string{"acme": "us-east-2"})
	resolver := regionrouter.NewTenantRegionResolver(regionrouter.NewCache(10*time.Second), stub)

	region, err := resolver.LookupHomeRegion(context.Background(), "acme")
	if err != nil || region != "us-east-2" {
		t.Fatalf("expected us-east-2, got (%q, %v)", region, err)
	}
	// Second call must hit the cache, not the stub.
	_, _ = resolver.LookupHomeRegion(context.Background(), "acme")
	stub.mu.Lock()
	hits := stub.hits["acme"]
	stub.mu.Unlock()
	if hits != 1 {
		t.Fatalf("expected 1 stub hit (cache should absorb second call), got %d", hits)
	}
}

func TestTenantRegionResolver_CacheMiss_FallsThrough(t *testing.T) {
	stub := newStubLookup(map[string]string{"globex": "us-east-2"})
	resolver := regionrouter.NewTenantRegionResolver(regionrouter.NewCache(10*time.Second), stub)

	_, err := resolver.LookupHomeRegion(context.Background(), "unknown-tenant")
	if !errors.Is(err, regionrouter.ErrTenantNotFound) {
		t.Fatalf("expected ErrTenantNotFound, got %v", err)
	}
}

// TestTenantRegionResolver_IsolationAcrossTenants is the multi-tenant isolation
// test: tenant A's successful lookup must not bleed into tenant B.
func TestTenantRegionResolver_IsolationAcrossTenants(t *testing.T) {
	stub := newStubLookup(map[string]string{
		"tenant-a": "us-east-2",
	})
	resolver := regionrouter.NewTenantRegionResolver(regionrouter.NewCache(10*time.Second), stub)

	// Populate cache for tenant-a.
	_, err := resolver.LookupHomeRegion(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("unexpected error for tenant-a: %v", err)
	}

	// tenant-b must not find tenant-a's region.
	_, err = resolver.LookupHomeRegion(context.Background(), "tenant-b")
	if !errors.Is(err, regionrouter.ErrTenantNotFound) {
		t.Fatalf("ISOLATION VIOLATION: tenant-b lookup returned result from tenant-a's scope; err=%v", err)
	}
}

// -----------------------------------------------------------------------
// CrossRegionRedirectHandler
// -----------------------------------------------------------------------

func TestCrossRegionRedirectHandler_SameRegion_PassesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := regionrouter.CrossRegionRedirectHandler("us-east-2", inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/cameras", nil)
	ctx := regionrouter.WithTenantHomeRegion(req.Context(), "us-east-2")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestCrossRegionRedirectHandler_DifferentRegion_Redirects(t *testing.T) {
	// Inject eu-west-1 into the BaseURLForRegion map for this test.
	regionrouter.BaseURLForRegion["eu-west-1"] = "https://eu-west-1.api.yourbrand.com"
	defer delete(regionrouter.BaseURLForRegion, "eu-west-1")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be called on cross-region redirect")
	})
	h := regionrouter.CrossRegionRedirectHandler("us-east-2", inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/cameras?page=1", nil)
	ctx := regionrouter.WithTenantHomeRegion(req.Context(), "eu-west-1")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	want := "https://eu-west-1.api.yourbrand.com/v1/cameras?page=1"
	if loc != want {
		t.Fatalf("Location: got %q, want %q", loc, want)
	}
}

func TestCrossRegionRedirectHandler_NoHomeRegion_421(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be called")
	})
	h := regionrouter.CrossRegionRedirectHandler("us-east-2", inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/cameras", nil)
	// No home region injected into context.

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMisdirectedRequest {
		t.Fatalf("expected 421, got %d", rr.Code)
	}
}

// -----------------------------------------------------------------------
// HealthHandler
// -----------------------------------------------------------------------

func TestHealthHandler_NoParam_Returns200(t *testing.T) {
	h := regionrouter.HealthHandler("us-east-2")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealthHandler_CorrectRegion_Returns200(t *testing.T) {
	h := regionrouter.HealthHandler("us-east-2")
	req := httptest.NewRequest(http.MethodGet, "/healthz?region=us-east-2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealthHandler_KnownPeerRegion_Redirects(t *testing.T) {
	regionrouter.BaseURLForRegion["eu-west-1"] = "https://eu-west-1.api.yourbrand.com"
	regionrouter.KnownRegions = append(regionrouter.KnownRegions, "eu-west-1")
	defer func() {
		delete(regionrouter.BaseURLForRegion, "eu-west-1")
		regionrouter.KnownRegions = regionrouter.KnownRegions[:1] // back to just us-east-2
	}()

	h := regionrouter.HealthHandler("us-east-2")
	req := httptest.NewRequest(http.MethodGet, "/healthz?region=eu-west-1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302 for known peer region, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
}

func TestHealthHandler_UnknownRegion_Returns400(t *testing.T) {
	h := regionrouter.HealthHandler("us-east-2")
	req := httptest.NewRequest(http.MethodGet, "/healthz?region=ap-southeast-1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
