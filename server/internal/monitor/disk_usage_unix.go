//go:build !windows

package monitor

import (
	"syscall"
)

func getDiskUsage(path string) (total, free, avail uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	total = uint64(stat.Blocks) * uint64(stat.Bsize)
	free = uint64(stat.Bfree) * uint64(stat.Bsize)
	avail = uint64(stat.Bavail) * uint64(stat.Bsize)
	return total, free, avail, nil
}
