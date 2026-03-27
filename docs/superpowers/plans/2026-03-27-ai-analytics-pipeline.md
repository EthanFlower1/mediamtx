# AI Analytics Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the snapshot-polling AI pipeline with a modular, FFmpeg-based pipeline featuring object tracking and live WebSocket overlay delivery.

**Architecture:** Per-camera pipeline of four channel-connected stages (FrameSrc → Detector → Tracker → Publisher), each running as a goroutine. FFmpeg subprocess decodes RTSP frames. IoU tracker assigns persistent object IDs. ONVIF metadata optionally merges with YOLO detections.

**Tech Stack:** Go, FFmpeg (subprocess), ONNX Runtime (YOLO), SQLite, WebSocket

**Spec:** `docs/superpowers/specs/2026-03-27-ai-analytics-pipeline-design.md`

---

## File Structure

```
internal/nvr/ai/
├── detector.go          # UNCHANGED — YOLO inference
├── embedder.go          # UNCHANGED — CLIP embeddings
├── search.go            # UNCHANGED — semantic search
├── types.go             # NEW — shared types: Frame, Detection, TrackedObject, etc.
├── frame_source.go      # NEW — FFmpeg subprocess, frame reading, reconnect
├── tracker.go           # NEW — IoU matching, track lifecycle
├── publisher.go         # NEW — WS broadcast, DB writes, embedding gen
├── onvif_source.go      # NEW — metadata stream subscription
├── merge.go             # NEW — YOLO + ONVIF dedup
├── pipeline.go          # REWRITE — orchestrator wiring stages
├── types_test.go        # NEW
├── frame_source_test.go # NEW
├── tracker_test.go      # NEW
├── publisher_test.go    # NEW
├── merge_test.go        # NEW
├── pipeline_test.go     # NEW

internal/nvr/db/
├── migrations.go        # MODIFY — add migration 22
├── cameras.go           # MODIFY — update UpdateCameraAIConfig

internal/nvr/api/
├── cameras.go           # MODIFY — expand PUT /cameras/:id/ai

internal/nvr/
├── nvr.go               # MODIFY — update pipeline creation/management
```

---

### Task 1: Shared Types

**Files:**
- Create: `internal/nvr/ai/types.go`
- Create: `internal/nvr/ai/types_test.go`

- [ ] **Step 1: Create types.go with all shared types**

```go
// internal/nvr/ai/types.go
package ai

import (
	"image"
	"time"
)

// Frame is a decoded video frame from the FrameSrc stage.
type Frame struct {
	Image     image.Image
	Timestamp time.Time
	Width     int
	Height    int
}

// BoundingBox holds normalized [0,1] coordinates.
type BoundingBox struct {
	X float32
	Y float32
	W float32
	H float32
}

// Detection is a single detected object from YOLO or ONVIF.
type Detection struct {
	Class      string
	Confidence float32
	Box        BoundingBox
	Source     DetectionSource
}

// DetectionSource identifies where a detection came from.
type DetectionSource int

const (
	SourceYOLO DetectionSource = iota
	SourceONVIF
)

// DetectionFrame is the output of the Detector stage.
type DetectionFrame struct {
	Timestamp  time.Time
	Image      image.Image
	Detections []Detection
}

// ObjectState is the lifecycle state of a tracked object.
type ObjectState int

const (
	ObjectEntered ObjectState = iota
	ObjectActive
	ObjectLeft
)

func (s ObjectState) String() string {
	switch s {
	case ObjectEntered:
		return "entered"
	case ObjectActive:
		return "active"
	case ObjectLeft:
		return "left"
	default:
		return "unknown"
	}
}

// TrackedObject is a detection with a persistent track ID and lifecycle state.
type TrackedObject struct {
	TrackID    int
	State      ObjectState
	Class      string
	Confidence float32
	Box        BoundingBox
	FirstSeen  time.Time
	LastSeen   time.Time
}

// TrackedFrame is the output of the Tracker stage.
type TrackedFrame struct {
	Timestamp time.Time
	Objects   []TrackedObject
	// Image retained for embedding generation on enter events.
	Image image.Image
}

// DetectionFrameData holds per-detection bounding box data for WebSocket
// broadcasting. This is the JSON-serialisable format sent to clients.
type DetectionFrameData struct {
	Class      string  `json:"class"`
	Confidence float32 `json:"confidence"`
	TrackID    int     `json:"track_id"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	W          float32 `json:"w"`
	H          float32 `json:"h"`
}

// EventPublisher is the interface for publishing detection events to
// notification subscribers (WebSocket clients via EventBroadcaster).
type EventPublisher interface {
	PublishAIDetection(cameraName, className string, confidence float32)
	PublishDetectionFrame(camera string, detections []DetectionFrameData)
}

// PipelineConfig holds per-camera pipeline configuration.
type PipelineConfig struct {
	CameraID         string
	CameraName       string
	StreamURL        string  // RTSP URL of the stream to decode
	StreamWidth      int     // expected frame width (0 = probe via ffprobe)
	StreamHeight     int     // expected frame height (0 = probe via ffprobe)
	ConfidenceThresh float32 // YOLO confidence threshold, default 0.5
	TrackTimeout     int     // seconds before lost track marked "left", default 5

	// ONVIF metadata endpoint (empty = disabled).
	ONVIFMetadataURL string
	ONVIFUsername     string
	ONVIFPassword     string
}
```

- [ ] **Step 2: Write test to verify ObjectState.String()**

```go
// internal/nvr/ai/types_test.go
package ai

import "testing"

func TestObjectStateString(t *testing.T) {
	tests := []struct {
		state ObjectState
		want  string
	}{
		{ObjectEntered, "entered"},
		{ObjectActive, "active"},
		{ObjectLeft, "left"},
		{ObjectState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("ObjectState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/ai/ -run TestObjectState -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/types.go internal/nvr/ai/types_test.go
git commit -m "feat(ai): add shared pipeline types"
```

---

### Task 2: DB Migration — Add stream_id and track_timeout

**Files:**
- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/cameras.go`

- [ ] **Step 1: Add migration 22 to migrations.go**

Locate the `migrations` slice in `internal/nvr/db/migrations.go` and append after the last entry (migration 21). Also update the `currentVersion` constant.

```go
// Migration 22: Add AI pipeline stream selection and track timeout.
{
    Version: 22,
    SQL: `
        ALTER TABLE cameras ADD COLUMN ai_stream_id TEXT DEFAULT '';
        ALTER TABLE cameras ADD COLUMN ai_track_timeout INTEGER DEFAULT 5;
    `,
},
```

Update `currentVersion`:
```go
const currentVersion = 22
```

- [ ] **Step 2: Add fields to Camera struct in cameras.go**

Add to the `Camera` struct in `internal/nvr/db/cameras.go` after `AIEnabled`:

```go
AIStreamID       string `json:"ai_stream_id,omitempty"`
AITrackTimeout   int    `json:"ai_track_timeout"`
```

Update all `scanCamera` helper / `Scan()` calls that read from the cameras table to include the two new columns. Add the new columns to the SELECT column list and Scan destination list in `GetCamera`, `ListCameras`, and any other functions that read full camera rows.

- [ ] **Step 3: Update UpdateCameraAIConfig**

Replace the existing `UpdateCameraAIConfig` in `internal/nvr/db/cameras.go`:

```go
func (d *DB) UpdateCameraAIConfig(id string, aiEnabled bool, streamID string, confidence float32, trackTimeout int) error {
	if trackTimeout <= 0 {
		trackTimeout = 5
	}
	if confidence <= 0 {
		confidence = 0.5
	}
	res, err := d.db.Exec(`
		UPDATE cameras
		SET ai_enabled = ?, ai_stream_id = ?, ai_track_timeout = ?, updated_at = ?
		WHERE id = ?`,
		aiEnabled, streamID, trackTimeout,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

Note: `confidence` is already stored in the detections themselves (per-detection), and the threshold is applied at detection time. We pass it through to the pipeline config but don't need a separate DB column — it's part of the API request and pipeline config, not persisted separately. If the user changes it, `RestartAIPipeline` picks up the new value.

Actually, we should store confidence in the DB too so it persists across restarts. Add to migration 22:

```go
{
    Version: 22,
    SQL: `
        ALTER TABLE cameras ADD COLUMN ai_stream_id TEXT DEFAULT '';
        ALTER TABLE cameras ADD COLUMN ai_track_timeout INTEGER DEFAULT 5;
        ALTER TABLE cameras ADD COLUMN ai_confidence REAL DEFAULT 0.5;
    `,
},
```

And add to Camera struct:
```go
AIConfidence     float64 `json:"ai_confidence"`
```

Update `UpdateCameraAIConfig`:
```go
func (d *DB) UpdateCameraAIConfig(id string, aiEnabled bool, streamID string, confidence float64, trackTimeout int) error {
	if trackTimeout <= 0 {
		trackTimeout = 5
	}
	if confidence <= 0 {
		confidence = 0.5
	}
	res, err := d.db.Exec(`
		UPDATE cameras
		SET ai_enabled = ?, ai_stream_id = ?, ai_confidence = ?, ai_track_timeout = ?, updated_at = ?
		WHERE id = ?`,
		aiEnabled, streamID, confidence, trackTimeout,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run existing DB tests to verify migration**

Run: `go test ./internal/nvr/db/ -v -count=1 2>&1 | tail -20`
Expected: PASS (migration applies cleanly on fresh DB)

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/cameras.go
git commit -m "feat(db): add ai_stream_id, ai_confidence, ai_track_timeout columns"
```

---

### Task 3: Frame Source Stage

**Files:**
- Create: `internal/nvr/ai/frame_source.go`
- Create: `internal/nvr/ai/frame_source_test.go`

- [ ] **Step 1: Write frame_source.go**

```go
// internal/nvr/ai/frame_source.go
package ai

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FrameSrc spawns an FFmpeg subprocess to decode RTSP frames and sends them
// to an output channel.
type FrameSrc struct {
	streamURL string
	width     int
	height    int
	out       chan Frame
}

// NewFrameSrc creates a new FrameSrc. If width/height are 0, they must be
// probed before calling Run.
func NewFrameSrc(streamURL string, width, height int, out chan Frame) *FrameSrc {
	return &FrameSrc{
		streamURL: streamURL,
		width:     width,
		height:    height,
		out:       out,
	}
}

// ProbeResolution uses ffprobe to determine stream resolution.
func ProbeResolution(streamURL string) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x",
		"-rtsp_transport", "tcp",
		streamURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe failed: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("ffprobe unexpected output: %q", string(out))
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse width: %w", err)
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse height: %w", err)
	}
	return w, h, nil
}

// Run starts the FFmpeg subprocess and reads frames until ctx is cancelled.
// It retries on failure with exponential backoff.
func (fs *FrameSrc) Run(ctx context.Context) {
	defer close(fs.out)

	backoff := 3 * time.Second
	maxBackoff := 30 * time.Second
	connected := false

	for {
		if ctx.Err() != nil {
			return
		}

		err := fs.readFrames(ctx)

		if ctx.Err() != nil {
			return
		}

		// Log state transition.
		if connected {
			log.Printf("[ai][%s] ffmpeg disconnected: %v", fs.streamURL, err)
			connected = false
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (fs *FrameSrc) readFrames(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", fs.streamURL,
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-an", "-sn",
		"-v", "quiet",
		"-",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	defer cmd.Wait() //nolint:errcheck

	log.Printf("[ai][%s] ffmpeg connected (%dx%d)", fs.streamURL, fs.width, fs.height)

	frameSize := fs.width * fs.height * 3
	buf := make([]byte, frameSize)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err := io.ReadFull(stdout, buf)
		if err != nil {
			return fmt.Errorf("read frame: %w", err)
		}

		img := rgbToImage(buf, fs.width, fs.height)
		frame := Frame{
			Image:     img,
			Timestamp: time.Now(),
			Width:     fs.width,
			Height:    fs.height,
		}

		// Drop-oldest: if channel is full, drain old frame and send new.
		select {
		case fs.out <- frame:
		default:
			select {
			case <-fs.out:
			default:
			}
			fs.out <- frame
		}
	}
}

// rgbToImage converts raw RGB24 bytes to an image.NRGBA.
func rgbToImage(data []byte, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := (y*width + x) * 3
			img.SetNRGBA(x, y, color.NRGBA{
				R: data[off],
				G: data[off+1],
				B: data[off+2],
				A: 255,
			})
		}
	}
	return img
}
```

- [ ] **Step 2: Write test for rgbToImage**

```go
// internal/nvr/ai/frame_source_test.go
package ai

import (
	"image/color"
	"testing"
)

func TestRgbToImage(t *testing.T) {
	// 2x2 image: red, green, blue, white
	data := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	img := rgbToImage(data, 2, 2)

	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected 2x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	tests := []struct {
		x, y int
		want color.NRGBA
	}{
		{0, 0, color.NRGBA{255, 0, 0, 255}},
		{1, 0, color.NRGBA{0, 255, 0, 255}},
		{0, 1, color.NRGBA{0, 0, 255, 255}},
		{1, 1, color.NRGBA{255, 255, 255, 255}},
	}
	for _, tt := range tests {
		got := img.NRGBAAt(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("pixel(%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/nvr/ai/ -run TestRgbToImage -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/frame_source.go internal/nvr/ai/frame_source_test.go
git commit -m "feat(ai): add FrameSrc stage with FFmpeg subprocess"
```

---

### Task 4: IoU Tracker Stage

**Files:**
- Create: `internal/nvr/ai/tracker.go`
- Create: `internal/nvr/ai/tracker_test.go`

- [ ] **Step 1: Write tracker.go**

```go
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
			// Emit "left" for all remaining tracks.
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
		// Keep higher-confidence class on class switch.
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
			// Don't add to remaining — track is removed.
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
```

- [ ] **Step 2: Write tracker tests**

```go
// internal/nvr/ai/tracker_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

func TestIoUBoxes(t *testing.T) {
	// Identical boxes → IoU = 1.0
	a := BoundingBox{0.1, 0.1, 0.5, 0.5}
	if got := iouBoxes(a, a); got < 0.99 {
		t.Errorf("identical boxes IoU = %f, want ~1.0", got)
	}

	// No overlap → IoU = 0
	b := BoundingBox{0.8, 0.8, 0.1, 0.1}
	if got := iouBoxes(a, b); got != 0 {
		t.Errorf("non-overlapping IoU = %f, want 0", got)
	}

	// Partial overlap
	c := BoundingBox{0.3, 0.3, 0.5, 0.5}
	iou := iouBoxes(a, c)
	if iou < 0.1 || iou > 0.5 {
		t.Errorf("partial overlap IoU = %f, expected between 0.1 and 0.5", iou)
	}
}

func TestTrackerAssignsIDs(t *testing.T) {
	in := make(chan DetectionFrame, 1)
	out := make(chan TrackedFrame, 1)
	tr := NewTracker(in, out, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()
	in <- DetectionFrame{
		Timestamp: now,
		Detections: []Detection{
			{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}},
			{Class: "car", Confidence: 0.8, Box: BoundingBox{0.6, 0.6, 0.2, 0.2}},
		},
	}

	tf := <-out
	if len(tf.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(tf.Objects))
	}
	for _, obj := range tf.Objects {
		if obj.State != ObjectEntered {
			t.Errorf("track %d state = %v, want entered", obj.TrackID, obj.State)
		}
	}
	if tf.Objects[0].TrackID == tf.Objects[1].TrackID {
		t.Error("tracks should have different IDs")
	}
}

func TestTrackerMatchesAcrossFrames(t *testing.T) {
	in := make(chan DetectionFrame, 2)
	out := make(chan TrackedFrame, 2)
	tr := NewTracker(in, out, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	// Frame 1: person enters.
	in <- DetectionFrame{
		Timestamp:  now,
		Detections: []Detection{{Class: "person", Confidence: 0.9, Box: box}},
	}
	tf1 := <-out
	enteredID := tf1.Objects[0].TrackID

	// Frame 2: same person, slightly moved.
	movedBox := BoundingBox{0.12, 0.12, 0.3, 0.5}
	in <- DetectionFrame{
		Timestamp:  now.Add(200 * time.Millisecond),
		Detections: []Detection{{Class: "person", Confidence: 0.92, Box: movedBox}},
	}
	tf2 := <-out
	if len(tf2.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(tf2.Objects))
	}
	if tf2.Objects[0].TrackID != enteredID {
		t.Errorf("track ID changed: got %d, want %d", tf2.Objects[0].TrackID, enteredID)
	}
	if tf2.Objects[0].State != ObjectActive {
		t.Errorf("state = %v, want active", tf2.Objects[0].State)
	}
}

func TestTrackerEmitsLeft(t *testing.T) {
	in := make(chan DetectionFrame, 10)
	out := make(chan TrackedFrame, 10)
	tr := NewTracker(in, out, 1) // 1 second timeout

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()

	// Frame 1: person enters.
	in <- DetectionFrame{
		Timestamp:  now,
		Detections: []Detection{{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}}},
	}
	<-out

	// Frame 2: 2 seconds later, no detections → person should leave.
	in <- DetectionFrame{
		Timestamp:  now.Add(2 * time.Second),
		Detections: nil,
	}
	tf := <-out

	foundLeft := false
	for _, obj := range tf.Objects {
		if obj.State == ObjectLeft && obj.Class == "person" {
			foundLeft = true
		}
	}
	if !foundLeft {
		t.Error("expected person to be marked as left")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/nvr/ai/ -run TestTracker -v`
Run: `go test ./internal/nvr/ai/ -run TestIoU -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/tracker.go internal/nvr/ai/tracker_test.go
git commit -m "feat(ai): add IoU tracker stage with persistent object IDs"
```

---

### Task 5: Detection Merge (YOLO + ONVIF)

**Files:**
- Create: `internal/nvr/ai/merge.go`
- Create: `internal/nvr/ai/merge_test.go`

- [ ] **Step 1: Write merge.go**

```go
// internal/nvr/ai/merge.go
package ai

// MergeDetections combines YOLO and ONVIF detections, deduplicating where
// a YOLO box and ONVIF box overlap > 0.5 IoU and share the same class.
// YOLO detections take priority on duplicates.
func MergeDetections(yolo, onvif []Detection) []Detection {
	if len(onvif) == 0 {
		return yolo
	}
	if len(yolo) == 0 {
		return onvif
	}

	merged := make([]Detection, len(yolo))
	copy(merged, yolo)

	for _, od := range onvif {
		if isDuplicate(od, yolo) {
			continue
		}
		merged = append(merged, od)
	}
	return merged
}

func isDuplicate(onvifDet Detection, yoloDets []Detection) bool {
	for _, yd := range yoloDets {
		if yd.Class == onvifDet.Class && iouBoxes(yd.Box, onvifDet.Box) > 0.5 {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Write merge tests**

```go
// internal/nvr/ai/merge_test.go
package ai

import "testing"

func TestMergeDetectionsNoDuplicates(t *testing.T) {
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}, Source: SourceYOLO},
	}
	onvif := []Detection{
		{Class: "car", Confidence: 0.8, Box: BoundingBox{0.6, 0.6, 0.2, 0.2}, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 2 {
		t.Fatalf("expected 2 detections, got %d", len(merged))
	}
}

func TestMergeDetectionsDedup(t *testing.T) {
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: box, Source: SourceYOLO},
	}
	// ONVIF reports same person at nearly same location.
	onvif := []Detection{
		{Class: "person", Confidence: 0.85, Box: BoundingBox{0.11, 0.11, 0.29, 0.49}, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 1 {
		t.Fatalf("expected 1 detection (deduped), got %d", len(merged))
	}
	if merged[0].Source != SourceYOLO {
		t.Error("expected YOLO detection to be kept")
	}
}

func TestMergeDetectionsDifferentClass(t *testing.T) {
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}
	yolo := []Detection{
		{Class: "person", Confidence: 0.9, Box: box, Source: SourceYOLO},
	}
	// ONVIF reports "line_crossing" at same location — different class, should not dedup.
	onvif := []Detection{
		{Class: "line_crossing", Confidence: 1.0, Box: box, Source: SourceONVIF},
	}
	merged := MergeDetections(yolo, onvif)
	if len(merged) != 2 {
		t.Fatalf("expected 2 detections (different class), got %d", len(merged))
	}
}

func TestMergeEmpty(t *testing.T) {
	yolo := []Detection{{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}}}
	if got := MergeDetections(yolo, nil); len(got) != 1 {
		t.Errorf("yolo + nil = %d, want 1", len(got))
	}
	if got := MergeDetections(nil, yolo); len(got) != 1 {
		t.Errorf("nil + yolo = %d, want 1", len(got))
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/nvr/ai/ -run TestMerge -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/merge.go internal/nvr/ai/merge_test.go
git commit -m "feat(ai): add YOLO + ONVIF detection merge with IoU dedup"
```

---

### Task 6: Publisher Stage

**Files:**
- Create: `internal/nvr/ai/publisher.go`
- Create: `internal/nvr/ai/publisher_test.go`

- [ ] **Step 1: Write publisher.go**

```go
// internal/nvr/ai/publisher.go
package ai

import (
	"context"
	"image"
	"log"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// importantClasses are COCO classes that trigger notification events.
var importantClasses = map[string]bool{
	"person": true, "bicycle": true, "car": true, "motorcycle": true,
	"bus": true, "truck": true, "boat": true,
	"cat": true, "dog": true, "horse": true, "sheep": true,
	"cow": true, "elephant": true, "bear": true, "zebra": true, "giraffe": true,
}

// Publisher handles all output from tracked frames: WebSocket broadcast,
// database persistence, and CLIP embedding generation.
type Publisher struct {
	in         <-chan TrackedFrame
	cameraID   string
	cameraName string
	eventPub   EventPublisher
	database   *db.DB
	embedder   *Embedder // may be nil

	mu             sync.Mutex
	activeEventID  int64
	lastStoredAt   map[int]time.Time // trackID → last DB insert time
}

// NewPublisher creates a new Publisher stage.
func NewPublisher(
	in <-chan TrackedFrame,
	cameraID, cameraName string,
	eventPub EventPublisher,
	database *db.DB,
	embedder *Embedder,
) *Publisher {
	return &Publisher{
		in:           in,
		cameraID:     cameraID,
		cameraName:   cameraName,
		eventPub:     eventPub,
		database:     database,
		embedder:     embedder,
		lastStoredAt: make(map[int]time.Time),
	}
}

// Run processes tracked frames until the input channel closes or ctx is cancelled.
func (pub *Publisher) Run(ctx context.Context) {
	var lastActivityAt time.Time
	motionGap := 8 * time.Second

	for {
		select {
		case <-ctx.Done():
			pub.closeEvent(time.Now())
			return

		case tf, ok := <-pub.in:
			if !ok {
				pub.closeEvent(time.Now())
				return
			}

			hasImportant := false
			for _, obj := range tf.Objects {
				if importantClasses[obj.Class] {
					hasImportant = true
					break
				}
			}

			// Broadcast detection_frame to WebSocket clients.
			pub.broadcastFrame(tf)

			// Handle object lifecycle events.
			for _, obj := range tf.Objects {
				switch obj.State {
				case ObjectEntered:
					if importantClasses[obj.Class] {
						pub.ensureEvent(obj, tf.Timestamp)
						pub.eventPub.PublishAIDetection(pub.cameraName, obj.Class, obj.Confidence)
					}
					pub.storeDetection(obj, tf.Timestamp, tf.Image)

				case ObjectActive:
					pub.maybeStoreDetection(obj, tf.Timestamp, tf.Image)

				case ObjectLeft:
					pub.storeDetection(obj, tf.Timestamp, nil)
					delete(pub.lastStoredAt, obj.TrackID)
				}
			}

			if hasImportant {
				lastActivityAt = tf.Timestamp
			}

			// Close event if no important activity for motionGap.
			if pub.activeEventID != 0 && !lastActivityAt.IsZero() &&
				tf.Timestamp.Sub(lastActivityAt) > motionGap {
				pub.closeEvent(tf.Timestamp)
			}
		}
	}
}

func (pub *Publisher) broadcastFrame(tf TrackedFrame) {
	if len(tf.Objects) == 0 {
		return
	}
	dets := make([]DetectionFrameData, 0, len(tf.Objects))
	for _, obj := range tf.Objects {
		if obj.State == ObjectLeft {
			continue // don't render left objects in overlay
		}
		dets = append(dets, DetectionFrameData{
			Class:      obj.Class,
			Confidence: obj.Confidence,
			TrackID:    obj.TrackID,
			X:          obj.Box.X,
			Y:          obj.Box.Y,
			W:          obj.Box.W,
			H:          obj.Box.H,
		})
	}
	if len(dets) > 0 {
		pub.eventPub.PublishDetectionFrame(pub.cameraName, dets)
	}
}

func (pub *Publisher) ensureEvent(obj TrackedObject, ts time.Time) {
	pub.mu.Lock()
	defer pub.mu.Unlock()

	if pub.activeEventID != 0 {
		return
	}

	event := &db.MotionEvent{
		CameraID:    pub.cameraID,
		StartedAt:   ts.UTC().Format("2006-01-02T15:04:05.000Z"),
		EventType:   "ai_detection",
		ObjectClass: obj.Class,
		Confidence:  float64(obj.Confidence),
	}
	if err := pub.database.InsertMotionEvent(event); err != nil {
		log.Printf("[ai][%s] insert motion event: %v", pub.cameraName, err)
		return
	}
	pub.activeEventID = event.ID
}

func (pub *Publisher) closeEvent(ts time.Time) {
	pub.mu.Lock()
	defer pub.mu.Unlock()

	if pub.activeEventID == 0 {
		return
	}
	endTime := ts.UTC().Format("2006-01-02T15:04:05.000Z")
	if err := pub.database.EndMotionEvent(pub.cameraID, endTime); err != nil {
		log.Printf("[ai][%s] end motion event: %v", pub.cameraName, err)
	}
	pub.activeEventID = 0
}

func (pub *Publisher) storeDetection(obj TrackedObject, ts time.Time, img image.Image) {
	pub.mu.Lock()
	eventID := pub.activeEventID
	pub.mu.Unlock()

	if eventID == 0 {
		return
	}

	det := &db.Detection{
		MotionEventID: eventID,
		FrameTime:     ts.UTC().Format("2006-01-02T15:04:05.000Z"),
		Class:         obj.Class,
		Confidence:    float64(obj.Confidence),
		BoxX:          float64(obj.Box.X),
		BoxY:          float64(obj.Box.Y),
		BoxW:          float64(obj.Box.W),
		BoxH:          float64(obj.Box.H),
	}

	// Generate CLIP embedding asynchronously for enter events.
	if img != nil && pub.embedder != nil && obj.State == ObjectEntered {
		go pub.generateEmbedding(det, img, obj.Box)
	}

	if err := pub.database.InsertDetection(det); err != nil {
		log.Printf("[ai][%s] insert detection: %v", pub.cameraName, err)
	}
	pub.lastStoredAt[obj.TrackID] = ts
}

func (pub *Publisher) maybeStoreDetection(obj TrackedObject, ts time.Time, img image.Image) {
	last, ok := pub.lastStoredAt[obj.TrackID]
	if ok && ts.Sub(last) < 2*time.Second {
		return
	}
	pub.storeDetection(obj, ts, img)
}

func (pub *Publisher) generateEmbedding(det *db.Detection, img image.Image, box BoundingBox) {
	crop := cropRegion(img, box)
	if crop == nil {
		return
	}
	embedding, err := pub.embedder.EncodeImage(crop)
	if err != nil {
		log.Printf("[ai] embedding error: %v", err)
		return
	}
	det.Embedding = float32SliceToBytes(embedding)
}

// cropRegion extracts a bounding box region from an image.
func cropRegion(img image.Image, box BoundingBox) image.Image {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	x := int(float32(bounds.Dx()) * box.X)
	y := int(float32(bounds.Dy()) * box.Y)
	w := int(float32(bounds.Dx()) * box.W)
	h := int(float32(bounds.Dy()) * box.H)
	if w <= 0 || h <= 0 {
		return nil
	}
	rect := image.Rect(x, y, x+w, y+h).Intersect(bounds)
	if rect.Empty() {
		return nil
	}
	return cropImage(img, rect)
}
```

- [ ] **Step 2: Write publisher tests**

```go
// internal/nvr/ai/publisher_test.go
package ai

import (
	"image"
	"testing"
)

func TestCropRegion(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	box := BoundingBox{0.1, 0.2, 0.5, 0.5}
	crop := cropRegion(img, box)
	if crop == nil {
		t.Fatal("expected non-nil crop")
	}
	bounds := crop.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("crop size = %dx%d, want 50x50", bounds.Dx(), bounds.Dy())
	}
}

func TestCropRegionNilImage(t *testing.T) {
	crop := cropRegion(nil, BoundingBox{0.1, 0.1, 0.5, 0.5})
	if crop != nil {
		t.Error("expected nil crop for nil image")
	}
}

func TestCropRegionZeroSize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	crop := cropRegion(img, BoundingBox{0.1, 0.1, 0, 0})
	if crop != nil {
		t.Error("expected nil crop for zero-size box")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/nvr/ai/ -run TestCropRegion -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/publisher.go internal/nvr/ai/publisher_test.go
git commit -m "feat(ai): add Publisher stage for WS broadcast, DB writes, embeddings"
```

---

### Task 7: ONVIF Metadata Source

**Files:**
- Create: `internal/nvr/ai/onvif_source.go`

- [ ] **Step 1: Write onvif_source.go**

```go
// internal/nvr/ai/onvif_source.go
package ai

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// ONVIFSrc subscribes to a camera's ONVIF metadata stream and converts
// parsed detections into the Detection type for merging with YOLO results.
type ONVIFSrc struct {
	metadataURL string
	username    string
	password    string
	// latestDets holds the most recent ONVIF detections, read by the
	// Detector stage when merging.
	latestDets []Detection
}

// NewONVIFSrc creates a new ONVIFSrc. Returns nil if metadataURL is empty.
func NewONVIFSrc(metadataURL, username, password string) *ONVIFSrc {
	if metadataURL == "" {
		return nil
	}
	return &ONVIFSrc{
		metadataURL: metadataURL,
		username:    username,
		password:    password,
	}
}

// LatestDetections returns the most recently parsed ONVIF detections.
func (os *ONVIFSrc) LatestDetections() []Detection {
	return os.latestDets
}

// Run connects to the metadata stream and parses frames until ctx is cancelled.
// Errors are logged but not retried — ONVIF metadata is supplementary.
func (os *ONVIFSrc) Run(ctx context.Context) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", os.metadataURL, nil)
	if err != nil {
		log.Printf("[ai][onvif] invalid metadata URL: %v", err)
		return
	}
	if os.username != "" {
		req.SetBasicAuth(os.username, os.password)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ai][onvif] metadata connect failed: %v", err)
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			frame, parseErr := onvif.ParseMetadataFrame(buf[:n])
			if parseErr == nil && frame != nil {
				os.latestDets = convertONVIFDetections(frame)
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[ai][onvif] metadata read error: %v", err)
			}
			return
		}
	}
}

func convertONVIFDetections(frame *onvif.MetadataFrame) []Detection {
	dets := make([]Detection, 0, len(frame.Objects))
	for _, obj := range frame.Objects {
		dets = append(dets, Detection{
			Class:      obj.Class,
			Confidence: float32(obj.Score),
			Box: BoundingBox{
				X: float32(obj.Box.Left),
				Y: float32(obj.Box.Top),
				W: float32(obj.Box.Right - obj.Box.Left),
				H: float32(obj.Box.Bottom - obj.Box.Top),
			},
			Source: SourceONVIF,
		})
	}
	return dets
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/ai/onvif_source.go
git commit -m "feat(ai): add ONVIFSrc for camera metadata stream parsing"
```

---

### Task 8: Pipeline Orchestrator

**Files:**
- Rewrite: `internal/nvr/ai/pipeline.go`
- Create: `internal/nvr/ai/pipeline_test.go`

- [ ] **Step 1: Rename old pipeline.go to pipeline_legacy.go**

```bash
mv internal/nvr/ai/pipeline.go internal/nvr/ai/pipeline_legacy.go
```

We keep the legacy file temporarily so helper functions like `cropImage`, `float32SliceToBytes`, `captureAndDecode`, etc. remain available during migration. These will be cleaned up in a later task.

- [ ] **Step 2: Write new pipeline.go**

```go
// internal/nvr/ai/pipeline.go
package ai

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Pipeline orchestrates the four detection stages for a single camera.
type Pipeline struct {
	config   PipelineConfig
	detector *Detector
	embedder *Embedder
	database *db.DB
	eventPub EventPublisher

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPipeline creates a new pipeline. Call Start to begin processing.
func NewPipeline(
	config PipelineConfig,
	detector *Detector,
	embedder *Embedder,
	database *db.DB,
	eventPub EventPublisher,
) *Pipeline {
	return &Pipeline{
		config:   config,
		detector: detector,
		embedder: embedder,
		database: database,
		eventPub: eventPub,
	}
}

// Start launches all pipeline stages as goroutines.
func (p *Pipeline) Start(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	p.cancel = cancel

	// Probe resolution if not configured.
	width, height := p.config.StreamWidth, p.config.StreamHeight
	if width == 0 || height == 0 {
		var err error
		width, height, err = ProbeResolution(p.config.StreamURL)
		if err != nil {
			log.Printf("[ai][%s] ffprobe failed, using 640x480: %v", p.config.CameraName, err)
			width, height = 640, 480
		}
		log.Printf("[ai][%s] probed resolution: %dx%d", p.config.CameraName, width, height)
	}

	// Create channels between stages.
	frameCh := make(chan Frame, 1)
	detCh := make(chan DetectionFrame, 1)
	trackCh := make(chan TrackedFrame, 1)

	// Stage 1: FrameSrc
	frameSrc := NewFrameSrc(p.config.StreamURL, width, height, frameCh)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		frameSrc.Run(ctx)
	}()

	// Optional: ONVIF metadata source.
	var onvifSrc *ONVIFSrc
	if p.config.ONVIFMetadataURL != "" {
		onvifSrc = NewONVIFSrc(
			p.config.ONVIFMetadataURL,
			p.config.ONVIFUsername,
			p.config.ONVIFPassword,
		)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			onvifSrc.Run(ctx)
		}()
	}

	// Stage 2: Detector (reads frames, runs YOLO, merges ONVIF, emits DetectionFrame)
	confThresh := p.config.ConfidenceThresh
	if confThresh <= 0 {
		confThresh = 0.5
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(detCh)
		p.runDetector(ctx, frameCh, detCh, onvifSrc, confThresh)
	}()

	// Stage 3: Tracker
	tracker := NewTracker(detCh, trackCh, p.config.TrackTimeout)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		tracker.Run(ctx)
	}()

	// Stage 4: Publisher
	publisher := NewPublisher(trackCh, p.config.CameraID, p.config.CameraName, p.eventPub, p.database, p.embedder)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		publisher.Run(ctx)
	}()

	log.Printf("[ai][%s] pipeline started (%dx%d, conf=%.2f, timeout=%ds)",
		p.config.CameraName, width, height, confThresh, p.config.TrackTimeout)
}

// Stop cancels the pipeline context and waits for all stages to exit.
func (p *Pipeline) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Printf("[ai][%s] pipeline stopped", p.config.CameraName)
}

func (p *Pipeline) runDetector(
	ctx context.Context,
	in <-chan Frame,
	out chan<- DetectionFrame,
	onvifSrc *ONVIFSrc,
	confThresh float32,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-in:
			if !ok {
				return
			}

			yoloDets, err := p.detector.Detect(frame.Image, confThresh)
			if err != nil {
				log.Printf("[ai][%s] detect error: %v", p.config.CameraName, err)
				continue
			}

			// Convert YOLO detections to pipeline Detection type.
			dets := make([]Detection, len(yoloDets))
			for i, yd := range yoloDets {
				dets[i] = Detection{
					Class:      yd.Class,
					Confidence: yd.Confidence,
					Box:        BoundingBox{X: yd.X, Y: yd.Y, W: yd.W, H: yd.H},
					Source:     SourceYOLO,
				}
			}

			// Merge ONVIF detections if available.
			if onvifSrc != nil {
				onvifDets := onvifSrc.LatestDetections()
				dets = MergeDetections(dets, onvifDets)
			}

			df := DetectionFrame{
				Timestamp:  frame.Timestamp,
				Image:      frame.Image,
				Detections: dets,
			}
			select {
			case out <- df:
			case <-ctx.Done():
				return
			}
		}
	}
}
```

- [ ] **Step 3: Write pipeline integration test**

```go
// internal/nvr/ai/pipeline_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

// mockEventPub implements EventPublisher for testing.
type mockEventPub struct {
	frames      [][]DetectionFrameData
	detections  []string
}

func (m *mockEventPub) PublishAIDetection(camera, class string, conf float32) {
	m.detections = append(m.detections, class)
}

func (m *mockEventPub) PublishDetectionFrame(camera string, dets []DetectionFrameData) {
	m.frames = append(m.frames, dets)
}

func TestPipelineChannelWiring(t *testing.T) {
	// Test that data flows through the channel pipeline without a real FFmpeg
	// or YOLO model. We manually push frames into the frame channel and verify
	// tracked output arrives at the publisher.

	frameCh := make(chan Frame, 1)
	detCh := make(chan DetectionFrame, 1)
	trackCh := make(chan TrackedFrame, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewTracker(detCh, trackCh, 5)
	go tracker.Run(ctx)

	now := time.Now()

	// Simulate detector output.
	detCh <- DetectionFrame{
		Timestamp: now,
		Detections: []Detection{
			{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}},
		},
	}

	// Read tracked output.
	select {
	case tf := <-trackCh:
		if len(tf.Objects) != 1 {
			t.Fatalf("expected 1 tracked object, got %d", len(tf.Objects))
		}
		if tf.Objects[0].Class != "person" {
			t.Errorf("class = %q, want person", tf.Objects[0].Class)
		}
		if tf.Objects[0].State != ObjectEntered {
			t.Errorf("state = %v, want entered", tf.Objects[0].State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tracked frame")
	}

	_ = frameCh // unused in this test, just verifying channel types compile
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/nvr/ai/ -run TestPipelineChannelWiring -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/ai/pipeline.go internal/nvr/ai/pipeline_legacy.go internal/nvr/ai/pipeline_test.go
git commit -m "feat(ai): add modular Pipeline orchestrator wiring all stages"
```

---

### Task 9: Update API Endpoint

**Files:**
- Modify: `internal/nvr/api/cameras.go`

- [ ] **Step 1: Update the UpdateAIConfig handler**

Find the `UpdateAIConfig` handler in `internal/nvr/api/cameras.go`. Update the request struct and handler to accept the new fields:

```go
func (h *CameraHandler) UpdateAIConfig(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		AIEnabled    bool    `json:"ai_enabled"`
		StreamID     string  `json:"stream_id"`
		Confidence   float64 `json:"confidence"`
		TrackTimeout int     `json:"track_timeout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.UpdateCameraAIConfig(id, req.AIEnabled, req.StreamID, req.Confidence, req.TrackTimeout); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.AIRestarter != nil {
		h.AIRestarter.RestartAIPipeline(id)
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cam)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nvr/api/`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/cameras.go
git commit -m "feat(api): expand PUT /cameras/:id/ai with stream_id, confidence, track_timeout"
```

---

### Task 10: NVR Integration

**Files:**
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Update aiPipelines map type**

Change the `aiPipelines` field from `map[string]*ai.AIPipeline` to `map[string]*ai.Pipeline`:

```go
aiPipelines map[string]*ai.Pipeline
```

- [ ] **Step 2: Rewrite startAIPipelines()**

Replace the existing `startAIPipelines` function:

```go
func (n *NVR) startAIPipelines() {
	if n.aiDetector == nil {
		return
	}
	n.aiPipelines = make(map[string]*ai.Pipeline)

	cameras, err := n.database.ListCameras()
	if err != nil {
		log.Printf("ai: failed to list cameras: %v", err)
		return
	}

	for _, cam := range cameras {
		if !cam.AIEnabled {
			continue
		}
		n.startSinglePipeline(cam)
	}

	log.Printf("ai: started %d pipelines", len(n.aiPipelines))
}

func (n *NVR) startSinglePipeline(cam *db.Camera) {
	// Resolve stream URL: explicit stream_id > ai_detection role > lowest-res stream > legacy sub_stream_url.
	streamURL := ""
	var streamWidth, streamHeight int

	if cam.AIStreamID != "" {
		stream, err := n.database.GetCameraStream(cam.AIStreamID)
		if err == nil {
			streamURL = stream.RTSPURL
			streamWidth = stream.Width
			streamHeight = stream.Height
		}
	}
	if streamURL == "" {
		resolved, err := n.database.ResolveStreamURL(cam.ID, db.StreamRoleAIDetection)
		if err == nil && resolved != "" {
			streamURL = resolved
		}
	}
	if streamURL == "" && cam.SubStreamURL != "" {
		streamURL = cam.SubStreamURL
	}
	if streamURL == "" && cam.RTSPURL != "" {
		streamURL = cam.RTSPURL
	}
	if streamURL == "" {
		log.Printf("ai: camera %s (%s) has no stream URL for AI", cam.ID, cam.Name)
		return
	}

	// Embed credentials into RTSP URL if needed.
	streamURL = n.embedCredentials(cam, streamURL)

	config := ai.PipelineConfig{
		CameraID:         cam.ID,
		CameraName:       cam.Name,
		StreamURL:        streamURL,
		StreamWidth:      streamWidth,
		StreamHeight:     streamHeight,
		ConfidenceThresh: float32(cam.AIConfidence),
		TrackTimeout:     cam.AITrackTimeout,
	}

	pipeline := ai.NewPipeline(config, n.aiDetector, n.aiEmbedder, n.database, n.events)
	pipeline.Start(n.ctx)
	n.aiPipelines[cam.ID] = pipeline
}

// embedCredentials inserts ONVIF or RTSP credentials into the URL if not already present.
func (n *NVR) embedCredentials(cam *db.Camera, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.User != nil && u.User.Username() != "" {
		return rawURL // already has credentials
	}
	username := cam.ONVIFUsername
	password := cam.ONVIFPassword
	if password != "" && n.encryptionKey != nil {
		password = n.decryptPassword(n.encryptionKey, password)
	}
	if username != "" {
		u.User = url.UserPassword(username, password)
	}
	return u.String()
}
```

- [ ] **Step 3: Rewrite RestartAIPipeline()**

```go
func (n *NVR) RestartAIPipeline(cameraID string) {
	// Stop existing pipeline.
	if p, ok := n.aiPipelines[cameraID]; ok {
		p.Stop()
		delete(n.aiPipelines, cameraID)
	}

	if n.aiDetector == nil {
		return
	}

	cam, err := n.database.GetCamera(cameraID)
	if err != nil {
		log.Printf("ai: restart pipeline: get camera %s: %v", cameraID, err)
		return
	}

	if !cam.AIEnabled {
		return
	}

	n.startSinglePipeline(cam)
}
```

- [ ] **Step 4: Update Close() to stop new pipeline type**

Find the section in `Close()` that stops AI pipelines and update it:

```go
for id, p := range n.aiPipelines {
    p.Stop()
    delete(n.aiPipelines, id)
}
```

- [ ] **Step 5: Verify build**

Run: `go build .`
Expected: builds successfully

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(nvr): integrate modular AI pipeline with stream resolution"
```

---

### Task 11: Clean Up Legacy Pipeline

**Files:**
- Delete: `internal/nvr/ai/pipeline_legacy.go`
- Modify: `internal/nvr/ai/publisher.go` (if needed — ensure `cropImage` and `float32SliceToBytes` are available)

- [ ] **Step 1: Move needed helpers from legacy to appropriate files**

Check if `cropImage`, `float32SliceToBytes`, `bytesToFloat32Slice` are used by other files (search.go, embedder.go). If so, move them to a `helpers.go` or keep them in `publisher.go`. These functions are small:

```go
// cropImage in publisher.go — already referenced, ensure it exists.
// float32SliceToBytes in publisher.go — already referenced.
// bytesToFloat32Slice — used by search.go, move to types.go or a helpers file.
```

Create `internal/nvr/ai/helpers.go` with the shared utility functions that are referenced by `search.go` and `publisher.go`:

```go
// internal/nvr/ai/helpers.go
package ai

import (
	"encoding/binary"
	"image"
	"math"
)

// cropImage extracts a rectangular region from an image.
func cropImage(img image.Image, rect image.Rectangle) image.Image {
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(rect)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.Set(x-rect.Min.X, y-rect.Min.Y, img.At(x, y))
		}
	}
	return dst
}

// float32SliceToBytes converts a float32 slice to a byte slice.
func float32SliceToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32Slice converts a byte slice back to a float32 slice.
func bytesToFloat32Slice(b []byte) []float32 {
	fs := make([]float32, len(b)/4)
	for i := range fs {
		fs[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return fs
}
```

- [ ] **Step 2: Delete the legacy pipeline file**

```bash
rm internal/nvr/ai/pipeline_legacy.go
```

- [ ] **Step 3: Verify build**

Run: `go build .`
Expected: builds successfully. Fix any compilation errors from removed functions.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/ai/
git commit -m "refactor(ai): remove legacy pipeline, extract shared helpers"
```

---

### Task 12: End-to-End Verification

- [ ] **Step 1: Run all AI package tests**

Run: `go test ./internal/nvr/ai/ -v -count=1`
Expected: all tests pass

- [ ] **Step 2: Run all DB tests**

Run: `go test ./internal/nvr/db/ -v -count=1`
Expected: all tests pass

- [ ] **Step 3: Run full build**

Run: `go build .`
Expected: binary builds successfully

- [ ] **Step 4: Manual smoke test**

1. Start the server: `./mediamtx`
2. Enable AI on a camera via the Flutter UI or API: `PUT /api/nvr/cameras/:id/ai` with `{"ai_enabled": true}`
3. Verify in server logs:
   - `[ai][CameraName] probed resolution: WxH`
   - `[ai][CameraName] ffmpeg connected (WxH)`
   - `[ai][CameraName] pipeline started`
4. Open the Flutter live view — verify bounding boxes appear with stable track IDs
5. Disable AI — verify `[ai][CameraName] pipeline stopped` in logs

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "test: verify AI analytics pipeline end-to-end"
```
