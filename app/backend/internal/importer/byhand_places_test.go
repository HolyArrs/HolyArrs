package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCleanImportFolders(t *testing.T) {
	got, err := CleanImportFolders([]string{" /data/imports/ ", "", `D:\Imports\`, "d:/imports", "/data/imports"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/data/imports", `D:\Imports`}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, bad := range []string{"relative", "C:", "/data/../etc", `D:\a\..\b`, `\\host\share`, "//host/share", "/x\x00",
		"/", "///", "/.", "/data/./x", `C:\`, "c:/", `C:\.`} {
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

func TestByHandPlacesJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions only exist on Windows")
	}
	place, outside := t.TempDir(), t.TempDir()
	j := filepath.Join(place, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", j, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	write(t, filepath.Join(outside, "a.mkv"), 10)
	write(t, filepath.Join(outside, "sub", "b.mkv"), 10)
	write(t, filepath.Join(place, "own", "c.mkv"), 10)
	ps := ByHandPlaces{Extra: []string{place}}
	for _, p := range []string{j, filepath.Join(j, "a.mkv"), filepath.Join(j, "sub"), filepath.Join(j, "sub", "b.mkv")} {
		if ps.Holds(p) {
			t.Errorf("%s allowed through a junction", p)
		}
	}
	if !ps.Holds(filepath.Join(place, "own", "c.mkv")) || !ps.Holds(place) {
		t.Error("a plain folder in the place was refused")
	}
}

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
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.FileID != nil {
		t.Fatalf("slot filled %+v", sl)
	}
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

func TestByHandPlacesIgnoreWholeDisk(t *testing.T) {
	for p, want := range map[string]bool{"/": true, `C:\`: true, "C:/": true, "c:": true, "/data": false, `C:\Downloads`: false, "": true,
		"/data/..": true, "/.": true, `C:\x\..`: true, "C:/x/../": true, "/data/../media": false} {
		if got := wholeDisk(p); got != want {
			t.Errorf("wholeDisk(%q) = %v, want %v", p, got, want)
		}
	}
	w := setup(t)
	ctx := context.Background()
	root := filepath.VolumeName(w.lib) + string(filepath.Separator)
	maps, _ := json.Marshal([]domain.PathMap{{Theirs: "/remote", Ours: root}})
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET path_map = ?", string(maps))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s, SavePath: root}
	ps, err := im.ByHandPlaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Downloaders) != 0 {
		t.Fatalf("a disk root counts as a Downloader place: %v", ps.Downloaders)
	}
	if ps.Holds(filepath.Join(t.TempDir(), "x.mkv")) {
		t.Fatal("a path outside every place passes through a disk-root mapping")
	}
}

func TestWholeDiskThroughALink(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	link := filepath.Join(t.TempDir(), "disk")
	if err := os.Symlink(root, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("cannot make a link:", err)
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, root).CombinedOutput(); err != nil {
			t.Skipf("cannot make a junction: %v %s", err, out)
		}
	}
	if !wholeDisk(link) {
		t.Fatalf("a link to %s is not taken as the whole disk", root)
	}
	if wholeDisk(filepath.Dir(link)) {
		t.Fatal("a plain folder is taken as the whole disk")
	}
}

func TestSaveImportFoldersRefusesDataDir(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if _, err := SaveImportFolders(ctx, w.s, []string{filepath.Join(w.s.Dir(), "imports")}, domain.System); !errors.Is(err, ErrImportFolder) {
		t.Fatalf("inside the data folder: %v", err)
	}
	if _, err := SaveImportFolders(ctx, w.s, []string{filepath.Dir(w.s.Dir())}, domain.System); !errors.Is(err, ErrImportFolder) {
		t.Fatalf("around the data folder: %v", err)
	}
	if _, err := SaveImportFolders(ctx, w.s, []string{t.TempDir()}, domain.System); err != nil {
		t.Fatalf("own folder: %v", err)
	}
}
