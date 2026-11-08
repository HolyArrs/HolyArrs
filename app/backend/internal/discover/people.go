package discover

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

func (h *Handlers) PeopleRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/people/search", h.searchPeople)
	m.HandleFunc("GET /api/v1/people/popular", h.popularPeople)
	m.HandleFunc("GET /api/v1/people/shelves", h.peopleShelves)
	m.HandleFunc("GET /api/v1/people/shelves/{id}", h.peopleShelfOne)
	m.HandleFunc("GET /api/v1/people/{id}", h.getPerson)
	m.HandleFunc("POST /api/v1/people/{id}/movies/review", h.reviewPersonMovies)
	m.HandleFunc("GET /api/v1/titles/{id}/people", h.titlePeople)
}

// PeopleError answers a people provider error without its details.
func PeopleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such person")
	case errors.Is(err, metadata.ErrNoAccess):
		webserver.Fail(w, http.StatusServiceUnavailable, "people results are unavailable; try again shortly")
	case DetailsBusy(err):
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusServiceUnavailable, "people results are busy; try again in a moment")
	default:
		// Provider transport and answer details can contain private search
		// terms or links, so the public answer remains fixed.
		webserver.Fail(w, http.StatusBadGateway, "people results are unavailable; try again shortly")
	}
}

func (h *Handlers) peopleProvider(r *http.Request, w http.ResponseWriter) (metadata.PeopleProvider, bool) {
	if p, ok := metadata.PeopleProviderFor(r.Context(), h.Metadata); ok {
		return p, true
	}
	webserver.Fail(w, http.StatusServiceUnavailable, "people results are unavailable for this movie metadata provider")
	return nil, false
}

func intQuery(r *http.Request, name string, fallback, maximum int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 1 && n <= maximum
}

func (h *Handlers) searchPeople(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, pageOK := intQuery(r, "page", 1, 500)
	if q == "" || len(q) > 200 || !pageOK {
		webserver.Fail(w, http.StatusBadRequest, "send ?q=<person name> and optional &page=1..500")
		return
	}
	p, ok := h.peopleProvider(r, w)
	if !ok {
		return
	}
	result, err := p.SearchPeople(r.Context(), q, page)
	if err != nil {
		PeopleError(w, err)
		return
	}
	if result.People == nil {
		result.People = []metadata.Person{}
	}
	for i := range result.People {
		h.KidKnownFor(r.Context(), h.KidMax(r), &result.People[i])
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"people": result.People, "page": result.Page, "total_pages": result.TotalPages,
		"total_results": result.TotalResults, "attribution": metadata.Attribution})
}

// popularPeople answers GET /api/v1/people/popular?page=: Search's People
// tab popular list, re-ranked by fame from TMDB's first eight popular pages
// (20 a page). Adult people and people without a picture or a well-known
// title never appear; a Kid's known_for is cut like a search's.
func (h *Handlers) popularPeople(w http.ResponseWriter, r *http.Request) {
	page, pageOK := intQuery(r, "page", 1, popularPages)
	if !pageOK {
		webserver.Fail(w, http.StatusBadRequest, "page must be 1..8")
		return
	}
	p, ok := h.peopleProvider(r, w)
	if !ok {
		return
	}
	pop, ok := p.(metadata.PopularPeopler)
	if !ok {
		webserver.Fail(w, http.StatusServiceUnavailable, "popular people are unavailable for this movie metadata provider")
		return
	}
	pool, err := h.popularPool(r.Context(), p, pop)
	if err != nil {
		PeopleError(w, err)
		return
	}
	people := []metadata.Person{}
	if from := (page - 1) * popularPageSize; from < len(pool) {
		people = append(people, pool[from:min(len(pool), from+popularPageSize)]...)
	}
	for i := range people {
		h.KidKnownFor(r.Context(), h.KidMax(r), &people[i])
	}
	total := (len(pool) + popularPageSize - 1) / popularPageSize
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"people": people, "page": page, "total_pages": total,
		"total_results": len(pool), "attribution": metadata.Attribution})
}

type peopleMovieView struct {
	metadata.Movie
	Roles      []string           `json:"roles"`
	Characters []string           `json:"characters"`
	Popularity float64            `json:"popularity,omitempty"`
	TitleID    int64              `json:"title_id,omitempty"`
	NeverAdd   bool               `json:"never_add"`
	Ask        *requests.AskBrief `json:"ask,omitempty"`
}

func hasRole(roles []string, role string) bool {
	for _, got := range roles {
		if strings.EqualFold(got, role) {
			return true
		}
	}
	return false
}

func (h *Handlers) peopleMovieViews(r *http.Request, credits []metadata.PersonMovieCredit) ([]peopleMovieView, error) {
	out := make([]peopleMovieView, 0, len(credits))
	db := h.Store.Read()
	me := h.UserID(r)
	for _, c := range credits {
		tmdbID := c.Movie.IDs["tmdb"]
		if tmdbID == "" {
			continue
		}
		v := peopleMovieView{Movie: c.Movie, Roles: append([]string{}, c.Roles...), Characters: append([]string{}, c.Characters...), Popularity: c.Popularity}
		var err error
		if v.TitleID, err = library.TitleByTMDB(r.Context(), db, tmdbID); err != nil {
			return nil, err
		}
		if v.NeverAdd, err = library.OnNeverAdd(r.Context(), db, tmdbID); err != nil {
			return nil, err
		}
		if v.Ask, err = h.OpenAsk(r.Context(), tmdbID, me); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// movieReleased is a credit's release day when it is already out; an
// undated credit counts as out from the year after its year.
func movieReleased(m metadata.Movie, now time.Time) (time.Time, bool) {
	if m.CinemaDate != nil && !m.CinemaDate.After(now) {
		return *m.CinemaDate, true
	}
	if m.CinemaDate == nil && m.Year > 0 && m.Year < now.Year() {
		return time.Date(m.Year, 1, 1, 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}

// PersonStats is the hero's strip: years active, film count and the
// average audience score of acting credits with at least 25 votes.
type PersonStats struct {
	FirstYear int  `json:"first_year,omitempty"`
	LastYear  int  `json:"last_year,omitempty"`
	Active    bool `json:"active"`
	Films     int  `json:"films"`
	// Shows counts TV credits, each show once; Titles is films plus shows.
	Shows  int     `json:"shows,omitempty"`
	Titles int     `json:"titles,omitempty"`
	Rating float64 `json:"rating,omitempty"`
	Rated  int     `json:"rated,omitempty"`
}

func peopleStats(credits []metadata.PersonMovieCredit, now time.Time) PersonStats {
	st := PersonStats{Films: len(credits)}
	sum := 0.0
	for _, c := range credits {
		if c.Movie.Year > 0 {
			if st.FirstYear == 0 || c.Movie.Year < st.FirstYear {
				st.FirstYear = c.Movie.Year
			}
			st.LastYear = max(st.LastYear, c.Movie.Year)
		}
		if hasRole(c.Roles, "actor") && c.VoteCount >= 25 {
			sum += c.VoteAverage
			st.Rated++
		}
	}
	if st.Rated > 0 {
		st.Rating = float64(int(sum/float64(st.Rated)*10+0.5)) / 10
	}
	st.Active = st.LastYear >= now.Year()-1
	return st
}

// coStars counts the other people named on this person's Library titles
// (the cast stored with each movie and show this member may see) and looks
// the most frequent ones up, so the page can show who they often work
// with. A title counts when its stored cast names the person or it is one
// of their movie credits. Fewer than four found people show nothing.
func (h *Handlers) coStars(r *http.Request, p metadata.PeopleProvider, self string, titles []peopleMovieView) []metadata.Person {
	out := []metadata.Person{}
	if p == nil {
		return out
	}
	ids := map[int64]bool{}
	for _, t := range titles {
		ids[t.TitleID] = true
	}
	where := "t.kind IN ('movie', 'show') AND t.state != 'removed'"
	clause, args := mediafiles.KidSQL(h.KidMax(r))
	if clause != "" {
		where += " AND " + clause
	}
	rows, err := h.Store.Read().QueryContext(r.Context(), "SELECT t.id, t.people FROM titles t WHERE "+where+" ORDER BY t.id", args...)
	if err != nil {
		return out
	}
	counts, order := map[string]int{}, []string{}
	for rows.Next() {
		var id int64
		var raw string
		if rows.Scan(&id, &raw) != nil {
			continue
		}
		var names []string
		if json.Unmarshal([]byte(raw), &names) != nil {
			continue
		}
		mine := ids[id]
		for _, n := range names {
			mine = mine || strings.EqualFold(strings.TrimSpace(n), self)
		}
		if !mine {
			continue
		}
		seen := map[string]bool{}
		for _, n := range names {
			n = strings.TrimSpace(n)
			k := strings.ToLower(n)
			if n == "" || strings.EqualFold(n, self) || seen[k] {
				continue
			}
			seen[k] = true
			if counts[n] == 0 {
				order = append(order, n)
			}
			counts[n]++
		}
	}
	rows.Close()
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	order = order[:min(len(order), 12)]
	found := make([]*metadata.Person, len(order))
	eachLimited(len(order), func(i int) {
		got, err := p.SearchPeople(r.Context(), order[i], 1)
		if err != nil {
			return
		}
		for _, m := range got.People {
			if strings.EqualFold(m.Name, order[i]) {
				m.KnownFor = []metadata.Movie{}
				found[i] = &m
				return
			}
		}
	})
	for _, m := range found {
		if m != nil && len(out) < 8 {
			out = append(out, *m)
		}
	}
	if len(out) < 4 {
		return []metadata.Person{}
	}
	return out
}

// sortPeopleMovies orders credits by release: oldest first (or newest
// first), with undated and upcoming ones last; "popular" orders by TMDB's
// popularity score. Name and id break ties so pages stay stable.
func sortPeopleMovies(movies []metadata.PersonMovieCredit, order string, now time.Time) {
	released := func(m metadata.Movie) (time.Time, bool) { return movieReleased(m, now) }
	sort.SliceStable(movies, func(i, j int) bool {
		a, b := movies[i].Movie, movies[j].Movie
		if order == "popular" {
			if movies[i].Popularity != movies[j].Popularity {
				return movies[i].Popularity > movies[j].Popularity
			}
		} else {
			da, oka := released(a)
			db, okb := released(b)
			if oka != okb {
				return oka
			}
			if oka && !da.Equal(db) {
				if order == "newest" {
					return da.After(db)
				}
				return da.Before(db)
			}
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.IDs["tmdb"] < b.IDs["tmdb"]
	})
}

func (h *Handlers) personCredits(r *http.Request, w http.ResponseWriter) (metadata.Person, metadata.PersonMoviePage, bool) {
	p, ok := h.peopleProvider(r, w)
	if !ok {
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	id := "tmdb:" + r.PathValue("id")
	person, err := p.Person(r.Context(), id)
	if err != nil {
		PeopleError(w, err)
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	credits, err := p.PersonMovies(r.Context(), id)
	if err != nil {
		PeopleError(w, err)
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	order := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if order == "" {
		order = "oldest"
	}
	if order != "oldest" && order != "newest" && order != "popular" {
		webserver.Fail(w, http.StatusBadRequest, "sort must be oldest, newest or popular")
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	sortPeopleMovies(credits.Movies, order, time.Now().UTC())
	role := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if role == "" {
		role = "all"
	}
	if role != "all" && role != "actor" && role != "director" {
		webserver.Fail(w, http.StatusBadRequest, "role must be all, actor or director")
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	// A Kid sees only credits in the Library with a rating their limit
	// allows; the rest of the filmography stays hidden.
	max := h.KidMax(r)
	h.KidKnownFor(r.Context(), max, &person)
	filtered := credits.Movies[:0]
	for _, c := range credits.Movies {
		if (role == "all" || hasRole(c.Roles, role)) && (max == "" || h.LibraryMovieAllowed(r.Context(), max, c.Movie.IDs["tmdb"])) {
			filtered = append(filtered, c)
		}
	}
	credits.Movies = filtered
	return person, credits, true
}

func (h *Handlers) getPerson(w http.ResponseWriter, r *http.Request) {
	page, pageOK := intQuery(r, "page", 1, 500)
	size, sizeOK := intQuery(r, "page_size", 24, 100)
	if !pageOK || !sizeOK {
		webserver.Fail(w, http.StatusBadRequest, "page must be 1..500 and page_size must be 1..100")
		return
	}
	person, credits, ok := h.personCredits(r, w)
	if !ok {
		return
	}
	p, _ := h.peopleProvider(r, w)
	total := len(credits.Movies)
	totalPages := 0
	if total > 0 {
		totalPages = (total + size - 1) / size
	}
	start := min((page-1)*size, total)
	end := min(start+size, total)
	views, err := h.peopleMovieViews(r, credits.Movies[start:end])
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	// The personal rows (In your Library, You asked for) and Known for come
	// from every credit this member may see, so Kid limits hold here too.
	all, err := h.peopleMovieViews(r, credits.Movies)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	now := time.Now().UTC()
	stats := peopleStats(credits.Movies, now)
	// TV credits only count for members without a Kid limit (a Kid sees Library movies only).
	if c, ok := metadata.PersonShowCounterFor(r.Context(), h.Metadata); ok && h.KidMax(r) == "" {
		if n, err := c.PersonShowCount(r.Context(), "tmdb:"+r.PathValue("id")); err == nil && n > 0 {
			stats.Shows, stats.Titles = n, stats.Films+n
		}
	}
	library, asked, soon := []peopleMovieView{}, []peopleMovieView{}, []peopleMovieView{}
	for _, v := range all {
		if _, out := movieReleased(v.Movie, now); !out && v.Year >= now.Year() {
			soon = append(soon, v)
		}
		if v.TitleID != 0 {
			library = append(library, v)
		} else if v.Ask != nil && v.Ask.Mine && v.Ask.State != "turned_down" {
			asked = append(asked, v)
		}
	}
	sort.SliceStable(soon, func(i, j int) bool {
		a, b := soon[i].CinemaDate, soon[j].CinemaDate
		if (a == nil) != (b == nil) {
			return a != nil
		}
		return a != nil && a.Before(*b)
	})
	known := append([]peopleMovieView{}, all...)
	sort.SliceStable(known, func(i, j int) bool { return known[i].Popularity > known[j].Popularity })
	known = slices.DeleteFunc(known, func(v peopleMovieView) bool { return v.Poster == "" && v.Backdrop == "" })
	known = known[:min(len(known), 10)]
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"person": person, "movies": views, "page": page, "page_size": size,
		"total_pages": totalPages, "total_results": total, "complete": credits.Complete, "provider_pages": credits.ProviderPages,
		"library": library, "asked": asked, "known_for": known, "coming_soon": soon,
		"stats": stats, "often_with": h.coStars(r, p, person.Name, library), "attribution": metadata.Attribution})
}

func (h *Handlers) reviewPersonMovies(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TMDBIDs []string `json:"tmdb_ids"`
	}
	if !webserver.Decode(w, r, &body, `send {"tmdb_ids":["<number>"]}`) {
		return
	}
	if len(body.TMDBIDs) == 0 || len(body.TMDBIDs) > 100 {
		webserver.Fail(w, http.StatusBadRequest, "choose between 1 and 100 movies")
		return
	}
	_, credits, ok := h.personCredits(r, w)
	if !ok {
		return
	}
	known := map[string]metadata.PersonMovieCredit{}
	for _, credit := range credits.Movies {
		known[credit.Movie.IDs["tmdb"]] = credit
	}
	seen := map[string]bool{}
	selectedCredits := []metadata.PersonMovieCredit{}
	for _, id := range body.TMDBIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 12 || strings.Trim(id, "0123456789") != "" {
			webserver.Fail(w, http.StatusBadRequest, "every tmdb id must be a number")
			return
		}
		credit, exists := known[id]
		if !exists {
			webserver.Fail(w, http.StatusConflict, "the reviewed movies no longer match this filmography")
			return
		}
		if !seen[id] {
			seen[id] = true
			selectedCredits = append(selectedCredits, credit)
		}
	}
	selected, err := h.peopleMovieViews(r, selectedCredits)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"movies": selected, "automatic_future_credits": false})
}

func (h *Handlers) titlePeople(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var title mediafiles.Title
	if err := database.Get(r.Context(), h.Store.Read(), "titles", id, &title); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			webserver.Fail(w, http.StatusNotFound, "not found")
		} else {
			webserver.Internal(w, "discover", err)
		}
		return
	}
	if (title.Kind != "movie" && title.Kind != "show") || title.SourceIDs["tmdb"] == "" {
		webserver.Fail(w, http.StatusConflict, "this title has no id to look it up by")
		return
	}
	var people []metadata.MoviePersonCredit
	var err error
	if title.Kind == "show" {
		sp, ok := metadata.ShowPeoplerFor(r.Context(), h.Metadata)
		if !ok {
			webserver.Fail(w, http.StatusServiceUnavailable, "people results are unavailable for this show metadata provider")
			return
		}
		people, err = sp.ShowPeople(r.Context(), "tmdb:"+title.SourceIDs["tmdb"])
	} else {
		p, ok := h.peopleProvider(r, w)
		if !ok {
			return
		}
		people, err = p.MoviePeople(r.Context(), "tmdb:"+title.SourceIDs["tmdb"])
	}
	if err != nil {
		PeopleError(w, err)
		return
	}
	if people == nil {
		people = []metadata.MoviePersonCredit{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"people": people, "complete": false, "attribution": metadata.Attribution})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}
