// Package syscheck validates system requirements for MediaMTX NVR.
//
// It checks available disk space, RAM, CPU cores, required port availability,
// and network interfaces. Results are returned as structured data suitable
// for both startup logging and the /system/requirements-check API endpoint.
package syscheck

import (
	"fmt"
	"net"
	"runtime"
	"syscall"
)

// Minimum requirements.
const (
	MinDiskBytes   uint64 = 10 * 1024 * 1024 * 1024 // 10 GB
	MinRAMBytes    uint64 = 2 * 1024 * 1024 * 1024   // 2 GB
	MinCPUCores    int    = 2
	MinNetworkIFs  int    = 1
)

// DefaultPorts lists ports the NVR server needs.
var DefaultPorts = []int{8554, 8888, 8889, 9997, 9998}

// CheckStatus indicates pass, warn, or fail.
type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusWarn CheckStatus = "warn"
	StatusFail CheckStatus = "fail"
)

// CheckResult holds the outcome of a single requirement check.
type CheckResult struct {
	Name     string      `json:"name"`
	Status   CheckStatus `json:"status"`
	Required string      `json:"required"`
	Actual   string      `json:"actual"`
	Message  string      `json:"message"`
}

// Report holds the complete set of requirement check results.
type Report struct {
	Overall CheckStatus   `json:"overall"`
	Checks  []CheckResult `json:"checks"`
}

// portChecker abstracts port availability testing for testability.
type portChecker func(port int) bool

// memInfoProvider abstracts memory info retrieval for testability.
type memInfoProvider func() (totalBytes uint64, err error)

// diskInfoProvider abstracts disk info retrieval for testability.
type diskInfoProvider func(path string) (freeBytes uint64, err error)

// Checker validates system requirements.
type Checker struct {
	RecordingsPath string
	Ports          []int

	// Test overrides (nil = use real implementations).
	portAvailable portChecker
	getMemInfo    memInfoProvider
	getDiskInfo   diskInfoProvider
}

// New creates a Checker with the given recordings path and default ports.
func New(recordingsPath string) *Checker {
	return &Checker{
		RecordingsPath: recordingsPath,
		Ports:          DefaultPorts,
	}
}

// Run executes all system requirement checks and returns a report.
func (c *Checker) Run() *Report {
	r := &Report{}

	r.Checks = append(r.Checks, c.checkDisk())
	r.Checks = append(r.Checks, c.checkRAM())
	r.Checks = append(r.Checks, c.checkCPU())
	r.Checks = append(r.Checks, c.checkPorts()...)
	r.Checks = append(r.Checks, c.checkNetwork())

	// Compute overall status: fail > warn > pass.
	r.Overall = StatusPass
	for _, ch := range r.Checks {
		if ch.Status == StatusFail {
			r.Overall = StatusFail
			break
		}
		if ch.Status == StatusWarn {
			r.Overall = StatusWarn
		}
	}

	return r
}

func (c *Checker) checkDisk() CheckResult {
	path := c.RecordingsPath
	if path == "" {
		path = "."
	}

	getDisk := c.getDiskInfo
	if getDisk == nil {
		getDisk = func(p string) (uint64, error) {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(p, &stat); err != nil {
				return 0, err
			}
			return stat.Bavail * uint64(stat.Bsize), nil
		}
	}

	freeBytes, err := getDisk(path)
	if err != nil {
		return CheckResult{
			Name:     "disk_space",
			Status:   StatusWarn,
			Required: formatBytes(MinDiskBytes),
			Actual:   "unknown",
			Message:  fmt.Sprintf("Could not determine disk space: %v", err),
		}
	}

	status := StatusPass
	msg := "Sufficient disk space available"
	if freeBytes < MinDiskBytes {
		status = StatusWarn
		msg = fmt.Sprintf("Low disk space: %s free, recommend at least %s", formatBytes(freeBytes), formatBytes(MinDiskBytes))
	}

	return CheckResult{
		Name:     "disk_space",
		Status:   status,
		Required: formatBytes(MinDiskBytes),
		Actual:   formatBytes(freeBytes),
		Message:  msg,
	}
}

func (c *Checker) checkRAM() CheckResult {
	getMem := c.getMemInfo
	if getMem == nil {
		getMem = getSystemMemory
	}

	totalBytes, err := getMem()
	if err != nil {
		return CheckResult{
			Name:     "ram",
			Status:   StatusWarn,
			Required: formatBytes(MinRAMBytes),
			Actual:   "unknown",
			Message:  fmt.Sprintf("Could not determine system RAM: %v", err),
		}
	}

	status := StatusPass
	msg := "Sufficient RAM available"
	if totalBytes < MinRAMBytes {
		status = StatusWarn
		msg = fmt.Sprintf("Low RAM: %s total, recommend at least %s", formatBytes(totalBytes), formatBytes(MinRAMBytes))
	}

	return CheckResult{
		Name:     "ram",
		Status:   status,
		Required: formatBytes(MinRAMBytes),
		Actual:   formatBytes(totalBytes),
		Message:  msg,
	}
}

func (c *Checker) checkCPU() CheckResult {
	cores := runtime.NumCPU()

	status := StatusPass
	msg := fmt.Sprintf("%d CPU cores available", cores)
	if cores < MinCPUCores {
		status = StatusWarn
		msg = fmt.Sprintf("Only %d CPU core(s), recommend at least %d", cores, MinCPUCores)
	}

	return CheckResult{
		Name:     "cpu_cores",
		Status:   status,
		Required: fmt.Sprintf("%d", MinCPUCores),
		Actual:   fmt.Sprintf("%d", cores),
		Message:  msg,
	}
}

func (c *Checker) checkPorts() []CheckResult {
	var results []CheckResult

	isAvailable := c.portAvailable
	if isAvailable == nil {
		isAvailable = func(port int) bool {
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return false
			}
			ln.Close()
			return true
		}
	}

	for _, port := range c.Ports {
		status := StatusPass
		msg := fmt.Sprintf("Port %d is available", port)
		if !isAvailable(port) {
			status = StatusWarn
			msg = fmt.Sprintf("Port %d is already in use or unavailable", port)
		}

		results = append(results, CheckResult{
			Name:     fmt.Sprintf("port_%d", port),
			Status:   status,
			Required: "available",
			Actual:   boolToAvailability(!isAvailable(port)),
			Message:  msg,
		})
	}

	return results
}

func (c *Checker) checkNetwork() CheckResult {
	ifaces, err := net.Interfaces()
	if err != nil {
		return CheckResult{
			Name:     "network",
			Status:   StatusWarn,
			Required: fmt.Sprintf(">= %d interface(s)", MinNetworkIFs),
			Actual:   "unknown",
			Message:  fmt.Sprintf("Could not enumerate network interfaces: %v", err),
		}
	}

	// Count interfaces that are up and have at least one unicast address.
	var active []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		active = append(active, iface.Name)
	}

	status := StatusPass
	msg := fmt.Sprintf("%d active network interface(s): %v", len(active), active)
	if len(active) < MinNetworkIFs {
		status = StatusWarn
		msg = fmt.Sprintf("No active non-loopback network interfaces found; cameras require network access")
	}

	return CheckResult{
		Name:     "network",
		Status:   status,
		Required: fmt.Sprintf(">= %d interface(s)", MinNetworkIFs),
		Actual:   fmt.Sprintf("%d", len(active)),
		Message:  msg,
	}
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b uint64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if b >= gb {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	}
	return fmt.Sprintf("%.0f MB", float64(b)/float64(mb))
}

func boolToAvailability(inUse bool) string {
	if inUse {
		return "in_use"
	}
	return "available"
}
