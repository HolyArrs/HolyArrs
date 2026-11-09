package metadata

import "context"

// PeopleProvider is the optional movie-people surface. Providers that only
// know movies continue to satisfy Provider.
type PeopleProvider interface {
	SearchPeople(ctx context.Context, query string, page int) (PersonPage, error)
	Person(ctx context.Context, id string) (Person, error)
	PersonMovies(ctx context.Context, id string) (PersonMoviePage, error)
	MoviePeople(ctx context.Context, id string) ([]MoviePersonCredit, error)
}

// PeopleProviderFor returns the optional people surface selected for ctx.
// Switch stays a movie Provider, so existing providers do not grow methods.
func PeopleProviderFor(ctx context.Context, provider Provider) (PeopleProvider, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	p, ok := provider.(PeopleProvider)
	return p, ok
}

// Person is one stable provider identity. Dates use YYYY-MM-DD when known.
type Person struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Department string  `json:"department,omitempty"`
	Biography  string  `json:"biography,omitempty"`
	Birthday   string  `json:"birthday,omitempty"`
	Deathday   string  `json:"deathday,omitempty"`
	Birthplace string  `json:"birthplace,omitempty"`
	Profile    string  `json:"profile,omitempty"`
	KnownFor   []Movie `json:"known_for"`
	// Adult is TMDB's adult flag; it never leaves the server.
	Adult bool `json:"-"`
	// Popularity and KnownForVotes (vote counts of every known_for title,
	// movies and shows) feed the server's fame ranking; never sent.
	Popularity    float64 `json:"-"`
	KnownForVotes []int   `json:"-"`
}

// PopularPeopler is the optional popular-people surface (TMDB's popular list).
type PopularPeopler interface {
	PopularPeople(ctx context.Context, page int) (PersonPage, error)
}

// PersonPage is one provider search page.
type PersonPage struct {
	People       []Person `json:"people"`
	Page         int      `json:"page"`
	TotalPages   int      `json:"total_pages"`
	TotalResults int      `json:"total_results"`
}

// PersonMovieCredit is one movie with every supported role this person held.
type PersonMovieCredit struct {
	Movie      Movie    `json:"movie"`
	Roles      []string `json:"roles"`
	Characters []string `json:"characters"`
	Popularity float64  `json:"popularity,omitempty"`
	// TMDB's audience score for the movie and how many votes it rests on.
	VoteAverage float64 `json:"vote_average,omitempty"`
	VoteCount   int     `json:"vote_count,omitempty"`
}

// PersonMoviePage is the provider's unpaged credit result. Complete remains
// false because TMDB's endpoint has no page or completeness contract.
type PersonMoviePage struct {
	Movies        []PersonMovieCredit `json:"movies"`
	Complete      bool                `json:"complete"`
	ProviderPages int                 `json:"provider_pages"`
}

// MoviePersonCredit is one person credited on a movie.
type MoviePersonCredit struct {
	Person     Person   `json:"person"`
	Roles      []string `json:"roles"`
	Characters []string `json:"characters"`
}

// ShowPeopler is the optional show-people surface: a show's regular cast
// across every season, most episodes first.
type ShowPeopler interface {
	ShowPeople(ctx context.Context, id string) ([]MoviePersonCredit, error)
}

// ShowPeoplerFor returns the show-people surface selected for ctx.
func ShowPeoplerFor(ctx context.Context, provider Provider) (ShowPeopler, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	p, ok := provider.(ShowPeopler)
	return p, ok
}
