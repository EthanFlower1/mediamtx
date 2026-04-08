// Package inference defines the edge/cloud inference runtime abstraction
// used by every AI feature in the NVR (object detection, face recognition,
// CLIP search, behavioral analysis, LPR, audio events, forensic search, …).
//
// The package intentionally contains only the Go seam — no cgo, no vendored
// ML libraries, no model files. Real backends (ONNX Runtime, TensorRT,
// Core ML, DirectML) land in follow-up tickets because they all require cgo
// and platform-specific vendor libraries that cannot be installed in the
// pure-Go NVR build used for CI. A deterministic in-memory "fake" backend
// ships in this package so that feature tickets can be implemented and
// tested without waiting for the real backends.
//
// Contract
//
// Every backend must implement Inferencer. The lifecycle of a model is:
//
//	handle, err := inf.LoadModel(ctx, "yolo-v8-s", bytes)
//	// ...
//	out, err := inf.Infer(ctx, handle, input)
//	// ...
//	_ = inf.Unload(ctx, handle)
//
// Inferencer implementations MUST be safe for concurrent Infer calls.
// Implementations MAY serialise LoadModel/Unload internally.
//
// Routing
//
// Router picks an Inferencer per request based on the requested feature and
// the caller-supplied HardwareCapability. The routing matrix is defined in
// §11.2 of the v1 roadmap spec and encoded in defaultFeatureRoutes below.
// Hardware capability is passed in by the caller; real probing (NVIDIA
// GPU / Jetson / Apple Silicon / DirectML adapters) lands in a later
// ticket.
//
// Model registry
//
// LoadModel may consult a ModelRegistry to resolve a model id to an
// approved version + bytes. The registry is an interface here; the real
// implementation (pgvector-backed, signed, with rollout controls) lands in
// KAI-279 as part of Wave 3.
package inference
