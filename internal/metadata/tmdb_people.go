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
	Adult      bool        `json:"adult"`
	Popularity float64     `json:"popularity"`
}

// tmdbKnownVotes reads only the vote counts of a popular page's known_for
// titles, shows included (their names sit under "name", not "title").
type tmdbKnownVotes struct {
	Results []struct {
		KnownFor []struct {
			VoteCount int `json:"vote_count"`
		} `json:"known_for"`
	} `json:"results"`
}

func (p tmdbPerson) person(images string) Person {
	out := Person{ID: strconv.FormatInt(p.ID, 10), Name: p.Name, Department: p.Department, Biography: p.Biography,
		Birthday: p.Birthday, Birthplace: p.Birthplace, KnownFor: []Movie{}, Adult: p.Adult}
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

// PopularPeople returns TMDB's popular people (adult-flagged ones left out).
func (t *TMDB) PopularPeople(ctx context.Context, page int) (PersonPage, error) {
	if page < 1 || page > 500 {
		return PersonPage{}, ErrNotFound
	}
	b, err := t.get(ctx, "/person/popular", url.Values{"page": {strconv.Itoa(page)}})
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
	var votes tmdbKnownVotes
	_ = json.Unmarshal(b, &votes)
	out := PersonPage{People: []Person{}, Page: res.Page, TotalPages: res.TotalPages, TotalResults: res.TotalResults}
	for i, p := range res.Results {
		if p.ID > 0 && p.Name != "" && !p.Adult {
			person := p.person(t.images())
			person.Popularity = p.Popularity
			if i < len(votes.Results) {
				for _, k := range votes.Results[i].KnownFor {
					person.KnownForVotes = append(person.KnownForVotes, k.VoteCount)
				}
			}
			out.People = append(out.People, person)
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
	// Movie credits carry their own art and TMDB's popularity score.
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
}

func (c tmdbCredit) movie(images string) Movie {
	return (tmdbMovie{ID: c.ID, Title: c.Title, ReleaseDate: c.ReleaseDate, PosterPath: c.PosterPath,
		tmdbExtras: tmdbExtras{BackdropPath: c.BackdropPath}}).movie(images)
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
			v = &PersonMovieCredit{Movie: c.movie(t.images()), Roles: []string{}, Characters: []string{}, Popularity: c.Popularity,
				VoteAverage: c.VoteAverage, VoteCount: c.VoteCount}
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

// ShowPeople returns a show's cast from its credits across all seasons
// (TMDB's aggregate list, most episodes first), each actor once with the
// characters they played.
func (t *TMDB) ShowPeople(ctx context.Context, id string) ([]MoviePersonCredit, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return nil, err
	}
	b, err := t.get(ctx, "/tv/"+strconv.FormatInt(n, 10)+"/aggregate_credits", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Cast []struct {
			tmdbCredit
			Roles []struct {
				Character string `json:"character"`
			} `json:"roles"`
		}
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := []MoviePersonCredit{}
	seen := map[int64]bool{}
	for _, c := range res.Cast {
		if c.ID <= 0 || c.Name == "" || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		v := MoviePersonCredit{Person: c.person(t.images()), Roles: []string{"Actor"}, Characters: []string{}}
		for _, r := range c.Roles {
			v.Characters = addUnique(v.Characters, r.Character)
		}
		out = append(out, v)
	}
	return out, nil
}

// TitleCrew is a title's billed cast in billing order, its directors and
// its composers (TMDB's "Sound" people), for the People shelves.
type TitleCrew struct {
	Cast      []Person
	Directors []Person
	Composers []Person
}

// TitleCrewer is the optional surface behind the People shelves. kind is
// movie or show; id is "tmdb:<n>".
type TitleCrewer interface {
	TitleCrew(ctx context.Context, kind, id string) (TitleCrew, error)
}

// TitleCrewerFor returns the title-crew surface selected for ctx.
func TitleCrewerFor(ctx context.Context, provider Provider) (TitleCrewer, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	p, ok := provider.(TitleCrewer)
	return p, ok
}

func composerJob(job string) bool {
	return job == "Original Music Composer" || job == "Music" || job == "Composer"
}

// TitleCrew reads the same credits answers MoviePeople and ShowPeople use,
// so a title already looked at costs no new call.
func (t *TMDB) TitleCrew(ctx context.Context, kind, id string) (TitleCrew, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return TitleCrew{}, err
	}
	path := "/movie/" + strconv.FormatInt(n, 10) + "/credits"
	if kind == "show" {
		path = "/tv/" + strconv.FormatInt(n, 10) + "/aggregate_credits"
	}
	b, err := t.get(ctx, path, nil)
	if err != nil {
		return TitleCrew{}, err
	}
	var res struct {
		Cast []struct {
			tmdbCredit
			Order int `json:"order"`
		}
		Crew []struct {
			tmdbCredit
			Jobs []struct {
				Job string `json:"job"`
			} `json:"jobs"`
		}
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return TitleCrew{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	sort.SliceStable(res.Cast, func(i, j int) bool { return res.Cast[i].Order < res.Cast[j].Order })
	out := TitleCrew{Cast: []Person{}, Directors: []Person{}, Composers: []Person{}}
	seen := map[string]bool{}
	for _, c := range res.Cast {
		if c.ID > 0 && c.Name != "" && !seen["a"+strconv.FormatInt(c.ID, 10)] {
			seen["a"+strconv.FormatInt(c.ID, 10)] = true
			out.Cast = append(out.Cast, c.person(t.images()))
		}
	}
	for _, c := range res.Crew {
		jobs := []string{c.Job}
		for _, j := range c.Jobs {
			jobs = append(jobs, j.Job)
		}
		for _, job := range jobs {
			key := strconv.FormatInt(c.ID, 10)
			switch {
			case c.ID <= 0 || c.Name == "":
			case job == "Director" && kind != "show" && !seen["d"+key]:
				seen["d"+key] = true
				out.Directors = append(out.Directors, c.person(t.images()))
			case composerJob(job) && !seen["s"+key]:
				seen["s"+key] = true
				out.Composers = append(out.Composers, c.person(t.images()))
			}
		}
	}
	return out, nil
}

// PersonShowCounter is the optional surface that counts a person's TV
// credits (acting or directing), each show once however many episodes.
type PersonShowCounter interface {
	PersonShowCount(ctx context.Context, id string) (int, error)
}

// PersonShowCounterFor returns the show-count surface selected for ctx.
func PersonShowCounterFor(ctx context.Context, provider Provider) (PersonShowCounter, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	p, ok := provider.(PersonShowCounter)
	return p, ok
}

// PersonShowCount counts the distinct shows in TMDB's TV credits, as an
// actor or a director.
func (t *TMDB) PersonShowCount(ctx context.Context, id string) (int, error) {
	n, err := tmdbNumericID(id)
	if err != nil {
		return 0, err
	}
	b, err := t.get(ctx, "/person/"+strconv.FormatInt(n, 10)+"/tv_credits", nil)
	if err != nil {
		return 0, err
	}
	var res struct{ Cast, Crew []tmdbCredit }
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	seen := map[int64]bool{}
	for _, c := range res.Cast {
		if c.ID > 0 {
			seen[c.ID] = true
		}
	}
	for _, c := range res.Crew {
		if c.ID > 0 && c.Job == "Director" {
			seen[c.ID] = true
		}
	}
	return len(seen), nil
}
