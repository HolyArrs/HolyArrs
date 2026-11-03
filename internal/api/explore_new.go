package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// newPageTTL is how long a member's built New & Popular page is kept; the
// shared lists under it keep their own TTLs (6 to 24 hours).
const newPageTTL = 6 * time.Hour

// newFirstRows is how many rows the first page carries; later pages carry
// newPageRows each.
const (
	newFirstRows = 5
	newPageRows  = 4
)

// newWait is how long a cold build waits for its outside lists; lists still
// out are left for the next ask (the answer says building).
const newWait = 1500 * time.Millisecond

// newWorld are the original languages of "New from abroad".
var newWorld = []string{"ko", "ja", "es"}

// newPerService is about how many new titles each service carries in the
// services row.
const newPerService = 20

// newPage is one member's built page: the rows, the This week rail, why each
// title is new, its score and the service each services-row title is on.
type newPage struct {
	rows   []explore.Row
	rail   []newRailDay
	facts  map[string]explore.NewFact
	scores map[string]float64
	marks  map[string]streamingMark
	// picked says the member chose their services.
	picked bool
	// key is the page cache key (member, switch, Kid limit, region, day).
	key string
	// perService is each chip row's own list per service id (row type ->
	// service -> titles), so a chip shows that service's titles, not a
	// filter of the shared row.
	perService map[string]map[int][]explore.Item
	// svcNames names each service id on the page.
	svcNames map[int]string
}

// newRailDay is one day of the This week rail.
type newRailDay struct {
	Day   string
	Items []newRailItem
}

// newRailItem is one title on a day: what happens (added, out, episode),
// on which service, and whether the member follows it.
type newRailItem struct {
	explore.Item
	What      string
	Service   *streamingMark
	Following bool
	Upcoming  bool
}

type newPageEntry struct {
	at   time.Time
	page newPage
}

type newPageCache struct {
	mu    sync.Mutex
	pages map[string]newPageEntry
	swept time.Time // when expired pages were last dropped
}

var newPages newPageCache

// heroShown is each member's latest Explore hero (member id -> []explore.Item),
// so New & Popular's hero shows other titles. Entries expire after a day.
var heroShown = newTTLMap(24*time.Hour, 10000)

// newHeroMax is how many titles New & Popular's hero rotates through.
const newHeroMax = 6

// newHeroKeys keeps the hero picked for a built page (page cache key ->
// kind:tmdb set), so every page of rows leaves those titles out. Entries
// expire with the page cache.
var newHeroKeys = newTTLMap(explorePageTTL, 2000)

// newHero picks the hero: the best-scoring new titles of the rail, the
// services row and the hot and new rows that pass Explore's quality gate
// (a trailer, a logo and a backdrop), leaving out Explore's own hero.
func (a *API) newHero(r *http.Request, pg newPage, me int64) ([]discoverItemView, error) {
	skip := map[string]bool{}
	if v, ok := heroShown.Load(me); ok {
		for _, it := range v.([]explore.Item) {
			skip[it.Key()] = true
		}
	}
	var cands []explore.Item
	seen := map[string]bool{}
	take := func(it explore.Item) {
		if !seen[it.Key()] && !skip[it.Key()] && pg.scores[it.Key()] > 0 {
			seen[it.Key()] = true
			cands = append(cands, it)
		}
	}
	for _, d := range pg.rail {
		for _, it := range d.Items {
			take(it.Item)
		}
	}
	for _, row := range pg.rows {
		if row.Type == explore.TypeNPServices || row.Type == explore.TypeNPTopShows || row.Type == explore.TypeNPTopFilms {
			for _, it := range row.Items {
				take(it)
			}
		}
	}
	explore.SortByScore(cands, func(it explore.Item) float64 { return pg.scores[it.Key()] })
	cands = cands[:min(len(cands), 24)]
	views, err := a.exploreViews(r.Context(), r, []explore.Row{{Items: cands}})
	if err != nil || len(views) != 1 {
		return []discoverItemView{}, err
	}
	var vs []discoverItemView
	var titles [][2]string
	for _, v := range views[0].Items {
		if !v.InLibrary {
			vs = append(vs, v)
			titles = append(titles, [2]string{v.Kind, v.TMDBID})
		}
	}
	a.prefetchTrailers(r, titles)
	a.fillTrailerKeys(r, vs)
	a.fillCardArt(r, vs)
	now := a.exploreNow().UTC()
	out := []discoverItemView{}
	for _, v := range vs {
		if len(out) == newHeroMax || v.TrailerKey == "" || !heroArtReady(v) {
			continue
		}
		k := v.Kind + ":" + v.TMDBID
		if f := newFresh(pg.facts[k], now); f != nil {
			v.Fresh = f
		}
		if m, ok := pg.marks[k]; ok {
			v.Streaming = &m
		}
		out = append(out, v)
	}
	return out, nil
}

// withoutHero drops the hero's titles from rows.
func withoutHero(rows []explore.Row, hero map[string]bool) []explore.Row {
	if len(hero) == 0 {
		return rows
	}
	out := make([]explore.Row, 0, len(rows))
	for _, r := range rows {
		items := make([]explore.Item, 0, len(r.Items))
		for _, it := range r.Items {
			if !hero[it.Key()] {
				items = append(items, it)
			}
		}
		r.Items = items
		out = append(out, r)
	}
	return out
}

func (c *newPageCache) get(key string) (newPage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.pages[key]
	if !ok || time.Since(e.at) > newPageTTL {
		return newPage{}, false
	}
	return e.page, true
}

func (c *newPageCache) put(key string, p newPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	// A page's key carries its day, so past days' pages are dropped (at most
	// once a minute) instead of piling up until the cache is full.
	if now.Sub(c.swept) >= time.Minute {
		for k, e := range c.pages {
			if now.Sub(e.at) > newPageTTL {
				delete(c.pages, k)
			}
		}
		c.swept = now
	}
	if c.pages == nil || len(c.pages) > 500 {
		c.pages = map[string]newPageEntry{}
	}
	c.pages[key] = newPageEntry{at: now, page: p}
}

// newEpisode is a Library show's episode near now (aired in the last 14
// days or airing in the next 7).
type newEpisode struct {
	show            explore.Item
	at              time.Time
	season, episode int
}

// newHousehold is what New & Popular reads about the household (local).
type newHousehold struct {
	services []int
	region   string
	genres   map[string]int // Library titles per genre
	topGenre map[string]string
	follows  map[string]bool // kind:tmdb the member follows
	owned    map[string]bool
	episodes []newEpisode
}

func (a *API) newHousehold(ctx context.Context, me int64, now time.Time) newHousehold {
	h := newHousehold{genres: map[string]int{}, topGenre: map[string]string{}, follows: map[string]bool{}, owned: map[string]bool{}}
	db := a.s.Store.Read()
	var services string
	_ = db.QueryRowContext(ctx, "SELECT services, region FROM explore_prefs WHERE member_id = ?", me).Scan(&services, &h.region)
	h.region = strings.ToUpper(h.region)
	for _, s := range strings.Split(services, "|") {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			h.services = append(h.services, n)
		}
	}
	kindGenre := map[string]map[string]int{"movie": {}, "show": {}}
	if rows, err := db.QueryContext(ctx, `SELECT kind, json_extract(source_ids, '$.tmdb'), genres FROM titles
		WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL`); err == nil {
		for rows.Next() {
			var kind, id, genres string
			if rows.Scan(&kind, &id, &genres) != nil {
				continue
			}
			h.owned[kind+":"+id] = true
			var gs []string
			_ = json.Unmarshal([]byte(genres), &gs)
			for _, g := range gs {
				h.genres[g]++
				kindGenre[kind][g]++
			}
		}
		rows.Close()
	}
	for kind, m := range kindGenre {
		best := 0
		for g, n := range m {
			if n > best || (n == best && g < h.topGenre[kind]) {
				best, h.topGenre[kind] = n, g
			}
		}
	}
	if rows, err := db.QueryContext(ctx, `SELECT 'show', tmdb_id FROM tmdb_follows WHERE user_id = ?
		UNION SELECT 'movie', tmdb_id FROM tmdb_movie_follows WHERE user_id = ?
		UNION SELECT t.kind, json_extract(t.source_ids, '$.tmdb') FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = ?`, me, me, me); err == nil {
		for rows.Next() {
			var kind string
			var id *string
			if rows.Scan(&kind, &id) == nil && id != nil {
				h.follows[kind+":"+*id] = true
			}
		}
		rows.Close()
	}
	// Library shows' episodes aired in the last 14 days or airing in the next 7.
	if rows, err := db.QueryContext(ctx, `SELECT json_extract(t.source_ids, '$.tmdb'), t.name, COALESCE(t.year, 0), t.genres, t.artwork, t.backdrop,
		i.air_date, COALESCE(json_extract(i.numbers, '$.season'), 0), COALESCE(json_extract(i.numbers, '$.episode'), 0)
		FROM titles t JOIN items i ON i.title_id = t.id AND i.kind = 'episode' AND i.dropped_at IS NULL AND i.air_date >= ? AND i.air_date <= ?
		WHERE t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') IS NOT NULL
		ORDER BY i.air_date DESC, t.id LIMIT 400`, store.FormatTime(now.AddDate(0, 0, -14)), store.FormatTime(now.AddDate(0, 0, 8))); err == nil {
		for rows.Next() {
			var e newEpisode
			var genres, artwork, at string
			if rows.Scan(&e.show.TMDBID, &e.show.Name, &e.show.Year, &genres, &artwork, &e.show.Backdrop, &at, &e.season, &e.episode) != nil {
				continue
			}
			_ = json.Unmarshal([]byte(genres), &e.show.Genres)
			var art []string
			_ = json.Unmarshal([]byte(artwork), &art)
			if links := artworkLinks(art); len(links) > 0 {
				e.show.Poster = links[0]
			}
			e.show.Kind, e.show.Owned = "show", true
			if e.at = explore.ParseDay(at); !e.at.IsZero() {
				h.episodes = append(h.episodes, e)
			}
		}
		rows.Close()
	}
	return h
}

// suit is how much a title suits the household: its genres' share of the
// Library, plus a lot for a Library title or one the member follows.
func (h newHousehold) suit(it explore.Item) float64 {
	total := 0
	for _, n := range h.genres {
		total += n
	}
	s := 0.0
	if total > 0 {
		for _, g := range it.Genres {
			s += float64(h.genres[g]) / float64(total)
		}
	}
	if h.owned[it.Key()] || h.follows[it.Key()] {
		s += 1
	}
	return s
}

// newServiceLists are the titles newly added to each of the member's
// services (every Streaming Availability service when none is picked),
// newest first, each with when it was added; from the kept lists (never
// fetched here beyond the 12-hour service-top read).
func (a *API) newServiceLists(ctx context.Context, h newHousehold, kinds map[string]bool) ([]streamingMark, map[int][]explore.Item, map[string]time.Time, metadata.ServiceTop) {
	added := map[string]time.Time{}
	by := map[int][]explore.Item{}
	var order []streamingMark
	if a.s.ServiceTop == nil || h.region == "" {
		return nil, by, added, metadata.ServiceTop{}
	}
	list, err := a.cachedServiceTop(ctx, h.region)
	if err != nil {
		return nil, by, added, metadata.ServiceTop{}
	}
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
	for _, sv := range list.Services {
		id := serviceTopProviders[sv.Service]
		if id == 0 || (len(picked) > 0 && !picked[id]) {
			continue
		}
		order = append(order, streamingMark{Service: id, Name: sv.Name})
		for _, it := range sv.New {
			kind := serviceKind(it.Kind)
			if it.TMDBID <= 0 || !kinds[kind] {
				continue
			}
			e := explore.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				e.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			if at := explore.ParseDay(it.AddedAt); !at.IsZero() && at.After(added[e.Key()]) {
				added[e.Key()] = at
			}
			by[id] = append(by[id], e)
		}
	}
	return order, by, added, list
}

// newCharts are today's top titles of kind on each of the member's services
// (Streaming Availability's own charts), in rank order.
func newCharts(list metadata.ServiceTop, h newHousehold, kind string) ([]streamingMark, [][]explore.Item) {
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
	var marks []streamingMark
	var lists [][]explore.Item
	for _, sv := range list.Services {
		id := serviceTopProviders[sv.Service]
		if id == 0 || sv.Source != "streaming-availability" || (len(picked) > 0 && !picked[id]) {
			continue
		}
		var l []explore.Item
		for _, it := range sv.Items {
			if it.TMDBID <= 0 || serviceKind(it.Kind) != kind {
				continue
			}
			e := explore.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				e.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			l = append(l, e)
		}
		marks = append(marks, streamingMark{Service: id, Name: sv.Name})
		lists = append(lists, l)
	}
	return marks, lists
}

// newLists names one kind's shared lists (see newRequests).
type newLists struct {
	trending, upcoming, cinema, released, episodes, series, critics, genre []explore.Item
	world                                                                  [][]explore.Item
	// onService are the TMDB top-ups per service id (watch providers,
	// newest out first).
	onService map[int][]explore.Item
}

// newRequests are one kind's shared lists: TMDB only, each kept by the list
// cache (warmed like Browse; a cold build waits newWait and says building).
func newRequests(kind, genre, region string, services []streamingMark, now time.Time) ([]explore.Request, func([][]explore.Item) newLists) {
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	back := -45 // a movie release window
	if kind == "show" {
		back = -60 // a brand-new series window
	}
	var reqs []explore.Request
	at := map[string]int{}
	add := func(name string, q explore.Request) {
		at[name] = len(reqs)
		reqs = append(reqs, q)
	}
	add("trending", explore.Request{List: "trending"})
	add("upcoming", explore.Request{List: "upcoming"}) // shows: on the air now
	add("cinema", explore.Request{List: "now_playing", Region: region})
	if kind == "movie" {
		add("released", explore.Request{List: "discover", From: day(back), To: day(0), Types: "4|3", Region: region, MinVotes: 10})
	} else {
		// Shows with an episode out this week (a new episode or season).
		add("episodes", explore.Request{List: "discover", AirFrom: day(-6), AirTo: day(0), MinVotes: 20})
		add("series", explore.Request{List: "discover", From: day(back), To: day(0), MinVotes: 5})
	}
	add("critics", explore.Request{List: "discover", From: day(-60), To: day(0), MinVote: 7.2, MinVotes: 50, Sort: "rating"})
	for _, l := range newWorld {
		add("world:"+l, explore.Request{List: "discover", Lang: l, From: day(back), To: day(0), MinVotes: 10})
	}
	if genre != "" {
		add("genre", explore.Request{List: "discover", Genre: genre, From: day(back), To: day(0), MinVotes: 10})
	}
	for _, s := range services {
		q := explore.Request{List: "discover", Services: strconv.Itoa(s.Service), Region: region, From: day(back), To: day(0), Sort: "recent", MinVotes: 5}
		if kind == "show" {
			q = explore.Request{List: "discover", Services: strconv.Itoa(s.Service), Region: region, AirFrom: day(-6), AirTo: day(0), MinVotes: 5}
		}
		add("service:"+strconv.Itoa(s.Service), q)
	}
	for i := range reqs {
		reqs[i].Kind, reqs[i].Scope = kind, explore.ScopeShared
	}
	return reqs, func(got [][]explore.Item) newLists {
		get := func(name string) []explore.Item {
			if i, ok := at[name]; ok && i < len(got) {
				return got[i]
			}
			return nil
		}
		l := newLists{trending: get("trending"), upcoming: get("upcoming"), cinema: get("cinema"), released: get("released"), episodes: get("episodes"),
			series: get("series"), critics: get("critics"), genre: get("genre"), onService: map[int][]explore.Item{}}
		for _, w := range newWorld {
			l.world = append(l.world, get("world:"+w))
		}
		for _, s := range services {
			l.onService[s.Service] = get("service:" + strconv.Itoa(s.Service))
		}
		return l
	}
}

// newFacts keeps the freshest fact of each title.
type newFacts struct {
	now time.Time
	m   map[string]explore.NewFact
}

func (f newFacts) put(key string, n explore.NewFact) {
	if old, ok := f.m[key]; ok && explore.Freshness(old, f.now) >= explore.Freshness(n, f.now) {
		return
	}
	f.m[key] = n
}

// dated is the fact a title's own date gives: a movie's release (upcoming
// when ahead), a show's first air date (a brand-new series).
func (f newFacts) dated(it explore.Item) {
	d := explore.ParseDay(it.Released)
	if d.IsZero() {
		return
	}
	switch {
	case d.After(f.now):
		f.put(it.Key(), explore.NewFact{Kind: explore.NewUpcoming, At: d})
	case it.Kind == "movie":
		f.put(it.Key(), explore.NewFact{Kind: explore.NewRelease, At: d})
	case it.Kind == "show":
		f.put(it.Key(), explore.NewFact{Kind: explore.NewSeries, At: d})
	}
}

// buildNewPage builds the member's page (before per-person views): kept 6
// hours once every list is back; building says some lists were still out.
func (a *API) newRows(ctx context.Context, r *http.Request, me int64, sw string) (newPage, bool) {
	now := a.exploreNow().UTC()
	h := a.newHousehold(ctx, me, now)
	kinds := map[string]bool{}
	for _, k := range explore.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	key := strconv.FormatInt(me, 10) + "|" + sw + "|" + a.kidMax(r) + "|" + h.region + "|" + now.Format("2006-01-02")
	if p, ok := newPages.get(key); ok {
		return p, false
	}
	order, svcNew, added, top := a.newServiceLists(ctx, h, kinds)
	pg := newPage{facts: map[string]explore.NewFact{}, scores: map[string]float64{}, marks: map[string]streamingMark{}, picked: len(h.services) > 0, key: key,
		perService: map[string]map[int][]explore.Item{explore.TypeNPServices: {}, explore.TypeNPTopShows: {}, explore.TypeNPTopFilms: {}}, svcNames: map[int]string{}}
	for _, s := range order {
		pg.svcNames[s.Service] = s.Name
	}
	facts := newFacts{now: now, m: pg.facts}
	for k, at := range added {
		facts.put(k, explore.NewFact{Kind: explore.NewService, At: at})
	}
	ready := true
	lists := map[string]newLists{}
	for _, k := range []string{"movie", "show"} {
		if !kinds[k] {
			continue
		}
		reqs, name := newRequests(k, h.topGenre[k], h.region, order, now)
		g, ok := a.genreLists(ctx, k, reqs, newWait)
		lists[k], ready = name(g), ready && ok
	}
	// Every TMDB title's own date, then the stronger facts on top.
	var pool []explore.Item
	for _, l := range lists {
		all := [][]explore.Item{l.trending, l.upcoming, l.cinema, l.released, l.episodes, l.series, l.critics, l.genre}
		all = append(all, l.world...)
		for _, s := range l.onService {
			all = append(all, s)
		}
		for _, items := range all {
			for _, it := range items {
				facts.dated(it)
				pool = append(pool, it)
			}
		}
		for _, it := range l.episodes {
			// The air-date list says an episode came out this week; three
			// days is the middle of that week.
			facts.put(it.Key(), explore.NewFact{Kind: explore.NewEpisode, At: now.AddDate(0, 0, -3)})
		}
	}
	for _, e := range h.episodes {
		if e.at.After(now) {
			continue
		}
		f := explore.NewFact{Kind: explore.NewEpisode, At: e.at}
		if e.episode == 1 && e.season > 1 {
			f = explore.NewFact{Kind: explore.NewSeason, At: e.at, Season: e.season}
		} else if e.episode == 1 && e.season == 1 {
			f = explore.NewFact{Kind: explore.NewSeries, At: e.at}
		}
		facts.put(e.show.Key(), f)
		pool = append(pool, e.show)
	}
	pop := explore.PopPercentile(pool)
	maxSuit := 0.0
	for _, it := range pool {
		maxSuit = max(maxSuit, h.suit(it))
	}
	score := func(it explore.Item) float64 {
		if s, ok := pg.scores[it.Key()]; ok {
			return s
		}
		p, ok := pop[it.Key()]
		if !ok {
			p = 0.5 // a service add TMDB has not listed yet
		}
		t := 0.0
		if maxSuit > 0 {
			t = h.suit(it) / maxSuit
		}
		s := explore.NewScore(explore.Freshness(pg.facts[it.Key()], now), p, t)
		pg.scores[it.Key()] = s
		return s
	}
	fresh := func(items []explore.Item) []explore.Item {
		out := []explore.Item{}
		for _, it := range items {
			if kinds[it.Kind] && score(it) > 0 && pg.facts[it.Key()].Kind != explore.NewUpcoming {
				out = append(out, it)
			}
		}
		return out
	}
	ranked := func(items []explore.Item) []explore.Item {
		out := fresh(items)
		explore.SortByScore(out, score)
		return out
	}
	both := func(get func(newLists) []explore.Item) []explore.Item {
		var all []explore.Item
		for _, k := range []string{"show", "movie"} {
			if l, ok := lists[k]; ok {
				all = append(all, get(l)...)
			}
		}
		return ranked(all)
	}

	var rows []explore.Row
	addRow := func(id, typ, title string, items []explore.Item) {
		rows = append(rows, explore.Row{ID: id, Type: typ, Title: title, Items: items})
	}
	// Fresh on your services: each service's adds, topped up to about 20 from
	// TMDB's newest on it; Library titles after the rest.
	var perService [][]explore.Item
	for _, s := range order {
		var items []explore.Item
		items = append(items, svcNew[s.Service]...)
		for _, k := range []string{"show", "movie"} {
			items = append(items, lists[k].onService[s.Service]...)
		}
		items = ranked(items)
		sort.SliceStable(items, func(i, j int) bool { return !h.owned[items[i].Key()] && h.owned[items[j].Key()] })
		seen := map[string]bool{}
		var l []explore.Item
		for _, it := range items {
			if seen[it.Key()] || len(l) >= newPerService {
				continue
			}
			seen[it.Key()] = true
			l = append(l, it)
			if _, ok := pg.marks[it.Key()]; !ok {
				pg.marks[it.Key()] = s
			}
		}
		perService = append(perService, l)
		pg.perService[explore.TypeNPServices][s.Service] = l
	}
	addRow("np_services", explore.TypeNPServices, "Fresh on your services", explore.RankMerge(perService, explore.NewRowCap[explore.TypeNPServices]))
	// Hot and new: each service's real chart, new titles only, topped up from
	// the scored pool.
	for _, k := range []string{"show", "movie"} {
		if !kinds[k] {
			continue
		}
		marks, charts := newCharts(top, h, k)
		typ := explore.TypeNPTopFilms
		if k == "show" {
			typ = explore.TypeNPTopShows
		}
		var lists2 [][]explore.Item
		for i, c := range charts {
			l := fresh(c)
			for j := range l {
				if _, ok := pg.marks[l[j].Key()]; !ok {
					m := marks[i]
					m.Rank = j + 1
					pg.marks[l[j].Key()] = m
				}
			}
			lists2 = append(lists2, l)
			pg.perService[typ][marks[i].Service] = l
		}
		hot := explore.RankMerge(lists2, 10)
		have := map[string]bool{}
		for _, it := range hot {
			have[it.Key()] = true
		}
		var rest []explore.Item
		for _, it := range pool {
			if it.Kind == k && !have[it.Key()] {
				have[it.Key()] = true
				rest = append(rest, it)
			}
		}
		hot = append(hot, ranked(rest)...)
		if k == "show" {
			addRow("np_top_shows", explore.TypeNPTopShows, "Hot and new: shows", hot)
		} else {
			addRow("np_top_movies", explore.TypeNPTopFilms, "Hot and new: movies", hot)
		}
	}
	if kinds["show"] {
		var mine []explore.Item
		for _, e := range h.episodes {
			if !e.at.After(now) {
				mine = append(mine, e.show)
			}
		}
		for _, it := range lists["show"].episodes {
			if h.follows[it.Key()] || h.owned[it.Key()] {
				mine = append(mine, it)
			}
		}
		addRow("np_episodes", explore.TypeNPEpisodes, "Your shows, new episodes", ranked(mine))
	}
	var picks []explore.Item
	for _, it := range ranked(pool) {
		if h.suit(it) > 0 && !h.owned[it.Key()] {
			picks = append(picks, it)
		}
	}
	addRow("np_picks", explore.TypeNPPicks, "Made for you, just out", picks)
	if kinds["show"] {
		var first []explore.Item
		for _, it := range ranked(pool) {
			if it.Kind == "show" && pg.facts[it.Key()].Kind == explore.NewSeries {
				first = append(first, it)
			}
		}
		addRow("np_series", explore.TypeNPSeries, "First seasons", first)
	}
	if kinds["movie"] {
		addRow("np_cinema", explore.TypeNPCinema, "In cinemas now", ranked(lists["movie"].cinema))
	}
	if g := h.topGenre["show"]; g != "" || h.topGenre["movie"] != "" {
		if g == "" || (kinds["movie"] && !kinds["show"]) {
			g = h.topGenre["movie"]
		}
		addRow("np_genre", explore.TypeNPGenre, "Just out in "+strings.ToLower(g), both(func(l newLists) []explore.Item { return l.genre }))
	}
	addRow("np_acclaimed", explore.TypeNPAcclaim, "Critics' new favourites", both(func(l newLists) []explore.Item { return l.critics }))
	addRow("np_world", explore.TypeNPWorld, "New from abroad", both(func(l newLists) []explore.Item {
		var all []explore.Item
		for _, w := range l.world {
			all = append(all, w...)
		}
		return all
	}))
	weight := map[string]float64{explore.TypeNPEpisodes: 1.3, explore.TypeNPPicks: 1.2, explore.TypeNPSeries: 1, explore.TypeNPGenre: 1,
		explore.TypeNPCinema: 0.9, explore.TypeNPAcclaim: 0.8, explore.TypeNPWorld: 0.6}
	pg.rows = explore.OrderNewRows(explore.DedupeRows(rows, explore.NewRowMin), score, weight)
	pg.rail = newRail(h, lists, svcNew, order, pg, kinds, score, now)
	if ready {
		newPages.put(key, pg)
	}
	return pg, !ready
}

// newRailMax is the most titles a rail day carries.
const newRailMax = 12

// newRail is the This week rail: three days back to seven ahead. Past days
// carry service adds, releases and Library episodes; days ahead carry TMDB's
// upcoming movies and the Library's next episodes.
func newRail(h newHousehold, lists map[string]newLists, svcNew map[int][]explore.Item, order []streamingMark, pg newPage, kinds map[string]bool, score func(explore.Item) float64, now time.Time) []newRailDay {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	byDay := map[string][]newRailItem{}
	seen := map[string]bool{}
	put := func(d time.Time, it newRailItem) {
		if !kinds[it.Kind] {
			return
		}
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		off := int(d.Sub(today).Hours() / 24)
		if off < -3 || off > 7 {
			return
		}
		// A title once on the rail: the first fact put wins.
		if seen[it.Key()] {
			return
		}
		seen[it.Key()] = true
		it.Upcoming = off > 0
		it.Following = h.follows[it.Key()]
		it.Owned = h.owned[it.Key()]
		byDay[d.Format("2006-01-02")] = append(byDay[d.Format("2006-01-02")], it)
	}
	names := map[int]streamingMark{}
	for _, s := range order {
		names[s.Service] = s
	}
	for _, s := range order {
		id := s.Service
		for _, it := range svcNew[id] {
			f := pg.facts[it.Key()]
			if f.Kind == explore.NewService {
				m := names[id]
				put(f.At, newRailItem{Item: it, What: "On " + m.Name, Service: &m})
			}
		}
	}
	if l, ok := lists["movie"]; ok {
		for _, items := range [][]explore.Item{l.released, l.cinema, l.trending, l.upcoming} {
			for _, it := range items {
				d := explore.ParseDay(it.Released)
				if d.IsZero() {
					continue
				}
				what := "Out"
				if d.After(now) {
					what = "Coming"
				}
				put(d, newRailItem{Item: it, What: what})
			}
		}
	}
	for _, e := range h.episodes {
		what := "S" + strconv.Itoa(e.season) + " E" + strconv.Itoa(e.episode)
		if e.episode == 1 && e.season > 0 {
			what = "Season " + strconv.Itoa(e.season)
		}
		put(e.at, newRailItem{Item: e.show, What: what})
	}
	var out []newRailDay
	for off := -3; off <= 7; off++ {
		d := today.AddDate(0, 0, off).Format("2006-01-02")
		items := byDay[d]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Owned != items[j].Owned {
				return items[i].Owned // the household's own shows first
			}
			return items[i].Pop > items[j].Pop
		})
		if len(items) > newRailMax {
			items = items[:newRailMax]
		}
		if len(items) > 0 {
			out = append(out, newRailDay{Day: d, Items: items})
		}
	}
	return out
}

// newFresh is the card strip a title's newness gives (Phase C): Added 2d
// ago, Out Nov 4, Season 3, New episode Fri, New series.
func newFresh(f explore.NewFact, now time.Time) *explore.Fresh {
	if f.At.IsZero() {
		return nil
	}
	days := int(now.Sub(f.At).Hours() / 24)
	switch f.Kind {
	case explore.NewService:
		t := "Added today"
		if days == 1 {
			t = "Added yesterday"
		} else if days > 1 {
			t = "Added " + strconv.Itoa(days) + "d ago"
		}
		return &explore.Fresh{Kind: explore.FreshRecentlyAdded, Text: t}
	case explore.NewRelease:
		return &explore.Fresh{Kind: explore.FreshNewRelease, Text: "Out " + f.At.Format("Jan 2")}
	case explore.NewUpcoming:
		return &explore.Fresh{Kind: explore.FreshComing, Text: "Out " + f.At.Format("Jan 2")}
	case explore.NewSeason:
		return &explore.Fresh{Kind: explore.FreshNewSeason, Text: "Season " + strconv.Itoa(f.Season)}
	case explore.NewEpisode:
		if days <= 6 {
			return &explore.Fresh{Kind: explore.FreshNewEpisode, Text: "New episode " + f.At.Format("Mon")}
		}
		return &explore.Fresh{Kind: explore.FreshNewEpisode, Text: "New episodes"}
	case explore.NewSeries:
		return &explore.Fresh{Kind: explore.FreshNewRelease, Text: "New series"}
	}
	return nil
}

// newViews joins rows (Kid limit, never-add, Library), drops rows left
// under least and puts each card's newness strip and service on it.
func (a *API) newViews(r *http.Request, rows []explore.Row, pg newPage, least int) ([]exploreRowView, error) {
	views, err := a.exploreViews(r.Context(), r, rows)
	if err != nil {
		return nil, err
	}
	now := a.exploreNow().UTC()
	out := []exploreRowView{}
	for _, v := range views {
		if len(v.Items) < least {
			continue
		}
		a.fillTrailerKeys(r, v.Items)
		out = append(out, v)
	}
	a.fillRowsCardArt(r, out)
	a.fillStreaming(r, out)
	for i := range out {
		for j := range out[i].Items {
			it := &out[i].Items[j]
			k := it.Kind + ":" + it.TMDBID
			if f := newFresh(pg.facts[k], now); f != nil {
				it.Fresh = f
			}
			if m, ok := pg.marks[k]; ok && (out[i].Type == explore.TypeNPServices || out[i].Type == explore.TypeNPTopShows || out[i].Type == explore.TypeNPTopFilms) {
				it.Streaming = &m
			}
		}
	}
	return out, nil
}

// newRailView is one rail title: its card plus the day's word for it.
type newRailView struct {
	discoverItemView
	What      string `json:"what"`
	Following bool   `json:"following"`
	Upcoming  bool   `json:"upcoming"`
}

type newRailDayView struct {
	Day   string        `json:"day"`
	Items []newRailView `json:"items"`
}

// newRailViews joins the rail with the Library and the Kid limit.
func (a *API) newRailViews(r *http.Request, rail []newRailDay) ([]newRailDayView, error) {
	out := []newRailDayView{}
	for _, d := range rail {
		row := explore.Row{ID: "np_rail_" + d.Day, Type: "np_rail", Items: make([]explore.Item, 0, len(d.Items))}
		extra := map[string]newRailItem{}
		for _, it := range d.Items {
			row.Items = append(row.Items, it.Item)
			if _, ok := extra[it.Key()]; !ok {
				extra[it.Key()] = it
			}
		}
		views, err := a.exploreViews(r.Context(), r, []explore.Row{row})
		if err != nil {
			return nil, err
		}
		if len(views) == 0 || len(views[0].Items) == 0 {
			continue
		}
		dv := newRailDayView{Day: d.Day, Items: []newRailView{}}
		for _, v := range views[0].Items {
			e := extra[v.Kind+":"+v.TMDBID]
			if e.Service != nil {
				m := *e.Service
				v.Streaming = &m
			}
			dv.Items = append(dv.Items, newRailView{discoverItemView: v, What: e.What, Following: e.Following, Upcoming: e.Upcoming})
		}
		out = append(out, dv)
	}
	return out, nil
}

// newParams reads ?type= and checks the member may see the page.
func (a *API) newParams(w http.ResponseWriter, r *http.Request) (string, bool) {
	sw, ok := exploreSwitch(r.URL.Query().Get("type"))
	if !ok {
		fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return "", false
	}
	if a.kidLimited(w, r) {
		return "", false
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return "", false
	}
	return sw, true
}

// exploreNew answers GET /api/v1/explore/new?type=all|movies|shows&page=n:
// New & Popular, a page of its rows (5 first, then 4 a page); page 0 also
// carries the This week rail. more says another page follows, building
// that some lists are still on their way (ask again shortly), services_set
// whether the member picked their services.
func (a *API) exploreNew(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if r.URL.Query().Get("page") == "" {
		n, err = 0, nil
	}
	if err != nil || n < 0 || n > 20 {
		fail(w, http.StatusBadRequest, "page must be a number from 0")
		return
	}
	sw, ok := a.newParams(w, r)
	if !ok {
		return
	}
	me := a.person(r).UserID
	a.musicProvider().WarmNew() // the Music tab's lists build in the background (Music on)
	pg, building := a.newRows(r.Context(), r, me, sw)
	var hero []discoverItemView
	if n == 0 {
		if hero, err = a.newHero(r, pg, me); err != nil {
			internal(w, err)
			return
		}
		keys := map[string]bool{}
		for _, v := range hero {
			keys[v.Kind+":"+v.TMDBID] = true
		}
		newHeroKeys.Store(pg.key, keys)
	}
	heroKeys := map[string]bool{}
	if v, ok := newHeroKeys.Load(pg.key); ok {
		heroKeys = v.(map[string]bool)
	}
	rows := withoutHero(pg.rows, heroKeys)
	from, to := 0, newFirstRows
	if n > 0 {
		from = newFirstRows + (n-1)*newPageRows
		to = from + newPageRows
	}
	from, to = min(from, len(rows)), min(to, len(rows))
	views, err := a.newViews(r, rows[from:to], pg, explore.NewRowMin)
	if err != nil {
		internal(w, err)
		return
	}
	body := map[string]any{"type": sw, "page": n, "more": to < len(rows), "building": building, "services_set": pg.picked, "rows": views}
	if n == 0 {
		body["hero"] = hero
		rail, err := a.newRailViews(r, pg.rail)
		if err != nil {
			internal(w, err)
			return
		}
		body["rail"] = rail
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, body)
}

// newPreviewMax and newPreviewPer shape Explore's New on your services
// row: the best-scoring new titles of the page's services row, a few per
// service.
const (
	newPreviewMax = 12
	newPreviewPer = 3
)

// exploreNewServices answers GET /api/v1/explore/new/services?type=: the
// best of "Fresh on your services" for Explore (12 titles, at most 3 per
// service, all new).
func (a *API) exploreNewServices(w http.ResponseWriter, r *http.Request) {
	sw, ok := a.newParams(w, r)
	if !ok {
		return
	}
	pg, _ := a.newRows(r.Context(), r, a.person(r).UserID, sw)
	var items []explore.Item
	for _, row := range pg.rows {
		if row.Type == explore.TypeNPServices {
			items = append(items, row.Items...)
		}
	}
	explore.SortByScore(items, func(it explore.Item) float64 { return pg.scores[it.Key()] })
	items = explore.CapPer(items, func(it explore.Item) string { return strconv.Itoa(pg.marks[it.Key()].Service) }, newPreviewPer, newPreviewMax*2)
	row := exploreRowView{ID: "np_services", Type: explore.TypeNPServices, Title: "New on your services", Items: []discoverItemView{}}
	views, err := a.newViews(r, []explore.Row{{ID: "np_services", Type: explore.TypeNPServices, Title: "New on your services", Items: items}}, pg, 1)
	if err != nil {
		internal(w, err)
		return
	}
	if len(views) == 1 {
		row = views[0]
		row.Items = row.Items[:min(len(row.Items), newPreviewMax)]
	}
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "row": row})
}

// newServiceRows names the chip rows exploreNewService answers for.
var newServiceRows = map[string]string{"services": explore.TypeNPServices, "hot_shows": explore.TypeNPTopShows, "hot_movies": explore.TypeNPTopFilms}

// exploreNewService answers GET /api/v1/explore/new/service?id=&type=&row=:
// one service's own list for a chip row (row services, the default: up to
// 20 new titles, Library titles last; hot_shows or hot_movies: that
// service's chart, new titles only). From the built page (kept 6 hours), so
// it never asks Streaming Availability again.
func (a *API) exploreNewService(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "id must be a service id")
		return
	}
	name := r.URL.Query().Get("row")
	if name == "" {
		name = "services"
	}
	typ, ok := newServiceRows[name]
	if !ok {
		fail(w, http.StatusBadRequest, "row must be services, hot_shows or hot_movies")
		return
	}
	sw, ok := a.newParams(w, r)
	if !ok {
		return
	}
	pg, building := a.newRows(r.Context(), r, a.person(r).UserID, sw)
	items := pg.perService[typ][id]
	row := exploreRowView{ID: typ + "_" + strconv.Itoa(id), Type: typ, Items: []discoverItemView{}}
	views, err := a.newViews(r, []explore.Row{{ID: row.ID, Type: typ, Items: items}}, pg, 1)
	if err != nil {
		internal(w, err)
		return
	}
	if len(views) == 1 {
		row = views[0]
	}
	var mark streamingMark
	if n, ok := pg.svcNames[id]; ok {
		mark = streamingMark{Service: id, Name: n}
	}
	// Library titles last (the Library join may know more than the page).
	sort.SliceStable(row.Items, func(i, j int) bool { return !row.Items[i].InLibrary && row.Items[j].InLibrary })
	for i := range row.Items {
		m := mark
		if typ != explore.TypeNPServices {
			m.Rank = i + 1
		}
		if m.Service != 0 {
			row.Items[i].Streaming = &m
		}
	}
	cache := "private, max-age=21600"
	if building {
		cache = "private, no-cache"
	}
	a.exploreCaching(w, r, cache)
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "service": id, "building": building, "row": row})
}
