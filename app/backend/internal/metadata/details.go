package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	maxBackdrops = 4
	maxVideos    = 12
)

// DetailPerson is one credited person on a details page.
type DetailPerson struct {
	Name      string `json:"name"`
	Role      string `json:"role"` // actor, director, creator
	Character string `json:"character,omitempty"`
	Profile   string `json:"profile,omitempty"`
	// ID is the TMDB person id: actors link to their page; directors and creators also feed More from.
	ID string `json:"id,omitempty"`
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
	Logo           string `json:"logo,omitempty"`
	RuntimeMinutes int    `json:"runtime_minutes,omitempty"`
	Rating         string `json:"rating,omitempty"`
	// Score is TMDB's average vote out of 10 and Votes how many voted.
	Score  float64        `json:"score,omitempty"`
	Votes  int            `json:"votes,omitempty"`
	People []DetailPerson `json:"people"`
	// Moods are up to three mood words from TMDB keywords and genres.
	Moods []string `json:"moods"`
	// Keywords are the title's TMDB keyword names (the Explore strip's
	// keyword taste, and the page's topic tags).
	Keywords []string `json:"keywords,omitempty"`
	// Status is TMDB's word for where the title stands: Released, In
	// Production, Post Production (movies); Returning Series, Ended,
	// Canceled (shows).
	Status string `json:"status,omitempty"`
	// Tagline is the title's one-line tagline.
	Tagline string `json:"tagline,omitempty"`
	// OriginalTitle is the title in its own language, only when it differs.
	OriginalTitle string `json:"original_title,omitempty"`
	// Budget and Revenue are a movie's, in US dollars (0: not known).
	Budget  int64 `json:"budget,omitempty"`
	Revenue int64 `json:"revenue,omitempty"`
	// Studios are the first production companies, in TMDB's order.
	Studios []string `json:"studios,omitempty"`
	// ShowType is TMDB's kind of show (Scripted, Miniseries, Reality…);
	// EpisodeRuntime its usual episode length in minutes (0: not known).
	ShowType       string `json:"show_type,omitempty"`
	EpisodeRuntime int    `json:"episode_runtime,omitempty"`
	// SpokenLanguages are the languages spoken in it, by English name.
	SpokenLanguages []string `json:"spoken_languages,omitempty"`
	// Dates are a movie's US release dates (theaters, digital, disc) or a
	// show's first and last air dates, as YYYY-MM-DD.
	Dates *DetailDates `json:"dates,omitempty"`
	// NextEpisode is a show's next episode to air, when TMDB knows one.
	NextEpisode *DetailNextEpisode `json:"next_episode,omitempty"`
	Seasons     []DetailSeason     `json:"seasons,omitempty"`
	// OriginCountry and OriginalLanguage tell anime apart (LooksAnime).
	OriginCountry    []string       `json:"origin_country,omitempty"`
	OriginalLanguage string         `json:"original_language,omitempty"`
	Similar          []DiscoverItem `json:"similar"`
	// Backdrops are up to maxBackdrops more wide pictures for the hero's
	// slow crossfade, best voted first (the main backdrop not repeated).
	Backdrops []string `json:"backdrops,omitempty"`
	// Videos are the title's YouTube trailers and teasers as TMDB lists
	// them; the app picks one (official trailer, the person's language first).
	Videos []DetailVideo `json:"videos,omitempty"`
	// Collection is the TMDB collection a movie belongs to, with its
	// movies in release order (the movie itself included).
	Collection *DetailCollection `json:"collection,omitempty"`
	// Streaming is, per country, the subscription services TMDB lists the
	// title on; the API answers only the asked region as "streaming".
	Streaming map[string][]WatchService `json:"-"`
	// CollectionMissed says the movie belongs to a collection that could not
	// be read this time (the answer is then kept only briefly).
	CollectionMissed bool `json:"-"`
}

// DetailDates are the dates a page shows: a movie's release in theaters,
// digitally and on disc; a show's first and last air date.
type DetailDates struct {
	Theaters string `json:"theaters,omitempty"`
	Digital  string `json:"digital,omitempty"`
	Physical string `json:"physical,omitempty"`
	FirstAir string `json:"first_air,omitempty"`
	LastAir  string `json:"last_air,omitempty"`
}

// DetailNextEpisode is a show's next episode to air.
type DetailNextEpisode struct {
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date,omitempty"`
}

// maxStudios is how many production companies a page names.
const maxStudios = 3

// DetailCollection is a movie's TMDB collection on a details page.
type DetailCollection struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Parts []DiscoverItem `json:"parts"`
}

// DetailEpisode is one episode of a season on a details page.
type DetailEpisode struct {
	Number   int    `json:"number"`
	Name     string `json:"name,omitempty"`
	Overview string `json:"overview,omitempty"`
	Runtime  int    `json:"runtime,omitempty"`
	Still    string `json:"still,omitempty"`
	AirDay   string `json:"air_day,omitempty"`
}

// SeasonLister lists one season's episodes of a show by TMDB id.
type SeasonLister interface {
	SeasonEpisodes(ctx context.Context, tmdbID string, season int) ([]DetailEpisode, error)
}

// SeasonListerFor returns the season surface selected for ctx.
func SeasonListerFor(ctx context.Context, provider Provider) (SeasonLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(SeasonLister)
	return l, ok
}

// DetailVideo is one YouTube video TMDB lists for a title.
type DetailVideo struct {
	Key      string `json:"key"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type"`
	Official bool   `json:"official"`
	Language string `json:"language,omitempty"`
}

// youTubeKey is the shape of a YouTube video id, so nothing else reaches an embed address.
var youTubeKey = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

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
	appendTo := "release_dates,images,credits,recommendations,videos,keywords,translations"
	if kind == "show" {
		appendTo = "content_ratings,images,credits,recommendations,videos,keywords,translations"
	}
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(n, 10),
		url.Values{"append_to_response": {appendTo}, "include_image_language": {"en,null"}})
	if err != nil {
		return Details{}, err
	}
	var raw struct {
		VoteAverage   float64  `json:"vote_average"`
		VoteCount     int      `json:"vote_count"`
		Origin        []string `json:"origin_country"`
		Language      string   `json:"original_language"`
		Status        string   `json:"status"`
		Tagline       string   `json:"tagline"`
		OriginalTitle string   `json:"original_title"` // movies
		OriginalName  string   `json:"original_name"`  // shows
		Budget        int64    `json:"budget"`
		Revenue       int64    `json:"revenue"`
		ShowType      string   `json:"type"`             // shows: Scripted, Miniseries, Reality…
		EpisodeTimes  []int    `json:"episode_run_time"` // shows, minutes
		Spoken        []struct {
			English string `json:"english_name"`
		} `json:"spoken_languages"`
		Companies []struct {
			Name string `json:"name"`
		} `json:"production_companies"`
		FirstAirDate string `json:"first_air_date"`
		LastAirDate  string `json:"last_air_date"`
		NextEpisode  *struct {
			Season  int    `json:"season_number"`
			Episode int    `json:"episode_number"`
			Name    string `json:"name"`
			AirDate string `json:"air_date"`
		} `json:"next_episode_to_air"`
		ReleaseDates *struct {
			Results []tmdbReleaseCountry `json:"results"`
		} `json:"release_dates"`
		Credits *struct {
			Cast []struct {
				ID          int64  `json:"id"`
				Name        string `json:"name"`
				Character   string `json:"character"`
				ProfilePath string `json:"profile_path"`
			} `json:"cast"`
			Crew []struct {
				ID          int64  `json:"id"`
				Name        string `json:"name"`
				Job         string `json:"job"`
				ProfilePath string `json:"profile_path"`
			} `json:"crew"`
		} `json:"credits"`
		CreatedBy []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			ProfilePath string `json:"profile_path"`
		} `json:"created_by"`
		Keywords *struct {
			Keywords []struct {
				Name string `json:"name"`
			} `json:"keywords"` // movies
			Results []struct {
				Name string `json:"name"`
			} `json:"results"` // shows
		} `json:"keywords"`
		Seasons []struct {
			Number       int    `json:"season_number"`
			EpisodeCount int    `json:"episode_count"`
			AirDate      string `json:"air_date"`
		} `json:"seasons"`
		Recommendations *struct {
			Results []tmdbListItem `json:"results"`
		} `json:"recommendations"`
		Images *struct {
			Backdrops []struct {
				FilePath string  `json:"file_path"`
				Vote     float64 `json:"vote_average"`
			} `json:"backdrops"`
		} `json:"images"`
		Videos *struct {
			Results []struct {
				Key      string `json:"key"`
				Name     string `json:"name"`
				Site     string `json:"site"`
				Type     string `json:"type"`
				Official bool   `json:"official"`
				Language string `json:"iso_639_1"`
			} `json:"results"`
		} `json:"videos"`
		Translations struct {
			List []tmdbTranslation `json:"translations"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	d := Details{Score: float64(int(raw.VoteAverage*10+0.5)) / 10, Votes: raw.VoteCount, OriginCountry: raw.Origin, OriginalLanguage: raw.Language,
		Status: raw.Status, Tagline: strings.TrimSpace(raw.Tagline), Budget: raw.Budget, Revenue: raw.Revenue}
	if kind == "show" {
		d.ShowType = strings.TrimSpace(raw.ShowType)
		if len(raw.EpisodeTimes) > 0 && raw.EpisodeTimes[0] > 0 {
			d.EpisodeRuntime = raw.EpisodeTimes[0]
		}
	}
	for _, l := range raw.Spoken {
		if n := strings.TrimSpace(l.English); n != "" && len(d.SpokenLanguages) < 6 {
			d.SpokenLanguages = append(d.SpokenLanguages, n)
		}
	}
	for _, c := range raw.Companies {
		if len(d.Studios) == maxStudios {
			break
		}
		if n := strings.TrimSpace(c.Name); n != "" {
			d.Studios = append(d.Studios, n)
		}
	}
	if kind == "movie" {
		var m tmdbMovie
		if err := json.Unmarshal(b, &m); err != nil {
			return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		mv := m.movieWithCollection(t.images(), true)
		d.DiscoverItem = DiscoverItem{TMDBID: tmdbID, Kind: kind, Name: mv.Name, Year: mv.Year, Poster: mv.Poster, Backdrop: mv.Backdrop,
			Overview: mv.Overview, Genres: append([]string{}, mv.Genres...)}
		d.Logo, d.RuntimeMinutes, d.Rating = mv.Logo, mv.Runtime, mv.Rating
		if o := strings.TrimSpace(raw.OriginalTitle); o != "" && !strings.EqualFold(o, mv.Name) {
			d.OriginalTitle = o
		}
		if raw.ReleaseDates != nil {
			d.Dates = releaseDates(raw.ReleaseDates.Results)
		}
		if mv.Collection != nil {
			var read bool
			d.Collection, read = t.collection(ctx, mv.Collection.ID, mv.Collection.Name, names)
			d.CollectionMissed = !read
		}
	} else {
		var m tmdbShow
		if err := json.Unmarshal(b, &m); err != nil {
			return Details{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		sh := m.show(t.images())
		d.DiscoverItem = DiscoverItem{TMDBID: tmdbID, Kind: kind, Name: sh.Name, Year: sh.Year, Poster: sh.Poster, Backdrop: sh.Backdrop,
			Overview: sh.Overview, Genres: append([]string{}, sh.Genres...)}
		d.Logo, d.RuntimeMinutes, d.Rating = sh.Logo, sh.Runtime, sh.Rating
		if o := strings.TrimSpace(raw.OriginalName); o != "" && !strings.EqualFold(o, sh.Name) {
			d.OriginalTitle = o
		}
		if raw.FirstAirDate != "" || raw.LastAirDate != "" {
			d.Dates = &DetailDates{FirstAir: raw.FirstAirDate, LastAir: raw.LastAirDate}
		}
		if e := raw.NextEpisode; e != nil && e.AirDate != "" {
			d.NextEpisode = &DetailNextEpisode{Season: e.Season, Episode: e.Episode, Name: strings.TrimSpace(e.Name), AirDate: e.AirDate}
		}
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
			d.People = append(d.People, DetailPerson{Name: c.Name, Role: "actor", Character: c.Character, Profile: profile(c.ProfilePath), ID: personID(c.ID)})
		}
		for _, c := range raw.Credits.Crew {
			if c.Job == "Director" {
				d.People = append(d.People, DetailPerson{Name: c.Name, Role: "director", Profile: profile(c.ProfilePath), ID: personID(c.ID)})
			}
		}
	}
	for _, c := range raw.CreatedBy {
		d.People = append(d.People, DetailPerson{Name: c.Name, Role: "creator", Profile: profile(c.ProfilePath), ID: personID(c.ID)})
	}
	var keywords []string
	if raw.Keywords != nil {
		for _, k := range raw.Keywords.Keywords {
			keywords = append(keywords, k.Name)
		}
		for _, k := range raw.Keywords.Results {
			keywords = append(keywords, k.Name)
		}
	}
	d.Moods = Moods(keywords, d.Genres)
	d.Keywords = keywords
	if raw.Images != nil {
		bs := raw.Images.Backdrops
		sort.SliceStable(bs, func(i, j int) bool { return bs[i].Vote > bs[j].Vote })
		for _, b := range bs {
			if len(d.Backdrops) == maxBackdrops {
				break
			}
			u := t.images() + "/w1280" + b.FilePath
			if b.FilePath == "" || u == d.Backdrop || slices.Contains(d.Backdrops, u) {
				continue
			}
			d.Backdrops = append(d.Backdrops, u)
		}
	}
	if raw.Videos != nil {
		for _, v := range raw.Videos.Results {
			if len(d.Videos) == maxVideos {
				break
			}
			if v.Site != "YouTube" || (v.Type != "Trailer" && v.Type != "Teaser") || !youTubeKey.MatchString(v.Key) {
				continue
			}
			d.Videos = append(d.Videos, DetailVideo{Key: v.Key, Name: v.Name, Type: v.Type, Official: v.Official, Language: v.Language})
		}
	}
	d.Streaming = t.streaming(ctx, kind, n)
	d.Similar = []DiscoverItem{}
	if raw.Recommendations != nil {
		d.Similar = t.items(kind, raw.Recommendations.Results, names, maxSimilar)
	}
	// Kept even when empty: the title's texts are then known to be English only.
	t.keepTexts(ctx, kind, tmdbID, titleTexts(raw.Translations.List))
	return d, nil
}

// tmdbReleaseCountry is one country's release dates in TMDB's
// release_dates answer.
type tmdbReleaseCountry struct {
	Country string `json:"iso_3166_1"`
	Dates   []struct {
		Date string `json:"release_date"`
		Type int    `json:"type"`
	} `json:"release_dates"`
}

// releaseDates picks a movie's US release in theaters (TMDB types 2 and 3),
// digitally (4) and on disc (5), the earliest of each; with no US entry, the
// earliest of each type anywhere. Nothing known answers nil.
func releaseDates(results []tmdbReleaseCountry) *DetailDates {
	pick := func(only string) *DetailDates {
		out := DetailDates{}
		earliest := func(cur *string, day string) {
			if len(day) >= 10 && (*cur == "" || day[:10] < *cur) {
				*cur = day[:10]
			}
		}
		for _, c := range results {
			if only != "" && c.Country != only {
				continue
			}
			for _, d := range c.Dates {
				switch d.Type {
				case 2, 3:
					earliest(&out.Theaters, d.Date)
				case 4:
					earliest(&out.Digital, d.Date)
				case 5:
					earliest(&out.Physical, d.Date)
				}
			}
		}
		if out == (DetailDates{}) {
			return nil
		}
		return &out
	}
	if d := pick("US"); d != nil {
		return d
	}
	return pick("")
}

// personID is a TMDB person id as text; empty when unknown.
func personID(n int64) string {
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// collection looks a TMDB collection up; nil when it has fewer than two
// movies, or when it cannot be read (read false: the page shows without it
// and asks again soon).
func (t *TMDB) collection(ctx context.Context, id, name string, names map[int64]string) (*DetailCollection, bool) {
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return nil, true
	}
	b, err := t.get(ctx, "/collection/"+id, nil)
	if err != nil {
		return nil, false
	}
	var res struct {
		Name  string         `json:"name"`
		Parts []tmdbListItem `json:"parts"`
	}
	if json.Unmarshal(b, &res) != nil {
		return nil, false
	}
	parts := t.items("movie", res.Parts, names, maxCollection)
	sort.SliceStable(parts, func(i, j int) bool {
		a, b := parts[i].Year, parts[j].Year
		if a == 0 || b == 0 {
			return a != 0
		}
		return a < b
	})
	if len(parts) < 2 {
		return nil, true
	}
	if res.Name != "" {
		name = res.Name
	}
	return &DetailCollection{ID: id, Name: name, Parts: parts}, true
}

// SeasonEpisodes lists one season of a show for its details page.
func (t *TMDB) SeasonEpisodes(ctx context.Context, tmdbID string, season int) ([]DetailEpisode, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || season < 0 || season > 999 {
		return nil, ErrNotFound
	}
	b, err := t.get(ctx, "/tv/"+strconv.FormatInt(n, 10)+"/season/"+strconv.Itoa(season), nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Episodes []struct {
			Number   int     `json:"episode_number"`
			Name     string  `json:"name"`
			Overview string  `json:"overview"`
			AirDate  *string `json:"air_date"`
			Runtime  int     `json:"runtime"`
			Still    string  `json:"still_path"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := []DetailEpisode{}
	for _, e := range res.Episodes {
		ep := DetailEpisode{Number: e.Number, Name: e.Name, Overview: e.Overview, Runtime: e.Runtime}
		if e.Still != "" {
			ep.Still = t.images() + "/w780" + e.Still
		}
		if e.AirDate != nil {
			_, ep.AirDay = dayEnd(*e.AirDate)
		}
		out = append(out, ep)
	}
	return out, nil
}
