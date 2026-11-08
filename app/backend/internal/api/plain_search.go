package api

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/search/plainwords"
)

// plainSearchMax bounds each list of a plain-words search.
const plainSearchMax = 24

// showGenres are the TMDB show genres that stand for a movie genre.
var showGenres = map[string]string{
	"Science Fiction": "Sci-Fi & Fantasy", "Fantasy": "Sci-Fi & Fantasy",
	"Action": "Action & Adventure", "Adventure": "Action & Adventure", "War": "War & Politics",
}

// plainSearch is Search in plain words: ?q=that space movie with the robot
// from 2014. The answer holds what the words were understood as (query),
// the Library titles that fit (titles) and titles not in the Library from
// the movie and show details (more). Kid limits and parts that are off
// hide titles, as in the Library search.
func (a *API) plainSearch(w http.ResponseWriter, r *http.Request) {
	q := plainwords.Parse(r.URL.Query().Get("q"))
	out := map[string]any{"query": q, "titles": []TitleView{}, "more": []discoverItemView{}}
	if q.Text == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := withLookupMemo(r.Context())
	views, err := a.plainLibrary(ctx, r, q)
	if err != nil {
		internal(w, err)
		return
	}
	out["titles"] = views
	// A plain name is already searched outside by Search's own rows.
	if !q.Plain && q.Kind != "music" {
		more, note := a.plainMore(ctx, r, q)
		// Library titles are already in titles.
		out["more"] = slices.DeleteFunc(more, func(v discoverItemView) bool { return v.InLibrary })
		if note != "" {
			out["note"] = note
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// plainLibrary are the Library titles that fit q, those about q.Words first.
func (a *API) plainLibrary(ctx context.Context, r *http.Request, q plainwords.Query) ([]TitleView, error) {
	name := q.Name
	if q.Plain {
		name = q.Text
	}
	ts, err := mediafiles.SearchTitles(ctx, a.s.Store, name)
	if err != nil {
		// A name too long or odd to match finds nothing; it is not an error.
		ts = nil
	}
	want := map[string]string{"movie": "movie", "show": "show", "music": "artist"}[q.Kind]
	type hit struct {
		t     mediafiles.Title
		about int
	}
	var hits []hit
	for _, t := range ts {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			continue
		}
		if t.State == "removed" || !a.titleAllowed(r, t) || (want != "" && t.Kind != want) {
			continue
		}
		year := 0
		if t.Year != nil {
			year = *t.Year
		}
		if !inYears(q, year) || !plainGenre(q.Genres, t.Kind, t.Genres) || !hasPeople(q.People, t.People) {
			continue
		}
		overview := ""
		if t.Overview != nil {
			overview = *t.Overview
		}
		hits = append(hits, hit{t, about(q.Words, t.Name+" "+overview)})
	}
	if len(q.Words) > 0 && q.Name == "" && !q.Plain && len(q.Genres) == 0 && len(q.People) == 0 && q.YearFrom == 0 && q.YearTo == 0 {
		// Only "about" words: keep the titles about them.
		hits = slices.DeleteFunc(hits, func(h hit) bool { return h.about == 0 })
	}
	slices.SortStableFunc(hits, func(x, y hit) int { return cmp.Compare(y.about, x.about) })
	views := []TitleView{}
	for _, h := range hits {
		if len(views) == plainSearchMax {
			break
		}
		v, err := a.titleView(ctx, h.t)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

// plainMore are movies and shows not in the Library that fit q. Outside
// trouble never fails the answer: note says why the list is empty.
func (a *API) plainMore(ctx context.Context, r *http.Request, q plainwords.Query) ([]discoverItemView, string) {
	busy := "Titles outside your Library could not be looked up right now."
	me := a.person(r).UserID
	if a.kidMax(r) != "" && !a.discover.allow(me, time.Now()) {
		return []discoverItemView{}, busy
	}
	var items []metadata.DiscoverItem
	switch {
	case q.Name != "" && q.Kind != "show":
		if p, _ := parts.ForTitleKind("movie"); !a.partIsOn(p) {
			break
		}
		year := 0
		if q.YearFrom != 0 && q.YearFrom == q.YearTo {
			year = q.YearFrom
		}
		ms, err := a.s.Metadata.Search(ctx, q.Name, year)
		if err != nil {
			return []discoverItemView{}, busy
		}
		for _, m := range a.kidMovies(ctx, a.kidMax(r), ms) {
			items = append(items, metadata.DiscoverItem{TMDBID: m.IDs["tmdb"], Kind: "movie", Name: m.Name, Year: m.Year,
				Poster: m.Poster, Backdrop: m.Backdrop, Overview: m.Overview, Genres: m.Genres})
		}
	case q.Name == "" && len(q.Genres) > 0:
		d, ok := metadata.DiscovererFor(ctx, a.s.Metadata)
		if !ok {
			return []discoverItemView{}, ""
		}
		for _, k := range a.kindsOn(q.Kind) {
			g := q.Genres[0]
			if k == "show" && showGenres[g] != "" {
				g = showGenres[g]
			}
			got, err := a.cachedRow(ctx, d, "genre", k, "", g)
			if err != nil {
				return []discoverItemView{}, busy
			}
			items = append(items, got...)
		}
		items = a.kidItems(ctx, a.kidMax(r), items, plainSearchMax*2)
	}
	keep := items[:0]
	for _, it := range items {
		if it.TMDBID != "" && inYears(q, it.Year) && (len(it.Genres) == 0 || plainGenre(q.Genres, it.Kind, it.Genres)) {
			keep = append(keep, it)
		}
	}
	slices.SortStableFunc(keep, func(x, y metadata.DiscoverItem) int {
		return cmp.Compare(about(q.Words, y.Name+" "+y.Overview), about(q.Words, x.Name+" "+x.Overview))
	})
	keep, err := a.dropNeverAdd(ctx, keep)
	if err != nil {
		return []discoverItemView{}, busy
	}
	views, err := a.joinItems(ctx, keep, me, plainSearchMax)
	if err != nil {
		return []discoverItemView{}, busy
	}
	return views, ""
}

// inYears: year is inside q's years (an unknown year only fits no years).
func inYears(q plainwords.Query, year int) bool {
	if q.YearFrom == 0 && q.YearTo == 0 {
		return true
	}
	return year != 0 && (q.YearFrom == 0 || year >= q.YearFrom) && (q.YearTo == 0 || year <= q.YearTo)
}

// plainGenre: the title has one of want (no want: every title fits).
func plainGenre(want []string, kind string, have []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, g := range want {
		for _, h := range have {
			if strings.EqualFold(g, h) || (kind == "show" && strings.EqualFold(showGenres[g], h)) {
				return true
			}
		}
	}
	return false
}

// hasPeople: every wanted person is credited, by folded name words.
func hasPeople(want, have []string) bool {
	for _, p := range want {
		words := foldWords(p)
		if !slices.ContainsFunc(have, func(h string) bool { return foldMatch(h, words) }) {
			return false
		}
	}
	return true
}

// about counts the words that text mentions.
func about(words []string, text string) int {
	if len(words) == 0 {
		return 0
	}
	have := foldWords(text)
	n := 0
	for _, w := range words {
		stem := strings.TrimSuffix(fold(w), "s")
		if slices.ContainsFunc(have, func(h string) bool { return strings.HasPrefix(h, stem) }) {
			n++
		}
	}
	return n
}
