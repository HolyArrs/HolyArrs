//go:build windows

package importer

import (
	"errors"
	"io/fs"
	"syscall"
)

func linkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.Errno(50)) ||
		errors.Is(err, syscall.Errno(1)) || errors.Is(err, fs.ErrPermission)
}
