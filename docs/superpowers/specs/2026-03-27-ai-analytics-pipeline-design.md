# AI Analytics Pipeline Redesign

**Date:** 2026-03-27
**Status:** Approved
**Goal:** Replace the snapshot-polling AI pipeline with a modular, sub-stream-based detection system featuring object tracking and live overlay delivery, comparable to Frigate.

---

## Context

The current AI pipeline (`internal/nvr/ai/pipeline.go`) polls JPEG snapshots via HTTP at 2 FPS, runs YOLO inference, and publishes results via an EventBroadcaster. This has several problems:

- **High latency**: each snapshot requires a full HTTP request (200-500ms overhead)
- **No object tracking**: each frame is processed independently; `track_id` is always 0
- **Monolithic design**: one file handles snapshot capture, inference, motion event lifecycle, embedding generation, DB writes, and event publishing
- **ONVIF metadata unused**: `onvif/metadata.go` parses camera-native analytics with bounding boxes but isn't wired into anything
- **Fragmented delivery**: detection data was delivered via SSE, WebSocket, and REST polling at various points

## Decisions

| Decision         | Choice                                         | Rationale                                                                                 |
| ---------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Frame source     | FFmpeg subprocess decoding RTSP stream         | Proven (Frigate uses this), supports all codecs, no CGo dependency                        |
| Detection FPS    | Match sub-stream native FPS                    | No artificial throttling; user controls load via stream choice on camera                  |
| Stream selection | Configurable per camera (any stream)           | Not hardcoded to sub-stream; user assigns via `ai_detection` role or explicit `stream_id` |
| Object tracking  | Simple IoU tracker                             | Minimum needed for enter/leave/loiter events; upgrade path to SORT/DeepSORT exists        |
| Camera analytics | Always run YOLO; ONVIF metadata supplements    | Consistent detection quality across all cameras regardless of camera capabilities         |
| Architecture     | Modular pipeline with channel-connected stages | Clean separation, independently testable stages, natural backpressure                     |
| Overlay delivery | WebSocket (existing)                           | Already works; `detection_frame` event format stays the same with real track IDs          |

---

## Pipeline Architecture

Each AI-enabled camera gets a `Pipeline` instance that orchestrates four stages connected by typed Go channels:

```
┌─────────────────────────────────────────────────────────────────┐
│ Pipeline (per camera)                                           │
│                                                                 │
│  ┌──────────┐    ┌──────────┐    ┌─────────┐    ┌───────────┐ │
│  │ FrameSrc  │──→│ Detector │──→│ Tracker  │──→│ Publisher  │ │
│  │ (FFmpeg)  │   │ (YOLO)   │   │ (IoU)    │   │ (WS+DB)   │ │
│  └──────────┘    └──────────┘    └─────────┘    └───────────┘ │
│       ↑                                                         │
│  ┌──────────┐                                                   │
│  │ ONVIFSrc │ (optional, merges into Detector output)           │
│  └──────────┘                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Channel types

- `FrameSrc → Detector`: `chan Frame` — decoded RGB image + timestamp + dimensions
- `Detector → Tracker`: `chan DetectionFrame` — timestamp + image + `[]Detection` (class, confidence, box)
- `Tracker → Publisher`: `chan TrackedFrame` — timestamp + `[]TrackedObject` (detection + trackID + lifecycle state)

### Lifecycle

- `Pipeline.Start(ctx)` launches all stages as goroutines under a shared context
- `Pipeline.Stop()` cancels the context, which cascades: FFmpeg killed → channels drain → goroutines return
- Each stage does `defer close(outChan)` so downstream stages see the close and exit their range loops
- `Pipeline.Stop()` waits on a `sync.WaitGroup` covering all stage goroutines before returning

### Shared resources (not per-pipeline)

- `Detector` holds the YOLO model — single instance shared across all pipelines via thread-safe `Detect(img) []Detection` (unchanged from today)
- `Embedder` (CLIP) — same, shared instance

---

## Stage 1: FrameSrc

Spawns an FFmpeg subprocess that connects to the camera's RTSP stream and outputs raw decoded frames over a pipe.

### FFmpeg command

```
ffmpeg -rtsp_transport tcp -i rtsp://user:pass@camera/stream
       -f rawvideo -pix_fmt rgb24 -an -sn -v quiet -
```

- `-rtsp_transport tcp`: reliable delivery, avoids UDP packet loss
- `-f rawvideo -pix_fmt rgb24`: raw RGB frames, no encoding overhead
- `-an -sn`: drop audio and subtitle tracks
- Output goes to stdout via `cmd.StdoutPipe()`

### Frame reading

- Sub-stream resolution known from camera stream config (e.g., 640x480)
- Each frame is exactly `width * height * 3` bytes (RGB24)
- Read that many bytes per iteration, wrap in `Frame{Image, Timestamp, Width, Height}`, send to output channel
- Timestamp is `time.Now()` at read time

### Process management

- FFmpeg launched on `Start()`, killed on context cancellation
- Unexpected exit: retry with exponential backoff (3s → 6s → 12s → 30s max)
- Log only on state transitions (connected → disconnected, disconnected → connected)
- Output channel buffer size 1 with drop-oldest semantics: `FrameSrc` uses a non-blocking send (`select` with `default`) to drain the old frame from the channel before sending the new one. This ensures the detector always gets the most recent frame.

### Resolution handling

- Resolved from camera stream profile in DB
- If not available, probed once via `ffprobe` on startup
- YOLO resize (to 640x640) happens in the Detector stage, not here

---

## Stage 2: Detector

Receives `Frame` from FrameSrc, runs YOLO inference, merges ONVIF metadata, emits `DetectionFrame`.

### Processing per frame

1. Resize image to 640x640 (bilinear interpolation)
2. Normalize to float32 [0,1] CHW tensor
3. Run YOLO inference via ONNX Runtime
4. Post-process: confidence threshold + NMS
5. Emit `DetectionFrame{Timestamp, Image, Detections[]}`

### What stays unchanged

The core detection logic in `detector.go` is already a clean `Detect(img) []Detection` interface. It stays as-is. The only change is that it's called from a channel-reading goroutine instead of the monolithic `ProcessFrame()`.

### ONVIF metadata merging

- `ONVIFSrc` subscribes to the camera's ONVIF metadata stream in a separate goroutine, parses via `ParseMetadataFrame()`
- ONVIF detections are merged into the `DetectionFrame` output alongside YOLO detections
- Deduplication: if a YOLO box and ONVIF box overlap > 0.5 IoU and share the same class, keep the YOLO one
- ONVIF-only detections (classes YOLO doesn't cover, camera-specific analytics) pass through unmodified

### Confidence threshold

Per-camera configurable (stored in DB). Default 0.5.

---

## Stage 3: Tracker

Assigns persistent IDs to detections across frames using IoU matching.

### Algorithm

1. Maintain list of active `TrackedObject` entries: `trackID`, `lastBox`, `class`, `confidence`, `firstSeen`, `lastSeen`, `framesMissed`
2. On each `DetectionFrame`:
   - Build cost matrix: IoU between every active track's `lastBox` and every new detection's box
   - Greedy assignment: match pairs with highest IoU first, minimum threshold 0.3
   - **Matched**: update track's box, confidence, `lastSeen`, reset `framesMissed`
   - **Unmatched detections**: create new track with incrementing ID
   - **Unmatched tracks**: increment `framesMissed`
3. Tracks with `framesMissed > maxMissed` marked as `left` and removed

### Lifecycle states

- `entered` — first frame this object appears
- `active` — tracked across frames
- `left` — disappeared for `maxMissed` frames

### Output types

```go
type TrackedFrame struct {
    Timestamp time.Time
    Objects   []TrackedObject
}

type TrackedObject struct {
    TrackID    int
    State      ObjectState  // entered, active, left
    Class      string
    Confidence float32
    Box        BoundingBox  // normalized x, y, w, h
    FirstSeen  time.Time
    LastSeen   time.Time
}
```

### Track timeout

`maxMissed` represents a time duration, not a frame count: `trackTimeout * measuredFPS`. The measured FPS is calculated from the actual frame arrival rate (exponential moving average of inter-frame intervals) rather than a configured value. Default `track_timeout`: 5 seconds. Configurable per camera.

### Edge cases

- **Class switching**: if a matched track's new detection has a different class, keep the higher-confidence class
- **Overlapping objects**: IoU tracker handles reasonably at 5+ FPS; DeepSORT upgrade path exists for later

---

## Stage 4: Publisher

Final stage — handles all output from tracked frames.

### A) WebSocket Broadcast (live overlay)

Every frame with tracked objects is published to the `EventBroadcaster` as a `detection_frame` event:

```json
{
  "type": "detection_frame",
  "camera": "Front Door",
  "time": "2026-03-27T15:30:00Z",
  "detections": [
    {
      "class": "person",
      "confidence": 0.92,
      "track_id": 3,
      "x": 0.2,
      "y": 0.3,
      "w": 0.15,
      "h": 0.4
    }
  ]
}
```

Key change: `track_id` is now a real persistent ID from the tracker.

### B) Object Lifecycle Events

- **`entered`**: call `PublishAIDetection()` (notification), call `InsertMotionEvent()` if no active event
- **`left`**: if no other active tracks remain and motion gap timer expires, call `EndMotionEvent()`

This replaces the current "count important classes and compare to last frame" logic.

### C) Database Persistence + Embeddings

Not every frame is stored. Strategy:

- **On `entered`**: store detection, generate CLIP embedding for crop, capture thumbnail
- **On `active`**: store a detection every 2 seconds if confidence improves
- **On `left`**: store final detection, update motion event with best thumbnail across track lifetime

CLIP embedding generation is asynchronous — crops sent to shared `Embedder` without blocking the publish loop.

---

## Pipeline Configuration

### Per-camera config (stored in DB)

```go
type AIPipelineConfig struct {
    AIEnabled        bool    // master toggle
    StreamID         string  // FK to camera_streams — which stream to run detection on
    ConfidenceThresh float32 // YOLO confidence threshold, default 0.5
    TrackTimeout     int     // seconds before lost track marked "left", default 5
}
```

- `StreamID` resolves to an RTSP URL via `camera_streams` table
- Fallback if unset: stream with `ai_detection` role, or lowest-resolution stream

### Pipeline lifecycle in nvr.go

- **Boot**: `startAIPipelines()` iterates cameras, builds `Pipeline` for each with `ai_enabled=true`, calls `Start(ctx)`
- **Config change**: `RestartAIPipeline(cameraID)` stops existing, rebuilds with fresh config, starts
- **Disable/delete**: `stopAIPipeline(cameraID)` cancels context, waits for goroutines to drain
- **Shutdown**: all pipelines share parent NVR context; server shutdown stops all pipelines

### API endpoint

`PUT /cameras/:id/ai` accepts:

```json
{
  "ai_enabled": true,
  "stream_id": "uuid-of-stream",
  "confidence": 0.5,
  "track_timeout": 5
}
```

### Migration

- Add `stream_id` and `track_timeout` columns to cameras table
- Migrate existing cameras: resolve `sub_stream_url` to a `stream_id` if possible, otherwise null (falls back to lowest-res stream)

---

## File Structure

```
internal/nvr/ai/
├── detector.go          # YOLO inference (unchanged)
├── embedder.go          # CLIP embeddings (unchanged)
├── search.go            # Semantic search (unchanged)
├── pipeline.go          # Pipeline orchestrator — creates stages, wires channels, manages lifecycle
├── frame_source.go      # FrameSrc stage — FFmpeg subprocess, frame reading, reconnect
├── tracker.go           # Tracker stage — IoU matching, track lifecycle
├── publisher.go         # Publisher stage — WS broadcast, DB writes, embedding generation
├── onvif_source.go      # ONVIFSrc — metadata stream subscription, detection parsing
├── merge.go             # Detection merging/dedup (YOLO + ONVIF)
└── types.go             # Shared types: Frame, Detection, DetectionFrame, TrackedObject, etc.
```

**Deleted**: the frame capture loop, `ProcessFrame()`, motion event lifecycle, and publishing logic from the current monolithic `pipeline.go`.

**Unchanged**: `detector.go`, `embedder.go`, `search.go`.

---

## Error Handling

### FFmpeg process failures

- Camera offline: FFmpeg exits, `FrameSrc` enters backoff retry (3s → 6s → 12s → 30s max), logs state transitions only
- Invalid stream URL: after 3 rapid failures within 30 seconds, pipeline marks as `errored`, surfaced via camera status API
- Corrupt frames: partial reads discarded, continue reading

### Detector overload

- `FrameSrc → Detector` channel buffer size 1, drop-oldest semantics
- If inference is slower than sub-stream FPS, frames are skipped automatically
- Overlay always shows most recent detection regardless of actual detection FPS

### Tracker edge cases

- Class switching on matched track: keep higher-confidence class
- Overlapping objects: handled reasonably by IoU at 5+ FPS; DeepSORT upgrade available later

### Pipeline teardown

- Context cancellation cascades: FFmpeg killed → pipe closes → `FrameSrc` exits → closes channel → `Detector` exits → closes channel → `Tracker` exits → `Publisher` exits
- `sync.WaitGroup` ensures `Stop()` blocks until all goroutines return

### ONVIF metadata failures

- Optional and supplementary. If connection fails, `ONVIFSrc` stops silently. Pipeline runs on YOLO alone. No retry.

---

## Flutter Client Changes

Minimal. The `detection_frame` WebSocket event format stays the same. The only visible change is that `track_id` values are now real persistent IDs from the tracker, so overlay labels show stable identifiers instead of `0`.

No changes needed to:

- `detection_stream_provider.dart` — already consumes from WebSocket
- `analytics_overlay.dart` — already renders `track_id` in labels
- `detection_frame.dart` — already parses `trackId` field
