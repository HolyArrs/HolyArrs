//go:build linux

package health

import (
	"errors"
	"os"
	"syscall"
)

// fOFDSetLK is F_OFD_SETLK: a lock owned by the open file, so two opens in
// one process conflict like two processes would (the same on every arch).
const fOFDSetLK = 37

// lockProbe opens a temporary file twice, locks it through the first open
// and checks that the second open is refused. Where locks are ignored (some
// host folder shares) both succeed, and SQLite could damage the database.
var lockProbe = func(dir string) error {
	f, err := os.CreateTemp(dir, ".holyarrs-lock-*")
	if err != nil {
		return nil // the write check reports this
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	g, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	defer g.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	if err := syscall.FcntlFlock(f.Fd(), fOFDSetLK, &lk); err != nil {
		if errors.Is(err, syscall.EINVAL) {
			return nil // an old kernel without these locks: nothing to tell
		}
		return errors.New("a lock could not be taken: " + err.Error())
	}
	lk2 := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	if err := syscall.FcntlFlock(g.Fd(), fOFDSetLK, &lk2); err == nil {
		return errors.New("a second lock on the same file was not refused")
	}
	return nil
}
