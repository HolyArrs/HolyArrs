package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ratings"
)

// netflixService is Netflix's TMDB watch-provider id; its chip uses
// Netflix's own weekly Top 10 instead of TMDB popularity.
const netflixService = "8"

// topServicesMax is how many ranked titles the row shows.
const topServicesMax = 10

// genreMoodMin is the fewest titles a genre page's mood row needs.
const genreMoodMin = 8

// liveSource is the Explore list source with outside calls allowed (lists
// are cached by their own TTL).
func (a *API) liveSource(ctx context.Context) exploreSource {
	var lister metadata.ExploreLister
	if l, ok := metadata.ExploreListerFor(ctx, a.s.Metadata); ok {
		lister = l
	}
	return exploreSource{a: a, lister: lister, calls: &explore.Calls{}}
}

// exploreGenreParam reads ?genre= for kind: the TMDB genre id and name.
func exploreGenreParam(v, kind string) (int, string, bool) {
	if v == "" {
		return 0, "", true
	}
	id, err := strconv.Atoi(v)
	if err != nil || explore.GenreName(kind, id) == "" {
		return 0, "", false
	}
	return id, explore.GenreName(kind, id), true
}

// exploreTop answers GET /api/v1/explore/top?type=&chip=all|<service id>
// &services=&region=&genre=: the ranked "Top on your services" row for one
// chip. Netflix (no genre) is Netflix's weekly Top 10; any other service,
// and All my services, is TMDB's most popular on it in the region, kept a
// week, merged by popularity with each title once.
func (a *API) exploreTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := exploreSwitch(q.Get("type"))
	if !ok {
		fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	chip, services, region := q.Get("chip"), q.Get("services"), strings.ToUpper(q.Get("region"))
	if chip == "" {
		chip = "all"
	}
	if (chip != "all" && !metadata.ServiceIDs(chip)) || strings.Contains(chip, "|") || (services != "" && !metadata.ServiceIDs(services)) || !isCountry(region) {
		fail(w, http.StatusBadRequest, "chip must be all or one service id, services ids joined by |, region a country code")
		return
	}
	var kinds []string
	for _, k := range explore.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds = append(kinds, k)
		}
	}
	genre := ""
	if g := q.Get("genre"); g != "" {
		if sw == "all" {
			fail(w, http.StatusBadRequest, "genre needs type movies or shows")
			return
		}
		var gok bool
		if _, genre, gok = exploreGenreParam(g, kinds0(kinds, sw)); !gok {
			fail(w, http.StatusBadRequest, "genre must be a TMDB genre id of that type")
			return
		}
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	providers := chip
	if chip == "all" {
		providers = services
	}
	out := map[string]any{"chip": chip, "region": region, "source": "popular", "items": []top10View{}}
	if chip == netflixService && genre == "" && a.s.Top10 != nil {
		if items, week, ok := a.netflixTop(r, kinds, region); ok {
			out["source"], out["week"], out["items"] = "netflix_top10", week, items
			a.exploreCaching(w, r, "private, max-age=300")
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	if providers == "" || len(kinds) == 0 {
		a.exploreCaching(w, r, "private, max-age=300")
		writeJSON(w, http.StatusOK, out)
		return
	}
	src := a.liveSource(r.Context())
	var lists [][]explore.Item
	for _, k := range kinds {
		items, err := src.List(r.Context(), explore.Request{List: "weekly", Kind: k, Services: providers, Region: region, Genre: genre, Scope: explore.ScopeShared})
		if err != nil {
			continue
		}
		lists = append(lists, items)
	}
	// A few spare titles cover ones the Kid limit or never-add drops.
	merged := explore.MergeTop(lists, topServicesMax*2)
	views, err := a.exploreViews(r.Context(), r, []explore.Row{{ID: explore.TypeTopServices, Type: explore.TypeTopServices, Items: merged}})
	if err != nil {
		internal(w, err)
		return
	}
	items := []top10View{}
	if len(views) == 1 {
		for i, v := range views[0].Items {
			if i == topServicesMax {
				break
			}
			items = append(items, top10View{discoverItemView: v, Rank: i + 1})
		}
	}
	a.fillTopArt(r, items)
	out["items"] = items
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, out)
}

func kinds0(kinds []string, sw string) string {
	if len(kinds) > 0 {
		return kinds[0]
	}
	return explore.Kinds(sw)[0]
}

// netflixTop is Netflix's weekly Top 10 for the kinds (the region's list,
// else worldwide); two kinds take turns by rank and are numbered again.
func (a *API) netflixTop(r *http.Request, kinds []string, region string) ([]top10View, string, bool) {
	var lists [][]top10View
	week := ""
	for _, k := range kinds {
		for _, reg := range []string{region, "global"} {
			lang := ""
			if reg == "global" {
				lang = "en"
			}
			status, _, body := a.top10Answer(r.Context(), r, k, reg, lang)
			if status != http.StatusOK {
				continue
			}
			var got []top10View
			for _, v := range body["items"].([]top10View) {
				if v.TMDBID != "" {
					got = append(got, v)
				}
			}
			if len(got) > 0 {
				lists = append(lists, got)
				week, _ = body["week"].(string)
				break
			}
		}
	}
	if len(lists) == 0 {
		return nil, "", false
	}
	out := []top10View{}
	seen := map[string]bool{}
	for i := 0; len(out) < topServicesMax; i++ {
		added := false
		for _, l := range lists {
			if i < len(l) && len(out) < topServicesMax {
				added = true
				if seen[l[i].Kind+l[i].TMDBID] {
					continue
				}
				seen[l[i].Kind+l[i].TMDBID] = true
				v := l[i]
				v.Rank = len(out) + 1
				out = append(out, v)
			}
		}
		if !added {
			break
		}
	}
	return out, week, true
}

// fillTopArt adds trailer keys and card art to ranked items.
func (a *API) fillTopArt(r *http.Request, items []top10View) {
	views := make([]discoverItemView, len(items))
	for i := range items {
		views[i] = items[i].discoverItemView
	}
	a.fillTrailerKeys(r, views)
	a.fillCardArt(r, views)
	for i := range items {
		items[i].discoverItemView = views[i]
	}
}

// exploreGenre answers GET /api/v1/explore/genre?type=movies|shows&genre=<id>:
// a genre page. The hero comes from the genre's trending titles; then
// Trending <genre>, Best new <genre>, Because you have <a Library title in
// the genre>, Top-rated <genre> classics and the genre's mood rows (each
// only with 8 or more titles). The page puts Top on your services, in the
// genre, second.
func (a *API) exploreGenre(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := exploreSwitch(q.Get("type"))
	if !ok {
		fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	// Under All a genre is a movie genre id that TV has a combined genre
	// for; the page merges the movie page and the show page.
	kind := explore.Kinds(sw)[0]
	if sw == "all" {
		kind = "movie"
	}
	id, genre, ok := exploreGenreParam(q.Get("genre"), kind)
	if !ok || genre == "" || (sw == "all" && explore.ShowGenreFor(genre) == "") {
		fail(w, http.StatusBadRequest, "genre must be a TMDB genre id of that type")
		return
	}
	for _, k := range explore.Kinds(sw) {
		if len(a.kindsOn(k)) == 0 {
			fail(w, http.StatusNotFound, "that part is off")
			return
		}
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	st := newServerTiming()
	ctx := r.Context()
	rows, partial := a.genreBuild(ctx, r, kind, genre)
	if sw == "all" {
		more, p2 := a.genreBuild(ctx, r, "show", explore.ShowGenreFor(genre))
		rows, partial = mergeGenreRows(rows, more), partial || p2
	}
	st.mark("lists")
	// A title once per page, first row wins.
	seen := map[string]bool{}
	for i := range rows {
		var keep []explore.Item
		for _, it := range rows[i].Items {
			if !seen[it.Key()] && len(keep) < explore.RowMax {
				keep = append(keep, it)
			}
		}
		for _, it := range keep {
			seen[it.Key()] = true
		}
		rows[i].Items = keep
	}
	views, err := a.exploreViews(ctx, r, rows)
	if err != nil {
		internal(w, err)
		return
	}
	st.mark("views")
	hero := []discoverItemView{}
	if len(views) > 0 && views[0].ID == "genre_trending" {
		for _, v := range views[0].Items {
			if !v.InLibrary && len(hero) < 6 {
				hero = append(hero, v)
			}
		}
	}
	a.fillTrailerKeys(r, hero)
	a.fillCardArt(r, hero)
	for i := range views {
		a.fillTrailerKeys(r, views[i].Items)
	}
	st.mark("trailers")
	a.fillRowsCardArt(r, views)
	st.mark("cardart")
	st.write(w)
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "genre": map[string]any{"id": id, "name": genre, "kind": kind}, "hero": hero, "rows": views, "partial": partial})
}

// genreBuild is one kind's genre page rows (before per-person views): from
// the memory cache, else built from the genre's lists. partial says some
// lists were still out.
func (a *API) genreBuild(ctx context.Context, r *http.Request, kind, genre string) ([]explore.Row, bool) {
	inGenre := func(items []explore.Item) []explore.Item {
		var out []explore.Item
		for _, it := range items {
			if explore.HasGenre(it, genre) {
				out = append(out, it)
			}
		}
		return out
	}
	now := a.exploreNow().UTC()
	lower := strings.ToLower(genre)
	seed, hasSeed := a.genreSeed(ctx, r, kind, genre)
	cacheKey := kind + "|" + genre + "|" + a.kidMax(r) + "|" + seed.TMDBID + "|" + now.Format("2006-01-02")
	if rows, cached := a.genreRows.get(cacheKey); cached {
		return rows, false
	}
	var rows []explore.Row
	reqs := genreRequests(kind, genre, now)
	moods := metadata.SubMoods(genre)
	if hasSeed {
		reqs = append(reqs, explore.Request{List: "similar", Ref: seed.TMDBID})
	}
	got, ready := a.genreLists(ctx, kind, reqs, genreWait)
	trending := explore.Interleave([][]explore.Item{inGenre(got[0]), got[1]}, explore.RowMax)
	rows = append(rows, explore.Row{ID: "genre_trending", Type: explore.TypeTrending, Title: "Trending " + lower, Items: trending})
	rows = append(rows, explore.Row{ID: "genre_best", Type: explore.TypeBest, Title: "Best new " + lower, Items: got[2]})
	if hasSeed {
		rows = append(rows, explore.Row{ID: "genre_because", Type: explore.TypeBecauseHave, Title: "Because you have " + seed.Name,
			Items: inGenre(got[len(got)-1])})
	}
	rows = append(rows, explore.Row{ID: "genre_classics", Type: explore.TypeClassics, Title: "Top-rated " + lower + " classics", Items: got[3]})
	for i, m := range moods {
		rows = append(rows, explore.Row{ID: "genre_mood_" + strings.ReplaceAll(strings.ToLower(m.Label), " ", "_"), Type: explore.TypeMood, Title: m.Label,
			Items: got[4+i]})
	}
	if ready {
		a.genreRows.put(cacheKey, rows)
	}
	return rows, !ready
}

// mergeGenreRows folds the show page's rows into the movie page's: a row
// both have takes turns between the two kinds; the rest follow. The
// "Because you have" row keeps the first seed's.
func mergeGenreRows(movie, show []explore.Row) []explore.Row {
	out := append([]explore.Row{}, movie...)
	at := map[string]int{}
	for i, r := range out {
		at[r.ID] = i
	}
	for _, r := range show {
		i, ok := at[r.ID]
		switch {
		case !ok:
			at[r.ID] = len(out)
			out = append(out, r)
		case r.ID != "genre_because":
			out[i].Items = explore.Interleave([][]explore.Item{out[i].Items, r.Items}, explore.RowMax)
		}
	}
	return out
}

// genreSeed is the newest Library title of kind in the genre (a Kid's
// limit applies).
func (a *API) genreSeed(ctx context.Context, r *http.Request, kind, genre string) (explore.Item, bool) {
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT json_extract(source_ids, '$.tmdb'), name, rating FROM titles
		WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
		AND EXISTS (SELECT 1 FROM json_each(titles.genres) WHERE value = ?) ORDER BY created_at DESC, id DESC LIMIT 20`, kind, genre)
	if err != nil {
		return explore.Item{}, false
	}
	defer rows.Close()
	max := a.kidMax(r)
	for rows.Next() {
		var it explore.Item
		var rating string
		if rows.Scan(&it.TMDBID, &it.Name, &rating) != nil {
			continue
		}
		if max != "" && !ratings.Allows(max, kind, rating) {
			continue
		}
		it.Kind = kind
		return it, true
	}
	return explore.Item{}, false
}

// genreRowsTTL is how long a genre page's rows are kept in memory.
const genreRowsTTL = 15 * time.Minute

// genreRowCache keeps built genre-page rows (before per-person views) so a
// second visit skips the outside lists.
type genreRowCache struct {
	mu   sync.Mutex
	rows map[string]genreRowsEntry
}

type genreRowsEntry struct {
	at   time.Time
	rows []explore.Row
}

func (c *genreRowCache) get(key string) ([]explore.Row, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.rows[key]
	if !ok || time.Since(e.at) > genreRowsTTL {
		return nil, false
	}
	out := make([]explore.Row, len(e.rows))
	copy(out, e.rows)
	return out, true
}

func (c *genreRowCache) put(key string, rows []explore.Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil || len(c.rows) > 200 {
		c.rows = map[string]genreRowsEntry{}
	}
	keep := make([]explore.Row, len(rows))
	copy(keep, rows)
	c.rows[key] = genreRowsEntry{at: time.Now(), rows: keep}
}

// genreWait is how long a genre page waits for its outside lists. Rows
// whose lists are not back by then are left out and the answer says
// partial; the lists keep going and the page asks again. After genreSoft
// the page answers as soon as its first row's lists are back.
const (
	genreWait = 1200 * time.Millisecond
	genreSoft = 600 * time.Millisecond
)

// genreRequests are a genre page's shared lists, in row order: trending,
// the genre's popular titles, best new, classics, then one per mood (the
// "similar" list for the seed comes last when there is one).
func genreRequests(kind, genre string, now time.Time) []explore.Request {
	reqs := []explore.Request{
		{List: "trending"},
		{List: "discover", Genre: genre, MinVotes: 50},
		{List: "discover", Genre: genre, From: now.AddDate(-1, 0, 0).Format("2006-01-02"), To: now.Format("2006-01-02"), MinVote: 6.5, MinVotes: 50},
		{List: "discover", Genre: genre, To: now.AddDate(-20, 0, 0).Format("2006-01-02"), MinVotes: 1000, Sort: "rating"},
	}
	for _, m := range metadata.SubMoods(genre) {
		reqs = append(reqs, explore.Request{List: "discover", Genre: genre, Keywords: strings.Join(m.Keywords, "|"), MinVotes: 20})
	}
	for i := range reqs {
		reqs[i].Kind, reqs[i].Scope = kind, explore.ScopeShared
	}
	return reqs
}

// genreLists fetches reqs side by side and waits up to wait (0: until all
// are back). Lists still out at the deadline are nil and ready is false;
// they finish in the background and land in the list cache.
func (a *API) genreLists(ctx context.Context, kind string, reqs []explore.Request, wait time.Duration) ([][]explore.Item, bool) {
	bg := context.WithoutCancel(ctx)
	src := a.liveSource(bg)
	var mu sync.Mutex
	got := make([][]explore.Item, len(reqs))
	back := make([]bool, len(reqs))
	var wg sync.WaitGroup
	for i, q := range reqs {
		q.Kind, q.Scope = kind, explore.ScopeShared
		wg.Add(1)
		go func(i int, q explore.Request) {
			defer wg.Done()
			items, err := src.List(bg, q)
			if err != nil {
				items = nil
			}
			mu.Lock()
			got[i], back[i] = items, true
			mu.Unlock()
		}(i, q)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	var timeout, soft <-chan time.Time
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		timeout = t.C
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		soft = tick.C
	}
	start := time.Now()
wait:
	for {
		select {
		case <-done:
			break wait
		case <-timeout:
			break wait
		case <-ctx.Done():
			break wait
		case <-soft:
			mu.Lock()
			first := len(back) < 2 || (back[0] && back[1])
			mu.Unlock()
			if first && time.Since(start) >= genreSoft {
				break wait
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	out := make([][]explore.Item, len(got))
	copy(out, got)
	ready := true
	for _, b := range back {
		ready = ready && b
	}
	return out, ready
}

// genreWarmGap spaces the warm job's lists so a person's genre page still
// finds the list burst free.
const genreWarmGap = 3 * time.Second

// warmGenres fetches every genre page's shared lists for the parts that
// are on, so a genre page after a restart reads kept lists. It runs at
// start and again as the lists expire; kept lists cost no outside call.
func (a *API) warmGenres(ctx context.Context) {
	for {
		for _, kind := range []string{"movie", "show"} {
			if len(a.kindsOn(kind)) == 0 {
				continue
			}
			for _, genre := range explore.Genres[kind] {
				for _, q := range genreRequests(kind, genre, a.exploreNow().UTC()) {
					if ctx.Err() != nil {
						return
					}
					src := a.liveSource(ctx)
					if got, ok := src.cachedList(ctx, q.String()); ok && time.Since(got.at) < exploreListTTL[q.List] {
						continue
					}
					_, _ = src.List(ctx, q)
					select {
					case <-ctx.Done():
						return
					case <-time.After(genreWarmGap):
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}
