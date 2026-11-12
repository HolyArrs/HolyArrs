package downloads

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Reader timings.
const (
	callTimeout = 4 * time.Second // deadline of one read
	// sabFullTimeout is a SABnzbd read that pages the whole finished list, up
	// to 25 pages on a big history. Later reads use callTimeout.
	sabFullTimeout = 30 * time.Second
	kickDebounce   = 1 * time.Second   // a kick reads at most this soon after the last read
	widenCap       = 300 * time.Second // failures widen the cadence up to this
	flushTimeout   = 5 * time.Second   // FinalFlush's own deadline
)

// SettingSABOutside is the settings key for showing Usenet items HolyArrs
// did not send on the Downloads page.
const SettingSABOutside = "downloads_sab_outside"

// scanEvery is the coordinator's tick. Tests shorten it.
var scanEvery = 2 * time.Second

// restartBackoff is the wait before a poller that panicked starts again.
var restartBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, time.Minute, 300 * time.Second}

// Sync is the background reader. One poller per Downloader owns every list
// read through the Manager's guarded engine, and the Manager and the Downloads
// view read the Store it fills. It runs inside the "downloads" loop (see
// Around), so safe mode and the part switch govern it.
type Sync struct {
	Store   *Store
	DB      *database.Store
	Manager SnapshotManager
	Lines   LinesFunc
	Match   MatchFunc
	// Pub publishes patches to the live stream; nil turns that off.
	Pub *SnapshotPublisher
	Now func() time.Time
	// Sleep replaces the real wait in tests; nil waits for real.
	Sleep func(ctx context.Context, d time.Duration) error

	cursors Cursors
	sabs    SabCursors
	rts     rtCursors
	persist *persister
	bucket  bucket
	started atomic.Bool
	changed chan struct{}

	mu      sync.Mutex
	kicks   map[int64]chan struct{}
	pollers map[int64]context.CancelFunc
	onNow   bool
}

// SnapshotManager is what the reader needs from Manager.
type SnapshotManager interface {
	Reader(ctx context.Context, d domain.Downloader) (Engine, uint64, error)
	ReportRead(ctx context.Context, d domain.Downloader, err error)
	Generation(id int64) uint64
	Interval(active int) time.Duration
}

// On reports whether the reader runs. It implements SyncSource.
func (s *Sync) On() bool {
	return s != nil && s.started.Load()
}

// ClientStatus returns a qBittorrent's state from the reader's last read,
// when the reader is on and has one.
func (s *Sync) ClientStatus(id int64) (ClientStatus, bool) {
	if !s.On() {
		return ClientStatus{}, false
	}
	return s.cursors.Status(id)
}

// View returns one client's snapshot for the Manager. It implements
// SyncSource.
func (s *Sync) View(id int64) SyncView { return s.Store.View(id) }

// Fallback marks whether the Manager reads a client itself. It implements
// SyncSource.
func (s *Sync) Fallback(id int64, on bool) { s.Store.SetFallback(id, on) }

// Changed returns a channel that gets a signal after a read changed
// something. It implements SyncSource.
func (s *Sync) Changed() <-chan struct{} {
	s.init()
	return s.changed
}

// Kick asks for a read of one client soon, debounced. It does nothing when
// the reader is not running.
func (s *Sync) Kick(id int64) {
	if s == nil || !s.started.Load() {
		return
	}
	s.mu.Lock()
	k := s.kicks[id]
	s.mu.Unlock()
	if k != nil {
		select {
		case k <- struct{}{}:
		default:
		}
	}
}

// Watched records that someone views the Downloads page. When nobody did
// before, every client is read soon, so the faster viewer cadence starts now
// instead of after the idle wait.
func (s *Sync) Watched() {
	if s == nil || s.Store == nil {
		return
	}
	was := s.Store.Viewer()
	s.Store.Viewed()
	if was || !s.started.Load() {
		return
	}
	s.mu.Lock()
	ks := make([]chan struct{}, 0, len(s.kicks))
	for _, k := range s.kicks {
		ks = append(ks, k)
	}
	s.mu.Unlock()
	for _, k := range ks {
		select {
		case k <- struct{}{}:
		default:
		}
	}
}

// Around runs the "downloads" loop body run with the reader beside it. The
// pollers outlive run until it returns or panics, then they stop and are
// waited for, and pending writes are flushed with their own deadline.
func (s *Sync) Around(loopCtx context.Context, run func(context.Context)) {
	if s == nil {
		run(loopCtx)
		return
	}
	s.init()
	syncCtx, stop := context.WithCancel(context.WithoutCancel(loopCtx))
	var wg sync.WaitGroup
	s.started.Store(true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.coordinate(syncCtx, &wg)
	}()
	if s.Pub != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs.Guard("dlsync publisher", func() { s.Pub.Run(syncCtx) })
		}()
	}
	defer func() {
		stop()
		wg.Wait()
		s.started.Store(false)
		s.FinalFlush(loopCtx)
		// Let the next start load the saved rows again.
		s.mu.Lock()
		s.onNow = false
		s.mu.Unlock()
	}()
	run(loopCtx)
}

// FinalFlush writes everything pending with its own deadline, so a loop ctx
// that is already cancelled does not abort the commit.
func (s *Sync) FinalFlush(loopCtx context.Context) {
	if s == nil || s.persist == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(loopCtx), flushTimeout)
	defer cancel()
	if err := s.persist.flush(ctx); err != nil {
		log.Printf("dlsync: final flush: %v", err)
	}
}

// SABOutside reports whether Usenet items HolyArrs did not send are shown on
// the Downloads page. It is on unless the setting says false.
func (s *Sync) SABOutside(ctx context.Context) bool {
	if s == nil || s.DB == nil {
		return false
	}
	var v string
	_ = s.DB.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", SettingGroup, SettingSABOutside).Scan(&v)
	return v != "false"
}

// Cadence returns the read interval for one client: never slower than the
// Manager's own loop (active with HolyArrs Downloads, idle without), and
// faster while someone views the Downloads page.
func Cadence(adapter string, n int, viewer, outsideActive bool, active, idle time.Duration) time.Duration {
	pick := func(qbit, sab, other time.Duration) time.Duration {
		switch adapter {
		case "qbittorrent":
			return qbit
		case "sabnzbd", "nzbget":
			return sab
		}
		return other
	}
	switch {
	case n > 0 && viewer:
		return pick(min(1500*time.Millisecond, active), min(2*time.Second, active), active)
	case n > 0:
		return active
	case viewer && outsideActive:
		return min(pick(1500*time.Millisecond, 3*time.Second, 3*time.Second), idle)
	case viewer:
		return min(pick(5*time.Second, 10*time.Second, 10*time.Second), idle)
	}
	return idle
}

// now returns the clock, which tests can replace.
func (s *Sync) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// sleep waits for d or until ctx ends; tests can replace it.
func (s *Sync) sleep(ctx context.Context, d time.Duration) error {
	if s.Sleep != nil {
		return s.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// init makes the maps and channels on first use, so the zero value works.
func (s *Sync) init() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changed == nil {
		s.changed = make(chan struct{}, 1)
	}
	if s.kicks == nil {
		s.kicks = map[int64]chan struct{}{}
	}
	if s.pollers == nil {
		s.pollers = map[int64]context.CancelFunc{}
	}
	if s.persist == nil {
		s.persist = &persister{db: s.DB, lines: s.Lines, now: s.now}
	}
	if s.bucket.rate == 0 {
		s.bucket.rate, s.bucket.burst = matchRate, matchBurst
	}
}

// signal tells Changed listeners without blocking.
func (s *Sync) signal() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// coordinate keeps one poller per Downloader, flushes the persister and
// paces the match queue until ctx ends.
func (s *Sync) coordinate(ctx context.Context, wg *sync.WaitGroup) {
	var lastMatch time.Time
	for {
		s.enter(ctx)
		s.ensurePollers(ctx, wg)
		if s.persist.due() {
			if err := s.persist.flush(ctx); err != nil && ctx.Err() == nil {
				log.Printf("dlsync: flush: %v", err)
			}
		}
		if s.now().Sub(lastMatch) >= time.Second {
			lastMatch = s.now()
			s.matchStep(ctx)
		}
		if s.sleep(ctx, scanEvery) != nil {
			s.stopPollers()
			return
		}
	}
}

// enter runs once per start. It loads the last known rows as stale, so
// changes made while HolyArrs was off are diffed against them.
func (s *Sync) enter(ctx context.Context) {
	s.mu.Lock()
	if s.onNow {
		s.mu.Unlock()
		return
	}
	s.onNow = true
	s.mu.Unlock()
	rows, err := s.persist.load(ctx)
	if err != nil {
		log.Printf("dlsync: load last known rows: %v", err)
	}
	for id, jobs := range rows {
		s.Store.Load(id, jobs)
	}
}

// stopPollers cancels every poller.
func (s *Sync) stopPollers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.pollers {
		cancel()
		delete(s.pollers, id)
	}
}

// ensurePollers starts a poller for every Downloader that has none and stops
// the ones whose Downloader was removed.
func (s *Sync) ensurePollers(ctx context.Context, wg *sync.WaitGroup) {
	if s.DB == nil {
		return
	}
	all, err := database.Select[domain.Downloader](ctx, s.DB.Read(), "downloaders", "removed = 0")
	if err != nil {
		return
	}
	live := map[int64]bool{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range all {
		live[d.ID] = true
		if s.pollers[d.ID] != nil {
			continue
		}
		pctx, cancel := context.WithCancel(ctx)
		s.pollers[d.ID] = cancel
		k := make(chan struct{}, 1)
		s.kicks[d.ID] = k
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			s.supervise(pctx, id, k)
		}(d.ID)
	}
	for id, cancel := range s.pollers {
		if !live[id] {
			cancel()
			delete(s.pollers, id)
			delete(s.kicks, id)
			s.Store.Drop(id)
		}
	}
}

// supervise runs one client's poller and restarts it with backoff after a
// panic. A panic marks the client failed, so the Manager reads it itself.
func (s *Sync) supervise(ctx context.Context, id int64, kick chan struct{}) {
	for crashes := 0; ctx.Err() == nil; crashes++ {
		clean := false
		jobs.Guard("dlsync poller", func() {
			s.poll(ctx, id, kick)
			clean = true
		})
		if clean {
			return
		}
		s.Store.Running(id, false)
		s.Store.Fail(id, s.Manager.Generation(id))
		log.Printf("dlsync: reader for downloader %d stopped unexpectedly; the Manager reads it itself until it is back (sync degraded)", id)
		if s.sleep(ctx, restartBackoff[min(crashes, len(restartBackoff)-1)]) != nil {
			return
		}
	}
}

// poll reads one client on its cadence until ctx ends or the Downloader is
// gone.
func (s *Sync) poll(ctx context.Context, id int64, kick chan struct{}) {
	s.Store.Running(id, true)
	defer s.Store.Running(id, false)
	fails := 0
	var last time.Time
	wait := time.Duration(0)
	for {
		if wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-kick:
				t.Stop()
				if d := kickDebounce - s.now().Sub(last); d > 0 && s.sleep(ctx, d) != nil {
					return
				}
			case <-t.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		var d domain.Downloader
		if err := database.Get(ctx, s.DB.Read(), "downloaders", id, &d); err != nil || d.Removed {
			if err == nil || ctx.Err() != nil {
				return
			}
			// The lookup failed, so retry on the next scan. ensurePollers stops
			// this poller if the Downloader is really gone.
			wait = scanEvery
			continue
		}
		n := s.active(ctx, id)
		every := s.cadence(d, n)
		if s.Store.Current(id).Fallback {
			// The Manager reads this client itself, so ask for one recovery read
			// per its own interval.
			every = s.Manager.Interval(n)
		}
		s.Store.SetFloor(id, every)
		last = s.now()
		err := s.read(ctx, d)
		switch {
		case err == nil:
			fails = 0
			// Recompute from the new items, so a torrent that just started
			// moving speeds up the next wait at once.
			if !s.Store.Current(id).Fallback {
				every = s.cadence(d, n)
				s.Store.SetFloor(id, every)
			}
		case errors.Is(err, ErrAuth):
			// Stop until the settings change (a new engine generation).
			// Retrying a refused sign-in could get HolyArrs banned.
			gen := s.Manager.Generation(id)
			for s.Manager.Generation(id) == gen {
				if s.sleep(ctx, scanEvery) != nil {
					return
				}
			}
			wait = 0
			continue
		case ctx.Err() != nil:
			return
		default:
			fails++
		}
		wait = widen(every, fails, s.Store.Current(id).Shared)
	}
}

// widen grows the cadence one step per failure, and one more for a shared
// session, up to widenCap. A widened wait gets +/-20% jitter so clients do
// not retry together.
func widen(every time.Duration, fails int, shared bool) time.Duration {
	steps := fails
	if shared {
		steps++
	}
	if steps == 0 {
		return every
	}
	w := every << min(steps, 10)
	w = min(w, widenCap)
	j := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(w) * j)
}

// active counts the client's active HolyArrs Downloads.
func (s *Sync) active(ctx context.Context, id int64) int {
	var n int
	_ = s.DB.Read().QueryRowContext(ctx, "SELECT COUNT(*) FROM downloads WHERE downloader_id = ? AND state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')", id).Scan(&n)
	return n
}

// following lists the ids of the client's active HolyArrs Downloads.
func (s *Sync) following(ctx context.Context, id int64) []string {
	rows, err := s.DB.Read().QueryContext(ctx, "SELECT external_id FROM downloads WHERE downloader_id = ? AND state IN ("+activeStates+") AND external_id <> ''", id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var x string
		if rows.Scan(&x) == nil {
			out = append(out, x)
		}
	}
	return out
}

// cadence returns the client's read interval for its current load.
func (s *Sync) cadence(d domain.Downloader, n int) time.Duration {
	snap := s.Store.Current(d.ID)
	outside := false
	for _, j := range snap.Items {
		if j.State == StateDownloading {
			outside = true
			break
		}
	}
	return Cadence(d.Adapter, n, s.Store.Viewer(), outside, s.Manager.Interval(1), s.Manager.Interval(0))
}

// readBudget returns one read's deadline, longer for SABnzbd's first full
// read.
func readBudget(d domain.Downloader, sabs *SabCursors) time.Duration {
	if d.Adapter == "sabnzbd" && sabs.NeedsFull(d.ID) {
		return sabFullTimeout
	}
	return callTimeout
}

// read reads one client once through the guarded engine and records it.
func (s *Sync) read(ctx context.Context, d domain.Downloader) error {
	cctx, cancel := context.WithTimeout(ctx, readBudget(d, &s.sabs))
	defer cancel()
	e, gen, err := s.Manager.Reader(cctx, d)
	if err != nil {
		s.Store.Fail(d.ID, gen)
		return err
	}
	var jobs []Job
	scope := d.Category
	shared := false
	ms, isMain := e.(MainSyncer)
	switch {
	case d.Adapter == "qbittorrent" && isMain:
		var p MainPoll
		p, err = s.cursors.Poll(cctx, d.ID, gen, ms)
		jobs, scope, shared = p.Jobs, "", p.SharedSession
	case (d.Adapter == "sabnzbd" || d.Adapter == "nzbget") && isSab(e):
		// NZBGet answers the same two reads but has no change counter, so
		// its history is read in full each time.
		var p SabPoll
		p, err = s.sabs.Poll(cctx, d.ID, gen, e.(SabSource))
		jobs = p.Jobs
		if s.SABOutside(ctx) {
			scope = ""
		} else {
			jobs = inCategory(jobs, d.Categories())
		}
	case d.Adapter == "rtorrent":
		// Only what is still downloading, between whole reads on the sweep and
		// while the Downloads page is open.
		jobs, err = s.rts.poll(cctx, d.ID, gen, e, d.Categories(), s.following(ctx, d.ID), s.Store.Viewer(), s.now())
	case d.Protocol == "usenet" && s.SABOutside(ctx):
		jobs, err = e.List(cctx, "")
		scope = ""
	default:
		jobs, err = listCategories(cctx, e, d.Categories())
	}
	if ctx.Err() != nil && err != nil {
		return err // stopping, so the client is not to blame
	}
	s.Manager.ReportRead(ctx, d, err)
	if err != nil {
		s.Store.Fail(d.ID, gen)
		return err
	}
	s.Store.SetShared(d.ID, shared)
	// A good read ends any fallback, even when no HolyArrs download is
	// left to clear it.
	s.Store.SetFallback(d.ID, false)
	ch, prev, stale := s.Store.Apply(d.ID, gen, scope, jobs, s.now())
	if !ch.Empty() {
		why := ""
		if stale {
			why = "while HolyArrs was off"
		}
		cur := s.Store.Current(d.ID).Items
		s.persist.note(d.ID, ch, prev, cur, why)
		if rev := s.Store.Rev(d.ID); rev > 0 {
			s.Pub.Note(d.ID, rev-1, prev)
		}
		s.signal()
	}
	return nil
}

// isSab reports whether the engine has the paged SABnzbd reads.
func isSab(e Engine) bool {
	_, ok := e.(SabSource)
	return ok
}

// inCategory keeps the jobs in one of cats. "" or "*" among them keeps all.
func inCategory(jobs []Job, cats []string) []Job {
	if len(cats) == 0 || slices.Contains(cats, "") || slices.Contains(cats, "*") {
		return jobs
	}
	out := jobs[:0:0]
	for _, j := range jobs {
		if hasCategory(cats, j.Category) {
			out = append(out, j)
		}
	}
	return out
}
