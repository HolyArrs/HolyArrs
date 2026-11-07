//go:build windows

package importer

import (
	"errors"
	"io/fs"
	"syscall"
)

// 50 is ERROR_NOT_SUPPORTED and 1 is ERROR_INVALID_FUNCTION.
// linkUnsupported reports a link error meaning the filesystem cannot or will
// not make a hardlink here.
func linkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.Errno(50)) ||
		errors.Is(err, syscall.Errno(1)) || errors.Is(err, fs.ErrPermission)
}
