package audio

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics collects audio analytics performance data: inference latency,
// detection counts, false positive rate tracking, and per-camera statistics.
type Metrics struct {
	mu sync.RWMutex

	// Per-event-type inference latency.
	eventLatencies map[EventType]*latencyRing

	// Per-camera stats.
	cameraStats map[string]*CameraAudioStats

	// Global counters.
	totalInferences int64
	totalDetections int64

	startTime time.Time
}

// latencyRing is a fixed-size ring buffer for latency percentile computation.
type latencyRing struct {
	samples []float64
	maxSize int
	pos     int
	count   int
}

func newLatencyRing(maxSize int) *latencyRing {
	return &latencyRing{
		samples: make([]float64, maxSize),
		maxSize: maxSize,
	}
}

func (r *latencyRing) Add(v float64) {
	r.samples[r.pos] = v
	r.pos = (r.pos + 1) % r.maxSize
	if r.count < r.maxSize {
		r.count++
	}
}

func (r *latencyRing) Percentiles() (p50, p95, p99 float64) {
	if r.count == 0 {
		return
	}
	sorted := make([]float64, r.count)
	if r.count < r.maxSize {
		copy(sorted, r.samples[:r.count])
	} else {
		copy(sorted, r.samples)
	}
	sort.Float64s(sorted)
	p50 = sorted[int(math.Floor(float64(len(sorted)-1)*0.50))]
	p95 = sorted[int(math.Floor(float64(len(sorted)-1)*0.95))]
	p99 = sorted[int(math.Floor(float64(len(sorted)-1)*0.99))]
	return
}

func (r *latencyRing) Mean() float64 {
	if r.count == 0 {
		return 0
	}
	n := r.count
	var total float64
	if n < r.maxSize {
		for i := 0; i < n; i++ {
			total += r.samples[i]
		}
	} else {
		for _, v := range r.samples {
			total += v
		}
	}
	return total / float64(n)
}

// CameraAudioStats holds per-camera audio detection metrics.
type CameraAudioStats struct {
	CameraID   string
	CameraName string

	// Per-event counters.
	InferenceCounts  map[EventType]int64
	DetectionCounts  map[EventType]int64
	FalsePositives   map[EventType]int64
	LatencyHistogram map[EventType]*latencyRing

	// End-to-end latency (capture to event emission).
	E2ELatency *latencyRing
}

func newCameraAudioStats(cameraID, cameraName string) *CameraAudioStats {
	return &CameraAudioStats{
		CameraID:         cameraID,
		CameraName:       cameraName,
		InferenceCounts:  make(map[EventType]int64),
		DetectionCounts:  make(map[EventType]int64),
		FalsePositives:   make(map[EventType]int64),
		LatencyHistogram: make(map[EventType]*latencyRing),
		E2ELatency:       newLatencyRing(10000),
	}
}

// NewMetrics creates a new audio analytics metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		eventLatencies: make(map[EventType]*latencyRing),
		cameraStats:    make(map[string]*CameraAudioStats),
		startTime:      time.Now(),
	}
}

// RecordInference records a completed audio inference run.
func (m *Metrics) RecordInference(cameraID, cameraName string, evt EventType, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	secs := duration.Seconds()

	// Per-event latency.
	ring, ok := m.eventLatencies[evt]
	if !ok {
		ring = newLatencyRing(10000)
		m.eventLatencies[evt] = ring
	}
	ring.Add(secs)

	// Per-camera.
	cs := m.ensureCameraStats(cameraID, cameraName)
	cs.InferenceCounts[evt]++
	if _, ok := cs.LatencyHistogram[evt]; !ok {
		cs.LatencyHistogram[evt] = newLatencyRing(10000)
	}
	cs.LatencyHistogram[evt].Add(secs)

	m.totalInferences++
}

// RecordDetection records a detection event with its end-to-end latency.
func (m *Metrics) RecordDetection(cameraID, cameraName string, evt EventType, confidence float32, e2eLatency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs := m.ensureCameraStats(cameraID, cameraName)
	cs.DetectionCounts[evt]++
	cs.E2ELatency.Add(e2eLatency.Seconds())

	m.totalDetections++
}

// RecordFalsePositive increments the false positive counter for manual
// FP tracking. Called when an operator marks an event as a false positive.
func (m *Metrics) RecordFalsePositive(cameraID, cameraName string, evt EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs := m.ensureCameraStats(cameraID, cameraName)
	cs.FalsePositives[evt]++
}

func (m *Metrics) ensureCameraStats(cameraID, cameraName string) *CameraAudioStats {
	cs, ok := m.cameraStats[cameraID]
	if !ok {
		cs = newCameraAudioStats(cameraID, cameraName)
		m.cameraStats[cameraID] = cs
	}
	return cs
}

// Snapshot returns a serializable snapshot of all audio analytics metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := MetricsSnapshot{
		Timestamp:       time.Now().Unix(),
		UptimeSeconds:   int64(time.Since(m.startTime).Seconds()),
		TotalInferences: m.totalInferences,
		TotalDetections: m.totalDetections,
		EventTypes:      make([]EventTypeMetrics, 0),
		Cameras:         make([]CameraAudioMetrics, 0),
	}

	for evt, ring := range m.eventLatencies {
		etm := EventTypeMetrics{
			EventType:      evt,
			InferenceCount: int64(ring.count),
			MeanLatency:    ring.Mean(),
		}
		etm.P50, etm.P95, etm.P99 = ring.Percentiles()
		snap.EventTypes = append(snap.EventTypes, etm)
	}

	for _, cs := range m.cameraStats {
		cam := CameraAudioMetrics{
			CameraID:        cs.CameraID,
			CameraName:      cs.CameraName,
			DetectionCounts: make(map[EventType]int64),
			FalsePositives:  make(map[EventType]int64),
			FPRates:         make(map[EventType]float64),
		}

		var totalDet, totalFP int64
		for evt, count := range cs.DetectionCounts {
			cam.DetectionCounts[evt] = count
			totalDet += count
		}
		for evt, count := range cs.FalsePositives {
			cam.FalsePositives[evt] = count
			totalFP += count
		}
		cam.TotalDetections = totalDet
		cam.TotalFalsePositives = totalFP

		// Compute per-event FP rates.
		for evt, detCount := range cs.DetectionCounts {
			fpCount := cs.FalsePositives[evt]
			if detCount > 0 {
				cam.FPRates[evt] = float64(fpCount) / float64(detCount)
			}
		}

		// E2E latency.
		if cs.E2ELatency.count > 0 {
			cam.E2ELatency.Mean = cs.E2ELatency.Mean()
			cam.E2ELatency.P50, cam.E2ELatency.P95, cam.E2ELatency.P99 = cs.E2ELatency.Percentiles()
			cam.E2ELatency.Count = int64(cs.E2ELatency.count)
		}

		snap.Cameras = append(snap.Cameras, cam)
	}

	return snap
}

// PrometheusExport returns all audio metrics in Prometheus text exposition format.
func (m *Metrics) PrometheusExport() string {
	snap := m.Snapshot()
	var b strings.Builder

	w := func(name, help, typ string) {
		fmt.Fprintf(&b, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
	}

	w("nvr_audio_inferences_total", "Total audio inference runs.", "counter")
	fmt.Fprintf(&b, "nvr_audio_inferences_total %d\n", snap.TotalInferences)

	w("nvr_audio_detections_total", "Total audio events detected.", "counter")
	fmt.Fprintf(&b, "nvr_audio_detections_total %d\n", snap.TotalDetections)

	w("nvr_audio_event_inference_latency_seconds", "Per-event-type audio inference latency.", "summary")
	for _, etm := range snap.EventTypes {
		label := fmt.Sprintf("event_type=%q", etm.EventType)
		fmt.Fprintf(&b, "nvr_audio_event_inference_latency_seconds{%s,quantile=\"0.5\"} %g\n", label, etm.P50)
		fmt.Fprintf(&b, "nvr_audio_event_inference_latency_seconds{%s,quantile=\"0.95\"} %g\n", label, etm.P95)
		fmt.Fprintf(&b, "nvr_audio_event_inference_latency_seconds{%s,quantile=\"0.99\"} %g\n", label, etm.P99)
	}

	w("nvr_audio_camera_detections_total", "Total audio detections per camera.", "counter")
	for _, cam := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cam.CameraID, cam.CameraName)
		fmt.Fprintf(&b, "nvr_audio_camera_detections_total{%s} %d\n", labels, cam.TotalDetections)
	}

	w("nvr_audio_camera_false_positives_total", "Total false positives per camera.", "counter")
	for _, cam := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cam.CameraID, cam.CameraName)
		fmt.Fprintf(&b, "nvr_audio_camera_false_positives_total{%s} %d\n", labels, cam.TotalFalsePositives)
	}

	w("nvr_audio_camera_e2e_latency_seconds", "End-to-end audio event latency.", "summary")
	for _, cam := range snap.Cameras {
		labels := fmt.Sprintf("camera_id=%q,camera_name=%q", cam.CameraID, cam.CameraName)
		fmt.Fprintf(&b, "nvr_audio_camera_e2e_latency_seconds{%s,quantile=\"0.5\"} %g\n", labels, cam.E2ELatency.P50)
		fmt.Fprintf(&b, "nvr_audio_camera_e2e_latency_seconds{%s,quantile=\"0.95\"} %g\n", labels, cam.E2ELatency.P95)
		fmt.Fprintf(&b, "nvr_audio_camera_e2e_latency_seconds{%s,quantile=\"0.99\"} %g\n", labels, cam.E2ELatency.P99)
	}

	return b.String()
}

// MetricsSnapshot is the JSON-serializable snapshot of audio analytics metrics.
type MetricsSnapshot struct {
	Timestamp       int64                `json:"timestamp"`
	UptimeSeconds   int64                `json:"uptime_seconds"`
	TotalInferences int64                `json:"total_inferences"`
	TotalDetections int64                `json:"total_detections"`
	EventTypes      []EventTypeMetrics   `json:"event_types"`
	Cameras         []CameraAudioMetrics `json:"cameras"`
}

// EventTypeMetrics holds per-event-type inference metrics.
type EventTypeMetrics struct {
	EventType      EventType `json:"event_type"`
	InferenceCount int64     `json:"inference_count"`
	MeanLatency    float64   `json:"mean_latency_seconds"`
	P50            float64   `json:"p50"`
	P95            float64   `json:"p95"`
	P99            float64   `json:"p99"`
}

// CameraAudioMetrics holds per-camera audio detection metrics.
type CameraAudioMetrics struct {
	CameraID           string                `json:"camera_id"`
	CameraName         string                `json:"camera_name"`
	TotalDetections    int64                 `json:"total_detections"`
	TotalFalsePositives int64               `json:"total_false_positives"`
	DetectionCounts    map[EventType]int64   `json:"detection_counts"`
	FalsePositives     map[EventType]int64   `json:"false_positives"`
	FPRates            map[EventType]float64 `json:"fp_rates"`
	E2ELatency         LatencyStats          `json:"e2e_latency"`
}

// LatencyStats holds percentile latency statistics.
type LatencyStats struct {
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Mean  float64 `json:"mean"`
	Count int64   `json:"count"`
}
