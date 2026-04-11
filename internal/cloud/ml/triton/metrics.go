package triton

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for Triton inference operations.
// All metrics are labelled by model name; latency and throughput also
// carry tenant_id for per-tenant observability.
type Metrics struct {
	// InferenceLatency tracks P50/P95/P99 inference latency per model.
	InferenceLatency *prometheus.HistogramVec

	// InferenceRequestsTotal counts total inference requests per model.
	InferenceRequestsTotal *prometheus.CounterVec

	// InferenceErrorsTotal counts inference errors per model.
	InferenceErrorsTotal *prometheus.CounterVec

	// InferenceThroughput counts successful inferences per model.
	InferenceThroughput *prometheus.CounterVec

	// InferenceQueueDepth tracks current in-flight requests per model.
	InferenceQueueDepth *prometheus.GaugeVec

	// ModelLoadLatency tracks model loading time.
	ModelLoadLatency *prometheus.HistogramVec

	// GPUUtilization tracks GPU utilization percentage per model.
	GPUUtilization *prometheus.GaugeVec
}

// NewMetrics creates a new Metrics instance with all collectors initialized.
// Call Register() to register with a prometheus.Registerer.
func NewMetrics() *Metrics {
	return &Metrics{
		InferenceLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "inference_latency_seconds",
				Help:      "Inference request latency in seconds, per model and tenant.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
			},
			[]string{"model", "tenant_id"},
		),
		InferenceRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "inference_requests_total",
				Help:      "Total number of inference requests, per model and tenant.",
			},
			[]string{"model", "tenant_id"},
		),
		InferenceErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "inference_errors_total",
				Help:      "Total number of inference errors, per model and tenant.",
			},
			[]string{"model", "tenant_id"},
		),
		InferenceThroughput: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "inference_success_total",
				Help:      "Total number of successful inferences, per model and tenant.",
			},
			[]string{"model", "tenant_id"},
		),
		InferenceQueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "inference_queue_depth",
				Help:      "Current number of in-flight inference requests per model.",
			},
			[]string{"model"},
		),
		ModelLoadLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "model_load_latency_seconds",
				Help:      "Model loading latency in seconds.",
				Buckets:   []float64{1.0, 5.0, 10.0, 30.0, 60.0, 120.0},
			},
			[]string{"model"},
		),
		GPUUtilization: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "kaivue",
				Subsystem: "triton",
				Name:      "gpu_utilization_percent",
				Help:      "GPU utilization percentage per model instance.",
			},
			[]string{"model", "gpu_id"},
		),
	}
}

// Register registers all metrics with the given registerer.
func (m *Metrics) Register(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		m.InferenceLatency,
		m.InferenceRequestsTotal,
		m.InferenceErrorsTotal,
		m.InferenceThroughput,
		m.InferenceQueueDepth,
		m.ModelLoadLatency,
		m.GPUUtilization,
	}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}
