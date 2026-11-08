package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/titleview"

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

func (s ExploreSource) List(ctx context.Context, q engine.Request) ([]engine.Item, error) {
	e := s.E
	key := q.String()
	ttl := ExploreListTTL[q.List]
	got, ok := s.cachedList(ctx, key)
	if ok && time.Since(got.at) < ttl {
		return got.items, nil
	}
	if s.CacheOnly || s.Lister == nil {
		if ok {
			return got.items, nil
		}
		return []engine.Item{}, nil
	}
	e.mu.Lock()
	if f, busy := e.flight[key]; busy {
		e.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if f.err == nil {
			return f.items, nil
		}
		if ok {
			return got.items, nil
		}
		return nil, f.err
	}
	if e.flight == nil {
		e.flight = map[string]*listFlight{}
	}
	f := &listFlight{done: make(chan struct{})}
	e.flight[key] = f
	if e.Paced == nil {
		// A cold genre page asks up to 9 lists at once; a burst lets them
		// run side by side instead of 2 s apart (about 6 s cold before).
		e.Paced = &engine.Paced{Source: ListerSource{L: s.Lister}, Burst: exploreListBurst}
	}
	p := e.Paced
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.flight, key)
		e.mu.Unlock()
		close(f.done)
	}()
	s.Calls.Add(q)
	f.items, f.err = p.List(ctx, q)
	if f.err != nil {
		if ctx.Err() == nil && s.noteListDown(key, true) {
			log.Printf("explore: list %s failed: %v", q.List, f.err)
		}
		if ok {
			return got.items, nil
		}
		return nil, f.err
	}
	s.noteListDown(key, false)
	now := time.Now()
	s.keepList(key, exploreList{at: now, items: f.items})
	if s.E.Store() != nil {
		if b, err := json.Marshal(storedList{At: now, Items: f.items}); err == nil {
			_ = metadata.LookupCache{Store: s.E.Store()}.Put(context.WithoutCancel(ctx), "explore|"+key, b, now, now.Add(ttl))
		}
	}
	return f.items, nil
}

// noteListDown records whether key's last outside ask failed and says
// whether that changed. Kept per key: one list's keys fail on their own (a
// removed title's "similar"), and some answer from the TMDB copy in an
// outage. Bounded like the list cache; starting over only logs again.
func (s ExploreSource) noteListDown(key string, down bool) bool {
	e := s.E
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failing[key] == down {
		return false
	}
	if !down {
		delete(e.failing, key)
		return true
	}
	if e.failing == nil || len(e.failing) >= maxExploreRows {
		e.failing = map[string]bool{}
	}
	e.failing[key] = true
	return true
}

// ExploreMember is what a pool build needs about the member.
type ExploreMember struct {
	ID               int64
	Kid              bool
	MaxRating        string
	Services, Region string
}

func (e *Explore) LoadMember(ctx context.Context, id int64) (ExploreMember, error) {
	m := ExploreMember{ID: id}
	var role string
	err := e.Store().Read().QueryRowContext(ctx, "SELECT role, max_rating FROM users WHERE id = ?", id).Scan(&role, &m.MaxRating)
	if err != nil {
		return m, err
	}
	m.Kid = role == "member" && m.MaxRating != ""
	_ = e.Store().Read().QueryRowContext(ctx, "SELECT services, region FROM explore_prefs WHERE member_id = ?", id).Scan(&m.Services, &m.Region)
	return m, nil
}

// exploreInput reads the member's signals from the Library and the asks.
func (e *Explore) Input(ctx context.Context, m ExploreMember, sw string, now time.Time) (engine.Input, error) {
	in := engine.Input{Member: m.ID, Kid: m.Kid, Switch: sw, Now: now, Owned: map[string]bool{},
		OwnedPeople: map[string]int{}, OwnedDecadeGenre: map[string]int{}, Services: m.Services, Region: m.Region}
	db := e.Store().Read()
	rows, err := db.QueryContext(ctx, `SELECT kind, json_extract(source_ids, '$.tmdb'), name, COALESCE(year, 0), genres, people, rating, created_at
		FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return in, err
	}
	var lib []engine.LibTitle
	for rows.Next() {
		var t engine.LibTitle
		var genres, people, added string
		if err := rows.Scan(&t.Kind, &t.TMDBID, &t.Name, &t.Year, &genres, &people, &t.Rating, &added); err != nil {
			rows.Close()
			return in, err
		}
		_ = json.Unmarshal([]byte(genres), &t.Genres)
		_ = json.Unmarshal([]byte(people), &t.People)
		t.Added, _ = time.Parse(database.TimeLayout, added)
		key := t.Kind + ":" + t.TMDBID
		in.Owned[key] = true
		if m.Kid && !metadata.RatingAllows(m.MaxRating, t.Kind, t.Rating) {
			continue
		}
		lib = append(lib, t)
		for _, p := range t.People[:min(4, len(t.People))] {
			in.OwnedPeople[p]++
		}
		if t.Year > 0 {
			for _, g := range t.Genres {
				in.OwnedDecadeGenre[strconv.Itoa(t.Year/10*10)+"s|"+g]++
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return in, err
	}
	rows, err = db.QueryContext(ctx, `SELECT a.kind, a.tmdb_id, a.name, COALESCE(a.year, 0), a.poster, p.user_id, p.asked_at, u.role = 'member' AND u.max_rating != '',
		a.state IN ('asked', 'approved')
		FROM ask_people p JOIN asks a ON a.id = p.ask_id JOIN users u ON u.id = p.user_id
		WHERE a.kind IN ('movie', 'show') AND a.tmdb_id != '' AND a.state != 'turned_down' ORDER BY p.asked_at`)
	if err != nil {
		return in, err
	}
	var asks []engine.AskSeed
	type hh struct {
		item    engine.Item
		asks    int
		members map[int64]bool
	}
	house := map[string]*hh{}
	var order []string
	month := now.AddDate(0, 0, -30)
	for rows.Next() {
		var s engine.AskSeed
		var poster, at string
		var open bool
		if err := rows.Scan(&s.Kind, &s.TMDBID, &s.Name, &s.Year, &poster, &s.Member, &at, &s.Kid, &open); err != nil {
			rows.Close()
			return in, err
		}
		s.At, _ = time.Parse(database.TimeLayout, at)
		asks = append(asks, s)
		if s.Member == m.ID && open {
			in.MyAsks = append([]engine.Item{{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}}, in.MyAsks...)
		}
		if s.At.After(month) {
			key := s.Kind + ":" + s.TMDBID
			h, ok := house[key]
			if !ok {
				h = &hh{item: engine.Item{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}, members: map[int64]bool{}}
				house[key] = h
				order = append(order, key)
			}
			h.asks++
			h.members[s.Member] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return in, err
	}
	for _, k := range order {
		h := house[k]
		in.Household = append(in.Household, engine.HouseholdTitle{Item: h.item, Asks: h.asks, Members: len(h.members)})
	}
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE state = 'active'").Scan(&in.Members)
	if sw == "shows" {
		// Returning soon: Library shows whose next episode airs in 7 to 60 days.
		if in.Returning, err = ExploreReturning(ctx, db, now); err != nil {
			return in, err
		}
	}
	if sw == "movies" {
		// Finish the collection: TMDB collections the Library has a movie of.
		if in.Collections, err = ExploreCollections(ctx, db); err != nil {
			return in, err
		}
	}
	in.Seeds = engine.Seeds(m.ID, m.Kid, lib, asks, now)
	return in, nil
}

// ExploreReturning are the Library shows whose next episode airs in 7 to
// 60 days, soonest first (local data only).
func ExploreReturning(ctx context.Context, db *sql.DB, now time.Time) ([]engine.Item, error) {
	rows, err := db.QueryContext(ctx, `SELECT json_extract(t.source_ids, '$.tmdb'), t.name, COALESCE(t.year, 0), t.genres, t.artwork, t.backdrop, MIN(i.air_date) AS next
		FROM titles t JOIN items i ON i.title_id = t.id AND i.kind = 'episode' AND i.dropped_at IS NULL AND i.air_date > ?
		WHERE t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') IS NOT NULL
		GROUP BY t.id HAVING next >= ? AND next <= ? ORDER BY next, t.id LIMIT 40`,
		database.FormatTime(now), database.FormatTime(now.AddDate(0, 0, 7)), database.FormatTime(now.AddDate(0, 0, 60)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.Item
	for rows.Next() {
		var it engine.Item
		var genres, artwork, next string
		if err := rows.Scan(&it.TMDBID, &it.Name, &it.Year, &genres, &artwork, &it.Backdrop, &next); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &it.Genres)
		var art []string
		_ = json.Unmarshal([]byte(artwork), &art)
		if links := titleview.ArtworkLinks(art); len(links) > 0 {
			it.Poster = links[0]
		}
		if d, err := time.Parse(database.TimeLayout, next); err == nil {
			it.Released = d.Format("2006-01-02")
		}
		it.Kind, it.Owned = "show", true
		out = append(out, it)
	}
	return out, rows.Err()
}

// ExploreCollections are the TMDB collections with an owned movie, most
// owned first (local data only; the parts are looked up by collection id).
func ExploreCollections(ctx context.Context, db *sql.DB) ([]engine.LibCollection, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(c.source_id, 6), c.name, count(t.id) AS n
		FROM movie_collections c
		JOIN movie_collection_titles ct ON ct.collection_id = c.id
		JOIN titles t ON t.id = ct.title_id AND t.kind = 'movie' AND t.state != 'removed'
		WHERE substr(c.source_id, 1, 5) = 'tmdb:'
		GROUP BY c.id ORDER BY n DESC, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.LibCollection
	for rows.Next() {
		var c engine.LibCollection
		if err := rows.Scan(&c.TMDBID, &c.Name, &c.Owned); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// kidAllow is the Kid limit at candidate time: known ratings first, then a
// few lookups; an unknown rating hides the title.
func (e *Explore) KidAllow(ctx context.Context, max string) func(engine.Item) bool {
	left := exploreKidLookups
	return func(it engine.Item) bool {
		ref := mediafiles.RatingRef{Kind: it.Kind, TMDBID: it.TMDBID}
		r, ok := e.KnownRating(ctx, ref)
		if !ok && left > 0 {
			left--
			r, ok = e.LookupRating(ctx, ref)
		}
		return ok && metadata.RatingAllows(max, it.Kind, r)
	}
}

// readPool reads the member's pool for a day ("" takes the newest).
func (e *Explore) ReadPool(ctx context.Context, member int64, sw, day string, before bool) (*engine.Pool, bool, error) {
	q := "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day = ?"
	if before {
		q = "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day < ? ORDER BY day DESC LIMIT 1"
	}
	var raw string
	var stale bool
	err := e.Store().Read().QueryRowContext(ctx, q, member, sw, day).Scan(&raw, &stale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var p engine.Pool
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, false, err
	}
	return &p, stale, nil
}

// buildPool builds and stores one member's pool for today. It goes out
// only through the job's own limiter, never the per-person limit.
func (e *Explore) BuildPool(ctx context.Context, member int64, sw string) error {
	now := e.Now()
	m, err := e.LoadMember(ctx, member)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := e.Input(ctx, m, sw, now)
	if err != nil {
		return err
	}
	day := engine.Day(now)
	if in.Today, _, err = e.ReadPool(ctx, member, sw, day, false); err != nil {
		return err
	}
	if in.Prev, _, err = e.ReadPool(ctx, member, sw, day, true); err != nil {
		return err
	}
	if m.Kid {
		in.Allow = e.KidAllow(ctx, m.MaxRating)
	}
	var lister metadata.ExploreLister
	if l, ok := metadata.ExploreListerFor(ctx, e.Metadata()); ok {
		lister = l
	}
	calls := &engine.Calls{}
	start := time.Now()
	pool, err := engine.Build(ctx, ExploreSource{E: e, Lister: lister, Calls: calls}, in)
	if err != nil {
		return err
	}
	log.Printf("explore: pool %d/%s built in %s, %d seeds, outside calls %s", member, sw, time.Since(start).Round(time.Millisecond), pool.Seeds, calls)
	raw, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	err = e.Store().Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_pool (member_id, type_switch, day, built_at, stale, due, pool) VALUES (?, ?, ?, ?, 0, NULL, ?)
			ON CONFLICT (member_id, type_switch, day) DO UPDATE SET built_at = excluded.built_at, stale = 0, due = NULL, pool = excluded.pool`,
			member, sw, day, database.FormatTime(now), string(raw))
		return err
	})
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.builds++
	if e.calls == nil {
		e.calls = map[PoolKey]*engine.Calls{}
	}
	e.calls[PoolKey{member, sw}] = calls
	for k := range e.pages {
		if strings.HasPrefix(k, strconv.FormatInt(member, 10)+"|"+sw+"|") {
			delete(e.pages, k)
		}
	}
	e.mu.Unlock()
	e.QueueCardArt(ctx, &pool)
	return nil
}
