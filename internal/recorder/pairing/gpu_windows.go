//go:build windows

package pairing

import (
	"os/exec"
	"strings"
)

// probeGPU returns a short description of the installed GPU using wmic.
func probeGPU() string {
	out, err := exec.Command("wmic", "path", "win32_VideoController", "get", "Name").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "Name" {
			continue
		}
		return line
	}
	return ""
}
