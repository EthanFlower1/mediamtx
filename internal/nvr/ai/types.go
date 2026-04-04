package ai

import (
	"image"
	"time"
)

// Frame is a decoded video frame from the FrameSrc stage.
type Frame struct {
	Image     image.Image
	Timestamp time.Time
	Width     int
	Height    int
}

// BoundingBox holds normalized [0,1] coordinates.
type BoundingBox struct {
	X float32
	Y float32
	W float32
	H float32
}

// Detection is a single detected object from YOLO or ONVIF.
type Detection struct {
	Class      string
	Confidence float32
	Box        BoundingBox
	Source     DetectionSource
}

// DetectionSource identifies where a detection came from.
type DetectionSource int

const (
	SourceYOLO  DetectionSource = iota
	SourceONVIF
)

// DetectionFrame is the output of the Detector stage.
type DetectionFrame struct {
	Timestamp  time.Time
	Image      image.Image
	Detections []Detection
}

// ObjectState is the lifecycle state of a tracked object.
type ObjectState int

const (
	ObjectEntered ObjectState = iota
	ObjectActive
	ObjectLeft
)

func (s ObjectState) String() string {
	switch s {
	case ObjectEntered:
		return "entered"
	case ObjectActive:
		return "active"
	case ObjectLeft:
		return "left"
	default:
		return "unknown"
	}
}

// TrackedObject is a detection with a persistent track ID and lifecycle state.
type TrackedObject struct {
	TrackID    int
	State      ObjectState
	Class      string
	Confidence float32
	Box        BoundingBox
	FirstSeen  time.Time
	LastSeen   time.Time
}

// TrackedFrame is the output of the Tracker stage.
type TrackedFrame struct {
	Timestamp time.Time
	Objects   []TrackedObject
	// Image retained for embedding generation on enter events.
	Image image.Image
}

// DetectionFrameData holds per-detection bounding box data for a single frame,
// used by PublishDetectionFrame to broadcast live overlay data to WebSocket clients.
type DetectionFrameData struct {
	Class      string
	Confidence float32
	TrackID    int
	X, Y, W, H float32
}

// EventPublisher is the interface for publishing detection events to notification
// subscribers. The pipeline calls PublishAIDetection when it detects an
// important object class (person, vehicle, animal). It also calls
// PublishDetectionFrame every frame that contains detections so that Flutter
// analytics overlays can render live bounding boxes.
type EventPublisher interface {
	PublishAIDetection(cameraName, className string, confidence float32)
	PublishDetectionFrame(camera string, detections []DetectionFrameData)
}

// PipelineConfig holds per-camera pipeline configuration.
type PipelineConfig struct {
	CameraID         string
	CameraName       string
	ModelName        string  // ONNX model name for metrics labeling (default "yolov8n")
	StreamURL        string  // RTSP URL of the stream to decode
	StreamWidth      int     // expected frame width (0 = probe via ffprobe)
	StreamHeight     int     // expected frame height (0 = probe via ffprobe)
	ConfidenceThresh float32 // YOLO confidence threshold, default 0.5
	TrackTimeout     int     // seconds before lost track marked "left", default 5

	// ONVIF metadata endpoint (empty = disabled).
	ONVIFMetadataURL string
	ONVIFUsername    string
	ONVIFPassword    string
}
