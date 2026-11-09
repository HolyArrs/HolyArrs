package discover

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
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
func (h *Handlers) plainSearch(w http.ResponseWriter, r *http.Request) {
	q := ParsePlainWords(r.URL.Query().Get("q"))
	out := map[string]any{"query": q, "titles": []TitleView{}, "more": []ItemView{}}
	if q.Text == "" {
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	ctx := WithLookupMemo(r.Context())
	views, err := h.plainLibrary(ctx, r, q)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out["titles"] = views
	// A plain name is already searched outside by Search's own rows.
	if !q.Plain && q.Kind != "music" {
		more, note := h.plainMore(ctx, r, q)
		// Library titles are already in titles.
		out["more"] = slices.DeleteFunc(more, func(v ItemView) bool { return v.InLibrary })
		if note != "" {
			out["note"] = note
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// plainLibrary are the Library titles that fit q, those about q.Words first.
func (h *Handlers) plainLibrary(ctx context.Context, r *http.Request, q PlainQuery) ([]TitleView, error) {
	name := q.Name
	if q.Plain {
		name = q.Text
	}
	ts, err := mediafiles.SearchTitles(ctx, h.Store, name)
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
		if p, ok := parts.ForTitleKind(t.Kind); ok && !h.PartOn(p) && !h.Parts.SafeMode {
			continue
		}
		if t.State == "removed" || !h.TitleAllowed(r, t) || (want != "" && t.Kind != want) {
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
		hits = slices.DeleteFunc(hits, func(x hit) bool { return x.about == 0 })
	}
	slices.SortStableFunc(hits, func(x, y hit) int { return cmp.Compare(y.about, x.about) })
	views := []TitleView{}
	for _, x := range hits {
		if len(views) == plainSearchMax {
			break
		}
		v, err := h.TitleView(ctx, x.t)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

// plainMore are movies and shows not in the Library that fit q. Outside
// trouble never fails the answer: note says why the list is empty.
func (h *Handlers) plainMore(ctx context.Context, r *http.Request, q PlainQuery) ([]ItemView, string) {
	busy := "Titles outside your Library could not be looked up right now."
	me := h.UserID(r)
	if h.KidMax(r) != "" && !h.Cache.Allow(me, time.Now()) {
		return []ItemView{}, busy
	}
	var items []metadata.DiscoverItem
	switch {
	case q.Name != "" && q.Kind != "show":
		if p, _ := parts.ForTitleKind("movie"); !h.PartOn(p) {
			break
		}
		year := 0
		if q.YearFrom != 0 && q.YearFrom == q.YearTo {
			year = q.YearFrom
		}
		ms, err := h.Metadata.Search(ctx, q.Name, year)
		if err != nil {
			return []ItemView{}, busy
		}
		for _, m := range h.KidMovies(ctx, h.KidMax(r), ms) {
			items = append(items, metadata.DiscoverItem{TMDBID: m.IDs["tmdb"], Kind: "movie", Name: m.Name, Year: m.Year,
				Poster: m.Poster, Backdrop: m.Backdrop, Overview: m.Overview, Genres: m.Genres})
		}
	case q.Name == "" && len(q.Genres) > 0:
		d, ok := metadata.DiscovererFor(ctx, h.Metadata)
		if !ok {
			return []ItemView{}, ""
		}
		for _, k := range h.KindsOn(q.Kind) {
			g := q.Genres[0]
			if k == "show" && showGenres[g] != "" {
				g = showGenres[g]
			}
			got, err := h.CachedRow(ctx, d, "genre", k, "", g)
			if err != nil {
				return []ItemView{}, busy
			}
			items = append(items, got...)
		}
		items = h.KidItems(ctx, h.KidMax(r), items, plainSearchMax*2)
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
	keep, err := h.DropNeverAdd(ctx, keep)
	if err != nil {
		return []ItemView{}, busy
	}
	views, err := h.JoinItems(ctx, keep, me, plainSearchMax)
	if err != nil {
		return []ItemView{}, busy
	}
	return views, ""
}

// inYears: year is inside q's years (an unknown year only fits no years).
func inYears(q PlainQuery, year int) bool {
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
		words := mediafiles.FoldWords(p)
		if !slices.ContainsFunc(have, func(h string) bool { return mediafiles.FoldMatch(h, words) }) {
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
	have := mediafiles.FoldWords(text)
	n := 0
	for _, w := range words {
		stem := strings.TrimSuffix(mediafiles.Fold(w), "s")
		if slices.ContainsFunc(have, func(h string) bool { return strings.HasPrefix(h, stem) }) {
			n++
		}
	}
	return n
}
