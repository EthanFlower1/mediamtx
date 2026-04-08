package behavioral

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// crossRecord records a single line crossing for tailgating detection.
type crossRecord struct {
	at        time.Time
	direction CrossingDirection
	trackID   int64
}

// TailgatingDetector fires an EventTailgating event when two persons cross a
// configured line from the same direction within a configurable time window.
//
// Algorithm: buffer the last WindowSeconds of crossings.  On each new
// crossing, scan the buffer for any earlier crossing from the same direction
// within the window.  If found, emit a tailgating event.
//
// The detector is access-control adjacent: it catches multiple persons passing
// through a controlled portal without re-authentication between them.
type TailgatingDetector struct {
	cfg    TailgatingParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu        sync.Mutex
	crossings []crossRecord
	closed    bool
	ch        chan BehavioralEvent
}

// NewTailgatingDetector creates a TailgatingDetector from cfg.
func NewTailgatingDetector(
	id, tenantID, cameraID string,
	cfg TailgatingParams,
	logger *slog.Logger,
) *TailgatingDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &TailgatingDetector{
		cfg:    cfg,
		id:     id,
		tenant: tenantID,
		camera: cameraID,
		logger: logger.With("detector", "tailgating", "camera", cameraID),
		ch:     make(chan BehavioralEvent, 64),
	}
}

// Feed implements Detector.
func (d *TailgatingDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	now := frame.Timestamp
	window := time.Duration(float64(time.Second) * d.cfg.WindowSeconds)
	a, b := d.cfg.Line.A, d.cfg.Line.B

	// Detect new crossings in this frame.
	// TailgatingDetector detects when a centroid is on or very near the line
	// (|sideOf| < epsilon).  Production deployments should compose
	// LineCrossingDetector output into TailgatingDetector to avoid duplicating
	// per-track previous-position state.
	for i := range frame.Detections {
		det := &frame.Detections[i]
		if det.TrackID == 0 {
			continue
		}
		cur := det.Box.Center()

		const epsilon = 0.01
		side := sideOf(a, b, cur)
		if side > -epsilon && side < epsilon {
			// On the line: treat as a crossing.
			dir := DirectionAB
			if side < 0 {
				dir = DirectionBA
			}
			d.processCrossing(frame, det.TrackID, dir, now, window)
		}
	}

	// Prune old records outside the window.
	cutoff := now.Add(-window)
	fresh := d.crossings[:0]
	for _, r := range d.crossings {
		if r.at.After(cutoff) {
			fresh = append(fresh, r)
		}
	}
	d.crossings = fresh
}

func (d *TailgatingDetector) processCrossing(
	frame DetectionFrame,
	trackID int64,
	dir CrossingDirection,
	at time.Time,
	window time.Duration,
) {
	cutoff := at.Add(-window)

	// Check for earlier crossing from same direction within the window.
	for _, prev := range d.crossings {
		if prev.trackID == trackID {
			continue // same person
		}
		if prev.direction != dir {
			continue // different direction
		}
		if prev.at.Before(cutoff) {
			continue // too old
		}
		// Tailgate detected.
		evt := BehavioralEvent{
			TenantID:   frame.TenantID,
			CameraID:   frame.CameraID,
			Kind:       EventTailgating,
			At:         at,
			TrackID:    trackID,
			Direction:  dir,
			DetectorID: d.id,
		}
		select {
		case d.ch <- evt:
		default:
			d.logger.Warn("tailgating event channel full; dropping event")
		}
		break // one event per crossing
	}

	// Buffer this crossing.
	d.crossings = append(d.crossings, crossRecord{at: at, direction: dir, trackID: trackID})
}

// Events implements Detector.
func (d *TailgatingDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *TailgatingDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
