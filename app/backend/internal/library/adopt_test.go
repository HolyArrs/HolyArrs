package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// adoptTMDB stands in for TMDB in the adopt tests.
var adoptTMDB = &metadata.Static{Movies: []metadata.Movie{
	dune,
	{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, DigitalDate: d("1996-01-01")},
	{Name: "The Lantern Keepers", Year: 2027, IDs: map[string]string{"tmdb": "900001"}},
	{Name: "The Lantern Keepers Return", Year: 2029, IDs: map[string]string{"tmdb": "900002"}},
	{Name: "Empty Movie", Year: 2001, IDs: map[string]string{"tmdb": "5001"}, DigitalDate: d("2001-05-01")},
}}

// snapshot lists every file under root with its content hash.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		sum := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func adoptTree(t *testing.T, lib string) {
	t.Helper()
	writeFile(t, filepath.Join(lib, "Dune - Part Two (2024)", "Dune.Part.Two.2024.1080p.WEB-DL.H264-HOLYARRS.mkv"), strings.Repeat("d", 300))
	writeFile(t, filepath.Join(lib, "Dune - Part Two (2024)", "Sample", "dune-sample.mkv"), "s")
	writeFile(t, filepath.Join(lib, "Dune - Part Two (2024)", "Dune.nfo"), "info")
	writeFile(t, filepath.Join(lib, "Heat (1995) {tmdb-949}", "heat.avi"), "h")
	writeFile(t, filepath.Join(lib, "Lantern Keepers", "lk.mkv"), "l")
	writeFile(t, filepath.Join(lib, "Random Stuff", "holiday.mp4"), "r")
	writeFile(t, filepath.Join(lib, "Empty Movie (2001)", "readme.txt"), "e")
	writeFile(t, filepath.Join(lib, ".holyarrs-replaced", "2026-10-01", "x.mkv"), "x")
	writeFile(t, filepath.Join(lib, "loose file.mkv"), "z")
}

func byFolder(p AdoptPlan) map[string]AdoptCandidate {
	m := map[string]AdoptCandidate{}
	for _, c := range p.Folders {
		m[c.Folder] = c
	}
	return m
}

// 5.1.2: the planner matches movie folders with a confidence score and
// writes nothing.
func TestPlanAdoptOnAFixtureTree(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	before := snapshot(t, lf.Path)
	writes := s.WriteCount()
	plan, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.WriteCount() != writes || fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("planning changed something")
	}
	got := byFolder(plan)
	if len(got) != 5 {
		t.Fatalf("folders %v", plan.Folders)
	}
	for _, c := range []struct {
		folder, status, tmdb string
		minScore             float64
	}{
		{"Dune - Part Two (2024)", "matched", "693134", 0.95},
		{"Heat (1995) {tmdb-949}", "matched", "949", 1},
		{"Lantern Keepers", "unsure", "900001", 0.5},
		{"Random Stuff", "unmatched", "", 0},
		{"Empty Movie (2001)", "no_video", "5001", 0.95},
	} {
		g := got[c.folder]
		if g.Status != c.status {
			t.Errorf("%s: status %s (%s), want %s", c.folder, g.Status, g.Why, c.status)
		}
		if c.tmdb == "" {
			if g.Match != nil {
				t.Errorf("%s: matched %+v", c.folder, g.Match)
			}
			continue
		}
		if g.Match == nil || g.Match.TMDB != c.tmdb || g.Match.Score < c.minScore || g.Match.Score > 1 {
			t.Errorf("%s: match %+v", c.folder, g.Match)
		}
	}
	if d := got["Dune - Part Two (2024)"]; d.Main != "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.WEB-DL.H264-HOLYARRS.mkv" || len(d.Videos) != 1 {
		t.Fatalf("dune videos %+v main %q", d.Videos, d.Main)
	}
	if plan.Counts["matched"] != 2 || plan.Counts["unsure"] != 1 {
		t.Fatalf("counts %v", plan.Counts)
	}
}

// Adopting adds the movies with their folder as it is: no file is moved,
// renamed or written, and the Files are adopted.
func TestAdoptMovesNothing(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	before := snapshot(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"},
		{Folder: "Empty Movie (2001)", TMDB: "5001"}, {Folder: "../outside", TMDB: "949"}, {Folder: "Random Stuff", TMDB: "693134"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("adopting changed files on disk")
	}
	if res[0].TitleID == 0 || res[1].TitleID == 0 || res[2].TitleID == 0 || res[3].Error == "" || !strings.Contains(res[4].Error, "already") {
		t.Fatalf("results %+v", res)
	}
	files, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "1=1 ORDER BY id")
	if len(files) != 2 {
		t.Fatalf("files %+v", files)
	}
	paths := []string{files[0].RelativePath, files[1].RelativePath}
	sort.Strings(paths)
	if paths[0] != "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.WEB-DL.H264-HOLYARRS.mkv" || paths[1] != "Heat (1995) {tmdb-949}/heat.avi" {
		t.Fatalf("paths %v", paths)
	}
	for _, f := range files {
		if f.Origin != "adopted" || f.PlacedBy != "none" || f.State != "present" {
			t.Fatalf("file %+v", f)
		}
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", res[1].TitleID)
	if len(vs) != 1 || vs[0].FolderName != "Heat (1995) {tmdb-949}" {
		t.Fatalf("version %+v", vs)
	}
	for i, want := range []mediafiles.SlotState{mediafiles.SlotInLibrary, mediafiles.SlotBetterWanted, mediafiles.SlotWanted} {
		sl := slotsOf(t, s, res[i].TitleID)
		if sl[0].State != want {
			t.Errorf("%s: slot %s, want %s", res[i].Folder, sl[0].State, want)
		}
		if want != mediafiles.SlotWanted && (sl[0].FileID == nil || *sl[0].FileID == 0) {
			t.Errorf("%s: slot has no file", res[i].Folder)
		}
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", res[0].TitleID, &ti)
	if ti.WhyAdded != "import" {
		t.Fatalf("why added %s", ti.WhyAdded)
	}
	// A second scan sees them as in the Library.
	plan, _ := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if got := byFolder(plan)["Dune - Part Two (2024)"]; got.Status != "in_library" {
		t.Fatalf("rescan %+v", got)
	}
	// Adopted files are renamed only when asked, and then by the pattern.
	rp, _ := PlanRename(ctx, s.Read(), []int64{res[0].TitleID})
	if len(rp) != 1 || rp[0].To != "Dune - Part Two (2024)/Dune - Part Two (2024) 1080p WEB.mkv" {
		t.Fatalf("rename plan %+v", rp)
	}
}

// A Library folder that is gone (a mount that dropped) is a warning; no
// File becomes missing. A real missing file does.
func TestRescanMountGoneIsAWarning(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
	if err != nil || res[0].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	states := func() map[string]string {
		fs, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "1=1")
		m := map[string]string{}
		for _, f := range fs {
			m[f.RelativePath] = f.State + "/" + f.Origin
		}
		return m
	}
	// A file vanishes, and a stranger appears in a movie folder.
	if err := os.Remove(filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "heat.avi")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, "Dune - Part Two (2024)", "bonus.mkv"), "b")
	r, err := svc.Rescan(ctx, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	got := states()
	if got["Heat (1995) {tmdb-949}/heat.avi"] != "missing/adopted" || got["Dune - Part Two (2024)/bonus.mkv"] != "present/found" || r.Missing != 1 || r.Found != 1 {
		t.Fatalf("states %v result %+v", got, r)
	}
	if es, _, _ := svc.Store.Events(ctx, database.EventFilter{Kinds: []string{domain.EventLibraryRescanned}}); len(es) != 1 || es[0].Data["missing"] != float64(1) {
		t.Fatalf("rescan events %+v", es)
	}
	// It comes back.
	writeFile(t, filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "heat.avi"), "h")
	if r, _ = svc.Rescan(ctx, domain.System); r.Back != 1 || states()["Heat (1995) {tmdb-949}/heat.avi"] != "present/adopted" {
		t.Fatalf("back %+v %v", r, states())
	}
	// The mount drops: the folder is empty, or gone altogether.
	for _, drop := range []string{"empty", "gone"} {
		moved := lf.Path + "-away"
		if err := os.Rename(lf.Path, moved); err != nil {
			t.Fatal(err)
		}
		if drop == "empty" {
			if err := os.Mkdir(lf.Path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		r, err := svc.Rescan(ctx, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if r.Unreachable != 1 || r.Missing != 0 {
			t.Fatalf("%s: %+v", drop, r)
		}
		for p, st := range states() {
			if !strings.HasPrefix(st, "present/") {
				t.Fatalf("%s: %s became %s", drop, p, st)
			}
		}
		evs, _ := s.RecentEvents(ctx, 5)
		// Newest first: the rescan's summary, then the warning.
		if len(evs) < 2 || evs[0].Kind != domain.EventLibraryRescanned || evs[1].Kind != domain.EventFolderUnreachable {
			t.Fatalf("%s: events %+v", drop, evs)
		}
		if drop == "empty" {
			_ = os.Remove(lf.Path)
		}
		if err := os.Rename(moved, lf.Path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScanAndAdoptJobs(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	svc, q, _ := runQueue(t, s, adoptTMDB)
	ctx := context.Background()
	id, err := svc.ScanFolder(ctx, lf.ID, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	if js[0].ID != id || js[0].State != "done" || js[0].Result["plan"] == nil {
		t.Fatalf("scan job %+v", js[0])
	}
	if _, _, err := q.Enqueue(ctx, KindRescan, nil, nil, "", domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdoptLater(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, j := range waitJobs(t, s, 3) {
		if j.State != "done" {
			t.Fatalf("job %s %+v", j.Kind, j.Error)
		}
	}
	if _, err := svc.ScanFolder(ctx, 999, "", domain.System); err == nil {
		t.Fatal("unknown folder scanned")
	}
}

// failingTMDB fails the lookup of one folder name.
type failingTMDB struct {
	*metadata.Static
	bad string
}

func (f failingTMDB) Search(ctx context.Context, q string, year int) ([]metadata.Movie, error) {
	if q == f.bad {
		return nil, errors.New("tmdb answered 503")
	}
	return f.Static.Search(ctx, q, year)
}

// 5.1.7 minor 9: one failed lookup leaves that folder unmatched with the
// reason; the rest of the plan is still made.
func TestPlanAdoptOneLookupErrorDoesNotAbort(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	plan, err := PlanAdopt(context.Background(), s.Read(), failingTMDB{adoptTMDB, "Random Stuff"}, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := byFolder(plan)
	if c := got["Random Stuff"]; c.Status != "unmatched" || !strings.Contains(c.Why, "lookup failed") {
		t.Fatalf("failed folder %+v", c)
	}
	if got["Dune - Part Two (2024)"].Status != "matched" {
		t.Fatalf("others %+v", plan.Folders)
	}
}

// 5.1.7 minor 9: a scan reads at most the scan limit of folders and says
// where to go on; the next scan starts after the last one.
func TestPlanAdoptIsCappedAndGoesOn(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := range 13 {
		writeFile(t, filepath.Join(lf.Path, fmt.Sprintf("Movie %02d (2001)", i), "m.mkv"), "m")
	}
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if err != nil || len(p.Folders) != 10 || !p.More || p.Next != "Movie 09 (2001)" || p.Limit != 10 {
		t.Fatalf("first %d more=%v next=%q %v", len(p.Folders), p.More, p.Next, err)
	}
	p, err = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, p.Next)
	if err != nil || len(p.Folders) != 3 || p.More || p.Folders[0].Folder != "Movie 10 (2001)" {
		t.Fatalf("rest %+v %v", p, err)
	}
}
