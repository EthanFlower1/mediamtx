// Package recordermetrics owns all Prometheus metrics for the recorder
// subsystem. It uses an isolated *prometheus.Registry so the recorder's
// metrics do not collide with any upstream mediamtx metrics.
//
// Usage:
//
//	m := recordermetrics.New()
//	router.GET("/metrics", gin.WrapH(m.Handler()))
//
// Gauge metrics are updated by calling UpdateGauges with a Snapshot.
// Counter metrics are incremented directly by their owning subsystems via
// the exported Counter fields.
package recordermetrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns all recorder Prometheus metrics and their registry.
type Metrics struct {
	// Bedrock recording gauges.
	CamerasExpected   prometheus.Gauge
	CamerasPublishing prometheus.Gauge

	// Reconcile error counter — incremented by the watchdog or supervisor
	// when drift is detected or a push fails.
	ReconcileErrors prometheus.Counter

	// Recovery scan counters.
	RecoveryScanScanned     prometheus.Counter
	RecoveryScanRepaired    prometheus.Counter
	RecoveryScanUnrecoverable prometheus.Counter

	// Integrity scanner counters.
	IntegrityVerifications prometheus.Counter
	IntegrityQuarantines   prometheus.Counter

	// Fragment-backfill counter.
	FragmentBackfillIndexed prometheus.Counter

	// Disk usage gauges.
	DiskUsedBytes     prometheus.Gauge
	DiskCapacityBytes prometheus.Gauge
	DiskUsedPercent   prometheus.Gauge

	registry *prometheus.Registry
}

// New creates and registers all metrics against an isolated registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		CamerasExpected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cameras_expected_total",
			Help: "Number of cameras assigned to this recorder (from state.Store).",
		}),
		CamerasPublishing: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cameras_publishing_total",
			Help: "Number of cameras currently publishing (ready=true in mediamtx runtime).",
		}),
		ReconcileErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconcile_errors_total",
			Help: "Total supervisor or watchdog errors during reconciliation.",
		}),
		RecoveryScanScanned: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "recovery_scan_scanned_total",
			Help: "Total recordings scanned by startup recovery.",
		}),
		RecoveryScanRepaired: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "recovery_scan_repaired_total",
			Help: "Total recordings repaired during startup recovery.",
		}),
		RecoveryScanUnrecoverable: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "recovery_scan_unrecoverable_total",
			Help: "Total recordings flagged as unrecoverable during startup recovery.",
		}),
		IntegrityVerifications: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "integrity_verifications_total",
			Help: "Total integrity check runs across all recordings.",
		}),
		IntegrityQuarantines: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "integrity_quarantines_total",
			Help: "Total recordings quarantined due to detected corruption.",
		}),
		FragmentBackfillIndexed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fragmentbackfill_indexed_total",
			Help: "Total recordings backfilled with fragment metadata. Always 0 until Phase 2 instrumentation.",
		}),
		DiskUsedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "disk_used_bytes",
			Help: "Current bytes used on the recordings partition.",
		}),
		DiskCapacityBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "disk_capacity_bytes",
			Help: "Total bytes available on the recordings partition.",
		}),
		DiskUsedPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "disk_used_percent",
			Help: "Percentage of disk used on the recordings partition (100 * used / capacity).",
		}),
		registry: reg,
	}

	// Build info — labeled constant gauge.
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "recorder_build_info",
		Help: "Static build metadata. Value is always 1.",
	}, []string{"version"})
	buildInfo.WithLabelValues("dev").Set(1)

	reg.MustRegister(
		m.CamerasExpected,
		m.CamerasPublishing,
		m.ReconcileErrors,
		m.RecoveryScanScanned,
		m.RecoveryScanRepaired,
		m.RecoveryScanUnrecoverable,
		m.IntegrityVerifications,
		m.IntegrityQuarantines,
		m.FragmentBackfillIndexed,
		m.DiskUsedBytes,
		m.DiskCapacityBytes,
		m.DiskUsedPercent,
		buildInfo,
	)

	return m
}

// Registry returns the underlying isolated registry. Use this with
// promhttp.HandlerFor when you need fine-grained control over the handler.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// Handler returns an HTTP handler that serves the metrics text format.
// Mount it on "/metrics" in the recorder's gin router:
//
//	router.GET("/metrics", gin.WrapH(m.Handler()))
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry: m.registry,
	})
}

// Snapshot holds the current point-in-time values for gauge metrics.
type Snapshot struct {
	// CamerasExpected is the count of cameras in state.Store (ListAssigned).
	CamerasExpected int
	// CamerasPublishing is the count of cameras mediamtx reports ready=true.
	CamerasPublishing int
	// DiskUsedBytes is the current byte count used on the recordings partition.
	DiskUsedBytes int64
	// DiskCapacityBytes is the total byte capacity of the recordings partition.
	DiskCapacityBytes int64
}

// UpdateGauges writes a Snapshot into the gauge metrics.
// It is safe to call concurrently from multiple goroutines (prometheus.Gauge
// is internally atomic). DiskUsedPercent is derived from the snapshot values.
func (m *Metrics) UpdateGauges(snap Snapshot) {
	m.CamerasExpected.Set(float64(snap.CamerasExpected))
	m.CamerasPublishing.Set(float64(snap.CamerasPublishing))
	m.DiskUsedBytes.Set(float64(snap.DiskUsedBytes))
	m.DiskCapacityBytes.Set(float64(snap.DiskCapacityBytes))

	var pct float64
	if snap.DiskCapacityBytes > 0 {
		pct = float64(snap.DiskUsedBytes) / float64(snap.DiskCapacityBytes) * 100.0
	}
	m.DiskUsedPercent.Set(pct)
}
