//go:build !cgo

package ai

import (
	"errors"
	"image"
)

// ErrCGORequired is returned by every function in the no-cgo build of this
// package. The ONNX Runtime bindings (github.com/yalue/onnxruntime_go) use
// cgo, so when the binary is built with CGO_ENABLED=0 the detector and
// embedder become no-op stubs that fail fast at call time rather than
// failing to compile. Callers that want to cross-compile to platforms
// where we do not ship ONNX Runtime (e.g. Windows, arm64 macOS without the
// dylib installed) can do so by setting CGO_ENABLED=0; they just lose the
// AI features at runtime.
var ErrCGORequired = errors.New("internal/nvr/ai: CGO required for ONNX Runtime")

// InitONNXRuntime is a no-op in the no-cgo build. It returns nil so that
// Recorder startup does not fail on non-cgo targets; callers that then try
// to construct a Detector or Embedder will get ErrCGORequired at that
// point, which is the correct layer to surface the limitation.
func InitONNXRuntime() error { return nil }

// ShutdownONNXRuntime is a no-op in the no-cgo build. The cgo version
// destroys the ONNX Runtime environment; here there is nothing to tear
// down since InitONNXRuntime never allocated anything.
func ShutdownONNXRuntime() error { return nil }

// YOLODetection mirrors the cgo-build definition byte-for-byte so any code
// compiled in the no-cgo build that transitively references it via
// Detector.Detect compiles. The fields are documented on the cgo version.
type YOLODetection struct {
	Class      int     `json:"class"`
	ClassName  string  `json:"class_name"`
	Confidence float32 `json:"confidence"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	W          float32 `json:"w"`
	H          float32 `json:"h"`
}

// Detector is a no-op stub in the no-cgo build. All methods return
// ErrCGORequired (or zero values). It is never actually constructable —
// NewDetector always errors — so the fields are deliberately empty.
type Detector struct{}

// NewDetector returns ErrCGORequired. The real implementation lives in
// detector.go behind //go:build cgo.
func NewDetector(modelPath string) (*Detector, error) {
	return nil, ErrCGORequired
}

// Detect returns ErrCGORequired. Unreachable in practice because
// NewDetector cannot return a non-nil Detector in this build, but kept
// here so that consumers (pipeline.go, model_manager.go) compile.
func (d *Detector) Detect(img image.Image, confThreshold float32) ([]YOLODetection, error) {
	return nil, ErrCGORequired
}

// Close is a no-op.
func (d *Detector) Close() {}

// Labels returns nil.
func (d *Detector) Labels() []string { return nil }
