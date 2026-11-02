//go:build windows

package refresh

import (
	"errors"
	"syscall"
	"unsafe"
)

// DiskUsage reports used and total bytes of the volume holding path.
func DiskUsage(path string) (used, total uint64, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available, free uint64
	ok, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if ok == 0 {
		return 0, 0, errors.New("cannot inspect disk")
	}
	return total - free, total, nil
}
