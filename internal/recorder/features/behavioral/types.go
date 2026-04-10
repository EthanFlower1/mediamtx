// Package behavioral contains the recorder-side pipeline for the six
// behavioral analytics detectors (KAI-284):
//
//   - loitering
//   - line_crossing
//   - roi
//   - crowd_density
//   - tailgating
//   - fall_detection
//
// Package boundary: imports internal/shared/* and internal/recorder/* only.
// Never imports internal/cloud/* to keep the edge-cloud boundary clean.
//
// Configuration is delivered via the recorder-control wire message
// (KAI-253) on each StreamAssignments snapshot or delta event. The
// Pipeline calls LoadConfig at startup and re-applies on each received
// camera update.
package behavioral

import "encoding/json"

// DetectorType mirrors cloud/behavioral.DetectorType but is redeclared here
// to avoid an import of the cloud package from recorder code.
type DetectorType string

// Canonical DetectorType values. These must stay in sync with the cloud-side
// behavioral.DetectorType constants and the migration CHECK constraint.
const (
	DetectorLoitering     DetectorType = "loitering"
	DetectorLineCrossing  DetectorType = "line_crossing"
	DetectorROI           DetectorType = "roi"
	DetectorCrowdDensity  DetectorType = "crowd_density"
	DetectorTailgating    DetectorType = "tailgating"
	DetectorFallDetection DetectorType = "fall_detection"
)

// CameraDetectorConfig is the per-camera, per-detector configuration slice
// delivered to the recorder via the wire message (see wireCamera in
// recordercontrol/types.go). The Params field is the raw JSON object
// validated cloud-side before delivery.
type CameraDetectorConfig struct {
	DetectorType DetectorType `json:"detector_type"`
	Params       string       `json:"params"`
	Enabled      bool         `json:"enabled"`
}

// CameraConfig bundles all detector configs for a single camera.
type CameraConfig struct {
	CameraID  string                 `json:"camera_id"`
	Detectors []CameraDetectorConfig `json:"detectors"`
}

// ParseCameraConfig parses the raw JSON payload from the wire message into
// a CameraConfig. Returns an empty CameraConfig on empty/nil input so
// callers never see nil detectors.
func ParseCameraConfig(raw string) (CameraConfig, error) {
	if raw == "" || raw == "null" {
		return CameraConfig{}, nil
	}
	var cfg CameraConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return CameraConfig{}, err
	}
	return cfg, nil
}
