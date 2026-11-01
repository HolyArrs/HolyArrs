package dlsync

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// fakeSab is a SABnzbd as the reader sees it: a queue, a finished list and
// its change counter.
type fakeSab struct {
	mu      sync.Mutex
	queue   []downloader.Job
	hist    []downloader.Job
	counter int64
	sent    []int64 // counters History was called with
	histErr error
}

func (f *fakeSab) Health(context.Context) (downloader.Health, error) { return downloader.Health{}, nil }
func (f *fakeSab) Add(context.Context, downloader.AddRequest) (string, error) {
	return "", errors.New("no")
}
func (f *fakeSab) List(context.Context, string) ([]downloader.Job, error) {
	return nil, errors.New("the reader must not use List for SABnzbd")
}
func (f *fakeSab) Get(context.Context, string) (downloader.Job, error) {
	return downloader.Job{}, downloader.ErrNotFound
}
func (f *fakeSab) Remove(context.Context, string, bool) error { return nil }
func (f *fakeSab) Queue(context.Context) (downloader.QueuePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return downloader.QueuePage{Head: downloader.QueueHead{Speed: 1}, Jobs: append([]downloader.Job(nil), f.queue...)}, nil
}
func (f *fakeSab) History(_ context.Context, since int64) (downloader.HistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, since)
	if f.histErr != nil {
		return downloader.HistoryPage{}, f.histErr
	}
	if since > 0 && since == f.counter {
		return downloader.HistoryPage{Last: since}, nil
	}
	return downloader.HistoryPage{Last: f.counter, Changed: true, Jobs: append([]downloader.Job(nil), f.hist...)}, nil
}

func sabJob(id, cat, state string) downloader.Job {
	p := 0.5
	if state == downloader.StateFinished || state == downloader.StateFailed {
		p = 1
	}
	return downloader.Job{ID: id, Name: "Show.S01E01." + id, Category: cat, State: state, Progress: p}
}

func TestSabCursorsFastPathResetAndErrors(t *testing.T) {
	f := &fakeSab{queue: []downloader.Job{sabJob("q1", "tv", downloader.StateDownloading)},
		hist: []downloader.Job{sabJob("h1", "tv", downloader.StateFinished), sabJob("q1", "tv", downloader.StateFinished)}, counter: 5}
	var c SabCursors
	ctx := context.Background()
	p, err := c.Poll(ctx, 1, 0, f)
	if err != nil || !p.HistoryRead || len(p.Jobs) != 2 {
		t.Fatalf("first: %v %+v", err, p)
	}
	if p.Jobs[0].State != downloader.StateDownloading {
		t.Fatal("the queue row must win over the finished row with the same id")
	}
	// Unchanged: the finished list is kept, not read.
	p, err = c.Poll(ctx, 1, 0, f)
	if err != nil || p.HistoryRead || len(p.Jobs) != 2 || f.sent[1] != 5 {
		t.Fatalf("unchanged: %v %+v sent %v", err, p, f.sent)
	}
	// Restarted: the counter went down; a full read from 0 follows.
	f.counter, f.hist = 2, []downloader.Job{sabJob("h2", "tv", downloader.StateFailed)}
	p, err = c.Poll(ctx, 1, 0, f)
	if err != nil || !p.Reset || !p.HistoryRead || len(p.Jobs) != 2 {
		t.Fatalf("reset: %v %+v", err, p)
	}
	if n := len(f.sent); f.sent[n-2] != 5 || f.sent[n-1] != 0 {
		t.Fatalf("reset must read again from 0: %v", f.sent)
	}
	// An error drops the counter: the next poll reads everything.
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

func sabRig(t *testing.T, f *fakeSab, outside string) (*Sync, domain.Downloader) {
	t.Helper()
	db, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	d := &domain.Downloader{Name: "SAB", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab", Category: "holyarrs", Priority: 1,
		PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
	err = db.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
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
	m := &downloader.Manager{Store: db, Active: time.Second, Idle: time.Second, HealthEvery: time.Hour,
		Engine: func(domain.Downloader) (downloader.Engine, error) { return f, nil }}
	s := &Sync{Store: &Store{}, DB: db, Manager: m, Lines: testLines}
	s.init()
	return s, *d
}

// Every SABnzbd job shows by default (queue and finished list, any
// category); with the setting off only the Downloader's category does.
func TestSabReaderListsEveryJob(t *testing.T) {
	f := &fakeSab{counter: 3,
		queue: []downloader.Job{sabJob("q1", "holyarrs", downloader.StateDownloading), sabJob("q2", "tv", downloader.StatePaused)},
		hist:  []downloader.Job{sabJob("h1", "movies", downloader.StateFinished), sabJob("h2", "holyarrs", downloader.StateFailed)}}
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

// Until a full finished list has been read the poll gets the long budget;
// after that the short one.
func TestSabCursorsNeedsFull(t *testing.T) {
	f := &fakeSab{hist: []downloader.Job{sabJob("h1", "tv", downloader.StateFinished)}, counter: 5}
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
