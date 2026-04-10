// Package behavioral manages per-camera configuration for the six behavioral
// analytics detectors (KAI-284): loitering, line_crossing, roi,
// crowd_density, tailgating, and fall_detection.
//
// Package boundary: imports internal/shared/* and internal/cloud/db/* only.
// Never imports apiserver or any other cloud package to avoid cycles.
//
// Multi-tenant invariant: every exported Store method scopes every SQL
// predicate by tenantID. Cross-tenant lookup is impossible by construction.
package behavioral

import (
	"errors"
	"time"
)

// DetectorType is the canonical discriminant for the six KAI-284 detectors.
type DetectorType string

// Canonical DetectorType values. These must stay in sync with the CHECK
// constraint in migration 0015_behavioral_config.up.sql.
const (
	DetectorLoitering     DetectorType = "loitering"
	DetectorLineCrossing  DetectorType = "line_crossing"
	DetectorROI           DetectorType = "roi"
	DetectorCrowdDensity  DetectorType = "crowd_density"
	DetectorTailgating    DetectorType = "tailgating"
	DetectorFallDetection DetectorType = "fall_detection"
)

// allDetectorTypes is the exhaustive set used for CHECK constraint validation.
var allDetectorTypes = map[DetectorType]struct{}{
	DetectorLoitering:     {},
	DetectorLineCrossing:  {},
	DetectorROI:           {},
	DetectorCrowdDensity:  {},
	DetectorTailgating:    {},
	DetectorFallDetection: {},
}

// IsValid returns true if the DetectorType is one of the six canonical values.
func (d DetectorType) IsValid() bool {
	_, ok := allDetectorTypes[d]
	return ok
}

// Config is a single row from the behavioral_config table.
// Params is the raw JSON blob; the validator interprets it per detector type.
type Config struct {
	TenantID     string
	CameraID     string
	DetectorType DetectorType
	// Params is the raw JSON object. Never nil; empty object if no params.
	Params    string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Sentinel errors — callers use errors.Is.
var (
	// ErrNotFound is returned when a (tenant, camera, detector) row does not exist.
	ErrNotFound = errors.New("behavioral: not found")
	// ErrInvalidTenantID is returned when tenantID is empty.
	ErrInvalidTenantID = errors.New("behavioral: tenant_id is required")
	// ErrInvalidCameraID is returned when cameraID is empty.
	ErrInvalidCameraID = errors.New("behavioral: camera_id is required")
	// ErrInvalidDetectorType is returned when detector_type is not one of the six.
	ErrInvalidDetectorType = errors.New("behavioral: invalid detector_type")
	// ErrInvalidParams is returned by the validator for bad per-detector params.
	ErrInvalidParams = errors.New("behavioral: invalid params")
)
