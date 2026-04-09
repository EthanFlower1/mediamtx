package router

import (
	"context"
	"os"
	"runtime"
	"time"
)

// HardwareProbe snapshots the inference-relevant hardware on the local
// system.  It is injected into the Router so tests can supply a fake and
// production can supply a real NVIDIA-aware implementation.
//
// Implementations MUST be safe for concurrent use.  Probe SHOULD be cheap
// enough to call on hardware-change events (startup, udev, udev-like hook);
// expensive vendor-SDK calls should be cached inside the implementation.
type HardwareProbe interface {
	Probe(ctx context.Context) (HardwareCapability, error)
}

// StaticProbe is a HardwareProbe that always returns the same capability.
// It is the primary fake used by unit tests and by on-prem deployments where
// the operator wants to pin routing behaviour (e.g. "this appliance has a
// GPU, do not re-probe").
type StaticProbe struct {
	// Capability is the value returned by Probe.  Its ProbedAt is stamped at
	// Probe time if left zero.
	Capability HardwareCapability
}

// Probe implements HardwareProbe.
func (s *StaticProbe) Probe(_ context.Context) (HardwareCapability, error) {
	hw := s.Capability
	if hw.ProbedAt.IsZero() {
		hw.ProbedAt = time.Now().UTC()
	}
	return hw, nil
}

// NewStaticProbe returns a StaticProbe with the given capability.
func NewStaticProbe(hw HardwareCapability) HardwareProbe {
	return &StaticProbe{Capability: hw}
}

// LinuxNVIDIAProbe is the default production probe.  It detects an NVIDIA
// GPU by checking for /proc/driver/nvidia (present whenever the nvidia
// kernel module is loaded, regardless of whether nvidia-smi is installed).
// It deliberately avoids shelling out to nvidia-smi so that the Recorder
// binary has no runtime dependency on the CUDA toolkit.
//
// On non-Linux hosts Probe returns a GPU-absent capability.  On Linux hosts
// without an NVIDIA driver it also returns GPU-absent.  Callers that need
// finer-grained detection (AMD ROCm, Apple Metal, Intel Arc) should provide
// a different HardwareProbe implementation.
type LinuxNVIDIAProbe struct {
	// cpuClass is cached at construction time (runtime.NumCPU is cheap but
	// callers may want to override it for tests).
	cpuClass string
}

// NewLinuxNVIDIAProbe returns a HardwareProbe suitable for Linux NVR
// appliances.  CPU class is derived from runtime.NumCPU.
func NewLinuxNVIDIAProbe() HardwareProbe {
	return &LinuxNVIDIAProbe{cpuClass: classifyCPU(runtime.NumCPU())}
}

// Probe implements HardwareProbe.
func (p *LinuxNVIDIAProbe) Probe(_ context.Context) (HardwareCapability, error) {
	hw := HardwareCapability{
		CPUClass: p.cpuClass,
		ProbedAt: time.Now().UTC(),
	}
	if runtime.GOOS != "linux" {
		return hw, nil
	}
	if _, err := os.Stat("/proc/driver/nvidia"); err != nil {
		return hw, nil
	}
	// Count GPUs via /proc/driver/nvidia/gpus/<bus-id> directories.
	entries, err := os.ReadDir("/proc/driver/nvidia/gpus")
	if err != nil || len(entries) == 0 {
		// NVIDIA driver present but no usable GPUs — treat as GPU absent.
		return hw, nil
	}
	hw.GPUPresent = true
	hw.GPUCount = len(entries)
	// VRAM detection via /proc/driver/nvidia is vendor-specific and fragile;
	// leave GPUMemoryMB=0 unless a richer probe is wired in.  Routing
	// decisions in defaultMatrix only consult GPUPresent today, so 0 is safe.
	return hw, nil
}

// classifyCPU maps a core count to the coarse "low/mid/high" label used by
// the routing matrix.  Values chosen to put typical NVR appliances in the
// "mid" bucket and beefy edge servers in "high".
func classifyCPU(numCPU int) string {
	switch {
	case numCPU <= 2:
		return "low"
	case numCPU <= 8:
		return "mid"
	default:
		return "high"
	}
}
