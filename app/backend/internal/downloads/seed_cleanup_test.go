package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// seedEngine is a torrent client stand-in with a fixed list of jobs.
type seedEngine struct {
	jobs    []Job
	all     bool // offers ListAll
	removed []string
	listed  int
}

func (s *seedEngine) Health(context.Context) (Health, error)          { return Health{}, nil }
func (s *seedEngine) Add(context.Context, AddRequest) (string, error) { return "", nil }
func (s *seedEngine) List(_ context.Context, cat string) ([]Job, error) {
	s.listed++
	var out []Job
	for _, j := range s.jobs {
		if cat == "" || j.Category == cat {
			out = append(out, j)
		}
	}
	return out, nil
}
func (s *seedEngine) Get(_ context.Context, id string) (Job, error) {
	for _, j := range s.jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return Job{}, ErrNotFound
}
func (s *seedEngine) Remove(_ context.Context, id string, files bool) error {
	s.removed = append(s.removed, id+"|"+map[bool]string{true: "files", false: "keep"}[files])
	return nil
}

// seedEngineAll also lists every torrent in the client.
type seedEngineAll struct{ *seedEngine }

func (s seedEngineAll) ListAll(ctx context.Context) ([]Job, error) { return s.List(ctx, "") }

// seededDownload is a grabbed torrent whose import is done, on a Downloader
// set to remove after seeding with the given goal.
func seededDownload(t *testing.T, adapter, goal string, j Job) (*world, *Manager, int64, *seedEngine) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	for _, q := range []string{
		fmt.Sprintf(`UPDATE downloaders SET adapter = '%s', protocol = 'torrent', writable = 1, after_import = 'remove', seed_goal = '%s'`, adapter, goal),
		`UPDATE downloads SET state = 'finished', progress = 1`,
		fmt.Sprintf(`INSERT INTO imports (download_id, state, plan, from_path, kind, mode, created_at, created_by, updated_at, updated_by)
			VALUES (%d, 'done', '[]', '/data/downloads/complete/x', 'movie', 'auto', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`, id),
	} {
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
	}
	j.ID, j.Category = hash, "holyarrs"
	if j.State == "" {
		j.State = StateFinished
	}
	if j.Progress == 0 {
		j.Progress = 1
	}
	if j.ContentPath == "" {
		j.ContentPath = "/data/downloads/complete/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	}
	eng := &seedEngine{jobs: []Job{j}}
	m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id, eng
}

func TestSeedCleanupRemovesWhenClientStopped(t *testing.T) {
	w, m, id, eng := seededDownload(t, "qbittorrent", `{"delete_files":true}`, Job{Detail: "stoppedUP", Ratio: 0.3})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|files" {
		t.Fatalf("removed %v", eng.removed)
	}
	d := w.download(t, id)
	if d.State != "cleaned" || d.StateReason == nil || !strings.Contains(*d.StateReason, "seeding") {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	if countEvents(t, w, "download_removed", id) != 1 {
		t.Fatal("no History entry")
	}
	// Once cleaned it is never read again.
	eng.removed = nil
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed twice: %v", eng.removed)
	}
}

func TestSeedCleanupKeepsFilesUnlessAsked(t *testing.T) {
	_, m, _, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "pausedUP"})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
		t.Fatalf("removed %v", eng.removed)
	}
}

func TestSeedCleanupOffByDefault(t *testing.T) {
	w, m, _, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "stoppedUP"})
	if err := execSQL(w, `UPDATE downloaders SET after_import = 'leave'`); err != nil {
		t.Fatal(err)
	}
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 || eng.listed != 0 {
		t.Fatalf("removed %v, listed %d", eng.removed, eng.listed)
	}
}

func TestSeedCleanupLeavesStillSeeding(t *testing.T) {
	_, m, _, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading", Ratio: 5})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed %v", eng.removed)
	}
}

func TestSeedCleanupWaitsForDoneImport(t *testing.T) {
	for _, q := range []string{
		`UPDATE imports SET state = 'waiting_for_you'`,
		`UPDATE imports SET state = 'failed'`,
		`DELETE FROM imports`,
		`UPDATE slots SET state = 'needs_a_decision'`,
	} {
		w, m, _, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "stoppedUP"})
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 0 {
			t.Fatalf("%s: removed %v", q, eng.removed)
		}
	}
}

func TestSeedCleanupReadOnlyDownloader(t *testing.T) {
	w, m, _, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "stoppedUP"})
	if err := execSQL(w, `UPDATE downloaders SET writable = 0`); err != nil {
		t.Fatal(err)
	}
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed %v", eng.removed)
	}
}

func TestSeedCleanupRatioGoal(t *testing.T) {
	for _, adapter := range []string{"qbittorrent", "rtorrent"} {
		_, m, _, eng := seededDownload(t, adapter, `{"ratio":0.05}`, Job{Detail: "uploading", Ratio: 0.06})
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 1 {
			t.Fatalf("%s: removed %v", adapter, eng.removed)
		}
		// Unknown ratio (0) never meets the goal.
		_, m, _, eng = seededDownload(t, adapter, `{"ratio":0.05}`, Job{Detail: "uploading"})
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 0 {
			t.Fatalf("%s unknown ratio: removed %v", adapter, eng.removed)
		}
	}
}

func TestSeedCleanupTimeGoal(t *testing.T) {
	_, m, _, eng := seededDownload(t, "rtorrent", `{"minutes":60}`, Job{Detail: "seeding", SeedingTime: 3600})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 1 {
		t.Fatalf("removed %v", eng.removed)
	}
	_, m, _, eng = seededDownload(t, "rtorrent", `{"minutes":60}`, Job{Detail: "seeding", SeedingTime: 3599})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed early: %v", eng.removed)
	}
}

func TestSeedCleanupGoalOnlyForQbitAndRTorrent(t *testing.T) {
	_, m, _, eng := seededDownload(t, "deluge", `{"ratio":0.01}`, Job{Detail: "Seeding", Ratio: 2})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed %v", eng.removed)
	}
}

func TestSeedCleanupLibraryInsideDownloadFolderKeepsFiles(t *testing.T) {
	// The test Library folder is /data/media/movies; content at /data/media holds it.
	_, m, _, eng := seededDownload(t, "qbittorrent", `{"delete_files":true}`, Job{Detail: "stoppedUP", ContentPath: "/data/media"})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
		t.Fatalf("removed %v", eng.removed)
	}
	_, m, _, eng = seededDownload(t, "qbittorrent", `{"delete_files":true}`, Job{Detail: "stoppedUP", ContentPath: "/data/media/movies/Dune Part Two (2024)"})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
		t.Fatalf("content in the Library: removed %v", eng.removed)
	}
}

func TestSeedCleanupSharedContentKeepsFiles(t *testing.T) {
	for _, withAll := range []bool{false, true} {
		w, m, id, eng := seededDownload(t, "qbittorrent", `{"delete_files":true}`, Job{Detail: "stoppedUP"})
		twin := eng.jobs[0]
		twin.ID, twin.Category = strings.Repeat("b", 40), "cross-seed"
		eng.jobs = append(eng.jobs, twin)
		if withAll {
			m.Engine = func(domain.Downloader) (Engine, error) { return seedEngineAll{eng}, nil }
			m.Forget(w.download(t, id).DownloaderID)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
			t.Fatalf("all=%v removed %v", withAll, eng.removed)
		}
	}
}

func TestClientStopped(t *testing.T) {
	cases := []struct {
		adapter string
		j       Job
		want    bool
	}{
		{"qbittorrent", Job{State: StateFinished, Progress: 1, Detail: "pausedUP"}, true},
		{"qbittorrent", Job{State: StateFinished, Progress: 1, Detail: "stoppedUP"}, true},
		{"qbittorrent", Job{State: StateFinished, Progress: 1, Detail: "stalledUP"}, false},
		{"qbittorrent", Job{State: StatePaused, Progress: 1, Detail: "error"}, false},
		{"qbittorrent", Job{State: StatePaused, Progress: 1, Detail: "missingFiles"}, false},
		{"transmission", Job{State: StateFinished, Progress: 1, Detail: "0"}, true},
		{"transmission", Job{State: StateFinished, Progress: 1, Detail: "0; error 2: tracker gone"}, false},
		{"transmission", Job{State: StateFinished, Progress: 1, Detail: "6"}, false},
		{"deluge", Job{State: StateFinished, Progress: 1, Detail: "Paused"}, true},
		{"deluge", Job{State: StatePaused, Progress: 1, Detail: "Error"}, false},
		{"deluge", Job{State: StateFinished, Progress: 1, Detail: "Seeding"}, false},
		{"rtorrent", Job{State: StateFinished, Progress: 1, Detail: "complete"}, true},
		{"rtorrent", Job{State: StateFinished, Progress: 1, Detail: "seeding"}, false},
		{"rtorrent", Job{State: StatePaused, Progress: 1, Detail: "error: storage"}, false},
		{"aria2", Job{State: StateFinished, Progress: 1, Detail: "complete"}, true},
		{"aria2", Job{State: StateFinished, Progress: 1, Detail: "seeding"}, false},
		{"aria2", Job{State: StatePaused, Progress: 1, Detail: "error: aria2 error 1"}, false},
		{"qbittorrent", Job{State: StateFinished, Progress: 0.5, Detail: "pausedUP"}, false},
	}
	for _, c := range cases {
		if got := clientStopped(c.adapter, c.j); got != c.want {
			t.Errorf("%s %+v: %v, want %v", c.adapter, c.j, got, c.want)
		}
	}
}

func TestErroredTorrentNeverSeeded(t *testing.T) {
	_, m, _, eng := seededDownload(t, "qbittorrent", `{"ratio":0.01}`, Job{State: StatePaused, Detail: "error", Ratio: 3})
	m.cleanSeeded(context.Background())
	if len(eng.removed) != 0 {
		t.Fatalf("removed %v", eng.removed)
	}
}

func TestSeedingChoiceChecks(t *testing.T) {
	r := 0.05
	mins := 60
	bad := -1.0
	cases := []struct {
		adapter string
		in      SeedingChoice
		err     error
	}{
		{"qbittorrent", SeedingChoice{Remove: true, Ratio: &r, Minutes: &mins, DeleteFiles: true}, nil},
		{"rtorrent", SeedingChoice{Remove: true, Ratio: &r}, nil},
		{"transmission", SeedingChoice{Remove: true}, nil},
		{"transmission", SeedingChoice{Remove: true, Ratio: &r}, ErrSeedGoal},
		{"sabnzbd", SeedingChoice{Remove: true}, ErrSeedTorrent},
		{"qbittorrent", SeedingChoice{Remove: true, Ratio: &bad}, ErrSeedGoal},
	}
	for _, c := range cases {
		var d domain.Downloader
		d.Adapter = c.adapter
		d.Protocol = map[bool]string{true: "usenet", false: "torrent"}[c.adapter == "sabnzbd"]
		if err := applySeeding(c.in, &d); !errors.Is(err, c.err) {
			t.Errorf("%s %+v: %v, want %v", c.adapter, c.in, err, c.err)
		}
	}
	var d domain.Downloader
	d.Adapter, d.Protocol = "qbittorrent", "torrent"
	if err := applySeeding(SeedingChoice{Remove: true, Ratio: &r, DeleteFiles: true}, &d); err != nil || d.AfterImport != "remove" || !d.SeedGoal.DeleteFiles || *d.SeedGoal.Ratio != r {
		t.Fatalf("%v %+v", err, d)
	}
	if err := applySeeding(SeedingChoice{}, &d); err != nil || d.AfterImport != "leave" || d.SeedGoal.DeleteFiles || d.SeedGoal.Ratio != nil {
		t.Fatalf("off: %v %+v", err, d)
	}
}

// Transmission lists only HolyArrs's labels, yet a twin under another label
// shares the files, so they stay.
func TestSeedCleanupTransmissionTwinKeepsFiles(t *testing.T) {
	w, m, id, _ := seededDownload(t, "transmission", `{"delete_files":true}`, Job{})
	f := &fakeTransmission{token: "<owned-session>", modern: true, items: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	twin := strings.Repeat("b", 40)
	for h, label := range map[string]string{hash: "holyarrs", twin: "cross-seed"} {
		f.items[h] = map[string]any{"hash_string": h, "name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "labels": []string{label},
			"download_dir": "/data/downloads/complete", "status": 0, "percent_done": 1, "left_until_done": 0, "metadata_percent_complete": 1, "total_size": 100}
	}
	tr := NewTransmission(srv.URL, "task", "<transmission-test-password>", nil)
	tr.category = "holyarrs"
	m.Engine = func(domain.Downloader) (Engine, error) { return tr, nil }
	m.Forget(w.download(t, id).DownloaderID)
	m.cleanSeeded(context.Background())
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.items[hash] != nil || f.items[twin] == nil || f.deleteFiles {
		t.Fatalf("removed %v, twin kept %v, files deleted %v", f.items[hash] == nil, f.items[twin] != nil, f.deleteFiles)
	}
}
