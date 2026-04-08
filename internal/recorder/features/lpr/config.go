package lpr

import "fmt"

// Config is the detector-wide configuration for the LPR feature.
// Per-camera enable/disable is handled by the lpr_enabled column on the
// cameras table (migration 0008_cameras_lpr_enabled).
type Config struct {
	// LocalisationModelID is the inference registry key for the plate-
	// localisation model (YOLO-style). Resolved by the Inferencer at startup.
	// Example: "lpr-localisation-v1".
	LocalisationModelID string

	// ReaderModelID is the inference registry key for the CRNN plate-reader
	// model. Example: "lpr-reader-crnn-v1".
	ReaderModelID string

	// ConfidenceThreshold is the minimum per-box confidence from the
	// localisation model. Range [0, 1]. Default 0.5.
	ConfidenceThreshold float64

	// ReaderConfidenceThreshold is the minimum CTC decoding confidence to
	// accept a read. Range [0, 1]. Default 0.6.
	ReaderConfidenceThreshold float64

	// MaxPlatesPerFrame caps how many plate candidates are processed per
	// frame. 0 means no cap (not recommended on high-traffic cameras).
	MaxPlatesPerFrame int
}

// Validate reports configuration errors.
func (c Config) Validate() error {
	if c.LocalisationModelID == "" {
		return fmt.Errorf("%w: LocalisationModelID is required", ErrInvalidConfig)
	}
	if c.ReaderModelID == "" {
		return fmt.Errorf("%w: ReaderModelID is required", ErrInvalidConfig)
	}
	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("%w: ConfidenceThreshold must be in [0,1]", ErrInvalidConfig)
	}
	if c.ReaderConfidenceThreshold < 0 || c.ReaderConfidenceThreshold > 1 {
		return fmt.Errorf("%w: ReaderConfidenceThreshold must be in [0,1]", ErrInvalidConfig)
	}
	return nil
}

// withDefaults returns a copy of the config with zero-valued fields set to
// production-safe defaults.
func (c Config) withDefaults() Config {
	if c.ConfidenceThreshold == 0 {
		c.ConfidenceThreshold = 0.5
	}
	if c.ReaderConfidenceThreshold == 0 {
		c.ReaderConfidenceThreshold = 0.6
	}
	if c.MaxPlatesPerFrame == 0 {
		c.MaxPlatesPerFrame = 5
	}
	return c
}

// CameraLPRConfig is the per-camera LPR configuration stored in the cameras
// table alongside lpr_enabled. Callers set Enabled=false to short-circuit
// the pipeline without destroying the config.
type CameraLPRConfig struct {
	// Enabled gates LPR on this camera. Maps to cameras.lpr_enabled.
	Enabled bool

	// ConfidenceOverride overrides Config.ConfidenceThreshold for this
	// camera. 0 means "inherit from Config".
	ConfidenceOverride float64
}
