package behavioral

import (
	"context"
	"log/slog"
	"sync"
)

// CrowdDensityDetector fires an EventCrowdDensity event each time the number
// of persons inside the configured ROI crosses the threshold count.
//
// It fires once per upward threshold breach (count goes from below to at/above
// threshold) and resets when the count drops below the threshold, allowing
// the event to fire again on the next breach.
type CrowdDensityDetector struct {
	cfg    CrowdDensityParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu     sync.Mutex
	over   bool // true = currently at or above threshold (event already fired)
	closed bool
	ch     chan BehavioralEvent
}

// NewCrowdDensityDetector creates a CrowdDensityDetector from cfg.
func NewCrowdDensityDetector(
	id, tenantID, cameraID string,
	cfg CrowdDensityParams,
	logger *slog.Logger,
) *CrowdDensityDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &CrowdDensityDetector{
		cfg:    cfg,
		id:     id,
		tenant: tenantID,
		camera: cameraID,
		logger: logger.With("detector", "crowd_density", "camera", cameraID),
		ch:     make(chan BehavioralEvent, 64),
	}
}

// Feed implements Detector.
func (d *CrowdDensityDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	count := 0
	for i := range frame.Detections {
		det := &frame.Detections[i]
		if d.cfg.ROI.Contains(det.Box.Center()) {
			count++
		}
	}

	atOrAbove := count >= d.cfg.ThresholdCount
	if atOrAbove && !d.over {
		d.over = true
		evt := BehavioralEvent{
			TenantID:    frame.TenantID,
			CameraID:    frame.CameraID,
			Kind:        EventCrowdDensity,
			At:          frame.Timestamp,
			PersonCount: count,
			DetectorID:  d.id,
		}
		select {
		case d.ch <- evt:
		default:
			d.logger.Warn("crowd_density event channel full; dropping event")
		}
	} else if !atOrAbove && d.over {
		// Reset — allow re-firing on next breach.
		d.over = false
	}
}

// Events implements Detector.
func (d *CrowdDensityDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *CrowdDensityDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
