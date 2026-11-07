//go:build !windows

package importer

import (
	"errors"
	"io/fs"
	"syscall"
)

// linkUnsupported reports a link error meaning the filesystem cannot or will
// not make a hardlink here.
func linkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, fs.ErrPermission)
}
