package automation

import (
	"github.com/prometheus/client_golang/prometheus"

	sharedmetrics "github.com/bluenviron/mediamtx/internal/shared/metrics"
)

// Metrics holds Prometheus counters for the status automation loop.
type Metrics struct {
	// EvaluationsTotal counts automation evaluation cycles by result.
	// Label: result (ok, error).
	EvaluationsTotal *prometheus.CounterVec

	// TransitionsTotal counts component status transitions by component
	// and new status.
	// Labels: component, status.
	TransitionsTotal *prometheus.CounterVec

	// QueryErrorsTotal counts Prometheus query failures by component.
	// Label: component.
	QueryErrorsTotal *prometheus.CounterVec

	// PushErrorsTotal counts Statuspage.io API push failures by component.
	// Label: component.
	PushErrorsTotal *prometheus.CounterVec
}

// NewMetrics registers and returns automation metrics.
func NewMetrics(reg *sharedmetrics.Registry) *Metrics {
	return &Metrics{
		EvaluationsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_statuspage_evaluations_total",
			Help: "Total status automation evaluation cycles, by result.",
		}, []string{"result"}),

		TransitionsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_statuspage_transitions_total",
			Help: "Total component status transitions, by component and new status.",
		}, []string{"component", "status"}),

		QueryErrorsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_statuspage_query_errors_total",
			Help: "Total Prometheus query failures, by component.",
		}, []string{"component"}),

		PushErrorsTotal: reg.NewCounterVec(prometheus.CounterOpts{
			Name: "kaivue_statuspage_push_errors_total",
			Help: "Total Statuspage.io API push failures, by component.",
		}, []string{"component"}),
	}
}
