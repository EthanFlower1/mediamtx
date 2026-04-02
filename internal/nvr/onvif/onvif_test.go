package onvif

import (
	"net"
	"testing"
	"time"
)

func TestDiscoveryInitialState(t *testing.T) {
	d := NewDiscovery()

	status := d.GetStatus()
	if status != nil {
		t.Fatalf("expected nil status before any scan, got %+v", status)
	}

	results := d.GetResults()
	if len(results) != 0 {
		t.Fatalf("expected empty results before any scan, got %d devices", len(results))
	}
}

func TestDiscoveryConcurrentScanRejected(t *testing.T) {
	d := NewDiscovery()

	scanID, err := d.StartScan()
	if err != nil {
		t.Fatalf("first scan should succeed: %v", err)
	}
	if scanID == "" {
		t.Fatal("expected non-empty scan ID")
	}

	// Immediately try to start another scan while the first is running.
	_, err = d.StartScan()
	if err != ErrScanInProgress {
		t.Fatalf("expected ErrScanInProgress, got %v", err)
	}

	// Verify the scan status is "scanning".
	status := d.GetStatus()
	if status == nil {
		t.Fatal("expected non-nil status during scan")
	}
	if status.Status != ScanStatusScanning {
		t.Fatalf("expected status %q, got %q", ScanStatusScanning, status.Status)
	}
	if status.ScanID != scanID {
		t.Fatalf("expected scan ID %q, got %q", scanID, status.ScanID)
	}

	// Wait for the scan to complete (probe + listen takes ~5s per interface).
	time.Sleep(8 * time.Second)

	status = d.GetStatus()
	if status == nil {
		t.Fatal("expected non-nil status after scan")
	}
	if status.Status != ScanStatusComplete {
		t.Fatalf("expected status %q after scan, got %q", ScanStatusComplete, status.Status)
	}

	// A new scan should now be allowed.
	_, err = d.StartScan()
	if err != nil {
		t.Fatalf("scan after completion should succeed: %v", err)
	}
}

func TestNewDiscoveryWithConfig(t *testing.T) {
	cfg := DiscoveryConfig{
		StaleTimeout:   10 * time.Minute,
		ProbeInterval:  200 * time.Millisecond,
		ProbeCount:     5,
		ListenDuration: 2 * time.Second,
	}
	d := NewDiscoveryWithConfig(cfg)

	if d.config.StaleTimeout != 10*time.Minute {
		t.Fatalf("expected StaleTimeout 10m, got %v", d.config.StaleTimeout)
	}
	if d.config.ProbeCount != 5 {
		t.Fatalf("expected ProbeCount 5, got %d", d.config.ProbeCount)
	}
	if d.config.ListenDuration != 2*time.Second {
		t.Fatalf("expected ListenDuration 2s, got %v", d.config.ListenDuration)
	}
}

func TestDefaultDiscoveryConfig(t *testing.T) {
	cfg := DefaultDiscoveryConfig()

	if cfg.StaleTimeout != 5*time.Minute {
		t.Fatalf("expected StaleTimeout 5m, got %v", cfg.StaleTimeout)
	}
	if cfg.ProbeCount != 3 {
		t.Fatalf("expected ProbeCount 3, got %d", cfg.ProbeCount)
	}
	if cfg.ProbeInterval != 100*time.Millisecond {
		t.Fatalf("expected ProbeInterval 100ms, got %v", cfg.ProbeInterval)
	}
	if cfg.ListenDuration != 4*time.Second {
		t.Fatalf("expected ListenDuration 4s, got %v", cfg.ListenDuration)
	}
}

func TestNormalizeHostKey(t *testing.T) {
	tests := []struct {
		name  string
		xaddr string
		want  string
	}{
		{"ip with port", "http://192.168.1.100:8080/onvif/device_service", "192.168.1.100"},
		{"ip without port", "http://10.0.0.5/onvif/device_service", "10.0.0.5"},
		{"https", "https://192.168.1.50:443/onvif", "192.168.1.50"},
		{"empty string", "", ""},
		{"invalid url", "://bad", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeHostKey(tt.xaddr)
			if got != tt.want {
				t.Errorf("normalizeHostKey(%q) = %q, want %q", tt.xaddr, got, tt.want)
			}
		})
	}
}

func TestDiscoverableInterfaces(t *testing.T) {
	ifaces := discoverableInterfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			t.Errorf("interface %s is loopback, should be excluded", iface.Name)
		}
		if iface.Flags&net.FlagUp == 0 {
			t.Errorf("interface %s is not up, should be excluded", iface.Name)
		}
	}
}

func TestStaleDeviceCleanup(t *testing.T) {
	cfg := DiscoveryConfig{
		StaleTimeout:   1 * time.Second,
		ProbeInterval:  10 * time.Millisecond,
		ProbeCount:     1,
		ListenDuration: 100 * time.Millisecond,
	}
	d := NewDiscoveryWithConfig(cfg)

	// Manually inject a stale device.
	d.mu.Lock()
	d.devices["192.168.1.99"] = &DiscoveredDevice{
		XAddr:    "http://192.168.1.99:80/onvif/device_service",
		Model:    "StaleCamera",
		LastSeen: time.Now().Add(-2 * time.Minute),
	}
	d.mu.Unlock()

	// Run a scan — the stale device should be cleaned up.
	scanID, err := d.StartScan()
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := d.GetStatus()
		if status != nil && status.ScanID == scanID && status.Status == ScanStatusComplete {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	d.mu.Lock()
	_, staleExists := d.devices["192.168.1.99"]
	d.mu.Unlock()

	if staleExists {
		t.Fatal("expected stale device to be cleaned up, but it still exists")
	}
}

func TestStaleDeviceNotCleanedWhenTimeoutZero(t *testing.T) {
	cfg := DiscoveryConfig{
		StaleTimeout:   0,
		ProbeInterval:  10 * time.Millisecond,
		ProbeCount:     1,
		ListenDuration: 100 * time.Millisecond,
	}
	d := NewDiscoveryWithConfig(cfg)

	d.mu.Lock()
	d.devices["192.168.1.99"] = &DiscoveredDevice{
		XAddr:    "http://192.168.1.99:80/onvif/device_service",
		Model:    "OldCamera",
		LastSeen: time.Now().Add(-1 * time.Hour),
	}
	d.mu.Unlock()

	scanID, err := d.StartScan()
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := d.GetStatus()
		if status != nil && status.ScanID == scanID && status.Status == ScanStatusComplete {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	d.mu.Lock()
	_, exists := d.devices["192.168.1.99"]
	d.mu.Unlock()

	if !exists {
		t.Fatal("expected device to persist when StaleTimeout is 0")
	}
}

func TestParseScopes(t *testing.T) {
	dev := DiscoveredDevice{}
	parseScopes("onvif://www.onvif.org/manufacturer/Acme onvif://www.onvif.org/hardware/CamPro onvif://www.onvif.org/firmware/1.2.3", &dev)

	if dev.Manufacturer != "Acme" {
		t.Errorf("expected manufacturer Acme, got %q", dev.Manufacturer)
	}
	if dev.Model != "CamPro" {
		t.Errorf("expected model CamPro, got %q", dev.Model)
	}
	if dev.Firmware != "1.2.3" {
		t.Errorf("expected firmware 1.2.3, got %q", dev.Firmware)
	}
}
