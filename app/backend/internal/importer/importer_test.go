package importer

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const relName = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func target(lib string) Target {
	return Target{LibraryPath: lib, FolderName: FolderName("Dune: Part Two", 2024), TitleID: 1, TitleName: "Dune: Part Two", Year: 2024, SlotIDs: []int64{7}}
}

func TestNaming(t *testing.T) {
	if got := FolderName("Dune: Part Two", 2024); got != "Dune - Part Two (2024)" {
		t.Fatalf("folder %q", got)
	}
	if got := FileName("Dune: Part Two", 2024, "1080p_web", ".mkv"); got != "Dune - Part Two (2024) 1080p WEB.mkv" {
		t.Fatalf("file %q", got)
	}
	if got := FolderName(`A/B\C?*<>|"`, 2000); strings.ContainsAny(got, `/\?*<>|"`) {
		t.Fatalf("unsafe %q", got)
	}
}

func TestPlanMatchesAndRefuses(t *testing.T) {
	dl := t.TempDir()
	lib := t.TempDir()
	write(t, filepath.Join(dl, relName+".mkv"), 1000)
	write(t, filepath.Join(dl, "Sample", relName+"-sample.mkv"), 10)
	write(t, filepath.Join(dl, relName+".exe"), 10)
	write(t, filepath.Join(dl, relName+".nfo"), 10)
	cands, err := Scan(dl)
	if err != nil {
		t.Fatal(err)
	}
	steps := Plan(cands, target(lib))
	if len(steps) != 4 {
		t.Fatalf("got %d steps: %+v", len(steps), steps)
	}
	by := map[string]mediafiles.PlanStep{}
	for _, s := range steps {
		by[filepath.Base(s.SourceFile)] = s
	}
	main := by[relName+".mkv"]
	wantPath := filepath.Join(lib, "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv")
	if main.Method != "hardlink" || main.TargetPath != wantPath || main.Confidence < 0.99 || len(main.TargetSlotIDs) != 1 {
		t.Fatalf("main step %+v", main)
	}
	for _, n := range []string{relName + ".exe", relName + "-sample.mkv", relName + ".nfo"} {
		if s := by[n]; s.Method != "refuse" && s.Method != "skip" {
			t.Errorf("%s: method %s", n, s.Method)
		}
	}
	if by[relName+".exe"].Method != "refuse" || !strings.Contains(strings.Join(by[relName+".exe"].Why, " "), "program") {
		t.Errorf("exe not refused as a program: %+v", by[relName+".exe"])
	}
	if by[relName+"-sample.mkv"].Method != "refuse" {
		t.Errorf("sample not refused")
	}
}

func TestExecuteHardlink(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(dl, relName+".mkv")
	write(t, src, 1000)
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	res := Execute(steps, Ops{})
	if len(res) != 1 || res[0].Outcome != "placed" || res[0].PlacedBy != "hardlink" || res[0].FellBack {
		t.Fatalf("result %+v", res)
	}
	a, _ := os.Stat(src)
	b, err := os.Stat(steps[0].TargetPath)
	if err != nil || !os.SameFile(a, b) {
		t.Fatalf("not the same file: %v", err)
	}
}

func TestExecuteCrossDeviceFallsBackToCopy(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(dl, relName+".mkv")
	write(t, src, 4096)
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	ops := Ops{Link: func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }}
	res := Execute(steps, ops)
	if len(res) != 1 || res[0].Outcome != "placed" || res[0].PlacedBy != "copy" || !res[0].FellBack {
		t.Fatalf("result %+v", res)
	}
	a, _ := os.Stat(src)
	b, err := os.Stat(steps[0].TargetPath)
	if err != nil || os.SameFile(a, b) || b.Size() != 4096 {
		t.Fatalf("copy wrong: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source must stay where it is")
	}
}

func TestExecuteNeverTouchesFilesItDidNotPlace(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dl, relName+".mkv"), 1000)
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	write(t, steps[0].TargetPath, 7)
	res := Execute(steps, Ops{})
	if res[0].Outcome != "refused" {
		t.Fatalf("result %+v", res)
	}
	if fi, _ := os.Stat(steps[0].TargetPath); fi.Size() != 7 {
		t.Fatal("existing file was changed")
	}
}

func TestExecuteRefusedStepsDoNothing(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dl, relName+".exe"), 10)
	cands, _ := Scan(dl)
	res := Execute(Plan(cands, target(lib)), Ops{Link: func(string, string) error { t.Fatal("link called"); return nil }})
	if len(res) != 1 || res[0].Outcome != "refused" {
		t.Fatalf("result %+v", res)
	}
	if entries, _ := os.ReadDir(lib); len(entries) != 0 {
		t.Fatal("library changed")
	}
}

type world struct {
	s                 *database.Store
	slotID, dlID, vID int64
	dl, lib           string
}

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) world {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	setFake(t, s, "import_size_floors", "false")
	w := world{s: s, dl: t.TempDir(), lib: t.TempDir()}
	write(t, filepath.Join(w.dl, relName+".mkv"), 2048)
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: w.lib, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: ptr(2024), Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: FolderName(ti.Name, 2024), IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotDownloading}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abc", Progress: 1, State: "finished",
			FilesSeen: []domain.SeenFile{{Name: relName + ".mkv", Size: 2048}}, SavePath: w.dl, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		w.slotID, w.dlID, w.vID = sl.ID, dl.ID, v.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestImportDryRunChangesNothing(t *testing.T) {
	w := setup(t)
	im := &Importer{Store: w.s}
	before := w.s.WriteCount()
	imp, err := im.Import(context.Background(), w.dlID, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Mode != "dry_run" || len(imp.Plan) != 1 || len(imp.Result) != 0 {
		t.Fatalf("import %+v", imp)
	}
	if entries, _ := os.ReadDir(w.lib); len(entries) != 0 {
		t.Fatal("dry run touched the library")
	}
	if w.s.WriteCount() != before {
		t.Fatal("dry run wrote to the database")
	}
}

func TestImportPlacesAndRecords(t *testing.T) {
	w := setup(t)
	im := &Importer{Store: w.s}
	ctx := context.Background()
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" || len(imp.Result) != 1 || imp.Result[0].PlacedBy != "hardlink" {
		t.Fatalf("import %+v", imp)
	}
	files, _ := database.Select[mediafiles.File](ctx, w.s.Read(), "files", "")
	if len(files) != 1 || files[0].Origin != "imported" || files[0].PlacedBy != "hardlink" || files[0].VersionID != w.vID ||
		files[0].RelativePath != filepath.ToSlash(filepath.Join("Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p WEB.mkv")) {
		t.Fatalf("files %+v", files)
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.State != mediafiles.SlotInLibrary || sl.FileID == nil || *sl.FileID != files[0].ID {
		t.Fatalf("slot %+v", sl)
	}
	var d domain.Download
	_ = database.Get(ctx, w.s.Read(), "downloads", w.dlID, &d)
	if d.State != "finished" && d.State != "seeding" {
		t.Fatalf("download %s", d.State)
	}
	es, _, _ := w.s.Events(ctx, database.EventFilter{Kinds: []string{"import_done"}})
	if len(es) != 1 || es[0].TitleID == nil || es[0].SlotID == nil || es[0].Data["from"] == nil {
		t.Fatalf("import events %+v", es)
	}
	if ps, _ := es[0].Data["paths"].([]any); len(ps) != 1 || !strings.Contains(ps[0].(map[string]any)["to"].(string), "Dune - Part Two (2024)") {
		t.Fatalf("import event paths %+v", es[0].Data)
	}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err == nil {
		t.Fatal("second import should fail")
	}
}

func TestIsCrossDevice(t *testing.T) {
	if !errors.Is(&os.LinkError{Err: syscall.EXDEV}, syscall.EXDEV) {
		t.Fatal("sanity")
	}
}

func TestExecuteTakesHardlinkAlreadyAtTarget(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(dl, relName+".mkv")
	write(t, src, 1000)
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	if err := os.MkdirAll(filepath.Dir(steps[0].TargetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(src, steps[0].TargetPath); err != nil {
		t.Skip("hardlinks not supported here:", err)
	}
	res := Execute(steps, Ops{})
	if res[0].Outcome != "placed" {
		t.Fatalf("result %+v", res)
	}
}

func TestExecuteTakesIdenticalCopyAtTarget(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(dl, relName+".mkv")
	body := bytes.Repeat([]byte("holyarrs"), 400000)
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	if err := os.MkdirAll(filepath.Dir(steps[0].TargetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(steps[0].TargetPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := Execute(steps, Ops{})
	if res[0].Outcome != "placed" {
		t.Fatalf("result %+v", res)
	}
	if got, _ := os.ReadFile(steps[0].TargetPath); !bytes.Equal(got, body) {
		t.Fatal("existing file was changed")
	}
}

func TestExecuteRefusesSameSizeDifferentFile(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(dl, relName+".mkv")
	body := bytes.Repeat([]byte("holyarrs"), 400000)
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cands, _ := Scan(dl)
	steps := Plan(cands, target(lib))
	other := bytes.Clone(body)
	other[len(other)/2] ^= 1 // differs only in the middle
	if err := os.MkdirAll(filepath.Dir(steps[0].TargetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(steps[0].TargetPath, other, 0o644); err != nil {
		t.Fatal(err)
	}
	res := Execute(steps, Ops{})
	if res[0].Outcome != "refused" || !strings.Contains(res[0].Error, "did not place") {
		t.Fatalf("result %+v", res)
	}
	if got, _ := os.ReadFile(steps[0].TargetPath); !bytes.Equal(got, other) {
		t.Fatal("existing file was changed")
	}
}
