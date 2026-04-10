//go:build windows

package syscheck

import "golang.org/x/sys/windows"

// defaultDiskFree returns available bytes on the volume containing path,
// using GetDiskFreeSpaceExW. "Available" reflects the caller's quota on
// volumes that enforce per-user quotas, matching POSIX Bavail semantics.
func defaultDiskFree(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, err
	}
	return freeAvail, nil
}
