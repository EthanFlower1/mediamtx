# Embedded AI Analytics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add embedded AI object detection (YOLO) and semantic search (CLIP) to the NVR, running inference on every frame from each camera's sub stream via MediaMTX's internal reader pipeline.

**Architecture:** ONNX models are bundled in the binary via `go:embed`. Per-camera AI pipelines register as MediaMTX stream readers. Frames are decoded (MJPEG via `image/jpeg`, H.264 via `pion/mediadevices`), run through YOLOv8n for real-time detection, then CLIP for visual embeddings. Detections and embeddings are stored in SQLite. Semantic search computes cosine similarity in Go against stored embeddings.

**Tech Stack:** Go (yalue/onnxruntime_go, pion/mediadevices), ONNX models (YOLOv8n, YOLOv8m, CLIP ViT-B/32), SQLite, React + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-23-embedded-ai-analytics-design.md`

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `models/yolov8n.onnx` | Bundled YOLO nano model (~6MB) |
| `models/yolov8m.onnx` | Bundled YOLO medium model (~50MB) |
| `models/clip-vit-b32-visual.onnx` | Bundled CLIP visual encoder (~350MB) |
| `models/clip-vit-b32-text.onnx` | Bundled CLIP text encoder (~65MB) |
| `internal/nvr/ai/models.go` | go:embed for all ONNX model files |
| `internal/nvr/ai/detector.go` | YOLO inference wrapper (preprocess, run, postprocess NMS) |
| `internal/nvr/ai/detector_test.go` | Detector unit tests |
| `internal/nvr/ai/embedder.go` | CLIP inference wrapper (visual + text encoding) |
| `internal/nvr/ai/embedder_test.go` | Embedder unit tests |
| `internal/nvr/ai/decoder.go` | Frame decoder (MJPEG + H.264 → image.Image) |
| `internal/nvr/ai/pipeline.go` | Per-camera AI pipeline (reader → decode → detect → embed → store) |
| `internal/nvr/ai/search.go` | Semantic search (cosine similarity against stored embeddings) |
| `internal/nvr/api/search.go` | Search HTTP handler |
| `ui/src/components/AISearchBar.tsx` | Free-text search input for semantic search |
| `ui/src/components/DetectionOverlay.tsx` | Real-time bounding box overlay (replaces AnalyticsOverlay) |

### Modified files
| File | Change |
|------|--------|
| `go.mod` | Add yalue/onnxruntime_go, pion/mediadevices |
| `internal/nvr/db/migrations.go` | Migration v14: detections table, AI columns on cameras and motion_events |
| `internal/nvr/db/cameras.go` | Add sub_stream_url, ai_enabled fields |
| `internal/nvr/db/motion_events.go` | Add embedding, description fields |
| `internal/nvr/nvr.go` | Start/stop AI pipelines per camera |
| `internal/nvr/api/router.go` | Register search endpoint |
| `internal/nvr/api/cameras.go` | AI enable/disable, sub stream selection endpoints |
| `ui/src/hooks/useCameras.ts` | Add ai_enabled, sub_stream_url fields |
| `ui/src/pages/CameraManagement.tsx` | AI toggle, sub stream selector |
| `ui/src/pages/ClipSearch.tsx` | Semantic search bar integration |
| `ui/src/pages/LiveView.tsx` | Real-time detection overlay |
| `ui/src/pages/Settings.tsx` | AI settings panel |

---

### Task 1: Add dependencies and obtain ONNX models

**Files:**
- Modify: `go.mod`
- Create: `models/` directory
- Create: `internal/nvr/ai/models.go`

- [ ] **Step 1: Add Go dependencies**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
go get github.com/yalue/onnxruntime_go
go get github.com/pion/mediadevices
```

- [ ] **Step 2: Obtain ONNX models**

Download the pre-converted ONNX models:
- YOLOv8n: from Ultralytics GitHub releases or export via `ultralytics` Python package
- YOLOv8m: same source
- CLIP ViT-B/32: from Hugging Face ONNX exports

```bash
mkdir -p models
# YOLOv8n
curl -L -o models/yolov8n.onnx "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolov8n.onnx"
# YOLOv8m
curl -L -o models/yolov8m.onnx "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolov8m.onnx"
# CLIP visual encoder (needs export from Python — use a pre-exported version)
# CLIP text encoder (same)
```

Note: CLIP ONNX exports require a one-time Python conversion. Use `optimum` or `clip-as-service` to export. Store the exported files in `models/`.

- [ ] **Step 3: Create models.go with go:embed**

```go
// internal/nvr/ai/models.go
package ai

import _ "embed"

//go:embed ../../../../models/yolov8n.onnx
var YOLOv8nModel []byte

//go:embed ../../../../models/yolov8m.onnx
var YOLOv8mModel []byte

//go:embed ../../../../models/clip-vit-b32-visual.onnx
var CLIPVisualModel []byte

//go:embed ../../../../models/clip-vit-b32-text.onnx
var CLIPTextModel []byte
```

- [ ] **Step 4: Verify build**

```bash
go build ./internal/nvr/ai/...
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum models/ internal/nvr/ai/models.go
git commit -m "feat(nvr): add ONNX models and runtime dependency for AI analytics"
```

---

### Task 2: Database schema for detections and AI fields

**Files:**
- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/cameras.go`
- Modify: `internal/nvr/db/motion_events.go`
- Create: `internal/nvr/db/detections.go`
- Modify: `internal/nvr/db/db_test.go`
- Modify: `ui/src/hooks/useCameras.ts`

- [ ] **Step 1: Add migration v14**

```sql
ALTER TABLE cameras ADD COLUMN sub_stream_url TEXT DEFAULT '';
ALTER TABLE cameras ADD COLUMN ai_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE motion_events ADD COLUMN embedding BLOB;
ALTER TABLE motion_events ADD COLUMN description TEXT DEFAULT '';

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

- [ ] **Step 2: Update Camera struct and queries**

Add `SubStreamURL string` and `AIEnabled bool` to Camera struct. Update all SQL queries.

- [ ] **Step 3: Update MotionEvent struct**

Add `Embedding []byte` and `Description string` fields. Update queries.

- [ ] **Step 4: Create detections.go**

```go
type Detection struct {
    ID            int64   `json:"id"`
    MotionEventID int64   `json:"motion_event_id"`
    FrameTime     string  `json:"frame_time"`
    Class         string  `json:"class"`
    Confidence    float64 `json:"confidence"`
    BoxX          float64 `json:"box_x"`
    BoxY          float64 `json:"box_y"`
    BoxW          float64 `json:"box_w"`
    BoxH          float64 `json:"box_h"`
    Embedding     []byte  `json:"embedding,omitempty"`
    Attributes    string  `json:"attributes,omitempty"`
}

func (d *DB) InsertDetection(det *Detection) error
func (d *DB) ListDetectionsByEvent(eventID int64) ([]*Detection, error)
func (d *DB) ListDetectionsWithEmbeddings(cameraID string, start, end time.Time) ([]*Detection, error)
```

- [ ] **Step 5: Update TypeScript interfaces**

Add `sub_stream_url`, `ai_enabled` to Camera. Add `embedding`, `description` to MotionEvent (frontend won't use embedding directly but description is displayed).

- [ ] **Step 6: Test and commit**

```bash
go test ./internal/nvr/db/ -v -run TestOpen -count=1
git commit -m "feat(nvr): add detections table and AI fields for cameras and events"
```

---

### Task 3: YOLO detector wrapper

**Files:**
- Create: `internal/nvr/ai/detector.go`
- Create: `internal/nvr/ai/detector_test.go`

- [ ] **Step 1: Implement Detector struct**

```go
type Detection struct {
    Class      string
    Confidence float32
    X, Y, W, H float32 // normalized 0-1
}

type Detector struct {
    session *onnxruntime.Session
    inputW  int // 640 for YOLO
    inputH  int
    labels  []string // COCO class names
}

func NewDetector(modelData []byte) (*Detector, error)
func (d *Detector) Detect(img image.Image) ([]Detection, error)
func (d *Detector) Close()
```

The Detect method:
1. Resize image to 640x640
2. Convert to float32 tensor [1, 3, 640, 640] (CHW format, normalized 0-1)
3. Run ONNX inference
4. Parse output tensor: apply confidence threshold (0.5) and NMS
5. Map class indices to COCO labels
6. Return detections with normalized bounding boxes

YOLO output tensor shape: [1, 84, 8400] — 8400 candidate boxes, 84 values each (4 box coords + 80 class scores).

NMS (Non-Maximum Suppression): filter overlapping boxes with IoU > 0.45.

- [ ] **Step 2: Write tests**

Test with a sample image: load a JPEG, run detection, verify output format. Use the nano model for tests.

- [ ] **Step 3: Build and commit**

```bash
go test ./internal/nvr/ai/ -v -run TestDetector -count=1
git commit -m "feat(nvr): add YOLO detector wrapper with NMS postprocessing"
```

---

### Task 4: Frame decoder (MJPEG + H.264)

**Files:**
- Create: `internal/nvr/ai/decoder.go`

- [ ] **Step 1: Implement frame decoder**

```go
type FrameDecoder struct {
    codec string // "mjpeg" or "h264"
    // H.264 decoder state (pion/mediadevices)
}

func NewFrameDecoder(codec string) (*FrameDecoder, error)
func (d *FrameDecoder) Decode(data []byte) (image.Image, error)
func (d *FrameDecoder) Close()
```

For MJPEG: `image.Decode(bytes.NewReader(data))`
For H.264: use `pion/mediadevices` codec to decode NAL units to YUV, convert to RGBA.

- [ ] **Step 2: Build and commit**

```bash
go build ./internal/nvr/ai/...
git commit -m "feat(nvr): add frame decoder for MJPEG and H.264"
```

---

### Task 5: CLIP embedder wrapper

**Files:**
- Create: `internal/nvr/ai/embedder.go`
- Create: `internal/nvr/ai/embedder_test.go`

- [ ] **Step 1: Implement Embedder struct**

```go
type Embedder struct {
    visualSession *onnxruntime.Session
    textSession   *onnxruntime.Session
}

func NewEmbedder(visualModelData, textModelData []byte) (*Embedder, error)
func (e *Embedder) EncodeImage(img image.Image) ([]float32, error) // 512-dim vector
func (e *Embedder) EncodeText(text string) ([]float32, error)      // 512-dim vector
func (e *Embedder) Close()
```

EncodeImage:
1. Resize to 224x224
2. Normalize with CLIP mean/std ([0.48145466, 0.4578275, 0.40821073], [0.26862954, 0.26130258, 0.27577711])
3. Convert to float32 tensor [1, 3, 224, 224]
4. Run visual ONNX session
5. L2-normalize output vector

EncodeText:
1. Tokenize text (CLIP BPE tokenizer — need to implement or use a Go port)
2. Pad/truncate to 77 tokens
3. Run text ONNX session
4. L2-normalize output vector

Note: CLIP tokenization in Go is non-trivial. Use a pre-computed vocabulary file or port the BPE tokenizer. Alternatively, use a simple word-level tokenizer as approximation for v1.

- [ ] **Step 2: Write tests**

Test image encoding produces a 512-dim vector. Test text encoding produces a 512-dim vector. Test cosine similarity between "a photo of a dog" and a dog image is > 0.2.

- [ ] **Step 3: Build and commit**

```bash
go test ./internal/nvr/ai/ -v -run TestEmbedder -count=1
git commit -m "feat(nvr): add CLIP embedder with visual and text encoding"
```

---

### Task 6: AI pipeline (per-camera frame processing)

**Files:**
- Create: `internal/nvr/ai/pipeline.go`
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Implement AIPipeline struct**

```go
type AIPipeline struct {
    camera    *db.Camera
    detector  *Detector    // YOLOv8n
    embedder  *Embedder    // CLIP
    decoder   *FrameDecoder
    db        *db.DB
    eventPub  EventPublisher
    stopCh    chan struct{}
}

func NewAIPipeline(cam *db.Camera, detector *Detector, embedder *Embedder, database *db.DB, pub EventPublisher) *AIPipeline

// ProcessFrame is the callback registered with MediaMTX's stream.Reader.
// It receives encoded frame data, decodes it, runs inference, and stores results.
func (p *AIPipeline) ProcessFrame(frameData []byte, timestamp time.Time) error

func (p *AIPipeline) Stop()
```

ProcessFrame flow:
1. Decode frame data → `image.Image`
2. Run YOLOv8n → list of detections
3. Filter by confidence threshold
4. For each detection:
   a. Crop bounding box from original image
   b. Run CLIP visual encoder → 512-dim embedding
   c. Store detection in DB
5. If any person/vehicle/animal detected and no open motion event:
   a. Create motion event with object_class from top detection
   b. Store event embedding from top detection's CLIP vector
   c. Capture snapshot and set thumbnail_path
   d. Publish notification

- [ ] **Step 2: Wire into NVR lifecycle**

In `nvr.go`:
- Create shared Detector and Embedder instances on startup (load models once)
- For each camera with `ai_enabled && sub_stream_url != ""`, create AIPipeline
- Register pipeline as a MediaMTX stream reader on the sub stream path
- Stop all pipelines on NVR shutdown

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add per-camera AI pipeline with detection and embedding"
```

---

### Task 7: Semantic search API

**Files:**
- Create: `internal/nvr/ai/search.go`
- Create: `internal/nvr/api/search.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Implement search logic**

```go
// internal/nvr/ai/search.go

type SearchResult struct {
    Detection  *db.Detection
    Event      *db.MotionEvent
    Similarity float64
    CameraName string
}

func Search(embedder *Embedder, db *db.DB, query string, cameraID string, start, end time.Time, limit int) ([]SearchResult, error) {
    // 1. Encode query text with CLIP
    // 2. Fetch detections with embeddings from DB (filtered by camera/date)
    // 3. Compute cosine similarity for each
    // 4. Sort by similarity descending
    // 5. Return top N
}

func CosineSimilarity(a, b []float32) float64
```

- [ ] **Step 2: Add HTTP handler**

```go
// GET /api/nvr/search?q=...&camera_id=...&start=...&end=...&limit=20
```

Register in router.go.

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add semantic search API with CLIP text-to-image matching"
```

---

### Task 8: Camera AI configuration API and UI

**Files:**
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`
- Modify: `ui/src/pages/CameraManagement.tsx`
- Modify: `ui/src/hooks/useCameras.ts`

- [ ] **Step 1: Add API endpoints**

```
PUT /cameras/:id/ai    body: {"ai_enabled": true, "sub_stream_url": "rtsp://..."}
```

- [ ] **Step 2: Update camera management UI**

In the expanded camera detail panel, add:
- Sub stream profile selector (show all profiles from ONVIF probe, recommend MJPEG)
- AI toggle switch
- AI status indicator (active/disabled/error)
- Help text: "Select a sub stream for AI detection. MJPEG is recommended for best performance."

- [ ] **Step 3: Build and commit**

```bash
cd ui && npm run build && cd ..
git commit -m "feat(nvr): add AI configuration UI with sub stream selection"
```

---

### Task 9: Search UI and detection overlay

**Files:**
- Create: `ui/src/components/AISearchBar.tsx`
- Create: `ui/src/components/DetectionOverlay.tsx`
- Modify: `ui/src/pages/ClipSearch.tsx`
- Modify: `ui/src/pages/LiveView.tsx`
- Modify: `ui/src/pages/Settings.tsx`

- [ ] **Step 1: Create AISearchBar**

Free-text search input with:
- Text input field with search icon
- Camera filter dropdown (optional)
- Date range picker (optional)
- Results display as cards with similarity score percentage
- Each result shows the detection crop, class label, timestamp, camera name

Wire to `GET /api/nvr/search?q=...`

- [ ] **Step 2: Integrate into ClipSearch page**

Add AISearchBar at the top of the Clips page, above the existing event-type filter search. When a semantic search is active, show results ranked by similarity instead of chronologically.

- [ ] **Step 3: Create DetectionOverlay**

Real-time bounding box overlay for live view:
- WebSocket connection to receive detections from the AI pipeline
- Canvas overlay rendering boxes with class labels and confidence
- Color-coded: blue=person, green=vehicle, amber=animal, red=other
- Toggle button in camera modal

Note: For v1, poll the detections API every 500ms instead of WebSocket. Add WebSocket streaming in a future iteration.

- [ ] **Step 4: Add AI settings to Settings page**

New "AI Analytics" tab:
- Global AI enable/disable
- Confidence threshold slider (0.1 - 0.9, default 0.5)
- Model info: loaded models, inference device
- Stats: frames/sec, avg inference time, total detections

- [ ] **Step 5: Build and commit**

```bash
cd ui && npm run build && cd ..
git commit -m "feat(nvr): add semantic search UI, detection overlay, and AI settings"
```

---

### Task 10: Tier 2 refinement and integration test

**Files:**
- Modify: `internal/nvr/ai/pipeline.go`

- [ ] **Step 1: Add Tier 2 refinement**

After a motion event ends:
1. Load the event's snapshot
2. Run YOLOv8m on it
3. Update the event's `object_class` and `confidence` with the refined result
4. Generate a text description from all detections (e.g., "1 person, 1 vehicle near entrance")
5. Store in `motion_events.description`

Trigger: when the scheduler calls `EndMotionEvent`, if AI is enabled for that camera, queue a Tier 2 refinement job.

- [ ] **Step 2: Run full integration test**

```bash
go test ./internal/nvr/... -count=1
cd ui && npm run build
go run .
```

Verify:
1. AI pipeline starts for enabled cameras
2. Detections appear in real-time on live view
3. Events get refined classification after ending
4. Semantic search returns relevant results
5. Clips page shows AI-detected events with class labels

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(nvr): add Tier 2 refinement and complete AI analytics integration"
```
