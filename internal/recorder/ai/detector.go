//go:build cgo

package ai

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

// InitONNXRuntime initializes the ONNX Runtime environment. It searches
// standard locations for the shared library and must be called once at startup
// before creating any detector sessions.
func InitONNXRuntime() error {
	home, _ := os.UserHomeDir()
	libPaths := []string{
		filepath.Join(home, "lib", "libonnxruntime.dylib"),
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
	}
	for _, p := range libPaths {
		if _, err := os.Stat(p); err == nil {
			ort.SetSharedLibraryPath(p)
			return ort.InitializeEnvironment()
		}
	}
	return fmt.Errorf("ONNX Runtime library not found; searched: %v", libPaths)
}

// ShutdownONNXRuntime cleans up the ONNX Runtime environment.
func ShutdownONNXRuntime() error {
	return ort.DestroyEnvironment()
}

// YOLODetection represents a single detected object.
type YOLODetection struct {
	Class      int     `json:"class"`
	ClassName  string  `json:"class_name"`
	Confidence float32 `json:"confidence"`
	X          float32 `json:"x"` // normalized 0-1
	Y          float32 `json:"y"`
	W          float32 `json:"w"`
	H          float32 `json:"h"`
}

// Detector wraps an ONNX Runtime session for YOLO inference.
type Detector struct {
	session      *ort.AdvancedSession
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	inputShape   ort.Shape
	labels       []string
}

// NewDetector creates a YOLO detector from an ONNX model file path.
// InitONNXRuntime must be called before creating a detector.
func NewDetector(modelPath string) (*Detector, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file not found: %s: %w", modelPath, err)
	}

	inputShape := ort.NewShape(1, 3, 640, 640)
	outputShape := ort.NewShape(1, 84, 8400)

	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, fmt.Errorf("creating input tensor: %w", err)
	}

	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("creating output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output0"},
		[]ort.ArbitraryTensor{inputTensor},
		[]ort.ArbitraryTensor{outputTensor},
		nil,
	)
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	return &Detector{
		session:      session,
		inputTensor:  inputTensor,
		outputTensor: outputTensor,
		inputShape:   inputShape,
		labels:       cocoLabels(),
	}, nil
}

// Detect runs YOLO inference on an image and returns detections above the
// confidence threshold.
func (d *Detector) Detect(img image.Image, confThreshold float32) ([]YOLODetection, error) {
	// Preprocess: resize to 640x640 and convert to CHW float32 tensor normalized to [0,1].
	d.preprocess(img)

	// Run inference.
	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("ONNX inference: %w", err)
	}

	// Postprocess: parse output, apply confidence threshold and NMS.
	return d.postprocess(confThreshold), nil
}

// Close releases all ONNX Runtime resources.
func (d *Detector) Close() {
	if d.session != nil {
		d.session.Destroy()
	}
	if d.inputTensor != nil {
		d.inputTensor.Destroy()
	}
	if d.outputTensor != nil {
		d.outputTensor.Destroy()
	}
}

// Labels returns the COCO class labels used by this detector.
func (d *Detector) Labels() []string {
	return d.labels
}

// preprocess resizes img to 640x640 and fills the input tensor in CHW format,
// normalized to [0, 1].
func (d *Detector) preprocess(img image.Image) {
	const size = 640

	// Resize to 640x640 using bilinear interpolation.
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Fill input tensor in CHW format (channel, height, width) normalized to [0,1].
	data := d.inputTensor.GetData()
	chSize := size * size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, _ := dst.At(x, y).RGBA()
			idx := y*size + x
			data[0*chSize+idx] = float32(r>>8) / 255.0 // R channel
			data[1*chSize+idx] = float32(g>>8) / 255.0 // G channel
			data[2*chSize+idx] = float32(b>>8) / 255.0 // B channel
		}
	}
}

// postprocess parses the YOLO output tensor [1, 84, 8400], applies confidence
// thresholding, and runs NMS to produce the final detections.
func (d *Detector) postprocess(confThreshold float32) []YOLODetection {
	const (
		numBoxes   = 8400
		numClasses = 80
		nmsIoU     = float32(0.45)
	)

	output := d.outputTensor.GetData()

	// Output shape is [1, 84, 8400]. We need to transpose to [8400, 84].
	// output[row*8400 + col] where row is the feature (0-83), col is the box index.
	// Row 0-3: cx, cy, w, h
	// Row 4-83: class scores

	type candidate struct {
		det   YOLODetection
		score float32
	}

	var candidates []candidate

	for i := 0; i < numBoxes; i++ {
		// Find the best class score for this box.
		bestClass := -1
		bestScore := float32(0)
		for c := 0; c < numClasses; c++ {
			score := output[(4+c)*numBoxes+i]
			if score > bestScore {
				bestScore = score
				bestClass = c
			}
		}

		if bestScore < confThreshold {
			continue
		}

		// Get box coordinates (center format) and convert to x,y,w,h normalized.
		cx := output[0*numBoxes+i] / 640.0
		cy := output[1*numBoxes+i] / 640.0
		w := output[2*numBoxes+i] / 640.0
		h := output[3*numBoxes+i] / 640.0

		x := cx - w/2
		y := cy - h/2

		// Clamp to [0, 1].
		x = clampF32(x, 0, 1)
		y = clampF32(y, 0, 1)
		w = clampF32(w, 0, 1-x)
		h = clampF32(h, 0, 1-y)

		className := ""
		if bestClass >= 0 && bestClass < len(d.labels) {
			className = d.labels[bestClass]
		}

		candidates = append(candidates, candidate{
			det: YOLODetection{
				Class:      bestClass,
				ClassName:  className,
				Confidence: bestScore,
				X:          x,
				Y:          y,
				W:          w,
				H:          h,
			},
			score: bestScore,
		})
	}

	// Sort by confidence descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// NMS: for each class, suppress overlapping boxes.
	keep := make([]bool, len(candidates))
	for i := range keep {
		keep[i] = true
	}

	for i := 0; i < len(candidates); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(candidates); j++ {
			if !keep[j] {
				continue
			}
			if candidates[i].det.Class != candidates[j].det.Class {
				continue
			}
			if iou(candidates[i].det, candidates[j].det) > nmsIoU {
				keep[j] = false
			}
		}
	}

	var results []YOLODetection
	for i, c := range candidates {
		if keep[i] {
			results = append(results, c.det)
		}
	}
	return results
}

// iou computes Intersection over Union between two detections.
func iou(a, b YOLODetection) float32 {
	ax1, ay1 := a.X, a.Y
	ax2, ay2 := a.X+a.W, a.Y+a.H
	bx1, by1 := b.X, b.Y
	bx2, by2 := b.X+b.W, b.Y+b.H

	ix1 := float32(math.Max(float64(ax1), float64(bx1)))
	iy1 := float32(math.Max(float64(ay1), float64(by1)))
	ix2 := float32(math.Min(float64(ax2), float64(bx2)))
	iy2 := float32(math.Min(float64(ay2), float64(by2)))

	iw := float32(math.Max(0, float64(ix2-ix1)))
	ih := float32(math.Max(0, float64(iy2-iy1)))

	inter := iw * ih
	areaA := a.W * a.H
	areaB := b.W * b.H
	union := areaA + areaB - inter

	if union <= 0 {
		return 0
	}
	return inter / union
}

func clampF32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cocoLabels returns the 80 COCO class labels in order.
func cocoLabels() []string {
	return []string{
		"person", "bicycle", "car", "motorcycle", "airplane",
		"bus", "train", "truck", "boat", "traffic light",
		"fire hydrant", "stop sign", "parking meter", "bench", "bird",
		"cat", "dog", "horse", "sheep", "cow",
		"elephant", "bear", "zebra", "giraffe", "backpack",
		"umbrella", "handbag", "tie", "suitcase", "frisbee",
		"skis", "snowboard", "sports ball", "kite", "baseball bat",
		"baseball glove", "skateboard", "surfboard", "tennis racket", "bottle",
		"wine glass", "cup", "fork", "knife", "spoon",
		"bowl", "banana", "apple", "sandwich", "orange",
		"broccoli", "carrot", "hot dog", "pizza", "donut",
		"cake", "chair", "couch", "potted plant", "bed",
		"dining table", "toilet", "tv", "laptop", "mouse",
		"remote", "keyboard", "cell phone", "microwave", "oven",
		"toaster", "sink", "refrigerator", "book", "clock",
		"vase", "scissors", "teddy bear", "hair drier", "toothbrush",
	}
}
