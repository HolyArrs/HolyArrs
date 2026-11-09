//go:build unix

package files

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func sysLockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case errors.Is(err, unix.EWOULDBLOCK):
		return ErrDirLocked
	case errors.Is(err, unix.ENOLCK), errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOSYS):
		return fmt.Errorf("%w: %v", errLockUnsupported, err)
	}
	return err
}

func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
