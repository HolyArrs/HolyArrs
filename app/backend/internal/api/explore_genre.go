package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// netflixService is Netflix's TMDB watch-provider id; its chip uses
// Netflix's own weekly Top 10 instead of TMDB popularity.
const netflixService = "8"

const topServicesMax = discover.TopServicesMax

// genreMoodMin is the fewest titles a genre page's mood row needs.
const genreMoodMin = 8

// liveSource is the Explore list source with outside calls allowed (lists
// are cached by their own TTL).
func (a *API) liveSource(ctx context.Context) exploreSource {
	var lister metadata.ExploreLister
	if l, ok := metadata.ExploreListerFor(ctx, a.s.Metadata); ok {
		lister = l
	}
	return exploreSource{a: a, lister: lister, calls: &engine.Calls{}}
}

// exploreTop answers GET /api/v1/explore/top?type=&chip=all|<service id>
// &services=&region=&genre=: the ranked "Top on your services" row for one
// chip. Netflix (no genre) is Netflix's weekly Top 10; any other service,
// and All my services, is TMDB's most popular on it in the region, kept a
// week, merged by popularity with each title once.
func (a *API) exploreTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := discover.ExploreSwitch(q.Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	chip, services, region := q.Get("chip"), q.Get("services"), strings.ToUpper(q.Get("region"))
	if chip == "" {
		chip = "all"
	}
	if (chip != "all" && !metadata.ServiceIDs(chip)) || strings.Contains(chip, "|") || (services != "" && !metadata.ServiceIDs(services)) || !isCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "chip must be all or one service id, services ids joined by |, region a country code")
		return
	}
	var kinds []string
	for _, k := range engine.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds = append(kinds, k)
		}
	}
	genre := ""
	if g := q.Get("genre"); g != "" {
		if sw == "all" {
			webserver.Fail(w, http.StatusBadRequest, "genre needs type movies or shows")
			return
		}
		var gok bool
		if _, genre, gok = discover.ExploreGenreParam(g, discover.Kinds0(kinds, sw)); !gok {
			webserver.Fail(w, http.StatusBadRequest, "genre must be a TMDB genre id of that type")
			return
		}
	}
	if !a.discover.Allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	providers := chip
	if chip == "all" {
		providers = services
	}
	out := map[string]any{"chip": chip, "region": region, "source": "popular", "items": []rankedView{}}
	if providers == "" || len(kinds) == 0 {
		a.exploreCaching(w, r, "private, max-age=300")
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	src := a.liveSource(r.Context())
	var lists [][]engine.Item
	for _, k := range kinds {
		items, err := src.List(r.Context(), engine.Request{List: "weekly", Kind: k, Services: providers, Region: region, Genre: genre, Scope: engine.ScopeShared})
		if err != nil {
			continue
		}
		lists = append(lists, items)
	}
	// A few spare titles cover ones the Kid limit or never-add drops.
	merged := engine.MergeTop(lists, topServicesMax*2)
	views, err := a.exploreViews(r.Context(), r, []engine.Row{{ID: engine.TypeTopServices, Type: engine.TypeTopServices, Items: merged}})
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	items := []rankedView{}
	if len(views) == 1 {
		for i, v := range views[0].Items {
			if i == topServicesMax {
				break
			}
			items = append(items, rankedView{ItemView: v, Rank: i + 1})
		}
	}
	a.fillTopArt(r, items)
	out["items"] = items
	a.exploreCaching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, out)
}

// fillTopArt adds trailer keys and card art to ranked items.
func (a *API) fillTopArt(r *http.Request, items []rankedView) {
	views := make([]discoverItemView, len(items))
	for i := range items {
		views[i] = items[i].ItemView
	}
	a.fillTrailerKeys(r, views)
	a.fillCardArt(r, views)
	for i := range items {
		items[i].ItemView = views[i]
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
	sw, ok := discover.ExploreSwitch(q.Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	// Under All a genre is a movie genre id that TV has a combined genre
	// for; the page merges the movie page and the show page.
	kind := engine.Kinds(sw)[0]
	if sw == "all" {
		kind = "movie"
	}
	id, genre, ok := discover.ExploreGenreParam(q.Get("genre"), kind)
	if !ok || genre == "" || (sw == "all" && engine.ShowGenreFor(genre) == "") {
		webserver.Fail(w, http.StatusBadRequest, "genre must be a TMDB genre id of that type")
		return
	}
	for _, k := range engine.Kinds(sw) {
		if len(a.kindsOn(k)) == 0 {
			webserver.Fail(w, http.StatusNotFound, "that part is off")
			return
		}
	}
	if !a.discover.Allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	st := webserver.NewServerTiming()
	ctx := r.Context()
	rows, partial := a.genreBuild(ctx, r, kind, genre)
	if sw == "all" {
		more, p2 := a.genreBuild(ctx, r, "show", engine.ShowGenreFor(genre))
		rows, partial = discover.MergeGenreRows(rows, more), partial || p2
	}
	st.Mark("lists")
	// A title once per page, first row wins.
	seen := map[string]bool{}
	for i := range rows {
		var keep []engine.Item
		for _, it := range rows[i].Items {
			if !seen[it.Key()] && len(keep) < engine.RowMax {
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
		webserver.Internal(w, "api", err)
		return
	}
	st.Mark("views")
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
	st.Mark("trailers")
	a.fillRowsCardArt(r, views)
	st.Mark("cardart")
	st.Write(w)
	a.exploreCaching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "genre": map[string]any{"id": id, "name": genre, "kind": kind}, "hero": hero, "rows": views, "partial": partial})
}

// genreBuild is one kind's genre page rows (before per-person views): from
// the memory cache, else built from the genre's lists. partial says some
// lists were still out.
func (a *API) genreBuild(ctx context.Context, r *http.Request, kind, genre string) ([]engine.Row, bool) {
	inGenre := func(items []engine.Item) []engine.Item {
		var out []engine.Item
		for _, it := range items {
			if engine.HasGenre(it, genre) {
				out = append(out, it)
			}
		}
		return out
	}
	now := a.exploreNow().UTC()
	lower := strings.ToLower(genre)
	seed, hasSeed := a.genreSeed(ctx, r, kind, genre)
	cacheKey := kind + "|" + genre + "|" + a.kidMax(r) + "|" + seed.TMDBID + "|" + now.Format("2006-01-02")
	if rows, cached := a.genreRows.Get(cacheKey); cached {
		return rows, false
	}
	var rows []engine.Row
	reqs := discover.GenreRequests(kind, genre, now)
	moods := metadata.SubMoods(genre)
	if hasSeed {
		reqs = append(reqs, engine.Request{List: "similar", Ref: seed.TMDBID})
	}
	got, ready := a.genreLists(ctx, kind, reqs, genreWait)
	trending := engine.Interleave([][]engine.Item{inGenre(got[0]), got[1]}, engine.RowMax)
	rows = append(rows, engine.Row{ID: "genre_trending", Type: engine.TypeTrending, Title: "Trending " + lower, Items: trending})
	rows = append(rows, engine.Row{ID: "genre_best", Type: engine.TypeBest, Title: "Best new " + lower, Items: got[2]})
	if hasSeed {
		rows = append(rows, engine.Row{ID: "genre_because", Type: engine.TypeBecauseHave, Title: "Because you have " + seed.Name,
			Items: inGenre(got[len(got)-1])})
	}
	rows = append(rows, engine.Row{ID: "genre_classics", Type: engine.TypeClassics, Title: "Top-rated " + lower + " classics", Items: got[3]})
	for i, m := range moods {
		rows = append(rows, engine.Row{ID: "genre_mood_" + strings.ReplaceAll(strings.ToLower(m.Label), " ", "_"), Type: engine.TypeMood, Title: m.Label,
			Items: got[4+i]})
	}
	if ready {
		a.genreRows.Put(cacheKey, rows)
	}
	return rows, !ready
}

// genreSeed is the newest Library title of kind in the genre (a Kid's
// limit applies).
func (a *API) genreSeed(ctx context.Context, r *http.Request, kind, genre string) (engine.Item, bool) {
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT json_extract(source_ids, '$.tmdb'), name, rating FROM titles
		WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
		AND EXISTS (SELECT 1 FROM json_each(titles.genres) WHERE value = ?) ORDER BY created_at DESC, id DESC LIMIT 20`, kind, genre)
	if err != nil {
		return engine.Item{}, false
	}
	defer rows.Close()
	max := a.kidMax(r)
	for rows.Next() {
		var it engine.Item
		var rating string
		if rows.Scan(&it.TMDBID, &it.Name, &rating) != nil {
			continue
		}
		if max != "" && !metadata.RatingAllows(max, kind, rating) {
			continue
		}
		it.Kind = kind
		return it, true
	}
	return engine.Item{}, false
}

// genreWait is how long a genre page waits for its outside lists. Rows
// whose lists are not back by then are left out and the answer says
// partial; the lists keep going and the page asks again. After genreSoft
// the page answers as soon as its first row's lists are back.
const (
	genreWait = 1200 * time.Millisecond
	genreSoft = 600 * time.Millisecond
)

// genreLists fetches reqs side by side and waits up to wait (0: until all
// are back). Lists still out at the deadline are nil and ready is false;
// they finish in the background and land in the list cache.
func (a *API) genreLists(ctx context.Context, kind string, reqs []engine.Request, wait time.Duration) ([][]engine.Item, bool) {
	bg := context.WithoutCancel(ctx)
	src := a.liveSource(bg)
	var mu sync.Mutex
	got := make([][]engine.Item, len(reqs))
	back := make([]bool, len(reqs))
	var wg sync.WaitGroup
	for i, q := range reqs {
		q.Kind, q.Scope = kind, engine.ScopeShared
		wg.Add(1)
		go func(i int, q engine.Request) {
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
	out := make([][]engine.Item, len(got))
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
			for _, genre := range engine.Genres[kind] {
				for _, q := range discover.GenreRequests(kind, genre, a.exploreNow().UTC()) {
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

// genreRowCache keeps built genre-page rows (internal/discover).
type genreRowCache = discover.GenreRowCache
