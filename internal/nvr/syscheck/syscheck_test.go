package syscheck

import (
	"fmt"
	"testing"
)

func TestRunAllPass(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{0}, // ephemeral port, always available
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 8 * 1024 * 1024 * 1024, nil },
		getDiskInfo:    func(path string) (uint64, error) { return 100 * 1024 * 1024 * 1024, nil },
	}

	report := c.Run()
	if report.Overall != StatusPass {
		t.Errorf("expected overall pass, got %s", report.Overall)
		for _, ch := range report.Checks {
			t.Logf("  %s: %s - %s", ch.Name, ch.Status, ch.Message)
		}
	}
}

func TestDiskSpaceWarn(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{},
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 8 * 1024 * 1024 * 1024, nil },
		getDiskInfo:    func(path string) (uint64, error) { return 1 * 1024 * 1024 * 1024, nil }, // 1 GB
	}

	report := c.Run()
	found := false
	for _, ch := range report.Checks {
		if ch.Name == "disk_space" {
			found = true
			if ch.Status != StatusWarn {
				t.Errorf("expected disk warn, got %s", ch.Status)
			}
		}
	}
	if !found {
		t.Error("disk_space check not found")
	}
}

func TestRAMWarn(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{},
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 512 * 1024 * 1024, nil }, // 512 MB
		getDiskInfo:    func(path string) (uint64, error) { return 100 * 1024 * 1024 * 1024, nil },
	}

	report := c.Run()
	for _, ch := range report.Checks {
		if ch.Name == "ram" && ch.Status != StatusWarn {
			t.Errorf("expected ram warn, got %s", ch.Status)
		}
	}
}

func TestPortInUse(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{9999},
		portAvailable:  func(port int) bool { return false },
		getMemInfo:     func() (uint64, error) { return 8 * 1024 * 1024 * 1024, nil },
		getDiskInfo:    func(path string) (uint64, error) { return 100 * 1024 * 1024 * 1024, nil },
	}

	report := c.Run()
	found := false
	for _, ch := range report.Checks {
		if ch.Name == "port_9999" {
			found = true
			if ch.Status != StatusWarn {
				t.Errorf("expected port warn, got %s", ch.Status)
			}
		}
	}
	if !found {
		t.Error("port_9999 check not found")
	}
}

func TestDiskError(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{},
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 8 * 1024 * 1024 * 1024, nil },
		getDiskInfo:    func(path string) (uint64, error) { return 0, fmt.Errorf("disk error") },
	}

	report := c.Run()
	for _, ch := range report.Checks {
		if ch.Name == "disk_space" && ch.Status != StatusWarn {
			t.Errorf("expected disk warn on error, got %s", ch.Status)
		}
	}
}

func TestOverallWarnOnAnyWarn(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{},
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 512 * 1024 * 1024, nil }, // low RAM
		getDiskInfo:    func(path string) (uint64, error) { return 100 * 1024 * 1024 * 1024, nil },
	}

	report := c.Run()
	if report.Overall != StatusWarn {
		t.Errorf("expected overall warn, got %s", report.Overall)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{10 * 1024 * 1024 * 1024, "10.0 GB"},
		{512 * 1024 * 1024, "512 MB"},
		{1536 * 1024 * 1024, "1.5 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestNetworkCheck(t *testing.T) {
	c := &Checker{
		RecordingsPath: ".",
		Ports:          []int{},
		portAvailable:  func(port int) bool { return true },
		getMemInfo:     func() (uint64, error) { return 8 * 1024 * 1024 * 1024, nil },
		getDiskInfo:    func(path string) (uint64, error) { return 100 * 1024 * 1024 * 1024, nil },
	}

	report := c.Run()
	found := false
	for _, ch := range report.Checks {
		if ch.Name == "network" {
			found = true
			// On a dev machine this should pass.
			if ch.Status != StatusPass && ch.Status != StatusWarn {
				t.Errorf("unexpected network status: %s", ch.Status)
			}
		}
	}
	if !found {
		t.Error("network check not found")
	}
}
