package downloads

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// jobEngine answers Get with one finished job that lists no files, as a Usenet Downloader does.
type jobEngine struct {
	Engine
	job Job
}

func (e *jobEngine) Get(context.Context, string) (Job, error) { return e.job, nil }

// unseenJob is a finished Usenet movie download whose job folder HolyArrs
// could not see: no file list, and a card that names the path pair. The
// importer runs for real on every import the Manager asks for.
func unseenJob(t *testing.T) (*world, *Manager, *jobEngine, int64, string) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	root, lib := t.TempDir(), t.TempDir()
	missing := filepath.Join(root, "not-mounted", "complete", "job")
	w.exec(t, "UPDATE library_folders SET path = ?", lib)
	w.exec(t, "UPDATE downloaders SET protocol = 'usenet'")
	w.exec(t, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', 'import_size_floors', 'false', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')
		ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1, files_seen = '[]', save_path = ? WHERE id = ?", filepath.Dir(missing), id)
	w.exec(t, "UPDATE slots SET state = 'downloading', active_download_id = ? WHERE id = ?", id, w.slotID)
	e := &jobEngine{job: Job{ID: hash, State: StateFinished, SavePath: filepath.Dir(missing), ContentPath: missing}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	m.mu.Lock()
	m.engines = nil
	m.mu.Unlock()
	m.SavePath = root
	im := &importer.Importer{Store: w.s, SavePath: root}
	m.OnFinished = func(ctx context.Context, id int64) {
		if _, err := im.Import(ctx, id, false, domain.System); err != nil {
			t.Errorf("import: %v", err)
		}
	}
	m.OnFinished(context.Background(), id)
	wantUnseenCard(t, w)
	return w, m, e, id, root
}

func wantUnseenCard(t *testing.T, w *world) {
	t.Helper()
	sl := w.slot(t)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || !strings.Contains(*sl.LastReason, "where the Downloader says it is") {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
}

// Import again on a card whose job folder is still not there says the same true line: no loop, no second download.
func TestImportAgainOnAnUnseenJobThatIsStillUnseen(t *testing.T) {
	w, m, _, id, _ := unseenJob(t)
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	wantUnseenCard(t, w)
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d downloads", n)
	}
	if d := w.download(t, id); d.State != "finished" || len(d.FilesSeen) != 0 {
		t.Fatalf("download %s %v", d.State, d.FilesSeen)
	}
}

// Import again after the path pair or the mount is fixed reads the job folder, stores its files and places the movie.
func TestImportAgainOnAnUnseenJobThatIsThereNow(t *testing.T) {
	w, m, e, id, root := unseenJob(t)
	job := filepath.Join(root, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	e.job.SavePath, e.job.ContentPath = root, job
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); len(d.FilesSeen) != 1 || d.SavePath != root {
		t.Fatalf("download files %v in %q", d.FilesSeen, d.SavePath)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d downloads", n)
	}
}

// Import again wakes only a card whose reason is one the person can fix outside HolyArrs; any other card stays as it is.
func TestImportAgainLeavesOtherCardsAlone(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	for _, reason := range []string{"no other Release passed the Pick rules", "no file in the download matched this movie; match the files by hand", "edition is unknown; confirm the edition or choose an unconstrained Version"} {
		w.exec(t, "UPDATE slots SET state = 'needs_a_decision', last_reason = ? WHERE id = ?", reason, w.slotID)
		if err := m.ImportAgain(context.Background(), id, luis); !errors.Is(err, ErrNotWaiting) {
			t.Fatalf("%q: %v", reason, err)
		}
		if sl := w.slot(t); sl.State != mediafiles.SlotNeedsADecision || len(w.finshed) != 0 {
			t.Fatalf("%q: slot %s, imports asked %v", reason, sl.State, w.finshed)
		}
	}
	for _, reason := range []string{
		"HolyArrs may not read the downloaded file; give the user HolyArrs runs as (PUID and PGID) read access to the file and the folder it is in, then import again",
		"HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again",
		"the file could not be placed: something went wrong; see the log",
		"HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and q has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again",
	} {
		w.exec(t, "UPDATE slots SET state = 'needs_a_decision', last_reason = ? WHERE id = ?", reason, w.slotID)
		if err := m.ImportAgain(context.Background(), id, luis); err != nil {
			t.Fatalf("%q: %v", reason, err)
		}
		if sl := w.slot(t); sl.State != mediafiles.SlotImporting {
			t.Fatalf("%q: slot %s", reason, sl.State)
		}
	}
}

// stopEngine ends the request while the Downloader is asked, as a page reload does.
type stopEngine struct {
	Engine
	stop func()
	job  Job
}

func (e *stopEngine) Get(ctx context.Context, _ string) (Job, error) {
	e.stop()
	return e.job, nil
}

func useEngine(m *Manager, e Engine) {
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	m.mu.Lock()
	m.engines = nil
	m.mu.Unlock()
}

func importJobs(t *testing.T, w *world) int {
	t.Helper()
	var n int
	if err := w.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND state IN ('queued', 'running')", importer.KindImport).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func needsDecisionEvents(t *testing.T, w *world) int {
	t.Helper()
	var n int
	if err := w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = ?", mediafiles.EventNeedsDecision).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A request that ends while the Downloader is asked still queues the import of the Slot it woke.
func TestImportAgainSurvivesARequestThatEnds(t *testing.T) {
	w, m, _, id, _ := unseenJob(t)
	q := jobs.New(w.s, jobs.Options{})
	m.QueueImport = func(ctx context.Context, id int64) error {
		_, _, err := q.Enqueue(ctx, importer.KindImport, &domain.Subject{Type: "download", ID: id}, nil, "import:1", domain.System)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	useEngine(m, &stopEngine{stop: cancel})
	if err := m.ImportAgain(ctx, id, luis); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotImporting || importJobs(t, w) != 1 {
		t.Fatalf("slot %s with %d import jobs", sl.State, importJobs(t, w))
	}
}

// An import that cannot be queued puts the card back as it was, with no new alert.
func TestImportAgainPutsTheCardBackWhenNothingIsQueued(t *testing.T) {
	w, m, _, id, _ := unseenJob(t)
	before := needsDecisionEvents(t, w)
	reason := *w.slot(t).LastReason
	m.QueueImport = func(context.Context, int64) error { return errors.New("the queue is closed") }
	if err := m.ImportAgain(context.Background(), id, luis); err == nil {
		t.Fatal("no error")
	}
	sl := w.slot(t)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != reason {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	if n := needsDecisionEvents(t, w); n != before {
		t.Fatalf("%d needs_decision events, was %d", n, before)
	}
}

// Import again on a job the Downloader has not finished reads no half-written folder.
func TestImportAgainDoesNotReadAJobThatIsNotFinished(t *testing.T) {
	w, m, e, id, root := unseenJob(t)
	job := filepath.Join(root, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	e.job = Job{ID: hash, State: StateDownloading, Progress: 0.3, SavePath: root, ContentPath: job}
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); len(d.FilesSeen) != 0 {
		t.Fatalf("files %v", d.FilesSeen)
	}
	wantUnseenCard(t, w)
}

// Three tries on a folder that is still not there leave the one alert the card raised.
func TestImportAgainOnTheSameReasonRaisesNoNewAlert(t *testing.T) {
	w, m, _, id, _ := unseenJob(t)
	if n := needsDecisionEvents(t, w); n != 1 {
		t.Fatalf("%d needs_decision events before", n)
	}
	for range 3 {
		if err := m.ImportAgain(context.Background(), id, luis); err != nil {
			t.Fatal(err)
		}
		wantUnseenCard(t, w)
	}
	if n := needsDecisionEvents(t, w); n != 1 {
		t.Fatalf("%d needs_decision events after three tries", n)
	}
}

// The time limit of the read covers the walk through the job folder: a walk cut short lists nothing to store.
func TestReadFinishedStopsWhenItsTimeIsUp(t *testing.T) {
	root := t.TempDir()
	job := filepath.Join(root, "job")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "a.mkv"), make([]byte, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, files := readFinishedIn(context.Background(), Job{ContentPath: job}, nil, root); len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, files := readFinishedIn(ctx, Job{ContentPath: job}, nil, root); len(files) != 0 {
		t.Fatalf("files after the time was up: %v", files)
	}
}
