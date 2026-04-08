package inference_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
	"github.com/bluenviron/mediamtx/internal/shared/inference/fake"
)

func TestRouterEdgePreferred(t *testing.T) {
	r := inference.NewRouter()
	edge := fake.New(fake.WithName("edge-fake"))
	defer edge.Close()
	// Pretend the fake is an onnx runtime for routing purposes.
	r.RegisterEdge(fakeAsBackend(edge, inference.BackendONNXRuntime))

	hw := inference.HardwareCapability{
		HasGPU:   true,
		Backends: []inference.BackendKind{inference.BackendONNXRuntime},
	}
	dec, err := r.Pick(context.Background(), inference.FeatureLightweightObjectDetection, hw)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if dec.Location != inference.LocationEdge {
		t.Errorf("Location = %s, want edge", dec.Location)
	}
	if dec.Backend != inference.BackendONNXRuntime {
		t.Errorf("Backend = %s", dec.Backend)
	}
	if dec.EdgeInferencer == nil {
		t.Errorf("EdgeInferencer nil")
	}
}

func TestRouterFaceRecognitionFallsBackWithoutGPU(t *testing.T) {
	r := inference.NewRouter()
	edge := fake.New()
	defer edge.Close()
	r.RegisterEdge(fakeAsBackend(edge, inference.BackendONNXRuntime))

	hw := inference.HardwareCapability{
		HasGPU:   false,
		Backends: []inference.BackendKind{inference.BackendONNXRuntime},
	}
	dec, err := r.Pick(context.Background(), inference.FeatureFaceRecognition, hw)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if dec.Location != inference.LocationCloud {
		t.Errorf("Location = %s, want cloud", dec.Location)
	}
	if dec.Reason != "cloud:fallback:no-gpu" {
		t.Errorf("Reason = %q", dec.Reason)
	}
}

func TestRouterNoEdgeBackendFallsBack(t *testing.T) {
	r := inference.NewRouter()
	// No edge registered.
	hw := inference.HardwareCapability{
		HasGPU:   false,
		Backends: []inference.BackendKind{inference.BackendONNXRuntime},
	}
	dec, err := r.Pick(context.Background(), inference.FeatureLightweightObjectDetection, hw)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if dec.Location != inference.LocationCloud {
		t.Errorf("Location = %s, want cloud", dec.Location)
	}
}

func TestRouterUnknownFeature(t *testing.T) {
	r := inference.NewRouter()
	_, err := r.Pick(context.Background(), inference.Feature("nonsense"), inference.HardwareCapability{})
	if !errors.Is(err, inference.ErrUnsupportedFeature) {
		t.Errorf("err = %v, want ErrUnsupportedFeature", err)
	}
}

func TestRouterCloudPreferredOpportunisticEdge(t *testing.T) {
	// Force a feature to be cloud-preferred but without GPU requirement
	// so the opportunistic edge path triggers.
	r := inference.NewRouter()
	r.SetPolicy(inference.FeatureCLIPEmbedding, inference.FeaturePolicy{
		PreferredLocation:  inference.LocationCloud,
		RequireGPU:         false,
		AllowCloudFallback: true,
		PreferredBackends:  []inference.BackendKind{inference.BackendCoreML},
	})
	edge := fake.New()
	defer edge.Close()
	r.RegisterEdge(fakeAsBackend(edge, inference.BackendCoreML))

	hw := inference.HardwareCapability{
		HasGPU:   false,
		Backends: []inference.BackendKind{inference.BackendCoreML},
	}
	dec, err := r.Pick(context.Background(), inference.FeatureCLIPEmbedding, hw)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Location != inference.LocationEdge {
		t.Errorf("Location = %s, want edge (opportunistic)", dec.Location)
	}
	if dec.Reason != "edge:opportunistic" {
		t.Errorf("Reason = %q", dec.Reason)
	}
}

// fakeAsBackend wraps a fake.Inferencer so it reports a non-fake
// BackendKind for router tests. It only needs the Backend() method to
// differ from the underlying fake — other calls delegate.
type backendWrapper struct {
	inner   inference.Inferencer
	backend inference.BackendKind
}

func fakeAsBackend(inner inference.Inferencer, b inference.BackendKind) inference.Inferencer {
	return &backendWrapper{inner: inner, backend: b}
}

func (w *backendWrapper) Name() string                  { return w.inner.Name() }
func (w *backendWrapper) Backend() inference.BackendKind { return w.backend }
func (w *backendWrapper) LoadModel(ctx context.Context, id string, b []byte) (*inference.LoadedModel, error) {
	return w.inner.LoadModel(ctx, id, b)
}
func (w *backendWrapper) Infer(ctx context.Context, m *inference.LoadedModel, in inference.Tensor) (*inference.InferenceResult, error) {
	return w.inner.Infer(ctx, m, in)
}
func (w *backendWrapper) Unload(ctx context.Context, m *inference.LoadedModel) error {
	return w.inner.Unload(ctx, m)
}
func (w *backendWrapper) Stats() inference.Stats { return w.inner.Stats() }
func (w *backendWrapper) Close() error           { return w.inner.Close() }
