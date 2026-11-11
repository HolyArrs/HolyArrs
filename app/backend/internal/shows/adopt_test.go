package shows

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// showTree is a show Library folder as people keep one: the lab show with
// season folders, a multi-episode file, a sample, a file of no episode, a
// file of another show; and a folder no source knows.
func showTree(t *testing.T, lib string) {
	t.Helper()
	show := filepath.Join(lib, "Lab Show (2025)")
	writeFile(t, filepath.Join(show, "Season 1", "Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv"), strings.Repeat("a", 300))
	writeFile(t, filepath.Join(show, "Season 1", "Lab Show - S01E02 - Second.mkv"), strings.Repeat("b", 200))
	writeFile(t, filepath.Join(show, "Season 1", "S01E02.small.mkv"), "b")
	writeFile(t, filepath.Join(show, "Season 2", "Lab.Show.S02E01E02.720p.HDTV.x264-GRP.mkv"), strings.Repeat("c", 400))
	writeFile(t, filepath.Join(show, "Season 2", "Sample", "lab-sample.mkv"), "s")
	writeFile(t, filepath.Join(show, "Extras", "behind the lab.mkv"), "e")
	writeFile(t, filepath.Join(show, "Season 1", "Other.Show.S01E03.mkv"), "o")
	writeFile(t, filepath.Join(show, "tvshow.nfo"), "info")
	writeFile(t, filepath.Join(lib, "Holiday Videos", "beach.mp4"), "r")
}

// 5.2.2: the planner matches show folders with a confidence score, maps
// each file to its episodes and writes nothing.
func TestPlanShowAdopt(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	before := snapshot(t, lf.Path)
	p, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "show" || p.Warning != "" {
		t.Fatalf("plan %+v", p)
	}
	c := byFolder(p)
	lab := c["Lab Show (2025)"]
	if lab.Status != "matched" || lab.Match == nil || lab.Match.ID != "tmdb:100" || lab.Match.Score != 1 {
		t.Fatalf("lab %+v", lab)
	}
	got := map[string][]string{}
	why := map[string]string{}
	for _, f := range lab.Files {
		got[f.Path] = f.Episodes
		why[f.Path] = f.Why
	}
	want := map[string][]string{
		"Lab Show (2025)/Season 1/Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv": {"S01E01"},
		"Lab Show (2025)/Season 1/Lab Show - S01E02 - Second.mkv":            {"S01E02"},
		"Lab Show (2025)/Season 2/Lab.Show.S02E01E02.720p.HDTV.x264-GRP.mkv": {"S02E01", "S02E02"},
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("%s: %v (%s), want %v", k, got[k], why[k], v)
		}
	}
	for _, k := range []string{"Lab Show (2025)/Season 1/S01E02.small.mkv", "Lab Show (2025)/Extras/behind the lab.mkv", "Lab Show (2025)/Season 1/Other.Show.S01E03.mkv"} {
		if len(got[k]) != 0 || why[k] == "" {
			t.Errorf("%s should be left alone with a reason: %v %q", k, got[k], why[k])
		}
	}
	if _, ok := got["Lab Show (2025)/Season 2/Sample/lab-sample.mkv"]; ok {
		t.Error("a sample is listed")
	}
	if lab.Episodes != 4 || lab.Unknown != 3 {
		t.Errorf("counts %d %d", lab.Episodes, lab.Unknown)
	}
	if !strings.Contains(why["Lab Show (2025)/Season 1/Other.Show.S01E03.mkv"], "S01E03") {
		t.Errorf("other show why %q", why["Lab Show (2025)/Season 1/Other.Show.S01E03.mkv"])
	}
	if h := c["Holiday Videos"]; h.Status != "unmatched" {
		t.Errorf("holiday %+v", h)
	}
	if !reflect.DeepEqual(snapshot(t, lf.Path), before) || count(t, s, "titles") != 0 {
		t.Fatal("a dry run changed something")
	}
	// A movie folder is not scanned as shows, and the other way round.
	if _, err := library.PlanAdoptFrom(ctx, s.Read(), adoptTMDB, lf.ID, ""); err != library.ErrNotMovieFolder {
		t.Errorf("movie plan on a show folder: %v", err)
	}
	mf := addFolder(t, s)
	if _, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, mf.ID, ""); err != library.ErrNotShowFolder {
		t.Errorf("show plan on a movie folder: %v", err)
	}
}

// 5.2.2: adopting keeps every file where it is, fills the episodes the
// files hold (a multi-episode file fills both), reports the files left
// alone, and a second scan sees the folder in the Library.
func TestAdoptShowMovesNothing(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	before := snapshot(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}, {Folder: "../x", ID: "tmdb:100"}, {Folder: "Nope", ID: "tmdb:100"}}, domain.System)
	if err != nil || len(res) != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	if res[0].Error != "" || res[0].TitleID == 0 || res[0].Episodes != 4 || len(res[0].Unknown) != 3 {
		t.Fatalf("adopt %+v", res[0])
	}
	if res[1].Error == "" || res[2].Error == "" {
		t.Fatalf("bad folders adopted: %+v", res[1:])
	}
	if !reflect.DeepEqual(snapshot(t, lf.Path), before) {
		t.Fatal("adopting moved, renamed or wrote a file")
	}
	id := res[0].TitleID
	var v mediafiles.Version
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id)
	v = vs[0]
	if v.FolderName != "Lab Show (2025)" || v.LibraryFolderID != lf.ID {
		t.Fatalf("version %+v", v)
	}
	eps := episodes(t, s, id)
	for _, k := range []string{"S01E01", "S01E02", "S02E01", "S02E02"} {
		if st := eps[k].State; st != mediafiles.SlotInLibrary && st != mediafiles.SlotBetterWanted {
			t.Errorf("%s %s", k, st)
		}
	}
	if eps["S02E03"].State == mediafiles.SlotInLibrary || eps["S00E01"].State == mediafiles.SlotInLibrary {
		t.Error("an episode with no file is in the library")
	}
	files, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "version_id = ? ORDER BY id", v.ID)
	if len(files) != 3 {
		t.Fatalf("files %+v", files)
	}
	multi := 0
	for _, f := range files {
		if f.Origin != "adopted" || f.State != "present" || f.PlacedBy != "none" {
			t.Errorf("file %+v", f)
		}
		if len(f.SlotIDs) == 2 {
			multi++
			if f.SlotIDs[0] != eps["S02E01"].SlotID || f.SlotIDs[1] != eps["S02E02"].SlotID {
				t.Errorf("multi slots %v", f.SlotIDs)
			}
		}
	}
	if multi != 1 {
		t.Error("the multi-episode file does not hold two episodes")
	}
	if es, _, _ := svc.Store.Events(ctx, database.EventFilter{Kinds: []string{domain.EventAdopted}}); len(es) != 1 ||
		es[0].Summary != "Adopted Lab Show from Lab Show (2025): 4 episodes in 3 files, 3 files left alone" {
		t.Errorf("adopted Event %+v", es)
	}
	p, _ := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if c := byFolder(p)["Lab Show (2025)"]; c.Status != "in_library" || c.TitleID != id {
		t.Errorf("second scan %+v", c)
	}
	// The same show cannot be adopted twice from another folder.
	if err := os.Rename(filepath.Join(lf.Path, "Holiday Videos"), filepath.Join(lf.Path, "Lab Show")); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show", ID: "tmdb:100"}}, domain.System)
	if res[0].Error == "" {
		t.Error("the show was adopted twice")
	}
}

// 5.2.2: a rescan marks a missing episode file and one that came back,
// links a new video to its empty episode, and an empty Library
// folder (a dropped mount) marks nothing missing and warns in the scan.
func TestRescanShowFolder(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("%+v %v", res, err)
	}
	first := filepath.Join(lf.Path, "Lab Show (2025)", "Season 1", "Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv")
	if err := os.Rename(first, first+".away"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, "Lab Show (2025)", "Season 2", "Lab.Show.S02E03.1080p.WEB.mkv"), "new")
	r, err := svc.Rescan(ctx, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// The found files: the new one plus the three left alone at adopting.
	if r.Missing != 1 || r.Found != 4 || r.Unreachable != 0 {
		t.Fatalf("rescan %+v", r)
	}
	eps := episodes(t, s, res[0].TitleID)
	var found mediafiles.File
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path LIKE '%S02E03%'").Scan(&found.ID); err != nil {
		t.Fatal(err)
	}
	_ = database.Get(ctx, s.Read(), "files", found.ID, &found)
	if found.Origin != "adopted" || len(found.SlotIDs) != 1 || found.SlotIDs[0] != eps["S02E03"].SlotID {
		t.Errorf("found %+v", found)
	}
	if st := eps["S02E03"].State; st != mediafiles.SlotInLibrary && st != mediafiles.SlotBetterWanted {
		t.Errorf("the new file did not fill its empty episode: %s", st)
	}
	if err := os.Rename(first+".away", first); err != nil {
		t.Fatal(err)
	}
	if r, _ := svc.Rescan(ctx, domain.System); r.Back != 1 || r.Found != 0 {
		t.Errorf("second rescan %+v", r)
	}
	// A dropped mount: nothing becomes missing, and the scan warns.
	if err := os.RemoveAll(filepath.Join(lf.Path, "Lab Show (2025)")); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Join(lf.Path, "Holiday Videos"))
	if r, _ := svc.Rescan(ctx, domain.System); r.Unreachable != 1 || r.Missing != 0 {
		t.Errorf("dropped mount %+v", r)
	}
	if p, _ := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, ""); !strings.Contains(p.Warning, "not mounted") {
		t.Errorf("warning %q", p.Warning)
	}
}
