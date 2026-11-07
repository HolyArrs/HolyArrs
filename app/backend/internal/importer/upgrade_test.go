package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// upgradeWorld is setup with the downloaded file renamed to file, the
// Download's Release named rel, Keep up to date on, and (when old is set) a
// File already in place under that release name.
func upgradeWorld(t *testing.T, file, rel, old string) (world, int64, int64) {
	t.Helper()
	w := setup(t)
	ctx := context.Background()
	if err := os.Rename(filepath.Join(w.dl, relName+".mkv"), filepath.Join(w.dl, file+".mkv")); err != nil {
		t.Fatal(err)
	}
	var relID, oldID int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048)) WHERE id = ?`, file+".mkv", w.dlID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 1"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE versions SET keep_up_to_date = 1, pick_rules_id = (SELECT id FROM pick_rules WHERE name = ?)", domain.GoodEverydayName); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		r := &domain.Release{SourceID: src.ID, Name: rel, LinkRef: "http://x/1.torrent", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{w.slotID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		id, err := store.Insert(tx, "releases", r, domain.System)
		if err != nil {
			return err
		}
		relID = id
		if _, err := tx.Exec("UPDATE downloads SET release_id = ? WHERE id = ?", relID, w.dlID); err != nil {
			return err
		}
		if old == "" {
			return nil
		}
		rp := filepath.ToSlash(filepath.Join("Dune - Part Two (2024)", old+".mkv"))
		write(t, filepath.Join(w.lib, filepath.FromSlash(rp)), 100)
		f := &domain.File{SlotIDs: []int64{w.slotID}, VersionID: w.vID, RelativePath: rp, Size: 100, AddedAt: time.Now().Add(-time.Hour),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &old, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if oldID, err = store.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", oldID, w.slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, relID, oldID
}

func slotOf(t *testing.T, w world) domain.Slot {
	t.Helper()
	var sl domain.Slot
	if err := store.Get(context.Background(), w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

// pick-rules.md 4.3: a file that is lower than its Release name said is
// rejected and the Release turned down, so it is never picked again.
func TestImportRejectsLyingFile(t *testing.T) {
	w, relID, _ := upgradeWorld(t, "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "")
	var again []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
	ctx := context.Background()
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "rejected" {
		t.Fatalf("import %s", imp.State)
	}
	var rel domain.Release
	_ = store.Get(ctx, w.s.Read(), "releases", relID, &rel)
	if rel.State != "turned_down" || rel.Verdict["turned_down_reason"] != "file is 720p, name said 1080p" {
		t.Fatalf("release %s %v", rel.State, rel.Verdict)
	}
	if sl := slotOf(t, w); sl.State != domain.SlotRejected || sl.FileID != nil {
		t.Fatalf("slot %s", sl.State)
	}
	if entries, _ := os.ReadDir(w.lib); len(entries) != 0 {
		t.Fatal("a lying file was placed")
	}
	if len(again) != 1 || again[0] != w.slotID {
		t.Fatalf("OnRejected %v", again)
	}
}

func TestBetterCopyReplacesOldAndStopsAtGoodEnough(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER")
	ctx := context.Background()
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %v %v", imp, err)
	}
	if imp.Plan[0].ReplacesFileID == nil || *imp.Plan[0].ReplacesFileID != oldID {
		t.Fatalf("plan does not name the replaced File: %+v", imp.Plan[0])
	}
	var old domain.File
	_ = store.Get(ctx, w.s.Read(), "files", oldID, &old)
	if old.State != "replaced" {
		t.Fatalf("old file %s", old.State)
	}
	if _, err := os.Stat(filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune.Part.Two.2024.720p.HDTV.x264-OTHER.mkv")); !os.IsNotExist(err) {
		t.Fatal("old copy still in the movie folder")
	}
	if !strings.HasPrefix(old.RelativePath, HoldingDir+"/") {
		t.Fatalf("old row path %q does not follow the file", old.RelativePath)
	}
	held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "Dune - Part Two (2024)", "*720p*"))
	if len(held) != 1 {
		t.Fatalf("old copy not in the holding folder: %v", held)
	}
	sl := slotOf(t, w)
	if sl.State != domain.SlotInLibrary || !sl.GoodEnough || sl.FileID == nil || *sl.FileID == oldID {
		t.Fatalf("slot %s good enough %v", sl.State, sl.GoodEnough)
	}
}

func TestBelowGoodEnoughKeepsLooking(t *testing.T) {
	w, _, _ := upgradeWorld(t, "Dune.Part.Two.2024.720p.HDTV.x264-OTHER", "Dune.Part.Two.2024.720p.HDTV.x264-OTHER", "")
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slotOf(t, w); sl.State != domain.SlotBetterWanted || sl.GoodEnough || sl.FileID == nil {
		t.Fatalf("slot %s good enough %v", sl.State, sl.GoodEnough)
	}
}

func TestImportNeverDowngrades(t *testing.T) {
	w, relID, oldID := upgradeWorld(t, "Dune.Part.Two.2024.720p.HDTV.x264-OTHER", "Dune.Part.Two.2024.720p.HDTV.x264-OTHER",
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	ctx := context.Background()
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "rejected" {
		t.Fatalf("import %v %v", imp, err)
	}
	var rel domain.Release
	_ = store.Get(ctx, w.s.Read(), "releases", relID, &rel)
	if why, _ := rel.Verdict["turned_down_reason"].(string); rel.State != "turned_down" || !strings.Contains(why, "lower than the copy in place") {
		t.Fatalf("release %s %v", rel.State, rel.Verdict)
	}
	var old domain.File
	_ = store.Get(ctx, w.s.Read(), "files", oldID, &old)
	if sl := slotOf(t, w); old.State != "present" || sl.FileID == nil || *sl.FileID != oldID {
		t.Fatalf("old file %s, slot file %v", old.State, sl.FileID)
	}
}

// A fixed copy of the same step has the same file name: the old one steps
// aside first.
func TestProperWithTheSameFileName(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS",
		"Dune - Part Two (2024) 1080p WEB")
	ctx := context.Background()
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	fi, err := os.Stat(filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv"))
	if err != nil || fi.Size() != 2048 {
		t.Fatalf("new copy not in place: %v", err)
	}
	var old domain.File
	_ = store.Get(ctx, w.s.Read(), "files", oldID, &old)
	if old.State != "replaced" {
		t.Fatalf("old %s", old.State)
	}
}

// C1: the old File's row is present but its file is gone, and the better copy
// has the same file name. The new copy must stay in place.
func TestSameNameWhenOldFileIsGone(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS",
		"Dune - Part Two (2024) 1080p WEB")
	target := filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if fi, err := os.Stat(target); err != nil || fi.Size() != 2048 {
		t.Fatalf("new copy was moved away: %v", err)
	}
	if held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "*", "*")); len(held) != 0 {
		t.Fatalf("something was held aside: %v", held)
	}
	var old domain.File
	_ = store.Get(ctx, w.s.Read(), "files", oldID, &old)
	if old.State != "replaced" || strings.HasPrefix(old.RelativePath, HoldingDir) {
		t.Fatalf("old %s %s", old.State, old.RelativePath)
	}
}

// C1: a crash after the old copy stepped aside (or after the new one was
// placed) and before the database write; the run after restart must keep
// the new copy in place and the old row must follow the held copy.
func TestReplaceSurvivesCrash(t *testing.T) {
	for _, stage := range []string{"held", "placed"} {
		t.Run(stage, func(t *testing.T) {
			w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264.PROPER-HOLYARRS",
				"Dune - Part Two (2024) 1080p WEB")
			ctx := context.Background()
			crashAt = func(s string) error {
				if s == stage {
					return os.ErrDeadlineExceeded
				}
				return nil
			}
			_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
			crashAt = nil
			if err == nil {
				t.Fatal("no crash")
			}
			imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
			if err != nil || imp.State != "done" {
				t.Fatalf("restart %+v %v", imp, err)
			}
			target := filepath.Join(w.lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv")
			if fi, err := os.Stat(target); err != nil || fi.Size() != 2048 {
				t.Fatalf("new copy not in place: %v", err)
			}
			held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "Dune - Part Two (2024)", "*"))
			if len(held) != 1 {
				t.Fatalf("held %v", held)
			}
			if fi, _ := os.Stat(held[0]); fi == nil || fi.Size() != 100 {
				t.Fatal("the held copy is not the old one")
			}
			var old domain.File
			_ = store.Get(ctx, w.s.Read(), "files", oldID, &old)
			rel, _ := filepath.Rel(w.lib, held[0])
			if old.State != "replaced" || old.RelativePath != filepath.ToSlash(rel) {
				t.Fatalf("old %s %s, held at %s", old.State, old.RelativePath, rel)
			}
		})
	}
}

// M7, M8: two holds in one second never overwrite; a path leaving the
// Library folder is refused; a rename that fails falls back to copy and remove.
func TestHoldAsideNamesAndSafety(t *testing.T) {
	lib := t.TempDir()
	now := time.Now()
	var got []string
	for i := 0; i < 3; i++ {
		write(t, filepath.Join(lib, "M", "a.mkv"), 10+i)
		to, err := holdAside(lib, "M/a.mkv", now)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, to)
	}
	if got[0] == got[1] || got[1] == got[2] || got[0] == got[2] {
		t.Fatalf("names repeat: %v", got)
	}
	for i, p := range got {
		if fi, _ := os.Stat(p); fi == nil || fi.Size() != int64(10+i) {
			t.Fatalf("copy %d overwritten", i)
		}
	}
	if _, err := holdAside(lib, "../outside.mkv", now); err == nil {
		t.Fatal("a path outside the Library folder was accepted")
	}
	write(t, filepath.Join(lib, "M", "b.mkv"), 5)
	renameFile = func(string, string) error { return &os.LinkError{Op: "rename", Err: os.ErrInvalid} }
	t.Cleanup(func() { renameFile = os.Rename })
	to, err := holdAside(lib, "M/b.mkv", now)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(to); fi == nil || fi.Size() != 5 {
		t.Fatal("copy fallback did not keep the file")
	}
	if _, err := os.Stat(filepath.Join(lib, "M", "b.mkv")); !os.IsNotExist(err) {
		t.Fatal("source left after copy fallback")
	}
}

// I3: a lying file is rejected, but the Slot still has its File in place,
// so it goes back to looking for a better copy, not to rejected.
func TestLyingFileKeepsSlotWithItsFile(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "rejected" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slotOf(t, w); sl.State != domain.SlotBetterWanted || sl.FileID == nil || *sl.FileID != oldID {
		t.Fatalf("slot %s file %v", sl.State, sl.FileID)
	}
}

// Open question: a file named with less detail than its Release (here
// movie-1080p.mkv in a Remux Release's folder) is only compared when its
// name parses with confidence, so a real upgrade is not refused.
func TestPlainFileNameInsideRemuxRelease(t *testing.T) {
	const rel = "Dune.Part.Two.2024.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-GROUP"
	w, _, _ := upgradeWorld(t, "x", rel, "Dune.Part.Two.2024.1080p.BluRay.x264-GROUP")
	if err := os.MkdirAll(filepath.Join(w.dl, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(w.dl, "x.mkv"), filepath.Join(w.dl, rel, "movie-1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048)) WHERE id = ?`, rel+"/movie-1080p.mkv", w.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %s %v %+v", imp.State, err, imp.Plan)
	}
}
