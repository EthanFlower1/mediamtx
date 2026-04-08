package lpr

import (
	"errors"
	"time"
)

// PlateRead is the result of a successful license plate recognition pass on a
// single video frame.
type PlateRead struct {
	// TenantID scopes the read for multi-tenant isolation. Never empty.
	TenantID string

	// CameraID is the source camera. Never empty.
	CameraID string

	// Timestamp is the frame capture time (UTC).
	Timestamp time.Time

	// PlateText is the character sequence read from the plate (upper-case,
	// normalised). Example: "ABC123".
	PlateText string

	// Confidence is the combined plate-detector + reader confidence score,
	// range [0, 1].
	Confidence float32

	// Region is the matched regional format identifier, e.g. "US-CA",
	// "EU-DE", "UK", "AU". Empty means the text did not match any known
	// pattern — it is stored as-is for forensic completeness.
	Region string

	// BoundingBox is the plate location in pixel coordinates (X1/Y1
	// top-left, X2/Y2 bottom-right). Populated when a localisation model
	// is used; zero-value when cloud-fallback uses a pre-cropped image.
	BoundingBox Rect

	// CroppedImageRef is an optional object-storage reference for the
	// cropped plate thumbnail. Empty when thumbnail storage is disabled.
	CroppedImageRef string
}

// Rect mirrors objectdetection.Rect but is local to avoid a cross-feature
// import cycle for callers that only import this package.
type Rect struct {
	X1, Y1, X2, Y2 float64
}

// Sentinel errors.
var (
	// ErrDetectorClosed is returned by any method after Close is called.
	ErrDetectorClosed = errors.New("lpr: detector closed")

	// ErrInvalidConfig is returned when a Config fails validation.
	ErrInvalidConfig = errors.New("lpr: invalid configuration")

	// ErrInvalidFrame is returned when a frame is unprocessable
	// (e.g., zero dimensions).
	ErrInvalidFrame = errors.New("lpr: invalid frame")

	// ErrNoPlateFound is returned when the localisation model finds no
	// plate candidate above the confidence threshold in the given frame.
	ErrNoPlateFound = errors.New("lpr: no plate found")
)

// Frame is the unit of work consumed by the Detector. It carries a raw image
// in a tensor encoding (same shape convention as objectdetection.Frame) plus
// the camera context needed for multi-tenant scoping.
type Frame struct {
	// TenantID must match the owning tenant for this camera. Required.
	TenantID string

	// CameraID identifies the source camera. Required.
	CameraID string

	// CapturedAt is the wall-clock time the frame was captured.
	CapturedAt time.Time

	// Width and Height are the pixel dimensions of the image.
	Width, Height int

	// Tensor is the pre-processed image tensor ready for inference.
	// Shape convention: [1, C, H, W] float32, values in [0, 1].
	Tensor InferenceTensor
}

// InferenceTensor is a minimal local copy of the inference.Tensor fields
// the LPR feature cares about. We re-declare it here to avoid importing
// internal/shared/inference in callers that only handle frames.
type InferenceTensor struct {
	Name  string
	Shape []int
	DType string // "float32" etc.
	Data  []byte
}

// VehicleEvent carries the trigger signal from the object-detection pipeline.
// The LPR Pipeline subscribes to these; when Class is a vehicle class it
// requests an LPR pass on the associated frame region.
type VehicleEvent struct {
	TenantID   string
	CameraID   string
	CapturedAt time.Time
	Class      string  // "car", "truck", "bus", "motorcycle", "bicycle"
	Confidence float32 // object-detection confidence
	Box        Rect    // pixel bounding box of the detected vehicle
	Frame      Frame   // the full frame tensor for plate cropping
}

// vehicleClasses is the set of COCO classes that trigger LPR.
var vehicleClasses = map[string]bool{
	"car":        true,
	"truck":      true,
	"bus":        true,
	"motorcycle": true,
	"bicycle":    true,
}

// IsVehicleClass reports whether class should trigger LPR processing.
func IsVehicleClass(class string) bool { return vehicleClasses[class] }
