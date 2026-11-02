package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func lockDirectory(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("invalid storage ownership path")
	}
	handle, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, ErrOwned
	}
	return os.NewFile(uintptr(handle), path), nil
}

func localSpace(path string) (uint64, error) {
	if strings.HasPrefix(path, `\\`) {
		return 0, errors.New("network storage is unsupported")
	}
	root, err := syscall.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return 0, errors.New("invalid storage path")
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	driveType, _, _ := kernel.NewProc("GetDriveTypeW").Call(uintptr(unsafe.Pointer(root)))
	if driveType != 3 && driveType != 6 {
		return 0, errors.New("storage requires a local fixed drive")
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("invalid storage path")
	}
	var available, total, free uint64
	ok, _, _ := kernel.NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if ok == 0 {
		return 0, errors.New("cannot inspect free storage")
	}
	return available, nil
}
