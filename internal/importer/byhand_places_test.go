package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestCleanImportFolders(t *testing.T) {
	got, err := CleanImportFolders([]string{" /data/imports/ ", "", `D:\Imports\`, "d:/imports", "/data/imports", "/", `C:\`})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/data/imports", `D:\Imports`, "/", `C:\`}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, bad := range []string{"relative", "C:", "/data/../etc", `D:\a\..\b`, `\\host\share`, "//host/share", "/x\x00"} {
		if _, err := CleanImportFolders([]string{bad}); !errors.Is(err, ErrImportFolder) {
			t.Errorf("%q passed", bad)
		}
	}
	many := make([]string, MaxImportFolders+1)
	for i := range many {
		many[i] = fmt.Sprintf("/data/%d", i)
	}
	if _, err := CleanImportFolders(many); err == nil {
		t.Error("33 folders passed")
	}
	if _, err := CleanImportFolders(many[:MaxImportFolders]); err != nil {
		t.Errorf("32 folders: %v", err)
	}
}

// A path given by hand must be inside a Library folder, a Downloader's
// folder or an extra import folder; a sibling with the same start, or
// anything else, is refused.
func TestByHandPlacesHolds(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	base := t.TempDir()
	save, mapped, extra, outside := filepath.Join(base, "save"), filepath.Join(base, "mapped"), filepath.Join(base, "extra"), filepath.Join(base, "outside")
	for _, d := range []string{save, mapped, extra, outside, save + "2", w.lib + "2"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	maps, _ := json.Marshal([]domain.PathMap{{Theirs: "/remote/downloads", Ours: mapped}})
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET path_map = ?", string(maps))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveImportFolders(ctx, w.s, []string{extra}, domain.System); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s, SavePath: save}
	ps, err := im.ByHandPlaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{w.lib, filepath.Join(w.lib, "a.mkv"), save, filepath.Join(save, "x", "a.mkv"), filepath.Join(mapped, "a.mkv"), filepath.Join(extra, "a.mkv")} {
		if !ps.Holds(p) {
			t.Errorf("%s refused; places %+v", p, ps)
		}
	}
	for _, p := range []string{outside, filepath.Join(outside, "a.mkv"), save + "2", filepath.Join(save+"2", "a.mkv"), w.lib + "2", base} {
		if ps.Holds(p) {
			t.Errorf("%s allowed", p)
		}
	}
	upper := strings.ToUpper(filepath.Join(extra, "a.mkv"))
	if got := ps.Holds(upper); got != (runtime.GOOS == "windows") {
		t.Errorf("other case %s: %v", upper, got)
	}
	msg := ps.Refusal().Error()
	for _, p := range []string{w.lib, save, mapped, extra} {
		if !strings.Contains(msg, p) {
			t.Errorf("refusal %q does not name %s", msg, p)
		}
	}
	// A path map of a removed Downloader is no longer a place.
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET removed = 1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(filepath.Join(mapped, "a.mkv")) {
		t.Error("a removed Downloader's folder is still a place")
	}
}

// A link inside a place that leads out of it does not open the outside.
func TestByHandPlacesSymlink(t *testing.T) {
	place, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(place, "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges here")
		}
		t.Fatal(err)
	}
	write(t, filepath.Join(outside, "a.mkv"), 10)
	ps := ByHandPlaces{Extra: []string{place}}
	if ps.Holds(filepath.Join(place, "link")) || ps.Holds(filepath.Join(place, "link", "a.mkv")) {
		t.Fatal("a link out of the place was allowed")
	}
}

// Scanning, copying and moving a file outside every place are refused,
// and the file outside stays where it is.
func TestByHandRefusesOutside(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'wanted' WHERE id = ?", w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	place, outside := filepath.Join(base, "imports"), filepath.Join(base, "imports2")
	src := filepath.Join(outside, "Dune.Part.Two.2024.2160p.WEB-DL.mkv")
	write(t, src, 4096)
	if err := os.MkdirAll(place, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveImportFolders(ctx, w.s, []string{place}, domain.System); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	var pe *PlaceError
	for _, root := range []string{outside, src, base} {
		if _, err := im.ScanByHand(ctx, root); !errors.As(err, &pe) || !strings.Contains(err.Error(), place) {
			t.Errorf("scan %s: %v", root, err)
		}
	}
	res, err := im.ImportByHand(ctx, []ByHandItem{
		{Path: src, SlotID: w.slotID},
		{Path: src, SlotID: w.slotID, Mode: "move"},
		{Path: place + "/../imports2/" + filepath.Base(src), SlotID: w.slotID, Mode: "move"},
	}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range res {
		if r.Outcome != "refused" || r.Target != "" {
			t.Errorf("item %d: %+v", i, r)
		}
	}
	if !strings.Contains(res[0].Error, "Library folders") {
		t.Errorf("refusal %q", res[0].Error)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("the file outside was moved")
	}
	var sl domain.Slot
	_ = store.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.FileID != nil {
		t.Fatalf("slot filled %+v", sl)
	}
	// Inside the extra folder the same file goes in.
	in := filepath.Join(place, filepath.Base(src))
	if err := os.Rename(src, in); err != nil {
		t.Fatal(err)
	}
	if sc, err := im.ScanByHand(ctx, place); err != nil || len(sc.Files) != 1 {
		t.Fatalf("scan inside %+v %v", sc, err)
	}
	res, err = im.ImportByHand(ctx, []ByHandItem{{Path: in, SlotID: w.slotID, Mode: "move"}}, domain.System)
	if err != nil || res[0].Outcome != "placed" {
		t.Fatalf("import inside %+v %v", res, err)
	}
}
