package syscheck

import (
	"fmt"
	"net"
	"runtime"
	"syscall"
)

// Tier classifies NVR hardware capability.
type Tier string

const (
	TierInsufficient Tier = "insufficient"
	TierMini         Tier = "mini"         // 4-8 cameras
	TierMid          Tier = "mid"          // 16-32 cameras
	TierEnterprise   Tier = "enterprise"   // 64-128+ cameras
)

// Tier thresholds.
const (
	// Mini: 2 cores, 4 GB RAM, 100 GB disk
	MiniCPUCores   = 2
	MiniRAMBytes   = 4 * 1024 * 1024 * 1024
	MiniDiskBytes  = 100 * 1024 * 1024 * 1024

	// Mid: 4 cores, 8 GB RAM, 500 GB disk
	MidCPUCores   = 4
	MidRAMBytes   = 8 * 1024 * 1024 * 1024
	MidDiskBytes  = 500 * 1024 * 1024 * 1024

	// Enterprise: 8 cores, 16 GB RAM, 2 TB disk
	EntCPUCores   = 8
	EntRAMBytes   = 16 * 1024 * 1024 * 1024
	EntDiskBytes  = 2 * 1024 * 1024 * 1024 * 1024
)

// HardwareReport collects system hardware information for compatibility
// classification.
type HardwareReport struct {
	CPUCores    int      `json:"cpu_cores"`
	CPUArch     string   `json:"cpu_arch"`
	GOOS        string   `json:"goos"`
	TotalRAM    uint64   `json:"total_ram_bytes"`
	FreeDisk    uint64   `json:"free_disk_bytes"`
	GPUDetected bool     `json:"gpu_detected"`
	NetworkIFs  []string `json:"network_interfaces"`
	Tier        Tier     `json:"tier"`
}

// GenerateReport collects hardware information and classifies the system tier.
// The recordingsPath is used to check available disk space on the target volume.
func GenerateReport(recordingsPath string) (*HardwareReport, error) {
	return generateReport(recordingsPath, nil, nil)
}

// generateReport is the internal version that accepts test overrides.
func generateReport(recordingsPath string, getMem memInfoProvider, getDisk diskInfoProvider) (*HardwareReport, error) {
	report := &HardwareReport{
		CPUCores: runtime.NumCPU(),
		CPUArch:  runtime.GOARCH,
		GOOS:     runtime.GOOS,
	}

	// RAM
	if getMem == nil {
		getMem = getSystemMemory
	}
	totalRAM, err := getMem()
	if err != nil {
		return nil, fmt.Errorf("syscheck: get memory: %w", err)
	}
	report.TotalRAM = totalRAM

	// Disk
	if getDisk == nil {
		getDisk = func(p string) (uint64, error) {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(p, &stat); err != nil {
				return 0, err
			}
			return stat.Bavail * uint64(stat.Bsize), nil
		}
	}
	path := recordingsPath
	if path == "" {
		path = "."
	}
	freeDisk, err := getDisk(path)
	if err != nil {
		return nil, fmt.Errorf("syscheck: get disk: %w", err)
	}
	report.FreeDisk = freeDisk

	// Network interfaces
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil || len(addrs) == 0 {
				continue
			}
			report.NetworkIFs = append(report.NetworkIFs, iface.Name)
		}
	}

	// GPU detection is best-effort; we don't import CGO-heavy libraries.
	// For now, always false — a future PR can add nvidia-smi detection.
	report.GPUDetected = false

	report.Tier = ClassifyTier(report)
	return report, nil
}

// ClassifyTier determines the NVR hardware tier based on the report.
func ClassifyTier(r *HardwareReport) Tier {
	if r.CPUCores >= EntCPUCores && r.TotalRAM >= EntRAMBytes && r.FreeDisk >= EntDiskBytes {
		return TierEnterprise
	}
	if r.CPUCores >= MidCPUCores && r.TotalRAM >= MidRAMBytes && r.FreeDisk >= MidDiskBytes {
		return TierMid
	}
	if r.CPUCores >= MiniCPUCores && r.TotalRAM >= MiniRAMBytes && r.FreeDisk >= MiniDiskBytes {
		return TierMini
	}
	return TierInsufficient
}
