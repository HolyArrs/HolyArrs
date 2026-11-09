package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

type fix struct {
	s            *database.Store
	q            *jobs.Queue
	c            *parts.Control
	reg          *jobs.Registry
	slotID, dlID int64
	runs         sync.Map // kind -> *atomic.Int32
	gates        map[string]chan struct{}
	started      chan string
}

// newFix makes a store with one movie (a Slot and its Download), a queue
// whose import and grab Handlers wait for the test, and a Control as the
// queue's gate.
func newFix(t *testing.T) *fix {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &fix{s: s, reg: jobs.NewRegistry(), started: make(chan string, 20),
		gates: map[string]chan struct{}{"import_download": make(chan struct{}), "grab": make(chan struct{})}}
	f.reg.Logf = func(string, ...any) {}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		y := 2020
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Year: &y, Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		ver := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ver.ID, State: mediafiles.SlotDownloading}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:9", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: dr.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcd", State: "finished", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		f.slotID, f.dlID = sl.ID, dl.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.q = jobs.New(s, jobs.Options{Workers: 3, PollInterval: 5 * time.Millisecond})
	for _, kind := range []string{"import_download", "grab", "search_slot", "refresh_shows"} {
		f.q.Handle(kind, func(ctx context.Context, r *jobs.Run) error {
			n, _ := f.runs.LoadOrStore(kind, &atomic.Int32{})
			n.(*atomic.Int32).Add(1)
			f.started <- kind
			if g := f.gates[kind]; g != nil {
				// The import moves the Slot through importing to in the
				// Library, as the importer's state machine does.
				if kind == "import_download" {
					if err := mediafiles.MoveSlot(context.Background(), s, f.slotID, mediafiles.SlotImporting, "", domain.System); err != nil {
						return err
					}
				}
				select {
				case <-g:
				case <-ctx.Done(): // the test ended early: let the queue stop
					return ctx.Err()
				}
				if kind == "import_download" {
					return mediafiles.MoveSlot(context.Background(), s, f.slotID, mediafiles.SlotInLibrary, "", domain.System)
				}
			}
			return nil
		})
	}
	f.c = &parts.Control{Store: s, Reg: f.reg, Jobs: f.q, Drain: 5 * time.Second, Logf: func(string, ...any) {}}
	if err := f.c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.q.SetGate(f.c)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); f.q.Stop() })
	f.c.Start(ctx)
	if err := f.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fix) count(kind string) int {
	n, ok := f.runs.Load(kind)
	if !ok {
		return 0
	}
	return int(n.(*atomic.Int32).Load())
}

func (f *fix) enqueue(t *testing.T, kind string, subj *domain.Subject, key string) int64 {
	t.Helper()
	id, _, err := f.q.Enqueue(context.Background(), kind, subj, nil, key, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fix) waitStarted(t *testing.T, kind string) {
	t.Helper()
	select {
	case k := <-f.started:
		if k != kind {
			t.Fatalf("started %s, want %s", k, kind)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not start", kind)
	}
}

func (f *fix) jobState(t *testing.T, id int64) string {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM jobs WHERE id = ?", id).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *fix) waitJob(t *testing.T, id int64, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for f.jobState(t, id) != want {
		if time.Now().After(deadline) {
			t.Fatalf("job %d is %s, want %s", id, f.jobState(t, id), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fix) slotState(t *testing.T) mediafiles.SlotState {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.slotID).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return mediafiles.SlotState(st)
}

var owner = domain.Actor{Kind: "user", ID: 1}

// Turning Movies off while its import runs: the import finishes once and
// the Slot ends in the Library (never half-imported); a grab asked for
// while Movies is off waits in the queue (not lost, not run) while Shows
// work goes on; turning Movies on runs the grab once.
func TestOffAndOnDuringAnImport(t *testing.T) {
	f := newFix(t)
	imp := f.enqueue(t, "import_download", &domain.Subject{Type: "download", ID: f.dlID}, "import:1")
	f.waitStarted(t, "import_download")
	if err := f.c.Set(context.Background(), parts.Movies, false, owner); err != nil {
		t.Fatal(err)
	}
	if f.slotState(t) != mediafiles.SlotImporting {
		t.Fatalf("slot %s while the import runs", f.slotState(t))
	}
	grab := f.enqueue(t, "grab", &domain.Subject{Type: "slot", ID: f.slotID}, "grab:1")
	show := f.enqueue(t, "refresh_shows", nil, "")
	f.waitStarted(t, "refresh_shows")
	f.waitJob(t, show, "done")
	if dup := f.enqueue(t, "grab", &domain.Subject{Type: "slot", ID: f.slotID}, "grab:1"); dup != grab {
		t.Fatalf("a second grab for the Slot was queued: %d and %d", grab, dup)
	}
	close(f.gates["import_download"])
	f.waitJob(t, imp, "done")
	if f.slotState(t) != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s after the import", f.slotState(t))
	}
	time.Sleep(100 * time.Millisecond)
	if st := f.jobState(t, grab); st != "queued" || f.count("grab") != 0 {
		t.Fatalf("grab is %s (ran %d) while Movies is off", st, f.count("grab"))
	}
	if err := f.c.Set(context.Background(), parts.Movies, true, owner); err != nil {
		t.Fatal(err)
	}
	f.waitStarted(t, "grab")
	close(f.gates["grab"])
	f.waitJob(t, grab, "done")
	time.Sleep(50 * time.Millisecond)
	if f.count("import_download") != 1 || f.count("grab") != 1 {
		t.Fatalf("runs: import %d, grab %d; want one each", f.count("import_download"), f.count("grab"))
	}
}

// Restarting Movies while a download is being sent (grab) and an import
// runs: the restart waits for both to finish, nothing new of Movies starts
// in between, and after it every Job ran exactly once.
func TestRestartDuringADownloadAndAnImport(t *testing.T) {
	f := newFix(t)
	grab := f.enqueue(t, "grab", &domain.Subject{Type: "slot", ID: f.slotID}, "grab:1")
	f.waitStarted(t, "grab")
	imp := f.enqueue(t, "import_download", &domain.Subject{Type: "download", ID: f.dlID}, "import:1")
	f.waitStarted(t, "import_download")
	done := make(chan int, 1)
	go func() {
		left, err := f.c.Restart(context.Background(), parts.Movies)
		if err != nil {
			t.Error(err)
		}
		done <- left
	}()
	deadline := time.Now().Add(5 * time.Second)
	for f.c.On(parts.Movies) {
		if time.Now().After(deadline) {
			t.Fatal("restart did not hold Movies")
		}
		time.Sleep(time.Millisecond)
	}
	if st := f.c.Statuses()[0]; st.State != jobs.Starting {
		t.Fatalf("Movies while restarting: %+v", st)
	}
	search := f.enqueue(t, "search_slot", &domain.Subject{Type: "slot", ID: f.slotID}, "search_slot:1")
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("restart returned while its Jobs still ran")
	default:
	}
	if f.jobState(t, search) != "queued" {
		t.Fatal("a Movies Job started during the restart")
	}
	close(f.gates["grab"])
	close(f.gates["import_download"])
	if left := <-done; left != 0 {
		t.Fatalf("%d Jobs left running", left)
	}
	f.waitJob(t, grab, "done")
	f.waitJob(t, imp, "done")
	f.waitJob(t, search, "done")
	if !f.c.On(parts.Movies) || f.slotState(t) != mediafiles.SlotInLibrary {
		t.Fatalf("after restart: on %v, slot %s", f.c.On(parts.Movies), f.slotState(t))
	}
	if f.count("grab") != 1 || f.count("import_download") != 1 || f.count("search_slot") != 1 {
		t.Fatalf("runs: grab %d, import %d, search %d", f.count("grab"), f.count("import_download"), f.count("search_slot"))
	}
}

// A restart that waits past its cap leaves the running Job alone: it
// carries on and finishes once.
func TestRestartNeverCutsOffAJob(t *testing.T) {
	f := newFix(t)
	f.c.Drain = 50 * time.Millisecond
	imp := f.enqueue(t, "import_download", &domain.Subject{Type: "download", ID: f.dlID}, "import:1")
	f.waitStarted(t, "import_download")
	left, err := f.c.Restart(context.Background(), parts.Movies)
	if err != nil || left != 1 {
		t.Fatalf("restart: %d left, %v", left, err)
	}
	if f.jobState(t, imp) != "running" {
		t.Fatalf("import %s after the restart", f.jobState(t, imp))
	}
	close(f.gates["import_download"])
	f.waitJob(t, imp, "done")
	if f.count("import_download") != 1 || f.slotState(t) != mediafiles.SlotInLibrary {
		t.Fatalf("import ran %d times, slot %s", f.count("import_download"), f.slotState(t))
	}
}

// Subtitles need Movies or TV shows; Requests needs one of Movies, Shows
// and Music. A switch that breaks either is refused with the reason.
func TestDependencies(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	must := func(p parts.Part, on bool) {
		t.Helper()
		if err := f.c.Set(ctx, p, on, owner); err != nil {
			t.Fatalf("%s %v: %v", p, on, err)
		}
	}
	refused := func(p parts.Part, on bool, says string) {
		t.Helper()
		var ne parts.NeedsError
		if err := f.c.Set(ctx, p, on, owner); !errors.As(err, &ne) || !strings.Contains(err.Error(), says) {
			t.Fatalf("%s %v: %v, want refused with %q", p, on, err, says)
		}
	}
	must(parts.Movies, false)
	refused(parts.Shows, false, "Turn Subtitles off first")
	must(parts.Subtitles, false)
	must(parts.Shows, false)
	refused(parts.Subtitles, true, "Turn Movies or TV shows on first")
	refused(parts.Music, false, "Turn Requests off first")
	must(parts.Wishlist, false)
	must(parts.Music, false)
	refused(parts.Wishlist, true, "Turn one of them on first")
	must(parts.Music, true)
	must(parts.Wishlist, true)
	if f.c.On(parts.Subtitles) || !f.c.On(parts.Wishlist) || !f.c.On(parts.Music) {
		t.Fatalf("off now: %v", f.c.Off())
	}
	if err := f.c.Set(ctx, "books", false, owner); !errors.Is(err, parts.ErrNoPart) {
		t.Fatalf("unknown part: %v", err)
	}
}

// The switches are saved: a new Control (the next start) reads them.
func TestSwitchesAreSaved(t *testing.T) {
	f := newFix(t)
	if err := f.c.Set(context.Background(), parts.Music, false, owner); err != nil {
		t.Fatal(err)
	}
	next := &parts.Control{Store: f.s, Reg: jobs.NewRegistry()}
	if err := next.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if next.On(parts.Music) || !next.On(parts.Movies) || len(next.Off()) != 1 {
		t.Fatalf("after a restart: off %v", next.Off())
	}
	// Safe mode forces every part off, without changing what is saved.
	safe := &parts.Control{Store: f.s, Reg: jobs.NewRegistry(), SafeMode: true}
	if err := safe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(safe.Off()) != len(parts.All()) {
		t.Fatalf("safe mode: off %v", safe.Off())
	}
	for _, st := range safe.Statuses() {
		if st.State != jobs.Off || !strings.Contains(st.Reason, "Safe mode") {
			t.Fatalf("safe mode state: %+v", st)
		}
	}
	if err := safe.Set(context.Background(), parts.Movies, true, owner); !errors.Is(err, parts.ErrSafeMode) {
		t.Fatalf("switch in safe mode: %v", err)
	}
	if _, err := safe.Restart(context.Background(), parts.Movies); !errors.Is(err, parts.ErrSafeMode) {
		t.Fatalf("restart in safe mode: %v", err)
	}
}

// A part's loop stops while the part is off and starts when it is on; a
// paused core stage stays off until resumed; a failed loop shows on its
// part.
func TestPartLoopsAndStages(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.Logf = func(string, ...any) {}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := &parts.Control{Store: s, Reg: reg, Logf: func(string, ...any) {}}
	if err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var gates, backups atomic.Int32
	var fail atomic.Bool
	c.Add(jobs.Loop{Name: "gates", Label: "Release gates", Every: 5 * time.Millisecond, Run: func(context.Context) error {
		gates.Add(1)
		if fail.Load() {
			return errors.New("details source down")
		}
		return nil
	}}, true)
	c.Add(jobs.Loop{Name: "backups", Label: "Backups", Every: 5 * time.Millisecond, Run: func(context.Context) error { backups.Add(1); return nil }}, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); reg.Wait() })
	c.Start(ctx)
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out: %s", what)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	waitFor("gates run", func() bool { return gates.Load() > 2 })
	if err := c.Set(ctx, parts.Movies, false, owner); err != nil {
		t.Fatal(err)
	}
	n := gates.Load()
	time.Sleep(50 * time.Millisecond)
	if gates.Load() != n {
		t.Fatal("gates ran while Movies was off")
	}
	if st, _ := reg.Get(jobs.StageKey("gates")); st.State != jobs.Off || st.Reason != "Movies is off." {
		t.Fatalf("gates stage: %+v", st)
	}
	if err := c.Resume("gates"); !errors.As(err, new(parts.OffError)) {
		t.Fatalf("resume a stage of an off part: %v", err)
	}
	if err := c.Set(ctx, parts.Movies, true, owner); err != nil {
		t.Fatal(err)
	}
	waitFor("gates again", func() bool { return gates.Load() > n+2 })

	fail.Store(true)
	waitFor("movies retrying", func() bool { return c.Statuses()[0].State == jobs.Retrying })
	// Fixed plain words; the error itself stays in the log.
	if st := c.Statuses()[0]; strings.Contains(st.Reason, "details source down") || !strings.Contains(st.Reason, "Release gates: The last pass failed") {
		t.Fatalf("movies: %+v", st)
	}
	fail.Store(false)
	waitFor("movies running", func() bool { return c.Statuses()[0].State == jobs.Running })

	if err := c.Pause("backups", "Luis"); err != nil {
		t.Fatal(err)
	}
	b := backups.Load()
	time.Sleep(40 * time.Millisecond)
	if st, _ := reg.Get(jobs.StageKey("backups")); backups.Load() != b || st.State != jobs.Off || st.Reason != "Paused by Luis." {
		t.Fatalf("paused backups ran or shows %+v", st)
	}
	if err := c.Set(ctx, parts.Movies, false, owner); err != nil { // a part switch leaves a paused stage paused
		t.Fatal(err)
	}
	if err := c.Set(ctx, parts.Movies, true, owner); err != nil {
		t.Fatal(err)
	}
	if backups.Load() != b {
		t.Fatal("a part switch started a paused stage")
	}
	if err := c.Resume("backups"); err != nil {
		t.Fatal(err)
	}
	waitFor("backups again", func() bool { return backups.Load() > b+2 })
	if err := c.RestartStage("backups"); err != nil {
		t.Fatal(err)
	}
	if err := c.Pause("nope", "Luis"); !errors.Is(err, parts.ErrNoStage) {
		t.Fatalf("unknown stage: %v", err)
	}
}

// Scheduled Jobs of a part that is off are not queued; the schedule stays
// due and is queued once the part is on.
func TestSchedulesOfAnOffPartWait(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	if err := f.c.Set(ctx, parts.Shows, false, owner); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(48 * time.Hour)
	kinds, err := f.q.RunSchedules(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range kinds {
		if k == "refresh_shows" {
			t.Fatal("refresh_shows was queued while Shows is off")
		}
	}
	if err := f.c.Set(ctx, parts.Shows, true, owner); err != nil {
		t.Fatal(err)
	}
	kinds, err = f.q.RunSchedules(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(kinds, ",") != "refresh_shows" {
		t.Fatalf("queued after Shows came back: %v", kinds)
	}
}

func TestScreenOwner(t *testing.T) {
	for path, want := range map[string]parts.Owner{
		"/library/held": parts.Media, "/library/12": parts.ByTitle, "/library/12/albums/3": parts.Of(parts.Music), "/settings/music": parts.Of(parts.Music),
		"/requests": parts.Of(parts.Wishlist), "/": parts.Core, "/settings/sources/4": parts.Core,
	} {
		if got, ok := parts.ScreenOwner(path); !ok || got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	if _, ok := parts.ScreenOwner("/nowhere/at/all"); ok {
		t.Error("an unknown page has an owner")
	}
}

// Log masking is never paused or restarted on its own, is not offered as a
// controllable stage, and keeps running.
func TestLogMaskingIsNeverPaused(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.Logf = func(string, ...any) {}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := &parts.Control{Store: s, Reg: reg, Logf: func(string, ...any) {}}
	if err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var masks, backups atomic.Int32
	c.Add(jobs.Loop{Name: "log_secrets", Label: "Log masking", Every: 5 * time.Millisecond, Run: func(context.Context) error { masks.Add(1); return nil }}, false)
	c.Add(jobs.Loop{Name: "backups", Label: "Backups", Every: 5 * time.Millisecond, Run: func(context.Context) error { backups.Add(1); return nil }}, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); reg.Wait() })
	c.Start(ctx)
	if got := c.Stages(); len(got) != 1 || got[0] != "backups" {
		t.Fatalf("controllable stages = %v, want only backups", got)
	}
	if err := c.Pause("log_secrets", "Luis"); !errors.Is(err, parts.ErrNeverPause) {
		t.Fatalf("pause log masking: %v", err)
	}
	if err := c.RestartStage("log_secrets"); !errors.Is(err, parts.ErrNeverPause) {
		t.Fatalf("restart log masking: %v", err)
	}
	n := masks.Load()
	deadline := time.Now().Add(5 * time.Second)
	for masks.Load() < n+3 {
		if time.Now().After(deadline) {
			t.Fatal("log masking stopped running")
		}
		time.Sleep(2 * time.Millisecond)
	}
	// A restart of a pausable stage brings its work back.
	b := backups.Load()
	if err := c.RestartStage("backups"); err != nil {
		t.Fatal(err)
	}
	for backups.Load() < b+3 {
		if time.Now().After(deadline) {
			t.Fatal("backups did not run again after a restart")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if st, _ := reg.Get(jobs.StageKey("backups")); st.State != jobs.Running {
		t.Fatalf("backups after restart: %+v", st)
	}
}
