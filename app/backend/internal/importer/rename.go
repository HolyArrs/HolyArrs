package importer

import (
	"errors"
	"io/fs"
	"os"
)

// renameChecked renames src to dst unless dst exists. It is the fallback where
// the filesystem cannot rename without replacing (SMB, NFS, some FUSE mounts).
// A hard link is made first, since linking never replaces a file; only where
// links are not possible either does it check and then rename, which are not
// one step, so callers hold the files lock.
func renameChecked(src, dst string) error {
	err := os.Link(src, dst)
	if err == nil {
		if rerr := os.Remove(src); rerr != nil {
			// dst is a second name of the same file; the move is done
			// when src goes, so take dst back and report it.
			_ = os.Remove(dst)
			return rerr
		}
		return nil
	}
	if errors.Is(err, fs.ErrExist) {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fs.ErrExist}
	}
	if _, lerr := os.Lstat(dst); lerr == nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fs.ErrExist}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return lerr
	}
	return os.Rename(src, dst)
}
