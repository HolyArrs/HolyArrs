package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func rowCount(t *testing.T, s *database.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// folderRows counts the folder row and the Version and File rows of title
// on it.
func folderRows(t *testing.T, s *database.Store, lf mediafiles.LibraryFolder, title int64) (folders, versions, files int) {
	t.Helper()
	return rowCount(t, s, "SELECT count(*) FROM library_folders WHERE id = ?", lf.ID),
		rowCount(t, s, "SELECT count(*) FROM versions WHERE title_id = ? AND library_folder_id = ?", title, lf.ID),
		rowCount(t, s, "SELECT count(*) FROM files WHERE version_id IN (SELECT id FROM versions WHERE title_id = ?)", title)
}

// Every title deleted from the Library, files left on disk: the folder can
// go, the deleted title stays in History, and nothing on disk is touched.
func TestDeleteFolderAfterTitlesRemoved(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); err != nil {
		t.Fatalf("delete folder after every title was removed: %v", err)
	}
	if f, v, n := folderRows(t, s, lf, id); f != 0 || v != 0 || n != 0 {
		t.Fatalf("left rows: folder %d versions %d files %d", f, v, n)
	}
	if n := rowCount(t, s, "SELECT count(*) FROM slots WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", id); n != 0 {
		t.Fatalf("%d slots left", n)
	}
	var ti mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", id, &ti); err != nil || ti.State != "removed" {
		t.Fatalf("the removed title should stay in History: %+v %v", ti, err)
	}
	for _, f := range fs {
		if !onDisk(lf, f.RelativePath) {
			t.Fatalf("%s left the disk", f.RelativePath)
		}
	}
}

// The reported case: titles deleted with their files, then the folder's
// mount went away. Nothing there can be restored, so the folder can go.
func TestDeleteFolderMissingOnDisk(t *testing.T) {
	svc, s, lf, id, _ := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(lf.Path); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); err != nil {
		t.Fatalf("delete a missing folder whose titles were removed: %v", err)
	}
	if f, v, n := folderRows(t, s, lf, id); f != 0 || v != 0 || n != 0 {
		t.Fatalf("left rows: folder %d versions %d files %d", f, v, n)
	}
}

// Deleted files still waiting in the folder's holding folder can be
// restored, so the folder stays until they are restored or emptied.
func TestDeleteFolderKeepsRestorableHeldFiles(t *testing.T) {
	svc, s, lf, id, _ := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	held, err := svc.HeldFiles(ctx)
	if err != nil || len(held) == 0 {
		t.Fatalf("held %v %v", held, err)
	}
	f0, v0, n0 := folderRows(t, s, lf, id)
	if err := DeleteFolder(ctx, s, lf.ID); !errors.Is(err, ErrFolderHeld) {
		t.Fatalf("err = %v, want ErrFolderHeld", err)
	}
	if f, v, n := folderRows(t, s, lf, id); f != f0 || v != v0 || n != n0 {
		t.Fatalf("rows changed on a refused delete: %d/%d/%d, were %d/%d/%d", f, v, n, f0, v0, n0)
	}
	if again, _ := svc.HeldFiles(ctx); len(again) != len(held) {
		t.Fatalf("held files after a refused delete: %d, were %d", len(again), len(held))
	}
	if err := os.RemoveAll(filepath.Join(lf.Path, importer.HoldingDir)); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); err != nil {
		t.Fatalf("delete after holding was emptied by hand: %v", err)
	}
}

// An old copy an upgrade kept in holding cannot be restored from Recently
// deleted, so it does not keep the folder; it stays on disk like every file.
func TestDeleteFolderIgnoresReplacedCopies(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	rel := importer.HoldingDir + "/2026-10-01/Dune - Part Two (2024)/old copy.mkv"
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ? WHERE id = ?", rel, fs[1].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "x")
	if err := DeleteFolder(ctx, s, lf.ID); err != nil {
		t.Fatalf("an upgrade's old copy kept the folder: %v", err)
	}
	if !onDisk(lf, rel) {
		t.Fatal("the old copy left the disk")
	}
}

// A deleted title with Versions on two folders keeps a Main Version when
// the folder of its Main goes, so a later restore brings back a whole title.
func TestDeleteFolderKeepsAMainVersion(t *testing.T) {
	svc, s, lf, id, _ := deleteWorld(t)
	ctx := context.Background()
	other, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies 4K"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "4K", LibraryFolderID: other.ID}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); err != nil {
		t.Fatalf("delete the Main Version's folder: %v", err)
	}
	vs := versionsOf(t, s, id)
	if len(vs) != 1 || vs[0].LibraryFolderID != other.ID || !vs[0].IsMain {
		t.Fatalf("versions left %+v, want the 4K one on the other folder as Main", vs)
	}
}

// A title still in the Library keeps the folder, and the refusal leaves the
// removed title's rows as they were.
func TestDeleteFolderRefusesLiveTitleBesideRemoved(t *testing.T) {
	svc, s, lf, id, _ := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	f0, v0, n0 := folderRows(t, s, lf, id)
	if err := DeleteFolder(ctx, s, lf.ID); !errors.Is(err, ErrFolderInUse) {
		t.Fatalf("err = %v, want ErrFolderInUse", err)
	}
	if f, v, n := folderRows(t, s, lf, id); f != f0 || v != v0 || n != n0 {
		t.Fatalf("rows changed on a refused delete: %d/%d/%d, were %d/%d/%d", f, v, n, f0, v0, n0)
	}
}

// A folder an outside list adds to is refused with a clear answer, not a
// database error.
func TestDeleteFolderUsedByOutsideList(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO outside_lists(created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id)
			VALUES ('x', '{}', 'x', '{}', 'L', 'tmdb_list', '1', ?)`, lf.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); !errors.Is(err, ErrFolderListed) {
		t.Fatalf("err = %v, want ErrFolderListed", err)
	}
}
