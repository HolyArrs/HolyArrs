package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ErrFolderFilesChanged says files in the folder changed while the new path
// was being checked, so the check no longer holds.
var ErrFolderFilesChanged = errors.New("files in this Library folder changed while the new path was checked; try again")

// PathFileError names the first present file that is not at a folder's new
// path, or is there with another size.
type PathFileError struct {
	RelativePath string
	Missing      bool
	Want, Got    int64 // bytes the Library knows (the last rescan's, else the import's), bytes at the new path
}

func (e *PathFileError) Error() string {
	if e.Missing {
		return fmt.Sprintf("%s is not at the new path", e.RelativePath)
	}
	return fmt.Sprintf("%s is %d bytes at the new path, but %d bytes in the Library", e.RelativePath, e.Got, e.Want)
}

// ChangeFolderPath points a Library folder at a new path whose files were
// already moved there (a new mount point, a renamed share). The path is
// checked like a new folder's; then every present file must be at the new
// path with a size the Library knows for it, and the first that is not
// stops the change and is named. Only then is the path saved and the folder
// checked again. Nothing on disk is moved or written, beyond the check's
// own probe files.
func ChangeFolderPath(ctx context.Context, s *database.Store, id int64, newPath string, downloadPaths []string, by domain.Actor) (mediafiles.LibraryFolder, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, s.Read(), "library_folders", id, &lf); err != nil {
		return lf, err
	}
	if newPath == "" || !filepath.IsAbs(newPath) {
		return lf, ErrFolderPath
	}
	newPath = filepath.Clean(newPath)
	if UnsafeFolder(newPath, s.Dir()) {
		return lf, ErrFolderPath
	}
	if st, err := os.Stat(newPath); err != nil || !st.IsDir() {
		return lf, ErrFolderAbsent
	}
	if err := pathFree(ctx, s.Read(), id, newPath); err != nil {
		return lf, err
	}
	// Imports and deletes hold this while they place or hold files, so no
	// file lands under the old path between the check and the save.
	unlock := s.LockFiles()
	defer unlock()
	files, err := mediafiles.PresentFolderFiles(ctx, s.Read(), id)
	if err != nil {
		return lf, err
	}
	// The disk is read outside the write, so a slow share never holds it.
	if err := filesAt(ctx, newPath, files); err != nil {
		return lf, err
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "library_folders", id, &lf); err != nil {
			return err
		}
		if err := pathFree(ctx, tx, id, newPath); err != nil {
			return err
		}
		now, err := mediafiles.PresentFolderFiles(ctx, tx, id)
		if err != nil {
			return err
		}
		if !slices.Equal(files, now) {
			return ErrFolderFilesChanged
		}
		lf.Path = newPath
		return database.Update(tx, "library_folders", &lf, by)
	})
	if err != nil {
		return lf, err
	}
	lf, _, err = RecheckFolder(ctx, s, id, downloadPaths, by, false)
	return lf, err
}

// pathFree refuses a path inside or around any other Library folder.
func pathFree(ctx context.Context, db database.Lister, id int64, p string) error {
	all, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "")
	if err != nil {
		return err
	}
	for _, o := range all {
		if o.ID != id && overlaps(o.Path, p) {
			return ErrFolderTaken
		}
	}
	return nil
}

// filesAt checks each file under root in order and stops at the first that
// is missing or has a size the Library does not know for it. A request
// that goes away stops it.
func filesAt(ctx context.Context, root string, files []mediafiles.FolderFile) error {
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(f.RelativePath)))
		if err != nil || !st.Mode().IsRegular() {
			return &PathFileError{RelativePath: f.RelativePath, Missing: true, Want: f.Size}
		}
		if !sizeFits(st.Size(), f) {
			want := f.Size
			if f.Seen > 0 {
				want = f.Seen
			}
			return &PathFileError{RelativePath: f.RelativePath, Want: want, Got: st.Size()}
		}
	}
	return nil
}

// sizeFits says whether n bytes is a size the Library knows for f: the one
// recorded at import or the one the last rescan read. A file with neither
// only needs to be there.
func sizeFits(n int64, f mediafiles.FolderFile) bool {
	if f.Size <= 0 && f.Seen <= 0 {
		return true
	}
	return (f.Size > 0 && n == f.Size) || (f.Seen > 0 && n == f.Seen)
}
