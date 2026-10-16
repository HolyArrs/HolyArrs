//go:build !windows

package library

import (
	"errors"
	"os"
	"syscall"
)

// linkCount is the number of hard links to the file at p.
func linkCount(p string) (uint64, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("no link count")
	}
	return uint64(st.Nlink), nil
}
