package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	serviceMaxSeasons  = 100
	serviceMaxEpisodes = 10000
)

type serviceShows struct {
	service  *Service
	provider string
	direct   ShowProvider
}

// Shows uses the optional service for one supported show provider. Alternate
// identity kinds retain the direct provider path.
func (s *Service) Shows(provider string, direct ShowProvider) ShowProvider {
	return &serviceShows{service: s, provider: provider, direct: direct}
}

func (p *serviceShows) SearchShows(ctx context.Context, query string) ([]Show, error) {
	if p.provider != "tmdb" && p.provider != "tvmaze" {
		return nil, ErrUnknownSource
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	res, err := p.service.search(ctx, p.provider, "show", query, 1, 20)
	if err == nil {
		out := make([]Show, 0, len(res.Items))
		for _, item := range res.Items {
			var show Show
			if p.provider == "tmdb" {
				show, err = serviceTMDBShowItem(item)
			} else {
				show, _, err = serviceTVMazeShowItem(item)
			}
			if err != nil {
				break
			}
			out = append(out, show)
		}
		if err == nil {
			return out, nil
		}
	}
	if p.direct != nil && serviceFallback(ctx, err) {
		return p.direct.SearchShows(ctx, query)
	}
	return nil, err
}

func (p *serviceShows) Show(ctx context.Context, id string) (Show, error) {
	if p.provider != "tmdb" && p.provider != "tvmaze" {
		return Show{}, ErrUnknownSource
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	kind, value, ok := splitID(id)
	if !ok {
		return Show{}, ErrShowNotFound
	}
	if kind != p.provider {
		if p.direct != nil {
			return p.direct.Show(ctx, id)
		}
		return Show{}, ErrShowNotFound
	}
	if !serviceID(p.provider, value) {
		return Show{}, ErrShowNotFound
	}
	item, err := p.service.detail(ctx, p.provider, "show", value)
	if err == nil {
		var out Show
		if p.provider == "tmdb" {
			out, err = p.tmdbDetail(ctx, value, item)
			if err == nil && out.OriginalLanguage == "ja" {
				p.releaseNumbers(ctx, value, &out)
			}
		} else {
			out, err = serviceTVMazeDetail(item)
		}
		if err == nil {
			return out, nil
		}
	}
	if errors.Is(err, ErrNotFound) {
		return Show{}, ErrShowNotFound
	}
	if p.direct != nil && serviceFallback(ctx, err) {
		out, err := p.direct.Show(ctx, id)
		// The anime map was not read, so the stored release numbers stay.
		out.AltStale = p.provider == "tmdb" && out.OriginalLanguage == "ja"
		return out, err
	}
	return Show{}, err
}

// AnimeShows wraps the live TMDB show source, which reads TMDB through
// the server's /v1/tmdb and so never passes serviceShows.Show: Japanese
// TMDB shows it returns get their release-name numbers here.
func (s *Service) AnimeShows(direct ShowProvider) ShowProvider {
	return animeShows{ShowProvider: direct, numbers: &serviceShows{service: s, provider: "tmdb"}}
}

type animeShows struct {
	ShowProvider
	numbers *serviceShows
}

func (a animeShows) Show(ctx context.Context, id string) (Show, error) {
	out, err := a.ShowProvider.Show(ctx, id)
	if err == nil && out.Source == "tmdb" && out.OriginalLanguage == "ja" && out.IDs["tmdb"] != "" {
		ctx, cancel := serviceContext(ctx)
		defer cancel()
		a.numbers.releaseNumbers(ctx, out.IDs["tmdb"], &out)
	}
	return out, err
}

// releaseNumbers adds the release-name numbers from the server's anime
// map, or for an anime the map does not know or has out of date, from
// TVMaze's season sizes. A show neither knows keeps none; a failed call
// marks them stale so the stored ones stay.
func (p *serviceShows) releaseNumbers(ctx context.Context, tmdbID string, out *Show) {
	m, err := p.service.AnimeMap(ctx, "tmdb_tv", tmdbID)
	switch {
	case err == nil:
		if m.SetReleaseNumbers(tmdbID, out.Seasons, out.Episodes) {
			return
		}
	case !errors.Is(err, ErrNotFound):
		out.AltStale = true
		return
	}
	if !LooksAnime(out.Genres, nil, out.OriginalLanguage) {
		return
	}
	sizes, err := p.mazeSeasonSizes(ctx, out)
	if err != nil {
		out.AltStale = true
		return
	}
	setSeasonSizes(out.Episodes, sizes)
}

// mazeSeasonSizes reads the episode counts of TVMaze's regular seasons for
// the show with out's TVDB id, through the server's TVMaze search (by name,
// then original name) and detail. None when no TVMaze show has that id.
func (p *serviceShows) mazeSeasonSizes(ctx context.Context, out *Show) ([]int, error) {
	tvdb := out.IDs["tvdb"]
	if tvdb == "" {
		return nil, nil
	}
	names := append([]string{out.Name}, out.OtherNames...)
	for _, name := range names[:min(2, len(names))] {
		if strings.TrimSpace(name) == "" {
			continue
		}
		res, err := p.service.search(ctx, "tvmaze", "show", name, 1, 20)
		if err != nil {
			return nil, err
		}
		for _, item := range res.Items {
			if show, _, err := serviceTVMazeShowItem(item); err != nil || show.IDs["tvdb"] != tvdb {
				continue
			}
			detail, err := p.service.detail(ctx, "tvmaze", "show", item.SourceID)
			if err != nil {
				return nil, err
			}
			maze, err := serviceTVMazeDetail(detail)
			if err != nil {
				return nil, err
			}
			var sizes []int
			for _, e := range maze.Episodes {
				if e.Season > serviceMaxSeasons {
					return nil, nil
				}
				for e.Season > len(sizes) {
					sizes = append(sizes, 0)
				}
				if e.Season > 0 {
					sizes[e.Season-1]++
				}
			}
			return sizes, nil
		}
	}
	return nil, nil
}

type serviceTMDBShowCounts struct {
	NumberOfSeasons  *int `json:"number_of_seasons"`
	NumberOfEpisodes *int `json:"number_of_episodes"`
	Seasons          []struct {
		ID           int64 `json:"id"`
		Number       int   `json:"season_number"`
		EpisodeCount *int  `json:"episode_count"`
	} `json:"seasons"`
}

type serviceTMDBSeason struct {
	ID       int64 `json:"id"`
	Number   int   `json:"season_number"`
	Episodes []struct {
		ID      int64   `json:"id"`
		Season  int     `json:"season_number"`
		Number  int     `json:"episode_number"`
		Name    string  `json:"name"`
		AirDate *string `json:"air_date"`
		Runtime int     `json:"runtime"`
	} `json:"episodes"`
}

func serviceTMDBShowItem(item serviceItem) (Show, error) {
	if item.Source != "tmdb" || item.Kind != "show" || !serviceID("tmdb", item.SourceID) {
		return Show{}, ErrServiceInvalid
	}
	var raw tmdbShow
	if err := item.decode(&raw); err != nil || raw.ID <= 0 || strconv.FormatInt(raw.ID, 10) != item.SourceID {
		return Show{}, ErrServiceInvalid
	}
	var err error
	if raw.Name, err = serviceTitleText(raw.Name, false); err != nil || raw.Name == "" {
		return Show{}, ErrServiceInvalid
	}
	if raw.Overview, err = serviceTitleText(raw.Overview, true); err != nil {
		return Show{}, ErrServiceInvalid
	}
	if raw.OriginalLanguage, err = serviceTitleText(raw.OriginalLanguage, false); err != nil {
		return Show{}, ErrServiceInvalid
	}
	if raw.PosterPath != "" && !servicePosterPath(raw.PosterPath) {
		return Show{}, ErrServiceInvalid
	}
	if len(raw.Seasons) > serviceMaxSeasons {
		return Show{}, serviceIncomplete("show lists more than %d seasons", serviceMaxSeasons)
	}
	seen := make(map[int]bool, len(raw.Seasons))
	for i := range raw.Seasons {
		if raw.Seasons[i].Number < 0 || seen[raw.Seasons[i].Number] {
			return Show{}, serviceIncomplete("show has duplicate or invalid seasons")
		}
		seen[raw.Seasons[i].Number] = true
		raw.Seasons[i].Name, err = serviceTitleText(raw.Seasons[i].Name, false)
		if err != nil {
			return Show{}, ErrServiceInvalid
		}
	}
	return raw.show(ImageBase), nil
}

func (p *serviceShows) tmdbDetail(ctx context.Context, showID string, item serviceItem) (Show, error) {
	out, err := serviceTMDBShowItem(item)
	if err != nil {
		return Show{}, err
	}
	var counts serviceTMDBShowCounts
	if json.Unmarshal(item.SourceData, &counts) != nil || counts.Seasons == nil || len(counts.Seasons) != len(out.Seasons) {
		return Show{}, serviceIncomplete("show season list is missing")
	}
	if len(counts.Seasons) > serviceMaxSeasons {
		return Show{}, serviceIncomplete("show lists more than %d seasons", serviceMaxSeasons)
	}
	positiveSeasons := 0
	maxPositiveSeason := 0
	seenSeasons := map[int]bool{}
	seenEpisodeIDs := map[int64]bool{}
	seenEpisodes := map[[2]int]bool{}
	for _, listed := range counts.Seasons {
		if listed.ID <= 0 || listed.Number < 0 || seenSeasons[listed.Number] || listed.EpisodeCount != nil && *listed.EpisodeCount < 0 {
			return Show{}, serviceIncomplete("show has duplicate or invalid season metadata")
		}
		seenSeasons[listed.Number] = true
		if listed.Number > 0 {
			positiveSeasons++
			maxPositiveSeason = max(maxPositiveSeason, listed.Number)
		}
	}
	if maxPositiveSeason != positiveSeasons {
		return Show{}, serviceIncomplete("show season numbers have a gap")
	}
	for _, listed := range counts.Seasons {
		seasonItem, callErr := p.service.season(ctx, showID, listed.Number)
		if callErr != nil {
			return Show{}, callErr
		}
		var season serviceTMDBSeason
		if seasonItem.decode(&season) != nil || season.ID != listed.ID || season.Number != listed.Number || season.Episodes == nil {
			return Show{}, serviceIncomplete("season %d is missing or mismatched", listed.Number)
		}
		if listed.EpisodeCount != nil && len(season.Episodes) != *listed.EpisodeCount {
			return Show{}, serviceIncomplete("season %d episode count does not match", listed.Number)
		}
		if len(out.Episodes)+len(season.Episodes) > serviceMaxEpisodes {
			return Show{}, serviceIncomplete("show lists more than %d episodes", serviceMaxEpisodes)
		}
		maxEpisode := 0
		for _, raw := range season.Episodes {
			key := [2]int{raw.Season, raw.Number}
			if raw.ID <= 0 || raw.Season != listed.Number || raw.Number <= 0 || seenEpisodeIDs[raw.ID] || seenEpisodes[key] || raw.Runtime < 0 || raw.Runtime > 24*60 {
				return Show{}, serviceIncomplete("season %d has duplicate or invalid episodes", listed.Number)
			}
			name, textErr := serviceTitleText(raw.Name, false)
			if textErr != nil {
				return Show{}, ErrServiceInvalid
			}
			seenEpisodeIDs[raw.ID], seenEpisodes[key] = true, true
			maxEpisode = max(maxEpisode, raw.Number)
			ep := Episode{Season: raw.Season, Number: raw.Number, Name: name, Runtime: raw.Runtime, ID: strconv.FormatInt(raw.ID, 10)}
			if raw.AirDate != nil {
				ep.AirDate, ep.AirDay = dayEnd(*raw.AirDate)
			}
			out.Episodes = append(out.Episodes, ep)
		}
		if maxEpisode != len(season.Episodes) {
			return Show{}, serviceIncomplete("season %d episode numbers have a gap", listed.Number)
		}
	}
	if counts.NumberOfSeasons != nil && (*counts.NumberOfSeasons < 0 || *counts.NumberOfSeasons != positiveSeasons) {
		return Show{}, serviceIncomplete("advertised season count does not match")
	}
	if counts.NumberOfEpisodes != nil && (*counts.NumberOfEpisodes < 0 || *counts.NumberOfEpisodes != len(out.Episodes)) {
		return Show{}, serviceIncomplete("advertised episode count does not match")
	}
	out.finish()
	return out, nil
}

type serviceMazeEpisode struct {
	ID       int64   `json:"id"`
	Season   int     `json:"season"`
	Number   *int    `json:"number"`
	Name     string  `json:"name"`
	AirDay   string  `json:"airdate"`
	AirTime  *string `json:"airtime"`
	AirStamp *string `json:"airstamp"`
	Runtime  int     `json:"runtime"`
}

func serviceTVMazeShowItem(item serviceItem) (Show, []serviceMazeEpisode, error) {
	if item.Source != "tvmaze" || item.Kind != "show" || !serviceID("tvmaze", item.SourceID) {
		return Show{}, nil, ErrServiceInvalid
	}
	var raw mazeShow
	if err := item.decode(&raw); err != nil || raw.ID <= 0 || strconv.FormatInt(raw.ID, 10) != item.SourceID {
		return Show{}, nil, ErrServiceInvalid
	}
	var err error
	if raw.Name, err = serviceTitleText(raw.Name, false); err != nil || raw.Name == "" {
		return Show{}, nil, ErrServiceInvalid
	}
	if raw.Summary, err = serviceTitleText(raw.Summary, true); err != nil {
		return Show{}, nil, ErrServiceInvalid
	}
	if raw.Image != nil && raw.Image.Original != "" && !servicePictureURL(raw.Image.Original) {
		return Show{}, nil, ErrServiceInvalid
	}
	if raw.Externals.IMDb != "" && !serviceExternalID(raw.Externals.IMDb) {
		return Show{}, nil, ErrServiceInvalid
	}
	var embedded struct {
		Embedded *struct {
			Episodes []serviceMazeEpisode `json:"episodes"`
		} `json:"_embedded"`
	}
	if json.Unmarshal(item.SourceData, &embedded) != nil {
		return Show{}, nil, ErrServiceInvalid
	}
	var episodes []serviceMazeEpisode
	if embedded.Embedded != nil {
		episodes = embedded.Embedded.Episodes
	}
	return raw.show(), episodes, nil
}

func serviceTVMazeDetail(item serviceItem) (Show, error) {
	out, episodes, err := serviceTVMazeShowItem(item)
	if err != nil {
		return Show{}, err
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(item.SourceData, &shape) != nil || len(shape["_embedded"]) == 0 || episodes == nil {
		return Show{}, serviceIncomplete("TVMaze detail has no embedded episodes")
	}
	if len(episodes) > serviceMaxEpisodes {
		return Show{}, serviceIncomplete("show lists more than %d episodes", serviceMaxEpisodes)
	}
	seenIDs := map[int64]bool{}
	seenNumbers := map[[2]int]bool{}
	var specials []Episode
	for _, raw := range episodes {
		if raw.ID <= 0 || raw.Season < 0 || raw.Runtime < 0 || raw.Runtime > 24*60 || seenIDs[raw.ID] {
			return Show{}, serviceIncomplete("TVMaze detail has invalid episodes")
		}
		name, textErr := serviceTitleText(raw.Name, false)
		if textErr != nil {
			return Show{}, ErrServiceInvalid
		}
		seenIDs[raw.ID] = true
		ep := Episode{Season: raw.Season, Name: name, Runtime: raw.Runtime, ID: strconv.FormatInt(raw.ID, 10)}
		ep.AirDay, ep.AirDate, ep.Timed = mazeAir(raw.AirDay, raw.AirTime, raw.AirStamp)
		if raw.Number == nil {
			ep.Season = 0
			specials = append(specials, ep)
			continue
		}
		if *raw.Number <= 0 || seenNumbers[[2]int{raw.Season, *raw.Number}] {
			return Show{}, serviceIncomplete("TVMaze detail has duplicate or invalid episode numbers")
		}
		ep.Number = *raw.Number
		seenNumbers[[2]int{raw.Season, *raw.Number}] = true
		out.Episodes = append(out.Episodes, ep)
	}
	sort.SliceStable(specials, func(i, j int) bool { return specials[i].AirDay < specials[j].AirDay })
	for i := range specials {
		specials[i].Number = i + 1
	}
	out.Episodes = append(out.Episodes, specials...)
	out.finish()
	return out, nil
}

func servicePictureURL(value string) bool {
	if len(value) > 2048 || strings.ContainsAny(value, "\r\n\t") {
		return false
	}
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}
