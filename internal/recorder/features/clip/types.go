package clip

import (
	"errors"
	"time"
)

// Embedding is a single CLIP image embedding produced by the pipeline.
type Embedding struct {
	// CameraID identifies the source camera.
	CameraID string

	// CapturedAt is the wall-clock timestamp of the frame that produced
	// this embedding.
	CapturedAt time.Time

	// Vector is the L2-normalised CLIP embedding. For ViT-B/32 this is
	// 512 float32 values; for ViT-L/14 it is 768.
	Vector []float32

	// ModelID echoes the model that produced the embedding.
	ModelID string

	// Latency is the wall-clock inference time for this embedding.
	Latency time.Duration
}

// Frame is a single decoded video frame ready for CLIP inference.
type Frame struct {
	// CameraID identifies the source camera.
	CameraID string

	// CapturedAt is the wall-clock time the frame was captured.
	CapturedAt time.Time

	// Width, Height are the original frame dimensions in pixels.
	Width, Height int

	// Tensor is the preprocessed input tensor for the CLIP image encoder.
	// Expected shape: [1, 3, 224, 224] float32 for ViT-B/32.
	Tensor InferenceInput
}

// InferenceInput is the local alias for inference tensor data, kept to avoid
// leaking the inference package into callers that only assemble frames.
type InferenceInput struct {
	Name  string
	Shape []int
	DType string
	Data  []byte
}

// Sentinel errors for the CLIP pipeline.
var (
	// ErrClosed is returned after Close has been called.
	ErrClosed = errors.New("clip: pipeline closed")

	// ErrInvalidConfig is returned when Config validation fails.
	ErrInvalidConfig = errors.New("clip: invalid config")

	// ErrModelNotFound is returned when the CLIP model cannot be resolved.
	ErrModelNotFound = errors.New("clip: model not found")

	// ErrInvalidFrame is returned when a frame cannot be processed.
	ErrInvalidFrame = errors.New("clip: invalid frame")

	// ErrResourceBudgetExhausted is returned when the inference budget
	// semaphore cannot be acquired within the timeout.
	ErrResourceBudgetExhausted = errors.New("clip: resource budget exhausted")
)
