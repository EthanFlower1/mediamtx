// Package clip implements the CLIP edge embedding pipeline (KAI-479).
//
// The pipeline computes per-frame CLIP image embeddings on the Recorder at a
// configurable sample interval (default 1 frame/second) and ships them to the
// cloud via DirectoryIngest.PublishAIEvents. Embeddings are stored as base64-
// encoded float32 vectors in the AIEvent.Attributes map, enabling downstream
// forensic search (KAI-288) without requiring cloud-side inference.
//
// Key design decisions:
//
//   - Model loading uses the shared inference.Inferencer abstraction (KAI-278),
//     supporting ONNX Runtime, TensorRT, CoreML, DirectML, and the fake backend.
//   - Per-camera enable/disable via CameraConfig.Enabled.
//   - Resource budgeting via a configurable GPU/CPU share semaphore that applies
//     backpressure when the inference budget is exhausted, ensuring the video
//     pipeline is never starved.
//   - The pipeline is edge-only: embeddings are computed locally and published
//     as AI events. No cloud inference fallback.
package clip
