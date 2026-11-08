package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ratings"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Kid accounts: a member with an age-rating limit sees and asks for only
// the movies and shows rated at or below it. Titles without a rating, with
// a rating off the ladder, and music never pass a limit (see ratings).

// ratingLookups is how many TMDB details lookups for ratings run at once,
// over every request; each also waits for the TMDB client's own spacing.
const ratingLookups = 4

var ratingSlots = make(chan struct{}, ratingLookups)

// aboveLimit is the answer to a TMDB title above the limit.
const aboveLimit = "This title is rated above what this account may see."

// kidMax is the signed-in person's age-rating limit: "" for no limit, for
// owners and admins, and without sign-in.
func (a *API) kidMax(r *http.Request) string {
	who, ok := webserver.From(r.Context())
	if !ok || who.Role != webserver.RoleMember {
		return ""
	}
	return who.MaxRating
}

// titleAllowed says the signed-in person may see the Library title t.
func (a *API) titleAllowed(r *http.Request, t mediafiles.Title) bool {
	return ratings.Allows(a.kidMax(r), t.Kind, t.Rating)
}

// libraryRating is the rating of the Library movie or show with that TMDB
// id; false when it is not in the Library.
func (a *API) libraryRating(ctx context.Context, kind, tmdbID string) (string, bool) {
	var rating string
	err := a.s.Store.Read().QueryRowContext(ctx, `SELECT rating FROM titles WHERE kind = ? AND state != 'removed'
		AND json_extract(source_ids, '$.tmdb') = ? ORDER BY id LIMIT 1`, kind, tmdbID).Scan(&rating)
	return rating, err == nil
}

const (
	// ratingLookupsPerRequest caps the TMDB rating lookups one answer
	// makes; the titles past it stay hidden.
	ratingLookupsPerRequest = 24
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

// ratingRef is one TMDB movie or show to check; rating is the rating the
// answer at hand carries, if any. The Library's rating wins over it.
type ratingRef struct{ kind, tmdbID, rating string }

// knownRating finds a rating without asking TMDB: the Library's, the one
// the answer carries, then the rating and details caches.
func (a *API) knownRating(ctx context.Context, ref ratingRef) (string, bool) {
	if r, ok := a.libraryRating(ctx, ref.kind, ref.tmdbID); ok {
		return r, true
	}
	if ref.rating != "" {
		return ref.rating, true
	}
	if ref.tmdbID == "" || (ref.kind != "movie" && ref.kind != "show") {
		return "", false
	}
	if r, ok := a.rated.get(ref.kind+"|"+ref.tmdbID, time.Now()); ok {
		return r, true
	}
	if a.s.Store != nil {
		if b, ok, err := (store.LookupCache{Store: a.s.Store}).Get(ctx, "rating|"+ref.kind+"|"+ref.tmdbID, time.Now()); err == nil && ok {
			a.rated.put(ref.kind+"|"+ref.tmdbID, string(b), time.Now())
			return string(b), true
		}
	}
	if e, ok := a.discover.get("details|" + ref.kind + "|" + ref.tmdbID); ok && e.details != nil && time.Since(e.at) < discoverTTL["details"] {
		return e.details.Rating, true
	}
	return "", false
}

// lookupRating asks TMDB for one rating, with the light rating call when
// the provider has it, and keeps the answer in the rating cache.
func (a *API) lookupRating(ctx context.Context, ref ratingRef) (string, bool) {
	select {
	case ratingSlots <- struct{}{}:
	case <-ctx.Done():
		return "", false
	}
	defer func() { <-ratingSlots }()
	var rating string
	if rt, ok := metadata.RaterFor(ctx, a.s.Metadata); ok {
		r, err := rt.Rating(ctx, ref.kind, ref.tmdbID)
		if err != nil {
			return "", false
		}
		rating = r
	} else if d, ok := metadata.DiscovererFor(ctx, a.s.Metadata); ok {
		det, err := d.Details(ctx, ref.kind, ref.tmdbID)
		if err != nil {
			return "", false
		}
		rating = det.Rating
	} else {
		return "", false
	}
	now := time.Now()
	a.rated.put(ref.kind+"|"+ref.tmdbID, rating, now)
	if a.s.Store != nil {
		_ = store.LookupCache{Store: a.s.Store}.Put(context.WithoutCancel(ctx), "rating|"+ref.kind+"|"+ref.tmdbID, []byte(rating), now, now.Add(ratingStoredTTL))
	}
	return rating, true
}

// kidAllowed says, for each ref, whether max allows it. Ratings are
// looked up a few at a time, in order, stopping once n are allowed (n <=
// 0: never); the refs after that are false. At most
// ratingLookupsPerRequest go to TMDB, within the person's minute budget;
// titles left without a rating are false.
func (a *API) kidAllowed(ctx context.Context, max string, refs []ratingRef, n int) []bool {
	out := make([]bool, len(refs))
	who, _ := webserver.From(ctx)
	left := ratingLookupsPerRequest
	found := 0
	for i := 0; i < len(refs) && (n <= 0 || found < n); i += ratingLookups {
		batch := refs[i:min(i+ratingLookups, len(refs))]
		got := make([]string, len(batch))
		known := make([]bool, len(batch))
		var need []int
		for j, ref := range batch {
			if got[j], known[j] = a.knownRating(ctx, ref); !known[j] && ref.tmdbID != "" && (ref.kind == "movie" || ref.kind == "show") {
				need = append(need, j)
			}
		}
		grant := 0
		if len(need) > 0 && left > 0 {
			grant = a.rated.take(who.UserID, min(len(need), left), time.Now())
			left -= grant
		}
		var wg sync.WaitGroup
		for _, j := range need[:grant] {
			wg.Go(func() { got[j], known[j] = a.lookupRating(ctx, batch[j]) })
		}
		wg.Wait()
		for j, ref := range batch {
			if n > 0 && found == n {
				break
			}
			if known[j] && ratings.Allows(max, ref.kind, got[j]) {
				out[i+j] = true
				found++
			}
		}
	}
	return out
}

// kidItems keeps the Explore items max allows, at most n (n <= 0: all).
func (a *API) kidItems(ctx context.Context, max string, items []metadata.DiscoverItem, n int) []metadata.DiscoverItem {
	if max == "" {
		return items
	}
	refs := make([]ratingRef, len(items))
	for i, it := range items {
		refs[i] = ratingRef{kind: it.Kind, tmdbID: it.TMDBID}
	}
	ok := a.kidAllowed(ctx, max, refs, n)
	out := []metadata.DiscoverItem{}
	for i, it := range items {
		if ok[i] {
			out = append(out, it)
		}
	}
	return out
}

// kidMovies keeps the looked-up movies max allows.
func (a *API) kidMovies(ctx context.Context, max string, ms []metadata.Movie) []metadata.Movie {
	if max == "" {
		return ms
	}
	refs := make([]ratingRef, len(ms))
	for i, m := range ms {
		refs[i] = ratingRef{kind: "movie", tmdbID: m.IDs["tmdb"], rating: m.Rating}
	}
	ok := a.kidAllowed(ctx, max, refs, 0)
	out := []metadata.Movie{}
	for i, m := range ms {
		if ok[i] {
			out = append(out, m)
		}
	}
	return out
}

// kidShows keeps the looked-up shows max allows.
func (a *API) kidShows(ctx context.Context, max string, shows []metadata.Show) []metadata.Show {
	if max == "" {
		return shows
	}
	refs := make([]ratingRef, len(shows))
	for i, sh := range shows {
		refs[i] = ratingRef{kind: "show", tmdbID: sh.IDs["tmdb"], rating: sh.Rating}
	}
	ok := a.kidAllowed(ctx, max, refs, 0)
	out := []metadata.Show{}
	for i, sh := range shows {
		if ok[i] {
			out = append(out, sh)
		}
	}
	return out
}

// libraryMovieAllowed says the TMDB movie is in the Library with a rating
// max allows; person credits outside the Library stay hidden for a Kid.
func (a *API) libraryMovieAllowed(ctx context.Context, max, tmdbID string) bool {
	r, ok := a.libraryRating(ctx, "movie", tmdbID)
	return ok && ratings.Allows(max, "movie", r)
}

// kidKnownFor keeps the person's known-for movies a Kid may see.
func (a *API) kidKnownFor(ctx context.Context, max string, p *metadata.Person) {
	if max == "" {
		return
	}
	kept := []metadata.Movie{}
	for _, m := range p.KnownFor {
		if a.libraryMovieAllowed(ctx, max, m.IDs["tmdb"]) {
			kept = append(kept, m)
		}
	}
	p.KnownFor = kept
}

// kidSQL is a where clause (titles as t) keeping the titles max allows;
// "" for no limit.
func kidSQL(max string) (string, []any) {
	if max == "" {
		return "", nil
	}
	var ors []string
	var args []any
	for _, l := range []struct {
		kind   string
		ladder []string
	}{{"movie", ratings.Movie}, {"show", ratings.TV}} {
		var marks []string
		args = append(args, l.kind)
		for _, r := range l.ladder {
			if ratings.Allows(max, l.kind, r) {
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

// allowedTitleIDs says which of the Library title ids max allows.
func (a *API) allowedTitleIDs(ctx context.Context, max string, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, id := range ids {
		if _, done := out[id]; done {
			continue
		}
		var kind, rating string
		err := a.s.Store.Read().QueryRowContext(ctx, "SELECT kind, rating FROM titles WHERE id = ?", id).Scan(&kind, &rating)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out[id] = err == nil && ratings.Allows(max, kind, rating)
	}
	return out, nil
}

// kidGate answers 404 to a Kid for any /titles/{id} route of a title above
// their limit (or not there at all, so the two look the same), and for
// every album route, because music has no ratings. Routes the person may
// not use pass on, so the permission gate still answers 403 first.
func (a *API) kidGate(m *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max := a.kidMax(r)
		if max == "" {
			next.ServeHTTP(w, r)
			return
		}
		_, pattern := m.Handler(r)
		who, _ := webserver.From(r.Context())
		need, known := accounts.RoutePerms[pattern]
		if !known || !who.Has(need) || !who.HasScope(accounts.RouteScope(pattern)) {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case strings.Contains(pattern, " /api/v1/albums/{id}"):
			fail(w, http.StatusNotFound, "not found")
			return
		case strings.Contains(pattern, " /api/v1/titles/{id}"):
			var kind, rating string
			err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT kind, rating FROM titles WHERE id = ?",
				pathValue(pattern, r.URL.Path, "{id}")).Scan(&kind, &rating)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !ratings.Allows(max, kind, rating)) {
				fail(w, http.StatusNotFound, "not found")
				return
			}
			if err != nil {
				internal(w, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
