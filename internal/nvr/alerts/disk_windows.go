//go:build windows

package alerts

import "golang.org/x/sys/windows"

// diskUsage returns (total, free) bytes on the volume containing path,
// using GetDiskFreeSpaceExW.
func diskUsage(path string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeAvail, nil
}
