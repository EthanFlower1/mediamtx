# `internal/shared/inference`

Edge/cloud inference runtime seam for the MediaMTX NVR. Every AI feature
(object detection, face recognition, CLIP search, behavioral analysis,
license plate recognition, audio events, forensic search) depends on this
package. It is deliberately small, pure-Go, and has no cgo.

Ticket: **KAI-278** — Edge inference runtime: Inferencer Go interface
abstraction (ONNX Runtime, TensorRT, Core ML, DirectML).

## What's in this package

- `Inferencer` — the core interface every backend implements.
- `Tensor`, `LoadedModel`, `InferenceResult`, `Stats`, `LatencyStats`,
  `ModelStats`, `DType`, `BackendKind` — the supporting value types.
- `ModelRegistry` — interface stub. The real implementation (pgvector,
  signed manifests, rollout state machine) lands in **KAI-279**.
- `Router` — selects an `Inferencer` per request based on feature +
  hardware capability.
- `fake/` — deterministic in-memory implementation used by tests and
  feature tickets before the real backends land.

## Interface contract

```go
type Inferencer interface {
    Name() string
    Backend() BackendKind
    LoadModel(ctx context.Context, modelID string, bytes []byte) (*LoadedModel, error)
    Infer(ctx context.Context, model *LoadedModel, input Tensor) (*InferenceResult, error)
    Unload(ctx context.Context, model *LoadedModel) error
    Stats() Stats
    Close() error
}
```

Invariants every backend MUST preserve:

- `Infer` is safe for concurrent calls across multiple loaded models.
  `LoadModel` and `Unload` MAY be serialised internally.
- `LoadedModel.Handle` is opaque to callers; only the issuing
  `Inferencer` interprets it. Handles from one backend passed to another
  MUST return `ErrModelNotLoaded`.
- `Unload` is idempotent. `Close` releases all loaded models and all
  further calls return `ErrClosed`.
- Input tensors are validated (shape × dtype element size == data length);
  bad tensors return `ErrInvalidTensor`.
- Stats are cumulative per-instance and MUST track: models loaded, total
  inferences, total errors, and P50/P95/P99 latency per model.

Sentinel errors (see `types.go`):

- `ErrClosed`, `ErrModelNotFound`, `ErrModelNotLoaded`, `ErrInvalidTensor`,
  `ErrBackendUnavailable`, `ErrUnsupportedFeature`.

## Routing matrix

The `Router` picks a location + backend per feature based on a
caller-supplied `HardwareCapability`. Real hardware probing (GPU/NPU
detection) is a later ticket — for now the caller fills in the struct.

| Feature                        | Preferred  | Requires GPU/NPU | Cloud fallback | Preferred backends (in order)                              |
|--------------------------------|------------|------------------|----------------|------------------------------------------------------------|
| lightweight_object_detection   | edge       | no               | yes            | CoreML, DirectML, ONNXRuntime, TensorRT                    |
| heavy_object_detection         | edge       | yes              | yes            | TensorRT, ONNXRuntime, DirectML, CoreML                    |
| face_recognition               | edge       | yes              | yes            | TensorRT, ONNXRuntime, CoreML, DirectML                    |
| license_plate_recognition      | edge       | no               | yes            | ONNXRuntime, CoreML, DirectML, TensorRT                    |
| behavioral_analysis            | edge       | yes              | yes            | TensorRT, ONNXRuntime                                      |
| audio_event_detection          | edge       | no               | yes            | ONNXRuntime, CoreML, DirectML                              |
| clip_embedding                 | cloud      | yes              | yes            | TensorRT, ONNXRuntime                                      |
| forensic_search                | cloud      | yes              | yes            | TensorRT, ONNXRuntime                                      |

Decision logic (see `router.go`):

1. If the feature prefers edge **and** it requires GPU/NPU but the hardware
   has neither: return cloud (`cloud:fallback:no-gpu`) if fallback is
   allowed, else `ErrUnsupportedFeature`.
2. If the feature prefers edge and a preferred backend is registered and
   supported by hardware: return edge (`edge:preferred`).
3. If the feature prefers edge but no backend is registered/supported:
   fall back to cloud (`cloud:fallback:no-edge-backend`) if allowed.
4. If the feature prefers cloud but the hardware happens to support a
   preferred backend **and** GPU is not required: opportunistically
   return edge (`edge:opportunistic`).
5. Otherwise cloud (`cloud:preferred`).

Every decision is logged at `DEBUG` via `slog.Default()` (override with
`WithLogger`). Operators can trace why a request went where it did by
grepping `inference routing decision` in logs.

Customise the matrix per deployment by copying `DefaultFeaturePolicies()`,
mutating it, and passing the result to `NewRouter(WithFeaturePolicies(...))`.

## Adding a new backend

Each real backend will be its own ticket because they all need cgo +
vendor libraries that aren't in the pure-Go NVR build:

- **ONNX Runtime** — planned under a follow-up to KAI-278. Wrap
  `onnxruntime_go` (or the raw C API) in a new package
  `internal/shared/inference/onnx/` implementing `Inferencer`. Register
  backend kind `BackendONNXRuntime`.
- **TensorRT** — NVIDIA-only path. Package
  `internal/shared/inference/tensorrt/`. Backend kind `BackendTensorRT`.
  Will need engine-plan serialisation (`.plan` files) and a DLA fallback
  for Jetson.
- **Core ML** — Apple-only. Package
  `internal/shared/inference/coreml/`. Backend kind `BackendCoreML`.
  Wraps `MLModel` via an Objective-C shim.
- **DirectML** — Windows-only. Package
  `internal/shared/inference/directml/`. Backend kind `BackendDirectML`.
  Wraps the DirectML C API through a small cgo shim.

Checklist for any new backend:

1. New package under `internal/shared/inference/<backend>/`.
2. Add a `BackendKind` constant in `types.go` if not already present.
3. Implement all eight methods of `Inferencer`. Use the `fake` package
   as a reference for stats bookkeeping and concurrency discipline.
4. Return the package sentinel errors — never roll your own.
5. Unit tests using tiny fake models (no real model files in the repo).
6. Register the backend with `Router.RegisterEdge` at NVR startup.
7. Update the routing matrix table in this README if the feature set
   changes.

## Model registry integration

`LoadModel` accepts raw bytes OR (when bytes is nil) may consult a
`ModelRegistry`:

```go
type ModelRegistry interface {
    Resolve(ctx context.Context, modelID string) (bytes []byte, version string, err error)
}
```

The real implementation lands in **KAI-279** (Wave 3) and will:

- Store manifests in Postgres + pgvector.
- Require a signed bundle for every approved model version.
- Expose a rollout state machine (candidate → canary → GA → retired).
- Gate `Resolve` behind tenant + site scope (no cross-tenant model leak).

Feature tickets in Wave 2+ can already wire the seam today: pass a
`fake.WithRegistry(fakeRegistry)` in tests and swap in the real registry
when KAI-279 lands — no call-site changes required.

## Scope of KAI-278 (what ships here)

- The interface, value types, sentinel errors, routing matrix, registry
  seam, and a deterministic fake implementation.
- Tests covering: load/infer/unload round trip, deterministic
  reproducibility across runs, stats accounting, concurrent safety,
  close lifecycle, context cancellation, and the full router matrix.

Explicitly **out of scope**:

- Any real ML runtime bindings (cgo).
- Hardware probing.
- Real model files or model downloads.
- The pgvector-backed registry (KAI-279).
- The cross-site inference routing engine (KAI-280).
