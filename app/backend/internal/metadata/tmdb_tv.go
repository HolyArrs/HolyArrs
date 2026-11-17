package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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

// EpisodeGroupInfo is one of TMDB's other episode orders of a show. Type
// is TMDB's: 1 original air, 2 absolute, 3 DVD, 4 digital, 5 story arc,
// 6 production, 7 TV.
type EpisodeGroupInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Seasons  int    `json:"seasons"`
	Episodes int    `json:"episodes"`
}

// EpisodeGroup is one TMDB episode order with its seasons.
type EpisodeGroup struct {
	ID      string
	Name    string
	Type    int
	Seasons []GroupSeason
}

// GroupSeason is one season of an episode order; Order counts from 0.
type GroupSeason struct {
	Name     string
	Order    int
	Episodes []GroupEpisode
}

// GroupEpisode is one episode inside a group season: its TMDB episode id
// and its place there, counted from 0.
type GroupEpisode struct {
	ID    string
	Order int
}

// EpisodeGroupSource reads TMDB's episode orders.
type EpisodeGroupSource interface {
	EpisodeGroups(ctx context.Context, tmdbID string) ([]EpisodeGroupInfo, error)
	EpisodeGroup(ctx context.Context, id string) (EpisodeGroup, error)
}

var groupID = regexp.MustCompile(`^[0-9a-f]{24}$`)

// ValidGroupID reports whether id looks like a TMDB episode group id.
func ValidGroupID(id string) bool { return groupID.MatchString(id) }

// EpisodeGroups lists the episode orders TMDB has for a show; empty when none.
func (t *TMDB) EpisodeGroups(ctx context.Context, tmdbID string) ([]EpisodeGroupInfo, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 {
		return nil, ErrShowNotFound
	}
	b, err := t.get(ctx, "/tv/"+strconv.FormatInt(n, 10)+"/episode_groups", nil)
	if err != nil {
		return nil, notFoundShow(err)
	}
	var res struct {
		Results []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     int    `json:"type"`
			Groups   int    `json:"group_count"`
			Episodes int    `json:"episode_count"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := []EpisodeGroupInfo{}
	for _, g := range res.Results {
		if ValidGroupID(g.ID) {
			out = append(out, EpisodeGroupInfo{ID: g.ID, Name: g.Name, Type: g.Type, Seasons: g.Groups, Episodes: g.Episodes})
		}
	}
	return out, nil
}

// EpisodeGroup reads one TMDB episode order.
func (t *TMDB) EpisodeGroup(ctx context.Context, id string) (EpisodeGroup, error) {
	if !ValidGroupID(id) {
		return EpisodeGroup{}, ErrNotFound
	}
	b, err := t.get(ctx, "/tv/episode_group/"+id, nil)
	if err != nil {
		return EpisodeGroup{}, err
	}
	var res struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Type   int    `json:"type"`
		Groups []struct {
			Name     string `json:"name"`
			Order    int    `json:"order"`
			Episodes []struct {
				ID    int64 `json:"id"`
				Order int   `json:"order"`
			} `json:"episodes"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return EpisodeGroup{}, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := EpisodeGroup{ID: res.ID, Name: res.Name, Type: res.Type}
	for _, g := range res.Groups {
		s := GroupSeason{Name: g.Name, Order: g.Order}
		for _, e := range g.Episodes {
			s.Episodes = append(s.Episodes, GroupEpisode{ID: strconv.FormatInt(e.ID, 10), Order: e.Order})
		}
		out.Seasons = append(out.Seasons, s)
	}
	return out, nil
}

// SetNumbers gives each regular episode its season and number in the
// group as its release-name numbers, replacing any it had. An episode the
// group leaves out gets none. A first group season named Specials gets no
// numbers, and the seasons after it count from 1.
func (g EpisodeGroup) SetNumbers(eps []Episode) {
	first := 0
	for i, s := range g.Seasons {
		if i == 0 || s.Order < first {
			first = s.Order
		}
	}
	specials := strings.EqualFold(strings.TrimSpace(g.seasonAt(first).Name), "specials")
	at := map[string][2]int{}
	for _, s := range g.Seasons {
		season := s.Order - first + 1
		if specials {
			if s.Order == first {
				continue
			}
			season--
		}
		for _, e := range s.Episodes {
			if _, ok := at[e.ID]; !ok && e.ID != "" {
				at[e.ID] = [2]int{season, e.Order + 1}
			}
		}
	}
	for i := range eps {
		if eps[i].Season <= 0 {
			continue
		}
		n := at[eps[i].ID]
		eps[i].AltSeason, eps[i].AltEpisode = n[0], n[1]
	}
}

func (g EpisodeGroup) seasonAt(order int) GroupSeason {
	for _, s := range g.Seasons {
		if s.Order == order {
			return s
		}
	}
	return GroupSeason{}
}

// ShowTMDBID finds the tmdb id for a "tvdb:<id>" or "imdb:<tt id>" show
// through TMDB's find, without loading the show itself.
func (t *TMDB) ShowTMDBID(ctx context.Context, id string) (string, error) {
	kind, v, ok := splitID(id)
	if !ok || (kind != "tvdb" && kind != "imdb") {
		return "", ErrShowNotFound
	}
	b, err := t.get(ctx, "/find/"+url.PathEscape(v), url.Values{"external_source": {kind + "_id"}})
	if err != nil {
		return "", notFoundShow(err)
	}
	var res struct {
		TV []struct {
			ID int64 `json:"id"`
		} `json:"tv_results"`
	}
	if err := json.Unmarshal(b, &res); err != nil || len(res.TV) == 0 || res.TV[0].ID <= 0 {
		return "", ErrShowNotFound
	}
	return strconv.FormatInt(res.TV[0].ID, 10), nil
}
