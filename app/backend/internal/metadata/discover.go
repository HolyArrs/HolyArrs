package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrRateLimited means TMDB kept answering "too many requests".
var ErrRateLimited = errors.New("the movie and show details are busy; try again shortly")

// DiscoverItem is one movie or show in a discover row.
type DiscoverItem struct {
	TMDBID   string   `json:"tmdb_id"`
	Kind     string   `json:"kind"` // movie, show
	Name     string   `json:"name"`
	Year     int      `json:"year,omitempty"`
	Poster   string   `json:"poster,omitempty"`
	Backdrop string   `json:"backdrop,omitempty"`
	Overview string   `json:"overview,omitempty"`
	Genres   []string `json:"genres"`
	// What Explore scores with; not sent to the app.
	Popularity  float64 `json:"-"`
	VoteAverage float64 `json:"-"`
	VoteCount   int     `json:"-"`
	Released    string  `json:"-"` // YYYY-MM-DD
}

// Discoverer lists discover rows and looks up details pages. Kind is
// "movie" or "show"; ref is a TMDB id (similar); genre is a genre name.
type Discoverer interface {
	Discover(ctx context.Context, row, kind, ref, genre string) ([]DiscoverItem, error)
	Details(ctx context.Context, kind, tmdbID string) (Details, error)
}

// DiscovererFor returns the discover surface selected for ctx, when the
// chosen provider has one (TMDB does; the lab record does not).
func DiscovererFor(ctx context.Context, provider Provider) (Discoverer, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	d, ok := provider.(Discoverer)
	return d, ok
}

// BestQuery is the TMDB discover query for best new releases: out in the
// last 24 months, rated 7.5 or more by at least 300 people, best rated first.
func BestQuery(kind string, now time.Time) url.Values {
	from := now.UTC().AddDate(-2, 0, 0).Format("2006-01-02")
	date := "primary_release_date.gte"
	if kind == "show" {
		date = "first_air_date.gte"
	}
	return url.Values{date: {from}, "vote_average.gte": {"7.5"}, "vote_count.gte": {"300"}, "sort_by": {"vote_average.desc"}, "include_adult": {"false"}}
}

// maxDiscover caps a row before the Library join drops Never-add items.
const maxDiscover = 40

// maxSimilar caps the similar list on a details page.
const maxSimilar = 12

type tmdbListItem struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	GenreIDs     []int64 `json:"genre_ids"`
	Adult        bool    `json:"adult"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	MediaType    string  `json:"media_type"`
}

func tvPath(kind string) string {
	if kind == "show" {
		return "tv"
	}
	return "movie"
}

// genreNames maps TMDB genre ids to names for a kind.
func (t *TMDB) genreNames(ctx context.Context, kind string) (map[int64]string, error) {
	b, err := t.get(ctx, "/genre/"+tvPath(kind)+"/list", url.Values{"language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Genres []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"genres"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := map[int64]string{}
	for _, g := range res.Genres {
		out[g.ID] = g.Name
	}
	return out, nil
}

func (t *TMDB) items(kind string, list []tmdbListItem, names map[int64]string, n int) []DiscoverItem {
	out := []DiscoverItem{}
	for _, m := range list {
		if len(out) == n {
			break
		}
		if m.Adult || m.ID <= 0 {
			continue
		}
		it := DiscoverItem{TMDBID: strconv.FormatInt(m.ID, 10), Kind: kind, Name: m.Title, Overview: m.Overview, Genres: []string{},
			Year: yearOf(m.ReleaseDate), Released: m.ReleaseDate, Popularity: m.Popularity, VoteAverage: m.VoteAverage, VoteCount: m.VoteCount}
		if kind == "show" {
			it.Name, it.Year, it.Released = m.Name, yearOf(m.FirstAirDate), m.FirstAirDate
		}
		if m.PosterPath != "" {
			it.Poster = t.images() + "/w342" + m.PosterPath
		}
		if m.BackdropPath != "" {
			it.Backdrop = t.images() + "/w1280" + m.BackdropPath
		}
		for _, g := range m.GenreIDs {
			if s := names[g]; s != "" {
				it.Genres = append(it.Genres, s)
			}
		}
		out = append(out, it)
	}
	return out
}

// Discover reads one row of one kind from TMDB.
func (t *TMDB) Discover(ctx context.Context, row, kind, ref, genre string) ([]DiscoverItem, error) {
	if kind != "movie" && kind != "show" {
		return nil, fmt.Errorf("kind must be movie or show")
	}
	names, err := t.genreNames(ctx, kind)
	if err != nil {
		return nil, err
	}
	p := tvPath(kind)
	var path string
	q := url.Values{"language": {"en-US"}}
	switch row {
	case "trending":
		path = "/trending/" + p + "/week"
	case "popular":
		path = "/" + p + "/popular"
	case "upcoming":
		path = "/movie/upcoming"
		if kind == "show" {
			path = "/tv/on_the_air"
		}
	case "similar":
		n, err := strconv.ParseInt(ref, 10, 64)
		if err != nil || n <= 0 {
			return nil, ErrNotFound
		}
		path = "/" + p + "/" + strconv.FormatInt(n, 10) + "/recommendations"
	case "genre":
		id := int64(0)
		for k, v := range names {
			if strings.EqualFold(v, genre) {
				id = k
			}
		}
		if id == 0 {
			return []DiscoverItem{}, nil
		}
		path = "/discover/" + p
		q.Set("with_genres", strconv.FormatInt(id, 10))
		q.Set("sort_by", "popularity.desc")
		q.Set("include_adult", "false")
	case "best":
		path = "/discover/" + p
		for k, v := range BestQuery(kind, time.Now()) {
			q[k] = v
		}
	case "services":
		// ref: streaming service ids joined by |; genre: the region.
		if !ServiceIDs(ref) || !isRegion(genre) {
			return nil, fmt.Errorf("services needs service ids and a region")
		}
		path = "/discover/" + p
		q.Set("with_watch_providers", ref)
		q.Set("watch_region", genre)
		q.Set("with_watch_monetization_types", "flatrate")
		q.Set("sort_by", "popularity.desc")
		q.Set("include_adult", "false")
	default:
		return nil, fmt.Errorf("unknown row %q", row)
	}
	b, err := t.get(ctx, path, q)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []tmdbListItem `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	return t.items(kind, res.Results, names, maxDiscover), nil
}

// maxStreaming caps the services listed per region; maxCollection the
// movies of a collection.
const (
	maxStreaming  = 8
	maxCollection = 20
)
