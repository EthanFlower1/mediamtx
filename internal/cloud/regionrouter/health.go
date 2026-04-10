package regionrouter

import (
	"encoding/json"
	"net/http"
)

// HealthHandler returns an http.Handler for GET /healthz that validates an
// optional ?region=<r> query parameter. It is designed to be mounted by the
// apiserver at /healthz in place of (or wrapping) the liveness probe.
//
// Behaviour:
//   - ?region omitted: responds 200 {"status":"ok","region":"<local>"}.
//   - ?region == localRegion: responds 200 {"status":"ok","region":"<local>"}.
//   - ?region is a valid region != localRegion: responds 302 to the peer
//     region's /healthz so monitoring automation is directed to the right
//     endpoint without a 4xx alarm.
//   - ?region is not in the allowlist: responds 400 {"error":"unknown region"}.
//
// The handler never reaches the DB or any external service — liveness probes
// MUST NOT take external dependencies (k8s will kill the pod on transient
// outages otherwise).
func HealthHandler(localRegion string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := r.URL.Query().Get("region")
		if want == "" || want == localRegion {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "ok",
				"region": localRegion,
			})
			return
		}
		if !IsAllowedRegion(want) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":          "unknown region",
				"requested":      want,
				"local_region":   localRegion,
			})
			return
		}
		// Redirect to the canonical /healthz for the requested region.
		baseURL, ok := BaseURLForRegion[want]
		if !ok {
			// Allowlist and BaseURLForRegion are out of sync — programmer
			// error. Return 500 so the discrepancy is surfaced immediately.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "region base URL missing for allowed region " + want,
			})
			return
		}
		target := baseURL + "/healthz?region=" + want
		http.Redirect(w, r, target, http.StatusFound)
	})
}
