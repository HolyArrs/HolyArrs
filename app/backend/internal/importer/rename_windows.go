//go:build windows

package importer

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// renameNoReplace renames src to dst and never replaces an existing dst:
// MoveFileEx without the replace flag refuses an existing target.
func renameNoReplace(src, dst string) error {
	from, err := windows.UTF16PtrFromString(longPath(src))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	to, err := windows.UTF16PtrFromString(longPath(dst))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	if err := windows.MoveFileEx(from, to, 0); err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}

// longPath gives a path Windows accepts past 260 characters, as Go's own file
// calls do.
func longPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	const prefix, unc = "\\\\?\\", "\\\\"
	if len(p) < 248 || strings.HasPrefix(p, prefix) {
		return p
	}
	if strings.HasPrefix(p, unc) {
		return prefix + "UNC\\" + p[len(unc):]
	}
	return prefix + p
}
