package importer

import (
	"io/fs"
	"os"
)

// renameChecked renames src to dst unless dst exists. It is the fallback where
// the filesystem cannot rename without replacing (SMB, NFS, some FUSE mounts).
// The check and the rename are not one step, so callers hold the files lock.
func renameChecked(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: fs.ErrExist}
	}
	return os.Rename(src, dst)
}
