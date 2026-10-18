package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ErrRateLimited means TMDB kept answering "too many requests".
var ErrRateLimited = errors.New("TMDB: rate limited; try again shortly")

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
}

// DetailPerson is one credited person on a details page.
type DetailPerson struct {
	Name      string `json:"name"`
	Role      string `json:"role"` // actor, director, creator
	Character string `json:"character,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

// DetailSeason is one season of a show on a details page.
type DetailSeason struct {
	Number       int `json:"number"`
	EpisodeCount int `json:"episode_count"`
	Year         int `json:"year,omitempty"`
}

// Details is a movie or show looked up for its page, in the Library or not.
type Details struct {
	DiscoverItem
	Logo           string         `json:"logo,omitempty"`
	RuntimeMinutes int            `json:"runtime_minutes,omitempty"`
	Rating         string         `json:"rating,omitempty"`
	People         []DetailPerson `json:"people"`
	Seasons        []DetailSeason `json:"seasons,omitempty"`
	Similar        []DiscoverItem `json:"similar"`
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

// DiscoverRows are the row names Discover takes.
var DiscoverRows = []string{"trending", "popular", "upcoming", "similar", "genre"}

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
			Year: yearOf(m.ReleaseDate)}
		if kind == "show" {
			it.Name, it.Year = m.Name, yearOf(m.FirstAirDate)
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

// Details looks one movie or show up for its page, with credits and the
// recommended titles.
func (t *TMDB) Details(ctx context.Context, kind, tmdbID string) (Details, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return Details{}, ErrNotFound
	}
	names, err := t.genreNames(ctx, kind)
	if err != nil {
		return Details{}, err
	}
	appendTo := "release_dates,images,credits,recommendations"
	if kind == "show" {
		appendTo = "content_ratings,images,credits,recommendations"
	}
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(n, 10),
		url.Values{"append_to_response": {appendTo}, "include_image_language": {"en,null"}})
	if err != nil {
		return Details{}, err
	}
	var raw struct {
		Credits *struct {
			Cast []struct {
				Name        string `json:"name"`
				Character   string `json:"character"`
				ProfilePath string `json:"profile_path"`
			} `json:"cast"`
			Crew []struct {
				Name        string `json:"name"`
				Job         string `json:"job"`
				ProfilePath string `json:"profile_path"`
			} `json:"crew"`
		} `json:"credits"`
		CreatedBy []struct {
			Name        string `json:"name"`
			ProfilePath string `json:"profile_path"`
		} `json:"created_by"`
		Seasons []struct {
			Number       int    `json:"season_number"`
			EpisodeCount int    `json:"episode_count"`
			AirDate      string `json:"air_date"`
		} `json:"seasons"`
		Recommendations *struct {
			Results []tmdbListItem `json:"results"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	var d Details
	if kind == "movie" {
		var m tmdbMovie
		if err := json.Unmarshal(b, &m); err != nil {
			return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		mv := m.movieWithCollection(t.images(), true)
		d.DiscoverItem = DiscoverItem{TMDBID: tmdbID, Kind: kind, Name: mv.Name, Year: mv.Year, Poster: mv.Poster, Backdrop: mv.Backdrop,
			Overview: mv.Overview, Genres: append([]string{}, mv.Genres...)}
		d.Logo, d.RuntimeMinutes, d.Rating = mv.Logo, mv.Runtime, mv.Rating
	} else {
		var m tmdbShow
		if err := json.Unmarshal(b, &m); err != nil {
			return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		sh := m.show(t.images())
		d.DiscoverItem = DiscoverItem{TMDBID: tmdbID, Kind: kind, Name: sh.Name, Year: sh.Year, Poster: sh.Poster, Backdrop: sh.Backdrop,
			Overview: sh.Overview, Genres: append([]string{}, sh.Genres...)}
		d.Logo, d.RuntimeMinutes, d.Rating = sh.Logo, sh.Runtime, sh.Rating
		for _, s := range raw.Seasons {
			d.Seasons = append(d.Seasons, DetailSeason{Number: s.Number, EpisodeCount: s.EpisodeCount, Year: yearOf(s.AirDate)})
		}
	}
	profile := func(p string) string {
		if p == "" {
			return ""
		}
		return t.images() + "/w185" + p
	}
	d.People = []DetailPerson{}
	if raw.Credits != nil {
		for i, c := range raw.Credits.Cast {
			if i == maxCast {
				break
			}
			d.People = append(d.People, DetailPerson{Name: c.Name, Role: "actor", Character: c.Character, Profile: profile(c.ProfilePath)})
		}
		for _, c := range raw.Credits.Crew {
			if c.Job == "Director" {
				d.People = append(d.People, DetailPerson{Name: c.Name, Role: "director", Profile: profile(c.ProfilePath)})
			}
		}
	}
	for _, c := range raw.CreatedBy {
		d.People = append(d.People, DetailPerson{Name: c.Name, Role: "creator", Profile: profile(c.ProfilePath)})
	}
	d.Similar = []DiscoverItem{}
	if raw.Recommendations != nil {
		d.Similar = t.items(kind, raw.Recommendations.Results, names, maxSimilar)
	}
	return d, nil
}
