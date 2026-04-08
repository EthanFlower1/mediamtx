package inference

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// BackendKind identifies which inference engine an Inferencer is wrapping.
// Constants cover the four real backends we plan to ship plus the
// deterministic in-memory fake used by tests.
type BackendKind string

const (
	BackendONNXRuntime BackendKind = "onnxruntime"
	BackendTensorRT    BackendKind = "tensorrt"
	BackendCoreML      BackendKind = "coreml"
	BackendDirectML    BackendKind = "directml"
	BackendFake        BackendKind = "fake"
)

// IsReal reports whether the backend is a production engine (as opposed to
// the fake). Useful in assertions that want to avoid the fake in prod.
func (b BackendKind) IsReal() bool {
	switch b {
	case BackendONNXRuntime, BackendTensorRT, BackendCoreML, BackendDirectML:
		return true
	default:
		return false
	}
}

// DType is the element type of a Tensor. We only enumerate the types we
// actually plan to ship for NVR AI features. New dtypes require a protocol
// bump; do not reuse values.
type DType string

const (
	DTypeFloat32 DType = "float32"
	DTypeFloat16 DType = "float16"
	DTypeInt8    DType = "int8"
	DTypeUint8   DType = "uint8"
	DTypeInt32   DType = "int32"
	DTypeInt64   DType = "int64"
	DTypeBool    DType = "bool"
)

// ElementSize returns the byte width of a single element of the dtype.
// Float16 is reported as 2; callers are responsible for packing/unpacking.
func (d DType) ElementSize() int {
	switch d {
	case DTypeFloat32, DTypeInt32:
		return 4
	case DTypeFloat16:
		return 2
	case DTypeInt8, DTypeUint8, DTypeBool:
		return 1
	case DTypeInt64:
		return 8
	default:
		return 0
	}
}

// Tensor is a shape + dtype + byte buffer. Data is always little-endian and
// row-major (NCHW for images is the convention the NVR uses).
//
// The buffer length MUST equal the product of the Shape dimensions times
// the element size of DType. Validate via Tensor.Validate.
type Tensor struct {
	Name  string
	Shape []int
	DType DType
	Data  []byte
}

// NumElements returns the product of the Shape dimensions (1 for a scalar,
// 0 if any dimension is zero).
func (t Tensor) NumElements() int {
	if len(t.Shape) == 0 {
		return 1
	}
	n := 1
	for _, d := range t.Shape {
		if d < 0 {
			return -1
		}
		n *= d
	}
	return n
}

// ExpectedBytes returns the byte length Data must have for this tensor to
// be valid. Returns -1 for unknown dtypes or negative shape dims.
func (t Tensor) ExpectedBytes() int {
	n := t.NumElements()
	if n < 0 {
		return -1
	}
	sz := t.DType.ElementSize()
	if sz == 0 {
		return -1
	}
	return n * sz
}

// Validate reports whether the tensor is well-formed.
func (t Tensor) Validate() error {
	if t.DType == "" {
		return fmt.Errorf("inference: tensor %q: missing dtype", t.Name)
	}
	exp := t.ExpectedBytes()
	if exp < 0 {
		return fmt.Errorf("inference: tensor %q: invalid shape/dtype", t.Name)
	}
	if len(t.Data) != exp {
		return fmt.Errorf("inference: tensor %q: data length %d != expected %d", t.Name, len(t.Data), exp)
	}
	return nil
}

// LoadedModel is the opaque handle returned by LoadModel. Callers MUST
// treat the Handle field as opaque — only the issuing Inferencer knows
// how to interpret it.
type LoadedModel struct {
	// ID is the caller-supplied model identifier (typically a registry
	// key like "yolo-v8-s" or "clip-vit-b32"). Used in stats + logs.
	ID string

	// Backend is the BackendKind of the Inferencer that loaded this
	// model. Used by the Router for capacity decisions.
	Backend BackendKind

	// Version is the version string as reported by the backend
	// (e.g. "1.2.3" or a content hash). May be empty.
	Version string

	// LoadedAt is the wall-clock time at which LoadModel completed.
	LoadedAt time.Time

	// Handle is the backend-specific pointer/handle. Opaque to callers.
	Handle any
}

// InferenceResult is the output of a single Infer call.
type InferenceResult struct {
	// Outputs is the set of output tensors. The ordering and names are
	// determined by the model. For single-output models, the convention
	// is a single entry named "output".
	Outputs []Tensor

	// Latency is the wall-clock time the backend spent running the
	// inference (excluding any cross-process RPC overhead).
	Latency time.Duration

	// ModelID echoes the model id for convenience.
	ModelID string

	// Backend echoes the backend that served the inference.
	Backend BackendKind
}

// LatencyStats captures a P50/P95/P99 window for a single model.
// Implementations are free to use any reservoir/t-digest approach; for the
// fake backend we use a simple sorted-window approximation.
type LatencyStats struct {
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// ModelStats is the per-model rollup returned as part of Stats.
type ModelStats struct {
	ModelID        string
	InferenceCount uint64
	ErrorCount     uint64
	Latency        LatencyStats
}

// Stats is the aggregate telemetry view for an Inferencer.
type Stats struct {
	Backend       BackendKind
	ModelsLoaded  int
	TotalInferences uint64
	TotalErrors   uint64
	PerModel      map[string]ModelStats
}

// Inferencer is the edge/cloud inference runtime seam. Every AI feature
// in KAI-281..291 depends on this interface. Implementations wrap a
// specific backend (ONNX Runtime, TensorRT, Core ML, DirectML, or the
// in-memory fake).
//
// Implementations MUST be safe for concurrent Infer calls across multiple
// LoadedModel handles. They MAY serialise LoadModel and Unload.
type Inferencer interface {
	// Name returns a short human-readable identifier for the backend
	// instance, e.g. "onnx-cuda-0" or "fake-deterministic". It is used
	// in logs and Stats.
	Name() string

	// Backend returns the BackendKind this Inferencer is wrapping.
	Backend() BackendKind

	// LoadModel parses the raw model bytes (format is backend-specific
	// — .onnx, .plan, .mlmodel, .dml, etc.) and returns a LoadedModel
	// handle. The caller is responsible for calling Unload when done.
	//
	// If bytes is nil and the Inferencer has a ModelRegistry configured,
	// the implementation MAY resolve modelID via the registry.
	LoadModel(ctx context.Context, modelID string, bytes []byte) (*LoadedModel, error)

	// Infer runs a single forward pass. The input tensor(s) MUST match
	// the model's expected shape + dtype. Implementations MAY batch
	// internally but MUST return one InferenceResult per call.
	Infer(ctx context.Context, model *LoadedModel, input Tensor) (*InferenceResult, error)

	// Unload releases the backend resources associated with the model.
	// Idempotent: unloading an already-unloaded model returns nil.
	Unload(ctx context.Context, model *LoadedModel) error

	// Stats returns the current telemetry snapshot.
	Stats() Stats

	// Close releases ALL loaded models and tears down any backend
	// connections. After Close, any method call returns ErrClosed.
	Close() error
}

// ModelRegistry is consulted by LoadModel to resolve a model id to an
// approved version + bytes. The real implementation lands in KAI-279
// (Wave 3) and will be backed by pgvector with signed manifests and a
// rollout state machine. For now the interface is the seam.
type ModelRegistry interface {
	// Resolve returns the approved bytes + version for the given model
	// id. Returns ErrModelNotFound if no approved version exists.
	Resolve(ctx context.Context, modelID string) (bytes []byte, version string, err error)
}

// Sentinel errors. Implementations MUST return one of these (possibly
// wrapped) so that callers can switch on errors.Is.
var (
	// ErrClosed is returned by any method call after Close.
	ErrClosed = errors.New("inference: inferencer closed")

	// ErrModelNotFound is returned by LoadModel / the registry when the
	// model id is unknown.
	ErrModelNotFound = errors.New("inference: model not found")

	// ErrModelNotLoaded is returned by Infer / Unload when the LoadedModel
	// handle is unknown to this Inferencer (typically because it came
	// from a different backend).
	ErrModelNotLoaded = errors.New("inference: model not loaded")

	// ErrInvalidTensor is returned by Infer when the input tensor fails
	// validation (shape/dtype/byte length).
	ErrInvalidTensor = errors.New("inference: invalid tensor")

	// ErrBackendUnavailable is returned by LoadModel / Infer when the
	// backend has no free capacity (e.g. all GPUs busy) or when the
	// required hardware is absent.
	ErrBackendUnavailable = errors.New("inference: backend unavailable")

	// ErrUnsupportedFeature is returned by Router.Pick when no backend
	// can serve the requested feature under the provided hardware
	// capability.
	ErrUnsupportedFeature = errors.New("inference: unsupported feature for hardware")
)
