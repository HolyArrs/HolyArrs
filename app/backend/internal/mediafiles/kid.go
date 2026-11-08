package mediafiles

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Kid accounts: a member with an age-rating limit sees and asks for only
// the movies and shows rated at or below it. Titles without a rating, with
// a rating off the ladder, and music never pass a limit (see ratings).

// ratingLookups is how many TMDB details lookups for ratings run at once,
// over every request; each also waits for the TMDB client's own spacing.
const ratingLookups = 4

var ratingSlots = make(chan struct{}, ratingLookups)

// aboveLimit is the answer to a TMDB title above the limit.
const aboveLimit = metadata.AboveLimit

// TitleAllowed says the signed-in person may see the Library title t.
func (h *Handlers) TitleAllowed(r *http.Request, t Title) bool {
	return metadata.RatingAllows(h.KidMax(r), t.Kind, t.Rating)
}

// LibraryRating is the rating of the Library movie or show with that TMDB
// id; false when it is not in the Library.
func (h *Handlers) LibraryRating(ctx context.Context, kind, tmdbID string) (string, bool) {
	var rating string
	err := h.Store.Read().QueryRowContext(ctx, `SELECT rating FROM titles WHERE kind = ? AND state != 'removed'
		AND json_extract(source_ids, '$.tmdb') = ? ORDER BY id LIMIT 1`, kind, tmdbID).Scan(&rating)
	return rating, err == nil
}

const (
	// RatingLookupsPerRequest caps the TMDB rating lookups one answer
	// makes; the titles past it stay hidden.
	RatingLookupsPerRequest = 24
	// ratingLookupsPerMinute is each person's TMDB rating lookups a minute;
	// over it, titles with unknown ratings stay hidden.
	ratingLookupsPerMinute = 120
	maxRatingCache         = 2000
	ratingTTL              = 24 * time.Hour
	// ratingStoredTTL keeps a looked-up rating in SQLite: certifications
	// rarely change, and a restart should not ask again.
	ratingStoredTTL = 30 * 24 * time.Hour
)

type ratingEntry struct {
	rating string
	at     time.Time
}

// ratingState keeps the ratings looked up on TMDB (the oldest go first
// when full) and each person's lookup times.
type ratingState struct {
	mu    sync.Mutex
	cache map[string]ratingEntry
	order []string // cache keys, oldest first
	hits  map[int64][]time.Time
}

func (s *ratingState) get(key string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	if !ok || now.Sub(e.at) >= ratingTTL {
		return "", false
	}
	return e.rating, true
}

func (s *ratingState) put(key, rating string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]ratingEntry{}
	}
	if _, ok := s.cache[key]; !ok {
		for len(s.cache) >= maxRatingCache && len(s.order) > 0 {
			delete(s.cache, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, key)
	}
	s.cache[key] = ratingEntry{rating: rating, at: now}
}

// take grants up to n lookups from the person's minute budget.
func (s *ratingState) take(who int64, n int, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hits == nil {
		s.hits = map[int64][]time.Time{}
	}
	kept := s.hits[who][:0]
	for _, t := range s.hits[who] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	n = max(0, min(n, ratingLookupsPerMinute-len(kept)))
	for range n {
		kept = append(kept, now)
	}
	s.hits[who] = kept
	return n
}

// RatingRef is one TMDB movie or show to check; rating is the rating the
// answer at hand carries, if any. The Library's rating wins over it.
type RatingRef struct{ Kind, TMDBID, Rating string }

// KnownRating finds a rating without asking TMDB: the Library's, the one
// the answer carries, then the rating and details caches.
func (h *Handlers) KnownRating(ctx context.Context, ref RatingRef) (string, bool) {
	if r, ok := h.LibraryRating(ctx, ref.Kind, ref.TMDBID); ok {
		return r, true
	}
	if ref.Rating != "" {
		return ref.Rating, true
	}
	if ref.TMDBID == "" || (ref.Kind != "movie" && ref.Kind != "show") {
		return "", false
	}
	if r, ok := h.rated.get(ref.Kind+"|"+ref.TMDBID, time.Now()); ok {
		return r, true
	}
	if h.Store != nil {
		if b, ok, err := (metadata.LookupCache{Store: h.Store}).Get(ctx, "rating|"+ref.Kind+"|"+ref.TMDBID, time.Now()); err == nil && ok {
			h.rated.put(ref.Kind+"|"+ref.TMDBID, string(b), time.Now())
			return string(b), true
		}
	}
	if h.DetailsRating != nil {
		if r, ok := h.DetailsRating(ref.Kind, ref.TMDBID); ok {
			return r, true
		}
	}
	return "", false
}

// LookupRating asks TMDB for one rating, with the light rating call when
// the provider has it, and keeps the answer in the rating cache.
func (h *Handlers) LookupRating(ctx context.Context, ref RatingRef) (string, bool) {
	select {
	case ratingSlots <- struct{}{}:
	case <-ctx.Done():
		return "", false
	}
	defer func() { <-ratingSlots }()
	var rating string
	if rt, ok := metadata.RaterFor(ctx, h.Metadata); ok {
		r, err := rt.Rating(ctx, ref.Kind, ref.TMDBID)
		if err != nil {
			return "", false
		}
		rating = r
	} else if d, ok := metadata.DiscovererFor(ctx, h.Metadata); ok {
		det, err := d.Details(ctx, ref.Kind, ref.TMDBID)
		if err != nil {
			return "", false
		}
		rating = det.Rating
	} else {
		return "", false
	}
	now := time.Now()
	h.rated.put(ref.Kind+"|"+ref.TMDBID, rating, now)
	if h.Store != nil {
		_ = metadata.LookupCache{Store: h.Store}.Put(context.WithoutCancel(ctx), "rating|"+ref.Kind+"|"+ref.TMDBID, []byte(rating), now, now.Add(ratingStoredTTL))
	}
	return rating, true
}

// KidAllowed says, for each ref, whether max allows it. Ratings are
// looked up a few at a time, in order, stopping once n are allowed (n <=
// 0: never); the refs after that are false. At most
// RatingLookupsPerRequest go to TMDB, within the person's minute budget;
// titles left without a rating are false.
func (h *Handlers) KidAllowed(ctx context.Context, max string, refs []RatingRef, n int) []bool {
	out := make([]bool, len(refs))
	who, _ := webserver.From(ctx)
	left := RatingLookupsPerRequest
	found := 0
	for i := 0; i < len(refs) && (n <= 0 || found < n); i += ratingLookups {
		batch := refs[i:min(i+ratingLookups, len(refs))]
		got := make([]string, len(batch))
		known := make([]bool, len(batch))
		var need []int
		for j, ref := range batch {
			if got[j], known[j] = h.KnownRating(ctx, ref); !known[j] && ref.TMDBID != "" && (ref.Kind == "movie" || ref.Kind == "show") {
				need = append(need, j)
			}
		}
		grant := 0
		if len(need) > 0 && left > 0 {
			grant = h.rated.take(who.UserID, min(len(need), left), time.Now())
			left -= grant
		}
		var wg sync.WaitGroup
		for _, j := range need[:grant] {
			wg.Go(func() { got[j], known[j] = h.LookupRating(ctx, batch[j]) })
		}
		wg.Wait()
		for j, ref := range batch {
			if n > 0 && found == n {
				break
			}
			if known[j] && metadata.RatingAllows(max, ref.Kind, got[j]) {
				out[i+j] = true
				found++
			}
		}
	}
	return out
}

// KidItems keeps the Explore items max allows, at most n (n <= 0: all).
func (h *Handlers) KidItems(ctx context.Context, max string, items []metadata.DiscoverItem, n int) []metadata.DiscoverItem {
	if max == "" {
		return items
	}
	refs := make([]RatingRef, len(items))
	for i, it := range items {
		refs[i] = RatingRef{Kind: it.Kind, TMDBID: it.TMDBID}
	}
	ok := h.KidAllowed(ctx, max, refs, n)
	out := []metadata.DiscoverItem{}
	for i, it := range items {
		if ok[i] {
			out = append(out, it)
		}
	}
	return out
}

// KidMovies keeps the looked-up movies max allows.
func (h *Handlers) KidMovies(ctx context.Context, max string, ms []metadata.Movie) []metadata.Movie {
	if max == "" {
		return ms
	}
	refs := make([]RatingRef, len(ms))
	for i, m := range ms {
		refs[i] = RatingRef{Kind: "movie", TMDBID: m.IDs["tmdb"], Rating: m.Rating}
	}
	ok := h.KidAllowed(ctx, max, refs, 0)
	out := []metadata.Movie{}
	for i, m := range ms {
		if ok[i] {
			out = append(out, m)
		}
	}
	return out
}

// KidShows keeps the looked-up shows max allows.
func (h *Handlers) KidShows(ctx context.Context, max string, shows []metadata.Show) []metadata.Show {
	if max == "" {
		return shows
	}
	refs := make([]RatingRef, len(shows))
	for i, sh := range shows {
		refs[i] = RatingRef{Kind: "show", TMDBID: sh.IDs["tmdb"], Rating: sh.Rating}
	}
	ok := h.KidAllowed(ctx, max, refs, 0)
	out := []metadata.Show{}
	for i, sh := range shows {
		if ok[i] {
			out = append(out, sh)
		}
	}
	return out
}

// LibraryMovieAllowed says the TMDB movie is in the Library with a rating
// max allows; person credits outside the Library stay hidden for a Kid.
func (h *Handlers) LibraryMovieAllowed(ctx context.Context, max, tmdbID string) bool {
	r, ok := h.LibraryRating(ctx, "movie", tmdbID)
	return ok && metadata.RatingAllows(max, "movie", r)
}

// KidKnownFor keeps the person's known-for movies a Kid may see.
func (h *Handlers) KidKnownFor(ctx context.Context, max string, p *metadata.Person) {
	if max == "" {
		return
	}
	kept := []metadata.Movie{}
	for _, m := range p.KnownFor {
		if h.LibraryMovieAllowed(ctx, max, m.IDs["tmdb"]) {
			kept = append(kept, m)
		}
	}
	p.KnownFor = kept
}

// KidSQL is a where clause (titles as t) keeping the titles max allows;
// "" for no limit.
func KidSQL(max string) (string, []any) {
	if max == "" {
		return "", nil
	}
	var ors []string
	var args []any
	for _, l := range []struct {
		kind   string
		ladder []string
	}{{"movie", metadata.MovieRatings}, {"show", metadata.TVRatings}} {
		var marks []string
		args = append(args, l.kind)
		for _, r := range l.ladder {
			if metadata.RatingAllows(max, l.kind, r) {
				marks = append(marks, "?")
				args = append(args, r)
			}
		}
		if len(marks) == 0 {
			args = args[:len(args)-1]
			continue
		}
		ors = append(ors, "(t.kind = ? AND upper(trim(t.rating)) IN ("+strings.Join(marks, ",")+"))")
	}
	if len(ors) == 0 {
		return "0", nil
	}
	return "(" + strings.Join(ors, " OR ") + ")", args
}

// AllowedTitleIDs says which of the Library title ids max allows.
func (h *Handlers) AllowedTitleIDs(ctx context.Context, max string, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, id := range ids {
		if _, done := out[id]; done {
			continue
		}
		var kind, rating string
		err := h.Store.Read().QueryRowContext(ctx, "SELECT kind, rating FROM titles WHERE id = ?", id).Scan(&kind, &rating)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out[id] = err == nil && metadata.RatingAllows(max, kind, rating)
	}
	return out, nil
}

// KidGate answers 404 to a Kid for any /titles/{id} route of a title above
// their limit (or not there at all, so the two look the same), and for
// every album route, because music has no ratings. Routes the person may
// not use pass on, so the permission gate still answers 403 first.
func (h *Handlers) KidGate(m *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max := h.KidMax(r)
		if max == "" {
			next.ServeHTTP(w, r)
			return
		}
		_, pattern := m.Handler(r)
		if h.RouteUsable == nil || !h.RouteUsable(r, pattern) {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case strings.Contains(pattern, " /api/v1/albums/{id}"):
			webserver.Fail(w, http.StatusNotFound, "not found")
			return
		case strings.Contains(pattern, " /api/v1/titles/{id}"):
			var kind, rating string
			err := h.Store.Read().QueryRowContext(r.Context(), "SELECT kind, rating FROM titles WHERE id = ?",
				pathValue(pattern, r.URL.Path, "{id}")).Scan(&kind, &rating)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !metadata.RatingAllows(max, kind, rating)) {
				webserver.Fail(w, http.StatusNotFound, "not found")
				return
			}
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// pathValue is the value of the {name} segment of pattern in path.
func pathValue(pattern, path, name string) string {
	_, pat, _ := strings.Cut(pattern, " ")
	ps, xs := strings.Split(pat, "/"), strings.Split(path, "/")
	for i, seg := range ps {
		if seg == name && i < len(xs) {
			return xs[i]
		}
	}
	return ""
}

// KeepRating puts a known rating in the rating cache, as a lookup would.
func (h *Handlers) KeepRating(kind, tmdbID, rating string) {
	h.rated.put(kind+"|"+tmdbID, rating, time.Now())
}
