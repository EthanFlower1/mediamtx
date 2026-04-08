// Package metrics provides the shared Prometheus metrics registry for all
// Kaivue Recording Server components.
//
// Design rules:
//   - Never use promauto or the default prometheus registry. Every metric
//     must be registered through this package so it is isolated and testable.
//   - Every tenant-scoped metric must carry a tenant_id label.
//   - Every component metric must carry a component label.
//   - Metric writes that fail must never block the request path (fail-open).
//
// Usage:
//
//	reg := metrics.New()
//	metrics.Init(reg, metrics.BuildInfo{Version: "v1.0.0", Commit: "abc123", GoVersion: "go1.22"})
//	http.Handle("/metrics", reg.HTTPHandler())
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps a prometheus.Registry with helper constructors that enforce
// label conventions and register into the explicit (non-default) registry.
type Registry struct {
	reg *prometheus.Registry
}

// New returns a new Registry backed by a fresh prometheus.Registry. The
// Go process collector and Go build info collector are always included so
// callers get goroutine and memory gauges for free.
func New() *Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(prometheus.NewGoCollector())
	r.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	return &Registry{reg: r}
}

// Unwrap returns the underlying prometheus.Registry for callers that need
// direct access (e.g. promhttp.HandlerFor).
func (r *Registry) Unwrap() *prometheus.Registry { return r.reg }

// HTTPHandler returns an http.Handler that serves the Prometheus text
// exposition format for this registry only — never the default registry.
func (r *Registry) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{
		EnableOpenMetrics: false,
	})
}

// MustRegister registers collectors; panics on error (same contract as
// prometheus.MustRegister). Use in package init or TestMain — never in a
// hot path.
func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	r.reg.MustRegister(cs...)
}

// register registers a collector, returning an error on conflict.
func (r *Registry) register(c prometheus.Collector) error {
	return r.reg.Register(c)
}

// -----------------------------------------------------------------------
// Helper constructors
// -----------------------------------------------------------------------

// NewCounter registers and returns a new CounterVec with the given opts.
// The metric is registered into this Registry's explicit registry.
func (r *Registry) NewCounter(opts prometheus.CounterOpts) prometheus.Counter {
	c := prometheus.NewCounter(opts)
	_ = r.register(c) // fail-open: if already registered, discard error
	return c
}

// NewCounterVec registers and returns a new CounterVec.
func (r *Registry) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(opts, labelNames)
	_ = r.register(c)
	return c
}

// NewHistogram registers and returns a new Histogram.
func (r *Registry) NewHistogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	h := prometheus.NewHistogram(opts)
	_ = r.register(h)
	return h
}

// NewHistogramVec registers and returns a new HistogramVec.
func (r *Registry) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(opts, labelNames)
	_ = r.register(h)
	return h
}

// NewGauge registers and returns a new Gauge.
func (r *Registry) NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	g := prometheus.NewGauge(opts)
	_ = r.register(g)
	return g
}

// NewGaugeVec registers and returns a new GaugeVec.
func (r *Registry) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(opts, labelNames)
	_ = r.register(g)
	return g
}
