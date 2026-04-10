package metrics

import (
	"runtime"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// BuildInfo carries the values stamped into kaivue_build_info at startup.
type BuildInfo struct {
	Version   string
	Commit    string
	GoVersion string
}

// Standard holds the core metrics that every component wires into its
// middleware. Obtain one via Init.
type Standard struct {
	// RequestsTotal counts HTTP/Connect requests by component, method, route,
	// and response code. Label names: component, method, route, code.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration records request latency by component and route.
	// Buckets: 5ms to 10s.
	RequestDuration *prometheus.HistogramVec

	// ErrorsTotal counts errors by component and stable error code (KAI-424).
	ErrorsTotal *prometheus.CounterVec

	// BuildInfo is a gauge fixed at 1.0 carrying version/commit/goversion.
	BuildInfo *prometheus.GaugeVec

	// Goroutines tracks the live goroutine count. Updated by a background
	// goroutine started by Init.
	Goroutines prometheus.Gauge

	// MemoryBytes tracks runtime heap stats by type (heap_alloc,
	// heap_sys, heap_inuse).
	MemoryBytes *prometheus.GaugeVec
}

// requestDurationBuckets spans 5ms to 10s, capturing the full p99 range for
// an on-prem NVR API (LAN latency) and for the cloud apiserver.
var requestDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
}

// Init registers the standard metric set into reg and stamps build_info.
// Calling Init more than once on the same Registry is safe — subsequent
// registrations are silently ignored (fail-open on collision).
func Init(reg *Registry, info BuildInfo) *Standard {
	s := &Standard{}

	s.RequestsTotal = reg.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kaivue_requests_total",
			Help: "Total HTTP/Connect requests handled, by component, method, route and response code.",
		},
		[]string{"component", "method", "route", "code"},
	)

	s.RequestDuration = reg.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kaivue_request_duration_seconds",
			Help:    "HTTP/Connect request latency in seconds, by component and route.",
			Buckets: requestDurationBuckets,
		},
		[]string{"component", "route"},
	)

	s.ErrorsTotal = reg.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kaivue_errors_total",
			Help: "Total errors by component and stable error code (KAI-424).",
		},
		[]string{"component", "code"},
	)

	s.BuildInfo = reg.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kaivue_build_info",
			Help: "Build metadata. Value is always 1; use labels to identify the build.",
		},
		[]string{"version", "commit", "goversion"},
	)

	s.Goroutines = reg.NewGauge(prometheus.GaugeOpts{
		Name: "kaivue_goroutines",
		Help: "Current number of goroutines in the process.",
	})

	s.MemoryBytes = reg.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kaivue_memory_bytes",
			Help: "Go runtime memory statistics in bytes, by type.",
		},
		[]string{"type"},
	)

	// Stamp build info once. Use the process goversion if caller left it blank.
	goVer := info.GoVersion
	if goVer == "" {
		goVer = runtime.Version()
	}
	// Attempt to pull commit from build info if caller passed an empty string.
	commit := info.Commit
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
					break
				}
			}
		}
	}
	s.BuildInfo.WithLabelValues(info.Version, commit, goVer).Set(1)

	// Background goroutine: refresh goroutine + memory gauges every 15s.
	// The goroutine is leaked deliberately — it lives for the process lifetime.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.Goroutines.Set(float64(runtime.NumGoroutine()))

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			s.MemoryBytes.WithLabelValues("heap_alloc").Set(float64(ms.HeapAlloc))
			s.MemoryBytes.WithLabelValues("heap_sys").Set(float64(ms.HeapSys))
			s.MemoryBytes.WithLabelValues("heap_inuse").Set(float64(ms.HeapInuse))
		}
	}()

	return s
}
