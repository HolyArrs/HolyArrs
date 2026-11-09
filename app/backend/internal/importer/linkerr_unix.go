//go:build !windows

package importer

import (
	"errors"
	"io/fs"
	"syscall"
)

func linkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, fs.ErrPermission)
}
