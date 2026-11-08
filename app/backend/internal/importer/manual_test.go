package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCleanByHandPath(t *testing.T) {
	dir := t.TempDir()
	places := ByHandPlaces{Extra: []string{dir}}
	if got, err := CleanByHandPath("  "+dir+"  ", places); err != nil || got != filepath.Clean(dir) {
		t.Fatalf("plain folder: %q %v", got, err)
	}
	for _, bad := range []string{"", "relative/path", dir + "/../x", filepath.Join(dir, "missing"), dir + "\x00"} {
		if _, err := CleanByHandPath(bad, places); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
}

func TestByHandShow(t *testing.T) {
	w := showSetup(t, []int{2})
	ctx := context.Background()
	loose := t.TempDir()
	for _, n := range []int{1, 2, 3} {
		write(t, filepath.Join(loose, "pack", ep(n)), 2048)
	}
	write(t, filepath.Join(loose, "pack", "notes.txt"), 10)
	write(t, filepath.Join(loose, "pack", "Sample", "sample-"+ep(4)), 10)
	im := &Importer{Store: w.s, SavePath: loose}
	sc, err := im.ScanByHand(ctx, loose)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Files) != 4 || sc.More {
		t.Fatalf("scan %+v", sc)
	}
	byName := map[string]ByHandFile{}
	for _, f := range sc.Files {
		byName[filepath.Base(f.Path)] = f
	}
	g := byName[ep(1)].Guess
	if g.TitleName != "Copper Valley" || g.Season == nil || *g.Season != 1 || g.Episode == nil || *g.Episode != 1 || g.SlotID != w.slots[1] || g.HasFile {
		t.Fatalf("guess ep1 %+v", g)
	}
	if byName[ep(1)].Name != "pack/"+ep(1) || byName[ep(1)].Quality == "" {
		t.Fatalf("file ep1 %+v", byName[ep(1)])
	}
	if !strings.Contains(byName["sample-"+ep(4)].Note, "sample") {
		t.Fatalf("sample %+v", byName["sample-"+ep(4)])
	}

	one, two := 1, 2
	res, err := im.ImportByHand(ctx, []ByHandItem{
		{Path: filepath.Join(loose, "pack", ep(1)), TitleID: g.TitleID, Season: &one, Episode: &one},
		{Path: filepath.Join(loose, "pack", ep(3)), SlotID: w.slots[3], Mode: "move"},
		{Path: filepath.Join(loose, "pack", ep(2)), TitleID: g.TitleID, Season: &one, Episode: &two},
		{Path: filepath.Join(loose, "pack", ep(1)), SlotID: w.slots[1]},
		{Path: loose + "/pack/../pack/" + ep(1), SlotID: w.slots[4]},
		{Path: filepath.Join(loose, "pack", "notes.txt"), SlotID: w.slots[4]},
		{Path: filepath.Join(loose, "pack", ep(1)), SlotID: w.slots[4], Mode: "rename"},
	}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"placed", "placed", "refused", "refused", "refused", "refused", "refused"}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Errorf("item %d: %+v, want %s", i, r, want[i])
		}
	}
	if res[0].PlacedBy != "hardlink" || res[1].PlacedBy != "move" {
		t.Fatalf("placed by %+v %+v", res[0], res[1])
	}
	if !strings.Contains(res[2].Error, "download") || !strings.Contains(res[3].Error, "already has a file") {
		t.Fatalf("refusals %q %q", res[2].Error, res[3].Error)
	}
	if _, err := os.Stat(filepath.Join(loose, "pack", ep(1))); err != nil {
		t.Fatal("a copy removed the original")
	}
	if _, err := os.Stat(filepath.Join(loose, "pack", ep(3))); err == nil {
		t.Fatal("a move left the original")
	}
	for _, n := range []int{1, 3} {
		sl := w.slot(t, n)
		if sl.FileID == nil || (sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted) {
			t.Fatalf("slot %d %+v", n, sl)
		}
		f := w.file(t, *sl.FileID)
		if !strings.HasPrefix(f.RelativePath, w.folder+"/Season 1/") || f.Origin != "imported" {
			t.Fatalf("file %d %+v", n, f)
		}
		if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))); err != nil {
			t.Fatalf("file %d not on disk: %v", n, err)
		}
	}
	es, _, _ := w.s.Events(ctx, database.EventFilter{Kinds: []string{"import_done"}})
	if len(es) != 2 || es[0].SlotID == nil || es[0].Data["by_hand"] != true {
		t.Fatalf("events %+v", es)
	}
	imps, _ := database.Select[mediafiles.Import](ctx, w.s.Read(), "imports", "mode = 'by_hand'")
	if len(imps) != 2 || imps[0].State != "done" {
		t.Fatalf("imports %+v", imps)
	}
	sc, _ = im.ScanByHand(ctx, loose)
	for _, f := range sc.Files {
		if filepath.Base(f.Path) == ep(1) && !f.Guess.HasFile {
			t.Fatalf("rescan %+v", f)
		}
	}
	sc, _ = im.ScanByHand(ctx, w.lib)
	if len(sc.Files) != 2 || !sc.Files[0].InLibrary {
		t.Fatalf("library scan %+v", sc)
	}
}

func TestByHandMovie(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'wanted' WHERE id = ?", w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	loose := t.TempDir()
	src := filepath.Join(loose, "Dune.Part.Two.2024.2160p.WEB-DL.mkv")
	write(t, src, 4096)
	im := &Importer{Store: w.s, SavePath: loose}
	sc, err := im.ScanByHand(ctx, src)
	if err != nil || len(sc.Files) != 1 || sc.Files[0].Guess.SlotID != w.slotID || sc.Files[0].Guess.Season != nil {
		t.Fatalf("scan %+v %v", sc, err)
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID}}, domain.System)
	if err != nil || len(res) != 1 || res[0].Outcome != "placed" {
		t.Fatalf("import %+v %v", res, err)
	}
	if !strings.HasPrefix(res[0].Target, filepath.Join(w.lib, "Dune - Part Two (2024)")+string(filepath.Separator)) {
		t.Fatalf("target %s", res[0].Target)
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.FileID == nil || *sl.FileID != res[0].FileID {
		t.Fatalf("slot %+v", sl)
	}
}

func TestCleanByHandPathHidesOSError(t *testing.T) {
	dir := t.TempDir()
	_, err := CleanByHandPath(filepath.Join(dir, "missing"), ByHandPlaces{Extra: []string{dir}})
	if err == nil || err.Error() != ErrByHandPath.Error() {
		t.Fatalf("got %v", err)
	}
}

func TestCleanByHandPathSameAnswerOutside(t *testing.T) {
	base := t.TempDir()
	place, there := filepath.Join(base, "place"), filepath.Join(base, "there")
	for _, d := range []string{place, there} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	places := ByHandPlaces{Extra: []string{place}}
	_, a := CleanByHandPath(there, places)
	_, b := CleanByHandPath(filepath.Join(base, "missing"), places)
	var pe *PlaceError
	if !errors.As(a, &pe) || b == nil || a.Error() != b.Error() {
		t.Fatalf("there: %v; missing: %v", a, b)
	}
}

func TestCleanByHandPathRefusesUNC(t *testing.T) {
	if filepath.VolumeName(`\\host\share\x`) == "" {
		t.Skip("UNC paths only exist on Windows")
	}
	old := byHandStat
	defer func() { byHandStat = old }()
	byHandStat = func(p string) (os.FileInfo, error) {
		t.Errorf("stat called on %q", p)
		return os.Stat(t.TempDir())
	}
	for _, p := range []string{`\\attacker.example\share\x.mkv`, `\\?\UNC\host\share`, `\\.\pipe\x`, `//host/share/x`} {
		if _, err := CleanByHandPath(p, ByHandPlaces{Extra: []string{`D:\Imports`}}); err == nil {
			t.Errorf("%q passed", p)
		}
	}
}

func TestScanByHandStopsOnCancel(t *testing.T) {
	w := showSetup(t, []int{2})
	loose := t.TempDir()
	for _, n := range []int{1, 2, 3} {
		write(t, filepath.Join(loose, ep(n)), 2048)
	}
	im := &Importer{Store: w.s, SavePath: loose}
	if _, err := im.ScanByHand(goneCtx{context.Background()}, loose); err == nil {
		t.Fatal("a cancelled scan finished")
	}
}

type goneCtx struct{ context.Context }

func (goneCtx) Err() error { return context.Canceled }
