package regionrouter

import (
	"encoding/json"
	"net/http"
)

// CrossRegionRedirectHandler returns an http.Handler that, given a tenant's
// home region (looked up by the middleware), redirects the client to the
// canonical URL for that region.
//
// Usage: mount this at a "logged-in marketing-site → tenant home" gateway
// endpoint that does not know which region the user's tenant lives in. The
// caller resolves the home region via TenantRegionResolver and then calls
// this handler.
//
// HTTP semantics:
//   - Home region == local region: pass-through (call next).
//   - Home region is known: 302 Found to canonical base URL + request URI.
//   - Home region is unknown or empty: 421 Misdirected Request.
func CrossRegionRedirectHandler(localRegion string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		homeRegion, hasHome := TenantHomeRegionFromContext(r.Context())
		if !hasHome || homeRegion == "" {
			writeRedirectError(w, http.StatusMisdirectedRequest, "tenant home region not resolved")
			return
		}
		if homeRegion == localRegion {
			// Fast-path: this is already the right region.
			next.ServeHTTP(w, r)
			return
		}
		baseURL, ok := BaseURLForRegion[homeRegion]
		if !ok {
			writeRedirectError(w, http.StatusMisdirectedRequest, "unknown home region: "+homeRegion)
			return
		}
		target := baseURL + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusFound)
	})
}

// writeRedirectError writes a plain JSON error envelope used by redirect
// handlers. We deliberately avoid importing the apiserver's ConnectError type
// to keep this package dependency-light.
func writeRedirectError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
	})
}
