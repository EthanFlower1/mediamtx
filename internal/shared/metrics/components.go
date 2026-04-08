package metrics

import "github.com/prometheus/client_golang/prometheus"

// RecorderControlMetrics holds per-component counters for the
// recordercontrol package (KAI-252/253). Obtain via NewRecorderControlMetrics.
type RecorderControlMetrics struct {
	// Connected is 1 when the control stream to a recorder is up, 0 otherwise.
	// Label: recorder_id.
	Connected *prometheus.GaugeVec

	// EventsAppliedTotal counts directory events applied per type.
	// Label: event_type.
	EventsAppliedTotal *prometheus.CounterVec

	// ReconnectsTotal counts reconnects to a recorder per reason.
	// Label: reason.
	ReconnectsTotal *prometheus.CounterVec
}

// NewRecorderControlMetrics registers and returns RecorderControlMetrics.
func NewRecorderControlMetrics(reg *Registry) *RecorderControlMetrics {
	return &RecorderControlMetrics{
		Connected: reg.NewGaugeVec(prometheus.GaugeOpts{
			Name: "kaivue_recordercontrol_connected",
			Help: "1 when the Directory→Recorder control stream is active, 0 when disconnected.",
		}, []string{"recorder_id"}),

		EventsAppliedTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_recordercontrol_events_applied_total",
			Help: "Total directory events applied to a recorder, by event_type.",
		}, []string{"event_type"}),

		ReconnectsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_recordercontrol_reconnects_total",
			Help: "Total reconnection attempts to a recorder, by reason.",
		}, []string{"reason"}),
	}
}

// DirectoryIngestMetrics holds per-component counters for the
// directoryingest package (KAI-254). Obtain via NewDirectoryIngestMetrics.
type DirectoryIngestMetrics struct {
	// MessagesTotal counts ingest messages per stream and result.
	// Labels: stream, result.
	MessagesTotal *prometheus.CounterVec

	// BackpressureDropsTotal counts messages dropped due to backpressure.
	BackpressureDropsTotal prometheus.Counter
}

// NewDirectoryIngestMetrics registers and returns DirectoryIngestMetrics.
func NewDirectoryIngestMetrics(reg *Registry) *DirectoryIngestMetrics {
	return &DirectoryIngestMetrics{
		MessagesTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_directoryingest_messages_total",
			Help: "Total ingest messages received by the Directory, by stream and result.",
		}, []string{"stream", "result"}),

		BackpressureDropsTotal: reg.NewCounter(prometheus.CounterOpts{
			Name: "kaivue_directoryingest_backpressure_drops_total",
			Help: "Total messages dropped because the ingest buffer was full.",
		}),
	}
}

// StreamsMetrics holds per-component counters for the streams minting service
// (KAI-255). Obtain via NewStreamsMetrics.
type StreamsMetrics struct {
	// MintedTotal counts minted stream URLs by kind, protocol, and result.
	// Labels: kind, protocol, result.
	MintedTotal *prometheus.CounterVec

	// TTLSeconds is a histogram of stream token TTL values at mint time.
	TTLSeconds prometheus.Histogram
}

// NewStreamsMetrics registers and returns StreamsMetrics.
func NewStreamsMetrics(reg *Registry) *StreamsMetrics {
	return &StreamsMetrics{
		MintedTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_streams_minted_total",
			Help: "Total stream URLs minted, by kind, protocol, and result.",
		}, []string{"kind", "protocol", "result"}),

		TTLSeconds: reg.NewHistogram(prometheus.HistogramOpts{
			Name:    "kaivue_streams_ttl_seconds",
			Help:    "Distribution of stream token TTL values (seconds) at mint time.",
			Buckets: []float64{30, 60, 120, 300, 600, 1800, 3600},
		}),
	}
}

// CertMgrMetrics holds per-component counters for the certmgr package
// (KAI-242). These replace the atomic-based certmgr.Metrics.WritePrometheus
// approach with proper Prometheus types.
//
// Obtain via NewCertMgrMetrics. After migration, certmgr.Metrics should
// delegate to this struct rather than its own atomics.
type CertMgrMetrics struct {
	// CertExpiresAt is a gauge holding the Unix timestamp when the active
	// mTLS leaf cert expires.
	CertExpiresAt prometheus.Gauge

	// RenewalsTotal counts cert renewal attempts by result.
	// Label: result (ok, error, skipped).
	RenewalsTotal *prometheus.CounterVec

	// ReEnrollmentsTotal counts re-enrollment attempts by result.
	// Label: result (ok, error).
	ReEnrollmentsTotal *prometheus.CounterVec
}

// NewCertMgrMetrics registers and returns CertMgrMetrics.
func NewCertMgrMetrics(reg *Registry) *CertMgrMetrics {
	return &CertMgrMetrics{
		CertExpiresAt: reg.NewGauge(prometheus.GaugeOpts{
			Name: "kaivue_certmgr_cert_expires_at",
			Help: "Unix timestamp (seconds) when the active mTLS leaf cert expires.",
		}),

		RenewalsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_certmgr_renewals_total",
			Help: "Total cert renewal attempts, by result.",
		}, []string{"result"}),

		ReEnrollmentsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_certmgr_reenrollments_total",
			Help: "Total re-enrollment attempts (fallback path), by result.",
		}, []string{"result"}),
	}
}

// RelationshipsMetrics holds per-component counters for the relationships
// service (KAI-228). Obtain via NewRelationshipsMetrics.
type RelationshipsMetrics struct {
	// GrantedTotal counts integrator–customer relationship grants.
	GrantedTotal prometheus.Counter

	// RevokedTotal counts integrator–customer relationship revocations.
	RevokedTotal prometheus.Counter
}

// NewRelationshipsMetrics registers and returns RelationshipsMetrics.
func NewRelationshipsMetrics(reg *Registry) *RelationshipsMetrics {
	return &RelationshipsMetrics{
		GrantedTotal: reg.NewCounter(prometheus.CounterOpts{
			Name: "kaivue_relationships_granted_total",
			Help: "Total integrator–customer relationships granted.",
		}),

		RevokedTotal: reg.NewCounter(prometheus.CounterOpts{
			Name: "kaivue_relationships_revoked_total",
			Help: "Total integrator–customer relationships revoked.",
		}),
	}
}
