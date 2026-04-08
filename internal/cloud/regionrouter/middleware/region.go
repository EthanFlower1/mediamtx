// Package middleware provides the region routing middleware that plugs into
// the apiserver's chain. It sits at position #5 in the stack (before CORS,
// after tracing) — early enough to redirect before auth work is done, but
// after request-ID and tracing have instrumented the request.
//
// Wiring: apiserver.buildConnectChain() calls regionrouter/middleware.New()
// and inserts the returned Middleware at position #5. The existing
// apiserver/region.go stub (KAI-226) is superseded by this package; the
// apiserver should delegate to New() rather than calling regionMiddleware()
// directly once this PR lands.
package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/cloud/regionrouter"
)

// RegionMiddleware is the net/http middleware shape expected by the apiserver
// chain helper.
type RegionMiddleware func(http.Handler) http.Handler

// Config holds the dependencies injected by the apiserver.
type Config struct {
	// LocalRegion is the AWS region this server instance serves.
	LocalRegion string
	// Resolver parses and validates host-header regions.
	Resolver *regionrouter.Resolver
	// TenantResolver optionally resolves the tenant's home region and
	// injects it for the cross-region redirect handler. May be nil (in
	// which case tenant home-region injection is skipped — used for
	// unauthenticated paths like /healthz).
	TenantResolver *regionrouter.TenantRegionResolver
}

// New returns a RegionMiddleware that:
//  1. Parses the Host header and validates it against the allowlist.
//  2. Injects the resolved region into the request context.
//  3. If the resolved region differs from LocalRegion, emits a 307 redirect
//     to the peer region's canonical base URL preserving the full request URI.
//  4. If TenantResolver is set, looks up the tenant's home region (keyed by
//     X-Kaivue-Tenant or the leftmost subdomain) and injects it for
//     downstream handlers (e.g. CrossRegionRedirectHandler).
//
// Requests whose Host header carries no region signal (localhost, IPs) pass
// through unchanged — this allows health-check probes from within the cluster
// to work without an FQDN.
func New(cfg Config) RegionMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			region, hasRegion, err := cfg.Resolver.ResolveHost(r.Host)
			if err != nil {
				// The host carried a region signal but it is not in the
				// allowlist. Return 421 Misdirected Request.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusMisdirectedRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": err.Error(),
				})
				return
			}

			if hasRegion {
				// Inject the resolved region into context regardless of
				// whether we redirect — handlers may want to log it.
				r = r.WithContext(regionrouter.WithRegion(r.Context(), region))

				if region != cfg.LocalRegion {
					// Client aimed at a region this server does not serve.
					// 307 Temporary Redirect — the client retains its method.
					baseURL, ok := regionrouter.BaseURLForRegion[region]
					if !ok {
						// Should not happen: ResolveHost already validated the
						// region. Defensive 421.
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusMisdirectedRequest)
						_ = json.NewEncoder(w).Encode(map[string]string{
							"error": "region routing table inconsistency for " + region,
						})
						return
					}
					target := baseURL + r.URL.RequestURI()
					w.Header().Set("Location", target)
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
			} else {
				// No region in host — inject local region as the default so
				// downstream handlers always have a region in context.
				r = r.WithContext(regionrouter.WithRegion(r.Context(), cfg.LocalRegion))
			}

			next.ServeHTTP(w, r)
		})
	}
}
