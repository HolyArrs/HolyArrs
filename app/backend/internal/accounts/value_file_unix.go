//go:build unix

package accounts

import (
	"os"
	"syscall"
)

// openRegular opens path for reading only when it is a regular file. The
// open does not wait (a pipe with no writer would hold it forever), and the
// check is on the opened file, so nothing can swap the path in between.
func openRegular(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, errNotRegular
	}
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
