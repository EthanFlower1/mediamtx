// internal/nvr/ai/dedup.go
package ai

import (
	"context"
	"time"
)

// DefaultDedupWindow is the default time window within which duplicate
// detection events for the same object are suppressed.
const DefaultDedupWindow = 3 * time.Second

// DefaultDedupIoU is the minimum IoU overlap for two detections to be
// considered the same object for deduplication purposes.
const DefaultDedupIoU = float32(0.5)

// DedupConfig holds configuration for the deduplication filter.
type DedupConfig struct {
	// Window is the time window within which events with the same class
	// and overlapping bounding box are considered duplicates.
	Window time.Duration
	// MinIoU is the minimum IoU overlap threshold for two detections
	// to be considered the same object.
	MinIoU float32
}

// dedupEntry tracks a recently-emitted detection for dedup comparison.
type dedupEntry struct {
	class      string
	confidence float32
	box        BoundingBox
	trackID    int
	emittedAt  time.Time
}

// Dedup is a pipeline stage that sits between the Tracker and Publisher.
// It suppresses duplicate detection events within a configurable time window
// by comparing class labels and bounding-box IoU overlap. When duplicates
// are found, only the highest-confidence detection is kept as the
// representative event.
type Dedup struct {
	in     <-chan TrackedFrame
	out    chan TrackedFrame
	window time.Duration
	minIoU float32

	recent []dedupEntry
}

// NewDedup creates a new deduplication filter.
func NewDedup(in <-chan TrackedFrame, out chan TrackedFrame, cfg DedupConfig) *Dedup {
	window := cfg.Window
	if window <= 0 {
		window = DefaultDedupWindow
	}
	minIoU := cfg.MinIoU
	if minIoU <= 0 {
		minIoU = DefaultDedupIoU
	}
	return &Dedup{
		in:     in,
		out:    out,
		window: window,
		minIoU: minIoU,
	}
}

// Run processes tracked frames, filtering duplicates, until the input
// channel closes or ctx is cancelled.
func (d *Dedup) Run(ctx context.Context) {
	defer close(d.out)

	for {
		select {
		case <-ctx.Done():
			return
		case tf, ok := <-d.in:
			if !ok {
				return
			}
			filtered := d.process(tf)
			select {
			case d.out <- filtered:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (d *Dedup) process(tf TrackedFrame) TrackedFrame {
	// Prune expired entries.
	d.pruneExpired(tf.Timestamp)

	var kept []TrackedObject
	for _, obj := range tf.Objects {
		switch obj.State {
		case ObjectEntered:
			// Check if this "entered" event is a duplicate of a recent one.
			if idx, found := d.findDuplicate(obj); found {
				// Duplicate found within the window. Keep the higher-confidence
				// detection as the representative.
				if obj.Confidence > d.recent[idx].confidence {
					d.recent[idx].confidence = obj.Confidence
					d.recent[idx].box = obj.Box
					d.recent[idx].trackID = obj.TrackID
					d.recent[idx].emittedAt = tf.Timestamp
				}
				// Suppress this duplicate event from downstream.
				continue
			}
			// New unique detection -- record it and pass through.
			d.recent = append(d.recent, dedupEntry{
				class:      obj.Class,
				confidence: obj.Confidence,
				box:        obj.Box,
				trackID:    obj.TrackID,
				emittedAt:  tf.Timestamp,
			})
			kept = append(kept, obj)

		case ObjectActive, ObjectLeft:
			// Active/Left events always pass through; they represent
			// ongoing tracking state, not new detection triggers.
			kept = append(kept, obj)
		}
	}

	return TrackedFrame{
		Timestamp: tf.Timestamp,
		Objects:   kept,
		Image:     tf.Image,
	}
}

// findDuplicate checks if the given object matches a recent entry by
// class and bounding-box IoU overlap. Returns the index and true if found.
func (d *Dedup) findDuplicate(obj TrackedObject) (int, bool) {
	for i, entry := range d.recent {
		if entry.class != obj.Class {
			continue
		}
		if iouBoxes(entry.box, obj.Box) >= d.minIoU {
			return i, true
		}
	}
	return -1, false
}

// pruneExpired removes entries older than the dedup window.
func (d *Dedup) pruneExpired(now time.Time) {
	n := 0
	for _, entry := range d.recent {
		if now.Sub(entry.emittedAt) < d.window {
			d.recent[n] = entry
			n++
		}
	}
	d.recent = d.recent[:n]
}
