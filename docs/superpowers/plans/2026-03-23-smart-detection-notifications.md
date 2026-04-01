# Smart Detection Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace frame-level class-counting notifications with object tracking (ByteTrack), detection zones, per-zone cooldowns, and enter/loiter/leave state transitions.

**Architecture:** YOLO detections feed into ByteTrack for per-object tracking, then each tracked object is tested against user-defined polygon zones. A per-track per-zone state machine emits enter/loiter/leave events, filtered through per-zone per-class cooldowns before reaching the notification system.

**Tech Stack:** Go (pure ByteTrack + Kalman), SQLite (migration v15), React + TypeScript + Tailwind (zone editor UI)

**Spec:** `docs/superpowers/specs/2026-03-23-smart-detection-notifications-design.md`

---

## File Structure

| File                                     | Action | Responsibility                                   |
| ---------------------------------------- | ------ | ------------------------------------------------ |
| `internal/nvr/ai/kalman.go`              | Create | 2D Kalman filter for bounding box prediction     |
| `internal/nvr/ai/tracker.go`             | Create | ByteTrack multi-object tracker                   |
| `internal/nvr/ai/tracker_test.go`        | Create | Tracker unit tests                               |
| `internal/nvr/ai/zone.go`                | Create | Zone model + PointInPolygon                      |
| `internal/nvr/ai/zone_test.go`           | Create | Zone geometry tests                              |
| `internal/nvr/ai/state.go`               | Create | Per-track per-zone state machine                 |
| `internal/nvr/ai/state_test.go`          | Create | State transition tests                           |
| `internal/nvr/ai/cooldown.go`            | Create | Per-zone per-class cooldown manager              |
| `internal/nvr/ai/cooldown_test.go`       | Create | Cooldown tests                                   |
| `internal/nvr/ai/pipeline.go`            | Modify | Integrate tracker, zones, state, cooldowns       |
| `internal/nvr/db/zones.go`               | Create | Zone + alert rule CRUD                           |
| `internal/nvr/db/migrations.go`          | Modify | Add v15 migration                                |
| `internal/nvr/api/events.go`             | Modify | Structured Event fields, PublishTrackedDetection |
| `internal/nvr/api/zones.go`              | Create | Zone REST endpoints + snapshot proxy             |
| `internal/nvr/api/router.go`             | Modify | Register zone routes                             |
| `internal/nvr/nvr.go`                    | Modify | Pass DB to pipelines for zone loading            |
| `internal/nvr/ai/snapshot.go`            | Create | Shared snapshot capture with digest auth         |
| `ui/src/components/ZoneEditor.tsx`       | Create | Polygon drawing + zone config UI                 |
| `ui/src/components/AnalyticsOverlay.tsx` | Modify | Track IDs, zone overlays, loiter color           |
| `ui/src/hooks/useNotifications.ts`       | Modify | Structured AI fields, action-based titles        |
| `ui/src/pages/CameraManagement.tsx`      | Modify | Add "Zones" button per camera                    |

---

### Task 1: Kalman Filter

**Files:**

- Create: `internal/nvr/ai/kalman.go`

This is a pure-math module with no external dependencies. It implements a 2D constant-velocity Kalman filter for bounding box tracking. The state vector is `[cx, cy, area, aspect, dx, dy, da]` (center x, center y, area, aspect ratio, and their velocities).

- [ ] **Step 1: Create kalman.go with types and constructor**

```go
// internal/nvr/ai/kalman.go
package ai

import "math"

// KalmanState holds the state vector and covariance matrix for a single tracked object.
// State vector: [cx, cy, area, aspect, d_cx, d_cy, d_area]
//   cx, cy   = bounding box center (normalized 0-1)
//   area     = w * h
//   aspect   = w / h
//   d_*      = velocity components
type KalmanState struct {
	X [7]float64    // state vector
	P [7][7]float64 // covariance matrix
}

// NewKalmanState initializes a Kalman state from a bounding box [x, y, w, h] (normalized).
func NewKalmanState(x, y, w, h float64) KalmanState {
	cx := x + w/2
	cy := y + h/2
	area := w * h
	aspect := w / h
	if h == 0 {
		aspect = 1
	}

	ks := KalmanState{}
	ks.X = [7]float64{cx, cy, area, aspect, 0, 0, 0}

	// Initial covariance: high uncertainty on velocity, moderate on position
	for i := 0; i < 4; i++ {
		ks.P[i][i] = 10
	}
	for i := 4; i < 7; i++ {
		ks.P[i][i] = 1000
	}
	return ks
}

// BBox converts the Kalman state back to [x, y, w, h] (normalized).
func (ks *KalmanState) BBox() [4]float64 {
	cx, cy, area, aspect := ks.X[0], ks.X[1], ks.X[2], ks.X[3]
	if area < 0 {
		area = 0
	}
	if aspect <= 0 {
		aspect = 1
	}
	w := math.Sqrt(area * aspect)
	h := area / w
	if w == 0 {
		h = 0
	}
	return [4]float64{cx - w/2, cy - h/2, w, h}
}

// Predict advances the state by one time step using a constant-velocity model.
func (ks *KalmanState) Predict() {
	// State transition: x_new = F * x
	// F is identity with velocity terms:
	//   cx  += d_cx
	//   cy  += d_cy
	//   area += d_area
	ks.X[0] += ks.X[4]
	ks.X[1] += ks.X[5]
	ks.X[2] += ks.X[6]

	// Covariance: P = F*P*F' + Q
	// F is identity + velocity coupling, so we update P accordingly
	// For simplicity, add process noise directly
	var F [7][7]float64
	for i := 0; i < 7; i++ {
		F[i][i] = 1
	}
	F[0][4] = 1 // cx += d_cx
	F[1][5] = 1 // cy += d_cy
	F[2][6] = 1 // area += d_area

	// P = F * P * F^T + Q
	var tmp [7][7]float64
	for i := 0; i < 7; i++ {
		for j := 0; j < 7; j++ {
			for k := 0; k < 7; k++ {
				tmp[i][j] += F[i][k] * ks.P[k][j]
			}
		}
	}
	for i := 0; i < 7; i++ {
		for j := 0; j < 7; j++ {
			ks.P[i][j] = 0
			for k := 0; k < 7; k++ {
				ks.P[i][j] += tmp[i][k] * F[j][k]
			}
		}
	}

	// Process noise Q (diagonal)
	q := [7]float64{1, 1, 1, 1, 0.01, 0.01, 0.001}
	for i := 0; i < 7; i++ {
		ks.P[i][i] += q[i]
	}
}

// Update corrects the state with a measurement [cx, cy, area, aspect].
func (ks *KalmanState) Update(cx, cy, area, aspect float64) {
	// Measurement: z = H * x, where H selects first 4 elements
	z := [4]float64{cx, cy, area, aspect}

	// Innovation: y = z - H*x
	var y [4]float64
	for i := 0; i < 4; i++ {
		y[i] = z[i] - ks.X[i]
	}

	// Innovation covariance: S = H*P*H' + R
	var S [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			S[i][j] = ks.P[i][j]
		}
	}
	// Measurement noise R
	r := [4]float64{1, 1, 10, 10}
	for i := 0; i < 4; i++ {
		S[i][i] += r[i]
	}

	// Kalman gain: K = P*H' * S^-1
	// Since H selects first 4 rows, P*H' is just first 4 columns of P
	Sinv := invert4x4(S)
	var K [7][4]float64
	for i := 0; i < 7; i++ {
		for j := 0; j < 4; j++ {
			for k := 0; k < 4; k++ {
				K[i][j] += ks.P[i][k] * Sinv[k][j]
			}
		}
	}

	// State update: x = x + K*y
	for i := 0; i < 7; i++ {
		for j := 0; j < 4; j++ {
			ks.X[i] += K[i][j] * y[j]
		}
	}

	// Covariance update: P = (I - K*H) * P
	var KH [7][7]float64
	for i := 0; i < 7; i++ {
		for j := 0; j < 4; j++ {
			KH[i][j] = K[i][j] // H is identity for first 4
		}
	}
	var newP [7][7]float64
	for i := 0; i < 7; i++ {
		for j := 0; j < 7; j++ {
			ikh := 0.0
			if i == j {
				ikh = 1
			}
			ikh -= KH[i][j]
			for k := 0; k < 7; k++ {
				val := 0.0
				if i == k {
					val = 1
				}
				val -= KH[i][k]
				newP[i][j] += val * ks.P[k][j]
			}
		}
	}
	ks.P = newP
}

// invert4x4 computes the inverse of a 4x4 matrix using Gauss-Jordan elimination.
func invert4x4(m [4][4]float64) [4][4]float64 {
	var aug [4][8]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			aug[i][j] = m[i][j]
		}
		aug[i][i+4] = 1
	}
	for col := 0; col < 4; col++ {
		pivot := col
		for row := col + 1; row < 4; row++ {
			if math.Abs(aug[row][col]) > math.Abs(aug[pivot][col]) {
				pivot = row
			}
		}
		aug[col], aug[pivot] = aug[pivot], aug[col]
		if aug[col][col] == 0 {
			continue
		}
		div := aug[col][col]
		for j := 0; j < 8; j++ {
			aug[col][j] /= div
		}
		for row := 0; row < 4; row++ {
			if row == col {
				continue
			}
			factor := aug[row][col]
			for j := 0; j < 8; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}
	var inv [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			inv[i][j] = aug[i][j+4]
		}
	}
	return inv
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/ai/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/ai/kalman.go
git commit -m "feat(ai): add Kalman filter for bounding box tracking"
```

---

### Task 2: ByteTrack Tracker

**Files:**

- Create: `internal/nvr/ai/tracker.go`
- Create: `internal/nvr/ai/tracker_test.go`

- [ ] **Step 1: Write tracker_test.go with core tests**

```go
// internal/nvr/ai/tracker_test.go
package ai

import (
	"testing"
)

func TestNewTrackOnFirstDetection(t *testing.T) {
	bt := NewByteTracker()
	dets := []YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.9, X: 0.1, Y: 0.2, W: 0.1, H: 0.2},
	}
	tracked := bt.Update(dets)
	if len(tracked) != 1 {
		t.Fatalf("expected 1 tracked detection, got %d", len(tracked))
	}
	if tracked[0].TrackID != 1 {
		t.Errorf("expected track ID 1, got %d", tracked[0].TrackID)
	}
}

func TestSameObjectGetsConsistentTrackID(t *testing.T) {
	bt := NewByteTracker()

	// Frame 1: person at (0.5, 0.5)
	d1 := []YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.9, X: 0.5, Y: 0.5, W: 0.1, H: 0.2},
	}
	t1 := bt.Update(d1)

	// Frame 2: person moved slightly to (0.52, 0.51) — should match same track
	d2 := []YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.85, X: 0.52, Y: 0.51, W: 0.1, H: 0.2},
	}
	t2 := bt.Update(d2)

	if t1[0].TrackID != t2[0].TrackID {
		t.Errorf("expected same track ID across frames, got %d and %d", t1[0].TrackID, t2[0].TrackID)
	}
}

func TestTwoObjectsGetDifferentTrackIDs(t *testing.T) {
	bt := NewByteTracker()
	dets := []YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.9, X: 0.1, Y: 0.1, W: 0.1, H: 0.2},
		{Class: 2, ClassName: "car", Confidence: 0.8, X: 0.7, Y: 0.7, W: 0.15, H: 0.1},
	}
	tracked := bt.Update(dets)
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked, got %d", len(tracked))
	}
	if tracked[0].TrackID == tracked[1].TrackID {
		t.Error("expected different track IDs for different objects")
	}
}

func TestLostTrackIsDeleted(t *testing.T) {
	bt := NewByteTracker()
	bt.maxLost = 2 // delete after 2 missed frames

	// Frame 1: person detected
	bt.Update([]YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.9, X: 0.5, Y: 0.5, W: 0.1, H: 0.2},
	})

	// Frames 2-4: no detections
	bt.Update(nil)
	bt.Update(nil)
	bt.Update(nil)

	if len(bt.ActiveTracks()) != 0 {
		t.Errorf("expected 0 active tracks after lost timeout, got %d", len(bt.ActiveTracks()))
	}
}

func TestLowConfidenceRecovery(t *testing.T) {
	bt := NewByteTracker()

	// Frame 1: high confidence person
	t1 := bt.Update([]YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.9, X: 0.5, Y: 0.5, W: 0.1, H: 0.2},
	})

	// Frame 2: same person at lower confidence (occluded), should still match
	t2 := bt.Update([]YOLODetection{
		{Class: 0, ClassName: "person", Confidence: 0.35, X: 0.51, Y: 0.51, W: 0.1, H: 0.2},
	})

	if len(t2) != 1 {
		t.Fatalf("expected 1 tracked detection, got %d", len(t2))
	}
	if t1[0].TrackID != t2[0].TrackID {
		t.Errorf("low-confidence detection should recover same track, got %d vs %d", t1[0].TrackID, t2[0].TrackID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run TestNewTrack -v`
Expected: FAIL — `NewByteTracker` not defined

- [ ] **Step 3: Implement tracker.go**

```go
// internal/nvr/ai/tracker.go
package ai

import "sort"

// Track represents a tracked object across frames.
type Track struct {
	ID         int
	Class      string
	Confidence float32
	Kalman     KalmanState
	Lost       int                 // frames since last matched
	ZoneStates map[int64]ZoneState // zone ID -> state machine (populated by state.go)
}

// BBox returns the Kalman-filtered bounding box as [x, y, w, h].
func (t *Track) BBox() [4]float64 {
	return t.Kalman.BBox()
}

// Center returns the center point of the track's bounding box.
func (t *Track) Center() (float64, float64) {
	bb := t.BBox()
	return bb[0] + bb[2]/2, bb[1] + bb[3]/2
}

// TrackedDetection pairs a raw YOLO detection with a track ID.
type TrackedDetection struct {
	YOLODetection
	TrackID int
}

// ByteTracker implements the ByteTrack multi-object tracking algorithm.
type ByteTracker struct {
	tracks     []*Track
	nextID     int
	maxLost    int
	highThresh float32
	lowThresh  float32
	iouThresh  float32
}

// NewByteTracker creates a tracker with default parameters.
func NewByteTracker() *ByteTracker {
	return &ByteTracker{
		maxLost:    30, // 15 seconds at 2 FPS
		highThresh: 0.5,
		lowThresh:  0.3,
		iouThresh:  0.3,
	}
}

// ActiveTracks returns all currently active tracks.
func (bt *ByteTracker) ActiveTracks() []*Track {
	return bt.tracks
}

// Update processes a new frame of detections and returns tracked detections with IDs.
func (bt *ByteTracker) Update(detections []YOLODetection) []TrackedDetection {
	// Predict step for all existing tracks
	for _, t := range bt.tracks {
		t.Kalman.Predict()
	}

	// Split detections into high and low confidence
	var highDets, lowDets []YOLODetection
	for _, d := range detections {
		if d.Confidence >= bt.highThresh {
			highDets = append(highDets, d)
		} else if d.Confidence >= bt.lowThresh {
			lowDets = append(lowDets, d)
		}
	}

	// First pass: match high-confidence detections to tracks
	matchedTracks := make(map[int]bool)
	matchedDets := make(map[int]bool)
	result := make([]TrackedDetection, 0, len(detections))

	matches := bt.greedyMatch(bt.tracks, highDets)
	for _, m := range matches {
		track := bt.tracks[m.trackIdx]
		det := highDets[m.detIdx]
		bt.updateTrack(track, det)
		matchedTracks[m.trackIdx] = true
		matchedDets[m.detIdx] = true
		result = append(result, TrackedDetection{YOLODetection: det, TrackID: track.ID})
	}

	// Collect unmatched tracks for second pass
	var unmatchedTracks []*Track
	var unmatchedTrackIdxs []int
	for i, t := range bt.tracks {
		if !matchedTracks[i] {
			unmatchedTracks = append(unmatchedTracks, t)
			unmatchedTrackIdxs = append(unmatchedTrackIdxs, i)
		}
	}

	// Second pass: match unmatched tracks to low-confidence detections
	lowMatches := bt.greedyMatch(unmatchedTracks, lowDets)
	matchedLowDets := make(map[int]bool)
	for _, m := range lowMatches {
		origIdx := unmatchedTrackIdxs[m.trackIdx]
		track := bt.tracks[origIdx]
		det := lowDets[m.detIdx]
		bt.updateTrack(track, det)
		matchedTracks[origIdx] = true
		matchedLowDets[m.detIdx] = true
		result = append(result, TrackedDetection{YOLODetection: det, TrackID: track.ID})
	}

	// Create new tracks for unmatched high-confidence detections
	for i, det := range highDets {
		if !matchedDets[i] {
			track := bt.createTrack(det)
			result = append(result, TrackedDetection{YOLODetection: det, TrackID: track.ID})
		}
	}

	// Update lost count and prune dead tracks
	var alive []*Track
	for i, t := range bt.tracks {
		if matchedTracks[i] {
			t.Lost = 0
			alive = append(alive, t)
		} else {
			t.Lost++
			if t.Lost <= bt.maxLost {
				alive = append(alive, t)
			}
		}
	}
	bt.tracks = alive

	return result
}

type matchPair struct {
	trackIdx int
	detIdx   int
	iou      float64
}

func (bt *ByteTracker) greedyMatch(tracks []*Track, dets []YOLODetection) []matchPair {
	if len(tracks) == 0 || len(dets) == 0 {
		return nil
	}

	// Compute all IoU pairs
	var pairs []matchPair
	for ti, t := range tracks {
		tbb := t.BBox()
		for di, d := range dets {
			iou := computeIoU(tbb, [4]float64{float64(d.X), float64(d.Y), float64(d.W), float64(d.H)})
			if iou >= float64(bt.iouThresh) {
				pairs = append(pairs, matchPair{trackIdx: ti, detIdx: di, iou: iou})
			}
		}
	}

	// Sort by IoU descending (greedy)
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].iou > pairs[j].iou
	})

	// Greedily assign
	usedTracks := make(map[int]bool)
	usedDets := make(map[int]bool)
	var result []matchPair
	for _, p := range pairs {
		if usedTracks[p.trackIdx] || usedDets[p.detIdx] {
			continue
		}
		usedTracks[p.trackIdx] = true
		usedDets[p.detIdx] = true
		result = append(result, p)
	}
	return result
}

func (bt *ByteTracker) updateTrack(t *Track, det YOLODetection) {
	cx := float64(det.X) + float64(det.W)/2
	cy := float64(det.Y) + float64(det.H)/2
	area := float64(det.W) * float64(det.H)
	aspect := float64(det.W) / float64(det.H)
	if det.H == 0 {
		aspect = 1
	}
	t.Kalman.Update(cx, cy, area, aspect)
	t.Class = det.ClassName
	t.Confidence = det.Confidence
}

func (bt *ByteTracker) createTrack(det YOLODetection) *Track {
	bt.nextID++
	t := &Track{
		ID:         bt.nextID,
		Class:      det.ClassName,
		Confidence: det.Confidence,
		Kalman:     NewKalmanState(float64(det.X), float64(det.Y), float64(det.W), float64(det.H)),
		ZoneStates: make(map[int64]ZoneState),
	}
	bt.tracks = append(bt.tracks, t)
	return t
}

// computeIoU calculates intersection-over-union for two boxes [x, y, w, h].
func computeIoU(a, b [4]float64) float64 {
	ax1, ay1, ax2, ay2 := a[0], a[1], a[0]+a[2], a[1]+a[3]
	bx1, by1, bx2, by2 := b[0], b[1], b[0]+b[2], b[1]+b[3]

	ix1 := max64(ax1, bx1)
	iy1 := max64(ay1, by1)
	ix2 := min64(ax2, bx2)
	iy2 := min64(ay2, by2)

	iw := max64(0, ix2-ix1)
	ih := max64(0, iy2-iy1)
	inter := iw * ih

	areaA := a[2] * a[3]
	areaB := b[2] * b[3]
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestNewTrack|TestSameObject|TestTwoObjects|TestLostTrack|TestLowConfidence" -v`
Expected: All 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/ai/tracker.go internal/nvr/ai/tracker_test.go
git commit -m "feat(ai): add ByteTrack multi-object tracker with greedy matching"
```

---

### Task 3: Zone Model + Point-in-Polygon

**Files:**

- Create: `internal/nvr/ai/zone.go`
- Create: `internal/nvr/ai/zone_test.go`

- [ ] **Step 1: Write zone_test.go**

```go
// internal/nvr/ai/zone_test.go
package ai

import "testing"

func TestPointInPolygonSquare(t *testing.T) {
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	tests := []struct {
		x, y float64
		want bool
	}{
		{0.5, 0.5, true},   // center
		{0.0, 0.0, false},  // vertex — edge case, ray casting excludes
		{-0.1, 0.5, false}, // outside left
		{1.1, 0.5, false},  // outside right
		{0.5, -0.1, false}, // outside top
		{0.5, 1.1, false},  // outside bottom
	}
	for _, tt := range tests {
		got := PointInPolygon(tt.x, tt.y, square)
		if got != tt.want {
			t.Errorf("PointInPolygon(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestPointInPolygonTriangle(t *testing.T) {
	tri := [][2]float64{{0.5, 0.0}, {1.0, 1.0}, {0.0, 1.0}}
	if !PointInPolygon(0.5, 0.5, tri) {
		t.Error("center of triangle should be inside")
	}
	if PointInPolygon(0.1, 0.1, tri) {
		t.Error("top-left corner should be outside triangle")
	}
}

func TestPointInPolygonConcave(t *testing.T) {
	// L-shaped polygon
	lshape := [][2]float64{{0, 0}, {0.5, 0}, {0.5, 0.5}, {1, 0.5}, {1, 1}, {0, 1}}
	if !PointInPolygon(0.25, 0.5, lshape) {
		t.Error("should be inside L-shape")
	}
	if PointInPolygon(0.75, 0.25, lshape) {
		t.Error("should be outside L-shape concavity")
	}
}

func TestImplicitFullFrameZone(t *testing.T) {
	z := ImplicitFullFrameZone("cam1")
	if z.ID != -1 {
		t.Errorf("implicit zone ID should be -1, got %d", z.ID)
	}
	if !PointInPolygon(0.5, 0.5, z.Polygon) {
		t.Error("center should be inside full-frame zone")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run TestPointInPolygon -v`
Expected: FAIL — `PointInPolygon` not defined

- [ ] **Step 3: Implement zone.go**

```go
// internal/nvr/ai/zone.go
package ai

// Zone represents a detection zone polygon for a camera.
type Zone struct {
	ID       int64
	CameraID string
	Name     string
	Polygon  [][2]float64 // normalized coordinates 0-1
	Enabled  bool
	Rules    []ZoneAlertRule
}

// ZoneAlertRule configures notifications for a specific class in a zone.
type ZoneAlertRule struct {
	ID              int64
	ZoneID          int64
	ClassName       string // "person", "car", "*"
	Enabled         bool
	CooldownSeconds int
	LoiterSeconds   int
	NotifyOnEnter   bool
	NotifyOnLeave   bool
	NotifyOnLoiter  bool
}

// DefaultRulesForZone returns the default alert rules for a new zone.
// Includes a wildcard "*" rule to catch any importantClass not explicitly listed.
func DefaultRulesForZone(zoneID int64) []ZoneAlertRule {
	return []ZoneAlertRule{
		{ZoneID: zoneID, ClassName: "person", Enabled: true, CooldownSeconds: 30, NotifyOnEnter: true},
		{ZoneID: zoneID, ClassName: "car", Enabled: true, CooldownSeconds: 60, NotifyOnEnter: true},
		{ZoneID: zoneID, ClassName: "truck", Enabled: true, CooldownSeconds: 60, NotifyOnEnter: true},
		{ZoneID: zoneID, ClassName: "cat", Enabled: true, CooldownSeconds: 60, NotifyOnEnter: true},
		{ZoneID: zoneID, ClassName: "dog", Enabled: true, CooldownSeconds: 60, NotifyOnEnter: true},
		{ZoneID: zoneID, ClassName: "*", Enabled: true, CooldownSeconds: 60, NotifyOnEnter: true}, // wildcard for all other important classes
	}
}

// ImplicitFullFrameZone returns a synthetic full-frame zone for cameras with no explicit zones.
func ImplicitFullFrameZone(cameraID string) Zone {
	z := Zone{
		ID:       -1,
		CameraID: cameraID,
		Name:     "Full Frame",
		Polygon:  [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		Enabled:  true,
	}
	z.Rules = DefaultRulesForZone(-1)
	return z
}

// RuleForClass finds the alert rule matching a class name.
// Falls back to "*" wildcard if no exact match.
func (z *Zone) RuleForClass(className string) *ZoneAlertRule {
	var wildcard *ZoneAlertRule
	for i := range z.Rules {
		if z.Rules[i].ClassName == className {
			return &z.Rules[i]
		}
		if z.Rules[i].ClassName == "*" {
			wildcard = &z.Rules[i]
		}
	}
	return wildcard
}

// PointInPolygon tests if a point (px, py) is inside a polygon using ray-casting.
func PointInPolygon(px, py float64, polygon [][2]float64) bool {
	n := len(polygon)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := polygon[i][0], polygon[i][1]
		xj, yj := polygon[j][0], polygon[j][1]
		if ((yi > py) != (yj > py)) &&
			(px < (xj-xi)*(py-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}
	return inside
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestPointInPolygon|TestImplicit" -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/ai/zone.go internal/nvr/ai/zone_test.go
git commit -m "feat(ai): add Zone model with point-in-polygon ray casting"
```

---

### Task 4: State Machine

**Files:**

- Create: `internal/nvr/ai/state.go`
- Create: `internal/nvr/ai/state_test.go`

- [ ] **Step 1: Write state_test.go**

```go
// internal/nvr/ai/state_test.go
package ai

import (
	"testing"
	"time"
)

func TestEnterTransition(t *testing.T) {
	zone := Zone{ID: 1, Name: "Driveway", Polygon: [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}
	zone.Rules = []ZoneAlertRule{{ZoneID: 1, ClassName: "person", Enabled: true, NotifyOnEnter: true}}
	track := &Track{ID: 1, Class: "person", Confidence: 0.9, ZoneStates: make(map[int64]ZoneState)}

	reqs := EvaluateZoneTransitions(track, 0.5, 0.5, []Zone{zone}, time.Now())
	if len(reqs) != 1 {
		t.Fatalf("expected 1 notification request, got %d", len(reqs))
	}
	if reqs[0].Action != "entered" {
		t.Errorf("expected action 'entered', got '%s'", reqs[0].Action)
	}
}

func TestNoTransitionWhenAlreadyInside(t *testing.T) {
	zone := Zone{ID: 1, Name: "Test", Polygon: [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}
	zone.Rules = []ZoneAlertRule{{ZoneID: 1, ClassName: "person", Enabled: true, NotifyOnEnter: true}}
	track := &Track{ID: 1, Class: "person", Confidence: 0.9, ZoneStates: map[int64]ZoneState{
		1: {State: StateInside, EnteredAt: time.Now().Add(-1 * time.Second)},
	}}

	reqs := EvaluateZoneTransitions(track, 0.5, 0.5, []Zone{zone}, time.Now())
	if len(reqs) != 0 {
		t.Errorf("expected 0 requests when already inside, got %d", len(reqs))
	}
}

func TestLeaveTransition(t *testing.T) {
	zone := Zone{ID: 1, Name: "Test", Polygon: [][2]float64{{0, 0}, {0.4, 0}, {0.4, 0.4}, {0, 0.4}}}
	zone.Rules = []ZoneAlertRule{{ZoneID: 1, ClassName: "person", Enabled: true, NotifyOnEnter: true, NotifyOnLeave: true}}
	track := &Track{ID: 1, Class: "person", Confidence: 0.9, ZoneStates: map[int64]ZoneState{
		1: {State: StateInside, EnteredAt: time.Now().Add(-5 * time.Second)},
	}}

	// Point outside the zone
	reqs := EvaluateZoneTransitions(track, 0.8, 0.8, []Zone{zone}, time.Now())
	if len(reqs) != 1 {
		t.Fatalf("expected 1 leave request, got %d", len(reqs))
	}
	if reqs[0].Action != "left" {
		t.Errorf("expected 'left', got '%s'", reqs[0].Action)
	}
}

func TestLoiterTransition(t *testing.T) {
	zone := Zone{ID: 1, Name: "Test", Polygon: [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}
	zone.Rules = []ZoneAlertRule{{ZoneID: 1, ClassName: "person", Enabled: true, NotifyOnLoiter: true, LoiterSeconds: 10}}
	enteredAt := time.Now().Add(-15 * time.Second)
	track := &Track{ID: 1, Class: "person", Confidence: 0.9, ZoneStates: map[int64]ZoneState{
		1: {State: StateInside, EnteredAt: enteredAt},
	}}

	reqs := EvaluateZoneTransitions(track, 0.5, 0.5, []Zone{zone}, time.Now())
	if len(reqs) != 1 {
		t.Fatalf("expected 1 loiter request, got %d", len(reqs))
	}
	if reqs[0].Action != "loitering" {
		t.Errorf("expected 'loitering', got '%s'", reqs[0].Action)
	}

	// Should not fire again (LoiterNotified = true now)
	reqs2 := EvaluateZoneTransitions(track, 0.5, 0.5, []Zone{zone}, time.Now())
	if len(reqs2) != 0 {
		t.Errorf("loiter should not re-fire, got %d requests", len(reqs2))
	}
}

func TestLoiterNotifiedResetsOnLeave(t *testing.T) {
	zone := Zone{ID: 1, Name: "Test", Polygon: [][2]float64{{0, 0}, {0.4, 0}, {0.4, 0.4}, {0, 0.4}}}
	zone.Rules = []ZoneAlertRule{{ZoneID: 1, ClassName: "person", Enabled: true, NotifyOnEnter: true, NotifyOnLeave: true, NotifyOnLoiter: true, LoiterSeconds: 5}}
	track := &Track{ID: 1, Class: "person", Confidence: 0.9, ZoneStates: map[int64]ZoneState{
		1: {State: StateLoitering, EnteredAt: time.Now().Add(-10 * time.Second), LoiterNotified: true},
	}}

	// Leave the zone
	EvaluateZoneTransitions(track, 0.8, 0.8, []Zone{zone}, time.Now())

	// State should be reset
	state := track.ZoneStates[1]
	if state.State != StateOutside {
		t.Errorf("expected StateOutside after leaving, got %d", state.State)
	}
	if state.LoiterNotified {
		t.Error("LoiterNotified should be reset after leaving")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestEnter|TestNoTrans|TestLeave|TestLoiter" -v`
Expected: FAIL

- [ ] **Step 3: Implement state.go**

```go
// internal/nvr/ai/state.go
package ai

import "time"

// ObjectState represents the state of a tracked object relative to a zone.
type ObjectState int

const (
	StateOutside   ObjectState = iota // not inside this zone
	StateInside                       // inside zone, monitoring for loiter
	StateLoitering                    // inside zone past loiter threshold
)

// ZoneState holds the per-track, per-zone state machine data.
type ZoneState struct {
	State          ObjectState
	EnteredAt      time.Time
	LoiterNotified bool
}

// NotificationRequest is an internal event emitted by a state transition,
// before cooldown filtering.
type NotificationRequest struct {
	TrackID    int
	ZoneID     int64
	ZoneName   string
	Class      string
	Action     string // "entered", "loitering", "left"
	Confidence float32
	Timestamp  time.Time
}

// EvaluateZoneTransitions runs the state machine for a single track against all zones.
// Returns notification requests for any state transitions that occurred.
func EvaluateZoneTransitions(track *Track, cx, cy float64, zones []Zone, now time.Time) []NotificationRequest {
	var reqs []NotificationRequest

	for _, zone := range zones {
		if !zone.Enabled {
			continue
		}

		inZone := PointInPolygon(cx, cy, zone.Polygon)
		state, exists := track.ZoneStates[zone.ID]
		if !exists {
			state = ZoneState{State: StateOutside}
		}

		switch {
		case state.State == StateOutside && inZone:
			// Entered zone
			state.State = StateInside
			state.EnteredAt = now
			state.LoiterNotified = false
			reqs = append(reqs, NotificationRequest{
				TrackID:    track.ID,
				ZoneID:     zone.ID,
				ZoneName:   zone.Name,
				Class:      track.Class,
				Action:     "entered",
				Confidence: track.Confidence,
				Timestamp:  now,
			})

		case state.State == StateInside && inZone:
			// Check loiter
			rule := zone.RuleForClass(track.Class)
			if rule != nil && rule.LoiterSeconds > 0 && !state.LoiterNotified {
				if now.Sub(state.EnteredAt) > time.Duration(rule.LoiterSeconds)*time.Second {
					state.State = StateLoitering
					state.LoiterNotified = true
					reqs = append(reqs, NotificationRequest{
						TrackID:    track.ID,
						ZoneID:     zone.ID,
						ZoneName:   zone.Name,
						Class:      track.Class,
						Action:     "loitering",
						Confidence: track.Confidence,
						Timestamp:  now,
					})
				}
			}

		case (state.State == StateInside || state.State == StateLoitering) && !inZone:
			// Left zone
			reqs = append(reqs, NotificationRequest{
				TrackID:    track.ID,
				ZoneID:     zone.ID,
				ZoneName:   zone.Name,
				Class:      track.Class,
				Action:     "left",
				Confidence: track.Confidence,
				Timestamp:  now,
			})
			state.State = StateOutside
			state.EnteredAt = time.Time{}
			state.LoiterNotified = false
		}

		track.ZoneStates[zone.ID] = state
	}

	return reqs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestEnter|TestNoTrans|TestLeave|TestLoiter" -v`
Expected: All 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/ai/state.go internal/nvr/ai/state_test.go
git commit -m "feat(ai): add per-track per-zone state machine with enter/loiter/leave transitions"
```

---

### Task 5: Cooldown Manager

**Files:**

- Create: `internal/nvr/ai/cooldown.go`
- Create: `internal/nvr/ai/cooldown_test.go`

- [ ] **Step 1: Write cooldown_test.go**

```go
// internal/nvr/ai/cooldown_test.go
package ai

import (
	"testing"
	"time"
)

func TestCooldownAllowsFirstNotification(t *testing.T) {
	cm := NewCooldownManager()
	req := NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: time.Now()}
	rule := ZoneAlertRule{CooldownSeconds: 30, NotifyOnEnter: true}
	if !cm.ShouldNotify(req, rule) {
		t.Error("first notification should be allowed")
	}
}

func TestCooldownSuppressesDuplicate(t *testing.T) {
	cm := NewCooldownManager()
	now := time.Now()
	req := NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: now}
	rule := ZoneAlertRule{CooldownSeconds: 30, NotifyOnEnter: true}

	cm.ShouldNotify(req, rule) // first — allowed

	req2 := NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: now.Add(5 * time.Second)}
	if cm.ShouldNotify(req2, rule) {
		t.Error("notification within cooldown should be suppressed")
	}
}

func TestCooldownAllowsAfterExpiry(t *testing.T) {
	cm := NewCooldownManager()
	now := time.Now()
	req := NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: now}
	rule := ZoneAlertRule{CooldownSeconds: 30, NotifyOnEnter: true}

	cm.ShouldNotify(req, rule)

	req2 := NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: now.Add(31 * time.Second)}
	if !cm.ShouldNotify(req2, rule) {
		t.Error("notification after cooldown should be allowed")
	}
}

func TestCooldownPerClassIndependence(t *testing.T) {
	cm := NewCooldownManager()
	now := time.Now()
	rule := ZoneAlertRule{CooldownSeconds: 30, NotifyOnEnter: true}

	// Person enters
	cm.ShouldNotify(NotificationRequest{ZoneID: 1, Class: "person", Action: "entered", Timestamp: now}, rule)

	// Car enters same zone — should be allowed (different class)
	carReq := NotificationRequest{ZoneID: 1, Class: "car", Action: "entered", Timestamp: now.Add(1 * time.Second)}
	if !cm.ShouldNotify(carReq, rule) {
		t.Error("different class should not be affected by cooldown")
	}
}

func TestCooldownRespectsDisabledAction(t *testing.T) {
	cm := NewCooldownManager()
	req := NotificationRequest{ZoneID: 1, Class: "person", Action: "left", Timestamp: time.Now()}
	rule := ZoneAlertRule{NotifyOnLeave: false}
	if cm.ShouldNotify(req, rule) {
		t.Error("disabled action should be suppressed")
	}
}

func TestCooldownLoiteringAlwaysAllowed(t *testing.T) {
	cm := NewCooldownManager()
	req := NotificationRequest{ZoneID: 1, Class: "person", Action: "loitering", Timestamp: time.Now()}
	rule := ZoneAlertRule{NotifyOnLoiter: true, CooldownSeconds: 30}
	if !cm.ShouldNotify(req, rule) {
		t.Error("loitering should always pass cooldown (time-gated by loiter threshold)")
	}
}

func TestCooldownGarbageCollection(t *testing.T) {
	cm := NewCooldownManager()
	old := time.Now().Add(-15 * time.Minute)
	cm.lastNotified[cooldownKey{1, "person", "entered"}] = old
	cm.lastNotified[cooldownKey{1, "car", "entered"}] = time.Now()

	cm.GC(10 * time.Minute)

	if len(cm.lastNotified) != 1 {
		t.Errorf("expected 1 entry after GC, got %d", len(cm.lastNotified))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestCooldown" -v`
Expected: FAIL

- [ ] **Step 3: Implement cooldown.go**

```go
// internal/nvr/ai/cooldown.go
package ai

import "time"

type cooldownKey struct {
	ZoneID    int64
	ClassName string
	Action    string
}

// CooldownManager tracks per-zone per-class notification cooldowns.
// Each AIPipeline owns one instance; no mutex needed (single-goroutine access).
type CooldownManager struct {
	lastNotified map[cooldownKey]time.Time
}

// NewCooldownManager creates an empty cooldown manager.
func NewCooldownManager() *CooldownManager {
	return &CooldownManager{
		lastNotified: make(map[cooldownKey]time.Time),
	}
}

// ShouldNotify returns true if the notification should be sent (not suppressed).
func (cm *CooldownManager) ShouldNotify(req NotificationRequest, rule ZoneAlertRule) bool {
	// Check if this action type is enabled
	switch req.Action {
	case "entered":
		if !rule.NotifyOnEnter {
			return false
		}
	case "left":
		if !rule.NotifyOnLeave {
			return false
		}
	case "loitering":
		if !rule.NotifyOnLoiter {
			return false
		}
		// Loitering is already time-gated by the loiter threshold; always allow
		return true
	}

	key := cooldownKey{req.ZoneID, req.Class, req.Action}
	last, exists := cm.lastNotified[key]
	cooldown := time.Duration(rule.CooldownSeconds) * time.Second
	if exists && req.Timestamp.Sub(last) < cooldown {
		return false
	}
	cm.lastNotified[key] = req.Timestamp
	return true
}

// GC removes cooldown entries older than maxAge.
func (cm *CooldownManager) GC(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	for k, v := range cm.lastNotified {
		if v.Before(cutoff) {
			delete(cm.lastNotified, k)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run "TestCooldown" -v`
Expected: All 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/ai/cooldown.go internal/nvr/ai/cooldown_test.go
git commit -m "feat(ai): add per-zone per-class cooldown manager"
```

---

### Task 6: DB Migration v15 + Zone CRUD

**Files:**

- Modify: `internal/nvr/db/migrations.go`
- Create: `internal/nvr/db/zones.go`
- Modify: `internal/nvr/db/detections.go`

- [ ] **Step 1: Add v15 migration to migrations.go**

Append a new entry to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
{
    version: 15,
    sql: `
CREATE TABLE IF NOT EXISTS detection_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    polygon TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_zones_camera ON detection_zones(camera_id);

CREATE TABLE IF NOT EXISTS zone_alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL,
    class_name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    cooldown_seconds INTEGER NOT NULL DEFAULT 30,
    loiter_seconds INTEGER NOT NULL DEFAULT 0,
    notify_on_enter INTEGER NOT NULL DEFAULT 1,
    notify_on_leave INTEGER NOT NULL DEFAULT 0,
    notify_on_loiter INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (zone_id) REFERENCES detection_zones(id) ON DELETE CASCADE,
    UNIQUE(zone_id, class_name)
);

ALTER TABLE detections ADD COLUMN track_id INTEGER DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_detections_track ON detections(track_id);
    `,
},
```

- [ ] **Step 2: Create zones.go with CRUD methods**

```go
// internal/nvr/db/zones.go
package db

import (
	"encoding/json"
	"fmt"
)

// DetectionZone represents a detection zone stored in the database.
type DetectionZone struct {
	ID        int64      `json:"id"`
	CameraID  string     `json:"camera_id"`
	Name      string     `json:"name"`
	Polygon   [][2]float64 `json:"polygon"`
	Enabled   bool       `json:"enabled"`
	CreatedAt string     `json:"created_at,omitempty"`
	Rules     []AlertRule `json:"rules,omitempty"`
}

// AlertRule represents a zone alert rule stored in the database.
type AlertRule struct {
	ID              int64  `json:"id"`
	ZoneID          int64  `json:"zone_id"`
	ClassName       string `json:"class_name"`
	Enabled         bool   `json:"enabled"`
	CooldownSeconds int    `json:"cooldown_seconds"`
	LoiterSeconds   int    `json:"loiter_seconds"`
	NotifyOnEnter   bool   `json:"notify_on_enter"`
	NotifyOnLeave   bool   `json:"notify_on_leave"`
	NotifyOnLoiter  bool   `json:"notify_on_loiter"`
}

// ListZonesByCamera returns all zones and their rules for a camera.
func (d *DB) ListZonesByCamera(cameraID string) ([]DetectionZone, error) {
	rows, err := d.Query(`SELECT id, camera_id, name, polygon, enabled, created_at FROM detection_zones WHERE camera_id = ?`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []DetectionZone
	for rows.Next() {
		var z DetectionZone
		var polyJSON string
		var enabled int
		if err := rows.Scan(&z.ID, &z.CameraID, &z.Name, &polyJSON, &enabled, &z.CreatedAt); err != nil {
			return nil, err
		}
		z.Enabled = enabled != 0
		if err := json.Unmarshal([]byte(polyJSON), &z.Polygon); err != nil {
			return nil, fmt.Errorf("parse polygon for zone %d: %w", z.ID, err)
		}
		// Load rules
		rules, err := d.listRulesForZone(z.ID)
		if err != nil {
			return nil, err
		}
		z.Rules = rules
		zones = append(zones, z)
	}
	return zones, nil
}

func (d *DB) listRulesForZone(zoneID int64) ([]AlertRule, error) {
	rows, err := d.Query(`SELECT id, zone_id, class_name, enabled, cooldown_seconds, loiter_seconds,
		notify_on_enter, notify_on_leave, notify_on_loiter FROM zone_alert_rules WHERE zone_id = ?`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		var r AlertRule
		var enabled, enter, leave, loiter int
		if err := rows.Scan(&r.ID, &r.ZoneID, &r.ClassName, &enabled, &r.CooldownSeconds, &r.LoiterSeconds, &enter, &leave, &loiter); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.NotifyOnEnter = enter != 0
		r.NotifyOnLeave = leave != 0
		r.NotifyOnLoiter = loiter != 0
		rules = append(rules, r)
	}
	return rules, nil
}

// InsertZone creates a zone and returns it with the generated ID.
func (d *DB) InsertZone(z *DetectionZone) error {
	polyJSON, err := json.Marshal(z.Polygon)
	if err != nil {
		return err
	}
	enabled := 0
	if z.Enabled {
		enabled = 1
	}
	res, err := d.Exec(`INSERT INTO detection_zones (camera_id, name, polygon, enabled) VALUES (?, ?, ?, ?)`,
		z.CameraID, z.Name, string(polyJSON), enabled)
	if err != nil {
		return err
	}
	z.ID, _ = res.LastInsertId()
	return nil
}

// UpdateZone updates a zone's name, polygon, and enabled state.
func (d *DB) UpdateZone(z *DetectionZone) error {
	polyJSON, err := json.Marshal(z.Polygon)
	if err != nil {
		return err
	}
	enabled := 0
	if z.Enabled {
		enabled = 1
	}
	_, err = d.Exec(`UPDATE detection_zones SET name = ?, polygon = ?, enabled = ? WHERE id = ?`,
		z.Name, string(polyJSON), enabled, z.ID)
	return err
}

// DeleteZone deletes a zone and its rules (CASCADE).
func (d *DB) DeleteZone(zoneID int64) error {
	_, err := d.Exec(`DELETE FROM detection_zones WHERE id = ?`, zoneID)
	return err
}

// UpsertAlertRule creates or updates an alert rule for a zone.
func (d *DB) UpsertAlertRule(r *AlertRule) error {
	enabled, enter, leave, loiter := boolToInt(r.Enabled), boolToInt(r.NotifyOnEnter), boolToInt(r.NotifyOnLeave), boolToInt(r.NotifyOnLoiter)
	_, err := d.Exec(`INSERT INTO zone_alert_rules (zone_id, class_name, enabled, cooldown_seconds, loiter_seconds, notify_on_enter, notify_on_leave, notify_on_loiter)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(zone_id, class_name) DO UPDATE SET
			enabled = excluded.enabled,
			cooldown_seconds = excluded.cooldown_seconds,
			loiter_seconds = excluded.loiter_seconds,
			notify_on_enter = excluded.notify_on_enter,
			notify_on_leave = excluded.notify_on_leave,
			notify_on_loiter = excluded.notify_on_loiter`,
		r.ZoneID, r.ClassName, enabled, r.CooldownSeconds, r.LoiterSeconds, enter, leave, loiter)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 3: Add track_id to Detection struct and InsertDetection**

In `internal/nvr/db/detections.go`:

1. Add `TrackID int `json:"track_id"``field to the`Detection` struct
2. Update the `InsertDetection` SQL to include `track_id` in the INSERT column list and VALUES
3. Update `GetRecentDetections` to include `track_id` in its SELECT and Scan (this feeds the live overlay which needs track IDs)
4. Other read queries (`ListDetectionsByEvent`, `ListDetectionsWithEmbeddings`) do NOT need changes — `track_id` has DEFAULT 0 and those queries don't need the field

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/zones.go internal/nvr/db/detections.go
git commit -m "feat(db): add v15 migration for detection zones, alert rules, and track_id"
```

---

### Task 7: Event Model Updates

**Files:**

- Modify: `internal/nvr/api/events.go`

- [ ] **Step 1: Add structured fields to Event struct and PublishTrackedDetection**

In `internal/nvr/api/events.go`:

1. Add new fields to `Event` struct:

```go
type Event struct {
	Type       string  `json:"type"`
	Camera     string  `json:"camera"`
	Message    string  `json:"message"`
	Time       string  `json:"time"`
	Zone       string  `json:"zone,omitempty"`
	Class      string  `json:"class,omitempty"`
	Action     string  `json:"action,omitempty"`
	TrackID    int     `json:"track_id,omitempty"`
	Confidence float32 `json:"confidence,omitempty"`
}
```

2. Add `PublishTrackedDetection` method:

```go
func (b *EventBroadcaster) PublishTrackedDetection(camera, zone, class, action string, trackID int, confidence float32) {
	label := strings.ToUpper(class[:1]) + class[1:]
	var msg string
	switch action {
	case "entered":
		msg = fmt.Sprintf("%s entered %s (%.0f%%)", label, zone, confidence*100)
	case "loitering":
		msg = fmt.Sprintf("%s loitering in %s", label, zone)
	case "left":
		msg = fmt.Sprintf("%s left %s", label, zone)
	default:
		msg = fmt.Sprintf("%s %s %s", label, action, zone)
	}
	b.Publish(Event{
		Type: "ai_detection", Camera: camera, Message: msg, Zone: zone,
		Class: class, Action: action, TrackID: trackID, Confidence: confidence,
	})
}
```

3. Add `"strings"` to the imports.

- [ ] **Step 2: Add PublishTrackedDetection to EventPublisher interface (keep old method too)**

In `internal/nvr/ai/pipeline.go`, **add** the new method to the interface while keeping the old one. This keeps the build working until Task 9 replaces all call sites:

```go
type EventPublisher interface {
	PublishAIDetection(cameraName, className string, confidence float32)
	PublishTrackedDetection(camera, zone, class, action string, trackID int, confidence float32)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Clean build (both old and new methods exist)

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/events.go internal/nvr/ai/pipeline.go
git commit -m "feat(api): add structured Event fields and PublishTrackedDetection method"
```

---

### Task 8: Zone API Endpoints

**Files:**

- Create: `internal/nvr/api/zones.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create zones.go with REST handlers**

```go
// internal/nvr/api/zones.go
package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/gin-gonic/gin"
)

// ZoneHandler handles zone CRUD endpoints.
type ZoneHandler struct {
	DB *db.DB
}

// ListZones returns all zones for a camera.
func (h *ZoneHandler) ListZones(c *gin.Context) {
	cameraID := c.Param("id")
	zones, err := h.DB.ListZonesByCamera(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if zones == nil {
		zones = []db.DetectionZone{}
	}
	c.JSON(http.StatusOK, zones)
}

// CreateZone creates a new detection zone.
func (h *ZoneHandler) CreateZone(c *gin.Context) {
	cameraID := c.Param("id")
	var req struct {
		Name    string          `json:"name" binding:"required"`
		Polygon [][2]float64    `json:"polygon" binding:"required"`
		Enabled *bool           `json:"enabled"`
		Rules   []db.AlertRule  `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Polygon) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "polygon must have at least 3 points"})
		return
	}

	zone := &db.DetectionZone{
		CameraID: cameraID,
		Name:     req.Name,
		Polygon:  req.Polygon,
		Enabled:  req.Enabled == nil || *req.Enabled,
	}
	if err := h.DB.InsertZone(zone); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Insert rules (use defaults if none provided)
	rules := req.Rules
	if len(rules) == 0 {
		rules = defaultAlertRules(zone.ID)
	}
	for _, r := range rules {
		r.ZoneID = zone.ID
		if err := h.DB.UpsertAlertRule(&r); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Reload to get full zone with rules
	zones, _ := h.DB.ListZonesByCamera(cameraID)
	for _, z := range zones {
		if z.ID == zone.ID {
			c.JSON(http.StatusCreated, z)
			return
		}
	}
	c.JSON(http.StatusCreated, zone)
}

// UpdateZone updates a zone.
func (h *ZoneHandler) UpdateZone(c *gin.Context) {
	zoneID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid zone ID"})
		return
	}
	var req struct {
		Name    string         `json:"name"`
		Polygon [][2]float64   `json:"polygon"`
		Enabled *bool          `json:"enabled"`
		Rules   []db.AlertRule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	zone := &db.DetectionZone{ID: zoneID, Name: req.Name, Polygon: req.Polygon, Enabled: req.Enabled == nil || *req.Enabled}
	if err := h.DB.UpdateZone(zone); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, r := range req.Rules {
		r.ZoneID = zoneID
		if err := h.DB.UpsertAlertRule(&r); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DeleteZone deletes a zone and its rules.
func (h *ZoneHandler) DeleteZone(c *gin.Context) {
	zoneID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid zone ID"})
		return
	}
	if err := h.DB.DeleteZone(zoneID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// CameraSnapshot proxies the camera's ONVIF snapshot with digest auth.
// Uses the shared FetchSnapshot utility (extracted from pipeline.go's captureAndDecode).
func (h *ZoneHandler) CameraSnapshot(c *gin.Context) {
	cameraID := c.Param("id")
	cam, err := h.DB.GetCamera(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if cam.SnapshotURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no snapshot URI"})
		return
	}
	password := h.DecryptPassword(cam.ONVIFPassword)
	data, contentType, err := ai.FetchSnapshot(cam.SnapshotURI, cam.ONVIFUsername, password)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch snapshot: " + err.Error()})
		return
	}
	c.Data(http.StatusOK, contentType, data)
}

func defaultAlertRules(zoneID int64) []db.AlertRule {
	classes := []struct {
		name     string
		cooldown int
	}{
		{"person", 30}, {"car", 60}, {"truck", 60},
		{"cat", 60}, {"dog", 60}, {"*", 60},
	}
	rules := make([]db.AlertRule, len(classes))
	for i, cls := range classes {
		rules[i] = db.AlertRule{
			ZoneID: zoneID, ClassName: cls.name, Enabled: true,
			CooldownSeconds: cls.cooldown, NotifyOnEnter: true,
		}
	}
	return rules
}
```

**Important:** Before creating `zones.go`, first extract the snapshot fetching logic from `pipeline.go`'s `captureAndDecode` into a shared utility `internal/nvr/ai/snapshot.go`:

```go
// internal/nvr/ai/snapshot.go
package ai

// FetchSnapshot fetches a JPEG from the given URL with digest/basic auth.
// Returns the raw bytes, content type, and any error.
// This is extracted from pipeline.go's captureAndDecode so both the pipeline
// and the API snapshot proxy can reuse it.
func FetchSnapshot(snapshotURL, username, password string) ([]byte, string, error) {
	// ... extract the HTTP fetch + digest auth logic from captureAndDecode
	// Return (body bytes, content-type header, error)
}
```

The implementer should move the digest auth logic (URL-embedded creds, Basic auth, Digest auth challenge/response) from `pipeline.go`'s `captureAndDecode` into `FetchSnapshot`, then have both `captureAndDecode` and `CameraSnapshot` call it.

The `ZoneHandler` struct also needs access to the encryption key for decrypting camera passwords:

```go
type ZoneHandler struct {
	DB            *db.DB
	EncryptionKey []byte
}

func (h *ZoneHandler) DecryptPassword(encrypted string) string {
	// Same decryption logic as NVR.decryptPassword
}
```

- [ ] **Step 2: Register zone routes in router.go**

In `internal/nvr/api/router.go`, after the existing `protected` route registrations, add:

```go
zoneHandler := &ZoneHandler{DB: cfg.DB, EncryptionKey: cfg.EncryptionKey}
protected.GET("/cameras/:id/zones", zoneHandler.ListZones)
protected.POST("/cameras/:id/zones", zoneHandler.CreateZone)
protected.PUT("/zones/:id", zoneHandler.UpdateZone)
protected.DELETE("/zones/:id", zoneHandler.DeleteZone)
protected.GET("/cameras/:id/snapshot", zoneHandler.CameraSnapshot)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: May have compile errors in pipeline.go from the interface change in Task 7. Those are fixed in Task 9.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/zones.go internal/nvr/api/router.go
git commit -m "feat(api): add zone CRUD endpoints and snapshot proxy"
```

---

### Task 9: Pipeline Integration

**Files:**

- Modify: `internal/nvr/ai/pipeline.go`

This is the core integration task. Replace the `prevClassCounts`/`ensureMotionEvent` notification logic with tracker → zones → state machine → cooldowns.

- [ ] **Step 1: Update AIPipeline struct**

Replace the notification-related fields:

```go
type AIPipeline struct {
	cameraID   string
	cameraName string
	detector   *Detector
	embedder   *Embedder
	db         *db.DB
	eventPub   EventPublisher
	stopCh     chan struct{}

	confThreshold float32
	motionGap     time.Duration

	lastDetectionTime          time.Time
	lastImportantDetectionTime time.Time
	currentEventID             int64

	// New: tracking + zones + cooldowns
	tracker   *ByteTracker
	cooldowns *CooldownManager
	zones     []Zone
	zonesLastLoaded time.Time
}
```

Remove `prevClassCounts` field entirely.

- [ ] **Step 2: Update NewAIPipeline constructor**

```go
func NewAIPipeline(cameraID, cameraName string, detector *Detector, embedder *Embedder, database *db.DB, eventPub EventPublisher) *AIPipeline {
	return &AIPipeline{
		cameraID:      cameraID,
		cameraName:    cameraName,
		detector:      detector,
		embedder:      embedder,
		db:            database,
		eventPub:      eventPub,
		stopCh:        make(chan struct{}),
		confThreshold: 0.3, // lowered from 0.5 for ByteTrack two-pass matching
		motionGap:     8 * time.Second,
		tracker:       NewByteTracker(),
		cooldowns:     NewCooldownManager(),
	}
}
```

- [ ] **Step 3: Add zone loading method**

```go
func (p *AIPipeline) loadZones() {
	if time.Since(p.zonesLastLoaded) < 5*time.Second {
		return
	}
	dbZones, err := p.db.ListZonesByCamera(p.cameraID)
	if err != nil {
		log.Printf("AI [%s]: failed to load zones: %v", p.cameraName, err)
		return
	}
	p.zonesLastLoaded = time.Now()

	if len(dbZones) == 0 {
		p.zones = []Zone{ImplicitFullFrameZone(p.cameraID)}
		return
	}

	zones := make([]Zone, len(dbZones))
	for i, dz := range dbZones {
		zones[i] = Zone{
			ID: dz.ID, CameraID: dz.CameraID, Name: dz.Name,
			Polygon: dz.Polygon, Enabled: dz.Enabled,
		}
		for _, r := range dz.Rules {
			zones[i].Rules = append(zones[i].Rules, ZoneAlertRule{
				ID: r.ID, ZoneID: r.ZoneID, ClassName: r.ClassName,
				Enabled: r.Enabled, CooldownSeconds: r.CooldownSeconds,
				LoiterSeconds: r.LoiterSeconds, NotifyOnEnter: r.NotifyOnEnter,
				NotifyOnLeave: r.NotifyOnLeave, NotifyOnLoiter: r.NotifyOnLoiter,
			})
		}
	}
	p.zones = zones
}
```

- [ ] **Step 4: Rewrite ProcessFrame to use tracker + zones + state + cooldowns**

Replace the body of `ProcessFrame` with the new pipeline:

```go
func (p *AIPipeline) ProcessFrame(img image.Image, timestamp time.Time) error {
	select {
	case <-p.stopCh:
		return fmt.Errorf("pipeline stopped")
	default:
	}

	// 1. YOLO detection (threshold lowered to 0.3 for ByteTrack)
	detections, err := p.detector.Detect(img, p.confThreshold)
	if err != nil {
		return fmt.Errorf("detection: %w", err)
	}

	// 2. ByteTrack
	tracked := p.tracker.Update(detections)

	// Check if any important classes with high confidence are present
	hasImportant := false
	for _, td := range tracked {
		if importantClasses[td.ClassName] && td.Confidence >= 0.5 {
			hasImportant = true
			break
		}
	}

	// Motion event management (for recording/clips, decoupled from notifications).
	// Note: confThreshold is now 0.3 but we still only create motion events for
	// important detections above 0.5 (high-confidence). This preserves the old
	// behavior where low-confidence ghosts don't trigger recording events.
	if hasImportant {
		p.lastImportantDetectionTime = timestamp
		if p.currentEventID == 0 {
			best := bestImportantDetection(detections)
			event := &db.MotionEvent{
				CameraID: p.cameraID, StartedAt: timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
				EventType: "ai_detection", ObjectClass: best.ClassName,
				Confidence: float64(best.Confidence),
			}
			if err := p.db.InsertMotionEvent(event); err == nil {
				p.currentEventID = event.ID
			}
		}
	} else if p.currentEventID > 0 && !p.lastImportantDetectionTime.IsZero() &&
		time.Since(p.lastImportantDetectionTime) > p.motionGap {
		p.closeCurrentEvent(timestamp)
	}

	if len(tracked) > 0 {
		p.lastDetectionTime = timestamp
	}

	// 3. Load zones (cached, refreshed every 5s)
	p.loadZones()

	// 4. Zone assignment + state transitions + notification dispatch
	for _, td := range tracked {
		track := p.tracker.findTrack(td.TrackID)
		if track == nil {
			continue
		}
		cx := float64(td.X) + float64(td.W)/2
		cy := float64(td.Y) + float64(td.H)/2

		reqs := EvaluateZoneTransitions(track, cx, cy, p.zones, timestamp)

		// 5. Cooldown gate
		for _, req := range reqs {
			zone := p.findZone(req.ZoneID)
			if zone == nil {
				continue
			}
			rule := zone.RuleForClass(req.Class)
			if rule == nil || !rule.Enabled {
				continue
			}
			if p.cooldowns.ShouldNotify(req, *rule) && p.eventPub != nil {
				p.eventPub.PublishTrackedDetection(p.cameraName, req.ZoneName, req.Class, req.Action, req.TrackID, req.Confidence)
			}
		}
	}

	// 6. Store detections in DB
	if p.currentEventID > 0 {
		bounds := img.Bounds()
		imgW := float64(bounds.Dx())
		imgH := float64(bounds.Dy())

		for _, td := range tracked {
			// Crop + embed + store (existing logic, now with track_id)
			x := int(math.Round(float64(td.X) * imgW))
			y := int(math.Round(float64(td.Y) * imgH))
			w := int(math.Round(float64(td.W) * imgW))
			h := int(math.Round(float64(td.H) * imgH))
			if x < bounds.Min.X { x = bounds.Min.X }
			if y < bounds.Min.Y { y = bounds.Min.Y }
			if x+w > bounds.Max.X { w = bounds.Max.X - x }
			if y+h > bounds.Max.Y { h = bounds.Max.Y - y }
			if w < 8 || h < 8 { continue }

			crop := cropImage(img, image.Rect(x, y, x+w, y+h))
			var embeddingBytes []byte
			if p.embedder != nil {
				if emb, err := p.embedder.EncodeImage(crop); err == nil {
					embeddingBytes = float32SliceToBytes(emb)
				}
			}

			det := &db.Detection{
				MotionEventID: p.currentEventID,
				FrameTime:     timestamp.UTC().Format("2006-01-02T15:04:05.000Z"),
				Class:         td.ClassName,
				Confidence:    float64(td.Confidence),
				BoxX:          float64(td.X), BoxY: float64(td.Y),
				BoxW:          float64(td.W), BoxH: float64(td.H),
				Embedding:     embeddingBytes,
				TrackID:       td.TrackID,
			}
			_ = p.db.InsertDetection(det)
		}
	}

	return nil
}
```

- [ ] **Step 5: Add helper methods to ByteTracker and AIPipeline**

In `tracker.go`:

```go
func (bt *ByteTracker) findTrack(id int) *Track {
	for _, t := range bt.tracks {
		if t.ID == id {
			return t
		}
	}
	return nil
}
```

In `pipeline.go`:

```go
func (p *AIPipeline) findZone(id int64) *Zone {
	for i := range p.zones {
		if p.zones[i].ID == id {
			return &p.zones[i]
		}
	}
	return nil
}
```

- [ ] **Step 6: Remove old notification logic and clean up interface**

1. Delete the old `ensureMotionEvent` function entirely (notification dispatch is now in ProcessFrame). Keep `bestImportantDetection`, `closeCurrentEvent`, and `importantClasses` as they are still used for motion event management.
2. Remove `PublishAIDetection` from the `EventPublisher` interface (it was kept temporarily in Task 7 for build compatibility; all call sites now use `PublishTrackedDetection`).
3. Remove the `prevClassCounts` field from the struct if not already done.

- [ ] **Step 7: Add cooldown GC to Run loop**

In the `Run` method, add a ticker for cooldown garbage collection alongside the frame capture ticker:

```go
gcTicker := time.NewTicker(60 * time.Second)
defer gcTicker.Stop()

for {
	select {
	case <-p.stopCh:
		return
	case <-gcTicker.C:
		p.cooldowns.GC(10 * time.Minute)
	case <-frameTicker.C:
		// ... existing frame capture + ProcessFrame
	}
}
```

- [ ] **Step 8: Verify full build compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Clean build

- [ ] **Step 9: Commit**

```bash
git add internal/nvr/ai/pipeline.go internal/nvr/ai/tracker.go
git commit -m "feat(ai): integrate ByteTrack, zones, state machine, and cooldowns into pipeline"
```

---

### Task 10: Frontend — Updated Notifications

**Files:**

- Modify: `ui/src/hooks/useNotifications.ts`
- Modify: `ui/src/components/Toast.tsx`

- [ ] **Step 1: Update Notification interface and parsing**

In `ui/src/hooks/useNotifications.ts`:

1. Update the `Notification` interface to add structured fields:

```typescript
export interface Notification {
  id: string;
  type:
    | "motion"
    | "ai_detection"
    | "camera_offline"
    | "camera_online"
    | "recording_started"
    | "recording_stopped";
  camera: string;
  message: string;
  time: Date;
  read: boolean;
  zone?: string;
  className?: string;
  action?: string;
  trackId?: number;
  confidence?: number;
}
```

2. Update the WebSocket `onmessage` handler to parse the new fields:

```typescript
const notif: Notification = {
  id: crypto.randomUUID(),
  type: data.type,
  camera: data.camera,
  message: data.message,
  time: new Date(data.time),
  read: false,
  zone: data.zone,
  className: data.class,
  action: data.action,
  trackId: data.track_id,
  confidence: data.confidence,
};
```

3. Update `eventTypeToTitle` to handle structured AI events:

```typescript
function eventTypeToTitle(
  eventType: string,
  message: string,
  action?: string,
  className?: string,
): string {
  if (eventType === "ai_detection" && action && className) {
    const label = className.charAt(0).toUpperCase() + className.slice(1);
    switch (action) {
      case "entered":
        return `${label} Entered`;
      case "loitering":
        return `${label} Loitering`;
      case "left":
        return `${label} Left`;
    }
  }
  // ... existing switch for other event types
}
```

4. Update `eventTypeToToastType` for action-based severity:

```typescript
function eventTypeToToastType(
  eventType: string,
  action?: string,
): ToastMessage["type"] {
  if (eventType === "ai_detection") {
    switch (action) {
      case "loitering":
        return "error";
      case "left":
        return "info";
      default:
        return "warning";
    }
  }
  // ... existing switch
}
```

5. Update the `pushToast` call in `addNotification` to pass the new parameters:

```typescript
pushToast({
  id: notif.id,
  type: eventTypeToToastType(notif.type, notif.action),
  title: eventTypeToTitle(
    notif.type,
    notif.message,
    notif.action,
    notif.className,
  ),
  message: notif.zone ? `${notif.zone} — ${notif.camera}` : notif.message,
  timestamp: notif.time,
});
```

- [ ] **Step 2: Verify frontend builds**

Run: `export NVM_DIR="$HOME/.nvm" && [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh" && nvm use v20.20.1 && cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add ui/src/hooks/useNotifications.ts
git commit -m "feat(ui): update notifications with structured AI fields and action-based titles"
```

---

### Task 11: Frontend — AnalyticsOverlay Updates

**Files:**

- Modify: `ui/src/components/AnalyticsOverlay.tsx`

- [ ] **Step 1: Update Detection interface and rendering**

1. Add `track_id` to the `Detection` interface:

```typescript
export interface Detection {
  id: number;
  class: string;
  confidence: number;
  box_x: number;
  box_y: number;
  box_w: number;
  box_h: number;
  frame_time: string;
  track_id?: number;
}
```

2. Update the label rendering in the `draw` callback to show track ID:

```typescript
const label = det.track_id
  ? `${displayLabel(det.class)} #${det.track_id} ${Math.round(det.confidence * 100)}%`
  : formatLabel(det.class, det.confidence);
```

3. Add zone polygon overlay rendering. Fetch zones from `/cameras/:id/zones` and draw them as semi-transparent polygons on the canvas. Add a new `useEffect` that loads zones when `cameraId` changes and draws them in the `draw` callback.

- [ ] **Step 2: Verify frontend builds**

Run: `export NVM_DIR="$HOME/.nvm" && [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh" && nvm use v20.20.1 && cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/AnalyticsOverlay.tsx
git commit -m "feat(ui): show track IDs and zone overlays in analytics overlay"
```

---

### Task 12: Frontend — Zone Editor Component

**Files:**

- Create: `ui/src/components/ZoneEditor.tsx`
- Modify: `ui/src/pages/CameraManagement.tsx`

- [ ] **Step 1: Create ZoneEditor.tsx**

Build a component that:

1. Fetches a snapshot from `GET /cameras/:id/snapshot` and renders it as the canvas background
2. Renders existing zones from `GET /cameras/:id/zones` as colored polygons
3. Allows clicking to add polygon points, double-click to close
4. Shows a sidebar with zone list, per-zone config (class toggles, cooldown sliders 0-300s, loiter threshold 0-300s, enter/leave/loiter notification toggles)
5. Saves zones via `POST /cameras/:id/zones` and `PUT /zones/:id`
6. Deletes zones via `DELETE /zones/:id`

Use `apiFetch` from `../api/client` for all API calls (includes JWT auth).

The component should accept props: `{ cameraId: string }`.

- [ ] **Step 2: Add "Zones" button to CameraManagement.tsx**

In the expanded camera section of `CameraManagement.tsx`, add a "Detection Zones" section with a button that opens the ZoneEditor in a modal or inline panel. Place it near the existing "AI Detection Settings" section.

```tsx
{
  expandedCamera.ai_enabled && (
    <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
      <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide mb-2">
        AI Detection Zones
      </h4>
      <p className="text-xs text-nvr-text-muted mb-3">
        Draw zones on the camera view to control where and how detections
        trigger notifications
      </p>
      <ZoneEditor cameraId={expandedCamera.id} />
    </div>
  );
}
```

- [ ] **Step 3: Verify frontend builds**

Run: `export NVM_DIR="$HOME/.nvm" && [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh" && nvm use v20.20.1 && cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/ZoneEditor.tsx ui/src/pages/CameraManagement.tsx
git commit -m "feat(ui): add zone editor with polygon drawing and per-zone notification config"
```

---

### Task 13: Full Build + Smoke Test

**Files:** None (verification only)

- [ ] **Step 1: Clean rebuild frontend**

```bash
export NVM_DIR="$HOME/.nvm" && [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh" && nvm use v20.20.1
cd /Users/ethanflower/personal_projects/mediamtx/ui
rm -rf node_modules/.vite ../internal/nvr/ui/dist
npx tsc -b --clean && npm run build
```

- [ ] **Step 2: Build Go binary**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go build -o mediamtx .
```

- [ ] **Step 3: Run all tests**

```bash
cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -v
```

Expected: All tracker, zone, state, and cooldown tests pass.

- [ ] **Step 4: Start server and verify**

```bash
pkill -f "./mediamtx" 2>/dev/null
DYLD_LIBRARY_PATH=$HOME/lib ./mediamtx
```

Verify in the logs:

- AI pipeline starts with tracker
- Zones are loaded (or implicit full-frame zone used)
- Notifications fire with "entered"/"left" actions instead of generic "motion"

- [ ] **Step 5: Remove debug console.log statements**

Clean up the `console.log('[NVR WS]...')` and `console.log('[NVR Toast]...')` debug statements added during earlier debugging from:

- `ui/src/hooks/useNotifications.ts`
- `ui/src/components/Toast.tsx`

Rebuild frontend after cleanup.

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "chore: clean up debug logging, verify full smart detection pipeline"
```
