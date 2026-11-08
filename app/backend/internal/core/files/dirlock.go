package files

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// LockFile is the file in the config folder that HolyArrs holds an
// exclusive lock on while it runs, so tools that change a stopped config
// folder can tell it is in use.
const LockFile = ".holyarrs.lock"

// ErrDirLocked means another process holds the config folder's lock.
var ErrDirLocked = errors.New("the config folder is in use by a running HolyArrs")

// errLockUnsupported means the filesystem cannot lock files (some network
// or FUSE mounts). The folder is used without a lock.
var errLockUnsupported = errors.New("file locks not supported")

// lockFile takes the lock; tests replace it.
var lockFile = sysLockFile

// LockDir takes the config folder's lock without waiting. The lock goes
// with the process: the system releases it when the process ends, however
// it ends. unlock releases it earlier and may be called more than once.
func LockDir(dir string) (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(dir, LockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, errLockUnsupported) {
			log.Printf("store: warning: file locks not supported here (%s); do not run two HolyArrs on this folder", dir)
			return func() {}, nil
		}
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = unlockFile(f)
			f.Close()
		})
	}, nil
}
