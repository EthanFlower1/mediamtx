// Package behavioral implements the KAI-284 edge-side behavioral analytics
// layer for the Kaivue Recording Server.
//
// # Overview
//
// Behavioral analytics runs on top of object-detection frames produced by
// KAI-281.  The package provides six detectors that track persons and objects
// across consecutive frames and emit [BehavioralEvent] values when a
// configurable condition is met:
//
//   - [LoiteringDetector]    – person stays inside an ROI longer than a threshold
//   - [LineCrossingDetector] – person centroid crosses a configured line segment
//   - [ROIDetector]          – entry/exit transitions across an ROI boundary
//   - [CrowdDensityDetector] – person count inside an ROI exceeds a threshold
//   - [TailgatingDetector]   – two persons cross a line within N seconds
//   - [FallDetector]         – person bounding-box height collapses rapidly
//
// All detectors implement the [Detector] interface.  A [Pipeline] composes
// multiple detectors for a single camera and fan-outs incoming
// [DetectionFrame] values in parallel.
//
// # Multi-tenant isolation
//
// Every [DetectorConfig] carries a TenantID and CameraID.  State machines
// are per-camera-per-detector and are never shared across tenants.  Config
// reads from the database MUST include the tenant predicate (enforced by
// [BehavioralConfigStore]).
//
// # Edge-always
//
// All six detectors run edge-always per the inference routing rules in
// KAI-280.  They receive bounding-box output from KAI-281 and perform only
// lightweight geometric / temporal computation — no GPU required.
//
// # FPS assumption for fall detection
//
// [FallDetector] requires a stable frame rate to measure height change over
// time.  It is calibrated for 10–60 FPS.  At lower frame rates the height
// drop criterion may be missed or the window may span more wall-clock time
// than intended.  Document this in camera configuration.
//
// # Wire-up TODO
//
// The [AIEventPublisher] interface stubs event emission.  Wire the real
// publisher once KAI-254 (DirectoryIngest.PublishAIEvents) is available.
package behavioral
