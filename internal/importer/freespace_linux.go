//go:build linux

package importer

import (
	"strconv"
	"syscall"
)

// FreeSpace returns the bytes free for normal users at path, or -1.
func FreeSpace(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// diskID names the device holding path, or "" when unknown.
func diskID(path string) string {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return ""
	}
	return strconv.FormatUint(uint64(st.Dev), 10)
}
