package router

import (
	"context"
	"testing"
)

// newTestRouter builds a Router with a StaticProbe and an optional per-tenant
// policy map.  Returns the constructed Router — test failures in New are
// fatal to keep table tests terse.
func newTestRouter(t *testing.T, hw HardwareCapability, policies map[string]TenantPolicy) Router {
	t.Helper()
	r, err := New(context.Background(), Config{
		Probe:    NewStaticProbe(hw),
		Policies: NewStaticPolicyStore(policies),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// TestMatrixCoveragePerHardware exercises every cell of § 11.2: each Feature
// is routed twice — once with a GPU-equipped appliance and once without —
// and the decision is asserted against the spec.
func TestMatrixCoveragePerHardware(t *testing.T) {
	withGPU := HardwareCapability{GPUPresent: true, GPUCount: 1, CPUClass: "mid"}
	noGPU := HardwareCapability{GPUPresent: false, CPUClass: "mid"}

	// Expected cells: [feature][gpu?]Location.  "_" as a short alias.
	type cell struct {
		feature     Feature
		withGPULoc  Location
		withoutLoc  Location
	}
	cases := []cell{
		{FeatureObjectDetectionLight, LocationEdge, LocationEdge},
		{FeatureObjectDetectionHeavy, LocationEdge, LocationCloud},
		{FeatureFace, LocationEdge, LocationCloud},
		{FeatureLPR, LocationEdge, LocationCloud},
		{FeatureBehavioral, LocationEdge, LocationEdge},
		{FeatureAudio, LocationEdge, LocationEdge},
		{FeatureCLIPEmbed, LocationEdge, LocationEdge},
		{FeatureVectorSearch, LocationCloud, LocationCloud},
		{FeatureCrossCameraTrack, LocationCloud, LocationCloud},
		{FeatureAnomaly, LocationCloud, LocationCloud},
		{FeatureSummary, LocationCloud, LocationCloud},
		{FeatureForensic, LocationCloud, LocationCloud},
		{FeatureCustom, LocationCloud, LocationCloud},
	}

	if len(cases) != len(AllFeatures()) {
		t.Fatalf("matrix coverage drift: cases=%d features=%d", len(cases), len(AllFeatures()))
	}

	rGPU := newTestRouter(t, withGPU, nil)
	rNoGPU := newTestRouter(t, noGPU, nil)

	for _, tc := range cases {
		t.Run(string(tc.feature), func(t *testing.T) {
			if d, err := rGPU.Route(context.Background(), tc.feature, "tenant-a"); err != nil {
				t.Fatalf("Route gpu: %v", err)
			} else if d.Location != tc.withGPULoc {
				t.Errorf("with GPU: got %s, want %s", d.Location, tc.withGPULoc)
			} else if d.Fallback {
				t.Errorf("with GPU: unexpected Fallback=true")
			}
			if d, err := rNoGPU.Route(context.Background(), tc.feature, "tenant-a"); err != nil {
				t.Fatalf("Route no-gpu: %v", err)
			} else if d.Location != tc.withoutLoc {
				t.Errorf("no GPU: got %s, want %s", d.Location, tc.withoutLoc)
			}
		})
	}
}

// TestTenantForceCloud verifies that a per-tenant force_cloud override wins
// over the matrix default, even when the feature would otherwise run edge.
func TestTenantForceCloud(t *testing.T) {
	hw := HardwareCapability{GPUPresent: true, GPUCount: 1, CPUClass: "high"}
	policies := map[string]TenantPolicy{
		"acme": {
			TenantID:   "acme",
			ForceCloud: map[Feature]bool{FeatureFace: true},
		},
	}
	r := newTestRouter(t, hw, policies)

	d, err := r.Route(context.Background(), FeatureFace, "acme")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.Location != LocationCloud {
		t.Errorf("got %s, want cloud", d.Location)
	}
	if d.Reason != "tenant policy: force_cloud" {
		t.Errorf("got reason %q", d.Reason)
	}

	// A different tenant must still get the matrix default (edge with GPU).
	d, err = r.Route(context.Background(), FeatureFace, "other")
	if err != nil {
		t.Fatalf("Route other: %v", err)
	}
	if d.Location != LocationEdge {
		t.Errorf("other tenant: got %s, want edge", d.Location)
	}
}

// TestTenantForceEdgeCannotOverrideCloudOnly verifies that a malicious or
// misconfigured tenant cannot push a cloud-only feature onto the edge.
func TestTenantForceEdgeCannotOverrideCloudOnly(t *testing.T) {
	hw := HardwareCapability{GPUPresent: true, GPUCount: 2, CPUClass: "high"}
	policies := map[string]TenantPolicy{
		"acme": {
			TenantID: "acme",
			ForceEdge: map[Feature]bool{
				FeatureForensic:         true,
				FeatureAnomaly:          true,
				FeatureCrossCameraTrack: true,
			},
		},
	}
	r := newTestRouter(t, hw, policies)

	for _, f := range []Feature{FeatureForensic, FeatureAnomaly, FeatureCrossCameraTrack} {
		d, err := r.Route(context.Background(), f, "acme")
		if err != nil {
			t.Fatalf("Route %s: %v", f, err)
		}
		if d.Location != LocationCloud {
			t.Errorf("%s: force_edge leaked cloud-only feature onto edge", f)
		}
	}
}

// TestEdgeFailureFallback covers the "no-restart fallback" requirement:
// after ReportEdgeFailure, an edge decision for that Feature flips to cloud
// with Fallback=true, and ClearEdgeFailure restores the matrix default.
func TestEdgeFailureFallback(t *testing.T) {
	hw := HardwareCapability{GPUPresent: true, GPUCount: 1, CPUClass: "mid"}
	r := newTestRouter(t, hw, nil)

	// Baseline: face runs at the edge.
	d, _ := r.Route(context.Background(), FeatureFace, "t1")
	if d.Location != LocationEdge || d.Fallback {
		t.Fatalf("baseline: got %+v, want edge non-fallback", d)
	}

	r.ReportEdgeFailure(FeatureFace)

	d, _ = r.Route(context.Background(), FeatureFace, "t1")
	if d.Location != LocationCloud || !d.Fallback {
		t.Errorf("post-failure: got %+v, want cloud fallback", d)
	}
	// Other features are unaffected.
	d, _ = r.Route(context.Background(), FeatureLPR, "t1")
	if d.Location != LocationEdge || d.Fallback {
		t.Errorf("unrelated feature should be unaffected: got %+v", d)
	}

	r.ClearEdgeFailure(FeatureFace)
	d, _ = r.Route(context.Background(), FeatureFace, "t1")
	if d.Location != LocationEdge || d.Fallback {
		t.Errorf("post-clear: got %+v, want edge non-fallback", d)
	}
}

// TestSaturationFallback covers the GPU-saturation branch of the edge
// fallback logic, which is separate from ReportEdgeFailure.
func TestSaturationFallback(t *testing.T) {
	hw := HardwareCapability{GPUPresent: true, GPUCount: 1, CPUClass: "mid"}
	r := newTestRouter(t, hw, nil)

	r.SetSaturated(true)
	d, _ := r.Route(context.Background(), FeatureFace, "t1")
	if d.Location != LocationCloud || !d.Fallback {
		t.Errorf("saturated: got %+v, want cloud fallback", d)
	}
	if d.Reason != "fallback: GPU saturated" {
		t.Errorf("reason: got %q", d.Reason)
	}

	r.SetSaturated(false)
	d, _ = r.Route(context.Background(), FeatureFace, "t1")
	if d.Location != LocationEdge || d.Fallback {
		t.Errorf("cleared: got %+v, want edge non-fallback", d)
	}
}

// TestUnknownFeatureReturnsError guards the Feature enum against silent
// misuse — routing an unknown Feature must error rather than default.
func TestUnknownFeatureReturnsError(t *testing.T) {
	hw := HardwareCapability{GPUPresent: true, CPUClass: "mid"}
	r := newTestRouter(t, hw, nil)

	_, err := r.Route(context.Background(), Feature("bogus"), "t1")
	if err == nil {
		t.Fatal("expected ErrUnknownFeature, got nil")
	}
}

// TestAllFeaturesHaveMatrixRow is an audit safeguard: if someone adds a new
// Feature constant without a corresponding matrixRow, this test fails.
func TestAllFeaturesHaveMatrixRow(t *testing.T) {
	for _, f := range AllFeatures() {
		if _, ok := matrixIndex[f]; !ok {
			t.Errorf("feature %q missing from defaultMatrix", f)
		}
	}
}

// TestHardwareCapabilitySummary gives a readable label to what ends up in
// the structured log field, so log scrapers can be written against it.
func TestHardwareCapabilitySummary(t *testing.T) {
	cases := []struct {
		hw   HardwareCapability
		want string
	}{
		{HardwareCapability{CPUClass: "low"}, "no_gpu cpu=low"},
		{HardwareCapability{GPUPresent: true, GPUCount: 2, GPUMemoryMB: 16384, CPUClass: "high"}, "gpu=2 vram_mb=16384 cpu=high"},
	}
	for _, tc := range cases {
		if got := tc.hw.Summary(); got != tc.want {
			t.Errorf("Summary: got %q, want %q", got, tc.want)
		}
	}
}

// TestLinuxNVIDIAProbeRunsEverywhere just asserts that the default
// production probe does not panic and returns a ProbedAt timestamp.  It
// MUST NOT require root or a GPU — on CI it will simply return GPU-absent.
func TestLinuxNVIDIAProbeRunsEverywhere(t *testing.T) {
	p := NewLinuxNVIDIAProbe()
	hw, err := p.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if hw.ProbedAt.IsZero() {
		t.Error("ProbedAt not stamped")
	}
	if hw.CPUClass == "" {
		t.Error("CPUClass not set")
	}
}
