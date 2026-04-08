// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
//
// This file ships a minimal Prometheus-compatible /metrics endpoint. The
// real KAI-421 structured logging / metrics wiring will later swap this for
// github.com/prometheus/client_golang once that dependency is added to
// go.mod. Until then, emitting plain-text Prometheus exposition format from
// the stdlib is perfectly adequate for smoke tests and for the readiness
// dashboards described in the v1 roadmap.
package apiserver

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// metricsRegistry holds the counters the apiserver self-reports. Everything
// is atomic so the /metrics handler can read without holding a lock.
type metricsRegistry struct {
	requestsTotal    atomic.Uint64
	requestsAllowed  atomic.Uint64
	requestsDenied   atomic.Uint64
	requests5xx      atomic.Uint64
	rateLimitHits    atomic.Uint64
	panicsRecovered  atomic.Uint64
}

func newMetricsRegistry() *metricsRegistry { return &metricsRegistry{} }

// metricsMiddleware counts every request by terminal status class. It must
// run OUTSIDE the recovery middleware so panics still increment the 5xx
// counter via the statusRecorder shim.
func metricsMiddleware(reg *metricsRegistry) Middleware {
	if reg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reg.requestsTotal.Add(1)
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sr, r)
			switch {
			case sr.status >= 200 && sr.status < 300:
				reg.requestsAllowed.Add(1)
			case sr.status == http.StatusForbidden:
				reg.requestsDenied.Add(1)
			case sr.status == http.StatusTooManyRequests:
				reg.rateLimitHits.Add(1)
			case sr.status >= 500:
				reg.requests5xx.Add(1)
			}
		})
	}
}

// metricsHandler emits the Prometheus text exposition format.
func metricsHandler(reg *metricsRegistry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		write := func(name, help, typ string, val uint64) {
			fmt.Fprintf(w, "# HELP %s %s\n", name, help)
			fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
			fmt.Fprintf(w, "%s %d\n", name, val)
		}
		write("kaivue_apiserver_requests_total",
			"Total HTTP requests served since process start.",
			"counter", reg.requestsTotal.Load())
		write("kaivue_apiserver_requests_allowed_total",
			"Requests that returned a 2xx status.",
			"counter", reg.requestsAllowed.Load())
		write("kaivue_apiserver_requests_denied_total",
			"Requests that returned 403 (Casbin deny).",
			"counter", reg.requestsDenied.Load())
		write("kaivue_apiserver_requests_5xx_total",
			"Requests that returned 5xx.",
			"counter", reg.requests5xx.Load())
		write("kaivue_apiserver_rate_limit_hits_total",
			"Requests rejected by the per-tenant rate limiter.",
			"counter", reg.rateLimitHits.Load())
		write("kaivue_apiserver_panics_recovered_total",
			"Panics caught by the recovery middleware.",
			"counter", reg.panicsRecovered.Load())
	})
}
