//go:build linux || darwin || freebsd

package pairing

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// probeGPU returns a short description of the installed GPU(s) using lspci on
// Linux/BSD or system_profiler on macOS. Returns "" on failure.
func probeGPU() string {
	// macOS
	if _, err := exec.LookPath("system_profiler"); err == nil {
		out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Chipset Model:") {
					return strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
				}
			}
		}
	}

	// Linux — try /sys first (no root needed).
	const drmPath = "/sys/class/drm"
	if entries, err := os.ReadDir(drmPath); err == nil {
		for _, e := range entries {
			labelPath := drmPath + "/" + e.Name() + "/device/label"
			if b, err := os.ReadFile(labelPath); err == nil {
				label := strings.TrimSpace(string(b))
				if label != "" {
					return label
				}
			}
		}
	}

	// Fallback: lspci.
	if _, err := exec.LookPath("lspci"); err == nil {
		out, err := exec.Command("lspci").Output()
		if err == nil {
			for _, line := range bytes.Split(out, []byte("\n")) {
				s := string(line)
				if strings.Contains(s, "VGA") || strings.Contains(s, "3D") || strings.Contains(s, "Display") {
					// Remove the PCI address prefix.
					if idx := strings.Index(s, ": "); idx >= 0 {
						return strings.TrimSpace(s[idx+2:])
					}
					return strings.TrimSpace(s)
				}
			}
		}
	}

	return ""
}
