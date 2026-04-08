package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/regionrouter"
	"github.com/bluenviron/mediamtx/internal/cloud/regionrouter/middleware"
)

func newTestMiddleware(local string, allowed []string) middleware.RegionMiddleware {
	return middleware.New(middleware.Config{
		LocalRegion: local,
		Resolver: &regionrouter.Resolver{
			LocalRegion:    local,
			AllowedRegions: allowed,
		},
	})
}

func TestMiddleware_LocalRegion_PassesThrough(t *testing.T) {
	mw := newTestMiddleware("us-east-2", []string{"us-east-2"})
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Region should be injected in context.
		region, ok := regionrouter.RegionFromContext(r.Context())
		if !ok || region != "us-east-2" {
			t.Errorf("expected us-east-2 in context, got (%q, %v)", region, ok)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Host = "us-east-2.api.yourbrand.com"
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)

	if !called {
		t.Fatal("inner handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestMiddleware_CrossRegion_307(t *testing.T) {
	// Allow two regions so the middleware does not 421.
	regionrouter.BaseURLForRegion["eu-west-1"] = "https://eu-west-1.api.yourbrand.com"
	defer delete(regionrouter.BaseURLForRegion, "eu-west-1")

	mw := newTestMiddleware("us-east-2", []string{"us-east-2", "eu-west-1"})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be reached on cross-region 307")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/recorders", nil)
	req.Host = "eu-west-1.api.yourbrand.com"
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	want := "https://eu-west-1.api.yourbrand.com/v1/recorders"
	if loc != want {
		t.Fatalf("Location: got %q, want %q", loc, want)
	}
}

func TestMiddleware_UnknownRegion_421(t *testing.T) {
	mw := newTestMiddleware("us-east-2", []string{"us-east-2"})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be reached for unknown region")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cameras", nil)
	req.Host = "ap-northeast-1.api.yourbrand.com"
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)

	if rr.Code != http.StatusMisdirectedRequest {
		t.Fatalf("expected 421, got %d", rr.Code)
	}
}

func TestMiddleware_NoRegionHost_PassesThrough(t *testing.T) {
	mw := newTestMiddleware("us-east-2", []string{"us-east-2"})
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Local region should be injected as default.
		region, ok := regionrouter.RegionFromContext(r.Context())
		if !ok || region != "us-east-2" {
			t.Errorf("expected local region in context, got (%q, %v)", region, ok)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Host = "localhost:8080"
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)

	if !called {
		t.Fatal("inner handler was not called for localhost host")
	}
}

// TestMiddleware_MismatchTenantHomeRegion verifies that a request to region X
// for a tenant homed in region Y results in a 302 redirect to Y when the
// cross-region redirect handler is chained downstream.
func TestMiddleware_MismatchTenantHomeRegion_Redirects(t *testing.T) {
	regionrouter.BaseURLForRegion["eu-west-1"] = "https://eu-west-1.api.yourbrand.com"
	defer delete(regionrouter.BaseURLForRegion, "eu-west-1")

	mw := newTestMiddleware("us-east-2", []string{"us-east-2"})

	// Inner handler simulates what a post-auth handler would do: inject the
	// tenant's home region and then delegate to CrossRegionRedirectHandler.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := regionrouter.WithTenantHomeRegion(r.Context(), "eu-west-1")
		r = r.WithContext(ctx)
		// Simulate the cross-region redirect.
		redirect := regionrouter.CrossRegionRedirectHandler("us-east-2",
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("final handler should not be reached")
			}),
		)
		redirect.ServeHTTP(w, r)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cameras", nil)
	req.Host = "localhost" // no region in host — hits us-east-2
	rr := httptest.NewRecorder()
	mw(inner).ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	want := "https://eu-west-1.api.yourbrand.com/v1/cameras"
	if loc != want {
		t.Fatalf("Location: got %q, want %q", loc, want)
	}
}
