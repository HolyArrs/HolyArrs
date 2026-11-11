package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ErrNotFound is returned when no movie has the id.
var ErrNotFound = errors.New("no such movie")

// Movie is one looked-up movie.
type Movie struct {
	Name             string            `json:"name"`
	Year             int               `json:"year"`
	Overview         string            `json:"overview"`
	OriginalLanguage string            `json:"original_language"`
	Runtime          int               `json:"runtime,omitempty"`  // minutes, 0 when unknown
	IDs              map[string]string `json:"ids"`                // tmdb, imdb
	Poster           string            `json:"poster,omitempty"`   // full picture URL, "" when none
	Backdrop         string            `json:"backdrop,omitempty"` // wide picture URL (w1280), "" when none
	Logo             string            `json:"logo,omitempty"`     // English or no-language logo PNG URL (w500)
	Genres           []string          `json:"genres,omitempty"`   // provider genre names, English
	Rating           string            `json:"rating,omitempty"`   // US certification, "" when unknown
	Score            float64           `json:"score,omitempty"`    // members' average out of 10, one decimal
	Votes            int               `json:"votes,omitempty"`    // how many members voted
	People           []string          `json:"people,omitempty"`   // top cast, then directors (names only)
	// Directors are the directors alone (names only).
	Directors []string `json:"directors,omitempty"`
	// Studio is the first production company (a streaming film's is often the service).
	Studio string `json:"studio,omitempty"`
	// OtherNames are the original, alternative and translated names the
	// source knows (detail answers only); the library keeps the usable ones.
	OtherNames []string `json:"other_names,omitempty"`
	// Release dates (UTC midnight), nil when the source does not know them.
	CinemaDate   *time.Time `json:"cinema_date,omitempty"`
	DigitalDate  *time.Time `json:"digital_date,omitempty"`
	PhysicalDate *time.Time `json:"physical_date,omitempty"`
	// CollectionKnown distinguishes a confirmed "not in a collection" detail
	// response from search data, where collection membership is unknown.
	CollectionKnown bool             `json:"collection_known,omitempty"`
	Collection      *MovieCollection `json:"collection,omitempty"`
}

// MovieCollection is a provider's stable identity for a movie series.
type MovieCollection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChangeLister is a Provider that can list the movies changed since a time.
type ChangeLister interface {
	// Changed returns "<source>:<id>" for every movie changed since since.
	Changed(ctx context.Context, since time.Time) ([]string, error)
}

type noCacheKey struct{}

// NoCache asks providers to skip their cache for lookups made with ctx (a
// details refresh must see fresh data).
func NoCache(ctx context.Context) context.Context { return context.WithValue(ctx, noCacheKey{}, true) }

func noCache(ctx context.Context) bool { v, _ := ctx.Value(noCacheKey{}).(bool); return v }

// Fresh reports whether ctx asks for fresh data (NoCache).
func Fresh(ctx context.Context) bool { return noCache(ctx) }

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// Provider finds movies.
type Provider interface {
	Search(ctx context.Context, query string, year int) ([]Movie, error)
	// Movie takes "<source>:<id>", such as "tmdb:693134".
	Movie(ctx context.Context, id string) (Movie, error)
}

// Static is a Provider over a fixed list.
type Static struct{ Movies []Movie }

// Lab is the fixed local record for the lab movie. Its runtime is left out
// on purpose: the lab file is a short test encode, so size limits per minute
// cannot apply to it.
func Lab() *Static {
	return &Static{Movies: []Movie{{
		Name: "Dune: Part Two", Year: 2024, OriginalLanguage: "en",
		Overview:   "Paul Atreides unites with the Fremen while seeking revenge against those who destroyed his family.",
		IDs:        map[string]string{"tmdb": "693134", "imdb": "tt15239678"},
		CinemaDate: day(2024, time.March, 1), DigitalDate: day(2024, time.April, 16), PhysicalDate: day(2024, time.May, 14),
	}}}
}

// Search matches by clean name, and by year when year is not 0.
func (s *Static) Search(_ context.Context, query string, year int) ([]Movie, error) {
	q := releasename.CleanName(query)
	var out []Movie
	for _, m := range s.Movies {
		if (q == "" || strings.Contains(releasename.CleanName(m.Name), q)) && (year == 0 || m.Year == year) {
			out = append(out, m)
		}
	}
	return out, nil
}

// Movie finds one movie by "<source>:<id>".
func (s *Static) Movie(_ context.Context, id string) (Movie, error) {
	src, v, ok := strings.Cut(id, ":")
	if !ok {
		return Movie{}, ErrNotFound
	}
	for _, m := range s.Movies {
		if m.IDs[src] == v {
			return m, nil
		}
	}
	return Movie{}, ErrNotFound
}

// AboveLimit is the answer to a title above the asker's age-rating limit.
const AboveLimit = "This title is rated above what this account may see."

// AgeLimits is what the title-page routes need to know about the asker's
// age-rating limit; the owners of accounts and the Library provide it.
type AgeLimits interface {
	// Max is the asker's age-rating limit, "" for none.
	Max(r *http.Request) string
	// Limited answers 429 and returns true when a limited asker has made
	// too many lookups this minute.
	Limited(w http.ResponseWriter, r *http.Request) bool
	// LibraryRating is the stored rating of a Library title by TMDB id;
	// ok is false when the title is not in the Library.
	LibraryRating(ctx context.Context, kind, tmdbID string) (rating string, ok bool)
}

// ExploreCache is the shared Explore answer cache the title pages read
// through, so a title's details are fetched once for every page.
type ExploreCache interface {
	// Details is a title's details, from the cache while fresh.
	Details(ctx context.Context, kind, id string) (Details, error)
	// Caching sets the cache headers of an answer that depends on who asks
	// ("" for no shared cache).
	Caching(w http.ResponseWriter, r *http.Request, cache string)
}

// MusicAnswers is what an artist's title-page answer needs from music.
type MusicAnswers interface {
	// WriteJSON writes a music answer with its pictures' records.
	WriteJSON(w http.ResponseWriter, r *http.Request, v any)
	// ArtistPicture is the link of an artist picture (kind wide, logo or
	// artist) when the metadata server has one at size.
	ArtistPicture(ctx context.Context, kind, mbid, size string) (link string, ok bool)
}

// ServerReads are the metadata server's per-title reads; a nil func is a
// read the server does not offer.
type ServerReads struct {
	Art          func(ctx context.Context, kind, id string) (ServerArt, error)
	Ratings      func(ctx context.Context, kind, id string) (ServerRatings, error)
	WikiTitle    func(ctx context.Context, kind, id string) (WikiEntity, error)
	WikiPerson   func(ctx context.Context, id, name string) (WikiEntity, error)
	AwardWinners func(ctx context.Context, kind string) ([]AwardWinner, error)
	// ArtLink turns a server picture address into the app's artwork cache
	// link, "" when it is not one.
	ArtLink func(u string) string
}

// UISettings is the app look settings the trailer routes and pass read.
type UISettings interface {
	// AutoplayTrailers says whether heroes and cards may play trailers.
	AutoplayTrailers(ctx context.Context) bool
}

// People tells who asks.
type People interface {
	// UserID is the signed-in person (the owner without sign-in).
	UserID(r *http.Request) int64
}

// TrailerWants is what discover hands the trailer pass: the titles its
// pages show and the languages they were asked in.
type TrailerWants interface {
	// PassTitles are the movies and shows (kind, TMDB id) the pass covers,
	// in page order.
	PassTitles(ctx context.Context) [][2]string
	// PassLanguages are the languages the pass covers, at most
	// TrailerPassLangs.
	PassLanguages(ctx context.Context) []string
	// Warm fills titles' details and extras ahead.
	Warm(lang string, titles [][2]string)
}

// ArtistVideos is what music hands the trailer routes and pass: the
// artists whose music videos are kept ready.
type ArtistVideos interface {
	// On says whether artist music videos can play here.
	On() bool
	// Asks are the artists the pass keeps a video ready for, ranked.
	Asks(ctx context.Context, now time.Time) []string
	// Due says whether the pass looks one of the artist's videos up, and
	// whether that renews a ready one (urgent when it would lapse first).
	Due(ctx context.Context, mbid string, now time.Time) (due, renew, urgent bool)
	// Pass makes the artist's video ready, or renews the ready one.
	Pass(ctx context.Context, mbid string, renew bool) error
	// SaveAsks keeps the noted artists across restarts.
	SaveAsks(ctx context.Context)
	// Ready is the artist's ready video as the artist page answers it.
	Ready(r *http.Request, mbid string) (view any, ok bool)
	// Note notes artists for the pass.
	Note(ctx context.Context, mbids []string)
}
