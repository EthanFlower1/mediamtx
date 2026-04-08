package fake_test

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

func float32Tensor(name string, shape []int, values []float32) inference.Tensor {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return inference.Tensor{Name: name, Shape: shape, DType: inference.DTypeFloat32, Data: buf}
}

func readFloat32(t *testing.T, tensor inference.Tensor) []float32 {
	t.Helper()
	if tensor.DType != inference.DTypeFloat32 {
		t.Fatalf("expected float32, got %s", tensor.DType)
	}
	n := tensor.NumElements()
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(tensor.Data[i*4:]))
	}
	return out
}

func TestRoundTripLoadInferUnload(t *testing.T) {
	ctx := context.Background()
	inf := fake.New()
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "unit-test-model", []byte("fake-weights"))
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if model.ID != "unit-test-model" {
		t.Errorf("model id = %q", model.ID)
	}
	if model.Backend != inference.BackendFake {
		t.Errorf("backend = %s", model.Backend)
	}

	input := float32Tensor("input", []int{1, 3}, []float32{1, 2, 3})
	result, err := inf.Infer(ctx, model, input)
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("want 1 output, got %d", len(result.Outputs))
	}
	out := readFloat32(t, result.Outputs[0])
	if len(out) != 3 || out[0] != 1 || out[1] != 2 || out[2] != 3 {
		t.Errorf("echo output = %v, want [1 2 3]", out)
	}
	if result.ModelID != "unit-test-model" {
		t.Errorf("result model id = %q", result.ModelID)
	}

	if err := inf.Unload(ctx, model); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	// Inferring after unload should fail.
	if _, err := inf.Infer(ctx, model, input); !errors.Is(err, inference.ErrModelNotLoaded) {
		t.Errorf("Infer after Unload err = %v, want ErrModelNotLoaded", err)
	}
}

func TestDeterministicOutputReproducible(t *testing.T) {
	ctx := context.Background()
	inf := fake.New(fake.WithOutputMode(fake.OutputDeterministicRandom))
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "det-model", []byte("w"))
	if err != nil {
		t.Fatal(err)
	}

	input := float32Tensor("in", []int{2, 4}, []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8})

	first, err := inf.Infer(ctx, model, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inf.Infer(ctx, model, input)
	if err != nil {
		t.Fatal(err)
	}

	a := first.Outputs[0].Data
	b := second.Outputs[0].Data
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("byte %d differs: %x vs %x", i, a[i], b[i])
		}
	}

	// Re-load the model in a fresh inferencer and verify the output
	// still matches — this is the "reproducible across runs" guarantee.
	inf2 := fake.New(fake.WithOutputMode(fake.OutputDeterministicRandom))
	defer inf2.Close()
	model2, err := inf2.LoadModel(ctx, "det-model", []byte("w"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := inf2.Infer(ctx, model2, input)
	if err != nil {
		t.Fatal(err)
	}
	c := third.Outputs[0].Data
	for i := range a {
		if a[i] != c[i] {
			t.Fatalf("cross-run reproducibility failed at byte %d: %x vs %x", i, a[i], c[i])
		}
	}
}

func TestOnesOutputMode(t *testing.T) {
	ctx := context.Background()
	inf := fake.New(fake.WithOutputMode(fake.OutputOnes))
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "ones", []byte{0})
	if err != nil {
		t.Fatal(err)
	}
	input := float32Tensor("in", []int{4}, []float32{9, 9, 9, 9})
	res, err := inf.Infer(ctx, model, input)
	if err != nil {
		t.Fatal(err)
	}
	got := readFloat32(t, res.Outputs[0])
	for i, v := range got {
		if v != 1 {
			t.Errorf("element %d = %v, want 1", i, v)
		}
	}
}

func TestStatsTrackInferenceCount(t *testing.T) {
	ctx := context.Background()
	inf := fake.New()
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "m1", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	input := float32Tensor("in", []int{1}, []float32{1})

	const n = 17
	for i := 0; i < n; i++ {
		if _, err := inf.Infer(ctx, model, input); err != nil {
			t.Fatalf("Infer %d: %v", i, err)
		}
	}

	stats := inf.Stats()
	if stats.TotalInferences != n {
		t.Errorf("TotalInferences = %d, want %d", stats.TotalInferences, n)
	}
	if stats.ModelsLoaded != 1 {
		t.Errorf("ModelsLoaded = %d, want 1", stats.ModelsLoaded)
	}
	if stats.Backend != inference.BackendFake {
		t.Errorf("Backend = %s", stats.Backend)
	}
	ms, ok := stats.PerModel["m1"]
	if !ok {
		t.Fatalf("PerModel missing m1")
	}
	if ms.InferenceCount != n {
		t.Errorf("PerModel[m1].InferenceCount = %d, want %d", ms.InferenceCount, n)
	}
}

func TestStatsTrackErrors(t *testing.T) {
	ctx := context.Background()
	inf := fake.New()
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "m1", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	// Invalid tensor: data length mismatches shape.
	bad := inference.Tensor{
		Name:  "bad",
		Shape: []int{4},
		DType: inference.DTypeFloat32,
		Data:  []byte{1, 2, 3}, // should be 16 bytes
	}
	if _, err := inf.Infer(ctx, model, bad); !errors.Is(err, inference.ErrInvalidTensor) {
		t.Errorf("err = %v, want ErrInvalidTensor", err)
	}
	stats := inf.Stats()
	if stats.TotalErrors == 0 {
		t.Errorf("TotalErrors = 0, want >=1")
	}
}

func TestConcurrentInferIsSafe(t *testing.T) {
	ctx := context.Background()
	inf := fake.New()
	defer inf.Close()

	// Multiple models loaded concurrently.
	var models []*inference.LoadedModel
	for i := 0; i < 4; i++ {
		m, err := inf.LoadModel(ctx, "m", []byte{byte(i)})
		if err != nil {
			t.Fatal(err)
		}
		models = append(models, m)
	}

	input := float32Tensor("in", []int{8}, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	const workers = 16
	const perWorker = 64

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			m := models[w%len(models)]
			for i := 0; i < perWorker; i++ {
				if _, err := inf.Infer(ctx, m, input); err != nil {
					errCh <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Infer: %v", err)
	}

	stats := inf.Stats()
	if stats.TotalInferences != workers*perWorker {
		t.Errorf("TotalInferences = %d, want %d", stats.TotalInferences, workers*perWorker)
	}
}

func TestCloseReleasesModels(t *testing.T) {
	ctx := context.Background()
	inf := fake.New()

	for i := 0; i < 3; i++ {
		if _, err := inf.LoadModel(ctx, "m", []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := inf.Stats().ModelsLoaded; got != 3 {
		t.Fatalf("before close: ModelsLoaded = %d", got)
	}
	if err := inf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := inf.Stats().ModelsLoaded; got != 0 {
		t.Errorf("after close: ModelsLoaded = %d, want 0", got)
	}
	// Close is idempotent.
	if err := inf.Close(); err != nil {
		t.Errorf("Close (2nd): %v", err)
	}
	// Subsequent operations return ErrClosed.
	if _, err := inf.LoadModel(ctx, "m", []byte{1}); !errors.Is(err, inference.ErrClosed) {
		t.Errorf("LoadModel after close err = %v, want ErrClosed", err)
	}
}

func TestWithLatencySimulatesSlowInference(t *testing.T) {
	ctx := context.Background()
	inf := fake.New(fake.WithLatency(20 * time.Millisecond))
	defer inf.Close()

	model, err := inf.LoadModel(ctx, "slow", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	input := float32Tensor("in", []int{1}, []float32{1})

	start := time.Now()
	res, err := inf.Infer(ctx, model, input)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 18*time.Millisecond {
		t.Errorf("elapsed %v < configured 20ms latency", elapsed)
	}
	if res.Latency <= 0 {
		t.Errorf("result latency = %v, want >0", res.Latency)
	}
}

func TestContextCancellationDuringSlowInfer(t *testing.T) {
	inf := fake.New(fake.WithLatency(500 * time.Millisecond))
	defer inf.Close()

	model, err := inf.LoadModel(context.Background(), "slow", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	input := float32Tensor("in", []int{1}, []float32{1})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = inf.Infer(ctx, model, input)
	if err == nil {
		t.Fatalf("expected context error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context error", err)
	}
}

// fakeRegistry is a trivial inference.ModelRegistry for testing
// the nil-bytes resolution path.
type fakeRegistry struct {
	store map[string][]byte
}

func (r *fakeRegistry) Resolve(ctx context.Context, id string) ([]byte, string, error) {
	b, ok := r.store[id]
	if !ok {
		return nil, "", inference.ErrModelNotFound
	}
	return b, "v1", nil
}

func TestLoadModelViaRegistry(t *testing.T) {
	reg := &fakeRegistry{store: map[string][]byte{"reg-model": {1, 2, 3}}}
	inf := fake.New(fake.WithRegistry(reg))
	defer inf.Close()

	model, err := inf.LoadModel(context.Background(), "reg-model", nil)
	if err != nil {
		t.Fatalf("LoadModel via registry: %v", err)
	}
	if model.Version != "v1" {
		t.Errorf("Version = %q, want v1", model.Version)
	}

	if _, err := inf.LoadModel(context.Background(), "missing", nil); !errors.Is(err, inference.ErrModelNotFound) {
		t.Errorf("err = %v, want ErrModelNotFound", err)
	}
}
