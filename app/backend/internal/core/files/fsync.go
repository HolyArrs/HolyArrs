package files

import (
	"os"
	"path/filepath"
	"runtime"
)

// SyncFile flushes a file's contents to disk.
func SyncFile(p string) error {
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// SyncDir flushes a folder's entries (new names, renames) to disk. Windows
// cannot open a folder for this and makes renames durable itself, so it is
// skipped there.
func SyncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// WriteFileSynced writes data to p through a synced temporary file and a
// rename, then syncs the folder.
func WriteFileSynced(p string, data []byte, perm os.FileMode) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := SyncFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return SyncDir(filepath.Dir(p))
}
