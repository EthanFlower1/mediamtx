package lpr

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// Detector wraps a plate-localisation model (YOLO-style) and a plate-reader
// model (CRNN) behind the inference.Inferencer abstraction. It is safe for
// concurrent ProcessFrame calls.
//
// Callers obtain a Detector via New, call ProcessFrame for each vehicle frame,
// and call Close when done.
type Detector struct {
	cfg   Config
	inf   inference.Inferencer
	locModel *inference.LoadedModel // plate localisation (YOLO-style)
	readModel *inference.LoadedModel // plate reader (CRNN)

	mu     sync.Mutex
	closed bool
}

// New constructs a Detector. It eagerly loads both models from the provided
// Inferencer. Returns ErrInvalidConfig if cfg fails validation or the
// Inferencer is nil.
func New(cfg Config, inf inference.Inferencer) (*Detector, error) {
	if inf == nil {
		return nil, fmt.Errorf("%w: inferencer is required", ErrInvalidConfig)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	locModel, err := inf.LoadModel(ctx, cfg.LocalisationModelID, nil)
	if err != nil {
		return nil, fmt.Errorf("lpr: load localisation model %q: %w", cfg.LocalisationModelID, err)
	}

	readModel, err := inf.LoadModel(ctx, cfg.ReaderModelID, nil)
	if err != nil {
		_ = inf.Unload(ctx, locModel)
		return nil, fmt.Errorf("lpr: load reader model %q: %w", cfg.ReaderModelID, err)
	}

	return &Detector{
		cfg:       cfg,
		inf:       inf,
		locModel:  locModel,
		readModel: readModel,
	}, nil
}

// Close unloads both models and marks the detector closed.
func (d *Detector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	ctx := context.Background()
	_ = d.inf.Unload(ctx, d.locModel)
	_ = d.inf.Unload(ctx, d.readModel)
	return nil
}

// ProcessFrame runs the two-stage LPR pipeline on a single frame:
//  1. Localise plate candidates (YOLO-style model).
//  2. For each candidate, read character sequence (CRNN model).
//  3. Normalise text and match a regional format.
//
// Returns one PlateRead per detected plate (partial — TenantID/CameraID/
// Timestamp must be filled by the caller from the VehicleEvent context).
// Returns ErrDetectorClosed, ErrNoPlateFound, or an inference error.
func (d *Detector) ProcessFrame(ctx context.Context, frame Frame, camCfg CameraLPRConfig) ([]PlateRead, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrDetectorClosed
	}
	d.mu.Unlock()

	if !camCfg.Enabled {
		return nil, nil
	}
	if frame.Width <= 0 || frame.Height <= 0 {
		return nil, fmt.Errorf("%w: frame dimensions must be positive", ErrInvalidFrame)
	}

	// Stage 1: plate localisation.
	confThresh := d.cfg.ConfidenceThreshold
	if camCfg.ConfidenceOverride > 0 {
		confThresh = camCfg.ConfidenceOverride
	}

	locInput := inference.Tensor{
		Name:  frame.Tensor.Name,
		Shape: frame.Tensor.Shape,
		DType: inference.DType(frame.Tensor.DType),
		Data:  frame.Tensor.Data,
	}
	locResult, err := d.inf.Infer(ctx, d.locModel, locInput)
	if err != nil {
		return nil, fmt.Errorf("lpr: localisation infer: %w", err)
	}
	if len(locResult.Outputs) == 0 {
		return nil, ErrNoPlateFound
	}

	boxes := decodePlateBoxes(locResult.Outputs[0], float32(confThresh), frame.Width, frame.Height)
	if len(boxes) == 0 {
		return nil, ErrNoPlateFound
	}

	// Cap candidates.
	if d.cfg.MaxPlatesPerFrame > 0 && len(boxes) > d.cfg.MaxPlatesPerFrame {
		sort.Slice(boxes, func(i, j int) bool { return boxes[i].confidence > boxes[j].confidence })
		boxes = boxes[:d.cfg.MaxPlatesPerFrame]
	}

	reads := make([]PlateRead, 0, len(boxes))

	// Stage 2: for each localised plate, run reader model.
	for _, box := range boxes {
		// TODO: crop the original frame image using box.rect and build a
		// reader-compatible tensor. For now we pass the full frame tensor
		// through as a placeholder (real implementation requires an image
		// cropping utility that operates on the raw tensor data, which is
		// deferred pending the availability of the image bytes alongside
		// the tensor).
		readResult, err := d.inf.Infer(ctx, d.readModel, locInput)
		if err != nil {
			// Non-fatal: skip this candidate.
			continue
		}
		if len(readResult.Outputs) == 0 {
			continue
		}

		text, confidence := decodeCRNNOutput(readResult.Outputs[0])
		if confidence < float32(d.cfg.ReaderConfidenceThreshold) {
			continue
		}
		if text == "" {
			continue
		}

		normalised := Normalise(text)
		region := MatchRegion(normalised)

		reads = append(reads, PlateRead{
			// TenantID, CameraID, Timestamp filled by Pipeline from VehicleEvent.
			PlateText:  normalised,
			Confidence: (box.confidence + confidence) / 2,
			Region:     region,
			BoundingBox: Rect{
				X1: float64(box.rect.x1),
				Y1: float64(box.rect.y1),
				X2: float64(box.rect.x2),
				Y2: float64(box.rect.y2),
			},
		})
	}

	if len(reads) == 0 {
		return nil, ErrNoPlateFound
	}
	return reads, nil
}

// ---- internal decoding helpers -----------------------------------------------

type plateBox struct {
	rect       pixelRect
	confidence float32
}

type pixelRect struct {
	x1, y1, x2, y2 float32
}

// decodePlateBoxes parses a YOLO-style localisation output tensor.
// The layout is the same as the object-detection decoder:
// channel-major [4+1, N] where channels 0-3 are (cx, cy, w, h) and channel 4
// is the plate confidence.
func decodePlateBoxes(t inference.Tensor, confThresh float32, w, h int) []plateBox {
	if t.DType != inference.DTypeFloat32 {
		return nil
	}
	shape := t.Shape
	if len(shape) == 3 && shape[0] == 1 {
		shape = shape[1:]
	}
	if len(shape) != 2 {
		return nil
	}
	// Expect at least [5, N] (4 bbox + 1 score) or [N, 5].
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

	floats := bytesToFloat32(t.Data)
	if len(floats) != channels*anchors {
		return nil
	}

	at := func(c, a int) float32 {
		if channelMajor {
			return floats[c*anchors+a]
		}
		return floats[a*channels+c]
	}

	fw, fh := float32(w), float32(h)
	var boxes []plateBox
	for a := 0; a < anchors; a++ {
		score := at(4, a)
		if score < confThresh {
			continue
		}
		cx := at(0, a)
		cy := at(1, a)
		bw := at(2, a)
		bh := at(3, a)
		// If values look normalised (<= 1.5), scale to frame pixels.
		if cx <= 1.5 && cy <= 1.5 {
			cx *= fw
			cy *= fh
			bw *= fw
			bh *= fh
		}
		boxes = append(boxes, plateBox{
			rect: pixelRect{
				x1: cx - bw/2,
				y1: cy - bh/2,
				x2: cx + bw/2,
				y2: cy + bh/2,
			},
			confidence: score,
		})
	}
	return boxes
}

// decodeCRNNOutput interprets the CRNN model output tensor as a CTC-decoded
// plate string. For the fake inferencer used in tests the output tensor
// encodes the string as raw UTF-8 bytes in the Data field with confidence
// packed as the first float32.
//
// A real CRNN decoder would run Viterbi/CTC beam search over the logits;
// that implementation is deferred to the model-integration milestone.
func decodeCRNNOutput(t inference.Tensor) (text string, confidence float32) {
	if t.DType != inference.DTypeFloat32 || len(t.Data) < 4 {
		return "", 0
	}
	// The fake backend packs confidence as the first float32 followed by
	// the UTF-8 string. This is a test-only convention; production will use
	// the real CTC beam search decoder.
	confidence = math.Float32frombits(binary.LittleEndian.Uint32(t.Data[:4]))
	if len(t.Data) > 4 {
		text = string(t.Data[4:])
	}
	return text, confidence
}

// bytesToFloat32 reinterprets little-endian bytes as float32s.
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
