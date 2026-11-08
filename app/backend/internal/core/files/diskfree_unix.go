//go:build unix

package files

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// DiskFree returns the bytes free to this user on dir's disk.
func DiskFree(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// OnSameVolume reports whether a and b are on one device.
func OnSameVolume(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return fa.Sys().(*syscall.Stat_t).Dev == fb.Sys().(*syscall.Stat_t).Dev, nil
}
