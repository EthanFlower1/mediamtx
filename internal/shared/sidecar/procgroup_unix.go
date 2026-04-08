//go:build !windows

package sidecar

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so that a
// signal to -pgid fans out to the entire tree (including any helper
// processes a sidecar may spawn).
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalProcessGroup sends sig to the child's process group. Falls
// back to signaling the process directly if the pgid lookup fails.
func signalProcessGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return cmd.Process.Signal(sig)
	}
	return syscall.Kill(-pgid, sig)
}
