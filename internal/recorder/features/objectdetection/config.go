package objectdetection

import (
	"fmt"
	"time"
)

// BackendHint tells the Router which kind of Inferencer the caller
// prefers. The Router may fall back to another backend if the hint is
// unavailable. Currently informational — KAI-278's router consults it but
// nothing in this package hard-depends on it.
type BackendHint string

const (
	// BackendHintEdge prefers an on-device backend (CoreML, TensorRT on
	// a local GPU, DirectML, etc.). Lower latency, no network dependency.
	BackendHintEdge BackendHint = "edge"
	// BackendHintCloud prefers a cloud backend. Used for heavier models
	// or when the edge device is thermally constrained.
	BackendHintCloud BackendHint = "cloud"
	// BackendHintEither lets the Router pick freely.
	BackendHintEither BackendHint = "either"
)

// Config is the detector-wide configuration. One Detector instance is
// created per (model, camera-group) pairing; per-camera behavioural knobs
// live on CameraDetectionConfig.
type Config struct {
	// ModelID is the key resolved against the inference.ModelRegistry.
	// The bytes are loaded via Inferencer.LoadModel(ctx, ModelID, nil).
	ModelID string

	// ConfidenceThreshold is the default score cutoff applied before NMS.
	// Cameras may override this per-camera. Range [0, 1].
	ConfidenceThreshold float64

	// NMSIoUThreshold is the IoU above which two boxes of the same class
	// are considered duplicates during non-max suppression. Range [0, 1].
	NMSIoUThreshold float64

	// MaxDetectionsPerFrame caps the number of boxes the pipeline will
	// keep after NMS. 0 means "no cap".
	MaxDetectionsPerFrame int

	// BackendHint is a preference passed to the router. The detector
	// itself does not consult it — it is surfaced so sinks and telemetry
	// can record the operator's intent.
	BackendHint BackendHint

	// ClassMap is the vertical-specific id→label map. Required. Pick
	// one of GenericClasses, RetailLPClasses, ParkingClasses, or
	// HealthcareClasses.
	ClassMap ClassMap

	// CooldownBucketPixels is the size of the spatial grid cell used to
	// bucket detections for cooldown deduplication. Larger values
	// collapse more boxes to the same bucket. 0 defaults to 64.
	CooldownBucketPixels int

	// ROISamplesPerSide controls the grid resolution used when computing
	// the fraction of a box that lies inside an ROI polygon. 0 defaults
	// to 5 (25 samples per box).
	ROISamplesPerSide int

	// ROIOverlapThreshold is the minimum fraction of a detection box
	// that must fall inside an ROI polygon for the detection to pass
	// the ROI filter. Range [0, 1]. 0 means "any overlap is fine".
	// A camera with no ROIs configured bypasses this filter entirely.
	ROIOverlapThreshold float64
}

// Validate reports whether the config is well-formed.
func (c Config) Validate() error {
	if c.ModelID == "" {
		return fmt.Errorf("%w: model id is required", ErrInvalidConfig)
	}
	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("%w: confidence threshold out of range", ErrInvalidConfig)
	}
	if c.NMSIoUThreshold < 0 || c.NMSIoUThreshold > 1 {
		return fmt.Errorf("%w: nms iou threshold out of range", ErrInvalidConfig)
	}
	if c.ROIOverlapThreshold < 0 || c.ROIOverlapThreshold > 1 {
		return fmt.Errorf("%w: roi overlap threshold out of range", ErrInvalidConfig)
	}
	if len(c.ClassMap) == 0 {
		return fmt.Errorf("%w: class map is required", ErrInvalidConfig)
	}
	return nil
}

// withDefaults returns a copy of the config with unset fields filled in.
func (c Config) withDefaults() Config {
	if c.CooldownBucketPixels == 0 {
		c.CooldownBucketPixels = 64
	}
	if c.ROISamplesPerSide == 0 {
		c.ROISamplesPerSide = 5
	}
	return c
}

// CameraDetectionConfig is the per-camera override bundle. All fields are
// optional — a zero value (with Enabled=true) means "use the detector
// defaults and no filtering".
type CameraDetectionConfig struct {
	// Enabled gates the entire pipeline for this camera. When false,
	// ProcessFrame returns an empty slice without running inference.
	Enabled bool

	// ClassAllowlist restricts which classes emit events. A nil or
	// empty list means "all classes permitted by the detector's ClassMap".
	ClassAllowlist []string

	// ConfidenceThreshold overrides Config.ConfidenceThreshold for this
	// camera. Zero means "inherit".
	ConfidenceThreshold float64

	// ROIs is the set of regions inside which detections are kept.
	// Empty means "no ROI filter" (the entire frame is valid).
	ROIs []Polygon

	// MinBoxArea is the minimum pixel area a detection must occupy to
	// pass. 0 means no minimum.
	MinBoxArea float64

	// CooldownSeconds suppresses repeats of the same class + spatial
	// bucket within this window. 0 disables cooldown.
	CooldownSeconds int
}

// effectiveConfidence returns the confidence threshold to apply for this
// camera, preferring the per-camera override when set.
func (c CameraDetectionConfig) effectiveConfidence(detectorDefault float64) float64 {
	if c.ConfidenceThreshold > 0 {
		return c.ConfidenceThreshold
	}
	return detectorDefault
}

// cooldownDuration returns the per-camera cooldown as a Duration.
func (c CameraDetectionConfig) cooldownDuration() time.Duration {
	return time.Duration(c.CooldownSeconds) * time.Second
}

// allowlistSet returns the allowlist as a set for O(1) lookup, or nil if
// the camera permits all classes.
func (c CameraDetectionConfig) allowlistSet() map[string]struct{} {
	if len(c.ClassAllowlist) == 0 {
		return nil
	}
	s := make(map[string]struct{}, len(c.ClassAllowlist))
	for _, k := range c.ClassAllowlist {
		s[k] = struct{}{}
	}
	return s
}
