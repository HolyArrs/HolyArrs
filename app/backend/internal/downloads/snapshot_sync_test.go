package downloads

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fakeClient is an in-memory Downloader engine that counts every call.
type fakeClient struct {
	mu      sync.Mutex
	jobs    map[string]Job
	lists   int
	gets    int
	panicN  int // how many of the next List calls panic
	listErr error
}

func newFakeClient(js ...Job) *fakeClient {
	f := &fakeClient{jobs: map[string]Job{}}
	for _, j := range js {
		f.jobs[j.ID] = j
	}
	return f
}

func (f *fakeClient) Health(context.Context) (Health, error) {
	return Health{Version: "1"}, nil
}
func (f *fakeClient) Add(context.Context, AddRequest) (string, error) {
	return "", errors.New("no adds in this test")
}
func (f *fakeClient) List(_ context.Context, category string) ([]Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	if f.panicN > 0 {
		f.panicN--
		panic("fake client fell over")
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []Job
	for _, j := range f.jobs {
		if category == "" || j.Category == category {
			out = append(out, j)
		}
	}
	return out, nil
}
func (f *fakeClient) Get(_ context.Context, id string) (Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if j, ok := f.jobs[id]; ok {
		return j, nil
	}
	return Job{}, ErrNotFound
}
func (f *fakeClient) Remove(context.Context, string, bool) error { return nil }

func (f *fakeClient) set(j Job)      { f.mu.Lock(); f.jobs[j.ID] = j; f.mu.Unlock() }
func (f *fakeClient) drop(id string) { f.mu.Lock(); delete(f.jobs, id); f.mu.Unlock() }
func (f *fakeClient) listCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.lists }

// rig is a reader wired to a fake client, a Manager and a test database.
type rig struct {
	db     *database.Store
	client *fakeClient
	m      *Manager
	s      *Sync
	dlID   int64
}

// ownHash is the hash of the HolyArrs Download in the rig.
const ownHash = "1111111111111111111111111111111111111111"

// newRig makes a store with one Downloader of a list-only adapter, one active HolyArrs Download on it, a Manager reading a fake client, and a reader.
func newRig(t *testing.T, js ...Job) *rig {
	t.Helper()
	db, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := &rig{db: db, client: newFakeClient(js...)}
	err = db.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "Lab", Adapter: "transmission", Protocol: "torrent", URL: "http://lab", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		r.dlID = d.ID
		dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{}, ExternalID: ownHash, State: "sent", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r.m = &Manager{Store: db, Active: 50 * time.Millisecond, Idle: 100 * time.Millisecond, HealthEvery: time.Hour,
		Engine: func(domain.Downloader) (Engine, error) { return r.client, nil }}
	r.s = &Sync{Store: &Store{}, DB: db, Manager: r.m, Lines: testLines}
	r.m.Sync = r.s
	return r
}

// testLines is a LinesFunc that logs state changes.
func testLines(prev, cur Job) []LogLine {
	if prev.Progress < 1 && cur.Progress >= 1 {
		return []LogLine{{Kind: "completed", Summary: "Completed"}}
	}
	return nil
}

// fastTimers shortens the reader timers for the test.
func fastTimers(t *testing.T) {
	t.Helper()
	oldScan, oldBack := scanEvery, restartBackoff
	scanEvery, restartBackoff = 20*time.Millisecond, []time.Duration{100 * time.Millisecond}
	t.Cleanup(func() { scanEvery, restartBackoff = oldScan, oldBack })
}

// waitUntil polls ok until it is true or the wait runs out.
func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (r *rig) state(t *testing.T) (string, float64) {
	t.Helper()
	var st string
	var p float64
	if err := r.db.Read().QueryRow("SELECT state, progress FROM downloads WHERE external_id = ?", ownHash).Scan(&st, &p); err != nil {
		t.Fatal(err)
	}
	return st, p
}

// own returns the HolyArrs Download at the given progress.
func own(progress float64) Job {
	return Job{ID: ownHash, Name: "Own", Category: "holyarrs", Progress: progress, State: StateDownloading}
}

// outsider returns a job HolyArrs did not send.
func outsider(id string, progress float64) Job {
	return Job{ID: id, Name: "Outside " + id, Category: "holyarrs", Progress: progress, State: StateDownloading}
}

// With the reader on, it must make every list read, one per cadence tick, and the Manager none, while the Manager still applies the client's changes.
func TestSync_ReaderOwnsListReads(t *testing.T) {
	fastTimers(t)
	r := newRig(t, own(0.2))
	// Use a 200 ms cadence, so the freshness bound is 600 ms. At 50 ms a slow
	// machine (race detector, loaded CI) sometimes misses the 150 ms bound and
	// the Manager rightly falls back, which this test is not about.
	r.m.Active = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	start := time.Now()
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "the Manager to apply the snapshot", func() bool { st, _ := r.state(t); return st == "downloading" })
	r.client.set(own(0.7))
	waitUntil(t, "progress to reach the row", func() bool { _, p := r.state(t); return p == 0.7 })
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done
	elapsed := time.Since(start)
	if r.s.Store.Current(r.dlID).Fallback {
		t.Fatal("the Manager fell back to its own list reads; the reader owns list reads")
	}
	// One read per tick at most, plus a little slack.
	if lists, most := r.client.listCount(), int(elapsed/r.m.Active)+3; lists > most || lists == 0 {
		t.Fatalf("%d list reads in %v, want 1..%d", lists, elapsed, most)
	}
}

// On shutdown the pollers must stay up while the Manager is still busy after cancel, then stop, and the final flush must commit.
func TestSync_ShutdownOrderAndFinalFlush(t *testing.T) {
	fastTimers(t)
	scanEvery = time.Hour // so only the final flush writes
	r := newRig(t, own(0.2), outsider("aaaa", 0.1))
	ctx, cancel := context.WithCancel(context.Background())
	var liveAfterCancel atomic.Bool
	run := func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(500 * time.Millisecond)
		liveAfterCancel.Store(r.s.Store.Current(r.dlID).Running)
	}
	done := make(chan struct{})
	go func() { r.s.Around(ctx, run); close(done) }()
	waitUntil(t, "a first read", func() bool { return r.s.Store.Current(r.dlID).Complete })
	start := time.Now()
	cancel()
	<-done
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("shutdown took %v", took)
	}
	if !liveAfterCancel.Load() {
		t.Fatal("the poller stopped before the Manager returned")
	}
	if r.s.Store.Current(r.dlID).Running {
		t.Fatal("the poller still runs after Around returned")
	}
	var n int
	if err := r.db.Read().QueryRow("SELECT COUNT(*) FROM dl_items WHERE downloader_id = ?", r.dlID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("dl_items rows %d (%v), want 2 committed by the final flush", n, err)
	}
}

// A panic in the Manager's Run must stop that run's pollers before the supervisor starts Run again, so there is one poller per client.
func TestSync_RunPanicLeavesOnePoller(t *testing.T) {
	fastTimers(t)
	r := newRig(t, own(0.2))
	func() {
		defer func() { _ = recover() }()
		r.s.Around(context.Background(), func(context.Context) {
			waitUntil(t, "a first read", func() bool { return r.s.Store.Current(r.dlID).Complete })
			panic("manager fell over")
		})
	}()
	n := r.client.listCount()
	time.Sleep(200 * time.Millisecond)
	if r.client.listCount() != n {
		t.Fatal("a poller kept reading after the panicked run")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.s.Around(ctx, func(ctx context.Context) { <-ctx.Done() }); close(done) }()
	waitUntil(t, "reads again", func() bool { return r.client.listCount() > n })
	r.s.mu.Lock()
	pollers := len(r.s.pollers)
	r.s.mu.Unlock()
	cancel()
	<-done
	if pollers != 1 {
		t.Fatalf("%d pollers after the restart, want 1", pollers)
	}
}

// After a poller panic the process must live on, the Manager must fall back to its own guarded list for that client, and the poller must restart and make the client fresh again.
func TestSync_PollerPanicDegradesAndRecovers(t *testing.T) {
	fastTimers(t)
	r := newRig(t, own(0.2))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "a fresh read", func() bool { return r.s.Store.Current(r.dlID).Fresh })
	r.client.mu.Lock()
	r.client.panicN = 1
	r.client.mu.Unlock()
	waitUntil(t, "the Manager's fallback list", func() bool { return r.s.Store.Current(r.dlID).Fallback })
	waitUntil(t, "fresh again after the restart", func() bool {
		c := r.s.Store.Current(r.dlID)
		return c.Fresh && c.Running
	})
	waitUntil(t, "the fallback to clear", func() bool { return !r.s.Store.Current(r.dlID).Fallback })
	cancel()
	<-done
}

// Safe mode never runs the loop, so the reader must make no call, Kick must do nothing, and the Manager must keep its own reads.
func TestSync_NotStartedMakesNoCalls(t *testing.T) {
	r := newRig(t, own(0.2))
	r.s.Kick(r.dlID)
	time.Sleep(100 * time.Millisecond)
	if r.s.On() {
		t.Fatal("On without a running reader")
	}
	if n := r.client.listCount(); n != 0 {
		t.Fatalf("%d calls with the reader not started", n)
	}
	if _, err := r.m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := r.client.listCount(); n != 1 {
		t.Fatalf("the Manager did not keep its own read: %d client lists", n)
	}
}

// logLines returns the stored log summaries of one item.
func logLines(t *testing.T, db *database.Store, hash string) []string {
	t.Helper()
	rows, err := db.Read().Query("SELECT summary FROM torrent_log WHERE hash = ? ORDER BY id", hash)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

// With the part switched off and on again there must be no calls while off, and the next start must carry the log on from dl_items with one line for a change made while off.
func TestSync_PartToggleResumesFromItems(t *testing.T) {
	fastTimers(t)
	r := newRig(t, own(0.2), outsider("bbbb", 0.5))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "a first read", func() bool { return r.s.Store.Current(r.dlID).Complete })
	cancel()
	<-done
	n := r.client.listCount()
	r.client.set(outsider("bbbb", 1))
	time.Sleep(150 * time.Millisecond)
	if r.client.listCount() != n {
		t.Fatal("reads while the part is off")
	}
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "the completion line", func() bool { return len(logLines(t, r.db, "bbbb")) > 0 })
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done
	got := logLines(t, r.db, "bbbb")
	if len(got) != 1 || got[0] != "Completed (while HolyArrs was off)" {
		t.Fatalf("log %q, want one line marked while HolyArrs was off", got)
	}
}

// A process restart, a new reader on the same database, must continue the log.
func TestSync_RestartKeepsLogContinuity(t *testing.T) {
	fastTimers(t)
	r := newRig(t, own(0.2), outsider("cccc", 0.5), outsider("dddd", 0.5))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "a first read", func() bool { return r.s.Store.Current(r.dlID).Complete })
	cancel()
	<-done
	r.client.set(outsider("cccc", 1))
	r.client.drop("dddd")
	s2 := &Sync{Store: &Store{}, DB: r.db, Manager: r.m, Lines: testLines}
	r.m.Sync = s2
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { s2.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "both lines", func() bool { return len(logLines(t, r.db, "cccc")) > 0 && len(logLines(t, r.db, "dddd")) > 0 })
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
	if got := logLines(t, r.db, "cccc"); len(got) != 1 || got[0] != "Completed (while HolyArrs was off)" {
		t.Fatalf("cccc log %q", got)
	}
	if got := logLines(t, r.db, "dddd"); len(got) != 1 || got[0] != "Removed outside HolyArrs (while HolyArrs was off)" {
		t.Fatalf("dddd log %q", got)
	}
	var gone sql.NullString
	if err := r.db.Read().QueryRow("SELECT gone_at FROM dl_items WHERE key = 'dddd'").Scan(&gone); err != nil || !gone.Valid {
		t.Fatalf("dddd gone_at %v (%v)", gone, err)
	}
}

// The match queue must back off after each failure and mark nomatch after six. A restart must not try before next_try_at.
func TestMatchQueue_BackoffAndRestart(t *testing.T) {
	r := newRig(t)
	clock := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	var calls int
	fail := func(context.Context, int64, string, string, string) (string, int64, error) {
		calls++
		return "", 0, errors.New("metadata server down")
	}
	r.s.Now, r.s.Match = now, fail
	r.s.init()
	at := clock.Format(database.TimeLayout)
	if err := r.db.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO dl_match_queue (downloader_id, key, name, next_try_at) VALUES (?, 'k1', 'Some.Movie.2020', ?)", r.dlID, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	waits := []time.Duration{time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}
	for i, w := range waits {
		if n := r.s.matchStep(context.Background()); n != 1 {
			t.Fatalf("try %d ran %d lookups", i+1, n)
		}
		clock = clock.Add(w - time.Second)
		if n := r.s.matchStep(context.Background()); n != 0 {
			t.Fatalf("tried again before next_try_at after failure %d", i+1)
		}
		// A new reader after a restart must not retry any earlier.
		s2 := &Sync{Store: &Store{}, DB: r.db, Manager: r.m, Now: now, Match: fail}
		s2.init()
		if n := s2.matchStep(context.Background()); n != 0 {
			t.Fatalf("a restart tried before next_try_at after failure %d", i+1)
		}
		clock = clock.Add(time.Second)
	}
	if n := r.s.matchStep(context.Background()); n != 1 {
		t.Fatal("sixth try did not run")
	}
	var outcome string
	_ = r.db.Read().QueryRow("SELECT outcome FROM dl_match_queue WHERE key = 'k1'").Scan(&outcome)
	if outcome != "nomatch" {
		t.Fatalf("outcome %q after six failures, want nomatch", outcome)
	}
	clock = clock.Add(30 * 24 * time.Hour)
	if n := r.s.matchStep(context.Background()); n != 0 {
		t.Fatal("nomatch tried again without a new name")
	}
	if calls != 6 {
		t.Fatalf("%d lookups, want 6", calls)
	}
}

// The bucket must allow a burst of 10 and then 2 per second.
func TestBucket_BurstThenRate(t *testing.T) {
	b := bucket{rate: matchRate, burst: matchBurst}
	t0 := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	n := 0
	for b.take(t0) {
		n++
	}
	if n != 10 {
		t.Fatalf("burst %d, want 10", n)
	}
	got := 0
	for i := 1; i <= 100; i++ { // 10 s in steps of 100 ms
		if b.take(t0.Add(time.Duration(i) * 100 * time.Millisecond)) {
			got++
		}
	}
	if got < 19 || got > 21 {
		t.Fatalf("%d lookups in 10 s after the burst, want about 20", got)
	}
}

// Cadence must follow the table of adapter, active count, viewer and outside items.
func TestCadence_Table(t *testing.T) {
	a, i := 2*time.Second, 30*time.Second
	cases := []struct {
		adapter           string
		n                 int
		viewer, outActive bool
		want              time.Duration
	}{
		{"qbittorrent", 1, true, false, 1500 * time.Millisecond},
		{"sabnzbd", 1, true, false, 2 * time.Second},
		{"transmission", 1, true, false, 2 * time.Second},
		{"qbittorrent", 1, false, false, 2 * time.Second},
		{"qbittorrent", 0, true, true, 1500 * time.Millisecond},
		{"sabnzbd", 0, true, true, 3 * time.Second},
		{"qbittorrent", 0, true, false, 5 * time.Second},
		{"deluge", 0, true, false, 10 * time.Second},
		{"qbittorrent", 0, false, true, 30 * time.Second},
	}
	for _, c := range cases {
		if got := Cadence(c.adapter, c.n, c.viewer, c.outActive, a, i); got != c.want {
			t.Errorf("%s n=%d viewer=%v outside=%v: %v, want %v", c.adapter, c.n, c.viewer, c.outActive, got, c.want)
		}
	}
	// Custom intervals replace the defaults, and idle is never slower.
	if got := Cadence("qbittorrent", 0, true, false, time.Second, 4*time.Second); got != 4*time.Second {
		t.Errorf("viewer cadence slower than idle: %v", got)
	}
}

// The reader must never write the downloads table, so the Manager stays its only writer.
func TestSync_NeverWritesDownloads(t *testing.T) {
	write := regexp.MustCompile(`(?i)(UPDATE\s+downloads\b|INSERT\s+INTO\s+downloads\b|DELETE\s+FROM\s+downloads\b|store\.(Update|Insert|UpdateWhere)\([^)]*"downloads")`)
	files, _ := filepath.Glob("snapshot*.go") // the reader's files since the fold
	files = append(files, "routes_sync.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := write.FindIndex(b); loc != nil {
			t.Errorf("%s writes downloads: %q", f, b[loc[0]:loc[1]])
		}
	}
}

// The latency from client-finished to download-finished with the reader must be no worse than with the Manager reading the client itself: median plus 1 s and max plus 2 s, over 10 runs each.
func TestSync_FinishLatencyVsManagerReads(t *testing.T) {
	fastTimers(t)
	measure := func(reader bool) []time.Duration {
		var out []time.Duration
		for range 10 {
			r := newRig(t, own(0.5))
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			if reader {
				go func() { r.s.Around(ctx, r.m.Run); close(done) }()
			} else {
				r.m.Sync = nil
				go func() { r.m.Run(ctx); close(done) }()
			}
			waitUntil(t, "downloading", func() bool { st, _ := r.state(t); return st == "downloading" })
			j := own(1)
			j.State = StateFinished
			start := time.Now()
			r.client.set(j)
			waitUntil(t, "finished", func() bool { st, _ := r.state(t); return st == "finished" })
			out = append(out, time.Since(start))
			cancel()
			<-done
		}
		sort.Slice(out, func(i, k int) bool { return out[i] < out[k] })
		return out
	}
	self, on := measure(false), measure(true)
	t.Logf("own reads median %v max %v; reader median %v max %v", self[5], self[9], on[5], on[9])
	if on[5] > self[5]+time.Second || on[9] > self[9]+2*time.Second {
		t.Fatalf("the reader is slower than own reads: median %v vs %v, max %v vs %v", on[5], self[5], on[9], self[9])
	}
}
