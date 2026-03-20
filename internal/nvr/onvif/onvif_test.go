package onvif

import (
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

	// Wait for the scan to complete (probe + listen takes ~5s).
	time.Sleep(6 * time.Second)

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
