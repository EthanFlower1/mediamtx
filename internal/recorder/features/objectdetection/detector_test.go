package objectdetection

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
	"github.com/bluenviron/mediamtx/internal/shared/inference/fake"
)

// ----- test helpers --------------------------------------------------------

// stubRegistry is a minimal inference.ModelRegistry used only here.
// It returns fixed bytes for each known model id and ErrModelNotFound
// otherwise.
type stubRegistry struct {
	models map[string][]byte
}

func (s stubRegistry) Resolve(_ context.Context, id string) ([]byte, string, error) {
	b, ok := s.models[id]
	if !ok {
		return nil, "", inference.ErrModelNotFound
	}
	return b, "v1", nil
}

// scriptedInferencer wraps a fake.Inferencer but returns a scripted
// output tensor for every Infer call. This lets tests drive the
// post-processing pipeline with known raw YOLO boxes.
type scriptedInferencer struct {
	base   *fake.Inferencer
	output inference.Tensor
	calls  int
	mu     sync.Mutex
}

func (s *scriptedInferencer) Name() string                 { return s.base.Name() }
func (s *scriptedInferencer) Backend() inference.BackendKind { return s.base.Backend() }
func (s *scriptedInferencer) LoadModel(ctx context.Context, id string, b []byte) (*inference.LoadedModel, error) {
	return s.base.LoadModel(ctx, id, b)
}
func (s *scriptedInferencer) Unload(ctx context.Context, m *inference.LoadedModel) error {
	return s.base.Unload(ctx, m)
}
func (s *scriptedInferencer) Stats() inference.Stats { return s.base.Stats() }
func (s *scriptedInferencer) Close() error           { return s.base.Close() }

func (s *scriptedInferencer) Infer(ctx context.Context, m *inference.LoadedModel, in inference.Tensor) (*inference.InferenceResult, error) {
	s.mu.Lock()
	s.calls++
	out := s.output
	s.mu.Unlock()
	return &inference.InferenceResult{
		Outputs: []inference.Tensor{out},
		Latency: 0,
		ModelID: m.ID,
		Backend: s.base.Backend(),
	}, nil
}

// rawBox is a human-friendly description of one synthetic YOLO anchor.
type rawBox struct {
	classID int
	score   float32
	cx, cy  float32
	w, h    float32
}

// buildYOLOOutput packs rawBoxes into a [1, 4+C, N] float32 tensor
// matching the canonical YOLO-v8 head layout the decoder expects.
func buildYOLOOutput(boxes []rawBox, numClasses int) inference.Tensor {
	channels := 4 + numClasses
	anchors := len(boxes)
	if anchors == 0 {
		anchors = 1
	}
	data := make([]float32, channels*anchors)
	for a, b := range boxes {
		data[0*anchors+a] = b.cx
		data[1*anchors+a] = b.cy
		data[2*anchors+a] = b.w
		data[3*anchors+a] = b.h
		if b.classID >= 0 && b.classID < numClasses {
			data[(4+b.classID)*anchors+a] = b.score
		}
	}
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return inference.Tensor{
		Name:  "output",
		Shape: []int{1, channels, anchors},
		DType: inference.DTypeFloat32,
		Data:  buf,
	}
}

// newTestDetector wires up a scripted inferencer + stub registry and
// returns a Detector ready for pipeline assertions.
func newTestDetector(t *testing.T, cfg Config, output inference.Tensor, sink DetectionEventSink) (*Detector, *scriptedInferencer) {
	t.Helper()
	reg := stubRegistry{models: map[string][]byte{cfg.ModelID: []byte("fake-bytes")}}
	base := fake.New(fake.WithRegistry(reg))
	inf := &scriptedInferencer{base: base, output: output}
	opts := []Option{withClock(func() time.Time { return time.Unix(1_700_000_000, 0) })}
	if sink != nil {
		opts = append(opts, WithSink(sink))
	}
	d, err := New(cfg, inf, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d, inf
}

// defaultCfg produces a valid Config for most tests.
func defaultCfg() Config {
	return Config{
		ModelID:               "yolo-v8-s",
		ConfidenceThreshold:   0.25,
		NMSIoUThreshold:       0.5,
		MaxDetectionsPerFrame: 100,
		BackendHint:           BackendHintEdge,
		ClassMap:              GenericClasses,
		ROIOverlapThreshold:   0.25,
	}
}

// dummyFrame builds a minimal Frame the pipeline will accept.
func dummyFrame(cameraID string) Frame {
	return Frame{
		CameraID:   cameraID,
		CapturedAt: time.Unix(1_700_000_000, 0),
		Width:      640,
		Height:     480,
		Tensor: InferenceInput{
			Name:  "images",
			Shape: []int{1, 3, 2, 2},
			DType: string(inference.DTypeFloat32),
			Data:  make([]byte, 1*3*2*2*4),
		},
	}
}

// ----- tests ---------------------------------------------------------------

func TestHappyPathSingleDetection(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(GenericClasses))
	sink := NewInMemorySink()
	d, _ := newTestDetector(t, defaultCfg(), out, sink)

	cam := CameraDetectionConfig{Enabled: true}
	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection, got %d", len(dets))
	}
	if dets[0].Class != "person" {
		t.Errorf("want class person, got %q", dets[0].Class)
	}
	if sink.Len() != 1 {
		t.Errorf("sink got %d events, want 1", sink.Len())
	}
}

func TestConfidenceThresholdFilters(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.1, cx: 100, cy: 100, w: 40, h: 80},
		{classID: 0, score: 0.8, cx: 300, cy: 200, w: 40, h: 80},
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)

	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"),
		CameraDetectionConfig{Enabled: true})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection after confidence filter, got %d", len(dets))
	}
	if math.Abs(dets[0].Confidence-0.8) > 1e-5 {
		t.Errorf("wrong surviving box: %+v", dets[0])
	}
}

func TestClassAllowlistFilters(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80}, // person
		{classID: 2, score: 0.9, cx: 300, cy: 200, w: 40, h: 80}, // car
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)

	cam := CameraDetectionConfig{Enabled: true, ClassAllowlist: []string{"car"}}
	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 || dets[0].Class != "car" {
		t.Fatalf("want single car detection, got %+v", dets)
	}
}

func TestROIFilter(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 40}, // inside ROI
		{classID: 0, score: 0.9, cx: 500, cy: 400, w: 40, h: 40}, // outside
	}, len(GenericClasses))
	cfg := defaultCfg()
	cfg.ROIOverlapThreshold = 0.5
	d, _ := newTestDetector(t, cfg, out, nil)

	roi := Polygon{{50, 50}, {200, 50}, {200, 200}, {50, 200}}
	cam := CameraDetectionConfig{Enabled: true, ROIs: []Polygon{roi}}
	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection inside ROI, got %d", len(dets))
	}
	if cx, cy := dets[0].BoundingBox.Center(); cx != 100 || cy != 100 {
		t.Errorf("wrong surviving box center: (%v, %v)", cx, cy)
	}
}

func TestMinBoxAreaFilter(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 4, h: 4},   // tiny
		{classID: 0, score: 0.9, cx: 300, cy: 200, w: 40, h: 80}, // kept
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)

	cam := CameraDetectionConfig{Enabled: true, MinBoxArea: 500}
	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection, got %d", len(dets))
	}
}

func TestCooldownSuppressesDuplicates(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)

	cam := CameraDetectionConfig{Enabled: true, CooldownSeconds: 10}
	// First frame passes.
	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil || len(dets) != 1 {
		t.Fatalf("first frame: want 1 got %d err=%v", len(dets), err)
	}
	// Second identical frame should be suppressed.
	dets, err = d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if len(dets) != 0 {
		t.Fatalf("cooldown failed: got %d detections", len(dets))
	}
}

func TestNMSCollapsesOverlappingBoxes(t *testing.T) {
	// Two nearly identical boxes of the same class.
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
		{classID: 0, score: 0.8, cx: 102, cy: 102, w: 40, h: 80},
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)

	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"),
		CameraDetectionConfig{Enabled: true})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("NMS failed: got %d detections, want 1", len(dets))
	}
	if math.Abs(dets[0].Confidence-0.9) > 1e-5 {
		t.Errorf("wrong survivor: %+v", dets[0])
	}
}

func TestPerVerticalClassMap(t *testing.T) {
	// Using HealthcareClasses, class id 3 should resolve to fall_event.
	out := buildYOLOOutput([]rawBox{
		{classID: 3, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(HealthcareClasses))
	cfg := defaultCfg()
	cfg.ClassMap = HealthcareClasses
	d, _ := newTestDetector(t, cfg, out, nil)

	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"),
		CameraDetectionConfig{Enabled: true})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 1 || dets[0].Class != "fall_event" {
		t.Fatalf("want fall_event, got %+v", dets)
	}
}

func TestConcurrentProcessFrameThreadSafe(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
		{classID: 2, score: 0.9, cx: 400, cy: 300, w: 60, h: 40},
	}, len(GenericClasses))
	sink := NewInMemorySink()
	d, _ := newTestDetector(t, defaultCfg(), out, sink)

	cam := CameraDetectionConfig{Enabled: true, CooldownSeconds: 0}
	var wg sync.WaitGroup
	const workers = 16
	const iters = 50
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"), cam)
				if err != nil {
					t.Errorf("concurrent ProcessFrame: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	// Each frame emits 2 detections, no cooldown in this test.
	if got := sink.Len(); got != workers*iters*2 {
		t.Errorf("sink received %d events, want %d", got, workers*iters*2)
	}
}

func TestEventSinkReceivesDetections(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(GenericClasses))
	sink := NewInMemorySink()
	d, _ := newTestDetector(t, defaultCfg(), out, sink)

	_, err := d.ProcessFrame(context.Background(), dummyFrame("cam-xyz"),
		CameraDetectionConfig{Enabled: true})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("want 1 sink event, got %d", len(events))
	}
	e := events[0]
	if e.CameraID != "cam-xyz" || e.Class != "person" || e.Timestamp.IsZero() {
		t.Errorf("unexpected envelope: %+v", e)
	}
}

func TestDisabledCameraReturnsNoDetections(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(GenericClasses))
	d, inf := newTestDetector(t, defaultCfg(), out, nil)

	dets, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"),
		CameraDetectionConfig{Enabled: false})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if len(dets) != 0 {
		t.Errorf("disabled camera produced detections: %+v", dets)
	}
	if inf.calls != 0 {
		t.Errorf("disabled camera should skip inference, got %d calls", inf.calls)
	}
}

func TestModelNotFoundError(t *testing.T) {
	reg := stubRegistry{models: map[string][]byte{}} // empty
	base := fake.New(fake.WithRegistry(reg))

	cfg := defaultCfg()
	cfg.ModelID = "missing-model"
	_, err := New(cfg, base)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("want ErrModelNotFound, got %v", err)
	}
}

func TestClosedDetectorReturnsError(t *testing.T) {
	out := buildYOLOOutput([]rawBox{
		{classID: 0, score: 0.9, cx: 100, cy: 100, w: 40, h: 80},
	}, len(GenericClasses))
	d, _ := newTestDetector(t, defaultCfg(), out, nil)
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := d.ProcessFrame(context.Background(), dummyFrame("cam-1"),
		CameraDetectionConfig{Enabled: true})
	if !errors.Is(err, ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestInvalidConfig(t *testing.T) {
	base := fake.New()
	_, err := New(Config{}, base)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("want ErrInvalidConfig, got %v", err)
	}
}
