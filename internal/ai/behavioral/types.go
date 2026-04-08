package behavioral

import (
	"context"
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Geometry primitives
// ---------------------------------------------------------------------------

// Point is a 2-D coordinate in normalised image space (0.0–1.0 on both axes).
// Using normalised coords decouples the detector from a specific resolution.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// BoundingBox is the axis-aligned bounding box of a detection, expressed in
// normalised image space.  (X1,Y1) is the top-left, (X2,Y2) the bottom-right.
type BoundingBox struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

// Center returns the centroid of the bounding box.
func (b BoundingBox) Center() Point {
	return Point{
		X: (b.X1 + b.X2) / 2,
		Y: (b.Y1 + b.Y2) / 2,
	}
}

// Height returns the normalised height of the bounding box.
func (b BoundingBox) Height() float64 {
	h := b.Y2 - b.Y1
	if h < 0 {
		return -h
	}
	return h
}

// Width returns the normalised width of the bounding box.
func (b BoundingBox) Width() float64 {
	w := b.X2 - b.X1
	if w < 0 {
		return -w
	}
	return w
}

// Polygon is an ordered list of vertices forming a closed region.
// The closing edge is implied between the last and first vertex.
type Polygon []Point

// Contains returns true when p lies inside or on the boundary of the polygon
// using the ray-casting algorithm.  Degenerate polygons (<3 vertices) always
// return false.
func (poly Polygon) Contains(p Point) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := range n {
		vi := poly[i]
		vj := poly[j]
		if ((vi.Y > p.Y) != (vj.Y > p.Y)) &&
			(p.X < (vj.X-vi.X)*(p.Y-vi.Y)/(vj.Y-vi.Y)+vi.X) {
			inside = !inside
		}
		j = i
	}
	return inside
}

// LineSegment is a directed segment from A to B used by line-crossing logic.
type LineSegment struct {
	A Point `json:"a"`
	B Point `json:"b"`
}

// ---------------------------------------------------------------------------
// Detection frame (KAI-281 output wrapper)
// ---------------------------------------------------------------------------

// Detection is a single object detected in a video frame.
type Detection struct {
	// TrackID is the cross-frame tracking identifier assigned by the tracker
	// in KAI-281.  0 means no track (single-frame detection only).
	TrackID int64 `json:"track_id"`

	// Class is the object class label (e.g. "person", "vehicle").
	Class string `json:"class"`

	// Confidence is the detection score in [0,1].
	Confidence float32 `json:"confidence"`

	// Box is the bounding box of the detection.
	Box BoundingBox `json:"box"`
}

// DetectionFrame is the unit of work consumed by every Detector.  It wraps
// the output of the KAI-281 object-detection pipeline for a single video frame.
type DetectionFrame struct {
	// TenantID and CameraID identify the source for multi-tenant isolation.
	TenantID string `json:"tenant_id"`
	CameraID string `json:"camera_id"`

	// FrameID is a monotonically increasing sequence number for this camera.
	FrameID uint64 `json:"frame_id"`

	// Timestamp is when the frame was captured (not when it was processed).
	Timestamp time.Time `json:"timestamp"`

	// Detections is the list of objects found in the frame.
	Detections []Detection `json:"detections"`
}

// ---------------------------------------------------------------------------
// Behavioral event
// ---------------------------------------------------------------------------

// EventKind identifies the type of behavioral event.
type EventKind string

// EventKind constants for all supported behavioral analytics events.
const (
	EventLoitering    EventKind = "loitering"
	EventLineCrossing EventKind = "line_crossing"
	EventROIEntry     EventKind = "roi_entry"
	EventROIExit      EventKind = "roi_exit"
	EventCrowdDensity EventKind = "crowd_density"
	EventTailgating   EventKind = "tailgating"
	EventFall         EventKind = "fall"
)

// CrossingDirection indicates which side of a line the object moved toward.
type CrossingDirection string

// CrossingDirection constants: AB means A-side to B-side, BA is the reverse.
const (
	DirectionAB CrossingDirection = "AB" // from A-side to B-side
	DirectionBA CrossingDirection = "BA" // from B-side to A-side
)

// BehavioralEvent is emitted by a Detector when a configurable condition is
// met.  It flows into the AI event pipeline (KAI-254 wire-up pending).
// The package-qualified name (behavioral.BehavioralEvent) is intentionally
// descriptive for cross-package clarity.
//
//nolint:revive
type BehavioralEvent struct {
	// TenantID and CameraID mirror the source DetectionFrame values.
	TenantID string `json:"tenant_id"`
	CameraID string `json:"camera_id"`

	// Kind is the type of behavioral event.
	Kind EventKind `json:"kind"`

	// At is the wall-clock time of the triggering frame.
	At time.Time `json:"at"`

	// TrackID is the track that triggered the event, if applicable.
	TrackID int64 `json:"track_id,omitempty"`

	// DurationInROI is populated for loitering events.
	DurationInROI time.Duration `json:"duration_in_roi,omitempty"`

	// Direction is populated for line-crossing and tailgating events.
	Direction CrossingDirection `json:"direction,omitempty"`

	// PersonCount is populated for crowd-density events.
	PersonCount int `json:"person_count,omitempty"`

	// DetectorID is the opaque configuration ID of the detector that fired.
	DetectorID string `json:"detector_id"`

	// Meta is an optional JSON blob for detector-specific extra data.
	Meta json.RawMessage `json:"meta,omitempty"`
}

// ---------------------------------------------------------------------------
// Detector interface
// ---------------------------------------------------------------------------

// Detector is the core interface for every behavioral analytics detector.
// All implementations MUST be safe for concurrent Feed calls; they MUST NOT
// share state with detectors belonging to other tenants or cameras.
type Detector interface {
	// Feed delivers a detection frame.  The implementation updates internal
	// state and may emit one or more events onto the channel returned by
	// Events.  Feed MUST NOT block; drop or buffer internally if needed.
	Feed(ctx context.Context, frame DetectionFrame)

	// Events returns the read-only channel on which this detector publishes
	// BehavioralEvent values.  The channel is closed when the detector is
	// closed.
	Events() <-chan BehavioralEvent

	// Close drains any pending state and closes the Events channel.
	// After Close, Feed calls are no-ops.
	Close()
}

// ---------------------------------------------------------------------------
// Event publisher seam (KAI-254 TODO)
// ---------------------------------------------------------------------------

// AIEventPublisher is the seam between behavioral analytics and the wider
// AI event pipeline.  The real implementation will live in KAI-254
// (DirectoryIngest.PublishAIEvents).  For now only the fake is used in tests.
//
// TODO(KAI-254): wire real publisher.
type AIEventPublisher interface {
	Publish(ctx context.Context, event BehavioralEvent) error
}

// NoopPublisher is an AIEventPublisher that discards all events.
// Used when no real publisher is configured.
type NoopPublisher struct{}

// Publish discards the event.
func (NoopPublisher) Publish(_ context.Context, _ BehavioralEvent) error { return nil }

// ---------------------------------------------------------------------------
// Configuration model
// ---------------------------------------------------------------------------

// DetectorType enumerates the six behavioral detector types.
type DetectorType string

// DetectorType constants for all six behavioral detector types.
const (
	DetectorTypeLoitering    DetectorType = "loitering"
	DetectorTypeLineCrossing DetectorType = "line_crossing"
	DetectorTypeROI          DetectorType = "roi"
	DetectorTypeCrowdDensity DetectorType = "crowd_density"
	DetectorTypeTailgating   DetectorType = "tailgating"
	DetectorTypeFall         DetectorType = "fall"
)

// LoiteringParams is the JSON params blob for a LoiteringDetector config.
type LoiteringParams struct {
	ROI              Polygon       `json:"roi"`
	ThresholdSeconds float64       `json:"threshold_seconds"`
	CheckInterval    time.Duration `json:"check_interval,omitempty"`
}

// LineCrossingParams is the JSON params blob for a LineCrossingDetector config.
type LineCrossingParams struct {
	Line LineSegment `json:"line"`
}

// ROIParams is the JSON params blob for an ROIDetector config.
type ROIParams struct {
	ROI Polygon `json:"roi"`
}

// CrowdDensityParams is the JSON params blob for a CrowdDensityDetector config.
type CrowdDensityParams struct {
	ROI            Polygon `json:"roi"`
	ThresholdCount int     `json:"threshold_count"`
}

// TailgatingParams is the JSON params blob for a TailgatingDetector config.
type TailgatingParams struct {
	Line          LineSegment `json:"line"`
	WindowSeconds float64     `json:"window_seconds"`
}

// FallParams is the JSON params blob for a FallDetector config.
type FallParams struct {
	// HeightDropFraction is the minimum fractional height drop (0–1) that
	// triggers a fall event.  Default is 0.40 (40% drop).
	HeightDropFraction float64 `json:"height_drop_fraction"`

	// WindowSeconds is the maximum time window over which the drop must occur.
	// Default is 0.5 seconds.  FallDetector assumes 10–60 FPS.
	WindowSeconds float64 `json:"window_seconds"`
}

// DetectorConfig is the per-camera, per-detector configuration row stored in
// the behavioral_config table.
type DetectorConfig struct {
	// ID is the opaque, stable identifier for this configuration row.
	ID string `json:"id"`

	// TenantID and CameraID scope this config.  Multi-tenant isolation MUST
	// be enforced by the store: never return configs for a different tenant.
	TenantID string `json:"tenant_id"`
	CameraID string `json:"camera_id"`

	// Type identifies the detector.
	Type DetectorType `json:"type"`

	// Enabled gates the detector.  Disabled detectors MUST NOT fire events.
	Enabled bool `json:"enabled"`

	// Params is the JSON-encoded detector-specific parameters struct.
	// Decode using the helper methods below.
	Params json.RawMessage `json:"params"`
}

// LoiteringConfig decodes Params into LoiteringParams.
func (c DetectorConfig) LoiteringConfig() (LoiteringParams, error) {
	var p LoiteringParams
	return p, json.Unmarshal(c.Params, &p)
}

// LineCrossingConfig decodes Params into LineCrossingParams.
func (c DetectorConfig) LineCrossingConfig() (LineCrossingParams, error) {
	var p LineCrossingParams
	return p, json.Unmarshal(c.Params, &p)
}

// ROIConfig decodes Params into ROIParams.
func (c DetectorConfig) ROIConfig() (ROIParams, error) {
	var p ROIParams
	return p, json.Unmarshal(c.Params, &p)
}

// CrowdDensityConfig decodes Params into CrowdDensityParams.
func (c DetectorConfig) CrowdDensityConfig() (CrowdDensityParams, error) {
	var p CrowdDensityParams
	return p, json.Unmarshal(c.Params, &p)
}

// TailgatingConfig decodes Params into TailgatingParams.
func (c DetectorConfig) TailgatingConfig() (TailgatingParams, error) {
	var p TailgatingParams
	return p, json.Unmarshal(c.Params, &p)
}

// FallConfig decodes Params into FallParams.
func (c DetectorConfig) FallConfig() (FallParams, error) {
	var p FallParams
	return p, json.Unmarshal(c.Params, &p)
}

// ---------------------------------------------------------------------------
// Config store seam (backed by behavioral_config DB table)
// ---------------------------------------------------------------------------

// BehavioralConfigStore is the read-only seam for loading detector configs.
// Implementations MUST scope every query to tenantID (fail-closed).
//
// The real implementation wires to internal/cloud/db and must include
// the Casbin behavioral.config.read permission check.
//
//nolint:revive
type BehavioralConfigStore interface {
	// ListForCamera returns all enabled detector configs for the given camera.
	// Results MUST belong to tenantID; returning configs for another tenant
	// is a tenant isolation violation.
	ListForCamera(ctx context.Context, tenantID, cameraID string) ([]DetectorConfig, error)
}
