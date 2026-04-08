// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"errors"
	"net/http"
	"strings"
)

// RegionHeader is the canonical header clients use to pin a request to a
// specific region. Matches KAI-218 DB default region column naming.
const RegionHeader = "X-Kaivue-Region"

// regionMiddleware rejects cross-region requests with a 307 Temporary
// Redirect to the canonical URL for the requested region (seam #9).
//
// Rules:
//   - No header → request proceeds on the local region (implicit opt-in).
//   - Header matches local Region → request proceeds.
//   - Header matches a known RegionRoute → 307 to RegionRoute.BaseURL + path.
//   - Header is unknown → 421 Misdirected Request JSON envelope.
func regionMiddleware(localRegion string, routes []RegionRoute) Middleware {
	table := make(map[string]string, len(routes))
	for _, r := range routes {
		table[r.Region] = strings.TrimRight(r.BaseURL, "/")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			want := r.Header.Get(RegionHeader)
			if want == "" || want == localRegion {
				next.ServeHTTP(w, r)
				return
			}
			baseURL, ok := table[want]
			if !ok {
				// Unknown region: 421 per RFC 7540 semantics, body in
				// our standard JSON envelope.
				writeError(w, &ConnectError{
					code: CodeInvalidArgument,
					err:  errors.New("unknown region: " + want),
				})
				return
			}
			// Preserve the path + query verbatim; the peer region's
			// apiserver will re-run the middleware chain.
			target := baseURL + r.URL.RequestURI()
			w.Header().Set("Location", target)
			w.WriteHeader(http.StatusTemporaryRedirect)
		})
	}
}
