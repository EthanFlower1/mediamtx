package onvif

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// DiscoveredDevice represents an ONVIF device found during a WS-Discovery scan.
type DiscoveredDevice struct {
	XAddr        string `json:"xaddr"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
}

// ScanStatus represents the current state of a discovery scan.
type ScanStatus string

const (
	// ScanStatusScanning indicates a scan is currently in progress.
	ScanStatusScanning ScanStatus = "scanning"
	// ScanStatusComplete indicates a scan has finished.
	ScanStatusComplete ScanStatus = "complete"
)

// ScanResult holds the state and results of a discovery scan.
type ScanResult struct {
	ScanID  string             `json:"scan_id"`
	Status  ScanStatus         `json:"status"`
	Devices []DiscoveredDevice `json:"devices"`
}

// Discovery manages ONVIF WS-Discovery scans.
type Discovery struct {
	mu     sync.Mutex
	result *ScanResult
}

// NewDiscovery returns a new Discovery instance.
func NewDiscovery() *Discovery {
	return &Discovery{}
}

// StartScan begins an asynchronous ONVIF discovery scan. Returns
// ErrScanInProgress if a scan is already running.
func (d *Discovery) StartScan() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result != nil && d.result.Status == ScanStatusScanning {
		return "", ErrScanInProgress
	}

	scanID := uuid.New().String()
	d.result = &ScanResult{
		ScanID:  scanID,
		Status:  ScanStatusScanning,
		Devices: []DiscoveredDevice{},
	}

	go d.runScan(scanID)

	return scanID, nil
}

// GetStatus returns a copy of the current scan result, or nil if no scan
// has been started.
func (d *Discovery) GetStatus() *ScanResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result == nil {
		return nil
	}

	// Return a copy to avoid data races.
	devices := make([]DiscoveredDevice, len(d.result.Devices))
	copy(devices, d.result.Devices)
	return &ScanResult{
		ScanID:  d.result.ScanID,
		Status:  d.result.Status,
		Devices: devices,
	}
}

// GetResults returns the discovered devices from the most recent scan.
// Returns an empty slice if no scan has been run.
func (d *Discovery) GetResults() []DiscoveredDevice {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result == nil {
		return []DiscoveredDevice{}
	}

	devices := make([]DiscoveredDevice, len(d.result.Devices))
	copy(devices, d.result.Devices)
	return devices
}

// runScan performs the actual WS-Discovery probe for ONVIF devices.
// This is a stub implementation that simulates a scan duration.
// In a production build, this would use WS-Discovery multicast probes
// to find ONVIF-compliant devices on the local network.
func (d *Discovery) runScan(scanID string) {
	// Simulate scan duration.
	time.Sleep(2 * time.Second)

	// TODO: Implement actual WS-Discovery probe using multicast to
	// 239.255.255.250:3702 with ONVIF device type filter.
	// When the kerberos-io/onvif library is available, use it to perform
	// real device discovery.

	d.mu.Lock()
	defer d.mu.Unlock()

	// Only update if this scan is still the current one.
	if d.result != nil && d.result.ScanID == scanID {
		d.result.Status = ScanStatusComplete
	}
}
