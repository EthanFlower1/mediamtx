//go:build !windows

package syscheck

import "syscall"

// defaultDiskFree returns available (non-superuser) bytes on the filesystem
// containing path, using POSIX statfs.
func defaultDiskFree(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
