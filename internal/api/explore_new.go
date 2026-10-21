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

// newPageTTL is how long a member's built New & Popular rows are kept; the
// shared lists under them keep their own TTLs (6 to 24 hours).
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

// newWorld are the original languages of "From around the world".
var newWorld = []string{"ko", "ja", "es"}

type newPageEntry struct {
	at   time.Time
	rows []explore.Row
}

type newPageCache struct {
	mu   sync.Mutex
	rows map[string]newPageEntry
}

var newPages newPageCache

func (c *newPageCache) get(key string) ([]explore.Row, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.rows[key]
	if !ok || time.Since(e.at) > newPageTTL {
		return nil, false
	}
	return e.rows, true
}

func (c *newPageCache) put(key string, rows []explore.Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil || len(c.rows) > 500 {
		c.rows = map[string]newPageEntry{}
	}
	c.rows[key] = newPageEntry{at: time.Now(), rows: rows}
}

// newHousehold is what New & Popular reads about the household (local).
type newHousehold struct {
	services []int
	region   string
	genres   map[string]int // Library titles per genre
	topGenre map[string]string
	people   []string
	follows  map[string]bool // kind:tmdb the member follows
	owned    map[string]bool
	back     []explore.Item // Library shows with an episode out in 14 days
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
	people := map[string]int{}
	kindGenre := map[string]map[string]int{"movie": {}, "show": {}}
	if rows, err := db.QueryContext(ctx, `SELECT kind, json_extract(source_ids, '$.tmdb'), genres, people FROM titles
		WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL`); err == nil {
		for rows.Next() {
			var kind, id, genres, ppl string
			if rows.Scan(&kind, &id, &genres, &ppl) != nil {
				continue
			}
			h.owned[kind+":"+id] = true
			var gs, ps []string
			_ = json.Unmarshal([]byte(genres), &gs)
			_ = json.Unmarshal([]byte(ppl), &ps)
			for _, g := range gs {
				h.genres[g]++
				kindGenre[kind][g]++
			}
			for _, p := range ps[:min(4, len(ps))] {
				people[p]++
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
	for p, n := range people {
		if n >= 2 {
			h.people = append(h.people, p)
		}
	}
	sort.Slice(h.people, func(i, j int) bool {
		if people[h.people[i]] != people[h.people[j]] {
			return people[h.people[i]] > people[h.people[j]]
		}
		return h.people[i] < h.people[j]
	})
	h.people = h.people[:min(3, len(h.people))]
	if rows, err := db.QueryContext(ctx, `SELECT 'show', tmdb_id FROM tmdb_follows WHERE user_id = ?
		UNION SELECT t.kind, json_extract(t.source_ids, '$.tmdb') FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = ?`, me, me); err == nil {
		for rows.Next() {
			var kind string
			var id *string
			if rows.Scan(&kind, &id) == nil && id != nil {
				h.follows[kind+":"+*id] = true
			}
		}
		rows.Close()
	}
	// Library shows with an episode out in the last 14 days, newest first.
	if rows, err := db.QueryContext(ctx, `SELECT json_extract(t.source_ids, '$.tmdb'), t.name, COALESCE(t.year, 0), t.genres, t.artwork, t.backdrop, MAX(i.air_date) AS last
		FROM titles t JOIN items i ON i.title_id = t.id AND i.kind = 'episode' AND i.dropped_at IS NULL AND i.air_date >= ? AND i.air_date <= ?
		WHERE t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') IS NOT NULL
		GROUP BY t.id ORDER BY last DESC, t.id LIMIT 40`, store.FormatTime(now.AddDate(0, 0, -14)), store.FormatTime(now)); err == nil {
		for rows.Next() {
			var it explore.Item
			var genres, artwork, last string
			if rows.Scan(&it.TMDBID, &it.Name, &it.Year, &genres, &artwork, &it.Backdrop, &last) != nil {
				continue
			}
			_ = json.Unmarshal([]byte(genres), &it.Genres)
			var art []string
			_ = json.Unmarshal([]byte(artwork), &art)
			if links := artworkLinks(art); len(links) > 0 {
				it.Poster = links[0]
			}
			it.Kind, it.Owned = "show", true
			h.back = append(h.back, it)
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

// newServiceItems are the titles newly added to the member's services in
// their region (every Streaming Availability service when none is picked),
// newest first, each with its service; from the kept lists (never fetched
// here beyond the 12-hour service-top read).
func (a *API) newServiceItems(ctx context.Context, h newHousehold, kinds map[string]bool) ([]explore.Item, map[string]streamingMark) {
	marks := map[string]streamingMark{}
	if a.s.ServiceTop == nil || h.region == "" {
		return nil, marks
	}
	list, err := a.cachedServiceTop(ctx, h.region)
	if err != nil {
		return nil, marks
	}
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
	var lists [][]explore.Item
	for _, sv := range list.Services {
		id := serviceTopProviders[sv.Service]
		if id == 0 || (len(picked) > 0 && !picked[id]) {
			continue
		}
		var l []explore.Item
		for _, it := range sv.New {
			kind := serviceKind(it.Kind)
			if it.TMDBID <= 0 || !kinds[kind] {
				continue
			}
			e := explore.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				e.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			if _, ok := marks[e.Key()]; !ok {
				marks[e.Key()] = streamingMark{Service: id, Name: sv.Name}
			}
			l = append(l, e)
		}
		lists = append(lists, l)
	}
	return explore.RankMerge(lists, explore.RowMax*2), marks
}

// newTopItems is today's top titles of kind across the member's services
// (their own top lists only), taking turns by rank.
func (a *API) newTopItems(ctx context.Context, h newHousehold, kind string) []explore.Item {
	if a.s.ServiceTop == nil || h.region == "" {
		return nil
	}
	list, err := a.cachedServiceTop(ctx, h.region)
	if err != nil {
		return nil
	}
	picked := map[int]bool{}
	for _, s := range h.services {
		picked[s] = true
	}
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
		lists = append(lists, l)
	}
	return explore.RankMerge(lists, 10)
}

// newRequests are one kind's shared lists, in a fixed order (see newRows).
func newRequests(kind, genre, region string, now time.Time) []explore.Request {
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	reqs := []explore.Request{
		{List: "trending"},
		{List: "upcoming"}, // shows: on the air now
		{List: "now_playing", Region: region},
		{List: "discover", From: day(-60), To: day(0), MinVotes: 20},
		{List: "discover", From: day(-120), To: day(0), MinVote: 7.2, MinVotes: 100, Sort: "rating"},
	}
	for _, l := range newWorld {
		reqs = append(reqs, explore.Request{List: "discover", Lang: l, From: day(-90), To: day(0), MinVotes: 10})
	}
	if genre != "" {
		reqs = append(reqs, explore.Request{List: "discover", Genre: genre, From: day(-90), To: day(0), MinVotes: 10})
	}
	for i := range reqs {
		reqs[i].Kind, reqs[i].Scope = kind, explore.ScopeShared
	}
	return reqs
}

// newRows builds the member's rows (before per-person views): kept 6
// hours once every list is back; building says some lists were still out.
func (a *API) newRows(ctx context.Context, r *http.Request, me int64, sw string) ([]explore.Row, map[string]streamingMark, bool) {
	now := a.exploreNow().UTC()
	h := a.newHousehold(ctx, me, now)
	kinds := map[string]bool{}
	for _, k := range explore.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	svc, marks := a.newServiceItems(ctx, h, kinds)
	key := strconv.FormatInt(me, 10) + "|" + sw + "|" + a.kidMax(r) + "|" + h.region + "|" + now.Format("2006-01-02")
	if rows, ok := newPages.get(key); ok {
		return rows, marks, false
	}
	var rows []explore.Row
	add := func(id, typ, title string, items []explore.Item) {
		rows = append(rows, explore.Row{ID: id, Type: typ, Title: title, Items: items})
	}
	add("np_services", explore.TypeNPServices, "Just landed on your services", svc)
	if kinds["show"] {
		add("np_top_shows", explore.TypeNPTopShows, "Today's top 10 shows", a.newTopItems(ctx, h, "show"))
	}
	if kinds["movie"] {
		add("np_top_movies", explore.TypeNPTopFilms, "Today's top 10 movies", a.newTopItems(ctx, h, "movie"))
	}
	ready := true
	got := map[string][][]explore.Item{}
	for _, k := range []string{"movie", "show"} {
		if !kinds[k] {
			continue
		}
		g, ok := a.genreLists(ctx, k, newRequests(k, h.topGenre[k], h.region, now), newWait)
		got[k], ready = g, ready && ok
	}
	var people [][]explore.Item
	if len(h.people) > 0 {
		var reqs []explore.Request
		for _, p := range h.people {
			reqs = append(reqs, explore.Request{List: "person", Ref: p})
		}
		g, ok := a.genreLists(ctx, "", reqs, newWait)
		people, ready = g, ready && ok
	}
	both := func(i int) []explore.Item {
		var ls [][]explore.Item
		for _, k := range []string{"movie", "show"} {
			if g := got[k]; i < len(g) {
				ls = append(ls, g[i])
			}
		}
		return explore.Interleave(ls, explore.RowMax*2)
	}
	fresh := func(items []explore.Item) []explore.Item { return explore.FilterNew(items, now) }
	if kinds["show"] {
		// Library and followed shows first: back with new episodes.
		var mine, rest []explore.Item
		if g := got["show"]; len(g) > 1 {
			for _, it := range g[1] {
				if h.owned[it.Key()] || h.follows[it.Key()] {
					mine = append(mine, it)
				} else {
					rest = append(rest, it)
				}
			}
		}
		add("np_returning", explore.TypeNPReturn, "Back with new episodes", append(append([]explore.Item{}, h.back...), mine...))
		add("np_episodes", explore.TypeNPEpisodes, "New episodes this fortnight", append(mine, rest...))
		if g := got["show"]; len(g) > 3 {
			add("np_series", explore.TypeNPSeries, "Brand-new series", fresh(g[3]))
		}
	}
	if kinds["movie"] {
		if g := got["movie"]; len(g) > 3 {
			add("np_cinema", explore.TypeNPCinema, "Now in cinemas", g[2])
			add("np_released", explore.TypeNPReleased, "Just released", fresh(g[3]))
		}
	}
	// Fresh picks: every new title above, by how much it suits the household.
	var pool []explore.Item
	for _, r := range rows {
		pool = append(pool, r.Items...)
	}
	for k := range kinds {
		for _, l := range got[k] {
			pool = append(pool, fresh(l)...)
		}
	}
	picks := append([]explore.Item{}, pool...)
	sort.SliceStable(picks, func(i, j int) bool { return h.suit(picks[i]) > h.suit(picks[j]) })
	var liked []explore.Item
	for _, it := range picks {
		if h.suit(it) > 0 && !h.owned[it.Key()] {
			liked = append(liked, it)
		}
	}
	add("np_picks", explore.TypeNPPicks, "Fresh picks for you", liked)
	if len(h.genres) > 0 {
		var ls [][]explore.Item
		name := ""
		for _, k := range []string{"movie", "show"} {
			if g := got[k]; kinds[k] && h.topGenre[k] != "" && len(g) > 5+len(newWorld) {
				ls = append(ls, fresh(g[5+len(newWorld)]))
				if name == "" {
					name = h.topGenre[k]
				}
			}
		}
		if name != "" {
			add("np_genre", explore.TypeNPGenre, "New in "+strings.ToLower(name), explore.Interleave(ls, explore.RowMax*2))
		}
	}
	var mine []explore.Item
	for _, l := range people {
		for _, it := range fresh(l) {
			if kinds[it.Kind] {
				mine = append(mine, it)
			}
		}
	}
	add("np_people", explore.TypeNPPeople, "New from names in your Library", mine)
	add("np_week", explore.TypeNPWeek, "Everyone's watching this week", both(0))
	add("np_acclaimed", explore.TypeNPAcclaim, "Acclaimed and new", both(4))
	var world [][]explore.Item
	for i := range newWorld {
		world = append(world, both(5+i))
	}
	add("np_world", explore.TypeNPWorld, "Fresh from around the world", explore.Interleave(world, explore.RowMax*2))
	weight := map[string]float64{explore.TypeNPReturn: 0.9, explore.TypeNPPicks: 0.6, explore.TypeNPEpisodes: 0.5, explore.TypeNPCinema: 0.4,
		explore.TypeNPReleased: 0.35, explore.TypeNPGenre: 0.35, explore.TypeNPPeople: 0.3, explore.TypeNPSeries: 0.3, explore.TypeNPWeek: 0.25, explore.TypeNPAcclaim: 0.2, explore.TypeNPWorld: 0.1}
	rows = explore.OrderNewRows(explore.DedupeRows(rows, explore.NewRowMin), h.suit, weight)
	if ready {
		newPages.put(key, rows)
	}
	return rows, marks, !ready
}

// newViews joins rows (Kid limit, never-add, Library) and drops rows left
// under six; the services row's cards carry their service.
func (a *API) newViews(r *http.Request, rows []explore.Row, marks map[string]streamingMark) ([]exploreRowView, error) {
	views, err := a.exploreViews(r.Context(), r, rows)
	if err != nil {
		return nil, err
	}
	out := []exploreRowView{}
	for _, v := range views {
		if len(v.Items) < explore.NewRowMin {
			continue
		}
		a.fillTrailerKeys(r, v.Items)
		out = append(out, v)
	}
	a.fillRowsCardArt(r, out)
	a.fillStreaming(r, out)
	for i := range out {
		if out[i].Type != explore.TypeNPServices {
			continue
		}
		for j := range out[i].Items {
			it := &out[i].Items[j]
			if m, ok := marks[it.Kind+":"+it.TMDBID]; ok {
				it.Streaming = &m
			}
		}
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
// New & Popular, a page of its rows (5 first, then 4 a page); more says
// another page follows, building that some lists are still on their way
// (ask again shortly).
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
	rows, marks, building := a.newRows(r.Context(), r, a.person(r).UserID, sw)
	from, to := 0, newFirstRows
	if n > 0 {
		from = newFirstRows + (n-1)*newPageRows
		to = from + newPageRows
	}
	from, to = min(from, len(rows)), min(to, len(rows))
	views, err := a.newViews(r, rows[from:to], marks)
	if err != nil {
		internal(w, err)
		return
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "page": n, "more": to < len(rows), "building": building, "rows": views})
}

// exploreNewServices answers GET /api/v1/explore/new/services?type=: the
// "Just landed on your services" row alone, for Explore (kept lists only;
// the page leaves out titles it already shows).
func (a *API) exploreNewServices(w http.ResponseWriter, r *http.Request) {
	sw, ok := a.newParams(w, r)
	if !ok {
		return
	}
	now := a.exploreNow().UTC()
	h := a.newHousehold(r.Context(), a.person(r).UserID, now)
	kinds := map[string]bool{}
	for _, k := range explore.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	items, marks := a.newServiceItems(r.Context(), h, kinds)
	views, err := a.exploreViews(r.Context(), r, []explore.Row{{ID: "np_services", Type: explore.TypeNPServices, Title: "New on your services", Items: items}})
	if err != nil {
		internal(w, err)
		return
	}
	row := exploreRowView{ID: "np_services", Type: explore.TypeNPServices, Title: "New on your services", Items: []discoverItemView{}}
	if len(views) == 1 {
		row = views[0]
		a.fillTrailerKeys(r, row.Items)
		rs := []exploreRowView{row}
		a.fillRowsCardArt(r, rs)
		row = rs[0]
		for j := range row.Items {
			it := &row.Items[j]
			if m, ok := marks[it.Kind+":"+it.TMDBID]; ok {
				it.Streaming = &m
			}
		}
	}
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "row": row})
}
