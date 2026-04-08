package behavioral

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// trackEntry records the moment a tracked person entered the ROI.
type trackEntry struct {
	enteredAt time.Time
	firedAt   time.Time // zero if not yet fired
}

// LoiteringDetector fires an event when a person (identified by TrackID)
// remains inside a configured ROI for longer than ThresholdSeconds.
//
// The detector fires once per entry and resets when the person leaves the ROI.
// It does not re-fire during the same continuous stay.
//
// Algorithm: on each frame, for every person Detection whose centroid is inside
// the ROI, check whether (now - entryTime) >= threshold.  On exit, remove the
// track from state.
type LoiteringDetector struct {
	cfg    LoiteringParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu     sync.Mutex
	tracks map[int64]*trackEntry
	closed bool
	ch     chan BehavioralEvent
}

// NewLoiteringDetector creates a LoiteringDetector from cfg.
func NewLoiteringDetector(id, tenantID, cameraID string, cfg LoiteringParams, logger *slog.Logger) *LoiteringDetector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 0 // no periodic check; driven purely by frames
	}
	return &LoiteringDetector{
		cfg:    cfg,
		id:     id,
		tenant: tenantID,
		camera: cameraID,
		logger: logger.With("detector", "loitering", "camera", cameraID),
		tracks: make(map[int64]*trackEntry),
		ch:     make(chan BehavioralEvent, 64),
	}
}

// Feed implements Detector.
func (d *LoiteringDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	now := frame.Timestamp
	threshold := time.Duration(float64(time.Second) * d.cfg.ThresholdSeconds)

	// Build set of track IDs currently in the ROI.
	inROI := make(map[int64]struct{}, len(frame.Detections))
	for i := range frame.Detections {
		det := &frame.Detections[i]
		if det.TrackID == 0 {
			continue // no tracking info; cannot maintain state
		}
		if d.cfg.ROI.Contains(det.Box.Center()) {
			inROI[det.TrackID] = struct{}{}
		}
	}

	// Update entry state and fire events.
	for trackID := range inROI {
		entry, ok := d.tracks[trackID]
		if !ok {
			d.tracks[trackID] = &trackEntry{enteredAt: now}
			continue
		}
		dur := now.Sub(entry.enteredAt)
		if dur >= threshold && entry.firedAt.IsZero() {
			entry.firedAt = now
			evt := BehavioralEvent{
				TenantID:      frame.TenantID,
				CameraID:      frame.CameraID,
				Kind:          EventLoitering,
				At:            now,
				TrackID:       trackID,
				DurationInROI: dur,
				DetectorID:    d.id,
			}
			select {
			case d.ch <- evt:
			default:
				d.logger.Warn("loitering event channel full; dropping event",
					"track_id", trackID)
			}
		}
	}

	// Remove tracks that have left the ROI.
	for trackID := range d.tracks {
		if _, stillIn := inROI[trackID]; !stillIn {
			delete(d.tracks, trackID)
		}
	}
}

// Events implements Detector.
func (d *LoiteringDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *LoiteringDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
