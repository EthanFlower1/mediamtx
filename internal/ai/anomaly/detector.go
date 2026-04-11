package anomaly

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/ai/behavioral"
)

// Detector is the per-camera anomaly detector. It ingests DetectionFrames from
// the behavioral analytics pipeline, maintains a per-camera baseline, and emits
// AnomalyEvents when deviations exceed the sensitivity-derived threshold.
//
// Thread-safe: all methods may be called concurrently.
type Detector struct {
	cfg      Config
	baseline *Baseline
	logger   *slog.Logger

	mu        sync.Mutex
	closed    bool
	ch        chan AnomalyEvent
	lastEvent *AnomalyEvent
}

// NewDetector creates an anomaly Detector with the given config.
func NewDetector(cfg Config, logger *slog.Logger) *Detector {
	if logger == nil {
		logger = slog.Default()
	}
	// Only apply defaults when LearningDays is not explicitly set.
	// LearningDays == 0 means "no learning phase" (useful for testing).
	if cfg.LearningDays < 0 {
		cfg.LearningDays = DefaultLearningDays
	}
	if cfg.Sensitivity < 0 {
		cfg.Sensitivity = DefaultSensitivity
	}
	return &Detector{
		cfg:      cfg,
		baseline: NewBaseline(cfg.CameraID),
		logger:   logger.With("detector", "anomaly", "camera", cfg.CameraID),
		ch:       make(chan AnomalyEvent, 64),
	}
}

// NewDetectorWithBaseline creates a Detector with a pre-populated baseline,
// useful for restoring state or testing.
func NewDetectorWithBaseline(cfg Config, baseline *Baseline, logger *slog.Logger) *Detector {
	d := NewDetector(cfg, logger)
	if baseline != nil {
		d.baseline = baseline
	}
	return d
}

// Feed delivers a DetectionFrame to the anomaly detector. It updates the
// baseline and, if out of the learning phase, checks for anomalies.
func (d *Detector) Feed(_ context.Context, frame behavioral.DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed || !d.cfg.Enabled {
		return
	}

	// Count total objects and per-class counts.
	totalCount := len(frame.Detections)
	classCounts := make(map[string]int, 8)
	for i := range frame.Detections {
		classCounts[frame.Detections[i].Class]++
	}

	// During learning phase, just accumulate and don't score.
	if d.baseline.IsLearning(d.cfg.LearningDays) {
		d.baseline.Observe(frame.Timestamp, totalCount, classCounts)
		return
	}

	// Score BEFORE updating the baseline so the new observation doesn't
	// pollute the statistics used for scoring.
	score, details := d.baseline.Score(frame.Timestamp, totalCount, classCounts)

	// Update the baseline after scoring.
	d.baseline.Observe(frame.Timestamp, totalCount, classCounts)

	threshold := SensitivityToThreshold(d.cfg.Sensitivity)
	if score >= threshold {
		hour := frame.Timestamp.Hour()
		evt := AnomalyEvent{
			TenantID:       frame.TenantID,
			CameraID:       frame.CameraID,
			At:             frame.Timestamp,
			Score:          score,
			Threshold:      threshold,
			HourOfDay:      hour,
			ObservedCount:  totalCount,
			BaselineMean:   d.baseline.HourMean(hour),
			BaselineStdDev: d.baseline.HourStdDev(hour),
			Details:        details,
			Beta:           Beta,
		}
		d.lastEvent = &evt

		select {
		case d.ch <- evt:
		default:
			d.logger.Warn("anomaly event channel full; dropping event",
				"score", score,
				"threshold", threshold)
		}
	}
}

// Events returns the read-only channel on which anomaly events are published.
func (d *Detector) Events() <-chan AnomalyEvent { return d.ch }

// Close shuts down the detector and closes the event channel.
func (d *Detector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}

// Status returns the current status of the detector.
func (d *Detector) Status() StatusResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	return StatusResponse{
		CameraID:     d.cfg.CameraID,
		Enabled:      d.cfg.Enabled,
		Sensitivity:  d.cfg.Sensitivity,
		Learning:     d.baseline.IsLearning(d.cfg.LearningDays),
		LearningDays: d.cfg.LearningDays,
		DaysLearned:  d.baseline.DaysLearned(),
		LastAnomaly:  d.lastEvent,
		Beta:         Beta,
	}
}

// Baseline returns the detector's baseline for inspection.
func (d *Detector) Baseline() *Baseline {
	return d.baseline
}

// UpdateSensitivity updates the sensitivity knob at runtime.
func (d *Detector) UpdateSensitivity(sensitivity float64) error {
	if sensitivity < 0 || sensitivity > 1 {
		return &ValidationError{Field: "sensitivity", Msg: "must be in [0.0, 1.0]"}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg.Sensitivity = sensitivity
	d.logger.Info("sensitivity updated", "sensitivity", sensitivity,
		"threshold", SensitivityToThreshold(sensitivity))
	return nil
}

// Manager maintains anomaly detectors for multiple cameras.
//
// Thread-safe for concurrent use.
type Manager struct {
	mu        sync.RWMutex
	detectors map[string]*Detector // cameraID -> detector
	logger    *slog.Logger
}

// NewManager creates a new anomaly detection Manager.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		detectors: make(map[string]*Detector),
		logger:    logger.With("component", "anomaly-manager"),
	}
}

// GetOrCreate returns the Detector for the given camera, creating one if it
// doesn't exist.
func (m *Manager) GetOrCreate(cfg Config) *Detector {
	m.mu.Lock()
	defer m.mu.Unlock()

	if d, ok := m.detectors[cfg.CameraID]; ok {
		return d
	}

	d := NewDetector(cfg, m.logger)
	m.detectors[cfg.CameraID] = d
	m.logger.Info("created anomaly detector", "camera", cfg.CameraID,
		"sensitivity", cfg.Sensitivity, "learning_days", cfg.LearningDays)
	return d
}

// Get returns the Detector for the given camera, or nil if none exists.
func (m *Manager) Get(cameraID string) *Detector {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.detectors[cameraID]
}

// Remove stops and removes the detector for the given camera.
func (m *Manager) Remove(cameraID string) {
	m.mu.Lock()
	d, ok := m.detectors[cameraID]
	if ok {
		delete(m.detectors, cameraID)
	}
	m.mu.Unlock()

	if ok {
		d.Close()
	}
}

// All returns the status of all detectors.
func (m *Manager) All() []StatusResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]StatusResponse, 0, len(m.detectors))
	for _, d := range m.detectors {
		result = append(result, d.Status())
	}
	return result
}

// CloseAll stops all detectors.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	dets := m.detectors
	m.detectors = make(map[string]*Detector)
	m.mu.Unlock()

	for _, d := range dets {
		d.Close()
	}
}

// FeedAll delivers a DetectionFrame to the appropriate camera's detector.
func (m *Manager) FeedAll(ctx context.Context, frame behavioral.DetectionFrame) {
	d := m.Get(frame.CameraID)
	if d != nil {
		d.Feed(ctx, frame)
	}
}

// SetStartTime overrides the baseline start time for the given camera (testing).
func (d *Detector) SetStartTime(t time.Time) {
	d.baseline.mu.Lock()
	defer d.baseline.mu.Unlock()
	d.baseline.startedAt = t
}
