package downloads

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type fakeSource struct {
	mu       sync.Mutex
	on       bool
	view     SyncView
	views    int
	fallback map[int64]bool
	kicks    []int64
	changed  chan struct{}
}

type callCounter struct {
	Engine
	mu                            sync.Mutex
	lists, gets, removes, healths int
}

func (f *fakeSource) On() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.on }

func (f *fakeSource) View(int64) SyncView {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.views++
	v := f.view
	items := make(map[string]Job, len(v.Items))
	for k, j := range v.Items {
		items[k] = j
	}
	v.Items = items
	return v
}

func (f *fakeSource) Fallback(id int64, on bool) { f.mu.Lock(); f.fallback[id] = on; f.mu.Unlock() }

func (f *fakeSource) Kick(id int64) { f.mu.Lock(); f.kicks = append(f.kicks, id); f.mu.Unlock() }

func (f *fakeSource) Changed() <-chan struct{} { return f.changed }

func (c *callCounter) List(ctx context.Context, cat string) ([]Job, error) {
	c.mu.Lock()
	c.lists++
	c.mu.Unlock()
	return c.Engine.List(ctx, cat)
}

func (c *callCounter) Get(ctx context.Context, id string) (Job, error) {
	c.mu.Lock()
	c.gets++
	c.mu.Unlock()
	return c.Engine.Get(ctx, id)
}

func (c *callCounter) Remove(ctx context.Context, id string, files bool) error {
	c.mu.Lock()
	c.removes++
	c.mu.Unlock()
	return c.Engine.Remove(ctx, id, files)
}

func (c *callCounter) Health(ctx context.Context) (Health, error) {
	c.mu.Lock()
	c.healths++
	c.mu.Unlock()
	return c.Engine.Health(ctx)
}

func TestStoreUnknownBeforeNewerSnapshot(t *testing.T) {
	w, m, src, cc, id := storeWorld(t)
	ctx := context.Background()
	before := time.Now().Add(-time.Hour)
	src.set(SyncView{Items: map[string]Job{}, Complete: true, Fresh: true, ReadAt: before})
	w.advance(10 * time.Minute)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := w.download(t, id).State; got != "sent" {
		t.Fatalf("state %q from a snapshot older than the grab, want sent (unknown)", got)
	}
	for i := range missedSyncs {
		w.advance(missWindow)
		src.set(SyncView{Items: map[string]Job{}, Complete: true, Fresh: true, ReadAt: time.Now().Add(time.Duration(i+1) * time.Second)})
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.download(t, id).State; got != "vanished" {
		t.Fatalf("state %q after a newer complete snapshot past grace, want vanished", got)
	}
	if lists, _, _, _ := cc.counts(); lists != 0 {
		t.Fatalf("Manager listed the client %d times; the reader owns list reads", lists)
	}
}

func TestStoreStallsWithUnchangedSnapshot(t *testing.T) {
	w, m, src, cc, id := storeWorld(t)
	ctx := context.Background()
	w.advance(time.Second)
	job := Job{ID: hash, Name: "Dune", Category: "holyarrs", Progress: 0.3, State: StateDownloading}
	src.set(SyncView{Items: map[string]Job{hash: job}, Complete: true, Fresh: true, ReadAt: w.now()})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := w.download(t, id).State; got != "downloading" {
		t.Fatalf("state %q, want downloading", got)
	}
	w.advance(11 * time.Minute)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := w.download(t, id).State; got != "stalled" {
		t.Fatalf("state %q after the stall window with no change, want stalled", got)
	}
	if lists, _, _, _ := cc.counts(); lists != 0 {
		t.Fatalf("Manager listed the client %d times", lists)
	}
}

func TestStoreDegradedFallsBack(t *testing.T) {
	w, m, src, cc, id := storeWorld(t)
	ctx := context.Background()
	src.set(SyncView{Fresh: false, Degraded: false})
	w.advance(10 * time.Minute)
	if n, err := m.SyncOnce(ctx); err != nil || n != 1 {
		t.Fatalf("active %d err %v, want 1 nil", n, err)
	}
	if got := w.download(t, id).State; got != "sent" {
		t.Fatalf("state %q while unknown, want sent", got)
	}
	if lists, _, _, _ := cc.counts(); lists != 0 {
		t.Fatalf("listed %d times while the snapshot is within its bound", lists)
	}
	src.set(SyncView{Degraded: true})
	w.fake.set(hash, map[string]any{"name": "Dune", "category": "holyarrs", "progress": 0.5, "state": "downloading"})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if lists, _, _, _ := cc.counts(); lists != 1 {
		t.Fatalf("fallback listed %d times, want 1", lists)
	}
	if !src.fallback[1] {
		t.Fatal("fallback flag not set for the degraded client")
	}
	if got := w.download(t, id).State; got != "downloading" {
		t.Fatalf("state %q from the fallback list, want downloading", got)
	}
	src.set(SyncView{Items: map[string]Job{hash: {ID: hash, Category: "holyarrs", Progress: 0.6, State: StateDownloading}}, Complete: true, Fresh: true, ReadAt: w.now().Add(time.Second)})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if src.fallback[1] {
		t.Fatal("fallback flag not cleared once the snapshot is fresh again")
	}
}

func TestStoreRetriesRemovalsWithoutRevChange(t *testing.T) {
	w, m, src, cc, id := storeWorld(t)
	ctx := context.Background()
	src.set(SyncView{Items: map[string]Job{}, Complete: true, Fresh: true, ReadAt: w.now()})
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET state = 'failed', removal_pending = 1 WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, removes, _ := cc.counts(); removes != 1 {
		t.Fatalf("removal retried %d times, want 1", removes)
	}
}

func TestRunWakesOnChangeAndKeepsHealthClock(t *testing.T) {
	_, m, src, cc, _ := storeWorld(t)
	m.Active, m.Idle, m.HealthEvery = time.Hour, time.Hour, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	waitFor(t, func() bool { return src.viewCount() >= 1 })
	n := src.viewCount()
	src.changed <- struct{}{}
	waitFor(t, func() bool { return src.viewCount() > n })
	cancel()
	<-done

	m.HealthEvery = 40 * time.Millisecond
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	_, _, _, h0 := cc.counts()
	go func() { m.Run(ctx); close(done) }()
	waitFor(t, func() bool { _, _, _, h := cc.counts(); return h >= h0+3 })
	cancel()
	<-done
}

func TestSnapshotJobsCategory(t *testing.T) {
	v := SyncView{Items: map[string]Job{"a": {ID: "a", Category: "holyarrs"}, "b": {ID: "b", Category: "other"}}}
	if got := snapshotJobs(v, []string{"holyarrs"}); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("filtered %v", got)
	}
	v.Scope = "holyarrs"
	if got := snapshotJobs(v, []string{"holyarrs"}); len(got) != 2 {
		t.Fatalf("a read already in the category is not filtered again: %v", got)
	}
}

func TestGrabKicks(t *testing.T) {
	w, m := setup(t)
	f := newFakeSource()
	m.Sync = f
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(f.kicks) != 1 || f.kicks[0] != 1 {
		t.Fatalf("kicks %v, want [1]", f.kicks)
	}
}

func newFakeSource() *fakeSource {
	return &fakeSource{on: true, fallback: map[int64]bool{}, changed: make(chan struct{}, 1)}
}

func (f *fakeSource) set(v SyncView) { f.mu.Lock(); f.view = v; f.mu.Unlock() }

func (f *fakeSource) viewCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.views }

func (c *callCounter) counts() (int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lists, c.gets, c.removes, c.healths
}

func storeWorld(t *testing.T) (*world, *Manager, *fakeSource, *callCounter, int64) {
	t.Helper()
	w, m := setup(t)
	cc := &callCounter{}
	m.Engine = func(d domain.Downloader) (Engine, error) {
		e, err := (&Manager{}).EngineFor(context.Background(), d)
		cc.Engine = e
		return cc, err
	}
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	m.Sync = src
	return w, m, src, cc, id
}

func (w *world) advance(d time.Duration) {
	w.mu.Lock()
	w.clock = w.clock.Add(d)
	w.mu.Unlock()
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
