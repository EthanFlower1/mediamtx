// Package objectdetection implements the object detection feature pipeline
// for the Kaivue NVR. It consumes video frames, runs them through the
// shared inference.Inferencer seam (KAI-278), post-processes raw YOLO-style
// output tensors, and emits structured detection events.
//
// This package is deliberately backend-agnostic: it operates against the
// inference.Inferencer interface and is tested against the deterministic
// fake backend. Real YOLO v8/v9 weights and cgo ONNX/TensorRT/CoreML
// bindings land with the follow-up cgo-bindings ticket. When that work
// arrives, nothing in this package needs to change — only the Inferencer
// passed to Detector.New.
//
// The pipeline stages, in order:
//
//  1. Inference          — Inferencer.Infer on the provided Frame tensor
//  2. Tensor decode      — extract [class, cx, cy, w, h, score] rows
//  3. Confidence filter  — drop rows below the effective threshold
//  4. NMS                — non-max suppression per class (IoU threshold)
//  5. Class allowlist    — drop classes not permitted on this camera
//  6. ROI filter         — drop boxes not sufficiently inside any ROI
//  7. Min box area       — drop boxes smaller than the configured area
//  8. Cooldown dedup     — suppress repeats of the same class+spatial bucket
//  9. Event emission     — publish via the DetectionEventSink
//
// Per-vertical class maps live in classes.go. Callers pick the map that
// matches the deployment vertical (Generic, RetailLP, Parking, Healthcare)
// and pass it via Config.ClassMap.
package objectdetection
