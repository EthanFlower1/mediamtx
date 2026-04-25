package ai

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// DetectionMetrics collects inference latency, queue depth, frame drops, and
// per-camera/per-model statistics for the AI detection pipeline. It is safe
// for concurrent use.
type DetectionMetrics struct {
	mu sync.RWMutex

	// Per-model inference latency histograms (model -> latencies in seconds).
	modelLatencies map[string]*latencyHistogram

	// Per-camera stats.
	cameraStats map[string]*CameraDetectionStats

	// Global counters.
	totalInferences   int64
	totalFrameDrops   int64
	totalDetections   int64
	currentQueueDepth int64

	startTime time.Time
}

// latencyHistogram stores recent latency samples for percentile computation.
type latencyHistogram struct {
	samples []float64
	maxSize int
	pos     int
	count   int
}

func newLatencyHistogram(maxSize int) *latencyHistogram {
	return &latencyHistogram{
		samples: make([]float64, maxSize),
		maxSize: maxSize,
	}
}

func (h *latencyHistogram) Add(v float64) {
	h.samples[h.pos] = v
	h.pos = (h.pos + 1) % h.maxSize
	if h.count < h.maxSize {
		h.count++
	}
}

func (h *latencyHistogram) Percentiles() (p50, p95, p99 float64) {
	if h.count == 0 {
		return 0, 0, 0
	}
	sorted := make([]float64, h.count)
	if h.count < h.maxSize {
		copy(sorted, h.samples[:h.count])
	} else {
		copy(sorted, h.samples)
	}
	sort.Float64s(sorted)
	p50 = sorted[int(math.Floor(float64(len(sorted)-1)*0.50))]
	p95 = sorted[int(math.Floor(float64(len(sorted)-1)*0.95))]
	p99 = sorted[int(math.Floor(float64(len(sorted)-1)*0.99))]
	return
}

func (h *latencyHistogram) Mean() float64 {
	if h.count == 0 {
		return 0
	}
	n := h.count
	var total float64
	if n < h.maxSize {
		for i := 0; i < n; i++ {
			total += h.samples[i]
		}
	} else {
		for _, v := range h.samples {
			total += v
		}
	}
	return total / float64(n)
}

func (h *latencyHistogram) Count() int {
	return h.count
}

// CameraDetectionStats holds per-camera detection throughput metrics.
type CameraDetectionStats struct {
	CameraID       string
	CameraName     string
	TotalFrames    int64
	TotalDropped   int64
	TotalDetected  int64 // total detection count across all frames
	InferenceCount int64
	latency        *latencyHistogram
}

// NewDetectionMetrics creates a new metrics collector.
func NewDetectionMetrics() *DetectionMetrics {
	return &DetectionMetrics{
		modelLatencies: make(map[string]*latencyHistogram),
		cameraStats:    make(map[string]*CameraDetectionStats),
		startTime:      time.Now(),
	}
}

// RecordInference records a completed inference with its model name, camera,
// duration, and number of detections found.
func (m *DetectionMetrics) RecordInference(model, cameraID, cameraName string, duration time.Duration, detectionCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	secs := duration.Seconds()

	// Per-model latency.
	hist, ok := m.modelLatencies[model]
	if !ok {
		hist = newLatencyHistogram(10000)
		m.modelLatencies[model] = hist
	}
	hist.Add(secs)

	// Per-camera stats.
	cs, ok := m.cameraStats[cameraID]
	if !ok {
		cs = &CameraDetectionStats{
			CameraID:   cameraID,
			CameraName: cameraName,
			latency:    newLatencyHistogram(10000),
		}
		m.cameraStats[cameraID] = cs
	}
	cs.TotalFrames++
	cs.InferenceCount++
	cs.TotalDetected += int64(detectionCount)
	cs.latency.Add(secs)

	m.totalInferences++
	m.totalDetections += int64(detectionCount)
}

// RecordFrameDrop records a dropped frame for a camera.
func (m *DetectionMetrics) RecordFrameDrop(cameraID, cameraName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs, ok := m.cameraStats[cameraID]
	if !ok {
		cs = &CameraDetectionStats{
			CameraID:   cameraID,
			CameraName: cameraName,
			latency:    newLatencyHistogram(10000),
		}
		m.cameraStats[cameraID] = cs
	}
	cs.TotalFrames++
	cs.TotalDropped++
	m.totalFrameDrops++
}

// SetQueueDepth sets the current detection queue depth.
func (m *DetectionMetrics) SetQueueDepth(depth int64) {
	m.mu.Lock()
	m.currentQueueDepth = depth
	m.mu.Unlock()
}

// Snapshot returns a JSON-serialisable snapshot of all detection metrics.
func (m *DetectionMetrics) Snapshot() DetectionMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := DetectionMetricsSnapshot{
		Timestamp:       time.Now().Unix(),
		UptimeSeconds:   int64(time.Since(m.startTime).Seconds()),
		TotalInferences: m.totalInferences,
		TotalDetections: m.totalDetections,
		TotalFrameDrops: m.totalFrameDrops,
		QueueDepth:      m.currentQueueDepth,
		Models:          make([]ModelMetrics, 0, len(m.modelLatencies)),
		Cameras:         make([]CameraMetrics, 0, len(m.cameraStats)),
	}

	// Global latency across all models.
	if m.totalInferences > 0 {
		allHist := newLatencyHistogram(10000)
		for _, hist := range m.modelLatencies {
			n := hist.count
			if n < hist.maxSize {
				for i := 0; i < n; i++ {
					allHist.Add(hist.samples[i])
				}
			} else {
				for _, v := range hist.samples {
					allHist.Add(v)
				}
			}
		}
		snap.InferenceLatency.P50, snap.InferenceLatency.P95, snap.InferenceLatency.P99 = allHist.Percentiles()
		snap.InferenceLatency.Mean = allHist.Mean()
		snap.InferenceLatency.Count = int64(allHist.Count())
	}

	for model, hist := range m.modelLatencies {
		mm := ModelMetrics{
			Model:          model,
			InferenceCount: int64(hist.Count()),
			MeanLatency:    hist.Mean(),
		}
		mm.P50, mm.P95, mm.P99 = hist.Percentiles()
		snap.Models = append(snap.Models, mm)
	}

	for _, cs := range m.cameraStats {
		cm := CameraMetrics{
			CameraID:        cs.CameraID,
			CameraName:      cs.CameraName,
			TotalFrames:     cs.TotalFrames,
			DroppedFrames:   cs.TotalDropped,
			TotalDetections: cs.TotalDetected,
			InferenceCount:  cs.InferenceCount,
			MeanLatency:     cs.latency.Mean(),
		}
		if cs.TotalFrames > 0 {
			cm.DropRate = float64(cs.TotalDropped) / float64(cs.TotalFrames)
		}
		cm.P50, cm.P95, cm.P99 = cs.latency.Percentiles()
		snap.Cameras = append(snap.Cameras, cm)
	}

	return snap
}

// PrometheusExport returns all detection metrics in Prometheus text exposition format.
func (m *DetectionMetrics) PrometheusExport() string {
	snap := m.Snapshot()
	var b strings.Builder

	w := func(name, help, typ string) {
		fmt.Fprintf(&b, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
	}

	// Global counters.
	w("nvr_ai_inferences_total", "Total number of AI inference runs.", "counter")
	fmt.Fprintf(&b, "nvr_ai_inferences_total %d\n", snap.TotalInferences)

	w("nvr_ai_detections_total", "Total number of objects detected.", "counter")
	fmt.Fprintf(&b, "nvr_ai_detections_total %d\n", snap.TotalDetections)

	w("nvr_ai_frame_drops_total", "Total number of frames dropped.", "counter")
	fmt.Fprintf(&b, "nvr_ai_frame_drops_total %d\n", snap.TotalFrameDrops)

	w("nvr_ai_queue_depth", "Current detection queue depth.", "gauge")
	fmt.Fprintf(&b, "nvr_ai_queue_depth %d\n", snap.QueueDepth)

	// Global latency summary.
	w("nvr_ai_inference_latency_seconds", "Inference latency in seconds.", "summary")
	fmt.Fprintf(&b, "nvr_ai_inference_latency_seconds{quantile=\"0.5\"} %g\n", snap.InferenceLatency.P50)
	fmt.Fprintf(&b, "nvr_ai_inference_latency_seconds{quantile=\"0.95\"} %g\n", snap.InferenceLatency.P95)
	fmt.Fprintf(&b, "nvr_ai_inference_latency_seconds{quantile=\"0.99\"} %g\n", snap.InferenceLatency.P99)
	fmt.Fprintf(&b, "nvr_ai_inference_latency_seconds_sum %g\n", snap.InferenceLatency.Mean*float64(snap.InferenceLatency.Count))
	fmt.Fprintf(&b, "nvr_ai_inference_latency_seconds_count %d\n", snap.InferenceLatency.Count)

	// Per-model metrics.
	w("nvr_ai_model_inference_latency_seconds", "Per-model inference latency in seconds.", "summary")
	for _, mm := range snap.Models {
		label := fmt.Sprintf("model=%q", mm.Model)
		fmt.Fprintf(&b, "nvr_ai_model_inference_latency_seconds{%s,quantile=\"0.5\"} %g\n", label, mm.P50)
		fmt.Fprintf(&b, "nvr_ai_model_inference_latency_seconds{%s,quantile=\"0.95\"} %g\n", label, mm.P95)
		fmt.Fprintf(&b, "nvr_ai_model_inference_latency_seconds{%s,quantile=\"0.99\"} %g\n", label, mm.P99)
		fmt.Fprintf(&b, "nvr_ai_model_inference_latency_seconds_sum{%s} %g\n", label, mm.MeanLatency*float64(mm.InferenceCount))
		fmt.Fprintf(&b, "nvr_ai_model_inference_latency_seconds_count{%s} %d\n", label, mm.InferenceCount)
	}

	// Per-camera metrics.
	w("nvr_ai_camera_frames_total", "Total frames processed per camera.", "counter")
	for _, cm := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cm.CameraID, cm.CameraName)
		fmt.Fprintf(&b, "nvr_ai_camera_frames_total{%s} %d\n", labels, cm.TotalFrames)
	}

	w("nvr_ai_camera_frame_drops_total", "Total frames dropped per camera.", "counter")
	for _, cm := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cm.CameraID, cm.CameraName)
		fmt.Fprintf(&b, "nvr_ai_camera_frame_drops_total{%s} %d\n", labels, cm.DroppedFrames)
	}

	w("nvr_ai_camera_detections_total", "Total detections per camera.", "counter")
	for _, cm := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cm.CameraID, cm.CameraName)
		fmt.Fprintf(&b, "nvr_ai_camera_detections_total{%s} %d\n", labels, cm.TotalDetections)
	}

	w("nvr_ai_camera_inference_latency_seconds", "Per-camera inference latency in seconds.", "summary")
	for _, cm := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cm.CameraID, cm.CameraName)
		fmt.Fprintf(&b, "nvr_ai_camera_inference_latency_seconds{%s,quantile=\"0.5\"} %g\n", labels, cm.P50)
		fmt.Fprintf(&b, "nvr_ai_camera_inference_latency_seconds{%s,quantile=\"0.95\"} %g\n", labels, cm.P95)
		fmt.Fprintf(&b, "nvr_ai_camera_inference_latency_seconds{%s,quantile=\"0.99\"} %g\n", labels, cm.P99)
		fmt.Fprintf(&b, "nvr_ai_camera_inference_latency_seconds_sum{%s} %g\n", labels, cm.MeanLatency*float64(cm.InferenceCount))
		fmt.Fprintf(&b, "nvr_ai_camera_inference_latency_seconds_count{%s} %d\n", labels, cm.InferenceCount)
	}

	w("nvr_ai_camera_drop_rate", "Frame drop rate per camera (0-1).", "gauge")
	for _, cm := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cm.CameraID, cm.CameraName)
		fmt.Fprintf(&b, "nvr_ai_camera_drop_rate{%s} %g\n", labels, cm.DropRate)
	}

	return b.String()
}

// DetectionMetricsSnapshot is the JSON-serialisable snapshot returned by the API.
type DetectionMetricsSnapshot struct {
	Timestamp        int64           `json:"timestamp"`
	UptimeSeconds    int64           `json:"uptime_seconds"`
	TotalInferences  int64           `json:"total_inferences"`
	TotalDetections  int64           `json:"total_detections"`
	TotalFrameDrops  int64           `json:"total_frame_drops"`
	QueueDepth       int64           `json:"queue_depth"`
	InferenceLatency LatencyStats    `json:"inference_latency"`
	Models           []ModelMetrics  `json:"models"`
	Cameras          []CameraMetrics `json:"cameras"`
}

// LatencyStats holds percentile latency statistics.
type LatencyStats struct {
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Mean  float64 `json:"mean"`
	Count int64   `json:"count"`
}

// ModelMetrics holds per-model inference metrics.
type ModelMetrics struct {
	Model          string  `json:"model"`
	InferenceCount int64   `json:"inference_count"`
	MeanLatency    float64 `json:"mean_latency_seconds"`
	P50            float64 `json:"p50"`
	P95            float64 `json:"p95"`
	P99            float64 `json:"p99"`
}

// CameraMetrics holds per-camera detection throughput metrics.
type CameraMetrics struct {
	CameraID        string  `json:"camera_id"`
	CameraName      string  `json:"camera_name"`
	TotalFrames     int64   `json:"total_frames"`
	DroppedFrames   int64   `json:"dropped_frames"`
	DropRate        float64 `json:"drop_rate"`
	TotalDetections int64   `json:"total_detections"`
	InferenceCount  int64   `json:"inference_count"`
	MeanLatency     float64 `json:"mean_latency_seconds"`
	P50             float64 `json:"p50"`
	P95             float64 `json:"p95"`
	P99             float64 `json:"p99"`
}
