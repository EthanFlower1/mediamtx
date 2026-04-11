package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Collector registers and records Prometheus metrics for model inference
// performance. All metrics are labelled by tenant_id, model_id, and version.
type Collector struct {
	inferenceTotal   *prometheus.CounterVec
	inferenceErrors  *prometheus.CounterVec
	truePositives    *prometheus.CounterVec
	falsePositives   *prometheus.CounterVec
	trueNegatives    *prometheus.CounterVec
	falseNegatives   *prometheus.CounterVec
	latencySeconds   *prometheus.HistogramVec
	driftKL          *prometheus.GaugeVec
	driftPSI         *prometheus.GaugeVec
	driftAlerts      *prometheus.CounterVec
	accuracyGauge    *prometheus.GaugeVec
	fpRateGauge      *prometheus.GaugeVec

	registry prometheus.Registerer
}

var modelLabels = []string{"tenant_id", "model_id", "version"}

// NewCollector creates a Collector and registers all metrics with the given
// Prometheus registerer. Pass prometheus.DefaultRegisterer for production use,
// or prometheus.NewRegistry() for testing.
func NewCollector(reg prometheus.Registerer) *Collector {
	c := &Collector{registry: reg}

	c.inferenceTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "inference_total",
		Help:      "Total inference requests by model.",
	}, modelLabels)

	c.inferenceErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "inference_errors_total",
		Help:      "Total inference errors by model.",
	}, modelLabels)

	c.truePositives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "true_positives_total",
		Help:      "True positive count for accuracy tracking.",
	}, modelLabels)

	c.falsePositives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "false_positives_total",
		Help:      "False positive count for FP rate tracking.",
	}, modelLabels)

	c.trueNegatives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "true_negatives_total",
		Help:      "True negative count for accuracy tracking.",
	}, modelLabels)

	c.falseNegatives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "false_negatives_total",
		Help:      "False negative count for accuracy tracking.",
	}, modelLabels)

	c.latencySeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "inference_latency_seconds",
		Help:      "Inference latency distribution in seconds.",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
	}, modelLabels)

	c.driftKL = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "drift_kl_divergence",
		Help:      "Current KL divergence from reference distribution.",
	}, modelLabels)

	c.driftPSI = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "drift_psi",
		Help:      "Current Population Stability Index.",
	}, modelLabels)

	c.driftAlerts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "drift_alerts_total",
		Help:      "Total drift alerts fired.",
	}, modelLabels)

	c.accuracyGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "accuracy_pct",
		Help:      "Current model accuracy percentage.",
	}, modelLabels)

	c.fpRateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kaivue",
		Subsystem: "model",
		Name:      "false_positive_rate_pct",
		Help:      "Current false positive rate percentage.",
	}, modelLabels)

	// Register all collectors.
	for _, col := range []prometheus.Collector{
		c.inferenceTotal, c.inferenceErrors,
		c.truePositives, c.falsePositives, c.trueNegatives, c.falseNegatives,
		c.latencySeconds,
		c.driftKL, c.driftPSI, c.driftAlerts,
		c.accuracyGauge, c.fpRateGauge,
	} {
		reg.MustRegister(col)
	}

	return c
}

// RecordInference records a single inference call's latency.
func (c *Collector) RecordInference(key ModelKey, latencySeconds float64) {
	labels := prometheus.Labels{
		"tenant_id": key.TenantID,
		"model_id":  key.ModelID,
		"version":   key.Version,
	}
	c.inferenceTotal.With(labels).Inc()
	c.latencySeconds.With(labels).Observe(latencySeconds)
}

// RecordError records an inference error.
func (c *Collector) RecordError(key ModelKey) {
	labels := prometheus.Labels{
		"tenant_id": key.TenantID,
		"model_id":  key.ModelID,
		"version":   key.Version,
	}
	c.inferenceErrors.With(labels).Inc()
}

// RecordClassification records a classification result for accuracy tracking.
func (c *Collector) RecordClassification(key ModelKey, tp, fp, tn, fn int64) {
	labels := prometheus.Labels{
		"tenant_id": key.TenantID,
		"model_id":  key.ModelID,
		"version":   key.Version,
	}
	c.truePositives.With(labels).Add(float64(tp))
	c.falsePositives.With(labels).Add(float64(fp))
	c.trueNegatives.With(labels).Add(float64(tn))
	c.falseNegatives.With(labels).Add(float64(fn))

	// Update computed gauges.
	total := float64(tp + fp + tn + fn)
	if total > 0 {
		accuracy := float64(tp+tn) / total * 100.0
		c.accuracyGauge.With(labels).Set(accuracy)

		fpTotal := float64(fp + tn)
		if fpTotal > 0 {
			fpRate := float64(fp) / fpTotal * 100.0
			c.fpRateGauge.With(labels).Set(fpRate)
		}
	}
}

// RecordDrift updates the drift gauges after a drift check.
func (c *Collector) RecordDrift(result DriftResult) {
	labels := prometheus.Labels{
		"tenant_id": result.Key.TenantID,
		"model_id":  result.Key.ModelID,
		"version":   result.Key.Version,
	}
	c.driftKL.With(labels).Set(result.KLDivergence)
	c.driftPSI.With(labels).Set(result.PSI)
	if result.Drifted {
		c.driftAlerts.With(labels).Inc()
	}
}
