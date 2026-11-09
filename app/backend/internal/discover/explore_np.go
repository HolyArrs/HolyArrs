package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
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
	Rows   []engine.Row
	Rail   []newRailDay
	Facts  map[string]engine.NewFact
	Scores map[string]float64
	marks  map[string]StreamingMark
	// picked says the member chose their services.
	picked bool
	// key is the page cache key (member, switch, Kid limit, region, day).
	key string
	// perService is each chip row's own list per service id (row type ->
	// service -> titles), so a chip shows that service's titles, not a
	// filter of the shared row.
	perService map[string]map[int][]engine.Item
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
	engine.Item
	What      string
	Service   *StreamingMark
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

// newHeroMax is how many titles New & Popular's hero rotates through.
const newHeroMax = 6

// newHeroKeys keeps the hero picked for a built page (page cache key ->
// kind:tmdb set), so every page of rows leaves those titles out. Entries
// expire with the page cache.
var newHeroKeys = NewTTLMap(ExplorePageTTL, 2000)

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
	show            engine.Item
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

func (hd *Handlers) newHousehold(ctx context.Context, me int64, now time.Time) newHousehold {
	h := newHousehold{genres: map[string]int{}, topGenre: map[string]string{}, follows: map[string]bool{}, owned: map[string]bool{}}
	db := hd.Store.Read()
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
		ORDER BY i.air_date DESC, t.id LIMIT 400`, database.FormatTime(now.AddDate(0, 0, -14)), database.FormatTime(now.AddDate(0, 0, 8))); err == nil {
		for rows.Next() {
			var e newEpisode
			var genres, artwork, at string
			if rows.Scan(&e.show.TMDBID, &e.show.Name, &e.show.Year, &genres, &artwork, &e.show.Backdrop, &at, &e.season, &e.episode) != nil {
				continue
			}
			_ = json.Unmarshal([]byte(genres), &e.show.Genres)
			var art []string
			_ = json.Unmarshal([]byte(artwork), &art)
			if links := ArtworkLinks(art); len(links) > 0 {
				e.show.Poster = links[0]
			}
			e.show.Kind, e.show.Owned = "show", true
			if e.at = engine.ParseDay(at); !e.at.IsZero() {
				h.episodes = append(h.episodes, e)
			}
		}
		rows.Close()
	}
	return h
}

// suit is how much a title suits the household: its genres' share of the
// Library, plus a lot for a Library title or one the member follows.
func (h newHousehold) suit(it engine.Item) float64 {
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
func (hd *Handlers) newServiceLists(ctx context.Context, h newHousehold, kinds map[string]bool) ([]StreamingMark, map[int][]engine.Item, map[string]time.Time, metadata.ServiceTop) {
	added := map[string]time.Time{}
	by := map[int][]engine.Item{}
	var order []StreamingMark
	if hd.ServiceTop == nil || hd.ServiceTop() == nil || h.region == "" {
		return nil, by, added, metadata.ServiceTop{}
	}
	list, err := hd.cachedServiceTop(ctx, h.region)
	if err != nil {
		return nil, by, added, metadata.ServiceTop{}
	}
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
	for _, sv := range list.Services {
		id := ServiceTopProviders[sv.Service]
		if id == 0 || (len(picked) > 0 && !picked[id]) {
			continue
		}
		order = append(order, StreamingMark{Service: id, Name: sv.Name})
		for _, it := range sv.New {
			kind := ServiceKind(it.Kind)
			if it.TMDBID <= 0 || !kinds[kind] {
				continue
			}
			e := engine.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				e.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			if at := engine.ParseDay(it.AddedAt); !at.IsZero() && at.After(added[e.Key()]) {
				added[e.Key()] = at
			}
			by[id] = append(by[id], e)
		}
	}
	return order, by, added, list
}

// newCharts are today's top titles of kind on each of the member's services
// (Streaming Availability's own charts), in rank order.
func newCharts(list metadata.ServiceTop, h newHousehold, kind string) ([]StreamingMark, [][]engine.Item) {
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
	var marks []StreamingMark
	var lists [][]engine.Item
	for _, sv := range list.Services {
		id := ServiceTopProviders[sv.Service]
		if id == 0 || sv.Source != "streaming-availability" || (len(picked) > 0 && !picked[id]) {
			continue
		}
		var l []engine.Item
		for _, it := range sv.Items {
			if it.TMDBID <= 0 || ServiceKind(it.Kind) != kind {
				continue
			}
			e := engine.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				e.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			l = append(l, e)
		}
		marks = append(marks, StreamingMark{Service: id, Name: sv.Name})
		lists = append(lists, l)
	}
	return marks, lists
}

// newLists names one kind's shared lists (see newRequests).
type newLists struct {
	trending, upcoming, cinema, released, episodes, series, critics, genre []engine.Item
	world                                                                  [][]engine.Item
	// onService are the TMDB top-ups per service id (watch providers,
	// newest out first).
	onService map[int][]engine.Item
}

// newRequests are one kind's shared lists: TMDB only, each kept by the list
// cache (warmed like Browse; a cold build waits newWait and says building).
func newRequests(kind, genre, region string, services []StreamingMark, now time.Time) ([]engine.Request, func([][]engine.Item) newLists) {
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	back := -45 // a movie release window
	if kind == "show" {
		back = -60 // a brand-new series window
	}
	var reqs []engine.Request
	at := map[string]int{}
	add := func(name string, q engine.Request) {
		at[name] = len(reqs)
		reqs = append(reqs, q)
	}
	add("trending", engine.Request{List: "trending"})
	add("upcoming", engine.Request{List: "upcoming"}) // shows: on the air now
	add("cinema", engine.Request{List: "now_playing", Region: region})
	if kind == "movie" {
		add("released", engine.Request{List: "discover", From: day(back), To: day(0), Types: "4|3", Region: region, MinVotes: 10})
	} else {
		// Shows with an episode out this week (a new episode or season).
		add("episodes", engine.Request{List: "discover", AirFrom: day(-6), AirTo: day(0), MinVotes: 20})
		add("series", engine.Request{List: "discover", From: day(back), To: day(0), MinVotes: 5})
	}
	add("critics", engine.Request{List: "discover", From: day(-60), To: day(0), MinVote: 7.2, MinVotes: 50, Sort: "rating"})
	for _, l := range newWorld {
		add("world:"+l, engine.Request{List: "discover", Lang: l, From: day(back), To: day(0), MinVotes: 10})
	}
	if genre != "" {
		add("genre", engine.Request{List: "discover", Genre: genre, From: day(back), To: day(0), MinVotes: 10})
	}
	for _, s := range services {
		q := engine.Request{List: "discover", Services: strconv.Itoa(s.Service), Region: region, From: day(back), To: day(0), Sort: "recent", MinVotes: 5}
		if kind == "show" {
			q = engine.Request{List: "discover", Services: strconv.Itoa(s.Service), Region: region, AirFrom: day(-6), AirTo: day(0), MinVotes: 5}
		}
		add("service:"+strconv.Itoa(s.Service), q)
	}
	for i := range reqs {
		reqs[i].Kind, reqs[i].Scope = kind, engine.ScopeShared
	}
	return reqs, func(got [][]engine.Item) newLists {
		get := func(name string) []engine.Item {
			if i, ok := at[name]; ok && i < len(got) {
				return got[i]
			}
			return nil
		}
		l := newLists{trending: get("trending"), upcoming: get("upcoming"), cinema: get("cinema"), released: get("released"), episodes: get("episodes"),
			series: get("series"), critics: get("critics"), genre: get("genre"), onService: map[int][]engine.Item{}}
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
	m   map[string]engine.NewFact
}

func (f newFacts) put(key string, n engine.NewFact) {
	if old, ok := f.m[key]; ok && engine.Freshness(old, f.now) >= engine.Freshness(n, f.now) {
		return
	}
	f.m[key] = n
}

// dated is the fact a title's own date gives: a movie's release (upcoming
// when ahead), a show's first air date (a brand-new series).
func (f newFacts) dated(it engine.Item) {
	d := engine.ParseDay(it.Released)
	if d.IsZero() {
		return
	}
	switch {
	case d.After(f.now):
		f.put(it.Key(), engine.NewFact{Kind: engine.NewUpcoming, At: d})
	case it.Kind == "movie":
		f.put(it.Key(), engine.NewFact{Kind: engine.NewRelease, At: d})
	case it.Kind == "show":
		f.put(it.Key(), engine.NewFact{Kind: engine.NewSeries, At: d})
	}
}

// buildNewPage builds the member's page (before per-person views): kept 6
// hours once every list is back; building says some lists were still out.
func (hd *Handlers) newRows(ctx context.Context, r *http.Request, me int64, sw string) (newPage, bool) {
	now := hd.Explore.Now().UTC()
	h := hd.newHousehold(ctx, me, now)
	kinds := map[string]bool{}
	for _, k := range engine.Kinds(sw) {
		if len(hd.KindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	key := strconv.FormatInt(me, 10) + "|" + sw + "|" + hd.KidMax(r) + "|" + h.region + "|" + now.Format("2006-01-02")
	if p, ok := newPages.get(key); ok {
		return p, false
	}
	order, svcNew, added, top := hd.newServiceLists(ctx, h, kinds)
	pg := newPage{Facts: map[string]engine.NewFact{}, Scores: map[string]float64{}, marks: map[string]StreamingMark{}, picked: len(h.services) > 0, key: key,
		perService: map[string]map[int][]engine.Item{engine.TypeNPServices: {}, engine.TypeNPTopShows: {}, engine.TypeNPTopFilms: {}}, svcNames: map[int]string{}}
	for _, s := range order {
		pg.svcNames[s.Service] = s.Name
	}
	facts := newFacts{now: now, m: pg.Facts}
	for k, at := range added {
		facts.put(k, engine.NewFact{Kind: engine.NewService, At: at})
	}
	ready := true
	lists := map[string]newLists{}
	for _, k := range []string{"movie", "show"} {
		if !kinds[k] {
			continue
		}
		reqs, name := newRequests(k, h.topGenre[k], h.region, order, now)
		g, ok := hd.GenreLists(ctx, k, reqs, newWait)
		lists[k], ready = name(g), ready && ok
	}
	// Every TMDB title's own date, then the stronger facts on top.
	var pool []engine.Item
	for _, l := range lists {
		all := [][]engine.Item{l.trending, l.upcoming, l.cinema, l.released, l.episodes, l.series, l.critics, l.genre}
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
			facts.put(it.Key(), engine.NewFact{Kind: engine.NewEpisode, At: now.AddDate(0, 0, -3)})
		}
	}
	for _, e := range h.episodes {
		if e.at.After(now) {
			continue
		}
		f := engine.NewFact{Kind: engine.NewEpisode, At: e.at}
		if e.episode == 1 && e.season > 1 {
			f = engine.NewFact{Kind: engine.NewSeason, At: e.at, Season: e.season}
		} else if e.episode == 1 && e.season == 1 {
			f = engine.NewFact{Kind: engine.NewSeries, At: e.at}
		}
		facts.put(e.show.Key(), f)
		pool = append(pool, e.show)
	}
	pop := engine.PopPercentile(pool)
	maxSuit := 0.0
	for _, it := range pool {
		maxSuit = max(maxSuit, h.suit(it))
	}
	score := func(it engine.Item) float64 {
		if s, ok := pg.Scores[it.Key()]; ok {
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
		s := engine.NewScore(engine.Freshness(pg.Facts[it.Key()], now), p, t)
		pg.Scores[it.Key()] = s
		return s
	}
	fresh := func(items []engine.Item) []engine.Item {
		out := []engine.Item{}
		for _, it := range items {
			if kinds[it.Kind] && score(it) > 0 && pg.Facts[it.Key()].Kind != engine.NewUpcoming {
				out = append(out, it)
			}
		}
		return out
	}
	ranked := func(items []engine.Item) []engine.Item {
		out := fresh(items)
		engine.SortByScore(out, score)
		return out
	}
	both := func(get func(newLists) []engine.Item) []engine.Item {
		var all []engine.Item
		for _, k := range []string{"show", "movie"} {
			if l, ok := lists[k]; ok {
				all = append(all, get(l)...)
			}
		}
		return ranked(all)
	}

	var rows []engine.Row
	addRow := func(id, typ, title string, items []engine.Item) {
		rows = append(rows, engine.Row{ID: id, Type: typ, Title: title, Items: items})
	}
	// Fresh on your services: each service's adds, topped up to about 20 from
	// TMDB's newest on it; Library titles after the rest.
	var perService [][]engine.Item
	for _, s := range order {
		var items []engine.Item
		items = append(items, svcNew[s.Service]...)
		for _, k := range []string{"show", "movie"} {
			items = append(items, lists[k].onService[s.Service]...)
		}
		items = ranked(items)
		sort.SliceStable(items, func(i, j int) bool { return !h.owned[items[i].Key()] && h.owned[items[j].Key()] })
		seen := map[string]bool{}
		var l []engine.Item
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
		pg.perService[engine.TypeNPServices][s.Service] = l
	}
	services := engine.RankMerge(perService, engine.NewRowCap[engine.TypeNPServices])
	// The top of the page, in this order: the Top 10 new shows and movies
	// (each service's real chart, new titles only, topped up from the scored
	// pool), what everyone is watching (the same charts past their top 10,
	// shows and movies together), then the critics' best new releases. A
	// title shows once: the first row it is in keeps it.
	var watching [][]engine.Item
	for _, k := range []string{"show", "movie"} {
		if !kinds[k] {
			continue
		}
		marks, charts := newCharts(top, h, k)
		typ := engine.TypeNPTopFilms
		if k == "show" {
			typ = engine.TypeNPTopShows
		}
		var lists2 [][]engine.Item
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
			watching = append(watching, l)
			pg.perService[typ][marks[i].Service] = l
		}
		hot := engine.RankMerge(lists2, 10)
		have := map[string]bool{}
		for _, it := range hot {
			have[it.Key()] = true
		}
		var rest []engine.Item
		for _, it := range pool {
			if it.Kind == k && !have[it.Key()] {
				have[it.Key()] = true
				rest = append(rest, it)
			}
		}
		hot = append(hot, ranked(rest)...)
		if k == "show" {
			addRow("np_top_shows", engine.TypeNPTopShows, "Top 10 new shows", hot)
		} else {
			addRow("np_top_movies", engine.TypeNPTopFilms, "Top 10 new movies", hot)
		}
	}
	addRow("np_watching", engine.TypeNPWatching, "Everyone's watching", engine.RankMerge(watching, 3*engine.RowMax))
	addRow("np_acclaimed", engine.TypeNPAcclaim, "Critically acclaimed new releases", both(func(l newLists) []engine.Item { return l.critics }))
	addRow("np_services", engine.TypeNPServices, "Fresh on your services", services)
	if kinds["show"] {
		var mine []engine.Item
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
		addRow("np_episodes", engine.TypeNPEpisodes, "Your shows, new episodes", ranked(mine))
	}
	var picks []engine.Item
	for _, it := range ranked(pool) {
		if h.suit(it) > 0 && !h.owned[it.Key()] {
			picks = append(picks, it)
		}
	}
	addRow("np_picks", engine.TypeNPPicks, "Made for you, just out", picks)
	if kinds["show"] {
		var first []engine.Item
		for _, it := range ranked(pool) {
			if it.Kind == "show" && pg.Facts[it.Key()].Kind == engine.NewSeries {
				first = append(first, it)
			}
		}
		addRow("np_series", engine.TypeNPSeries, "First seasons", first)
	}
	if kinds["movie"] {
		addRow("np_cinema", engine.TypeNPCinema, "In cinemas now", ranked(lists["movie"].cinema))
	}
	if g := h.topGenre["show"]; g != "" || h.topGenre["movie"] != "" {
		if g == "" || (kinds["movie"] && !kinds["show"]) {
			g = h.topGenre["movie"]
		}
		addRow("np_genre", engine.TypeNPGenre, "Just out in "+strings.ToLower(g), both(func(l newLists) []engine.Item { return l.genre }))
	}
	addRow("np_world", engine.TypeNPWorld, "New from abroad", both(func(l newLists) []engine.Item {
		var all []engine.Item
		for _, w := range l.world {
			all = append(all, w...)
		}
		return all
	}))
	weight := map[string]float64{engine.TypeNPEpisodes: 1.3, engine.TypeNPPicks: 1.2, engine.TypeNPSeries: 1, engine.TypeNPGenre: 1,
		engine.TypeNPCinema: 0.9, engine.TypeNPAcclaim: 0.8, engine.TypeNPWorld: 0.6}
	pg.Rows = engine.OrderNewRows(engine.DedupeRows(rows, engine.NewRowMin), score, weight)
	pg.Rail = newRail(h, lists, svcNew, order, pg, kinds, score, now)
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
func newRail(h newHousehold, lists map[string]newLists, svcNew map[int][]engine.Item, order []StreamingMark, pg newPage, kinds map[string]bool, score func(engine.Item) float64, now time.Time) []newRailDay {
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
	names := map[int]StreamingMark{}
	for _, s := range order {
		names[s.Service] = s
	}
	for _, s := range order {
		id := s.Service
		for _, it := range svcNew[id] {
			f := pg.Facts[it.Key()]
			if f.Kind == engine.NewService {
				m := names[id]
				put(f.At, newRailItem{Item: it, What: "On " + m.Name, Service: &m})
			}
		}
	}
	if l, ok := lists["movie"]; ok {
		for _, items := range [][]engine.Item{l.released, l.cinema, l.trending, l.upcoming} {
			for _, it := range items {
				d := engine.ParseDay(it.Released)
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
