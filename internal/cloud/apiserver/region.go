// Package apiserver — region routing glue (KAI-230).
//
// This file replaces the KAI-226 header-only stub with the full region
// routing infrastructure from internal/cloud/regionrouter. The middleware
// now:
//
//  1. Parses the Host header to extract the region (e.g. "us-east-2" from
//     "us-east-2.api.yourbrand.com").
//  2. Falls back to the X-Kaivue-Region header for clients behind a proxy
//     that rewrites the Host.
//  3. Validates the resolved region against the platform allowlist.
//  4. Injects the region into the request context for downstream handlers.
//  5. 307-redirects cross-region requests to the canonical peer URL.
//
// The RegionRoute table from Config is merged into regionrouter.BaseURLForRegion
// so operator-supplied overrides take precedence over the package defaults.
package apiserver

import (
	"errors"
	"net/http"
	"strings"

	regionmw "github.com/bluenviron/mediamtx/internal/cloud/regionrouter/middleware"

	"github.com/bluenviron/mediamtx/internal/cloud/regionrouter"
)

// RegionHeader is the canonical header clients use to pin a request to a
// specific region. Matches KAI-218 DB default region column naming. Kept for
// backward compatibility with load-balancer / proxy configs set up before
// KAI-230 landed.
const RegionHeader = "X-Kaivue-Region"

// buildRegionMiddleware constructs the region routing middleware for the
// apiserver middleware chain. It merges the operator-supplied RegionRoutes
// into the global base-URL table (so integration tests can inject custom
// routes without modifying package-level vars).
func buildRegionMiddleware(localRegion string, routes []RegionRoute) Middleware {
	// Merge operator-supplied routes into the global table. We do this here
	// rather than in the regionrouter package to keep that package free of
	// apiserver types. The table is only ever written at startup so there is no
	// race condition in production.
	for _, r := range routes {
		if r.BaseURL != "" {
			regionrouter.BaseURLForRegion[r.Region] = strings.TrimRight(r.BaseURL, "/")
		}
	}

	// Build the allowlist from both the package-level KnownRegions and the
	// operator-supplied table (a route entry for an unknown region adds it).
	allowed := make([]string, len(regionrouter.KnownRegions))
	copy(allowed, regionrouter.KnownRegions)
	for _, r := range routes {
		found := false
		for _, a := range allowed {
			if a == r.Region {
				found = true
				break
			}
		}
		if !found {
			allowed = append(allowed, r.Region)
		}
	}

	mw := regionmw.New(regionmw.Config{
		LocalRegion: localRegion,
		Resolver: &regionrouter.Resolver{
			LocalRegion:    localRegion,
			AllowedRegions: allowed,
		},
	})

	// Wrap the regionmw.RegionMiddleware (func(http.Handler) http.Handler)
	// into our local Middleware type alias — they are the same underlying type.
	return Middleware(mw)
}

// regionMiddleware is the legacy entry-point called from server.go. It
// delegates to buildRegionMiddleware. The header-fallback logic (X-Kaivue-Region)
// is handled inside the regionrouter middleware when the host carries no region
// signal; for complete header-based override we wrap the outer middleware with
// a thin shim that rewrites the host before the resolver runs.
func regionMiddleware(localRegion string, routes []RegionRoute) Middleware {
	base := buildRegionMiddleware(localRegion, routes)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If the client provided the legacy X-Kaivue-Region header AND
			// the host does not already carry a region signal, synthesise
			// a canonical host so the resolver can do a single code path.
			if want := r.Header.Get(RegionHeader); want != "" {
				if _, ok := regionrouter.ParseRegionFromHost(r.Host); !ok {
					// No region in host — rewrite host so the resolver sees it.
					r = r.Clone(r.Context())
					r.Host = want + ".api.yourbrand.com"
				}
			}
			base(next).ServeHTTP(w, r)
		})
	}
}

// unknownRegionError constructs the 421 error used by legacy callers. Kept for
// any code path that still constructs a ConnectError directly (e.g. tests that
// import apiserver and call writeError).
func unknownRegionError(region string) *ConnectError {
	return NewError(CodeInvalidArgument, errors.New("unknown region: "+region))
}
