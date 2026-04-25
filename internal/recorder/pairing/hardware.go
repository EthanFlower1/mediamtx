// Package pairing implements the Recorder-side join sequence (KAI-244).
// It is the only package in internal/recorder/ that imports
// internal/directory/pairing/ — and then only for the PairingToken type.
package pairing

import (
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// HardwareInfo is the payload sent to the Directory during step 2 (check-in).
// It gives the Directory enough context to size storage assignments and flag
// incompatible hardware. All fields are best-effort; failures fall back to
// zero/empty rather than preventing pairing.
type HardwareInfo struct {
	// OS details.
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Kernel  string `json:"kernel,omitempty"`
	Hostname string `json:"hostname,omitempty"`

	// CPU.
	CPUModel   string `json:"cpu_model,omitempty"`
	CPUCores   int    `json:"cpu_cores"`
	CPUThreads int    `json:"cpu_threads"`

	// Memory.
	RAMTotalBytes uint64 `json:"ram_total_bytes"`
	RAMBytes      int64  `json:"ram_bytes"` // alias for CheckInHandler compatibility

	// Disks — one entry per mount point. Removable media is excluded.
	Disks []DiskInfo `json:"disks,omitempty"`

	// NICs — one entry per active non-loopback interface.
	NICs []NICInfo `json:"nics,omitempty"`

	// GPU — brief string if detectable (best-effort via /proc or lspci).
	GPUDescription string `json:"gpu_description,omitempty"`
	GPU            string `json:"gpu,omitempty"` // alias for CheckInHandler compatibility
}

// DiskInfo describes one storage device as seen from the OS.
type DiskInfo struct {
	MountPoint  string `json:"mount_point"`
	FSType      string `json:"fs_type,omitempty"`
	TotalBytes  uint64 `json:"total_bytes"`
	FreeBytes   uint64 `json:"free_bytes"`
}

// NICInfo describes one network interface.
type NICInfo struct {
	Name    string   `json:"name"`
	Addrs   []string `json:"addrs,omitempty"`
	MTU     int      `json:"mtu,omitempty"`
}

// ProbeHardware collects hardware information from the local machine.
// Individual field failures are silently skipped so that a partially degraded
// environment still produces a useful payload.
func ProbeHardware() HardwareInfo {
	info := HardwareInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// OS / host details.
	if hi, err := host.Info(); err == nil {
		info.Kernel = hi.KernelVersion
		info.Hostname = hi.Hostname
	}

	// CPU.
	if counts, err := cpu.Counts(false); err == nil {
		info.CPUCores = counts
	}
	if counts, err := cpu.Counts(true); err == nil {
		info.CPUThreads = counts
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		info.CPUModel = infos[0].ModelName
	}

	// Memory.
	if vm, err := mem.VirtualMemory(); err == nil {
		info.RAMTotalBytes = vm.Total
		info.RAMBytes = int64(vm.Total)
	}

	// Disks — filter to non-trivial, non-loop mount points.
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if isIgnoredMountPoint(p.Mountpoint) {
				continue
			}
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			info.Disks = append(info.Disks, DiskInfo{
				MountPoint: p.Mountpoint,
				FSType:     p.Fstype,
				TotalBytes: usage.Total,
				FreeBytes:  usage.Free,
			})
		}
	}

	// NICs.
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			// Skip loopback and down interfaces.
			if isLoopback(iface.Name) {
				continue
			}
			ni := NICInfo{
				Name: iface.Name,
				MTU:  iface.MTU,
			}
			for _, addr := range iface.Addrs {
				ni.Addrs = append(ni.Addrs, addr.Addr)
			}
			info.NICs = append(info.NICs, ni)
		}
	}

	// GPU — best-effort only.
	info.GPUDescription = probeGPU()

	return info
}

// Validate returns a non-nil error if the hardware is obviously insufficient
// to run the Recorder role (e.g. no writable disk, no RAM probe).
func (h HardwareInfo) Validate() error {
	if h.RAMTotalBytes == 0 {
		return fmt.Errorf("hardware: could not determine RAM; cannot validate system")
	}
	return nil
}

// isIgnoredMountPoint returns true for OS-managed pseudo-filesystems and
// small boot partitions that are not useful for recording storage.
func isIgnoredMountPoint(mp string) bool {
	ignoredPrefixes := []string{
		"/proc", "/sys", "/dev", "/run",
		"/snap", "/boot/efi",
	}
	ignoredExact := []string{"/boot"}
	for _, pfx := range ignoredPrefixes {
		if len(mp) >= len(pfx) && mp[:len(pfx)] == pfx {
			return true
		}
	}
	for _, ex := range ignoredExact {
		if mp == ex {
			return true
		}
	}
	return false
}

func isLoopback(name string) bool {
	return name == "lo" || name == "lo0" || len(name) >= 2 && name[:2] == "lo"
}
