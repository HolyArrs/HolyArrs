//go:build windows

package importer

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// FreeSpace returns the bytes free for normal users at path, or -1 when unknown.
func FreeSpace(path string) int64 {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var avail, total, free uint64
	r, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if r == 0 {
		return -1
	}
	return int64(avail)
}

// DiskSize returns the total bytes of the disk holding path, or -1 when unknown.
func DiskSize(path string) int64 {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var avail, total, free uint64
	r, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if r == 0 {
		return -1
	}
	return int64(total)
}

func diskID(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return strings.ToLower(filepath.VolumeName(abs))
	}
	return ""
}
