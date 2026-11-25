package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A library scan notes a file in a show's folder as found with the episode
// its name gives. While that episode is still empty, Import files takes the
// file and its import replaces the note; once the episode has a file, the
// note still counts as a file in the Library.
func TestByHandTakesFoundFileOfEmptyEpisode(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	rel3 := w.folder + "/Season 1/Copper Valley (US) - S01E03.mkv"
	rel4 := w.folder + "/Season 1/Copper Valley (US) - S01E04.mkv"
	rel4b := w.folder + "/Season 1/Copper.Valley.S01E04.720p.mkv"
	for _, rel := range []string{rel3, rel4, rel4b} {
		write(t, filepath.Join(w.lib, filepath.FromSlash(rel)), 2048)
	}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		note := func(rel string, slot int64, origin string) (int64, error) {
			f := &mediafiles.File{SlotIDs: []int64{slot}, VersionID: w.vID, RelativePath: rel, Size: 2048, Languages: []string{}, MediaInfo: map[string]any{},
				Origin: origin, PlacedBy: "none", Role: "main", State: "present"}
			return database.Insert(tx, "files", f, domain.System)
		}
		if _, err := note(rel3, w.slots[3], "found"); err != nil {
			return err
		}
		id, err := note(rel4b, w.slots[4], "imported")
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", id, w.slots[4]); err != nil {
			return err
		}
		_, err = note(rel4, w.slots[4], "found")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	sc, err := im.ScanByHand(ctx, w.lib)
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]bool{}
	for _, f := range sc.Files {
		in[filepath.Base(f.Path)] = f.InLibrary
	}
	if in[filepath.Base(rel3)] || !in[filepath.Base(rel4)] {
		t.Fatalf("in library %v", in)
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{
		{Path: filepath.Join(w.lib, filepath.FromSlash(rel3)), SlotID: w.slots[3], Mode: "move"},
		{Path: filepath.Join(w.lib, filepath.FromSlash(rel4)), SlotID: w.slots[4], Mode: "move"},
	}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != "placed" {
		t.Fatalf("found file of an empty episode: %+v", res[0])
	}
	if res[1].Outcome != "refused" || !strings.Contains(res[1].Error, "already a file in the Library") {
		t.Fatalf("found file of a filled episode: %+v", res[1])
	}
	sl := w.slot(t, 3)
	if sl.FileID == nil || w.file(t, *sl.FileID).Origin != "imported" {
		t.Fatalf("slot 3 %+v", sl)
	}
	var notes int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files f, json_each(f.slot_ids) j WHERE f.origin = 'found' AND j.value = ?", w.slots[3]).Scan(&notes)
	if notes != 0 {
		t.Fatalf("the found note stayed next to the import: %d", notes)
	}
}

// A found note the person protected stays protected once Import files takes it.
func TestByHandKeepsFoundNoteProtection(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	rel3 := w.folder + "/Season 1/Copper Valley (US) - S01E03.mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(rel3)), 2048)
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{w.slots[3]}, VersionID: w.vID, RelativePath: rel3, Size: 2048, Languages: []string{}, MediaInfo: map[string]any{},
			Origin: "found", PlacedBy: "none", Role: "main", State: "present", Protected: true}
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: filepath.Join(w.lib, filepath.FromSlash(rel3)), SlotID: w.slots[3], Mode: "move"}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("import %+v %v", res, err)
	}
	sl := w.slot(t, 3)
	if sl.FileID == nil || !w.file(t, *sl.FileID).Protected {
		t.Fatalf("the imported file lost the note's protection: %+v", sl)
	}
}

func TestByHandOverStaleMissingRow(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	src := filepath.Join(w.dl, ep(3))
	write(t, src, 2048)
	im := &Importer{Store: w.s, SavePath: w.dl}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slots[3], Mode: "copy"}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("first import %+v %v", res, err)
	}
	sl := w.slot(t, 3)
	if sl.FileID == nil {
		t.Fatal("E03 has no file after first import")
	}
	staleID := *sl.FileID
	origRel := w.file(t, staleID).RelativePath
	if err := os.Remove(res[0].Target); err != nil {
		t.Fatal(err)
	}
	otherRel := w.folder + "/Season 1/Copper Valley - S01E03 - 720p.mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(otherRel)), 2048)
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		other := &mediafiles.File{SlotIDs: []int64{w.slots[3]}, VersionID: w.vID, RelativePath: otherRel, Size: 2048,
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
		otherID, err := database.Insert(tx, "files", other, domain.System)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", staleID); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", otherID, w.slots[3])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err = im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slots[3], Mode: "copy", Replace: true}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("import over stale missing row: %+v %v", res, err)
	}
	sl = w.slot(t, 3)
	if sl.FileID == nil {
		t.Fatal("E03 has no file after second import")
	}
	if got := w.file(t, *sl.FileID).RelativePath; got != origRel {
		t.Fatalf("relative_path = %q, want %q", got, origRel)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", w.vID, origRel).Scan(&n)
	if n != 1 {
		t.Fatalf("%d rows at %q", n, origRel)
	}
}

// A protected file row left at the target path with the file gone from disk
// (state missing, no longer the episode's file) must not block an import to
// that path, and the new file keeps the protection. The episode's own
// protected file then refuses a replacement.
func TestByHandOverProtectedMissingRow(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	src := filepath.Join(w.dl, ep(3))
	write(t, src, 2048)
	im := &Importer{Store: w.s, SavePath: w.dl}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slots[3], Mode: "copy"}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("first import %+v %v", res, err)
	}
	sl := w.slot(t, 3)
	if sl.FileID == nil {
		t.Fatal("E03 has no file after the first import")
	}
	staleID := *sl.FileID
	rel := w.file(t, staleID).RelativePath
	if err := os.Remove(res[0].Target); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET state = 'missing', protected = 1 WHERE id = ?", staleID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = NULL, state = 'wanted' WHERE id = ?", w.slots[3])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err = im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slots[3], Mode: "copy"}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("import over a protected missing row: %+v %v", res, err)
	}
	sl = w.slot(t, 3)
	if sl.FileID == nil {
		t.Fatal("E03 has no file after the second import")
	}
	got := w.file(t, *sl.FileID)
	if got.RelativePath != rel || got.State != "present" || !got.Protected {
		t.Fatalf("new file at %q state %s protected %v, want %q present and protected", got.RelativePath, got.State, got.Protected, rel)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", w.vID, rel).Scan(&n)
	if n != 1 {
		t.Fatalf("%d rows at %q", n, rel)
	}

	// The episode's own file is protected now: a replacement is refused.
	res, err = im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slots[3], Mode: "copy", Replace: true}}, domain.System)
	if err != nil || res[0].Outcome != "refused" {
		t.Fatalf("replacing a protected file: %+v %v", res, err)
	}
}
