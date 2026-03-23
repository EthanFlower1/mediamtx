# Embedded AI Analytics Design

## Overview

Add embedded AI object detection and semantic search to the NVR. Models are bundled in the binary (no external services, no downloads). Inference runs on every frame from the camera's sub stream, tapped directly from MediaMTX's internal reader pipeline.

## Models (all bundled via go:embed)

| Model | Size | Format | Purpose |
|-------|------|--------|---------|
| YOLOv8n (nano) | ~6MB | ONNX | Real-time detection on every sub-stream frame |
| YOLOv8m (medium) | ~50MB | ONNX | Tier 2 refinement after event ends |
| CLIP ViT-B/32 | ~350MB | ONNX | Visual embeddings for semantic search |

Total binary size increase: ~406MB. Trade-off: works offline with zero setup.

## Runtime

**ONNX Runtime Go** (`yalue/onnxruntime_go`) for model inference. Supports:
- CPU (all platforms, default)
- CoreML (Apple Silicon, auto-detected)
- CUDA (NVIDIA GPUs, auto-detected)
- DirectML (Windows GPUs, auto-detected)

The runtime auto-selects the best backend available.

## Frame Pipeline

### Source: MediaMTX Internal Reader

Register as an internal `stream.Reader` on the camera's sub-stream path using `stream.AddReader()`. This receives encoded frames without opening a separate RTSP connection.

### Decoding

- **MJPEG** (recommended): `image/jpeg.Decode()` — pure Go, standard library, trivial
- **H.264**: `pion/mediadevices` pure-Go decoder — no CGo, no FFmpeg
- **H.265**: extract I-frames only as fallback, recommend users switch sub stream to MJPEG or H.264

Camera setup UI recommends MJPEG for the AI detection stream.

### Inference Flow

```
Sub stream path (registered via stream.AddReader)
    ↓
Encoded frame (MJPEG / H.264 NAL units)
    ↓
Decode to image.Image
    ↓
Resize to 640x640 (YOLO input)
    ↓
Tier 1: YOLOv8n → [{class:"person", box:{0.1,0.2,0.3,0.4}, conf:0.87}]
    ↓
For each detection with conf > threshold:
    ├── Crop bounding box from original frame
    ├── Resize crop to 224x224 (CLIP input)
    ├── CLIP → 512-float embedding vector
    ├── Insert into detections table
    └── If no open motion event → create one + capture snapshot
    ↓
After event ends:
    ├── Tier 2: YOLOv8m on the event's snapshot
    ├── Update motion_event with refined object_class + confidence
    └── Generate text description from top detections
```

## Sub Stream Configuration

Most ONVIF cameras expose multiple media profiles. During camera probing (`ProbeDeviceFull`), we already discover all profiles. The second profile is typically the sub stream (lower resolution, lower FPS).

Add `sub_stream_url` field to the Camera model. During camera setup, the UI shows all available profiles and lets the user select which one to use for AI detection. Recommend the lowest resolution MJPEG profile.

If no sub stream is configured, AI detection is disabled for that camera.

## Database Changes

### New migration:

```sql
ALTER TABLE cameras ADD COLUMN sub_stream_url TEXT DEFAULT '';
ALTER TABLE cameras ADD COLUMN ai_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE detections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    motion_event_id INTEGER NOT NULL,
    frame_time TEXT NOT NULL,
    class TEXT NOT NULL,
    confidence REAL NOT NULL,
    box_x REAL NOT NULL,
    box_y REAL NOT NULL,
    box_w REAL NOT NULL,
    box_h REAL NOT NULL,
    embedding BLOB,
    attributes TEXT DEFAULT '',
    FOREIGN KEY (motion_event_id) REFERENCES motion_events(id) ON DELETE CASCADE
);
CREATE INDEX idx_detections_event ON detections(motion_event_id);
CREATE INDEX idx_detections_class ON detections(class);
```

### Updated motion_events columns (already exist):
- `object_class` — set by Tier 1, refined by Tier 2
- `confidence` — set by Tier 1, refined by Tier 2
- `embedding` — CLIP vector for the event's primary detection (new column, BLOB)
- `description` — auto-generated text description (new column, TEXT)

## Semantic Search

### API

```
GET /api/nvr/search?q=woman+in+red+shirt&camera_id=...&start=...&end=...&limit=20
```

1. CLIP encodes the query text → 512-float vector
2. Cosine similarity against all stored embeddings in `detections` table
3. Join with `motion_events` for event metadata
4. Return results ranked by similarity score
5. Filter by camera, date range, minimum similarity threshold

### Cosine Similarity in SQLite

SQLite doesn't have native vector operations. Two options:
- **Option A**: Load embeddings in Go, compute similarity in-memory. Simple, works for <100K events.
- **Option B**: Use SQLite's `sqlite-vec` extension for vector search. More scalable but adds dependency.

Use **Option A** for v1. Fetch candidate embeddings with SQL filters (camera, date range), compute cosine similarity in Go, sort and return top N. For typical NVR usage (<100K events), this is fast enough.

## Architecture

### New files

```
internal/nvr/ai/
├── detector.go      # YOLO inference wrapper
├── embedder.go      # CLIP inference wrapper
├── pipeline.go      # Frame pipeline: decode → detect → embed → store
├── models.go        # go:embed for ONNX model files
├── search.go        # Semantic search (cosine similarity)
└── decoder.go       # Frame decoding (MJPEG + H.264 via pion)

models/
├── yolov8n.onnx     # Bundled YOLO nano model
├── yolov8m.onnx     # Bundled YOLO medium model
└── clip-vit-b32.onnx # Bundled CLIP model
```

### Integration points

- `internal/nvr/nvr.go` — start/stop AI pipeline per camera
- `internal/nvr/api/cameras.go` — AI enable/disable, sub stream selection
- `internal/nvr/api/search.go` — semantic search endpoint
- `internal/core/path.go` — register AI reader on sub stream path

### Per-camera AI pipeline lifecycle

```go
type AIPipeline struct {
    camera    *db.Camera
    reader    *stream.Reader    // MediaMTX internal reader
    detector  *Detector         // YOLOv8n
    embedder  *Embedder         // CLIP
    db        *db.DB
    stopCh    chan struct{}
}

func (p *AIPipeline) Start(stream *stream.Stream)
func (p *AIPipeline) Stop()
```

The NVR starts an AIPipeline for each camera that has `ai_enabled=true` and `sub_stream_url` set. The pipeline registers as a reader on the sub stream path and processes every frame.

## UI Changes

### Camera Management
- Sub stream selector during camera setup (show all profiles, recommend MJPEG)
- AI toggle per camera (enable/disable)
- AI status indicator (processing, idle, error)

### Clips Page
- Free-text search bar: "red car", "person at door", "large brown dog"
- Results ranked by CLIP similarity with percentage match
- Each result shows the matched detection crop

### Live View
- Real-time bounding box overlay from Tier 1 detections
- Class labels with confidence: "Person 87%"
- Color-coded by class (blue=person, green=vehicle, amber=animal)
- Toggle on/off per camera

### Settings
- Global AI settings: enable/disable, confidence threshold
- Model info: show which models are loaded, inference device (CPU/CoreML/CUDA)
- Detection statistics: frames/sec processed, average inference time

## Performance Targets

On Apple Silicon (M1/M2/M3) with CoreML:
- YOLOv8n: ~10ms/frame → 100 FPS capacity
- CLIP: ~20ms/crop → 50 embeddings/sec
- Sub stream at 5fps: uses ~5% of inference capacity per camera

On modern x86 CPU:
- YOLOv8n: ~30ms/frame → 33 FPS capacity
- CLIP: ~50ms/crop → 20 embeddings/sec
- Sub stream at 5fps: uses ~15% of inference capacity per camera

Target: 10+ cameras with AI on a single modern machine.

## Configuration

No complex configuration. Per camera:
- `ai_enabled`: boolean toggle
- `sub_stream_url`: RTSP URL for the detection stream

Global (in Settings):
- AI confidence threshold (default 0.5)
- Whether to run CLIP embeddings (default: on)
