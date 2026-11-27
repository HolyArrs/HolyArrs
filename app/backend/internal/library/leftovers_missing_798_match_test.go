package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// missingOnSlot leaves a movie's copy row marked missing while its Slot still
// points at it, and records the file that took its place as an older version
// did: a found note with movie_has_file. That is what a Library looks like
// when a main file was replaced outside the app before 1.3.5.
func missingOnSlot(t *testing.T, s *database.Store, v mediafiles.Version, old mediafiles.Slot, newRel string) int64 {
	t.Helper()
	ctx := context.Background()
	var added int64
	reason := "movie_has_file"
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", *old.FileID); err != nil {
			return err
		}
		f := &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: newRel, Size: 12, AddedAt: now,
			MediaInfo: map[string]any{}, Languages: []string{}, Origin: "found", PlacedBy: "none", Role: "main",
			State: "present", LeftoverReason: &reason}
		var err error
		added, err = database.Insert(tx, "files", f, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return added
}

// folderOfMovie returns the movie's one Version and the Library folder it lives in.
func folderOfMovie(t *testing.T, s *database.Store, titleID int64) (mediafiles.Version, mediafiles.LibraryFolder) {
	t.Helper()
	v := versionsOf(t, s, titleID)[0]
	var lf mediafiles.LibraryFolder
	if err := database.Get(context.Background(), s.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil {
		t.Fatal(err)
	}
	return v, lf
}

// A Library left stuck by an older version, where the Slot points at a
// missing copy and the file that replaced it is a movie_has_file leftover:
// Match links it without replace, because a missing copy fills nothing.
func TestMatchByHandAdoptsOverAMissingMovieFile(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "")
	ctx := context.Background()
	v, lf := folderOfMovie(t, s, id)
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	old := slotsOf(t, s, id)[0]
	if old.FileID == nil {
		t.Fatal("the first copy was not linked")
	}
	newRel := filepath.ToSlash(filepath.Join(filepath.Dir(rel), "Dune Part Two 2024 2160p.mkv"))
	if err := os.Remove(filepath.Join(lf.Path, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(newRel)), "the new copy")
	added := missingOnSlot(t, s, v, old, newRel)
	ls, err := svc.Leftovers(ctx, id)
	if err != nil || len(ls) != 1 || ls[0].FileID != added || ls[0].Reason != "movie_has_file" {
		t.Fatalf("leftovers %+v %v", ls, err)
	}
	out, err := svc.MatchLeftovers(ctx, id, []LeftoverMatch{{FileID: added}}, false, domain.System)
	if err != nil || !out[0].Linked || out[0].Refused != "" {
		t.Fatalf("match without replace %+v %v", out, err)
	}
	if sl := slotsOf(t, s, id)[0]; sl.FileID == nil || *sl.FileID != added || sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %+v", sl)
	}
	var gone mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", *old.FileID, &gone); err != nil || gone.State != "missing" || len(gone.SlotIDs) != 0 {
		t.Fatalf("missing row %+v %v", gone, err)
	}
	if gone.LeftoverReason == nil || *gone.LeftoverReason != "missing_from_disk" {
		t.Fatalf("missing row reason %v", gone.LeftoverReason)
	}
}

// The whole fix in one pass: a rescan marks the gone copy missing and adopts
// the one new video in the folder on the same run, without a second rescan
// to clear the missing row first.
func TestOneRescanAdoptsReplacedMovieFile(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "")
	ctx := context.Background()
	_, lf := folderOfMovie(t, s, id)
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	old := slotsOf(t, s, id)[0]
	if old.FileID == nil {
		t.Fatal("the first copy was not linked")
	}
	newRel := filepath.ToSlash(filepath.Join(filepath.Dir(rel), "Dune.Part.Two.2024.2160p.WEB-DL.H265-GRP.mkv"))
	if err := os.Remove(filepath.Join(lf.Path, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(newRel)), "the new copy")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	if sl.FileID == nil || *sl.FileID == *old.FileID {
		t.Fatalf("one rescan left the slot on the missing file: %+v", sl)
	}
	var f mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", *sl.FileID, &f); err != nil || f.RelativePath != newRel || f.State != "present" {
		t.Fatalf("linked file %+v %v", f, err)
	}
	var gone mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", *old.FileID, &gone); err != nil || gone.State != "missing" || len(gone.SlotIDs) != 0 {
		t.Fatalf("missing row %+v %v", gone, err)
	}
	if ls, err := svc.Leftovers(ctx, id); err != nil || len(ls) != 0 {
		t.Fatalf("the adopted file is still a leftover: %+v %v", ls, err)
	}
}
