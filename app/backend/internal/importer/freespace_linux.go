//go:build linux

package importer

import (
	"strconv"
	"syscall"
)

// FreeSpace returns the bytes free for normal users at path, or -1 when unknown.
func FreeSpace(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// DiskSize returns the total bytes of the disk holding path, or -1 when unknown.
func DiskSize(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return -1
	}
	return int64(st.Blocks) * int64(st.Bsize)
}

func diskID(path string) string {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return ""
	}
	return strconv.FormatUint(uint64(st.Dev), 10)
}
