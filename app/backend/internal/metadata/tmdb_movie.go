package metadata

import (
	"strconv"
	"strings"
	"time"
)

type tmdbMovie struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	ReleaseDate      string `json:"release_date"`
	Overview         string `json:"overview"`
	OriginalLanguage string `json:"original_language"`
	Runtime          int    `json:"runtime"`
	PosterPath       string `json:"poster_path"`
	tmdbExtras
	IMDbID              string `json:"imdb_id"`
	BelongsToCollection *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
	ProductionCompanies []struct {
		Name string `json:"name"`
	} `json:"production_companies"`
	AltTitles *struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
	} `json:"alternative_titles,omitempty"`
	Translations *struct {
		List []struct {
			Data struct {
				Title string `json:"title"`
			} `json:"data"`
		} `json:"translations"`
	} `json:"translations,omitempty"`
	ReleaseDates *struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Date          string `json:"release_date"`
				Type          int    `json:"type"`
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates,omitempty"`
}

// TMDB release types: 1 premiere, 2 limited cinema, 3 cinema, 4 digital,
// 5 physical, 6 TV.
const (
	relLimited  = 2
	relCinema   = 3
	relDigital  = 4
	relPhysical = 5
)

// dates picks the cinema, digital and physical dates: the United States
// dates when it has them, else the earliest date of that type anywhere.
// A cinema date falls back to a limited cinema run, then to the main
// release date.
func (m tmdbMovie) dates() (cinema, digital, physical *time.Time) {
	us, anywhere := map[int]time.Time{}, map[int]time.Time{}
	keep := func(into map[int]time.Time, typ int, t time.Time) {
		if cur, seen := into[typ]; !seen || t.Before(cur) {
			into[typ] = t
		}
	}
	if m.ReleaseDates != nil {
		for _, c := range m.ReleaseDates.Results {
			for _, d := range c.Dates {
				t, ok := parseDay(d.Date)
				if !ok {
					continue
				}
				keep(anywhere, d.Type, t)
				if c.Country == "US" {
					keep(us, d.Type, t)
				}
			}
		}
	}
	pick := func(typ int) *time.Time {
		if t, ok := us[typ]; ok {
			return &t
		}
		if t, ok := anywhere[typ]; ok {
			return &t
		}
		return nil
	}
	cinema, digital, physical = pick(relCinema), pick(relDigital), pick(relPhysical)
	if cinema == nil {
		cinema = pick(relLimited)
	}
	if cinema == nil {
		if t, ok := parseDay(m.ReleaseDate); ok {
			cinema = &t
		}
	}
	return cinema, digital, physical
}

// certification is the US certification, preferring the cinema release.
func (m tmdbMovie) certification() string {
	if m.ReleaseDates == nil {
		return ""
	}
	best, bestType := "", 0
	for _, c := range m.ReleaseDates.Results {
		if c.Country != "US" {
			continue
		}
		for _, d := range c.Dates {
			if d.Certification == "" {
				continue
			}
			if best == "" || d.Type == relCinema || (bestType != relCinema && d.Type < bestType) {
				best, bestType = d.Certification, d.Type
			}
		}
	}
	return best
}

// tmdbExtras are the detail fields movies and shows share: wide picture,
// genres, logos and credits (the last two only with append_to_response).
type tmdbExtras struct {
	BackdropPath string `json:"backdrop_path"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	// GenreIDs are what list and search answers carry in place of names.
	GenreIDs []int64 `json:"genre_ids"`
	Images   *struct {
		Logos []struct {
			FilePath string  `json:"file_path"`
			Language *string `json:"iso_639_1"`
		} `json:"logos"`
	} `json:"images"`
	Credits *struct {
		Cast []struct {
			Name string `json:"name"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

// score is the members' average to one decimal and its vote count.
func (x tmdbExtras) score() (float64, int) {
	return float64(int(x.VoteAverage*10+0.5)) / 10, x.VoteCount
}

// tmdbGenreNames are TMDB's fixed movie and TV genre ids with their English
// names (the two lists share the ids they have in common).
var tmdbGenreNames = map[int64]string{
	28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama",
	10751: "Family", 14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music", 9648: "Mystery", 10749: "Romance",
	878: "Science Fiction", 10770: "TV Movie", 53: "Thriller", 10752: "War", 37: "Western",
	10759: "Action & Adventure", 10762: "Kids", 10763: "News", 10764: "Reality", 10765: "Sci-Fi & Fantasy",
	10766: "Soap", 10767: "Talk", 10768: "War & Politics",
}

// maxCast is how many cast names a Title keeps.
const maxCast = 10

// pictures returns the backdrop (w1280), the logo (w500 PNG: English
// first, then one with no language), the genre names and the people (top
// cast, then directors not already listed).
func (x tmdbExtras) pictures(images string) (backdrop, logo string, genres, people []string) {
	if x.BackdropPath != "" {
		backdrop = images + "/w1280" + x.BackdropPath
	}
	if x.Images != nil {
		var none string
		for _, l := range x.Images.Logos {
			if !strings.HasSuffix(strings.ToLower(l.FilePath), ".png") {
				continue
			}
			if l.Language != nil && *l.Language == "en" {
				logo = images + "/w500" + l.FilePath
				break
			}
			if l.Language == nil && none == "" {
				none = images + "/w500" + l.FilePath
			}
		}
		if logo == "" {
			logo = none
		}
	}
	for _, g := range x.Genres {
		if g.Name != "" {
			genres = append(genres, g.Name)
		}
	}
	if len(x.Genres) == 0 {
		for _, id := range x.GenreIDs {
			if name := tmdbGenreNames[id]; name != "" {
				genres = append(genres, name)
			}
		}
	}
	if x.Credits != nil {
		seen := map[string]bool{}
		for _, c := range x.Credits.Cast {
			if len(people) == maxCast {
				break
			}
			if c.Name != "" && !seen[c.Name] {
				seen[c.Name] = true
				people = append(people, c.Name)
			}
		}
		for _, c := range x.Credits.Crew {
			if c.Job == "Director" && c.Name != "" && !seen[c.Name] {
				seen[c.Name] = true
				people = append(people, c.Name)
			}
		}
	}
	return backdrop, logo, genres, people
}

// directors returns the directors' names in credit order (empty without credits).
func (x tmdbExtras) directors() []string {
	var out []string
	if x.Credits == nil {
		return out
	}
	seen := map[string]bool{}
	for _, c := range x.Credits.Crew {
		if c.Job == "Director" && c.Name != "" && !seen[c.Name] {
			seen[c.Name] = true
			out = append(out, c.Name)
		}
	}
	return out
}

// parseDay reads a YYYY-MM-DD date or a full time, as UTC midnight of that day.
func parseDay(s string) (time.Time, bool) {
	if len(s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return t, err == nil
}

func (m tmdbMovie) movie(images string) Movie {
	return m.movieWithCollection(images, false)
}

func (m tmdbMovie) movieWithCollection(images string, collectionKnown bool) Movie {
	out := Movie{Name: m.Title, Overview: m.Overview, OriginalLanguage: m.OriginalLanguage, Runtime: m.Runtime,
		IDs: map[string]string{"tmdb": strconv.FormatInt(m.ID, 10)}, CollectionKnown: collectionKnown}
	if len(m.ReleaseDate) >= 4 {
		out.Year, _ = strconv.Atoi(m.ReleaseDate[:4])
	}
	if m.IMDbID != "" {
		out.IDs["imdb"] = m.IMDbID
	}
	if m.PosterPath != "" {
		out.Poster = images + "/w500" + m.PosterPath
	}
	if collectionKnown && m.BelongsToCollection != nil {
		out.Collection = &MovieCollection{ID: strconv.FormatInt(m.BelongsToCollection.ID, 10), Name: m.BelongsToCollection.Name}
	}
	out.CinemaDate, out.DigitalDate, out.PhysicalDate = m.dates()
	out.Backdrop, out.Logo, out.Genres, out.People = m.tmdbExtras.pictures(images)
	out.Directors = m.tmdbExtras.directors()
	out.Score, out.Votes = m.tmdbExtras.score()
	for _, c := range m.ProductionCompanies {
		if c.Name != "" {
			out.Studio = c.Name
			break
		}
	}
	out.Rating = m.certification()
	out.OtherNames = append(out.OtherNames, m.OriginalTitle)
	if m.AltTitles != nil {
		for _, a := range m.AltTitles.Titles {
			out.OtherNames = append(out.OtherNames, a.Title)
		}
	}
	if m.Translations != nil {
		for _, tr := range m.Translations.List {
			out.OtherNames = append(out.OtherNames, tr.Data.Title)
		}
	}
	return out
}
