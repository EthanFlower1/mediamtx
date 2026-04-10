//go:build windows

package syscheck

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// memoryStatusEx mirrors the Win32 MEMORYSTATUSEX struct.
// https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/ns-sysinfoapi-memorystatusex
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
)

// getSystemMemory returns the total physical memory on Windows using
// GlobalMemoryStatusEx from kernel32.
func getSystemMemory() (uint64, error) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r1, _, e1 := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r1 == 0 {
		if e1 != nil {
			return 0, fmt.Errorf("GlobalMemoryStatusEx: %w", e1)
		}
		return 0, fmt.Errorf("GlobalMemoryStatusEx failed")
	}
	return m.TotalPhys, nil
}
