package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// a download named by the movie's other title (a German release of an
// English-named movie) matches like the main name does.
func TestPlanMatchesOtherNames(t *testing.T) {
	lib := t.TempDir()
	tg := target(lib)
	tg.Aliases = []string{"Der Wüstenplanet: Teil Zwei"}
	cands := []Candidate{{Path: filepath.Join(t.TempDir(), "Der.Wuestenplanet.Teil.Zwei.2024.German.DL.1080p.BluRay.x264-GROUP.mkv"), Size: 1000}}
	if s := Plan(cands, target(lib))[0]; s.Method != "refuse" {
		t.Fatalf("without the other name it should not match: %+v", s)
	}
	cands[0].Path = filepath.Join(filepath.Dir(cands[0].Path), "Der.Wustenplanet.Teil.Zwei.2024.German.DL.1080p.BluRay.x264-GROUP.mkv")
	s := Plan(cands, tg)[0]
	if s.Method != "hardlink" || s.Confidence < 0.99 || !strings.HasPrefix(s.TargetPath, filepath.Join(lib, "Dune - Part Two (2024)")) {
		t.Fatalf("other name: %+v", s)
	}
	// An odd short file name in a well named folder takes the folder's name.
	dir := filepath.Join(t.TempDir(), "Der.Wustenplanet.Teil.Zwei.2024.German.DL.1080p.BluRay.x265-GROUP")
	s = Plan([]Candidate{{Path: filepath.Join(dir, "group-shortname.1080p.mkv"), Size: 1000}}, tg)[0]
	if s.Method != "hardlink" {
		t.Fatalf("folder name: %+v", s)
	}
	s = Plan([]Candidate{{Path: filepath.Join(dir, "grp-wuepl2.2023.1080p.mkv"), Size: 1000}}, tg)[0]
	if s.Method != "hardlink" {
		t.Fatalf("a file name with a year that names nothing should fall back to the folder: %+v", s)
	}
}

func TestOtherNamesHoldsTheOriginalTitle(t *testing.T) {
	orig := "Le Fabuleux Destin"
	got := OtherNames(mediafiles.Title{Name: "Amelie", Aliases: []string{"Amélie"}, OriginalName: &orig})
	if len(got) != 2 || got[0] != "Amélie" || got[1] != orig {
		t.Fatalf("%v", got)
	}
	same := "Amelie"
	if got := OtherNames(mediafiles.Title{Name: "Amelie", Aliases: []string{}, OriginalName: &same}); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func setAliases(t *testing.T, w world, aliases string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET aliases = ?", aliases)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func setFilesSeen(t *testing.T, w world, names ...string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var d domain.Download
		if err := database.Get(context.Background(), w.s.Read(), "downloads", w.dlID, &d); err != nil {
			return err
		}
		d.FilesSeen = nil
		for _, n := range names {
			write(t, filepath.Join(w.dl, filepath.FromSlash(n)), 2048)
			d.FilesSeen = append(d.FilesSeen, domain.SeenFile{Name: n, Size: 2048})
		}
		return database.Update(tx, "downloads", &d, domain.System)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestImportUsesStoredOtherNames(t *testing.T) {
	w := setup(t)
	setAliases(t, w, `["Der Wüstenplanet Teil Zwei"]`)
	setFilesSeen(t, w, "Der.Wustenplanet.Teil.Zwei.2024.German.DL.1080p.BluRay.x264-GROUP.mkv")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" {
		t.Fatalf("import %+v", imp)
	}
}

func slot308(t *testing.T, w world) mediafiles.Slot {
	t.Helper()
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

// by hand, a file may replace the one a movie has; the old file goes to
// the holding folder as a deleted file.
func TestByHandReplacesCurrentFile(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("first import %+v %v", imp, err)
	}
	old := slot308(t, w)
	var oldFile mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", *old.FileID, &oldFile)
	oldAbs := filepath.Join(w.lib, filepath.FromSlash(oldFile.RelativePath))

	loose := t.TempDir()
	src := filepath.Join(loose, "Dune.Part.Two.2024.2160p.WEB-DL.mkv")
	write(t, src, 4096)
	im.SavePath = loose
	res, _ := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID}}, domain.System)
	if res[0].Outcome != "refused" || !strings.Contains(res[0].Error, "already has a file") {
		t.Fatalf("without replace: %+v", res[0])
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Replace: true}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("replace: %+v %v", res, err)
	}
	sl := slot308(t, w)
	if sl.FileID == nil || *sl.FileID == *old.FileID || *sl.FileID != res[0].FileID {
		t.Fatalf("slot %+v", sl)
	}
	var gone mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", oldFile.ID, &gone)
	if gone.State != "gone" || !strings.HasPrefix(gone.RelativePath, HoldingDir+"/") {
		t.Fatalf("old file row %+v", gone)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(gone.RelativePath))); err != nil {
		t.Fatalf("old file not held: %v", err)
	}
	if _, err := os.Stat(oldAbs); err == nil && filepath.Clean(oldAbs) != filepath.Clean(res[0].Target) {
		t.Fatal("old file left in place")
	}
	es, _, _ := w.s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventFileHeld}})
	if len(es) != 1 || es[0].Data["to"] != gone.RelativePath || es[0].Data["from"] != oldFile.RelativePath {
		t.Fatalf("held events %+v", es)
	}
}

// A replace whose new file cannot be placed puts the old file back.
func TestByHandReplacePutsBackOnFailure(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	old := slot308(t, w)
	var oldFile mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", *old.FileID, &oldFile)
	loose := t.TempDir()
	src := filepath.Join(loose, "Dune.Part.Two.2024.2160p.WEB-DL.mkv")
	write(t, src, 4096)
	im.SavePath = loose
	im.Ops = Ops{Link: func(string, string) error { return os.ErrPermission }, Copy: func(string, string) error { return os.ErrPermission }}
	res, _ := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Replace: true}}, domain.System)
	if res[0].Outcome == "placed" {
		t.Fatalf("placed %+v", res[0])
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(oldFile.RelativePath))); err != nil {
		t.Fatalf("old file not put back: %v", err)
	}
	if sl := slot308(t, w); sl.FileID == nil || *sl.FileID != oldFile.ID {
		t.Fatalf("slot %+v", sl)
	}
}

// Review imports a download's one video into the movie it was grabbed
// for, even when its name matches nothing.
func TestImportReviewFile(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	setFilesSeen(t, w, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.mkv", "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/Sample/group-sn-sample.mkv",
		"Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.nfo")
	im := &Importer{Store: w.s}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("auto import should not place it: %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision {
		t.Fatalf("slot %s", sl.State)
	}
	if f := im.ReviewFile(ctx, w.dlID); filepath.Base(f) != "group-sn.mkv" {
		t.Fatalf("review file %q", f)
	}
	cleared := int64(0)
	im.OnImported = func(_ context.Context, id int64) { cleared = id }
	if r, _ := im.ImportReviewFile(ctx, w.slotID+99, w.dlID, false, domain.System); r.Outcome == "placed" {
		t.Fatalf("another slot: %+v", r)
	}
	r, err := im.ImportReviewFile(ctx, w.slotID, w.dlID, false, domain.System)
	if err != nil || r.Outcome != "placed" {
		t.Fatalf("review import %+v %v", r, err)
	}
	if sl := slot308(t, w); sl.FileID == nil || (sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted) {
		t.Fatalf("slot %+v", sl)
	}
	if cleared != w.dlID {
		t.Fatalf("download not cleared: %d", cleared)
	}
	imps, _ := database.Select[mediafiles.Import](ctx, w.s.Read(), "imports", "mode = 'by_hand'")
	if len(imps) != 1 || imps[0].DownloadID == nil || *imps[0].DownloadID != w.dlID {
		t.Fatalf("imports %+v", imps)
	}
	// Two videos: no single clear file.
	setFilesSeen(t, w, "a/one.mkv", "a/two.mkv")
	if f := im.ReviewFile(ctx, w.dlID); f != "" {
		t.Fatalf("two videos gave %q", f)
	}
}
