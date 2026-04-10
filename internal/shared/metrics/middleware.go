package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// HTTPMiddleware returns a net/http middleware that records
// kaivue_requests_total and kaivue_request_duration_seconds for every
// request. component is the label value stamped on every observation (e.g.
// "cloud-apiserver", "directory", "recorder").
//
// The middleware is fail-open: a nil Standard or a nil Registry means the
// middleware is a transparent pass-through.
func HTTPMiddleware(std *Standard, component string) func(http.Handler) http.Handler {
	if std == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sr, r)
			elapsed := time.Since(start).Seconds()
			code := strconv.Itoa(sr.status)
			route := r.URL.Path

			// Fail-open: recover from any label cardinality panic.
			safeInc(func() {
				std.RequestsTotal.WithLabelValues(component, r.Method, route, code).Inc()
			})
			safeObserve(func() {
				std.RequestDuration.WithLabelValues(component, route).Observe(elapsed)
			})
		})
	}
}

// safeInc calls f, recovering from any panic so that metric writes never
// block the request path (fail-open policy).
func safeInc(f func()) {
	defer func() { recover() }() //nolint:errcheck
	f()
}

// safeObserve is identical to safeInc; separate name for readability at
// call sites.
func safeObserve(f func()) {
	defer func() { recover() }() //nolint:errcheck
	f()
}

// statusRecorder wraps http.ResponseWriter to capture the first written
// status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}
