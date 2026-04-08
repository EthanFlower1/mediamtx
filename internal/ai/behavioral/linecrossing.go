package behavioral

import (
	"context"
	"log/slog"
	"sync"
)

// LineCrossingDetector fires an event each time a tracked person's centroid
// crosses the configured line segment.  It records the previous centroid for
// each track and uses the cross-product sign change to detect crossings.
//
// Direction is reported as DirectionAB (from A-side to B-side) or DirectionBA.
//
// The line AB divides the plane.  For a point P, the signed area of the
// triangle (A, B, P) is positive on one side and negative on the other.  A
// crossing occurs when the sign flips between consecutive frames.
type LineCrossingDetector struct {
	cfg    LineCrossingParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu     sync.Mutex
	prevP  map[int64]Point // last known centroid per track
	closed bool
	ch     chan BehavioralEvent
}

// NewLineCrossingDetector creates a LineCrossingDetector from cfg.
func NewLineCrossingDetector(
	id, tenantID, cameraID string,
	cfg LineCrossingParams,
	logger *slog.Logger,
) *LineCrossingDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &LineCrossingDetector{
		cfg:    cfg,
		id:     id,
		tenant: tenantID,
		camera: cameraID,
		logger: logger.With("detector", "line_crossing", "camera", cameraID),
		prevP:  make(map[int64]Point),
		ch:     make(chan BehavioralEvent, 64),
	}
}

// sideOf returns the signed area of triangle (A, B, P) × 2.  Positive means
// P is to the left of vector AB; negative means right.
func sideOf(a, b, p Point) float64 {
	return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
}

// Feed implements Detector.
func (d *LineCrossingDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	a, b := d.cfg.Line.A, d.cfg.Line.B

	for i := range frame.Detections {
		det := &frame.Detections[i]
		if det.TrackID == 0 {
			continue
		}
		cur := det.Box.Center()
		prev, seen := d.prevP[det.TrackID]
		d.prevP[det.TrackID] = cur
		if !seen {
			continue
		}

		prevSide := sideOf(a, b, prev)
		curSide := sideOf(a, b, cur)
		// A crossing occurs when the signs differ AND neither is zero (on the line).
		if prevSide == 0 || curSide == 0 {
			continue
		}
		if (prevSide > 0) == (curSide > 0) {
			continue // same side
		}

		var dir CrossingDirection
		if prevSide > 0 {
			dir = DirectionAB // was on A-side (left), now B-side (right)
		} else {
			dir = DirectionBA
		}

		evt := BehavioralEvent{
			TenantID:   frame.TenantID,
			CameraID:   frame.CameraID,
			Kind:       EventLineCrossing,
			At:         frame.Timestamp,
			TrackID:    det.TrackID,
			Direction:  dir,
			DetectorID: d.id,
		}
		select {
		case d.ch <- evt:
		default:
			d.logger.Warn("line_crossing event channel full; dropping event",
				"track_id", det.TrackID)
		}
	}
}

// Events implements Detector.
func (d *LineCrossingDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *LineCrossingDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
