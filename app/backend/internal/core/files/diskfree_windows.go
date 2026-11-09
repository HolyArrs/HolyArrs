//go:build windows

package files

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// DiskFree returns the bytes free to this user on dir's disk.
func DiskFree(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, nil, nil); err != nil {
		return 0, err
	}
	return free, nil
}

// OnSameVolume reports whether a and b are on one drive or share.
func OnSameVolume(a, b string) (bool, error) {
	va, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	vb, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(filepath.VolumeName(va), filepath.VolumeName(vb)), nil
}
