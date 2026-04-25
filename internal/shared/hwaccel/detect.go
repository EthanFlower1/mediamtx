// Package hwaccel detects hardware acceleration capabilities (NVIDIA CUDA,
// Intel QSV, VAAPI) on the host system and recommends an AI backend
// configuration for the NVR.
package hwaccel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AcceleratorType identifies a hardware acceleration technology.
type AcceleratorType string

const (
	AccelNone  AcceleratorType = "none"
	AccelCUDA  AcceleratorType = "cuda"
	AccelQSV   AcceleratorType = "qsv"
	AccelVAAPI AcceleratorType = "vaapi"
)

// GPUInfo describes a detected GPU.
type GPUInfo struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	MemoryMB   int    `json:"memory_mb,omitempty"`
	DriverVer  string `json:"driver_version,omitempty"`
	CUDAVer    string `json:"cuda_version,omitempty"`
}

// VAAPIProfile describes a VAAPI profile reported by vainfo.
type VAAPIProfile struct {
	Name       string `json:"name"`
	Entrypoint string `json:"entrypoint"`
}

// HardwareInfo holds the full detection result.
type HardwareInfo struct {
	Platform       string          `json:"platform"`
	CPUModel       string          `json:"cpu_model,omitempty"`
	NVIDIA         *NVIDIAInfo     `json:"nvidia,omitempty"`
	IntelQSV       *QSVInfo        `json:"intel_qsv,omitempty"`
	VAAPI          *VAAPIInfo      `json:"vaapi,omitempty"`
	Recommended    Recommendation  `json:"recommended"`
	DetectedAt     time.Time       `json:"detected_at"`
}

// NVIDIAInfo holds NVIDIA detection results.
type NVIDIAInfo struct {
	Available bool      `json:"available"`
	GPUs      []GPUInfo `json:"gpus"`
	SMIPath   string    `json:"smi_path,omitempty"`
}

// QSVInfo holds Intel QSV detection results.
type QSVInfo struct {
	Available bool   `json:"available"`
	Device    string `json:"device,omitempty"`
	Details   string `json:"details,omitempty"`
}

// VAAPIInfo holds VAAPI detection results.
type VAAPIInfo struct {
	Available bool           `json:"available"`
	Device    string         `json:"device,omitempty"`
	Profiles  []VAAPIProfile `json:"profiles,omitempty"`
}

// Recommendation is the auto-configuration suggestion for the AI backend.
type Recommendation struct {
	AccelType   AcceleratorType `json:"accel_type"`
	Backend     string          `json:"backend"`
	Device      string          `json:"device,omitempty"`
	Reason      string          `json:"reason"`
}

// commandRunner abstracts exec.CommandContext for testing.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// defaultRunner uses os/exec.
func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Detector probes the host for hardware acceleration capabilities.
type Detector struct {
	run commandRunner

	mu     sync.Mutex
	cached *HardwareInfo
}

// NewDetector creates a Detector with the default command runner.
func NewDetector() *Detector {
	return &Detector{run: defaultRunner}
}

// Detect probes the system and returns a HardwareInfo result.  Results are
// cached after the first successful call; pass force=true to re-probe.
func (d *Detector) Detect(force bool) *HardwareInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil && !force {
		return d.cached
	}
	info := d.detect()
	d.cached = info
	return info
}

func (d *Detector) detect() *HardwareInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	info := &HardwareInfo{
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		CPUModel:   detectCPUModel(),
		DetectedAt: time.Now().UTC(),
	}

	// Probe all accelerators concurrently.
	var wg sync.WaitGroup
	var nvidia *NVIDIAInfo
	var qsv *QSVInfo
	var vaapi *VAAPIInfo

	wg.Add(3)
	go func() { defer wg.Done(); nvidia = d.probeNVIDIA(ctx) }()
	go func() { defer wg.Done(); qsv = d.probeQSV(ctx) }()
	go func() { defer wg.Done(); vaapi = d.probeVAAPI(ctx) }()
	wg.Wait()

	if nvidia != nil && nvidia.Available {
		info.NVIDIA = nvidia
	}
	if qsv != nil && qsv.Available {
		info.IntelQSV = qsv
	}
	if vaapi != nil && vaapi.Available {
		info.VAAPI = vaapi
	}

	info.Recommended = recommend(info)
	return info
}

// probeNVIDIA checks for nvidia-smi and queries GPU details.
func (d *Detector) probeNVIDIA(ctx context.Context) *NVIDIAInfo {
	smiPath, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return &NVIDIAInfo{Available: false}
	}

	out, err := d.run(ctx, smiPath, "--query-gpu=index,name,memory.total,driver_version", "--format=csv,noheader,nounits")
	if err != nil {
		return &NVIDIAInfo{Available: false, SMIPath: smiPath}
	}

	var gpus []GPUInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ", ", 4)
		if len(parts) < 2 {
			continue
		}
		gpu := GPUInfo{
			Name: strings.TrimSpace(parts[1]),
		}
		fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &gpu.Index)
		if len(parts) >= 3 {
			fmt.Sscanf(strings.TrimSpace(parts[2]), "%d", &gpu.MemoryMB)
		}
		if len(parts) >= 4 {
			gpu.DriverVer = strings.TrimSpace(parts[3])
		}
		gpus = append(gpus, gpu)
	}

	// Try to get CUDA version from nvidia-smi header output.
	headerOut, herr := d.run(ctx, smiPath)
	if herr == nil {
		re := regexp.MustCompile(`CUDA Version:\s*([\d.]+)`)
		if m := re.FindStringSubmatch(string(headerOut)); len(m) > 1 {
			for i := range gpus {
				gpus[i].CUDAVer = m[1]
			}
		}
	}

	return &NVIDIAInfo{
		Available: len(gpus) > 0,
		GPUs:      gpus,
		SMIPath:   smiPath,
	}
}

// probeQSV checks for Intel QSV by looking for the i915 driver and
// /dev/dri/renderD128 on Linux, or via vainfo output on supported platforms.
func (d *Detector) probeQSV(ctx context.Context) *QSVInfo {
	if runtime.GOOS != "linux" {
		return &QSVInfo{Available: false}
	}

	// Check for render device.
	device := "/dev/dri/renderD128"
	if _, err := os.Stat(device); err != nil {
		return &QSVInfo{Available: false}
	}

	// Try vainfo with the render device to confirm Intel QSV.
	out, err := d.run(ctx, "vainfo", "--display", "drm", "--device", device)
	if err != nil {
		// vainfo not installed but device exists -- may still have QSV.
		return &QSVInfo{
			Available: true,
			Device:    device,
			Details:   "render device found, vainfo not available for detailed probe",
		}
	}

	outStr := string(out)
	if strings.Contains(strings.ToLower(outStr), "intel") || strings.Contains(outStr, "iHD") {
		return &QSVInfo{
			Available: true,
			Device:    device,
			Details:   "Intel QSV via iHD/i965 driver confirmed",
		}
	}

	return &QSVInfo{Available: false, Device: device}
}

// probeVAAPI checks for VAAPI support via vainfo.
func (d *Detector) probeVAAPI(ctx context.Context) *VAAPIInfo {
	if runtime.GOOS != "linux" {
		return &VAAPIInfo{Available: false}
	}

	device := "/dev/dri/renderD128"
	if _, err := os.Stat(device); err != nil {
		return &VAAPIInfo{Available: false}
	}

	out, err := d.run(ctx, "vainfo")
	if err != nil {
		return &VAAPIInfo{Available: false, Device: device}
	}

	outStr := string(out)
	var profiles []VAAPIProfile
	re := regexp.MustCompile(`(VAProfile\w+)\s*:\s*(VAEntrypoint\w+)`)
	for _, m := range re.FindAllStringSubmatch(outStr, -1) {
		profiles = append(profiles, VAAPIProfile{
			Name:       m[1],
			Entrypoint: m[2],
		})
	}

	return &VAAPIInfo{
		Available: len(profiles) > 0,
		Device:    device,
		Profiles:  profiles,
	}
}

// recommend picks the best accelerator for AI inference.
func recommend(info *HardwareInfo) Recommendation {
	// NVIDIA CUDA is the best choice for AI workloads.
	if info.NVIDIA != nil && info.NVIDIA.Available && len(info.NVIDIA.GPUs) > 0 {
		gpu := info.NVIDIA.GPUs[0]
		return Recommendation{
			AccelType: AccelCUDA,
			Backend:   "cuda",
			Device:    fmt.Sprintf("gpu:%d", gpu.Index),
			Reason:    fmt.Sprintf("NVIDIA %s detected with %d MB VRAM; CUDA provides best AI inference performance", gpu.Name, gpu.MemoryMB),
		}
	}

	// Intel QSV is second choice -- good for video decode offload.
	if info.IntelQSV != nil && info.IntelQSV.Available {
		return Recommendation{
			AccelType: AccelQSV,
			Backend:   "openvino",
			Device:    info.IntelQSV.Device,
			Reason:    "Intel QSV detected; OpenVINO backend recommended for Intel hardware AI inference",
		}
	}

	// VAAPI can offload decode but AI runs on CPU.
	if info.VAAPI != nil && info.VAAPI.Available {
		return Recommendation{
			AccelType: AccelVAAPI,
			Backend:   "cpu",
			Device:    info.VAAPI.Device,
			Reason:    "VAAPI detected for video decode offload; AI inference will use CPU (no dedicated AI accelerator found)",
		}
	}

	return Recommendation{
		AccelType: AccelNone,
		Backend:   "cpu",
		Reason:    "No hardware acceleration detected; AI inference will use CPU",
	}
}

// detectCPUModel returns a human-readable CPU model string.
func detectCPUModel() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}
