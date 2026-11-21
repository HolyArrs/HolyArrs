package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Errors of a plain folder check.
var (
	ErrFolderReadOnly = errors.New("HolyArrs cannot write to the folder")
	ErrFolderOverlap  = errors.New("that folder, or one inside or around it, is a Library folder or a Downloader folder")
)

// CheckPlainFolder checks a folder HolyArrs keeps its own files in, outside
// the Library: an absolute path that is not a drive root and not around the
// data folder, that exists, takes writes, and neither holds nor sits inside
// a Library folder or a Downloader folder. Its probe file is removed again.
func CheckPlainFolder(ctx context.Context, s *database.Store, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return ErrFolderPath
	}
	path = filepath.Clean(path)
	if UnsafeFolder(path, s.Dir()) {
		return ErrFolderPath
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return ErrFolderAbsent
	}
	probe, err := os.CreateTemp(path, ".holyarrs-check-*")
	if err != nil {
		return ErrFolderReadOnly
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	libs, err := mediafiles.LibraryFolderPaths(ctx, s.Read())
	if err != nil {
		return err
	}
	dls, err := DownloadPaths(ctx, s.Read())
	if err != nil {
		return err
	}
	target := resolved(path)
	for _, other := range append(libs, dls...) {
		if other != "" && (overlaps(other, path) || overlaps(resolved(other), target)) {
			return ErrFolderOverlap
		}
	}
	return nil
}

// resolved is p with symlinks followed, or p cleaned when that fails.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
