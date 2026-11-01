package dlsync

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// Manager is what the reader needs from downloader.Manager.
type Manager interface {
	Reader(ctx context.Context, d domain.Downloader) (downloader.Engine, uint64, error)
	ReportRead(ctx context.Context, d domain.Downloader, err error)
	Generation(id int64) uint64
	Interval(active int) time.Duration
}

// Reader timings.
const (
	callTimeout  = 4 * time.Second   // one read of one client
	kickDebounce = 1 * time.Second   // a kick reads at most this soon after the last read
	widenCap     = 300 * time.Second // failures widen the cadence up to this
	flushTimeout = 5 * time.Second   // FinalFlush's own deadline
)

// scanEvery is the coordinator's tick (tests shorten it).
var scanEvery = 2 * time.Second

// restartBackoff is the wait before a poller that panicked starts again.
var restartBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, time.Minute, 300 * time.Second}

// Sync is the background reader (setting downloads.sync = on): one poller
// per Downloader owns every list read, through the Manager's guarded
// engine; the Manager and the Downloads view read its Store. It runs
// inside the "downloads" loop (see Around), so safe mode and the part
// switch govern it. With any other setting value it reads nothing and
// writes nothing.
type Sync struct {
	Store   *Store
	DB      *store.Store
	Mode    *Shadow // reads downloads.sync (shared with the shadow hook)
	Manager Manager
	Lines   LinesFunc
	Match   MatchFunc
	// Pub sends push updates of the snapshots (nil: none).
	Pub *Publisher
	Now func() time.Time
	// Sleep waits (tests may shorten it); nil waits for real.
	Sleep func(ctx context.Context, d time.Duration) error

	cursors Cursors
	sabs    SabCursors
	persist *persister
	bucket  bucket
	started atomic.Bool
	changed chan struct{}

	mu      sync.Mutex
	kicks   map[int64]chan struct{}
	pollers map[int64]context.CancelFunc
	wasOn   bool // on during this process's life before (re-entry after legacy)
	onNow   bool
}

func (s *Sync) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

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

// mode reads downloads.sync.
func (s *Sync) mode(ctx context.Context) string {
	if s.Mode == nil {
		return ModeLegacy
	}
	return s.Mode.Mode(ctx)
}

// On is downloader.SyncSource.On: the reader runs and the setting is on.
func (s *Sync) On() bool {
	if s == nil || !s.started.Load() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.mode(ctx) == ModeOn
}

// ClientStatus is a qBittorrent's state from the reader's last read, when
// the reader is on and has one.
func (s *Sync) ClientStatus(id int64) (downloader.ClientStatus, bool) {
	if !s.On() {
		return downloader.ClientStatus{}, false
	}
	return s.cursors.Status(id)
}

// View is downloader.SyncSource.View.
func (s *Sync) View(id int64) downloader.SyncView { return s.Store.View(id) }

// Fallback is downloader.SyncSource.Fallback.
func (s *Sync) Fallback(id int64, on bool) { s.Store.SetFallback(id, on) }

// Changed is downloader.SyncSource.Changed.
func (s *Sync) Changed() <-chan struct{} {
	s.init()
	return s.changed
}

// Kick asks for a read of one client soon (debounced). A no-op when the
// reader is not running.
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
// before, every client is read soon, so the faster viewer cadence starts
// now instead of after the idle wait.
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

func (s *Sync) signal() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Around runs the "downloads" loop body run with the reader beside it.
// Pollers outlive run until it returns (or panics), then they stop, are
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
			supervisor.Guard("dlsync publisher", func() { s.Pub.Run(syncCtx) })
		}()
	}
	defer func() {
		stop()
		wg.Wait()
		s.started.Store(false)
		s.FinalFlush(loopCtx)
		// The next start loads the last known rows again.
		s.mu.Lock()
		s.onNow = false
		s.mu.Unlock()
	}()
	run(loopCtx)
}

// FinalFlush writes everything pending, with its own 5 s deadline, so an
// already cancelled loop ctx does not abort the commit.
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

// coordinate keeps one poller per Downloader while the setting is on,
// flushes the persister and paces the match queue.
func (s *Sync) coordinate(ctx context.Context, wg *sync.WaitGroup) {
	var lastMatch time.Time
	for {
		on := s.mode(ctx) == ModeOn
		if on {
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
		} else {
			s.leave(ctx)
		}
		if s.sleep(ctx, scanEvery) != nil {
			s.stopPollers()
			return
		}
	}
}

// enter runs once on each switch to on: the last known rows come back as
// stale, so changes while off are diffed against them.
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

// leave runs on each switch away from on: pollers stop and pending writes
// are flushed. dl_items and the match queue are left alone from then on.
func (s *Sync) leave(ctx context.Context) {
	s.mu.Lock()
	if !s.onNow {
		s.mu.Unlock()
		return
	}
	s.onNow, s.wasOn = false, true
	s.mu.Unlock()
	s.stopPollers()
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
	defer cancel()
	if err := s.persist.flush(fctx); err != nil {
		log.Printf("dlsync: flush on leaving: %v", err)
	}
}

// offNote says why stale rows were not seen live.
func (s *Sync) offNote() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wasOn {
		return "while sync was off"
	}
	return "while HolyArrs was off"
}

func (s *Sync) stopPollers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.pollers {
		cancel()
		delete(s.pollers, id)
	}
}

// ensurePollers starts a poller for every Downloader that has none and
// stops the ones whose Downloader was removed.
func (s *Sync) ensurePollers(ctx context.Context, wg *sync.WaitGroup) {
	if s.DB == nil {
		return
	}
	all, err := store.Select[domain.Downloader](ctx, s.DB.Read(), "downloaders", "removed = 0")
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

// supervise runs one client's poller, restarting it after a panic with
// backoff. A panic marks the client stale, so the Manager falls back.
func (s *Sync) supervise(ctx context.Context, id int64, kick chan struct{}) {
	for crashes := 0; ctx.Err() == nil; crashes++ {
		clean := false
		supervisor.Guard("dlsync poller", func() {
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

// poll reads one client on its cadence until ctx ends or the Downloader
// is gone.
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
		if err := store.Get(ctx, s.DB.Read(), "downloaders", id, &d); err != nil || d.Removed {
			return
		}
		n := s.active(ctx, id)
		every := s.cadence(d, n)
		if s.Store.Current(id).Fallback {
			// The Manager reads it itself: only one recovery read per floor.
			every = s.Manager.Interval(n)
		}
		s.Store.SetFloor(id, every)
		last = s.now()
		err := s.read(ctx, d)
		switch {
		case err == nil:
			fails = 0
			// The wait follows what this read saw (a torrent that just
			// started moving gets the faster step now, not one read later).
			if !s.Store.Current(id).Fallback {
				every = s.cadence(d, n)
				s.Store.SetFloor(id, every)
			}
		case errors.Is(err, downloader.ErrAuth):
			// Turned down the sign-in: stop until the settings change (a
			// new engine generation) so the client never bans us.
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

// widen grows the cadence one step per failure (and one for a shared
// session), capped, with +/-20% jitter when widened.
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

// cadence is the client's read interval from the cadence table.
func (s *Sync) cadence(d domain.Downloader, n int) time.Duration {
	snap := s.Store.Current(d.ID)
	outside := false
	for _, j := range snap.Items {
		if j.State == downloader.StateDownloading {
			outside = true
			break
		}
	}
	return Cadence(d.Adapter, n, s.Store.Viewer(), outside, s.Manager.Interval(1), s.Manager.Interval(0))
}

// Cadence is the read interval for one client: never slower than the
// Manager's own loop (active with HolyArrs Downloads, idle without), faster
// while someone views the Downloads page.
func Cadence(adapter string, n int, viewer, outsideActive bool, active, idle time.Duration) time.Duration {
	pick := func(qbit, sab, other time.Duration) time.Duration {
		switch adapter {
		case "qbittorrent":
			return qbit
		case "sabnzbd":
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

// read reads one client once through the guarded engine and records it.
func (s *Sync) read(ctx context.Context, d domain.Downloader) error {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	e, gen, err := s.Manager.Reader(cctx, d)
	if err != nil {
		s.Store.Fail(d.ID, gen)
		return err
	}
	var jobs []downloader.Job
	scope := d.Category
	shared := false
	ms, isMain := e.(downloader.MainSyncer)
	switch {
	case d.Adapter == "qbittorrent" && isMain:
		var p MainPoll
		p, err = s.cursors.Poll(cctx, d.ID, gen, ms)
		jobs, scope, shared = p.Jobs, "", p.SharedSession
	case d.Adapter == "sabnzbd" && isSab(e):
		var p SabPoll
		p, err = s.sabs.Poll(cctx, d.ID, gen, e.(SabSource))
		jobs = p.Jobs
		if s.SABOutside(ctx) {
			scope = ""
		} else {
			jobs = inCategory(jobs, d.Category)
		}
	case d.Protocol == "usenet" && s.SABOutside(ctx):
		jobs, err = e.List(cctx, "")
		scope = ""
	default:
		jobs, err = e.List(cctx, d.Category)
	}
	if ctx.Err() != nil && err != nil {
		return err // stopping: not the client's fault
	}
	s.Manager.ReportRead(ctx, d, err)
	if err != nil {
		s.Store.Fail(d.ID, gen)
		return err
	}
	s.Store.SetShared(d.ID, shared)
	ch, prev, stale := s.Store.Apply(d.ID, gen, scope, jobs, s.now())
	if !ch.Empty() {
		why := ""
		if stale {
			why = s.offNote()
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

// isSab says the engine has the paged SABnzbd reads.
func isSab(e downloader.Engine) bool {
	_, ok := e.(SabSource)
	return ok
}

// inCategory keeps the jobs of one category ("" or "*" keeps all).
func inCategory(jobs []downloader.Job, cat string) []downloader.Job {
	if cat == "" || cat == "*" {
		return jobs
	}
	out := jobs[:0:0]
	for _, j := range jobs {
		if j.Category == cat {
			out = append(out, j)
		}
	}
	return out
}

// SettingSABOutside is the setting downloads.sab_outside (Usenet items
// HolyArrs did not send shown on the Downloads page); default on.
const SettingSABOutside = "downloads_sab_outside"

// SABOutside reads downloads.sab_outside.
func (s *Sync) SABOutside(ctx context.Context) bool {
	if s == nil || s.DB == nil {
		return false
	}
	var v string
	_ = s.DB.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", SettingGroup, SettingSABOutside).Scan(&v)
	return v != "false"
}
