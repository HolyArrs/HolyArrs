//go:build !unix

package accounts

import "os"

// openRegular opens path for reading only when it is a regular file.
func openRegular(path string) (*os.File, error) {
	f, err := os.Open(path)
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
	return f, nil
}
