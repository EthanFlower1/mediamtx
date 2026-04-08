package objectdetection

import (
	"errors"
	"time"
)

// Rect is an axis-aligned bounding box in pixel coordinates. X1/Y1 is the
// top-left corner and X2/Y2 is the bottom-right (exclusive). The coordinate
// system is origin-top-left.
type Rect struct {
	X1, Y1, X2, Y2 float64
}

// Width returns the box width. Never negative.
func (r Rect) Width() float64 {
	if r.X2 < r.X1 {
		return 0
	}
	return r.X2 - r.X1
}

// Height returns the box height. Never negative.
func (r Rect) Height() float64 {
	if r.Y2 < r.Y1 {
		return 0
	}
	return r.Y2 - r.Y1
}

// Area returns the pixel area of the box.
func (r Rect) Area() float64 { return r.Width() * r.Height() }

// Center returns the (cx, cy) midpoint of the box.
func (r Rect) Center() (float64, float64) {
	return (r.X1 + r.X2) / 2, (r.Y1 + r.Y2) / 2
}

// IoU computes the intersection-over-union of two rectangles.
func (r Rect) IoU(o Rect) float64 {
	ix1 := max64(r.X1, o.X1)
	iy1 := max64(r.Y1, o.Y1)
	ix2 := min64(r.X2, o.X2)
	iy2 := min64(r.Y2, o.Y2)
	iw := ix2 - ix1
	ih := iy2 - iy1
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	union := r.Area() + o.Area() - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Point is a single (x, y) coordinate.
type Point struct{ X, Y float64 }

// Polygon is a closed polygon expressed as an ordered slice of points.
// The polygon is implicitly closed — callers should NOT repeat the first
// point at the end. A polygon with fewer than three points is treated as
// empty (contains no points, overlaps no rectangles).
type Polygon []Point

// Contains reports whether the polygon contains the point using the
// ray-casting algorithm. Points on the edge are considered inside.
func (p Polygon) Contains(pt Point) bool {
	if len(p) < 3 {
		return false
	}
	inside := false
	n := len(p)
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := p[i].X, p[i].Y
		xj, yj := p[j].X, p[j].Y
		if ((yi > pt.Y) != (yj > pt.Y)) &&
			pt.X < (xj-xi)*(pt.Y-yi)/(yj-yi+1e-12)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

// Bounds returns the axis-aligned bounding box of the polygon.
func (p Polygon) Bounds() Rect {
	if len(p) == 0 {
		return Rect{}
	}
	r := Rect{X1: p[0].X, Y1: p[0].Y, X2: p[0].X, Y2: p[0].Y}
	for _, pt := range p[1:] {
		if pt.X < r.X1 {
			r.X1 = pt.X
		}
		if pt.X > r.X2 {
			r.X2 = pt.X
		}
		if pt.Y < r.Y1 {
			r.Y1 = pt.Y
		}
		if pt.Y > r.Y2 {
			r.Y2 = pt.Y
		}
	}
	return r
}

// OverlapFraction estimates the fraction of rect that lies inside the
// polygon, using a coarse grid sample (samplesPerSide * samplesPerSide
// points). Returns a value in [0, 1]. samplesPerSide must be >= 1.
func (p Polygon) OverlapFraction(r Rect, samplesPerSide int) float64 {
	if samplesPerSide < 1 || len(p) < 3 || r.Area() == 0 {
		return 0
	}
	hit := 0
	total := 0
	w := r.Width()
	h := r.Height()
	for i := 0; i < samplesPerSide; i++ {
		for j := 0; j < samplesPerSide; j++ {
			// Sample at grid cell centers to avoid edge bias.
			fx := (float64(i) + 0.5) / float64(samplesPerSide)
			fy := (float64(j) + 0.5) / float64(samplesPerSide)
			pt := Point{X: r.X1 + fx*w, Y: r.Y1 + fy*h}
			total++
			if p.Contains(pt) {
				hit++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// Frame is a single decoded video frame ready for inference. The pipeline
// is agnostic to how the frame was decoded — the caller is responsible for
// preparing the Tensor in the shape + dtype the loaded model expects.
type Frame struct {
	// CameraID identifies the source camera. Used for cooldown bucket
	// keying and event envelopes.
	CameraID string

	// CapturedAt is the wall-clock timestamp at which the frame was
	// captured. Used as the event timestamp.
	CapturedAt time.Time

	// Width, Height are the frame dimensions in pixels, used to resolve
	// normalized model output coordinates into absolute pixel rects.
	Width, Height int

	// Tensor is the input tensor that will be fed to Inferencer.Infer.
	// Shape/dtype must match what the loaded model expects.
	Tensor InferenceInput
}

// InferenceInput is the subset of inference.Tensor the feature cares about.
// Kept as a local alias to avoid leaking the inference package into
// callers that only need to assemble frames.
type InferenceInput struct {
	Name  string
	Shape []int
	DType string
	Data  []byte
}

// Detection is the post-processed, filtered detection event that survives
// the pipeline. TrackID is left empty if cross-camera tracking (KAI-285)
// isn't running — downstream consumers MUST treat an empty TrackID as a
// "not tracked" signal rather than a shared identity.
type Detection struct {
	Class       string
	ClassID     int
	Confidence  float64
	BoundingBox Rect
	TrackID     string

	// CameraID echoes the source camera for convenience in sinks.
	CameraID string
	// Timestamp is the frame capture time.
	Timestamp time.Time
}

// Typed errors returned by the feature. Downstream code can switch on
// errors.Is to react specifically to each failure mode.
var (
	// ErrModelNotFound is returned by New / ProcessFrame when the
	// configured model id cannot be resolved against the registry.
	ErrModelNotFound = errors.New("objectdetection: model not found")

	// ErrClosed is returned after Close has been called.
	ErrClosed = errors.New("objectdetection: detector closed")

	// ErrInvalidConfig is returned when the Config supplied to New
	// fails validation.
	ErrInvalidConfig = errors.New("objectdetection: invalid config")

	// ErrInvalidFrame is returned when ProcessFrame receives a frame
	// that cannot be converted to an inference tensor.
	ErrInvalidFrame = errors.New("objectdetection: invalid frame")
)
