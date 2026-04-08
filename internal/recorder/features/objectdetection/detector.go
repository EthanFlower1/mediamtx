package objectdetection

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// Detector runs the object detection feature pipeline against a single
// LoadedModel handle. It is safe for concurrent ProcessFrame calls.
type Detector struct {
	cfg   Config
	inf   inference.Inferencer
	model *inference.LoadedModel
	sink  DetectionEventSink

	now func() time.Time // swapped in tests

	mu       sync.Mutex
	closed   bool
	cooldown map[cooldownKey]time.Time
}

// cooldownKey is the dedup key used to suppress repeated detections of
// the same class + approximate location within a camera.
type cooldownKey struct {
	camera string
	class  string
	xb, yb int
}

// Option customises a Detector at construction time.
type Option func(*Detector)

// WithSink attaches a DetectionEventSink that receives every filtered
// detection batch emitted by ProcessFrame. Multiple sinks can be
// installed by wrapping them in a fan-out sink at the caller layer.
func WithSink(s DetectionEventSink) Option {
	return func(d *Detector) { d.sink = s }
}

// withClock overrides the clock used for cooldown bookkeeping. Package-
// private: tests in this package use it directly.
func withClock(fn func() time.Time) Option {
	return func(d *Detector) { d.now = fn }
}

// New constructs a Detector. It eagerly loads the model from the provided
// Inferencer (which is expected to have a ModelRegistry wired up so that
// nil bytes resolve to the approved version). If the registry returns
// inference.ErrModelNotFound, the error is wrapped as ErrModelNotFound.
func New(cfg Config, inf inference.Inferencer, opts ...Option) (*Detector, error) {
	if inf == nil {
		return nil, fmt.Errorf("%w: inferencer is required", ErrInvalidConfig)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	model, err := inf.LoadModel(ctx, cfg.ModelID, nil)
	if err != nil {
		if errors.Is(err, inference.ErrModelNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, cfg.ModelID)
		}
		return nil, fmt.Errorf("objectdetection: load model %q: %w", cfg.ModelID, err)
	}

	d := &Detector{
		cfg:      cfg,
		inf:      inf,
		model:    model,
		now:      time.Now,
		cooldown: make(map[cooldownKey]time.Time),
	}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

// Close unloads the model and marks the detector closed. After Close, any
// ProcessFrame call returns ErrClosed.
func (d *Detector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if d.model != nil {
		_ = d.inf.Unload(context.Background(), d.model)
	}
	return nil
}

// ProcessFrame runs a single frame through the full pipeline and emits
// any events that survive to the configured sink.
func (d *Detector) ProcessFrame(
	ctx context.Context,
	frame Frame,
	cameraCfg CameraDetectionConfig,
) ([]Detection, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	d.mu.Unlock()

	if !cameraCfg.Enabled {
		return nil, nil
	}
	if frame.Width <= 0 || frame.Height <= 0 {
		return nil, fmt.Errorf("%w: frame dimensions must be positive", ErrInvalidFrame)
	}

	// Translate the feature-local InferenceInput back into the
	// inference.Tensor the backend expects. Keeping the two types
	// separate means downstream callers can build frames without
	// importing the inference package directly.
	in := inference.Tensor{
		Name:  frame.Tensor.Name,
		Shape: frame.Tensor.Shape,
		DType: inference.DType(frame.Tensor.DType),
		Data:  frame.Tensor.Data,
	}

	res, err := d.inf.Infer(ctx, d.model, in)
	if err != nil {
		return nil, fmt.Errorf("objectdetection: infer: %w", err)
	}
	if len(res.Outputs) == 0 {
		return nil, nil
	}

	// Decode YOLO-style output — see decodeYOLOOutput for the layout.
	raw := decodeYOLOOutput(res.Outputs[0], frame.Width, frame.Height)

	// 1. Confidence filter.
	thresh := cameraCfg.effectiveConfidence(d.cfg.ConfidenceThreshold)
	raw = filterConfidence(raw, thresh)

	// 2. NMS per class.
	raw = nms(raw, d.cfg.NMSIoUThreshold)

	// Cap to MaxDetectionsPerFrame (highest confidence wins).
	if d.cfg.MaxDetectionsPerFrame > 0 && len(raw) > d.cfg.MaxDetectionsPerFrame {
		sort.SliceStable(raw, func(i, j int) bool {
			return raw[i].Confidence > raw[j].Confidence
		})
		raw = raw[:d.cfg.MaxDetectionsPerFrame]
	}

	// 3. Resolve labels via the vertical class map. Unknown class ids
	// are dropped here.
	labelled := make([]Detection, 0, len(raw))
	for _, r := range raw {
		label, ok := d.cfg.ClassMap.Label(r.ClassID)
		if !ok {
			continue
		}
		labelled = append(labelled, Detection{
			Class:       label,
			ClassID:     r.ClassID,
			Confidence:  r.Confidence,
			BoundingBox: r.Box,
			CameraID:    frame.CameraID,
			Timestamp:   frame.CapturedAt,
		})
	}

	// 4. Class allowlist.
	allowed := cameraCfg.allowlistSet()
	if allowed != nil {
		kept := labelled[:0]
		for _, det := range labelled {
			if _, ok := allowed[det.Class]; ok {
				kept = append(kept, det)
			}
		}
		labelled = kept
	}

	// 5. ROI filter.
	if len(cameraCfg.ROIs) > 0 {
		kept := labelled[:0]
		for _, det := range labelled {
			if roiAccepts(cameraCfg.ROIs, det.BoundingBox, d.cfg.ROISamplesPerSide, d.cfg.ROIOverlapThreshold) {
				kept = append(kept, det)
			}
		}
		labelled = kept
	}

	// 6. Min box area.
	if cameraCfg.MinBoxArea > 0 {
		kept := labelled[:0]
		for _, det := range labelled {
			if det.BoundingBox.Area() >= cameraCfg.MinBoxArea {
				kept = append(kept, det)
			}
		}
		labelled = kept
	}

	// 7. Cooldown dedup.
	if cameraCfg.CooldownSeconds > 0 && len(labelled) > 0 {
		labelled = d.applyCooldown(frame.CameraID, labelled, cameraCfg.cooldownDuration())
	}

	// 8. Publish.
	if len(labelled) > 0 && d.sink != nil {
		if err := d.sink.Publish(ctx, labelled); err != nil {
			return labelled, fmt.Errorf("objectdetection: publish: %w", err)
		}
	}

	return labelled, nil
}

// applyCooldown drops detections whose (camera, class, spatial bucket)
// has already emitted within the cooldown window, and records the
// acceptance timestamps for the rest.
func (d *Detector) applyCooldown(cameraID string, dets []Detection, window time.Duration) []Detection {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	// Garbage collect expired entries before testing new ones to keep
	// the map bounded under long-running detectors.
	for k, t := range d.cooldown {
		if now.Sub(t) > window*4 {
			delete(d.cooldown, k)
		}
	}

	bucket := d.cfg.CooldownBucketPixels
	if bucket <= 0 {
		bucket = 64
	}
	out := dets[:0]
	for _, det := range dets {
		cx, cy := det.BoundingBox.Center()
		key := cooldownKey{
			camera: cameraID,
			class:  det.Class,
			xb:     int(math.Floor(cx / float64(bucket))),
			yb:     int(math.Floor(cy / float64(bucket))),
		}
		if last, ok := d.cooldown[key]; ok && now.Sub(last) < window {
			continue
		}
		d.cooldown[key] = now
		out = append(out, det)
	}
	return out
}

// ---- YOLO output decoding -------------------------------------------------

// rawDetection is the internal struct used between decoding and filtering.
// It carries the normalized class id and confidence alongside the pixel
// rect. Downstream we materialise it into a Detection once the label is
// known.
type rawDetection struct {
	ClassID    int
	Confidence float64
	Box        Rect
}

// decodeYOLOOutput parses a YOLO-v8/v9 style output tensor. The canonical
// layout is [1, 4+C, N] where N is the number of anchor boxes and C is
// the number of classes. For each anchor the first four channels are
// (cx, cy, w, h) in pixel units matching the input image, and the
// remaining C channels are per-class confidences (raw, not softmaxed).
//
// We select the argmax class for each anchor; the pipeline's confidence
// and NMS filters handle everything after that.
//
// The fake backend used in tests produces its own output shapes — this
// decoder accepts either [1, 4+C, N] or [1, N, 4+C] layouts by inspecting
// shape. Any malformed tensor produces an empty slice (the pipeline then
// no-ops and ProcessFrame returns nil).
func decodeYOLOOutput(t inference.Tensor, frameW, frameH int) []rawDetection {
	if t.DType != inference.DTypeFloat32 {
		return nil
	}
	if len(t.Shape) < 2 {
		return nil
	}
	// Flatten the batch dim if present.
	shape := t.Shape
	if len(shape) == 3 && shape[0] == 1 {
		shape = shape[1:]
	}
	if len(shape) != 2 {
		return nil
	}

	// YOLO-v8/v9 canonical head layout is [1, 4+C, N] (channel-major).
	// We require the first dim to be >= 5 (4 bbox + at least 1 class).
	// Some exporters emit the transposed [1, N, 4+C] layout; we detect
	// that case only when the first dim is < 5.
	var channels, anchors int
	channelMajor := true
	if shape[0] >= 5 {
		channels = shape[0]
		anchors = shape[1]
	} else if shape[1] >= 5 {
		channels = shape[1]
		anchors = shape[0]
		channelMajor = false
	} else {
		return nil
	}
	numClasses := channels - 4
	floats := bytesToFloat32(t.Data)
	if len(floats) != channels*anchors {
		return nil
	}

	// at returns the scalar at (channel, anchor) regardless of layout.
	at := func(c, a int) float32 {
		if channelMajor {
			return floats[c*anchors+a]
		}
		return floats[a*channels+c]
	}

	out := make([]rawDetection, 0, anchors)
	for a := 0; a < anchors; a++ {
		cx := float64(at(0, a))
		cy := float64(at(1, a))
		w := float64(at(2, a))
		h := float64(at(3, a))
		bestID := -1
		bestScore := float32(0)
		for c := 0; c < numClasses; c++ {
			s := at(4+c, a)
			if s > bestScore {
				bestScore = s
				bestID = c
			}
		}
		if bestID < 0 || bestScore <= 0 {
			continue
		}
		// If coordinates look normalized (<= 1.5) scale to frame.
		if cx <= 1.5 && cy <= 1.5 && w <= 1.5 && h <= 1.5 {
			cx *= float64(frameW)
			cy *= float64(frameH)
			w *= float64(frameW)
			h *= float64(frameH)
		}
		out = append(out, rawDetection{
			ClassID:    bestID,
			Confidence: float64(bestScore),
			Box: Rect{
				X1: cx - w/2,
				Y1: cy - h/2,
				X2: cx + w/2,
				Y2: cy + h/2,
			},
		})
	}
	return out
}

// bytesToFloat32 reinterprets a little-endian byte slice as float32s.
// A copy is returned because the underlying tensor buffer may be reused.
func bytesToFloat32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// ---- Filter helpers -------------------------------------------------------

func filterConfidence(dets []rawDetection, thresh float64) []rawDetection {
	if thresh <= 0 {
		return dets
	}
	out := dets[:0]
	for _, d := range dets {
		if d.Confidence >= thresh {
			out = append(out, d)
		}
	}
	return out
}

// nms runs class-wise non-max suppression and returns the surviving
// boxes in confidence-descending order.
func nms(dets []rawDetection, iouThresh float64) []rawDetection {
	if len(dets) < 2 {
		return dets
	}
	// Group by class id.
	byClass := map[int][]rawDetection{}
	for _, d := range dets {
		byClass[d.ClassID] = append(byClass[d.ClassID], d)
	}
	out := make([]rawDetection, 0, len(dets))
	for _, group := range byClass {
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].Confidence > group[j].Confidence
		})
		suppressed := make([]bool, len(group))
		for i := 0; i < len(group); i++ {
			if suppressed[i] {
				continue
			}
			out = append(out, group[i])
			for j := i + 1; j < len(group); j++ {
				if suppressed[j] {
					continue
				}
				if group[i].Box.IoU(group[j].Box) >= iouThresh {
					suppressed[j] = true
				}
			}
		}
	}
	return out
}

// roiAccepts returns true when the box overlaps any ROI by at least the
// configured overlap fraction. An overlap threshold of 0 means "any
// overlap, including a single sample point inside an ROI, is enough".
func roiAccepts(rois []Polygon, box Rect, samples int, overlapThresh float64) bool {
	for _, poly := range rois {
		// Quick reject via bounding boxes.
		if !rectsIntersect(poly.Bounds(), box) {
			continue
		}
		frac := poly.OverlapFraction(box, samples)
		if overlapThresh <= 0 {
			if frac > 0 {
				return true
			}
			continue
		}
		if frac >= overlapThresh {
			return true
		}
	}
	return false
}

func rectsIntersect(a, b Rect) bool {
	if a.X2 < b.X1 || b.X2 < a.X1 {
		return false
	}
	if a.Y2 < b.Y1 || b.Y2 < a.Y1 {
		return false
	}
	return true
}
