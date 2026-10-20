package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
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
	if !ok || sw == "all" {
		fail(w, http.StatusBadRequest, "type must be movies or shows")
		return
	}
	kind := explore.Kinds(sw)[0]
	id, genre, ok := exploreGenreParam(q.Get("genre"), kind)
	if !ok || genre == "" {
		fail(w, http.StatusBadRequest, "genre must be a TMDB genre id of that type")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusNotFound, "that part is off")
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	ctx := r.Context()
	src := a.liveSource(ctx)
	list := func(q explore.Request) []explore.Item {
		q.Kind, q.Scope = kind, explore.ScopeShared
		items, err := src.List(ctx, q)
		if err != nil {
			return nil
		}
		return items
	}
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
	var rows []explore.Row
	trending := explore.Interleave([][]explore.Item{inGenre(list(explore.Request{List: "trending"})), list(explore.Request{List: "discover", Genre: genre, MinVotes: 50})}, explore.RowMax)
	rows = append(rows, explore.Row{ID: "genre_trending", Type: explore.TypeTrending, Title: "Trending " + lower, Items: trending})
	rows = append(rows, explore.Row{ID: "genre_best", Type: explore.TypeBest, Title: "Best new " + lower,
		Items: list(explore.Request{List: "discover", Genre: genre, From: now.AddDate(-1, 0, 0).Format("2006-01-02"), To: now.Format("2006-01-02"), MinVote: 6.5, MinVotes: 50})})
	if seed, ok := a.genreSeed(ctx, r, kind, genre); ok {
		rows = append(rows, explore.Row{ID: "genre_because", Type: explore.TypeBecauseHave, Title: "Because you have " + seed.Name,
			Items: inGenre(list(explore.Request{List: "similar", Ref: seed.TMDBID}))})
	}
	rows = append(rows, explore.Row{ID: "genre_classics", Type: explore.TypeClassics, Title: "Top-rated " + lower + " classics",
		Items: list(explore.Request{List: "discover", Genre: genre, To: now.AddDate(-20, 0, 0).Format("2006-01-02"), MinVotes: 1000, Sort: "rating"})})
	for _, m := range metadata.SubMoods(genre) {
		rows = append(rows, explore.Row{ID: "genre_mood_" + strings.ReplaceAll(strings.ToLower(m.Label), " ", "_"), Type: explore.TypeMood, Title: m.Label,
			Items: list(explore.Request{List: "discover", Genre: genre, Keywords: strings.Join(m.Keywords, "|"), MinVotes: 20})})
	}
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
	a.fillRowsCardArt(r, views)
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "genre": map[string]any{"id": id, "name": genre, "kind": kind}, "hero": hero, "rows": views})
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
