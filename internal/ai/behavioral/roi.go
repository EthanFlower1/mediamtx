package behavioral

import (
	"context"
	"log/slog"
	"sync"
)

// ROIDetector fires an EventROIEntry when a tracked person's centroid enters
// the configured polygon, and an EventROIExit when it leaves.
//
// Unlike LoiteringDetector, it fires on every transition rather than after a
// dwell threshold.
type ROIDetector struct {
	cfg    ROIParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu     sync.Mutex
	inROI  map[int64]bool // true = currently inside
	closed bool
	ch     chan BehavioralEvent
}

// NewROIDetector creates an ROIDetector from cfg.
func NewROIDetector(
	id, tenantID, cameraID string,
	cfg ROIParams,
	logger *slog.Logger,
) *ROIDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &ROIDetector{
		cfg:    cfg,
		id:     id,
		tenant: tenantID,
		camera: cameraID,
		logger: logger.With("detector", "roi", "camera", cameraID),
		inROI:  make(map[int64]bool),
		ch:     make(chan BehavioralEvent, 64),
	}
}

// Feed implements Detector.
func (d *ROIDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	seen := make(map[int64]struct{}, len(frame.Detections))
	for i := range frame.Detections {
		det := &frame.Detections[i]
		if det.TrackID == 0 {
			continue
		}
		seen[det.TrackID] = struct{}{}

		nowIn := d.cfg.ROI.Contains(det.Box.Center())
		wasIn, known := d.inROI[det.TrackID]

		if !known {
			d.inROI[det.TrackID] = nowIn
			if nowIn {
				// First frame already inside — fire entry.
				d.emit(frame, det.TrackID, EventROIEntry)
			}
			continue
		}

		if nowIn && !wasIn {
			d.inROI[det.TrackID] = true
			d.emit(frame, det.TrackID, EventROIEntry)
		} else if !nowIn && wasIn {
			d.inROI[det.TrackID] = false
			d.emit(frame, det.TrackID, EventROIExit)
		}
	}

	// Clean up vanished tracks.
	for trackID, wasIn := range d.inROI {
		if _, ok := seen[trackID]; !ok {
			if wasIn {
				// Implicit exit when track disappears.
				d.inROI[trackID] = false
			}
		}
	}
}

func (d *ROIDetector) emit(frame DetectionFrame, trackID int64, kind EventKind) {
	evt := BehavioralEvent{
		TenantID:   frame.TenantID,
		CameraID:   frame.CameraID,
		Kind:       kind,
		At:         frame.Timestamp,
		TrackID:    trackID,
		DetectorID: d.id,
	}
	select {
	case d.ch <- evt:
	default:
		d.logger.Warn("roi event channel full; dropping event",
			"kind", kind, "track_id", trackID)
	}
}

// Events implements Detector.
func (d *ROIDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *ROIDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
