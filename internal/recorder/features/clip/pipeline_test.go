package clip

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
	"github.com/bluenviron/mediamtx/internal/shared/inference/fake"
)

// fakeRegistry resolves any model id to a fixed byte slice.
type fakeRegistry struct{}

func (r *fakeRegistry) Resolve(_ context.Context, _ string) ([]byte, string, error) {
	return []byte("fake-model-bytes"), "v1", nil
}

var _ inference.ModelRegistry = (*fakeRegistry)(nil)

// newFakeInf creates a fake inferencer with a registry so LoadModel works.
func newFakeInf(opts ...fake.Option) *fake.Inferencer {
	return fake.New(append([]fake.Option{fake.WithRegistry(&fakeRegistry{})}, opts...)...)
}

// makeFakeFrame builds a minimal valid frame with float32 tensor data for
// the fake backend. The data length must equal the product of shape dims * 4.
func makeFakeFrame(cameraID string, capturedAt time.Time, dim int) Frame {
	// Build a [1, dim] float32 tensor filled with 0.5 values.
	data := make([]byte, dim*4)
	for i := 0; i < dim; i++ {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(0.5))
	}
	return Frame{
		CameraID:   cameraID,
		CapturedAt: capturedAt,
		Width:      224,
		Height:     224,
		Tensor: InferenceInput{
			Name:  "input",
			Shape: []int{1, dim},
			DType: "float32",
			Data:  data,
		},
	}
}

func TestNew_LoadsModel(t *testing.T) {
	inf := newFakeInf()
	defer inf.Close()

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           512,
		MaxConcurrentInferences: 2,
	}
	p, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	stats := inf.Stats()
	if stats.ModelsLoaded != 1 {
		t.Errorf("expected 1 model loaded, got %d", stats.ModelsLoaded)
	}
}

func TestNew_NilInferencer(t *testing.T) {
	cfg := Config{ModelID: "clip-vit-b32", EmbeddingDim: 512, MaxConcurrentInferences: 1}
	_, err := New(cfg, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestProcessFrame_Enabled(t *testing.T) {
	inf := newFakeInf(fake.WithOutputMode(fake.OutputOnes))
	defer inf.Close()

	sink := NewInMemorySink()
	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8, // small for test
		MaxConcurrentInferences: 2,
		SampleInterval:          0, // no rate limit
	}
	p, err := New(cfg, inf, WithSink(sink))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	frame := makeFakeFrame("cam-1", time.Now(), 8)
	emb, err := p.ProcessFrame(context.Background(), frame, CameraConfig{Enabled: true})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if emb == nil {
		t.Fatal("expected non-nil embedding")
	}
	if len(emb.Vector) != 8 {
		t.Errorf("expected vector dim 8, got %d", len(emb.Vector))
	}
	if emb.CameraID != "cam-1" {
		t.Errorf("expected camera cam-1, got %s", emb.CameraID)
	}
	if emb.ModelID != "clip-vit-b32" {
		t.Errorf("expected model clip-vit-b32, got %s", emb.ModelID)
	}

	// Check embedding is L2-normalised.
	var norm float64
	for _, v := range emb.Vector {
		norm += float64(v) * float64(v)
	}
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}

	// Check sink received the embedding.
	if sink.Len() != 1 {
		t.Errorf("expected 1 embedding in sink, got %d", sink.Len())
	}
}

func TestProcessFrame_Disabled(t *testing.T) {
	inf := newFakeInf()
	defer inf.Close()

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 2,
	}
	p, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	frame := makeFakeFrame("cam-1", time.Now(), 8)
	emb, err := p.ProcessFrame(context.Background(), frame, CameraConfig{Enabled: false})
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if emb != nil {
		t.Error("expected nil embedding for disabled camera")
	}
}

func TestProcessFrame_RateLimit(t *testing.T) {
	inf := newFakeInf(fake.WithOutputMode(fake.OutputOnes))
	defer inf.Close()

	sink := NewInMemorySink()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	advanceClock := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 2,
		SampleInterval:          1 * time.Second,
	}
	p, err := New(cfg, inf, WithSink(sink), withClock(clock))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	camCfg := CameraConfig{Enabled: true}

	// First frame should succeed.
	frame1 := makeFakeFrame("cam-1", now, 8)
	emb1, err := p.ProcessFrame(context.Background(), frame1, camCfg)
	if err != nil {
		t.Fatalf("ProcessFrame 1: %v", err)
	}
	if emb1 == nil {
		t.Fatal("first frame should produce embedding")
	}

	// Second frame at same time should be rate-limited.
	frame2 := makeFakeFrame("cam-1", now, 8)
	emb2, err := p.ProcessFrame(context.Background(), frame2, camCfg)
	if err != nil {
		t.Fatalf("ProcessFrame 2: %v", err)
	}
	if emb2 != nil {
		t.Error("second frame should be rate-limited")
	}

	// Advance clock past sample interval.
	advanceClock(2 * time.Second)
	frame3 := makeFakeFrame("cam-1", now, 8)
	emb3, err := p.ProcessFrame(context.Background(), frame3, camCfg)
	if err != nil {
		t.Fatalf("ProcessFrame 3: %v", err)
	}
	if emb3 == nil {
		t.Fatal("third frame should produce embedding after interval")
	}

	if sink.Len() != 2 {
		t.Errorf("expected 2 embeddings, got %d", sink.Len())
	}
}

func TestProcessFrame_PerCameraSampleInterval(t *testing.T) {
	inf := newFakeInf(fake.WithOutputMode(fake.OutputOnes))
	defer inf.Close()

	sink := NewInMemorySink()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	advanceClock := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 2,
		SampleInterval:          10 * time.Second, // pipeline default: 10s
	}
	p, err := New(cfg, inf, WithSink(sink), withClock(clock))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	// Per-camera override: 1s interval.
	camCfg := CameraConfig{
		Enabled:        true,
		SampleInterval: 1 * time.Second,
	}

	frame1 := makeFakeFrame("cam-1", now, 8)
	emb1, _ := p.ProcessFrame(context.Background(), frame1, camCfg)
	if emb1 == nil {
		t.Fatal("first frame should produce embedding")
	}

	advanceClock(2 * time.Second) // past per-camera 1s, but within pipeline 10s
	frame2 := makeFakeFrame("cam-1", now, 8)
	emb2, _ := p.ProcessFrame(context.Background(), frame2, camCfg)
	if emb2 == nil {
		t.Fatal("should use per-camera interval, not pipeline default")
	}
}

func TestProcessFrame_ResourceBudget(t *testing.T) {
	// Use a slow fake to hold the budget slots.
	inf := newFakeInf(
		fake.WithOutputMode(fake.OutputOnes),
		fake.WithLatency(200*time.Millisecond),
	)
	defer inf.Close()

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 1, // only 1 slot
		SampleInterval:          0,
		BudgetTimeout:           50 * time.Millisecond,
	}
	p, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	camCfg := CameraConfig{Enabled: true}

	// Start one inference that will hold the slot for 200ms.
	var wg sync.WaitGroup
	wg.Add(1)
	var firstErr error
	go func() {
		defer wg.Done()
		frame := makeFakeFrame("cam-1", time.Now(), 8)
		_, firstErr = p.ProcessFrame(context.Background(), frame, camCfg)
	}()

	// Give the first goroutine time to acquire the slot.
	time.Sleep(20 * time.Millisecond)

	// Second frame should be dropped (budget timeout 50ms < 200ms latency).
	frame2 := makeFakeFrame("cam-2", time.Now(), 8)
	emb2, err := p.ProcessFrame(context.Background(), frame2, camCfg)
	if err != nil {
		t.Fatalf("ProcessFrame: %v", err)
	}
	if emb2 != nil {
		t.Error("expected nil embedding when budget exhausted")
	}

	wg.Wait()
	if firstErr != nil {
		t.Fatalf("first ProcessFrame: %v", firstErr)
	}
}

func TestProcessFrame_ConcurrentCameras(t *testing.T) {
	inf := newFakeInf(fake.WithOutputMode(fake.OutputOnes))
	defer inf.Close()

	sink := NewInMemorySink()
	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 4,
		SampleInterval:          0,
	}
	p, err := New(cfg, inf, WithSink(sink))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	camCfg := CameraConfig{Enabled: true}
	var wg sync.WaitGroup
	var errCount atomic.Int32
	numCameras := 10

	for i := 0; i < numCameras; i++ {
		wg.Add(1)
		cameraID := "cam-" + string(rune('A'+i))
		go func() {
			defer wg.Done()
			frame := makeFakeFrame(cameraID, time.Now(), 8)
			_, err := p.ProcessFrame(context.Background(), frame, camCfg)
			if err != nil {
				errCount.Add(1)
			}
		}()
	}

	wg.Wait()
	if errCount.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", errCount.Load())
	}
	if sink.Len() != numCameras {
		t.Errorf("expected %d embeddings, got %d", numCameras, sink.Len())
	}
}

func TestProcessFrame_Closed(t *testing.T) {
	inf := newFakeInf()
	defer inf.Close()

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 2,
	}
	p, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.Close()

	frame := makeFakeFrame("cam-1", time.Now(), 8)
	_, err = p.ProcessFrame(context.Background(), frame, CameraConfig{Enabled: true})
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestProcessFrame_InvalidFrame(t *testing.T) {
	inf := newFakeInf()
	defer inf.Close()

	cfg := Config{
		ModelID:                 "clip-vit-b32",
		EmbeddingDim:           8,
		MaxConcurrentInferences: 2,
	}
	p, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	frame := Frame{CameraID: "cam-1", Width: 0, Height: 0}
	_, err = p.ProcessFrame(context.Background(), frame, CameraConfig{Enabled: true})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Errorf("expected ErrInvalidFrame, got %v", err)
	}
}

func TestNormalise(t *testing.T) {
	v := []float32{3, 4}
	normalise(v)

	// Expected: [0.6, 0.8]
	if math.Abs(float64(v[0])-0.6) > 1e-5 {
		t.Errorf("expected v[0]=0.6, got %f", v[0])
	}
	if math.Abs(float64(v[1])-0.8) > 1e-5 {
		t.Errorf("expected v[1]=0.8, got %f", v[1])
	}

	// L2 norm should be 1.
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1.0) > 1e-5 {
		t.Errorf("expected L2 norm 1.0, got %f", norm)
	}
}

func TestNormalise_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	normalise(v) // should not panic or produce NaN
	for _, x := range v {
		if x != 0 {
			t.Errorf("expected 0, got %f", x)
		}
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid",
			cfg:     Config{ModelID: "m", EmbeddingDim: 512, MaxConcurrentInferences: 2},
			wantErr: false,
		},
		{
			name:    "missing model id",
			cfg:     Config{EmbeddingDim: 512, MaxConcurrentInferences: 2},
			wantErr: true,
		},
		{
			name:    "zero embedding dim",
			cfg:     Config{ModelID: "m", EmbeddingDim: 0, MaxConcurrentInferences: 2},
			wantErr: true,
		},
		{
			name:    "negative sample interval",
			cfg:     Config{ModelID: "m", EmbeddingDim: 512, MaxConcurrentInferences: 2, SampleInterval: -1},
			wantErr: true,
		},
		{
			name:    "zero concurrent inferences",
			cfg:     Config{ModelID: "m", EmbeddingDim: 512, MaxConcurrentInferences: 0},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.ModelID != "clip-vit-b32" {
		t.Errorf("expected default model id clip-vit-b32, got %s", cfg.ModelID)
	}
	if cfg.EmbeddingDim != DefaultEmbeddingDim {
		t.Errorf("expected default dim %d, got %d", DefaultEmbeddingDim, cfg.EmbeddingDim)
	}
	if cfg.SampleInterval != DefaultSampleInterval {
		t.Errorf("expected default interval %v, got %v", DefaultSampleInterval, cfg.SampleInterval)
	}
	if cfg.MaxConcurrentInferences != DefaultMaxConcurrentInferences {
		t.Errorf("expected default max concurrent %d, got %d", DefaultMaxConcurrentInferences, cfg.MaxConcurrentInferences)
	}
}
