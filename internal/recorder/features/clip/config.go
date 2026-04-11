package clip

import (
	"fmt"
	"time"
)

// DefaultSampleInterval is the default frame sample rate: 1 frame per second.
const DefaultSampleInterval = 1 * time.Second

// DefaultEmbeddingDim is the output dimension for ViT-B/32.
const DefaultEmbeddingDim = 512

// DefaultMaxConcurrentInferences is the default number of concurrent inference
// slots (resource budget). This caps how many frames can be in the inference
// pipeline simultaneously across all cameras.
const DefaultMaxConcurrentInferences = 2

// DefaultBudgetTimeout is the maximum time to wait for an inference slot
// before dropping a frame.
const DefaultBudgetTimeout = 500 * time.Millisecond

// Config is the pipeline-wide configuration for the CLIP embedding feature.
type Config struct {
	// ModelID is the key resolved against the inference.ModelRegistry.
	// Defaults to "clip-vit-b32".
	ModelID string

	// EmbeddingDim is the expected output vector dimension. Must match the
	// model. Defaults to 512 (ViT-B/32).
	EmbeddingDim int

	// SampleInterval is the minimum time between frames submitted for
	// embedding per camera. Defaults to 1s.
	SampleInterval time.Duration

	// MaxConcurrentInferences is the GPU/CPU share budget: the maximum
	// number of inference calls running concurrently across all cameras.
	// This prevents the CLIP pipeline from starving the video pipeline.
	// Defaults to 2.
	MaxConcurrentInferences int

	// BudgetTimeout is how long to wait for an inference slot before
	// dropping the frame. Defaults to 500ms.
	BudgetTimeout time.Duration
}

// Validate checks the config for obvious errors.
func (c Config) Validate() error {
	if c.ModelID == "" {
		return fmt.Errorf("%w: model id is required", ErrInvalidConfig)
	}
	if c.EmbeddingDim <= 0 {
		return fmt.Errorf("%w: embedding dim must be positive", ErrInvalidConfig)
	}
	if c.SampleInterval < 0 {
		return fmt.Errorf("%w: sample interval must be non-negative", ErrInvalidConfig)
	}
	if c.MaxConcurrentInferences <= 0 {
		return fmt.Errorf("%w: max concurrent inferences must be positive", ErrInvalidConfig)
	}
	if c.BudgetTimeout < 0 {
		return fmt.Errorf("%w: budget timeout must be non-negative", ErrInvalidConfig)
	}
	return nil
}

// withDefaults returns a copy with unset fields filled in.
func (c Config) withDefaults() Config {
	if c.ModelID == "" {
		c.ModelID = "clip-vit-b32"
	}
	if c.EmbeddingDim == 0 {
		c.EmbeddingDim = DefaultEmbeddingDim
	}
	if c.SampleInterval == 0 {
		c.SampleInterval = DefaultSampleInterval
	}
	if c.MaxConcurrentInferences == 0 {
		c.MaxConcurrentInferences = DefaultMaxConcurrentInferences
	}
	if c.BudgetTimeout == 0 {
		c.BudgetTimeout = DefaultBudgetTimeout
	}
	return c
}

// CameraConfig is the per-camera configuration for the CLIP pipeline.
type CameraConfig struct {
	// Enabled gates the pipeline for this camera. When false, ProcessFrame
	// returns immediately without running inference.
	Enabled bool

	// SampleInterval overrides Config.SampleInterval for this camera.
	// Zero means "inherit from pipeline config".
	SampleInterval time.Duration
}

// effectiveSampleInterval returns the sample interval to use, preferring
// the per-camera override when set.
func (c CameraConfig) effectiveSampleInterval(pipelineDefault time.Duration) time.Duration {
	if c.SampleInterval > 0 {
		return c.SampleInterval
	}
	return pipelineDefault
}
