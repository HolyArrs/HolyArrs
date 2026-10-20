package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
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
	// What Explore scores with; not sent to the app.
	Popularity  float64 `json:"-"`
	VoteAverage float64 `json:"-"`
	VoteCount   int     `json:"-"`
	Released    string  `json:"-"` // YYYY-MM-DD
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
var DiscoverRows = []string{"trending", "popular", "upcoming", "similar", "genre", "services", "best"}

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

// WatchService is one streaming service people can pick on Explore.
type WatchService struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Logo     string `json:"logo,omitempty"`
	Priority int    `json:"priority"`
}

// ServiceLister lists the streaming services TMDB knows for a region.
type ServiceLister interface {
	WatchServices(ctx context.Context, kind, region string) ([]WatchService, error)
}

// ServiceListerFor returns the service list surface selected for ctx.
func ServiceListerFor(ctx context.Context, provider Provider) (ServiceLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(ServiceLister)
	return l, ok
}

// ServiceIDs says whether s is 1 to 20 service ids joined by |.
func ServiceIDs(s string) bool {
	parts := strings.Split(s, "|")
	if s == "" || len(parts) > 20 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 || n > 9999999 || strconv.FormatInt(n, 10) != p {
			return false
		}
	}
	return true
}

func isRegion(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// WatchServices reads TMDB's streaming services for one kind and region,
// best known first (TMDB's own order for that region).
func (t *TMDB) WatchServices(ctx context.Context, kind, region string) ([]WatchService, error) {
	if (kind != "movie" && kind != "show") || !isRegion(region) {
		return nil, fmt.Errorf("kind must be movie or show, region a country code")
	}
	b, err := t.get(ctx, "/watch/providers/"+tvPath(kind), url.Values{"watch_region": {region}, "language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []struct {
			ID         int64          `json:"provider_id"`
			Name       string         `json:"provider_name"`
			Logo       string         `json:"logo_path"`
			Priority   int            `json:"display_priority"`
			Priorities map[string]int `json:"display_priorities"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := []WatchService{}
	for _, r := range res.Results {
		if r.ID <= 0 || r.Name == "" {
			continue
		}
		s := WatchService{ID: r.ID, Name: r.Name, Priority: r.Priority}
		if p, ok := r.Priorities[region]; ok {
			s.Priority = p
		}
		if r.Logo != "" {
			s.Logo = t.images() + "/w92" + r.Logo
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out, nil
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
