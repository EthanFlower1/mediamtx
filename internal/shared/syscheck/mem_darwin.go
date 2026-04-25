package syscheck

import (
	"syscall"
	"unsafe"
)

// getSystemMemory returns the total physical memory on macOS using sysctl.
func getSystemMemory() (uint64, error) {
	mib := [2]int32{6 /* CTL_HW */, 24 /* HW_MEMSIZE */}
	var memSize uint64
	size := unsafe.Sizeof(memSize)
	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		2,
		uintptr(unsafe.Pointer(&memSize)),
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)
	if errno != 0 {
		return 0, errno
	}
	return memSize, nil
}
