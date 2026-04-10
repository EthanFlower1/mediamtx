//go:build !windows

package alerts

import "syscall"

// diskUsage returns (total, free) bytes for the filesystem containing path,
// using POSIX statfs. free is the available-to-unprivileged (Bavail) count,
// matching the semantics used by the rest of the NVR disk checks.
func diskUsage(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free, nil
}
