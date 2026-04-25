//go:build !cgo

package ai

import (
	"image"
)

// Embedder is a no-op stub in the no-cgo build. The cgo version wraps CLIP
// ONNX sessions via github.com/yalue/onnxruntime_go; in CGO_ENABLED=0
// builds we keep the type so that callers of search.go, publisher.go, and
// pipeline.go compile, but NewEmbedder always errors so an Embedder value
// never actually exists at runtime.
type Embedder struct{}

// NewEmbedder returns ErrCGORequired.
func NewEmbedder(visualModelPath, textModelPath, vocabPath string, projectionPath ...string) (*Embedder, error) {
	return nil, ErrCGORequired
}

// EncodeImage returns ErrCGORequired. Unreachable in practice because
// NewEmbedder cannot succeed in this build.
func (e *Embedder) EncodeImage(img image.Image) ([]float32, error) {
	return nil, ErrCGORequired
}

// EncodeText returns ErrCGORequired.
func (e *Embedder) EncodeText(text string) ([]float32, error) {
	return nil, ErrCGORequired
}

// ProjectVisual returns nil.
func (e *Embedder) ProjectVisual(visual []float32) []float32 { return nil }

// VisualDim returns 0.
func (e *Embedder) VisualDim() int { return 0 }

// TextDim returns 0.
func (e *Embedder) TextDim() int { return 0 }

// Close is a no-op.
func (e *Embedder) Close() {}
