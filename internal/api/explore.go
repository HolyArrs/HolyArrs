package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ratings"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// The Explore engine's server side: the background pool job (its own
// paced limiter, never the per-person Explore limit), pools in SQLite,
// invalidation, and the head and tail answers.

const (
	// exploreAddWait is how long a rebuild waits after the last title add,
	// so a season or a bulk import triggers one rebuild per pool.
	exploreAddWait = 10 * time.Minute
	// explorePageTTL keeps an assembled page per member and switch.
	explorePageTTL = 10 * time.Minute
	// exploreKidLookups caps the rating lookups one Kid pool build makes.
	exploreKidLookups = 60
	maxExploreRows    = 5000
	// exploreWorkers is how many pools build at once. They share one paced
	// limiter, so the outside rate is the same; a member whose lists are
	// already cached no longer waits behind another member's misses.
	exploreWorkers = 3
)

var exploreSwitches = []string{"all", "movies", "shows"}

// exploreListTTL is how long one outside list is kept for pool builds.
var exploreListTTL = map[string]time.Duration{
	"trending": 6 * time.Hour, "upcoming": 12 * time.Hour, "best": 24 * time.Hour,
	"similar": 24 * time.Hour, "discover": 12 * time.Hour, "person": 7 * 24 * time.Hour,
}

type poolKey struct {
	member int64
	sw     string
}

type exploreList struct {
	at    time.Time
	items []explore.Item
}

type explorePage struct {
	at    time.Time
	built time.Time
	page  explore.Page
	top10 map[string]any
}

// exploreState is the pool job and its caches.
type exploreState struct {
	mu      sync.Mutex
	due     map[poolKey]time.Time
	wake    chan struct{}
	started bool
	day     string
	paced   *explore.Paced
	lists   map[string]exploreList
	flight  map[string]*listFlight
	pages   map[string]explorePage
	// now is the clock (tests set it).
	now func() time.Time
	// builds counts finished pool builds (tests).
	builds int
	// building are the pools being built now; slots holds one token per
	// running build.
	building map[poolKey]bool
	slots    chan struct{}
	// calls are the outside calls of each pool's last build.
	calls map[poolKey]*explore.Calls
}

func (a *API) exploreNow() time.Time {
	if a.explore.now != nil {
		return a.explore.now()
	}
	return time.Now()
}

// StartExplore starts the pool job: it listens for asks and adds, builds
// queued pools (up to exploreWorkers at once), and rebuilds every recent pool each day.
func (a *API) StartExplore(ctx context.Context) {
	e := &a.explore
	e.mu.Lock()
	if e.started || a.s.Store == nil {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.wake = make(chan struct{}, 1)
	e.mu.Unlock()
	a.s.Store.OnEvent(a.exploreOnEvent)
	go supervisor.Guard("explore", func() { a.exploreLoop(ctx) })
}

// exploreOnEvent marks pools stale: an ask (or joining one) marks the
// member's pools, a title added marks every pool of that switch.
func (a *API) exploreOnEvent(ev domain.Event) {
	now := a.exploreNow()
	switch ev.Kind {
	case domain.EventAskMade, domain.EventAskJoined:
		if ev.Actor.Kind == "user" && ev.Actor.ID > 0 {
			a.markMemberStale(context.Background(), ev.Actor.ID, now)
		}
	case domain.EventTitleAdded:
		kind := ""
		if ev.TitleID != nil {
			_ = a.s.Store.Read().QueryRow("SELECT kind FROM titles WHERE id = ?", *ev.TitleID).Scan(&kind)
		}
		if kind == "movie" || kind == "show" {
			a.markSwitchStale(context.Background(), kind, now.Add(exploreAddWait))
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
func (a *API) markMemberStale(ctx context.Context, member int64, due time.Time) {
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE explore_pool SET stale = 1, due = ? WHERE member_id = ?", store.FormatTime(due), member)
		return err
	})
	for _, sw := range exploreSwitches {
		a.queuePool(poolKey{member, sw}, due, false)
	}
}

// markSwitchStale marks every member's pools for the kind's switches
// stale; the rebuild waits until due, and a later add moves due on.
func (a *API) markSwitchStale(ctx context.Context, kind string, due time.Time) {
	sws := switchesFor(kind)
	var keys []poolKey
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE explore_pool SET stale = 1, due = ? WHERE type_switch IN (?, ?)", store.FormatTime(due), sws[0], sws[1])
		return err
	})
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool WHERE type_switch IN (?, ?)", sws[0], sws[1])
	if err != nil {
		return
	}
	for rows.Next() {
		var k poolKey
		if rows.Scan(&k.member, &k.sw) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	for _, k := range keys {
		a.queuePool(k, due, true)
	}
}

// queuePool queues one pool build. later moves an existing due time on
// (debounce); otherwise the earlier time wins.
func (a *API) queuePool(k poolKey, due time.Time, later bool) {
	e := &a.explore
	e.mu.Lock()
	if e.due == nil {
		e.due = map[poolKey]time.Time{}
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

func (a *API) exploreLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		a.exploreDaily(ctx)
		a.runDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-a.explore.wake:
		}
	}
}

// runDue starts builds for due pools while a worker slot is free; a
// finished build wakes the loop for the next one.
func (a *API) runDue(ctx context.Context) {
	e := &a.explore
	e.mu.Lock()
	if e.slots == nil {
		e.slots = make(chan struct{}, exploreWorkers)
	}
	slots := e.slots
	e.mu.Unlock()
	for ctx.Err() == nil {
		select {
		case slots <- struct{}{}:
		default:
			return
		}
		k, ok := a.nextDue()
		if !ok {
			<-slots
			return
		}
		go supervisor.Guard("explore-build", func() {
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
			if err := a.buildPool(ctx, k.member, k.sw); err != nil && ctx.Err() == nil {
				log.Printf("explore: pool %d/%s: %v", k.member, k.sw, err)
			}
		})
	}
}

// nextDue takes the earliest pool that is due and not being built.
func (a *API) nextDue() (poolKey, bool) {
	e := &a.explore
	now := a.exploreNow()
	e.mu.Lock()
	defer e.mu.Unlock()
	var best poolKey
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
			e.building = map[poolKey]bool{}
		}
		e.building[best] = true
	}
	return best, found
}

// exploreDaily runs once per day: it queues every pool built in the last
// two days for the new day, and prunes old pools and counts.
func (a *API) exploreDaily(ctx context.Context) {
	now := a.exploreNow()
	day := explore.Day(now)
	e := &a.explore
	e.mu.Lock()
	same := e.day == day
	e.day = day
	e.mu.Unlock()
	if same {
		return
	}
	cut := explore.Day(now.AddDate(0, 0, -2))
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM explore_pool WHERE day < ?", cut); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM explore_titles WHERE day < ?", explore.Day(now.AddDate(0, 0, -30))); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM explore_title_counts WHERE day < ?", explore.Day(now.AddDate(0, 0, -90))); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM explore_counts WHERE day < ?", explore.Day(now.AddDate(0, 0, -90)))
		return err
	})
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool WHERE day < ?", day)
	if err != nil {
		return
	}
	var keys []poolKey
	for rows.Next() {
		var k poolKey
		if rows.Scan(&k.member, &k.sw) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	for _, k := range keys {
		a.queuePool(k, now, false)
	}
}

// exploreSource answers the engine's Requests from TMDB through the
// metadata server, cached per list in memory and in SQLite (so a restart
// or the next member reuses it); only misses wait for the limiter, and
// concurrent builds asking for the same list share one call.
type exploreSource struct {
	a      *API
	lister metadata.ExploreLister
	// cacheOnly answers from the cache and never goes out (cold pages).
	cacheOnly bool
	// calls counts this build's outside calls (nil: not counted).
	calls *explore.Calls
}

// listFlight is one outside list being fetched; others wait on done.
type listFlight struct {
	done  chan struct{}
	items []explore.Item
	err   error
}

type storedList struct {
	At    time.Time      `json:"at"`
	Items []explore.Item `json:"items"`
}

// cachedList finds a list in memory, then in SQLite.
func (s exploreSource) cachedList(ctx context.Context, key string) (exploreList, bool) {
	e := &s.a.explore
	e.mu.Lock()
	got, ok := e.lists[key]
	e.mu.Unlock()
	if ok || s.a.s.Store == nil {
		return got, ok
	}
	b, ok, err := store.LookupCache{Store: s.a.s.Store}.Get(ctx, "explore|"+key, time.Now())
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
func (s exploreSource) keepList(key string, l exploreList) {
	e := &s.a.explore
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lists == nil || len(e.lists) >= maxExploreRows {
		for k, v := range e.lists {
			if now.Sub(v.at) >= exploreListTTL[strings.SplitN(k, "|", 2)[0]] {
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
func (s exploreSource) Cached(ctx context.Context, q explore.Request) bool {
	got, ok := s.cachedList(ctx, q.String())
	return ok && time.Since(got.at) < exploreListTTL[q.List]
}

func (s exploreSource) List(ctx context.Context, q explore.Request) ([]explore.Item, error) {
	e := &s.a.explore
	key := q.String()
	ttl := exploreListTTL[q.List]
	got, ok := s.cachedList(ctx, key)
	if ok && time.Since(got.at) < ttl {
		return got.items, nil
	}
	if s.cacheOnly || s.lister == nil {
		if ok {
			return got.items, nil
		}
		return []explore.Item{}, nil
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
	if e.paced == nil {
		e.paced = &explore.Paced{Source: listerSource{s.lister}}
	}
	p := e.paced
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.flight, key)
		e.mu.Unlock()
		close(f.done)
	}()
	s.calls.Add(q)
	f.items, f.err = p.List(ctx, q)
	if f.err != nil {
		if ok {
			return got.items, nil
		}
		return nil, f.err
	}
	now := time.Now()
	s.keepList(key, exploreList{at: now, items: f.items})
	if s.a.s.Store != nil {
		if b, err := json.Marshal(storedList{At: now, Items: f.items}); err == nil {
			_ = store.LookupCache{Store: s.a.s.Store}.Put(context.WithoutCancel(ctx), "explore|"+key, b, now, now.Add(ttl))
		}
	}
	return f.items, nil
}

// listerSource maps a Request to TMDB lists.
type listerSource struct{ l metadata.ExploreLister }

func (s listerSource) List(ctx context.Context, q explore.Request) ([]explore.Item, error) {
	p := "movie"
	if q.Kind == "show" {
		p = "tv"
	}
	var out []metadata.DiscoverItem
	var err error
	switch q.List {
	case "trending":
		out, err = s.l.ExploreList(ctx, q.Kind, "/trending/"+p+"/week", nil)
	case "upcoming":
		path := "/movie/upcoming"
		if q.Kind == "show" {
			path = "/tv/on_the_air"
		}
		out, err = s.l.ExploreList(ctx, q.Kind, path, nil)
	case "best":
		out, err = s.l.ExploreList(ctx, q.Kind, "/discover/"+p, metadata.BestQuery(q.Kind, time.Now()))
	case "similar":
		var recs, sim []metadata.DiscoverItem
		if recs, err = s.l.ExploreList(ctx, q.Kind, "/"+p+"/"+q.Ref+"/recommendations", nil); err == nil {
			sim, err = s.l.ExploreList(ctx, q.Kind, "/"+p+"/"+q.Ref+"/similar", nil)
		}
		out = append(recs, sim...)
	case "person":
		var all []metadata.DiscoverItem
		all, err = s.l.PersonCredits(ctx, q.Ref)
		for _, it := range all {
			if q.Kind == "" || it.Kind == q.Kind {
				out = append(out, it)
			}
		}
	case "discover":
		v := url.Values{"include_adult": {"false"}, "sort_by": {"popularity.desc"}}
		if q.Sort == "rating" {
			v.Set("sort_by", "vote_average.desc")
		}
		if q.Genre != "" {
			id, gerr := s.l.GenreID(ctx, q.Kind, q.Genre)
			if gerr != nil {
				return nil, gerr
			}
			if id == 0 {
				return []explore.Item{}, nil
			}
			v.Set("with_genres", strconv.FormatInt(id, 10))
		}
		date := "primary_release_date"
		if q.Kind == "show" {
			date = "first_air_date"
		}
		if q.From != "" {
			v.Set(date+".gte", q.From)
		}
		if q.To != "" {
			v.Set(date+".lte", q.To)
		}
		if q.Services != "" && q.Region != "" {
			v.Set("with_watch_providers", q.Services)
			v.Set("watch_region", q.Region)
			v.Set("with_watch_monetization_types", "flatrate")
		}
		if q.MinVote > 0 {
			v.Set("vote_average.gte", strconv.FormatFloat(q.MinVote, 'f', 1, 64))
		}
		if q.MinVotes > 0 {
			v.Set("vote_count.gte", strconv.Itoa(q.MinVotes))
		}
		out, err = s.l.ExploreList(ctx, q.Kind, "/discover/"+p, v)
	default:
		return nil, errors.New("explore: unknown list " + q.List)
	}
	if err != nil {
		return nil, err
	}
	items := make([]explore.Item, 0, len(out))
	for _, d := range out {
		if q.MaxVotes > 0 && d.VoteCount > q.MaxVotes {
			continue
		}
		items = append(items, explore.Item{TMDBID: d.TMDBID, Kind: d.Kind, Name: d.Name, Year: d.Year, Poster: d.Poster, Backdrop: d.Backdrop,
			Overview: d.Overview, Genres: d.Genres, Pop: d.Popularity, Vote: d.VoteAverage, Votes: d.VoteCount, Released: d.Released})
	}
	return items, nil
}

// exploreMember is what a pool build needs about the member.
type exploreMember struct {
	id               int64
	kid              bool
	maxRating        string
	services, region string
}

func (a *API) loadMember(ctx context.Context, id int64) (exploreMember, error) {
	m := exploreMember{id: id}
	var role string
	err := a.s.Store.Read().QueryRowContext(ctx, "SELECT role, max_rating FROM users WHERE id = ?", id).Scan(&role, &m.maxRating)
	if err != nil {
		return m, err
	}
	m.kid = role == "member" && m.maxRating != ""
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT services, region FROM explore_prefs WHERE member_id = ?", id).Scan(&m.services, &m.region)
	return m, nil
}

// exploreInput reads the member's signals from the Library and the asks.
func (a *API) exploreInput(ctx context.Context, m exploreMember, sw string, now time.Time) (explore.Input, error) {
	in := explore.Input{Member: m.id, Kid: m.kid, Switch: sw, Now: now, Owned: map[string]bool{}, Monitored: map[string]bool{},
		OwnedPeople: map[string]int{}, OwnedDecadeGenre: map[string]int{}, Services: m.services, Region: m.region}
	db := a.s.Store.Read()
	rows, err := db.QueryContext(ctx, `SELECT kind, json_extract(source_ids, '$.tmdb'), name, COALESCE(year, 0), genres, people, rating, created_at,
		COALESCE(digital_date, cinema_date, ''), backdrop, artwork FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return in, err
	}
	var lib []explore.LibTitle
	soon := now.AddDate(0, 0, 90)
	for rows.Next() {
		var t explore.LibTitle
		var genres, people, added, release, backdrop, artwork string
		if err := rows.Scan(&t.Kind, &t.TMDBID, &t.Name, &t.Year, &genres, &people, &t.Rating, &added, &release, &backdrop, &artwork); err != nil {
			rows.Close()
			return in, err
		}
		_ = json.Unmarshal([]byte(genres), &t.Genres)
		_ = json.Unmarshal([]byte(people), &t.People)
		t.Added, _ = time.Parse(store.TimeLayout, added)
		key := t.Kind + ":" + t.TMDBID
		in.Owned[key] = true
		if m.kid && !ratings.Allows(m.maxRating, t.Kind, t.Rating) {
			continue
		}
		lib = append(lib, t)
		if now.Sub(t.Added) <= 14*24*time.Hour && t.Added.Before(now.Add(time.Minute)) {
			var art []string
			_ = json.Unmarshal([]byte(artwork), &art)
			it := explore.Item{TMDBID: t.TMDBID, Kind: t.Kind, Name: t.Name, Year: t.Year, Genres: t.Genres, Backdrop: backdrop, Owned: true}
			if links := artworkLinks(art); len(links) > 0 {
				it.Poster = links[0]
			}
			in.JustArrived = append(in.JustArrived, it)
		}
		for _, p := range t.People[:min(4, len(t.People))] {
			in.OwnedPeople[p]++
		}
		if t.Year > 0 {
			for _, g := range t.Genres {
				in.OwnedDecadeGenre[strconv.Itoa(t.Year/10*10)+"s|"+g]++
			}
		}
		if d, err := time.Parse(store.TimeLayout, release); err == nil && d.After(now) && d.Before(soon) {
			in.Monitored[key] = true
			in.Coming = append(in.Coming, explore.Item{TMDBID: t.TMDBID, Kind: t.Kind, Name: t.Name, Year: t.Year, Genres: t.Genres, Backdrop: backdrop,
				Released: d.Format("2006-01-02")})
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
	var asks []explore.AskSeed
	type hh struct {
		item    explore.Item
		asks    int
		members map[int64]bool
	}
	house := map[string]*hh{}
	var order []string
	month := now.AddDate(0, 0, -30)
	for rows.Next() {
		var s explore.AskSeed
		var poster, at string
		var open bool
		if err := rows.Scan(&s.Kind, &s.TMDBID, &s.Name, &s.Year, &poster, &s.Member, &at, &s.Kid, &open); err != nil {
			rows.Close()
			return in, err
		}
		s.At, _ = time.Parse(store.TimeLayout, at)
		asks = append(asks, s)
		if s.Member == m.id && open {
			in.MyAsks = append([]explore.Item{{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}}, in.MyAsks...)
		}
		if s.At.After(month) {
			key := s.Kind + ":" + s.TMDBID
			h, ok := house[key]
			if !ok {
				h = &hh{item: explore.Item{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}, members: map[int64]bool{}}
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
		in.Household = append(in.Household, explore.HouseholdTitle{Item: h.item, Asks: h.asks, Members: len(h.members)})
	}
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE state = 'active'").Scan(&in.Members)
	in.Seeds = explore.Seeds(m.id, m.kid, lib, asks, now)
	return in, nil
}

// kidAllow is the Kid limit at candidate time: known ratings first, then a
// few lookups; an unknown rating hides the title.
func (a *API) kidAllow(ctx context.Context, max string) func(explore.Item) bool {
	left := exploreKidLookups
	return func(it explore.Item) bool {
		ref := ratingRef{kind: it.Kind, tmdbID: it.TMDBID}
		r, ok := a.knownRating(ctx, ref)
		if !ok && left > 0 {
			left--
			r, ok = a.lookupRating(ctx, ref)
		}
		return ok && ratings.Allows(max, it.Kind, r)
	}
}

// readPool reads the member's pool for a day ("" takes the newest).
func (a *API) readPool(ctx context.Context, member int64, sw, day string, before bool) (*explore.Pool, bool, error) {
	q := "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day = ?"
	if before {
		q = "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day < ? ORDER BY day DESC LIMIT 1"
	}
	var raw string
	var stale bool
	err := a.s.Store.Read().QueryRowContext(ctx, q, member, sw, day).Scan(&raw, &stale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var p explore.Pool
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, false, err
	}
	return &p, stale, nil
}

// buildPool builds and stores one member's pool for today. It goes out
// only through the job's own limiter, never the per-person limit.
func (a *API) buildPool(ctx context.Context, member int64, sw string) error {
	now := a.exploreNow()
	m, err := a.loadMember(ctx, member)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := a.exploreInput(ctx, m, sw, now)
	if err != nil {
		return err
	}
	day := explore.Day(now)
	if in.Today, _, err = a.readPool(ctx, member, sw, day, false); err != nil {
		return err
	}
	if in.Prev, _, err = a.readPool(ctx, member, sw, day, true); err != nil {
		return err
	}
	if m.kid {
		in.Allow = a.kidAllow(ctx, m.maxRating)
	}
	var lister metadata.ExploreLister
	if l, ok := metadata.ExploreListerFor(ctx, a.s.Metadata); ok {
		lister = l
	}
	calls := &explore.Calls{}
	start := time.Now()
	pool, err := explore.Build(ctx, exploreSource{a: a, lister: lister, calls: calls}, in)
	if err != nil {
		return err
	}
	log.Printf("explore: pool %d/%s built in %s, %d seeds, outside calls %s", member, sw, time.Since(start).Round(time.Millisecond), pool.Seeds, calls)
	raw, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	err = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_pool (member_id, type_switch, day, built_at, stale, due, pool) VALUES (?, ?, ?, ?, 0, NULL, ?)
			ON CONFLICT (member_id, type_switch, day) DO UPDATE SET built_at = excluded.built_at, stale = 0, due = NULL, pool = excluded.pool`,
			member, sw, day, store.FormatTime(now), string(raw))
		return err
	})
	if err != nil {
		return err
	}
	a.explore.mu.Lock()
	a.explore.builds++
	if a.explore.calls == nil {
		a.explore.calls = map[poolKey]*explore.Calls{}
	}
	a.explore.calls[poolKey{member, sw}] = calls
	for k := range a.explore.pages {
		if strings.HasPrefix(k, strconv.FormatInt(member, 10)+"|"+sw+"|") {
			delete(a.explore.pages, k)
		}
	}
	a.explore.mu.Unlock()
	return nil
}
