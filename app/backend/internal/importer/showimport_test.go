package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const packName = "Copper.Valley.S01.1080p.WEB.H264-HOLYARRS"

type showWorld struct {
	s      *database.Store
	dl     string
	lib    string
	vID    int64
	dlID   int64
	relID  int64
	slots  map[int]int64
	folder string
}

func ep(n int) string {
	return "Copper.Valley.S01E0" + string(rune('0'+n)) + ".1080p.WEB.H264-HOLYARRS.mkv"
}

func showSetup(t *testing.T, dlEps []int, files ...string) showWorld {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	setFake(t, s, "import_size_floors", "false")
	w := showWorld{s: s, dl: t.TempDir(), lib: t.TempDir(), slots: map[int]int64{}, folder: "Copper Valley (2025)"}
	var seen []domain.SeenFile
	for _, f := range files {
		write(t, filepath.Join(w.dl, packName, filepath.FromSlash(f)), 2048)
		seen = append(seen, domain.SeenFile{Name: packName + "/" + f, Size: 2048})
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: w.lib, Kind: "show", Label: "Shows", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		order := "aired"
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Copper Valley", SortName: "copper valley", Year: ptr(2025), Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", EpisodeOrder: &order,
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: w.folder, IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		w.vID = v.ID
		names := []string{"", "The Mill", "Flood", "Ash", "Spring"}
		for n := 1; n <= 4; n++ {
			season, num, name := 1, n, names[n]
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &num}, Name: &name, KeepUpToDate: true,
				SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			state := mediafiles.SlotWanted
			if slices.Contains(dlEps, n) {
				state = mediafiles.SlotDownloading
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			w.slots[n] = sl.ID
		}
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		var ids []int64
		for _, n := range dlEps {
			ids = append(ids, w.slots[n])
		}
		r := &domain.Release{SourceID: src.ID, Name: packName, LinkRef: "http://x/1.torrent", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: ids, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		w.relID = r.ID
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &r.ID, SlotIDs: ids, ExternalID: "pack", Progress: 1, State: "finished",
			FilesSeen: seen, SavePath: w.dl, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		w.dlID = dl.ID
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w showWorld) slot(t *testing.T, n int) mediafiles.Slot {
	t.Helper()
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", w.slots[n], &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

func (w showWorld) file(t *testing.T, id int64) mediafiles.File {
	t.Helper()
	var f mediafiles.File
	if err := database.Get(context.Background(), w.s.Read(), "files", id, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func (w showWorld) release(t *testing.T) domain.Release {
	t.Helper()
	var r domain.Release
	if err := database.Get(context.Background(), w.s.Read(), "releases", w.relID, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSeasonPackFillsEveryEpisode(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3), "Copper.Valley.S01.nfo")
	im := &Importer{Store: w.s}
	ctx := context.Background()
	dry, err := im.Import(ctx, w.dlID, true, domain.System)
	if err != nil || dry.Mode != "dry_run" || len(dry.Plan) != 4 {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" || imp.Kind != "episodes" {
		t.Fatalf("import %s %s: %+v", imp.State, imp.Kind, imp.Result)
	}
	names := []string{"", "The Mill", "Flood", "Ash"}
	for n := 1; n <= 3; n++ {
		sl := w.slot(t, n)
		if sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted || sl.FileID == nil || sl.ActiveDownloadID != nil {
			t.Fatalf("E%d slot %+v", n, sl)
		}
		f := w.file(t, *sl.FileID)
		want := "Copper Valley (2025)/Season 1/Copper Valley - S01E0" + string(rune('0'+n)) + " - " + names[n] + " 1080p WEB.mkv"
		if f.RelativePath != want || len(f.SlotIDs) != 1 || f.SlotIDs[0] != w.slots[n] || f.PlacedBy != "hardlink" {
			t.Fatalf("E%d file %+v, want %s", n, f, want)
		}
		a, _ := os.Stat(filepath.Join(w.dl, packName, ep(n)))
		b, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(want)))
		if err != nil || !os.SameFile(a, b) {
			t.Fatalf("E%d not hardlinked: %v", n, err)
		}
	}
	if sl := w.slot(t, 4); sl.State != mediafiles.SlotWanted {
		t.Fatalf("E04 was not in the download but is %s", sl.State)
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatalf("a good pack was turned down: %+v", r.Verdict)
	}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err == nil {
		t.Fatal("a second import of a settled pack ran")
	}
}

func TestPartialPackSettlesEachEpisode(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(4), "Extras/behind the scenes.mkv", "readme.txt")
	var again []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "partly_done" {
		t.Fatalf("state %s", imp.State)
	}
	for _, n := range []int{1, 2} {
		if sl := w.slot(t, n); sl.FileID == nil {
			t.Fatalf("E%d not placed: %+v", n, sl)
		}
	}
	e3 := w.slot(t, 3)
	if e3.State != mediafiles.SlotWanted || e3.ActiveDownloadID != nil || !strings.Contains(*e3.LastReason, "did not hold S01E03") {
		t.Fatalf("E03 %+v %v", e3, *e3.LastReason)
	}
	if e4 := w.slot(t, 4); e4.State != mediafiles.SlotWanted || e4.FileID != nil {
		t.Fatalf("E04 was placed though not asked: %+v", e4)
	}
	if len(again) != 1 || again[0] != w.slots[3] {
		t.Fatalf("searched again %v", again)
	}
	r := w.release(t)
	ids, _ := r.Verdict["turned_down_slot_ids"].([]any)
	if r.State != "turned_down" || len(ids) != 1 || int64(ids[0].(float64)) != w.slots[3] {
		t.Fatalf("release %s %+v", r.State, r.Verdict)
	}
	why := map[string]string{}
	for _, st := range imp.Plan {
		why[filepath.Base(st.SourceFile)] = st.Method + ": " + strings.Join(st.Why, "; ")
	}
	if !strings.HasPrefix(why[ep(4)], "skip") || !strings.Contains(why[ep(4)], "S01E04 is not filled by this download") {
		t.Errorf("E04 file: %s", why[ep(4)])
	}
	if !strings.HasPrefix(why["behind the scenes.mkv"], "skip") || !strings.HasPrefix(why["readme.txt"], "skip") {
		t.Errorf("extras: %v", why)
	}
	if _, err := os.Stat(filepath.Join(w.dl, packName, ep(4))); err != nil {
		t.Error("a file left alone was touched")
	}
}

func TestMultiEpisodeFile(t *testing.T) {
	w := showSetup(t, []int{1, 2}, "Copper.Valley.S01E01E02.1080p.WEB.H264-HOLYARRS.mkv")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	a, b := w.slot(t, 1), w.slot(t, 2)
	if a.FileID == nil || b.FileID == nil || *a.FileID != *b.FileID {
		t.Fatalf("slots %+v %+v", a, b)
	}
	f := w.file(t, *a.FileID)
	if len(f.SlotIDs) != 2 || f.RelativePath != "Copper Valley (2025)/Season 1/Copper Valley - S01E01-E02 - The Mill + Flood 1080p WEB.mkv" {
		t.Fatalf("file %+v", f)
	}
}

func TestShowImportNothingMatched(t *testing.T) {
	w := showSetup(t, []int{1}, "video.mkv")
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "failed" {
		t.Fatalf("%v %+v", err, imp)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil {
		t.Fatalf("slot %+v", sl)
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatal("turned down with nothing matched")
	}
}

func TestPackFileLowerThanItsName(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), "Copper.Valley.S01E02.720p.WEB.H264-HOLYARRS.mkv")
	var again []int64
	im := &Importer{Store: w.s, OnRejected: func(_ context.Context, id int64) { again = append(again, id) }}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "partly_done" {
		t.Fatalf("%v %+v", err, imp)
	}
	if sl := w.slot(t, 2); sl.State != mediafiles.SlotRejected {
		t.Fatalf("E02 %+v", sl)
	}
	if len(again) != 1 || again[0] != w.slots[2] {
		t.Fatalf("again %v", again)
	}
}

func TestEpisodeBetterCopyReplacesOld(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	ctx := context.Background()
	old := "Copper.Valley.S01E01.720p.HDTV.x264-OLD"
	rel := "Copper Valley (2025)/Season 1/" + old + ".mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(rel)), 100)
	var oldID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{w.slots[1]}, VersionID: w.vID, RelativePath: rel, Size: 100, AddedAt: time.Now().Add(-time.Hour),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &old, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", f, domain.System)
		oldID = id
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", id, w.slots[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	of := w.file(t, oldID)
	if of.State != "replaced" || !strings.HasPrefix(of.RelativePath, HoldingDir+"/") {
		t.Fatalf("old file %+v", of)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(of.RelativePath))); err != nil {
		t.Fatalf("old copy not in the holding folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old copy still in the season folder")
	}
	if sl := w.slot(t, 1); sl.FileID == nil || *sl.FileID == oldID {
		t.Fatalf("slot %+v", sl)
	}
}

func TestShowImportResumesAfterCrash(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	ctx := context.Background()
	crashAt = func(stage string) error {
		if stage == "placed" {
			return errors.New("killed")
		}
		return nil
	}
	_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotImporting {
		t.Fatalf("after crash %s", sl.State)
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("resume %v %+v", err, imp)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files").Scan(&n)
	var imports int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM imports").Scan(&imports)
	if n != 3 || imports != 1 {
		t.Fatalf("files %d imports %d", n, imports)
	}
}

func TestShowImportFreeSpace(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	fakeDisk(t, 5*mb)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "failed" {
		t.Fatalf("%v %+v", err, imp)
	}
	sl := w.slot(t, 1)
	if sl.State != mediafiles.SlotNeedsADecision || !strings.Contains(*sl.LastReason, "free space") {
		t.Fatalf("slot %+v %s", sl, *sl.LastReason)
	}
	if entries, _ := os.ReadDir(w.lib); len(entries) != 0 {
		t.Fatal("placed with no space")
	}
}

func TestStrandedPackImported(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	var queued []int64
	n, err := im.RequeueStranded(ctx, func(_ context.Context, id int64) { queued = append(queued, id) })
	if err != nil || n != 1 || queued[0] != w.dlID {
		t.Fatalf("requeue %d %v %v", n, err, queued)
	}
	if _, err := im.Import(ctx, queued[0], false, domain.System); err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 3; k++ {
		if sl := w.slot(t, k); sl.FileID == nil {
			t.Fatalf("E%d not imported", k)
		}
	}
	queued = nil
	if n, _ := im.RequeueStranded(ctx, func(_ context.Context, id int64) { queued = append(queued, id) }); n != 0 {
		t.Fatalf("queued again after import: %v", queued)
	}
}

func TestPlanEpisodesDailyAndAbsolute(t *testing.T) {
	lib, dl := t.TempDir(), t.TempDir()
	day := filepath.Join(dl, "Night.Talk.2026.03.14.Guests.1080p.WEB.H264-HOLYARRS.mkv")
	write(t, day, 10)
	cands, _ := Scan(dl)
	tg := ShowTarget{LibraryPath: lib, FolderName: "Night Talk", Show: mediafiles.TitleRef{ID: 1, Name: "Night Talk"}, Order: "daily",
		Episodes: []Episode{{Ref: mediafiles.EpisodeRef{ItemID: 9, Season: 2026, Episode: 31, AirDay: "2026-03-14"}, Name: "Guests", SlotID: 5, Wanted: true}}}
	steps := PlanEpisodes(cands, tg)
	if len(steps) != 1 || steps[0].Method != "hardlink" || filepath.ToSlash(steps[0].TargetPath) != filepath.ToSlash(filepath.Join(lib, "Night Talk", "Season 2026", "Night Talk - 2026-03-14 - Guests 1080p WEB.mkv")) {
		t.Fatalf("daily %+v", steps)
	}
	dl2 := t.TempDir()
	write(t, filepath.Join(dl2, "[SubsPlease] Blade Song - 112 (1080p) [ABCD1234].mkv"), 10)
	cands, _ = Scan(dl2)
	tg = ShowTarget{LibraryPath: lib, FolderName: "Blade Song", Show: mediafiles.TitleRef{ID: 2, Name: "Blade Song"}, Order: "absolute",
		Episodes: []Episode{{Ref: mediafiles.EpisodeRef{ItemID: 3, Season: 5, Episode: 12, Absolute: 112}, SlotID: 8, Wanted: true}}}
	steps = PlanEpisodes(cands, tg)
	if len(steps) != 1 || steps[0].Method != "hardlink" || steps[0].TargetSlotIDs[0] != 8 ||
		filepath.Base(steps[0].TargetPath) != "Blade Song - S05E12 - 112 - 1080p WEB.mkv" {
		t.Fatalf("absolute %+v", steps)
	}
	dl3 := t.TempDir()
	write(t, filepath.Join(dl3, "Other.Show.S01E01.1080p.WEB.H264-X.mkv"), 10)
	cands, _ = Scan(dl3)
	tg = ShowTarget{LibraryPath: lib, FolderName: "Copper Valley", Show: mediafiles.TitleRef{ID: 1, Name: "Copper Valley"}, Order: "aired",
		Episodes: []Episode{{Ref: mediafiles.EpisodeRef{ItemID: 1, Season: 1, Episode: 1}, SlotID: 1, Wanted: true}}}
	if steps = PlanEpisodes(cands, tg); steps[0].Method != "refuse" {
		t.Fatalf("other show %+v", steps)
	}
	dl4 := t.TempDir()
	write(t, filepath.Join(dl4, packName, "S01E01.mkv"), 10)
	cands, _ = Scan(dl4)
	if steps = PlanEpisodes(cands, tg); steps[0].Method != "hardlink" {
		t.Fatalf("bare name %+v", steps)
	}
}

func TestShowImportFreeSpacePerPack(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	fakeDisk(t, 2*2048+100*mb)
	sameDiskOf = func(string, string) bool { return false }
	t.Cleanup(func() { sameDiskOf = sameDisk })
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	placed := 0
	for k := 1; k <= 3; k++ {
		sl := w.slot(t, k)
		if sl.FileID != nil {
			placed++
			continue
		}
		if sl.LastReason == nil || !strings.Contains(*sl.LastReason, "free space") {
			t.Fatalf("E%d %+v", k, sl)
		}
	}
	if placed != 2 {
		t.Fatalf("placed %d, want 2", placed)
	}
	w2 := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	sameDiskOf = func(string, string) bool { return true }
	if _, err := (&Importer{Store: w2.s}).Import(context.Background(), w2.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 3; k++ {
		if w2.slot(t, k).FileID == nil {
			t.Fatalf("same disk E%d not placed", k)
		}
	}
}

func TestShowImportPathTooLong(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	old := MaxPathLen
	MaxPathLen = len(w.lib) + 20
	t.Cleanup(func() { MaxPathLen = old })
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := w.slot(t, 1)
	if sl.FileID != nil || sl.LastReason == nil || !strings.Contains(*sl.LastReason, "too long") {
		t.Fatalf("slot %+v", sl)
	}
}

func TestShowImportRunsOneAtATime(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	im := &Importer{Store: w.s}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	showImportEntered = func(int64) { entered <- struct{}{}; <-release }
	t.Cleanup(func() { showImportEntered = nil })
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := im.Import(context.Background(), w.dlID, false, domain.System)
			errs <- err
		}()
	}
	<-entered
	select {
	case <-entered:
		t.Fatal("second run entered while the first was running")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	<-errs
	<-errs
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files").Scan(&n)
	if n != 3 {
		t.Fatalf("files %d", n)
	}
	for k := 1; k <= 3; k++ {
		if w.slot(t, k).FileID == nil {
			t.Fatalf("E%d not placed", k)
		}
	}
}

func TestShowImportLibrarySurvivesDownloaderDelete(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2))
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	var placed []string
	for k := 1; k <= 2; k++ {
		sl := w.slot(t, k)
		if sl.FileID == nil {
			t.Fatalf("E%d not placed", k)
		}
		f := w.file(t, *sl.FileID)
		placed = append(placed, filepath.Join(w.lib, filepath.FromSlash(f.RelativePath)))
	}
	if sl := w.slot(t, 3); sl.ActiveDownloadID != nil {
		t.Fatalf("missing episode still holds the download: %+v", sl)
	}
	if err := os.RemoveAll(filepath.Join(w.dl, packName)); err != nil {
		t.Fatal(err)
	}
	for _, p := range placed {
		if fi, err := os.Stat(p); err != nil || fi.Size() != 2048 {
			t.Fatalf("library copy %s: %v", p, err)
		}
	}
}

func TestStrandedSweepNeverDoubleImports(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	var queued []int64
	add := func(_ context.Context, id int64) { queued = append(queued, id) }
	_, _ = im.RequeueStranded(ctx, add)
	_, _ = im.RequeueStranded(ctx, add)
	if len(queued) != 2 {
		t.Fatalf("queued %v", queued)
	}
	for _, id := range queued {
		if _, err := im.Import(ctx, id, false, domain.System); err != nil {
			t.Logf("second run: %v", err)
		}
	}
	var files, imports int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files").Scan(&files)
	_ = w.s.Read().QueryRow("SELECT count(*) FROM imports").Scan(&imports)
	if files != 3 || imports != 1 {
		t.Fatalf("files %d imports %d", files, imports)
	}
	for k := 1; k <= 3; k++ {
		if sl := w.slot(t, k); sl.State != mediafiles.SlotInLibrary && sl.State != mediafiles.SlotBetterWanted {
			t.Fatalf("E%d %s", k, sl.State)
		}
	}
	queued = nil
	if n, _ := im.RequeueStranded(ctx, add); n != 0 {
		t.Fatalf("swept an imported pack again: %v", queued)
	}
}

// A file named the way releases number an anime part (S02E01) imports into
// the stored episode that carries those numbers, named by the stored ones.
func TestShowImportReleaseNumbers(t *testing.T) {
	w := showSetup(t, []int{3, 4}, "Copper.Valley.S02E01.1080p.WEB.H264-HOLYARRS.mkv", ep(4))
	sl := w.slot(t, 3)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE items SET numbers = json_set(numbers, '$.alt_season', 2, '$.alt_episode', 1) WHERE id = ?`, sl.ItemID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
	for n, want := range map[int]string{
		3: "Copper Valley (2025)/Season 1/Copper Valley - S01E03 - Ash 1080p WEB.mkv",
		4: "Copper Valley (2025)/Season 1/Copper Valley - S01E04 - Spring 1080p WEB.mkv",
	} {
		sl := w.slot(t, n)
		if sl.FileID == nil {
			t.Fatalf("E%d slot %+v", n, sl)
		}
		if f := w.file(t, *sl.FileID); f.RelativePath != want || len(f.SlotIDs) != 1 || f.SlotIDs[0] != w.slots[n] {
			t.Fatalf("E%d file %+v, want %s", n, f, want)
		}
	}
}
