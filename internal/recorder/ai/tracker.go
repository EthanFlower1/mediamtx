// internal/nvr/ai/tracker.go
package ai

import (
	"context"
	"sort"
	"time"
)

const defaultMinIoU = float32(0.3)

// Tracker assigns persistent IDs to detections across frames using IoU matching.
type Tracker struct {
	in           <-chan DetectionFrame
	out          chan TrackedFrame
	trackTimeout time.Duration

	nextID int
	tracks []*track
}

type track struct {
	id          int
	class       string
	confidence  float32
	box         BoundingBox
	firstSeen   time.Time
	lastSeen    time.Time
	missedFor   time.Duration
}

// NewTracker creates a new Tracker. trackTimeoutSec is how many seconds a
// track can be missing before it is marked as left.
func NewTracker(in <-chan DetectionFrame, out chan TrackedFrame, trackTimeoutSec int) *Tracker {
	if trackTimeoutSec <= 0 {
		trackTimeoutSec = 5
	}
	return &Tracker{
		in:           in,
		out:          out,
		trackTimeout: time.Duration(trackTimeoutSec) * time.Second,
		nextID:       1,
	}
}

// Run processes detection frames until the input channel closes or ctx is cancelled.
func (tr *Tracker) Run(ctx context.Context) {
	defer close(tr.out)

	var lastTimestamp time.Time

	for {
		select {
		case <-ctx.Done():
			tr.emitLeftAll(lastTimestamp)
			return

		case df, ok := <-tr.in:
			if !ok {
				tr.emitLeftAll(lastTimestamp)
				return
			}
			lastTimestamp = df.Timestamp
			tf := tr.process(df)
			select {
			case tr.out <- tf:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (tr *Tracker) process(df DetectionFrame) TrackedFrame {
	dt := df.Timestamp.Sub(tr.lastTime(df.Timestamp))

	// Update missed duration for all tracks.
	for _, t := range tr.tracks {
		t.missedFor += dt
	}

	// Build IoU cost pairs.
	type pair struct {
		trackIdx int
		detIdx   int
		iou      float32
	}
	var pairs []pair
	for ti, t := range tr.tracks {
		for di, d := range df.Detections {
			v := iouBoxes(t.box, d.Box)
			if v >= defaultMinIoU {
				pairs = append(pairs, pair{ti, di, v})
			}
		}
	}

	// Greedy assignment: highest IoU first.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].iou > pairs[j].iou })

	matchedTracks := make(map[int]bool)
	matchedDets := make(map[int]bool)

	for _, p := range pairs {
		if matchedTracks[p.trackIdx] || matchedDets[p.detIdx] {
			continue
		}
		matchedTracks[p.trackIdx] = true
		matchedDets[p.detIdx] = true

		t := tr.tracks[p.trackIdx]
		d := df.Detections[p.detIdx]
		t.box = d.Box
		if d.Confidence > t.confidence {
			t.class = d.Class
			t.confidence = d.Confidence
		}
		t.lastSeen = df.Timestamp
		t.missedFor = 0
	}

	var objects []TrackedObject

	// Emit matched tracks as active.
	for ti, t := range tr.tracks {
		if matchedTracks[ti] {
			objects = append(objects, TrackedObject{
				TrackID:    t.id,
				State:      ObjectActive,
				Class:      t.class,
				Confidence: t.confidence,
				Box:        t.box,
				FirstSeen:  t.firstSeen,
				LastSeen:   t.lastSeen,
			})
		}
	}

	// Create new tracks for unmatched detections.
	for di, d := range df.Detections {
		if matchedDets[di] {
			continue
		}
		t := &track{
			id:         tr.nextID,
			class:      d.Class,
			confidence: d.Confidence,
			box:        d.Box,
			firstSeen:  df.Timestamp,
			lastSeen:   df.Timestamp,
		}
		tr.nextID++
		tr.tracks = append(tr.tracks, t)
		objects = append(objects, TrackedObject{
			TrackID:    t.id,
			State:      ObjectEntered,
			Class:      t.class,
			Confidence: t.confidence,
			Box:        t.box,
			FirstSeen:  t.firstSeen,
			LastSeen:   t.lastSeen,
		})
	}

	// Emit "left" and prune expired tracks.
	var remaining []*track
	for ti, t := range tr.tracks {
		if matchedTracks[ti] || t.missedFor == 0 {
			remaining = append(remaining, t)
			continue
		}
		if t.missedFor >= tr.trackTimeout {
			objects = append(objects, TrackedObject{
				TrackID:    t.id,
				State:      ObjectLeft,
				Class:      t.class,
				Confidence: t.confidence,
				Box:        t.box,
				FirstSeen:  t.firstSeen,
				LastSeen:   t.lastSeen,
			})
		} else {
			remaining = append(remaining, t)
		}
	}
	tr.tracks = remaining

	return TrackedFrame{
		Timestamp: df.Timestamp,
		Objects:   objects,
		Image:     df.Image,
	}
}

func (tr *Tracker) lastTime(fallback time.Time) time.Time {
	for _, t := range tr.tracks {
		if !t.lastSeen.IsZero() {
			return t.lastSeen
		}
	}
	return fallback
}

func (tr *Tracker) emitLeftAll(ts time.Time) {
	if len(tr.tracks) == 0 {
		return
	}
	var objects []TrackedObject
	for _, t := range tr.tracks {
		objects = append(objects, TrackedObject{
			TrackID:    t.id,
			State:      ObjectLeft,
			Class:      t.class,
			Confidence: t.confidence,
			Box:        t.box,
			FirstSeen:  t.firstSeen,
			LastSeen:   t.lastSeen,
		})
	}
	tr.tracks = nil
	select {
	case tr.out <- TrackedFrame{Timestamp: ts, Objects: objects}:
	default:
	}
}

// iouBoxes computes IoU between two normalized bounding boxes.
func iouBoxes(a, b BoundingBox) float32 {
	ax1, ay1 := a.X, a.Y
	ax2, ay2 := a.X+a.W, a.Y+a.H
	bx1, by1 := b.X, b.Y
	bx2, by2 := b.X+b.W, b.Y+b.H

	ix1 := max32(ax1, bx1)
	iy1 := max32(ay1, by1)
	ix2 := min32(ax2, bx2)
	iy2 := min32(ay2, by2)

	if ix1 >= ix2 || iy1 >= iy2 {
		return 0
	}

	inter := (ix2 - ix1) * (iy2 - iy1)
	areaA := a.W * a.H
	areaB := b.W * b.H
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
