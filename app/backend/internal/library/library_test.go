package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func open(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var (
	now  = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	dune = metadata.Movie{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"}, Runtime: 166,
		CinemaDate: d("2024-03-01"), DigitalDate: d("2024-04-16"), PhysicalDate: d("2024-05-14")}
	future = metadata.Movie{Name: "The Lantern Keepers", Year: 2027, IDs: map[string]string{"tmdb": "900001"},
		CinemaDate: d("2027-02-01")}
)

func addFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := AddFolder(context.Background(), s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func slotsOf(t *testing.T, s *database.Store, titleID int64) []mediafiles.Slot {
	t.Helper()
	sl, err := database.Select[mediafiles.Slot](context.Background(), s.Read(), "slots", "item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", titleID)
	if err != nil {
		t.Fatal(err)
	}
	return sl
}

func TestAddMovieNeedsAFolder(t *testing.T) {
	if _, err := AddMovie(context.Background(), open(t), dune, AddOptions{}, nil, now, domain.System); !errors.Is(err, ErrNoFolder) {
		t.Fatalf("err %v", err)
	}
}

func TestAddMovieOnceWithGateAndRefusals(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil || id == 0 {
		t.Fatalf("id %d err %v", id, err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.SortName != "dune part two" || ti.Year == nil || *ti.Year != 2024 || ti.ReleaseGate != "released" || ti.GateDelayDays != 0 ||
		ti.GateDriver != "digital" || ti.GateOpensAt == nil || !ti.GateOpensAt.Equal(*d("2024-04-16")) || ti.Status != "released" ||
		ti.WhyAdded != "by_hand" || ti.MetadataSource != "tmdb" || !ti.KeepUpToDate {
		t.Fatalf("title %+v", ti)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id)
	sl := slotsOf(t, s, id)
	if len(vs) != 1 || vs[0].FolderName != "Dune - Part Two (2024)" || !vs[0].IsMain || vs[0].LibraryFolderID != lf.ID ||
		len(sl) != 1 || sl[0].State != mediafiles.SlotWanted {
		t.Fatalf("versions %+v slots %+v", vs, sl)
	}

	var dup *AlreadyAddedError
	if _, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System); !errors.As(err, &dup) || dup.TitleID != id {
		t.Fatalf("second add: %v", err)
	}

	if _, err := AddNeverAdd(ctx, s, mediafiles.NeverAdd{SourceIDs: map[string]string{"tmdb": "900001"}, Name: "The Lantern Keepers", Reason: "not for us"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System); !errors.Is(err, ErrNeverAdd) {
		t.Fatalf("never-add: %v", err)
	}
	na, _ := NeverAddList(ctx, s.Read())
	if err := RemoveNeverAdd(ctx, s, na[0].ID); err != nil {
		t.Fatal(err)
	}

	// Not out yet: the gate is in the future, with the reason on the Slot.
	keep := false
	delay := 3
	fid, err := AddMovie(ctx, s, future, AddOptions{ReleaseGate: GateInCinemas, GateDelayDays: &delay, KeepUpToDate: &keep, WhyAdded: "import"}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	fs := slotsOf(t, s, fid)
	var ft mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", fid, &ft)
	if fs[0].State != mediafiles.SlotNotOutYet || fs[0].LastReason == nil || *fs[0].LastReason != "release gate: searching from 2027-02-04 (cinema date)" ||
		ft.KeepUpToDate || ft.WhyAdded != "import" || ft.Status != "announced" {
		t.Fatalf("future slot %+v title %+v", fs[0], ft)
	}

	if _, err := AddMovie(ctx, s, metadata.Movie{Name: "X", IDs: map[string]string{"tmdb": "5"}}, AddOptions{ReleaseGate: "soon"}, nil, now, domain.System); !errors.Is(err, ErrBadGate) {
		t.Fatalf("bad gate: %v", err)
	}
}

func TestAddMovieRefusesUnusableFolder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	if err := os.Remove(lf.Path); err != nil {
		t.Fatal(err)
	}
	var fe *FolderError
	_, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if !errors.As(err, &fe) || fe.Health.State != "missing" || fe.Health.Reason == "" {
		t.Fatalf("err %v", err)
	}
	if n := count(t, s, "titles"); n != 0 {
		t.Fatalf("%d titles after a refused add", n)
	}
}

func count(t *testing.T, s *database.Store, table string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGateDefaultsSetting(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	g, err := ReadGateDefaults(ctx, s.Read())
	if err != nil || g.Gate != "released" || g.DelayDays != 0 || !g.BulkRespectsGate {
		t.Fatalf("defaults %+v %v", g, err)
	}
	if err := SetGateDefaults(ctx, s, GateDefaults{Gate: GateInCinemas, DelayDays: 2, BulkRespectsGate: false}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := SetGateDefaults(ctx, s, GateDefaults{Gate: "x"}, domain.System); !errors.Is(err, ErrBadGate) {
		t.Fatalf("bad gate %v", err)
	}
	id, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.ReleaseGate != GateInCinemas || ti.GateDelayDays != 2 || ti.GateOpensAt.Format("2006-01-02") != "2027-02-03" {
		t.Fatalf("title from new defaults %+v", ti)
	}
}

func TestSortName(t *testing.T) {
	if got := SortName("The Matrix"); got != "matrix" {
		t.Fatalf("got %q", got)
	}
}

func TestFolderChecksAndCRUD(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if h := CheckFolder(filepath.Join(t.TempDir(), "nope"), nil); h.State != "missing" || h.Usable() {
		t.Fatalf("missing: %+v", h)
	}
	lib, dl := t.TempDir(), t.TempDir()
	if h := CheckFolder(lib, []string{dl}); h.State != "ok" || !h.Usable() {
		t.Fatalf("same disk: %+v", h)
	}
	if h := CheckFolder(lib, []string{filepath.Join(dl, "gone")}); h.State != "no_hardlink" || !h.Usable() || h.Reason == "" {
		t.Fatalf("unreachable downloader folder: %+v", h)
	}
	if entries, _ := os.ReadDir(lib); len(entries) != 0 {
		t.Fatalf("probe files left behind: %v", entries)
	}

	if _, err := AddFolder(ctx, s, FolderInput{Path: "relative/path", Kind: "movie"}, nil, domain.System); !errors.Is(err, ErrFolderPath) {
		t.Fatalf("relative: %v", err)
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "books"}, nil, domain.System); !errors.Is(err, ErrFolderKind) {
		t.Fatalf("kind: %v", err)
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: filepath.Join(lib, "nope"), Kind: "movie"}, nil, domain.System); !errors.Is(err, ErrFolderAbsent) {
		t.Fatalf("absent: %v", err)
	}
	lf, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie"}, nil, domain.System)
	if err != nil || lf.Health != "ok" || lf.Label != filepath.Base(lib) {
		t.Fatalf("add %+v %v", lf, err)
	}
	inner := filepath.Join(lib, "inner")
	_ = os.Mkdir(inner, 0o755)
	if _, err := AddFolder(ctx, s, FolderInput{Path: inner, Kind: "movie"}, nil, domain.System); !errors.Is(err, ErrFolderTaken) {
		t.Fatalf("nested: %v", err)
	}
	label := "Films"
	bad := int64(999)
	if _, err := UpdateFolder(ctx, s, lf.ID, FolderChange{DefaultPickRules: &bad}, domain.System); !errors.Is(err, ErrNoPickRules) {
		t.Fatalf("bad rules: %v", err)
	}
	if got, err := UpdateFolder(ctx, s, lf.ID, FolderChange{Label: &label}, domain.System); err != nil || got.Label != "Films" {
		t.Fatalf("update %+v %v", got, err)
	}
	if _, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFolder(ctx, s, lf.ID); !errors.Is(err, ErrFolderInUse) {
		t.Fatalf("in use: %v", err)
	}
	empty, _ := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "show"}, nil, domain.System)
	if err := DeleteFolder(ctx, s, empty.ID); err != nil {
		t.Fatal(err)
	}
	// A folder that disappears is seen by the recheck.
	_ = os.RemoveAll(inner)
	gone := t.TempDir()
	g, _ := AddFolder(ctx, s, FolderInput{Path: gone, Kind: "music"}, nil, domain.System)
	_ = os.Remove(gone)
	got, changed, err := RecheckFolder(ctx, s, g.ID, nil, domain.System, false)
	if err != nil || !changed || got.Health != "missing" {
		t.Fatalf("recheck %+v %v %v", got, changed, err)
	}
	var stored mediafiles.LibraryFolder
	_ = database.Get(ctx, s.Read(), "library_folders", g.ID, &stored)
	if stored.Health != "missing" || stored.HealthReason == "" {
		t.Fatalf("stored %+v", stored)
	}
}

// runQueue starts a queue with the library's handlers and a fake search.
func runQueue(t *testing.T, s *database.Store, md metadata.Provider) (*Service, *jobs.Queue, chan int64) {
	t.Helper()
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	svc := &Service{Store: s, Queue: q, Metadata: md, Now: func() time.Time { return now }}
	svc.Register(q)
	searched := make(chan int64, 10)
	q.Handle(KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { searched <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	return svc, q, searched
}

func waitJobs(t *testing.T, s *database.Store, n int) []domain.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "1=1 ORDER BY id")
		done := 0
		for _, j := range js {
			if j.State == "done" || j.State == "failed" {
				done++
			}
		}
		if len(js) >= n && done == len(js) {
			return js
		}
		time.Sleep(10 * time.Millisecond)
	}
	js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "1=1 ORDER BY id")
	t.Fatalf("jobs never settled: %+v", js)
	return nil
}

func TestAddQueuesRefreshScanSearchInOrder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, searched := runQueue(t, s, &metadata.Static{Movies: []metadata.Movie{dune}})
	id, _, err := svc.Add(ctx, dune, AddOptions{SearchOnAdd: true}, domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 3)
	if len(js) != 3 || js[0].Kind != KindRefreshTitle || js[1].Kind != KindScanTitle || js[2].Kind != KindSearchSlot {
		t.Fatalf("jobs %+v", js)
	}
	for i, j := range js {
		if j.State != "done" {
			t.Fatalf("job %s: %s %+v", j.Kind, j.State, j.Error)
		}
		if i > 0 && j.CreatedAt.Before(*js[i-1].StartedAt) {
			t.Fatalf("%s was queued before %s started", j.Kind, js[i-1].Kind)
		}
	}
	if got := <-searched; got != slotsOf(t, s, id)[0].ID {
		t.Fatalf("searched slot %d", got)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.RefreshedAt == nil {
		t.Fatal("not refreshed")
	}
	es, _ := s.RecentEvents(ctx, 10)
	kinds := map[string]bool{}
	for _, e := range es {
		kinds[e.Kind] = true
	}
	for _, k := range []string{domain.EventTitleAdded, domain.EventTitleRefreshed, domain.EventFolderScanned} {
		if !kinds[k] {
			t.Errorf("no %s event in %v", k, kinds)
		}
	}
}

func TestAddWithoutSearchAndNotOutYetNeverSearches(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, searched := runQueue(t, s, &metadata.Static{Movies: []metadata.Movie{dune, future}})
	if _, _, err := svc.Add(ctx, dune, AddOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Add(ctx, future, AddOptions{SearchOnAdd: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 4)
	if len(js) != 4 {
		t.Fatalf("jobs %+v", js)
	}
	select {
	case id := <-searched:
		t.Fatalf("slot %d searched", id)
	default:
	}
}

func TestEditPickRulesGoodEnoughWithDryRun(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, _ := runQueue(t, s, &metadata.Static{Movies: []metadata.Movie{dune}})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// Put a 1080p WEB file in the library: good enough under Standard.
	sl := slotsOf(t, s, id)[0]
	strict := mediafiles.GoodEveryday()
	strict.GoodEnoughAt = "2160p_web"
	var strictID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		name := "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
		f := &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: name + ".mkv", Size: 1, AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state = 'in_library', file_id = ?, good_enough = 1 WHERE id = ?", f.ID, sl.ID); err != nil {
			return err
		}
		var err error
		strictID, err = database.Insert(tx, "pick_rules", &mediafiles.PickRules{Name: "Strict", Kinds: []string{"movie"}, Revision: 1, Body: strict}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Edit(ctx, id, TitleChange{PickRulesID: &strictID}, true, domain.System)
	if err != nil || !res.DryRun || res.GoodEnoughChange != 1 || len(res.SlotChanges) != 1 || res.SlotChanges[0].To != "better_wanted" {
		t.Fatalf("dry run %+v %v", res, err)
	}
	if got := slotsOf(t, s, id)[0]; got.State != mediafiles.SlotInLibrary || !got.GoodEnough {
		t.Fatalf("dry run changed the slot: %+v", got)
	}
	res, err = svc.Edit(ctx, id, TitleChange{PickRulesID: &strictID}, false, domain.System)
	if err != nil || res.GoodEnoughChange != 1 {
		t.Fatalf("edit %+v %v", res, err)
	}
	if got := slotsOf(t, s, id)[0]; got.State != mediafiles.SlotBetterWanted || got.GoodEnough {
		t.Fatalf("after edit: %+v", got)
	}
	// Turning Keep up to date off stops looking for a better copy.
	off := false
	if _, err := svc.Edit(ctx, id, TitleChange{KeepUpToDate: &off}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := slotsOf(t, s, id)[0]; got.State != mediafiles.SlotInLibrary {
		t.Fatalf("keep off: %+v", got)
	}
}

func TestEditGateMovesSlots(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, _ := runQueue(t, s, &metadata.Static{})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	long := 1000
	if _, err := svc.Edit(ctx, id, TitleChange{GateDelayDays: &long}, false, domain.System); !errors.Is(err, ErrBadDelay) {
		t.Fatalf("delay: %v", err)
	}
	year := 365
	res, err := svc.Edit(ctx, id, TitleChange{GateDelayDays: &year}, false, domain.System)
	// 16 April 2024 + 365 days is still before now, so nothing moves.
	if err != nil || len(res.SlotChanges) != 0 {
		t.Fatalf("delay edit %+v %v", res, err)
	}
	ann := GateAnnounced
	if _, err := svc.Edit(ctx, id, TitleChange{ReleaseGate: &ann}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	fid, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err = svc.Edit(ctx, fid, TitleChange{ReleaseGate: &ann}, false, domain.System)
	if err != nil || len(res.SlotChanges) != 1 || res.SlotChanges[0].To != "wanted" || res.GateDriver != "announced" {
		t.Fatalf("open gate %+v %v", res, err)
	}
	rel := GateReleased
	res, err = svc.Edit(ctx, fid, TitleChange{ReleaseGate: &rel}, false, domain.System)
	if err != nil || len(res.SlotChanges) != 1 || res.SlotChanges[0].To != "not_out_yet" {
		t.Fatalf("close gate %+v %v", res, err)
	}
}

func TestMoveDryRunThenApply(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	from := addFolder(t, s)
	to, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Second"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	svc, _, _ := runQueue(t, s, &metadata.Static{})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	dir := filepath.Join(from.Path, "Dune - Part Two (2024)")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "dune.mkv"), []byte("movie bytes"), 0o644)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: "Dune - Part Two (2024)/dune.mkv", Size: 11, AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Edit(ctx, id, TitleChange{LibraryFolderID: &to.ID}, false, domain.System)
	if err != nil || res.MoveJobID == nil || res.FolderMoved {
		t.Fatalf("edit %+v %v", res, err)
	}
	js := waitJobs(t, s, 1)
	if js[0].Result["dry_run"] != true || js[0].Result["files"] != float64(1) {
		t.Fatalf("dry run job %+v", js[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "dune.mkv")); err != nil {
		t.Fatal("dry run moved the file")
	}
	var v mediafiles.Version
	_ = database.Get(ctx, s.Read(), "versions", sl.VersionID, &v)
	if v.LibraryFolderID != from.ID {
		t.Fatal("dry run changed the Version folder")
	}
	if _, err := svc.Move(ctx, id, to.ID, true, domain.System); err != nil {
		t.Fatal(err)
	}
	js = waitJobs(t, s, 2)
	if js[1].State != "done" {
		t.Fatalf("apply %+v", js[1])
	}
	b, err := os.ReadFile(filepath.Join(to.Path, "Dune - Part Two (2024)", "dune.mkv"))
	if err != nil || string(b) != "movie bytes" {
		t.Fatalf("moved file %q %v", b, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("old empty folder left behind")
	}
	_ = database.Get(ctx, s.Read(), "versions", sl.VersionID, &v)
	if v.LibraryFolderID != to.ID {
		t.Fatal("Version folder not changed")
	}
}

func TestMoveRefusesFoundFiles(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	to, _ := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie"}, nil, domain.System)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{}, VersionID: sl.VersionID, RelativePath: "Dune - Part Two (2024)/x.mkv", AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "found", PlacedBy: "none", Role: "main", State: "present"}, domain.System)
		return err
	})
	plans, err := PlanMove(ctx, s.Read(), s.Read(), id, to.ID)
	if err != nil || len(plans) != 1 || plans[0].Blocked == "" || len(plans[0].Steps) != 0 {
		t.Fatalf("plans %+v %v", plans, err)
	}
}

func TestMoveFileResumes(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	_ = os.WriteFile(to, []byte("done"), 0o644)
	if err := moveFile(from, to); err == nil {
		t.Fatal("an unrecorded missing source adopted an unrelated destination")
	}
	proof, err := fileProof(to)
	if err != nil {
		t.Fatal(err)
	}
	if err := moveRecordedFile(from, to, proof); err != nil {
		t.Fatalf("recorded move did not resume: %v", err)
	}
	_ = os.WriteFile(from, []byte("done"), 0o644)
	if err := moveFile(from, to); err != nil {
		t.Fatalf("copied but source left: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("source not removed")
	}
	_ = os.WriteFile(from, []byte("different"), 0o644)
	if err := moveFile(from, to); err == nil {
		t.Fatal("overwrote a different file")
	}
}

// M1: a File's path is relative to the Library folder (the importer's
// convention), so the scan finds an imported file.
func TestScanFindsImportedFile(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	from := addFolder(t, s)
	_, q, _ := runQueue(t, s, &metadata.Static{})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	dir := filepath.Join(from.Path, "Dune - Part Two (2024)")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "dune.mkv"), []byte("x"), 0o644)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: "Dune - Part Two (2024)/dune.mkv", Size: 1, AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	jid, _, err := q.Enqueue(ctx, KindScanTitle, &domain.Subject{Type: "title", ID: id}, nil, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var j domain.Job
		_ = database.Get(ctx, s.Read(), "jobs", jid, &j)
		if j.State == "done" {
			v := j.Result["versions"].([]any)[0].(map[string]any)
			if v["known_present"] != float64(1) || v["known_absent"] != float64(0) {
				t.Fatalf("scan %+v", v)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
}
