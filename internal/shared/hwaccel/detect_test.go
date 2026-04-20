package hwaccel

import (
	"context"
	"fmt"
	"testing"
)

// mockRunner returns a commandRunner that returns predefined output for
// specific command names.
func mockRunner(responses map[string]struct {
	out []byte
	err error
}) commandRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		key := name
		if len(args) > 0 {
			key = fmt.Sprintf("%s %s", name, args[0])
		}
		if r, ok := responses[key]; ok {
			return r.out, r.err
		}
		if r, ok := responses[name]; ok {
			return r.out, r.err
		}
		return nil, fmt.Errorf("command not found: %s", name)
	}
}

func TestDetector_NoHardware(t *testing.T) {
	d := &Detector{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	info := d.Detect(false)
	if info == nil {
		t.Fatal("expected non-nil HardwareInfo")
	}
	if info.Recommended.AccelType != AccelNone {
		t.Errorf("expected AccelNone, got %s", info.Recommended.AccelType)
	}
	if info.Recommended.Backend != "cpu" {
		t.Errorf("expected cpu backend, got %s", info.Recommended.Backend)
	}
}

func TestDetector_CachedResult(t *testing.T) {
	d := &Detector{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	info1 := d.Detect(false)
	info2 := d.Detect(false)
	// Cached result should be the exact same pointer.
	if info1 != info2 {
		t.Error("expected cached result to return same pointer")
	}
}

func TestDetector_ForceReprobe(t *testing.T) {
	d := &Detector{
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	info1 := d.Detect(false)
	info2 := d.Detect(true)
	// Force should produce a new result.
	if info1 == info2 {
		t.Error("expected force to produce a new HardwareInfo pointer")
	}
}

func TestRecommend_NVIDIA(t *testing.T) {
	info := &HardwareInfo{
		NVIDIA: &NVIDIAInfo{
			Available: true,
			GPUs: []GPUInfo{
				{Index: 0, Name: "Tesla T4", MemoryMB: 16384},
			},
		},
	}
	rec := recommend(info)
	if rec.AccelType != AccelCUDA {
		t.Errorf("expected CUDA, got %s", rec.AccelType)
	}
	if rec.Backend != "cuda" {
		t.Errorf("expected cuda backend, got %s", rec.Backend)
	}
}

func TestRecommend_QSV(t *testing.T) {
	info := &HardwareInfo{
		IntelQSV: &QSVInfo{
			Available: true,
			Device:    "/dev/dri/renderD128",
		},
	}
	rec := recommend(info)
	if rec.AccelType != AccelQSV {
		t.Errorf("expected QSV, got %s", rec.AccelType)
	}
	if rec.Backend != "openvino" {
		t.Errorf("expected openvino backend, got %s", rec.Backend)
	}
}

func TestRecommend_VAAPI(t *testing.T) {
	info := &HardwareInfo{
		VAAPI: &VAAPIInfo{
			Available: true,
			Device:    "/dev/dri/renderD128",
			Profiles: []VAAPIProfile{
				{Name: "VAProfileH264Main", Entrypoint: "VAEntrypointVLD"},
			},
		},
	}
	rec := recommend(info)
	if rec.AccelType != AccelVAAPI {
		t.Errorf("expected VAAPI, got %s", rec.AccelType)
	}
	if rec.Backend != "cpu" {
		t.Errorf("expected cpu backend for VAAPI, got %s", rec.Backend)
	}
}

func TestRecommend_Priority(t *testing.T) {
	// When both NVIDIA and QSV are available, NVIDIA should win.
	info := &HardwareInfo{
		NVIDIA: &NVIDIAInfo{
			Available: true,
			GPUs: []GPUInfo{
				{Index: 0, Name: "RTX 3090", MemoryMB: 24576},
			},
		},
		IntelQSV: &QSVInfo{
			Available: true,
			Device:    "/dev/dri/renderD128",
		},
	}
	rec := recommend(info)
	if rec.AccelType != AccelCUDA {
		t.Errorf("expected CUDA over QSV, got %s", rec.AccelType)
	}
}
