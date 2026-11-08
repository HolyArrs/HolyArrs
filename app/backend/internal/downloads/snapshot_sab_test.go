package downloads

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fakeSnapshotSab is a SABnzbd as the reader sees it: a queue, a finished list and its change counter.
type fakeSnapshotSab struct {
	mu      sync.Mutex
	queue   []Job
	hist    []Job
	counter int64
	sent    []int64 // counters History was called with
	histErr error
}

func (f *fakeSnapshotSab) Health(context.Context) (Health, error) { return Health{}, nil }
func (f *fakeSnapshotSab) Add(context.Context, AddRequest) (string, error) {
	return "", errors.New("no")
}
func (f *fakeSnapshotSab) List(context.Context, string) ([]Job, error) {
	return nil, errors.New("the reader must not use List for SABnzbd")
}
func (f *fakeSnapshotSab) Get(context.Context, string) (Job, error) {
	return Job{}, ErrNotFound
}
func (f *fakeSnapshotSab) Remove(context.Context, string, bool) error { return nil }
func (f *fakeSnapshotSab) Queue(context.Context) (QueuePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return QueuePage{Head: QueueHead{Speed: 1}, Jobs: append([]Job(nil), f.queue...)}, nil
}
func (f *fakeSnapshotSab) History(_ context.Context, since int64) (HistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, since)
	if f.histErr != nil {
		return HistoryPage{}, f.histErr
	}
	if since > 0 && since == f.counter {
		return HistoryPage{Last: since}, nil
	}
	return HistoryPage{Last: f.counter, Changed: true, Jobs: append([]Job(nil), f.hist...)}, nil
}

// sabJob makes a job with the given id, category and state.
func sabJob(id, cat, state string) Job {
	p := 0.5
	if state == StateFinished || state == StateFailed {
		p = 1
	}
	return Job{ID: id, Name: "Show.S01E01." + id, Category: cat, State: state, Progress: p}
}

// The finished list must be read only when its counter moves, a lower counter means a restart, and an error must drop the counter.
func TestSabCursors_FastPathResetAndErrors(t *testing.T) {
	f := &fakeSnapshotSab{queue: []Job{sabJob("q1", "tv", StateDownloading)},
		hist: []Job{sabJob("h1", "tv", StateFinished), sabJob("q1", "tv", StateFinished)}, counter: 5}
	var c SabCursors
	ctx := context.Background()
	p, err := c.Poll(ctx, 1, 0, f)
	if err != nil || !p.HistoryRead || len(p.Jobs) != 2 {
		t.Fatalf("first: %v %+v", err, p)
	}
	if p.Jobs[0].State != StateDownloading {
		t.Fatal("the queue row must win over the finished row with the same id")
	}
	// Unchanged counter: the finished list is kept, not read.
	p, err = c.Poll(ctx, 1, 0, f)
	if err != nil || p.HistoryRead || len(p.Jobs) != 2 || f.sent[1] != 5 {
		t.Fatalf("unchanged: %v %+v sent %v", err, p, f.sent)
	}
	// Lower counter: SABnzbd restarted, so a full read from 0 follows.
	f.counter, f.hist = 2, []Job{sabJob("h2", "tv", StateFailed)}
	p, err = c.Poll(ctx, 1, 0, f)
	if err != nil || !p.Reset || !p.HistoryRead || len(p.Jobs) != 2 {
		t.Fatalf("reset: %v %+v", err, p)
	}
	if n := len(f.sent); f.sent[n-2] != 5 || f.sent[n-1] != 0 {
		t.Fatalf("reset must read again from 0: %v", f.sent)
	}
	// An error drops the counter, so the next poll reads everything.
	f.histErr = errors.New("down")
	if _, err = c.Poll(ctx, 1, 0, f); err == nil {
		t.Fatal("error lost")
	}
	f.histErr = nil
	if _, err = c.Poll(ctx, 1, 0, f); err != nil || f.sent[len(f.sent)-1] != 0 {
		t.Fatalf("after error: %v %v", err, f.sent)
	}
	// A new engine generation starts from 0 too.
	if _, err = c.Poll(ctx, 1, 1, f); err != nil || f.sent[len(f.sent)-1] != 0 {
		t.Fatalf("new generation: %v %v", err, f.sent)
	}
}

// sabRig builds a reader over a fake SABnzbd.
func sabRig(t *testing.T, f *fakeSnapshotSab, outside string) (*Sync, domain.Downloader) {
	t.Helper()
	db, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	d := &domain.Downloader{Name: "SAB", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab", Category: "holyarrs", Priority: 1,
		PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
	err = db.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		if outside == "" {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, '', '{}', '', '{}')`, SettingGroup, SettingSABOutside, outside)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: db, Active: time.Second, Idle: time.Second, HealthEvery: time.Hour,
		Engine: func(domain.Downloader) (Engine, error) { return f, nil }}
	s := &Sync{Store: &Store{}, DB: db, Manager: m, Lines: testLines}
	s.init()
	return s, *d
}

// Every SABnzbd job must show by default, queue and finished list in any category. With the setting off only the Downloader's category does.
func TestSync_SabnzbdListsEveryJob(t *testing.T) {
	f := &fakeSnapshotSab{counter: 3,
		queue: []Job{sabJob("q1", "holyarrs", StateDownloading), sabJob("q2", "tv", StatePaused)},
		hist:  []Job{sabJob("h1", "movies", StateFinished), sabJob("h2", "holyarrs", StateFailed)}}
	for _, c := range []struct {
		setting string
		want    int
	}{{"", 4}, {"true", 4}, {"false", 2}} {
		s, d := sabRig(t, f, c.setting)
		if err := s.read(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		if got := len(s.Store.Current(d.ID).Items); got != c.want {
			t.Fatalf("setting %q: %d items, want %d", c.setting, got, c.want)
		}
		before := len(f.sent)
		if err := s.read(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		if f.sent[before] != 3 {
			t.Fatalf("second read must send the counter: %v", f.sent)
		}
		if got := len(s.Store.Current(d.ID).Items); got != c.want {
			t.Fatalf("setting %q, unchanged list: %d items", c.setting, got)
		}
	}
}

// The first poll must get the long budget until a full finished list was read, then the short one.
func TestSabCursors_NeedsFull(t *testing.T) {
	f := &fakeSnapshotSab{hist: []Job{sabJob("h1", "tv", StateFinished)}, counter: 5}
	var c SabCursors
	if !c.NeedsFull(1) {
		t.Fatal("new downloader should need a full read")
	}
	if _, err := c.Poll(context.Background(), 1, 0, f); err != nil {
		t.Fatal(err)
	}
	if c.NeedsFull(1) {
		t.Fatal("after a full read the short budget applies")
	}
	if readBudget(domain.Downloader{ID: 1, Adapter: "sabnzbd"}, &c) != callTimeout || readBudget(domain.Downloader{ID: 2, Adapter: "sabnzbd"}, &c) != sabFullTimeout {
		t.Fatal("budget")
	}
}
