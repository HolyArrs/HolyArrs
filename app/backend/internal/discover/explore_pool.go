package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The Explore engine's server side: the background pool job (its own
// paced limiter, never the per-person Explore limit), pools in SQLite,
// invalidation, and the head and tail answers.

const (
	// exploreAddWait is how long a rebuild waits after the last title add,
	// so a season or a bulk import triggers one rebuild per pool.
	exploreAddWait = 10 * time.Minute
	// ExplorePageTTL keeps an assembled page per member and switch.
	ExplorePageTTL = 10 * time.Minute
	// exploreKidLookups caps the rating lookups one Kid pool build makes.
	exploreKidLookups = 60
	maxExploreRows    = 5000
	// ExploreWorkers is how many pools build at once. They share one paced
	// limiter, so the outside rate is the same; a member whose lists are
	// already cached no longer waits behind another member's misses.
	ExploreWorkers = 3
)

var ExploreSwitches = []string{"all", "movies", "shows"}

// exploreListBurst is how many outside lists may go at once.
const exploreListBurst = 10

// ExploreListTTL is how long one outside list is kept for pool builds.
var ExploreListTTL = map[string]time.Duration{
	"trending": 6 * time.Hour, "upcoming": 12 * time.Hour, "best": 24 * time.Hour, "now_playing": 12 * time.Hour,
	"similar": 24 * time.Hour, "discover": 12 * time.Hour, "person": 24 * time.Hour,
	// weekly is a discover list kept a week (Top on your services).
	"weekly": 7 * 24 * time.Hour,
	// A new film joins a collection now and then; a day is soon enough.
	"collection": 24 * time.Hour,
}

// PoolKey names one member's pool for one switch.
type PoolKey struct {
	Member int64
	Switch string
}

type exploreList struct {
	at    time.Time
	items []engine.Item
}

// ExplorePage is one assembled page kept per member, switch and zone.
type ExplorePage struct {
	At    time.Time
	Built time.Time
	Page  engine.Page
}

// Explore is the pool job and its caches. The funcs are filled by the
// wiring (api.New); the rest is zero-value ready.
type Explore struct {
	Store        func() *database.Store
	Metadata     func() metadata.Provider
	KnownRating  func(context.Context, mediafiles.RatingRef) (string, bool)
	LookupRating func(context.Context, mediafiles.RatingRef) (string, bool)
	QueueCardArt func(context.Context, *engine.Pool)
	// Clock is the clock (tests set it).
	Clock func() time.Time
	// Paced is the job's own limiter (made on first use; tests set it).
	Paced *engine.Paced
	// Cards is the wide card art filler (cardart.go).
	Cards CardArt

	mu      sync.Mutex
	due     map[PoolKey]time.Time
	wake    chan struct{}
	started bool
	day     string
	lists   map[string]exploreList
	flight  map[string]*listFlight
	// failing are the list keys whose last outside ask failed, so a failing
	// list is logged once until it answers, not on every ask.
	failing map[string]bool
	pages   map[string]ExplorePage
	// builds counts finished pool builds (tests).
	builds int
	// building are the pools being built now; slots holds one token per
	// running build.
	building map[PoolKey]bool
	slots    chan struct{}
	// calls are the outside calls of each pool's last build.
	calls map[PoolKey]*engine.Calls
}

// Now is the Explore clock.
func (e *Explore) Now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

// Page gives the kept page for key.
func (e *Explore) Page(key string) (ExplorePage, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.pages[key]
	return p, ok
}

// KeepPage keeps an assembled page (at most 1000, then starts over).
func (e *Explore) KeepPage(key string, p ExplorePage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pages == nil || len(e.pages) >= 1000 {
		e.pages = map[string]ExplorePage{}
	}
	e.pages[key] = p
}

// BuildCounts says how many builds finished, are queued and are running (tests).
func (e *Explore) BuildCounts() (done, queued, running int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.builds, len(e.due), len(e.building)
}

// LastCalls copies each pool's last build calls (tests).
func (e *Explore) LastCalls() map[PoolKey]*engine.Calls {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[PoolKey]*engine.Calls, len(e.calls))
	for k, v := range e.calls {
		out[k] = v
	}
	return out
}

// Start starts the pool job: it listens for asks and adds, builds queued
// pools (up to ExploreWorkers at once), and rebuilds every recent pool each
// day. It says whether this call started it.
func (e *Explore) Start(ctx context.Context) bool {
	e.mu.Lock()
	if e.started || e.Store() == nil {
		e.mu.Unlock()
		return false
	}
	e.started = true
	e.wake = make(chan struct{}, 1)
	e.mu.Unlock()
	e.Store().OnEvent(e.OnEvent)
	go jobs.Guard("explore", func() { e.exploreLoop(ctx) })
	return true
}

// OnEvent marks pools stale: an ask (or joining one) marks the
// member's pools, a title added marks every pool of that switch.
func (e *Explore) OnEvent(ev domain.Event) {
	now := e.Now()
	switch ev.Kind {
	case domain.EventAskMade, domain.EventAskJoined:
		if ev.Actor.Kind == "user" && ev.Actor.ID > 0 {
			e.MarkMemberStale(context.Background(), ev.Actor.ID, now)
		}
	case domain.EventTitleAdded:
		kind := ""
		if ev.TitleID != nil {
			_ = e.Store().Read().QueryRow("SELECT kind FROM titles WHERE id = ?", *ev.TitleID).Scan(&kind)
		}
		if kind == "movie" || kind == "show" {
			e.markSwitchStale(context.Background(), kind, now.Add(exploreAddWait))
		}
	}
}

func switchesFor(kind string) []string {
	if kind == "movie" {
		return []string{"all", "movies"}
	}
	return []string{"all", "shows"}
}

// markMemberStale marks the member's pools stale, due now.
func (e *Explore) MarkMemberStale(ctx context.Context, member int64, due time.Time) {
	_ = e.Store().Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE explore_pool SET stale = 1, due = ? WHERE member_id = ?", database.FormatTime(due), member)
		return err
	})
	for _, sw := range ExploreSwitches {
		e.QueuePool(PoolKey{member, sw}, due, false)
	}
}

// markSwitchStale marks every member's pools for the kind's switches
// stale; the rebuild waits until due, and a later add moves due on.
func (e *Explore) markSwitchStale(ctx context.Context, kind string, due time.Time) {
	sws := switchesFor(kind)
	var keys []PoolKey
	_ = e.Store().Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE explore_pool SET stale = 1, due = ? WHERE type_switch IN (?, ?)", database.FormatTime(due), sws[0], sws[1])
		return err
	})
	rows, err := e.Store().Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool WHERE type_switch IN (?, ?)", sws[0], sws[1])
	if err != nil {
		return
	}
	for rows.Next() {
		var k PoolKey
		if rows.Scan(&k.Member, &k.Switch) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	for _, k := range keys {
		e.QueuePool(k, due, true)
	}
}

// queuePool queues one pool build. later moves an existing due time on
// (debounce); otherwise the earlier time wins.
func (e *Explore) QueuePool(k PoolKey, due time.Time, later bool) {
	e.mu.Lock()
	if e.due == nil {
		e.due = map[PoolKey]time.Time{}
	}
	cur, ok := e.due[k]
	if !ok || (later && due.After(cur)) || (!later && due.Before(cur)) {
		e.due[k] = due
	}
	wake := e.wake
	e.mu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (e *Explore) exploreLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		e.exploreDaily(ctx)
		e.RunDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-e.wake:
		}
	}
}

// runDue starts builds for due pools while a worker slot is free; a
// finished build wakes the loop for the next one.
func (e *Explore) RunDue(ctx context.Context) {
	e.mu.Lock()
	if e.slots == nil {
		// Low-memory mode builds one pool at a time (catching up after a restart takes longer).
		e.slots = make(chan struct{}, startup.LowMemoryPick(ExploreWorkers, 1))
	}
	slots := e.slots
	e.mu.Unlock()
	for ctx.Err() == nil {
		select {
		case slots <- struct{}{}:
		default:
			return
		}
		k, ok := e.NextDue()
		if !ok {
			<-slots
			return
		}
		go jobs.Guard("explore-build", func() {
			defer func() {
				e.mu.Lock()
				delete(e.building, k)
				wake := e.wake
				e.mu.Unlock()
				<-slots
				if wake != nil {
					select {
					case wake <- struct{}{}:
					default:
					}
				}
			}()
			if err := e.BuildPool(ctx, k.Member, k.Switch); err != nil && ctx.Err() == nil {
				log.Printf("explore: pool %d/%s: %v", k.Member, k.Switch, err)
			}
		})
	}
}

// nextDue takes the earliest pool that is due and not being built.
func (e *Explore) NextDue() (PoolKey, bool) {
	now := e.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	var best PoolKey
	var at time.Time
	found := false
	for k, d := range e.due {
		if !d.After(now) && !e.building[k] && (!found || d.Before(at)) {
			best, at, found = k, d, true
		}
	}
	if found {
		delete(e.due, best)
		if e.building == nil {
			e.building = map[PoolKey]bool{}
		}
		e.building[best] = true
	}
	return best, found
}

// exploreDaily runs once per day: it queues every pool built in the last
// two days for the new day, and prunes old pools and counts.
func (e *Explore) exploreDaily(ctx context.Context) {
	now := e.Now()
	day := engine.Day(now)
	e.mu.Lock()
	same := e.day == day
	e.day = day
	e.mu.Unlock()
	if same {
		return
	}
	cut := engine.Day(now.AddDate(0, 0, -2))
	_ = e.Store().Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM explore_pool WHERE day < ?", cut); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM explore_titles WHERE day < ?", engine.Day(now.AddDate(0, 0, -30))); err != nil {
			return err
		}
		return accounts.PruneViewCounts(ctx, tx, engine.Day(now.AddDate(0, 0, -90)))
	})
	rows, err := e.Store().Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool WHERE day < ?", day)
	if err != nil {
		return
	}
	var keys []PoolKey
	for rows.Next() {
		var k PoolKey
		if rows.Scan(&k.Member, &k.Switch) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	for _, k := range keys {
		e.QueuePool(k, now, false)
	}
}

// ExploreSource answers the engine's Requests from TMDB through the
// metadata server, cached per list in memory and in SQLite (so a restart
// or the next member reuses it); only misses wait for the limiter, and
// concurrent builds asking for the same list share one call.
type ExploreSource struct {
	E      *Explore
	Lister metadata.ExploreLister
	// cacheOnly answers from the cache and never goes out (cold pages).
	CacheOnly bool
	// calls counts this build's outside calls (nil: not counted).
	Calls *engine.Calls
}

// listFlight is one outside list being fetched; others wait on done.
type listFlight struct {
	done  chan struct{}
	items []engine.Item
	err   error
}

type storedList struct {
	At    time.Time     `json:"at"`
	Items []engine.Item `json:"items"`
}

// cachedList finds a list in memory, then in SQLite.
func (s ExploreSource) cachedList(ctx context.Context, key string) (exploreList, bool) {
	e := s.E
	e.mu.Lock()
	got, ok := e.lists[key]
	e.mu.Unlock()
	if ok || s.E.Store() == nil {
		return got, ok
	}
	b, ok, err := metadata.LookupCache{Store: s.E.Store()}.Get(ctx, "explore|"+key, time.Now())
	if err != nil || !ok {
		return got, false
	}
	var sl storedList
	if json.Unmarshal(b, &sl) != nil {
		return got, false
	}
	got = exploreList{at: sl.At, items: sl.Items}
	s.keepList(key, got)
	return got, true
}

// keepList puts a list in the memory cache, dropping expired ones when full.
func (s ExploreSource) keepList(key string, l exploreList) {
	e := s.E
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lists == nil || len(e.lists) >= maxExploreRows {
		for k, v := range e.lists {
			if now.Sub(v.at) >= ExploreListTTL[strings.SplitN(k, "|", 2)[0]] {
				delete(e.lists, k)
			}
		}
		if e.lists == nil || len(e.lists) >= maxExploreRows {
			e.lists = map[string]exploreList{}
		}
	}
	e.lists[key] = l
}

// Cached says q is answered from a fresh cached list (no outside call).
func (s ExploreSource) Cached(ctx context.Context, q engine.Request) bool {
	got, ok := s.cachedList(ctx, q.String())
	return ok && time.Since(got.at) < ExploreListTTL[q.List]
}
