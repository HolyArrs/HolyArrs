package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func (a *API) peopleRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/people/search", a.searchPeople)
	m.HandleFunc("GET /api/v1/people/popular", a.popularPeople)
	m.HandleFunc("GET /api/v1/people/{id}", a.getPerson)
	m.HandleFunc("POST /api/v1/people/{id}/movies/review", a.reviewPersonMovies)
	m.HandleFunc("GET /api/v1/titles/{id}/people", a.titlePeople)
}

func peopleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrNotFound):
		fail(w, http.StatusNotFound, "no such person")
	case errors.Is(err, metadata.ErrNoAccess):
		fail(w, http.StatusServiceUnavailable, "people results are unavailable; try again shortly")
	default:
		// Provider transport and answer details can contain private search
		// terms or links, so the public answer remains fixed.
		fail(w, http.StatusBadGateway, "people results are unavailable; try again shortly")
	}
}

func (a *API) peopleProvider(r *http.Request, w http.ResponseWriter) (metadata.PeopleProvider, bool) {
	if p, ok := metadata.PeopleProviderFor(r.Context(), a.s.Metadata); ok {
		return p, true
	}
	fail(w, http.StatusServiceUnavailable, "people results are unavailable for this movie metadata provider")
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

func (a *API) searchPeople(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, pageOK := intQuery(r, "page", 1, 500)
	if q == "" || len(q) > 200 || !pageOK {
		fail(w, http.StatusBadRequest, "send ?q=<person name> and optional &page=1..500")
		return
	}
	p, ok := a.peopleProvider(r, w)
	if !ok {
		return
	}
	result, err := p.SearchPeople(r.Context(), q, page)
	if err != nil {
		peopleError(w, err)
		return
	}
	if result.People == nil {
		result.People = []metadata.Person{}
	}
	for i := range result.People {
		a.kidKnownFor(r.Context(), a.kidMax(r), &result.People[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": result.People, "page": result.Page, "total_pages": result.TotalPages,
		"total_results": result.TotalResults, "attribution": metadata.Attribution})
}

// popularPeople answers GET /api/v1/people/popular?page=: TMDB's popular
// people for Search's People tab. Adult-flagged people never appear (the
// provider drops them; page is 1..8, 20 a page); a Kid's known_for is cut like a search's.
func (a *API) popularPeople(w http.ResponseWriter, r *http.Request) {
	page, pageOK := intQuery(r, "page", 1, 8)
	if !pageOK {
		fail(w, http.StatusBadRequest, "page must be 1..8")
		return
	}
	p, ok := a.peopleProvider(r, w)
	if !ok {
		return
	}
	pop, ok := p.(metadata.PopularPeopler)
	if !ok {
		fail(w, http.StatusServiceUnavailable, "popular people are unavailable for this movie metadata provider")
		return
	}
	result, err := pop.PopularPeople(r.Context(), page)
	if err != nil {
		peopleError(w, err)
		return
	}
	if result.People == nil {
		result.People = []metadata.Person{}
	}
	for i := range result.People {
		a.kidKnownFor(r.Context(), a.kidMax(r), &result.People[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": result.People, "page": result.Page, "total_pages": result.TotalPages,
		"total_results": result.TotalResults, "attribution": metadata.Attribution})
}

type peopleMovieView struct {
	metadata.Movie
	Roles      []string  `json:"roles"`
	Characters []string  `json:"characters"`
	TitleID    int64     `json:"title_id,omitempty"`
	NeverAdd   bool      `json:"never_add"`
	Ask        *askBrief `json:"ask,omitempty"`
}

func hasRole(roles []string, role string) bool {
	for _, got := range roles {
		if strings.EqualFold(got, role) {
			return true
		}
	}
	return false
}

func (a *API) peopleMovieViews(r *http.Request, credits []metadata.PersonMovieCredit) ([]peopleMovieView, error) {
	out := make([]peopleMovieView, 0, len(credits))
	db := a.s.Store.Read()
	me := a.person(r).UserID
	for _, c := range credits {
		tmdbID := c.Movie.IDs["tmdb"]
		if tmdbID == "" {
			continue
		}
		v := peopleMovieView{Movie: c.Movie, Roles: append([]string{}, c.Roles...), Characters: append([]string{}, c.Characters...)}
		var err error
		if v.TitleID, err = library.TitleByTMDB(r.Context(), db, tmdbID); err != nil {
			return nil, err
		}
		if v.NeverAdd, err = library.OnNeverAdd(r.Context(), db, tmdbID); err != nil {
			return nil, err
		}
		if v.Ask, err = a.openAsk(r.Context(), tmdbID, me); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func sortPeopleMovies(movies []metadata.PersonMovieCredit) {
	sort.Slice(movies, func(i, j int) bool {
		a, b := movies[i].Movie, movies[j].Movie
		if a.Year != b.Year {
			return a.Year > b.Year
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.IDs["tmdb"] < b.IDs["tmdb"]
	})
}

func (a *API) personCredits(r *http.Request, w http.ResponseWriter) (metadata.Person, metadata.PersonMoviePage, bool) {
	p, ok := a.peopleProvider(r, w)
	if !ok {
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	id := "tmdb:" + r.PathValue("id")
	person, err := p.Person(r.Context(), id)
	if err != nil {
		peopleError(w, err)
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	credits, err := p.PersonMovies(r.Context(), id)
	if err != nil {
		peopleError(w, err)
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	sortPeopleMovies(credits.Movies)
	role := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if role == "" {
		role = "all"
	}
	if role != "all" && role != "actor" && role != "director" {
		fail(w, http.StatusBadRequest, "role must be all, actor or director")
		return metadata.Person{}, metadata.PersonMoviePage{}, false
	}
	// A Kid sees only credits in the Library with a rating their limit
	// allows; the rest of the filmography stays hidden.
	max := a.kidMax(r)
	a.kidKnownFor(r.Context(), max, &person)
	filtered := credits.Movies[:0]
	for _, c := range credits.Movies {
		if (role == "all" || hasRole(c.Roles, role)) && (max == "" || a.libraryMovieAllowed(r.Context(), max, c.Movie.IDs["tmdb"])) {
			filtered = append(filtered, c)
		}
	}
	credits.Movies = filtered
	return person, credits, true
}

func (a *API) getPerson(w http.ResponseWriter, r *http.Request) {
	page, pageOK := intQuery(r, "page", 1, 500)
	size, sizeOK := intQuery(r, "page_size", 24, 100)
	if !pageOK || !sizeOK {
		fail(w, http.StatusBadRequest, "page must be 1..500 and page_size must be 1..100")
		return
	}
	person, credits, ok := a.personCredits(r, w)
	if !ok {
		return
	}
	total := len(credits.Movies)
	totalPages := 0
	if total > 0 {
		totalPages = (total + size - 1) / size
	}
	start := min((page-1)*size, total)
	end := min(start+size, total)
	views, err := a.peopleMovieViews(r, credits.Movies[start:end])
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"person": person, "movies": views, "page": page, "page_size": size,
		"total_pages": totalPages, "total_results": total, "complete": credits.Complete, "provider_pages": credits.ProviderPages,
		"attribution": metadata.Attribution})
}

func (a *API) reviewPersonMovies(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TMDBIDs []string `json:"tmdb_ids"`
	}
	if !decode(w, r, &body, `send {"tmdb_ids":["<number>"]}`) {
		return
	}
	if len(body.TMDBIDs) == 0 || len(body.TMDBIDs) > 100 {
		fail(w, http.StatusBadRequest, "choose between 1 and 100 movies")
		return
	}
	_, credits, ok := a.personCredits(r, w)
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
			fail(w, http.StatusBadRequest, "every tmdb id must be a number")
			return
		}
		credit, exists := known[id]
		if !exists {
			fail(w, http.StatusConflict, "the reviewed movies no longer match this filmography")
			return
		}
		if !seen[id] {
			seen[id] = true
			selectedCredits = append(selectedCredits, credit)
		}
	}
	selected, err := a.peopleMovieViews(r, selectedCredits)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"movies": selected, "automatic_future_credits": false})
}

func (a *API) titlePeople(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var title domain.Title
	if err := store.Get(r.Context(), a.s.Store.Read(), "titles", id, &title); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, http.StatusNotFound, "not found")
		} else {
			internal(w, err)
		}
		return
	}
	if (title.Kind != "movie" && title.Kind != "show") || title.SourceIDs["tmdb"] == "" {
		fail(w, http.StatusConflict, "this title has no TMDB identity")
		return
	}
	var people []metadata.MoviePersonCredit
	var err error
	if title.Kind == "show" {
		sp, ok := metadata.ShowPeoplerFor(r.Context(), a.s.Metadata)
		if !ok {
			fail(w, http.StatusServiceUnavailable, "people results are unavailable for this show metadata provider")
			return
		}
		people, err = sp.ShowPeople(r.Context(), "tmdb:"+title.SourceIDs["tmdb"])
	} else {
		p, ok := a.peopleProvider(r, w)
		if !ok {
			return
		}
		people, err = p.MoviePeople(r.Context(), "tmdb:"+title.SourceIDs["tmdb"])
	}
	if err != nil {
		peopleError(w, err)
		return
	}
	if people == nil {
		people = []metadata.MoviePersonCredit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": people, "complete": false, "attribution": metadata.Attribution})
}
