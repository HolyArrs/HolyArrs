package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type tmdbPerson struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Department string      `json:"known_for_department"`
	Biography  string      `json:"biography"`
	Birthday   string      `json:"birthday"`
	Deathday   *string     `json:"deathday"`
	Birthplace string      `json:"place_of_birth"`
	Profile    string      `json:"profile_path"`
	KnownFor   []tmdbMovie `json:"known_for"`
}

func (p tmdbPerson) person(images string) Person {
	out := Person{ID: strconv.FormatInt(p.ID, 10), Name: p.Name, Department: p.Department, Biography: p.Biography,
		Birthday: p.Birthday, Birthplace: p.Birthplace, KnownFor: []Movie{}}
	if p.Deathday != nil {
		out.Deathday = *p.Deathday
	}
	if p.Profile != "" {
		out.Profile = images + "/w185" + p.Profile
	}
	for _, m := range p.KnownFor {
		if m.ID > 0 && m.Title != "" {
			out.KnownFor = append(out.KnownFor, m.movie(images))
		}
	}
	return out
}

func tmdbNumericID(id string) (int64, error) {
	src, raw, ok := strings.Cut(strings.TrimSpace(id), ":")
	if !ok || src != "tmdb" || raw == "" || len(raw) > 12 || strings.Trim(raw, "0123456789") != "" {
		return 0, ErrNotFound
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, ErrNotFound
	}
	return n, nil
}

// SearchPeople searches TMDB people. TMDB pages start at one.
func (t *TMDB) SearchPeople(ctx context.Context, query string, page int) (PersonPage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return PersonPage{}, ErrNotFound
	}
	if page < 1 {
		page = 1
	}
	if page > 500 {
		return PersonPage{}, ErrNotFound
	}
	b, err := t.get(ctx, "/search/person", url.Values{"query": {query}, "include_adult": {"false"}, "page": {strconv.Itoa(page)}})
	if err != nil {
		return PersonPage{}, err
	}
	var res struct {
		Results      []tmdbPerson `json:"results"`
		Page         int          `json:"page"`
		TotalPages   int          `json:"total_pages"`
		TotalResults int          `json:"total_results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return PersonPage{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := PersonPage{People: []Person{}, Page: res.Page, TotalPages: res.TotalPages, TotalResults: res.TotalResults}
	for _, p := range res.Results {
		if p.ID > 0 && p.Name != "" {
			out.People = append(out.People, p.person(t.images()))
		}
	}
	return out, nil
}

// Person returns one stable TMDB person identity.
func (t *TMDB) Person(ctx context.Context, id string) (Person, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return Person{}, err
	}
	b, err := t.get(ctx, "/person/"+strconv.FormatInt(n, 10), nil)
	if err != nil {
		return Person{}, err
	}
	var p tmdbPerson
	if err := json.Unmarshal(b, &p); err != nil {
		return Person{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	if p.ID <= 0 || p.Name == "" {
		return Person{}, ErrNotFound
	}
	return p.person(t.images()), nil
}

type tmdbCredit struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
	Name        string `json:"name"`
	Department  string `json:"known_for_department"`
	Profile     string `json:"profile_path"`
	Character   string `json:"character"`
	Job         string `json:"job"`
}

func (c tmdbCredit) movie(images string) Movie {
	return (tmdbMovie{ID: c.ID, Title: c.Title, ReleaseDate: c.ReleaseDate}).movie(images)
}

func (c tmdbCredit) person(images string) Person {
	return (tmdbPerson{ID: c.ID, Name: c.Name, Department: c.Department, Profile: c.Profile}).person(images)
}

func addUnique(xs []string, value string) []string {
	value = strings.TrimSpace(value)
	if value != "" && !slices.Contains(xs, value) {
		return append(xs, value)
	}
	return xs
}

// PersonMovies returns TMDB's unpaged movie credits, deduplicated by movie.
func (t *TMDB) PersonMovies(ctx context.Context, id string) (PersonMoviePage, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return PersonMoviePage{}, err
	}
	b, err := t.get(ctx, "/person/"+strconv.FormatInt(n, 10)+"/movie_credits", nil)
	if err != nil {
		return PersonMoviePage{}, err
	}
	var res struct{ Cast, Crew []tmdbCredit }
	if err := json.Unmarshal(b, &res); err != nil {
		return PersonMoviePage{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	byID := map[int64]*PersonMovieCredit{}
	add := func(c tmdbCredit, role string) {
		if c.ID <= 0 || c.Title == "" {
			return
		}
		v := byID[c.ID]
		if v == nil {
			v = &PersonMovieCredit{Movie: c.movie(t.images()), Roles: []string{}, Characters: []string{}}
			byID[c.ID] = v
		}
		v.Roles = addUnique(v.Roles, role)
		if role == "Actor" {
			v.Characters = addUnique(v.Characters, c.Character)
		}
	}
	for _, c := range res.Cast {
		add(c, "Actor")
	}
	for _, c := range res.Crew {
		if c.Job == "Director" {
			add(c, "Director")
		}
	}
	out := PersonMoviePage{Movies: []PersonMovieCredit{}, Complete: false, ProviderPages: 1}
	for _, v := range byID {
		out.Movies = append(out.Movies, *v)
	}
	sort.Slice(out.Movies, func(i, j int) bool {
		a, b := out.Movies[i].Movie, out.Movies[j].Movie
		if a.Year != b.Year {
			return a.Year > b.Year
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.IDs["tmdb"] < b.IDs["tmdb"]
	})
	return out, nil
}

// MoviePeople returns actors and directors for a movie. Other crew jobs are
// excluded, and repeated credits are merged under the stable person id.
func (t *TMDB) MoviePeople(ctx context.Context, id string) ([]MoviePersonCredit, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return nil, err
	}
	b, err := t.get(ctx, "/movie/"+strconv.FormatInt(n, 10)+"/credits", nil)
	if err != nil {
		return nil, err
	}
	var res struct{ Cast, Crew []tmdbCredit }
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	byID := map[int64]*MoviePersonCredit{}
	add := func(c tmdbCredit, role string) {
		if c.ID <= 0 || c.Name == "" {
			return
		}
		v := byID[c.ID]
		if v == nil {
			v = &MoviePersonCredit{Person: c.person(t.images()), Roles: []string{}, Characters: []string{}}
			byID[c.ID] = v
		}
		v.Roles = addUnique(v.Roles, role)
		if role == "Actor" {
			v.Characters = addUnique(v.Characters, c.Character)
		}
	}
	for _, c := range res.Cast {
		add(c, "Actor")
	}
	for _, c := range res.Crew {
		if c.Job == "Director" {
			add(c, "Director")
		}
	}
	out := []MoviePersonCredit{}
	for _, v := range byID {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Person.ID != out[j].Person.ID {
			ai, _ := strconv.ParseInt(out[i].Person.ID, 10, 64)
			aj, _ := strconv.ParseInt(out[j].Person.ID, 10, 64)
			return ai < aj
		}
		return out[i].Person.Name < out[j].Person.Name
	})
	return out, nil
}
