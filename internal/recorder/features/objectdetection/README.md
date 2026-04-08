# objectdetection

YOLO v8/v9 object detection feature pipeline for the Kaivue NVR.

This package implements **the feature**, not the inference backend. It
takes decoded video frames, runs them through the shared
`inference.Inferencer` seam from KAI-278, post-processes the raw YOLO
output tensor, and emits structured detection events via a
`DetectionEventSink`.

## Status

- Pipeline: complete
- Backends: depends on `internal/shared/inference`. Tested against the
  deterministic fake backend from `internal/shared/inference/fake`.
- Real YOLO weights + cgo ONNX Runtime / TensorRT / Core ML / DirectML
  bindings: **deferred to the cgo-bindings follow-up ticket**. Nothing in
  this package needs to change when that work lands; callers only swap
  the `inference.Inferencer` passed to `Detector.New`.

## Pipeline

```
  Frame
    |
    v
  Inferencer.Infer           (KAI-278 seam)
    |
    v
  decode YOLO tensor         [1, 4+C, N] float32
    |
    v
  confidence filter          camera override > detector default
    |
    v
  non-max suppression        per-class IoU threshold
    |
    v
  MaxDetectionsPerFrame cap  top-K by confidence
    |
    v
  class map resolution       per-vertical (Generic / RetailLP / Parking / Healthcare)
    |
    v
  class allowlist            per-camera
    |
    v
  ROI filter                 polygon overlap fraction
    |
    v
  min box area               per-camera
    |
    v
  cooldown dedup             (camera, class, spatial bucket) window
    |
    v
  DetectionEventSink.Publish
```

## Per-vertical class maps

Pick the `ClassMap` that matches the deployment vertical and pass it as
`Config.ClassMap`. Class ids are the dense ids emitted by the model's
classification head.

### Generic (COCO-ish)

| ID | Label      |
|----|------------|
| 0  | person     |
| 1  | bicycle    |
| 2  | car        |
| 3  | motorcycle |
| 4  | bus        |
| 5  | truck      |
| 6  | dog        |
| 7  | cat        |
| 8  | package    |
| 9  | bag        |

### Retail Loss Prevention

| ID | Label           |
|----|-----------------|
| 0  | person          |
| 1  | shopping_cart   |
| 2  | bag             |
| 3  | concealed_item  |

`concealed_item` is emitted by a specialised head that looks for items
being tucked into bags or clothing. That is one of the reasons the LP
vertical needs a distinct model from the generic one.

### Parking

| ID | Label         |
|----|---------------|
| 0  | car           |
| 1  | motorcycle    |
| 2  | truck         |
| 3  | bus           |
| 4  | license_plate |

`license_plate` detections feed into **KAI-282 (ALPR OCR)** as a
region-of-interest hint so that the OCR stage does not have to scan the
entire frame.

### Healthcare

| ID | Label       |
|----|-------------|
| 0  | person      |
| 1  | wheelchair  |
| 2  | stretcher   |
| 3  | fall_event  |

`fall_event` (id 3) is the behavioral-analytics integration hook: it is
consumed by **KAI-284 (behavioral analytics)** as the fall-detection
trigger. Keep this id reserved — KAI-284 hard-codes it.

## Configuration

### `Config` (detector-wide)

| Field                   | Purpose                                      |
|-------------------------|----------------------------------------------|
| `ModelID`               | Key resolved via `inference.ModelRegistry`   |
| `ConfidenceThreshold`   | Default score cutoff, `[0,1]`                |
| `NMSIoUThreshold`       | Duplicate-box IoU cutoff, `[0,1]`            |
| `MaxDetectionsPerFrame` | Top-K cap after NMS                          |
| `BackendHint`           | `edge` / `cloud` / `either` (router input)   |
| `ClassMap`              | Per-vertical id -> label                     |
| `CooldownBucketPixels`  | Spatial bucket size (default 64)             |
| `ROISamplesPerSide`     | Grid resolution for ROI overlap (default 5)  |
| `ROIOverlapThreshold`   | Min fraction of box inside any ROI           |

### `CameraDetectionConfig` (per camera)

| Field                 | Purpose                                  |
|-----------------------|------------------------------------------|
| `Enabled`             | Gate the entire pipeline for this camera |
| `ClassAllowlist`      | Drop classes not in this set             |
| `ConfidenceThreshold` | Override detector default                |
| `ROIs`                | Polygonal regions of interest            |
| `MinBoxArea`          | Drop boxes smaller than this pixel area  |
| `CooldownSeconds`     | Suppress repeats within this window      |

## Event emission

`DetectionEventSink` is the seam the recorder uses to wire detection
events into `DirectoryIngest.PublishAIEvents` (KAI-238 proto). This
package ships two local sinks:

- `InMemorySink` — collects events for tests and local inspection
- `LoggingSink` — publishes structured `slog` records via
  `internal/shared/logging` (stand-in until the recorder integration
  lands)

## Testing

Tests use the deterministic fake backend from
`internal/shared/inference/fake` via a thin `scriptedInferencer` wrapper
that returns synthetic YOLO output tensors for assertion.

```
go test -race ./internal/recorder/features/objectdetection/...
```

14 test cases cover the happy path, every filter stage, cooldown,
concurrency, per-vertical class maps, the sink envelope, disabled
cameras, model-not-found, and closed-detector behaviour.

## Handoff notes

- **Real YOLO weights** land with the cgo-bindings follow-up ticket.
  When that lands, callers pass the real `inference.Inferencer` to
  `Detector.New` and the pipeline is unchanged.
- **ALPR (KAI-282)** consumes `license_plate` detections from the
  Parking vertical as ROI hints.
- **Behavioral analytics (KAI-284)** consumes `fall_event` detections
  from the Healthcare vertical as its fall trigger.
- **Cross-camera tracking (KAI-285)** will populate `Detection.TrackID`.
  Until it ships, consumers MUST treat empty `TrackID` as "not tracked"
  rather than a shared identity.
