// internal/nvr/ai/dedup.go
package ai

import (
	"context"
	"time"
)

const (
	defaultDedupWindow = 5 * time.Second
	defaultDedupIoU    = float32(0.5)
)

// recentEntry records a recently seen object for deduplication.
type recentEntry struct {
	class      string
	confidence float32
	box        BoundingBox
	seenAt     time.Time
	trackID    int
}

// Deduplicator suppresses duplicate ObjectEntered events when the same class
// reappears at a similar position (IoU > threshold) within a configurable
// time window. It keeps the highest-confidence detection as representative.
type Deduplicator struct {
	in     <-chan TrackedFrame
	out    chan TrackedFrame
	window time.Duration
	minIoU float32

	recent []recentEntry
}

// NewDeduplicator creates a deduplication filter. windowSec controls the
// suppression window in seconds (default 5). minIoU is the minimum IoU
// overlap to consider two detections as duplicates (default 0.5).
func NewDeduplicator(
	in <-chan TrackedFrame,
	out chan TrackedFrame,
	windowSec int,
	minIoU float32,
) *Deduplicator {
	w := defaultDedupWindow
	if windowSec > 0 {
		w = time.Duration(windowSec) * time.Second
	}
	iou := defaultDedupIoU
	if minIoU > 0 {
		iou = minIoU
	}
	return &Deduplicator{
		in:     in,
		out:    out,
		window: w,
		minIoU: iou,
	}
}

// Run processes tracked frames until the input channel closes or ctx is cancelled.
func (d *Deduplicator) Run(ctx context.Context) {
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

func (d *Deduplicator) process(tf TrackedFrame) TrackedFrame {
	// Expire old entries.
	d.pruneExpired(tf.Timestamp)

	var kept []TrackedObject
	for _, obj := range tf.Objects {
		switch obj.State {
		case ObjectEntered:
			if dup := d.findDuplicate(obj); dup != nil {
				// Duplicate detected. If the new detection has higher
				// confidence, update the representative entry.
				if obj.Confidence > dup.confidence {
					dup.confidence = obj.Confidence
					dup.box = obj.Box
					dup.seenAt = tf.Timestamp
				}
				// Suppress this entered event entirely.
				continue
			}
			// New unique detection: record it and pass through.
			d.recent = append(d.recent, recentEntry{
				class:      obj.Class,
				confidence: obj.Confidence,
				box:        obj.Box,
				seenAt:     tf.Timestamp,
				trackID:    obj.TrackID,
			})
			kept = append(kept, obj)

		case ObjectActive:
			// Update the position/confidence of the tracked entry if present.
			d.updateEntry(obj, tf.Timestamp)
			kept = append(kept, obj)

		case ObjectLeft:
			kept = append(kept, obj)
		}
	}

	return TrackedFrame{
		Timestamp: tf.Timestamp,
		Objects:   kept,
		Image:     tf.Image,
	}
}

// findDuplicate checks if obj overlaps with a recent entry of the same class.
func (d *Deduplicator) findDuplicate(obj TrackedObject) *recentEntry {
	for i := range d.recent {
		e := &d.recent[i]
		if e.class != obj.Class {
			continue
		}
		if iouBoxes(e.box, obj.Box) >= d.minIoU {
			return e
		}
	}
	return nil
}

// updateEntry refreshes the bounding box and timestamp for a tracked object
// so the dedup window stays accurate for moving objects.
func (d *Deduplicator) updateEntry(obj TrackedObject, ts time.Time) {
	for i := range d.recent {
		if d.recent[i].trackID == obj.TrackID {
			d.recent[i].box = obj.Box
			d.recent[i].seenAt = ts
			if obj.Confidence > d.recent[i].confidence {
				d.recent[i].confidence = obj.Confidence
			}
			return
		}
	}
}

// pruneExpired removes entries older than the dedup window.
func (d *Deduplicator) pruneExpired(now time.Time) {
	n := 0
	for _, e := range d.recent {
		if now.Sub(e.seenAt) < d.window {
			d.recent[n] = e
			n++
		}
	}
	d.recent = d.recent[:n]
}
