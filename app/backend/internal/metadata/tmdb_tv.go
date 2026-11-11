package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

type tmdbShow struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	FirstAirDate     string `json:"first_air_date"`
	Overview         string `json:"overview"`
	OriginalLanguage string `json:"original_language"`
	Status           string `json:"status"`
	PosterPath       string `json:"poster_path"`
	tmdbExtras
	ContentRatings *struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
	RunTimes []int `json:"episode_run_time"`
	Networks []struct {
		Name string `json:"name"`
	} `json:"networks"`
	ExternalIDs *struct {
		IMDb string `json:"imdb_id"`
		TVDB int64  `json:"tvdb_id"`
	} `json:"external_ids"`
	Seasons []struct {
		Number int    `json:"season_number"`
		Name   string `json:"name"`
	} `json:"seasons"`
	AltTitles *struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	} `json:"alternative_titles,omitempty"`
	Translations *struct {
		List []struct {
			Data struct {
				Name string `json:"name"`
			} `json:"data"`
		} `json:"translations"`
	} `json:"translations,omitempty"`
}

// tmdbStatus maps TMDB's show status to ours.
func tmdbStatus(s string) string {
	switch s {
	case "Ended", "Canceled":
		return "ended"
	case "Planned", "Pilot":
		return "announced"
	}
	return "running"
}

func (m tmdbShow) show(images string) Show {
	out := Show{Source: "tmdb", Name: m.Name, Overview: m.Overview, OriginalLanguage: m.OriginalLanguage, Year: yearOf(m.FirstAirDate),
		Status: tmdbStatus(m.Status), IDs: map[string]string{"tmdb": strconv.FormatInt(m.ID, 10)}}
	if m.PosterPath != "" {
		out.Poster = images + "/w500" + m.PosterPath
	}
	if len(m.Networks) > 0 {
		out.Network = m.Networks[0].Name
	}
	if len(m.RunTimes) > 0 {
		out.Runtime = m.RunTimes[0]
	}
	if x := m.ExternalIDs; x != nil {
		if x.IMDb != "" {
			out.IDs["imdb"] = x.IMDb
		}
		if x.TVDB > 0 {
			out.IDs["tvdb"] = strconv.FormatInt(x.TVDB, 10)
		}
	}
	out.Backdrop, out.Logo, out.Genres, out.People = m.tmdbExtras.pictures(images)
	out.Rating = m.usRating()
	out.Score, out.Votes = m.tmdbExtras.score()
	for _, s := range m.Seasons {
		out.Seasons = append(out.Seasons, Season{Number: s.Number, Name: s.Name})
	}
	out.OtherNames = append(out.OtherNames, m.OriginalName)
	if m.AltTitles != nil {
		for _, a := range m.AltTitles.Results {
			out.OtherNames = append(out.OtherNames, a.Title)
		}
	}
	if m.Translations != nil {
		for _, tr := range m.Translations.List {
			out.OtherNames = append(out.OtherNames, tr.Data.Name)
		}
	}
	return out
}

// SearchShows finds shows by name on TMDB.
func (t *TMDB) SearchShows(ctx context.Context, query string) ([]Show, error) {
	b, err := t.get(ctx, "/search/tv", url.Values{"query": {query}, "include_adult": {"false"}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []tmdbShow `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := make([]Show, 0, len(res.Results))
	for _, m := range res.Results {
		out = append(out, m.show(t.images()))
	}
	return out, nil
}

// Show takes "tmdb:<id>", or "tvdb:<id>" or "imdb:<tt id>" found through
// TMDB's find, and returns the show with every season's episodes.
func (t *TMDB) Show(ctx context.Context, id string) (Show, error) {
	kind, v, ok := splitID(id)
	if !ok {
		return Show{}, ErrShowNotFound
	}
	switch kind {
	case "tvdb", "imdb":
		b, err := t.get(ctx, "/find/"+url.PathEscape(v), url.Values{"external_source": {kind + "_id"}})
		if err != nil {
			return Show{}, notFoundShow(err)
		}
		var res struct {
			TV []tmdbShow `json:"tv_results"`
		}
		if err := json.Unmarshal(b, &res); err != nil || len(res.TV) == 0 {
			return Show{}, ErrShowNotFound
		}
		return t.Show(ctx, "tmdb:"+strconv.FormatInt(res.TV[0].ID, 10))
	case "tmdb":
	default:
		return Show{}, ErrShowNotFound
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return Show{}, ErrShowNotFound
	}
	base := "/tv/" + strconv.FormatInt(n, 10)
	b, err := t.get(ctx, base, url.Values{"append_to_response": {"external_ids,images,credits,content_ratings,translations,alternative_titles"}, "include_image_language": {"en,null"}})
	if err != nil {
		return Show{}, notFoundShow(err)
	}
	var m tmdbShow
	if err := json.Unmarshal(b, &m); err != nil {
		return Show{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := m.show(t.images())
	for _, se := range m.Seasons {
		b, err := t.get(ctx, base+"/season/"+strconv.Itoa(se.Number), nil)
		if errors.Is(err, ErrNotFound) {
			continue // a season listed but not served yet
		}
		if err != nil {
			return Show{}, err
		}
		var res struct {
			Episodes []struct {
				ID      int64   `json:"id"`
				Season  int     `json:"season_number"`
				Number  int     `json:"episode_number"`
				Name    string  `json:"name"`
				AirDate *string `json:"air_date"`
				Runtime int     `json:"runtime"`
				Still   string  `json:"still_path"`
			} `json:"episodes"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return Show{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		for _, e := range res.Episodes {
			ep := Episode{Season: e.Season, Number: e.Number, Name: e.Name, Runtime: e.Runtime, ID: strconv.FormatInt(e.ID, 10)}
			if e.Still != "" {
				ep.Still = t.images() + "/w780" + e.Still
			}
			if e.AirDate != nil {
				ep.AirDate, ep.AirDay = dayEnd(*e.AirDate)
			}
			out.Episodes = append(out.Episodes, ep)
		}
	}
	out.finish()
	return out, nil
}

func notFoundShow(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrShowNotFound
	}
	return err
}

// usRating is the first US content rating.
func (m tmdbShow) usRating() string {
	if m.ContentRatings != nil {
		for _, c := range m.ContentRatings.Results {
			if c.Country == "US" && c.Rating != "" {
				return c.Rating
			}
		}
	}
	return ""
}
