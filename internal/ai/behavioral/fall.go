package behavioral

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// heightRecord holds the bounding-box height and timestamp for a single frame.
type heightRecord struct {
	height float64
	at     time.Time
}

// FallDetector fires an EventFall when a person's bounding-box height drops by
// more than HeightDropFraction within WindowSeconds.
//
// Algorithm:
//  1. Maintain a ring of (height, timestamp) records per track.
//  2. On each frame, for each person track, compare the current height against
//     every historical record within WindowSeconds.
//  3. If height dropped by >= HeightDropFraction from any historical record
//     within the window, fire a fall event.
//
// FPS assumption: calibrated for 10–60 FPS.  At lower frame rates the window
// may cover more wall-clock time than intended.
//
// Default parameters: HeightDropFraction = 0.40, WindowSeconds = 0.5.
type FallDetector struct {
	cfg    FallParams
	id     string
	tenant string
	camera string
	logger *slog.Logger

	mu      sync.Mutex
	history map[int64][]heightRecord // per-track ring buffer (kept bounded)
	fired   map[int64]bool           // prevent re-fire during the same fall
	closed  bool
	ch      chan BehavioralEvent
}

const (
	defaultHeightDropFraction = 0.40
	defaultWindowSeconds      = 0.5
	maxHistoryPerTrack        = 64 // enough for 60 FPS × 1s
)

// NewFallDetector creates a FallDetector from cfg.  Zero values are replaced
// by sensible defaults.
func NewFallDetector(id, tenantID, cameraID string, cfg FallParams, logger *slog.Logger) *FallDetector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.HeightDropFraction <= 0 {
		cfg.HeightDropFraction = defaultHeightDropFraction
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = defaultWindowSeconds
	}
	return &FallDetector{
		cfg:     cfg,
		id:      id,
		tenant:  tenantID,
		camera:  cameraID,
		logger:  logger.With("detector", "fall", "camera", cameraID),
		history: make(map[int64][]heightRecord),
		fired:   make(map[int64]bool),
		ch:      make(chan BehavioralEvent, 64),
	}
}

// Feed implements Detector.
func (d *FallDetector) Feed(_ context.Context, frame DetectionFrame) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	now := frame.Timestamp
	window := time.Duration(float64(time.Second) * d.cfg.WindowSeconds)
	cutoff := now.Add(-window)

	seenTracks := make(map[int64]struct{}, len(frame.Detections))
	for i := range frame.Detections {
		det := &frame.Detections[i]
		if det.TrackID == 0 {
			continue
		}
		seenTracks[det.TrackID] = struct{}{}

		curH := det.Box.Height()
		hist := d.history[det.TrackID]

		// Prune old records.
		fresh := hist[:0]
		for _, r := range hist {
			if r.at.After(cutoff) {
				fresh = append(fresh, r)
			}
		}
		hist = fresh

		// Check for fall: curH dropped significantly from any record in window.
		if !d.fired[det.TrackID] {
			for _, r := range hist {
				if r.height > 0 {
					drop := (r.height - curH) / r.height
					if drop >= d.cfg.HeightDropFraction {
						d.fired[det.TrackID] = true
						evt := BehavioralEvent{
							TenantID:   frame.TenantID,
							CameraID:   frame.CameraID,
							Kind:       EventFall,
							At:         now,
							TrackID:    det.TrackID,
							DetectorID: d.id,
						}
						select {
						case d.ch <- evt:
						default:
							d.logger.Warn("fall event channel full; dropping event",
								"track_id", det.TrackID)
						}
						break
					}
				}
			}
		}

		// Append current record, cap ring.
		hist = append(hist, heightRecord{height: curH, at: now})
		if len(hist) > maxHistoryPerTrack {
			hist = hist[len(hist)-maxHistoryPerTrack:]
		}
		d.history[det.TrackID] = hist
	}

	// Reset fired state for tracks that have recovered (height back up).
	// Also clean up vanished tracks.
	for trackID := range d.fired {
		if _, ok := seenTracks[trackID]; !ok {
			delete(d.fired, trackID)
			delete(d.history, trackID)
		}
	}
}

// Events implements Detector.
func (d *FallDetector) Events() <-chan BehavioralEvent { return d.ch }

// Close implements Detector.
func (d *FallDetector) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ch)
	}
}
