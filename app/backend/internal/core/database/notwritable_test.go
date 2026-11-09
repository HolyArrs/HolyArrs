package database

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

// A write refused for a plain reason (read-only, permission, full disk or
// quota) becomes ErrNotWritable with the cause kept; anything else is not
// classified.
func TestNotWritableKeepsTheCause(t *testing.T) {
	for _, no := range []syscall.Errno{syscall.EROFS, syscall.EACCES, syscall.EPERM, syscall.ENOSPC, syscall.EDQUOT} {
		err := NotWritable("/config", &fs.PathError{Op: "open", Path: "/config/.holyarrs.lock", Err: no})
		if !errors.Is(err, ErrNotWritable) || !errors.Is(err, no) {
			t.Errorf("%v: got %v", no, err)
		}
	}
	for _, err := range []error{nil, errors.New("disk on fire"), &fs.PathError{Op: "open", Err: syscall.EINVAL}} {
		if got := NotWritable("/config", err); got != nil {
			t.Errorf("%v: should not be classified, got %v", err, got)
		}
	}
}
