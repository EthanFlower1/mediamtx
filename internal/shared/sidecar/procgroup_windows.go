//go:build windows

package sidecar

import (
	"os/exec"
	"syscall"
)

// setProcessGroup is a no-op on Windows; the supervisor falls back to
// signaling the main process only. Windows sidecars are not a
// supported deployment target for the Kaivue Recording Server but we
// keep the build green.
func setProcessGroup(cmd *exec.Cmd) {}

func signalProcessGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(sig)
}
