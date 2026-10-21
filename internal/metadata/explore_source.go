package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ExploreLister reads the lists the Explore engine builds rows from. Only
// title ids, genres, dates, regions and service ids are sent; nothing about
// the people in the household.
type ExploreLister interface {
	// ExploreList reads one TMDB list path ("/movie/1/similar",
	// "/discover/tv", "/trending/movie/week") with q, as items of kind.
	ExploreList(ctx context.Context, kind, path string, q url.Values) ([]DiscoverItem, error)
	// PersonCredits finds a person by name and lists their movies and
	// shows, best known first.
	PersonCredits(ctx context.Context, name string) ([]DiscoverItem, error)
	// GenreID is the TMDB genre id of a genre name for kind, 0 if unknown.
	GenreID(ctx context.Context, kind, genre string) (int64, error)
}

// ExploreListerFor returns the Explore list surface selected for ctx.
func ExploreListerFor(ctx context.Context, provider Provider) (ExploreLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(ExploreLister)
	return l, ok
}

var exploreListPath = regexp.MustCompile(`^/((movie|tv)/[1-9][0-9]{0,11}/(recommendations|similar)|discover/(movie|tv)|trending/(movie|tv)/week|movie/upcoming|movie/now_playing|tv/on_the_air)$`)

// ExploreList reads one allowed TMDB list.
func (t *TMDB) ExploreList(ctx context.Context, kind, path string, q url.Values) ([]DiscoverItem, error) {
	if (kind != "movie" && kind != "show") || !exploreListPath.MatchString(path) {
		return nil, fmt.Errorf("explore list %q is not allowed", path)
	}
	names, err := t.genreNames(ctx, kind)
	if err != nil {
		return nil, err
	}
	v := url.Values{"language": {"en-US"}}
	for k, vals := range q {
		v[k] = vals
	}
	b, err := t.get(ctx, path, v)
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

// CollectionLister lists the movies of a TMDB collection, for Finish the
// collection.
type CollectionLister interface {
	CollectionParts(ctx context.Context, id string) ([]DiscoverItem, error)
}

// CollectionParts lists collection id's movies, oldest first.
func (t *TMDB) CollectionParts(ctx context.Context, id string) ([]DiscoverItem, error) {
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return nil, fmt.Errorf("collection %q is not a TMDB id", id)
	}
	names, err := t.genreNames(ctx, "movie")
	if err != nil {
		return nil, err
	}
	b, err := t.get(ctx, "/collection/"+id, url.Values{"language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Parts []tmdbListItem `json:"parts"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	parts := t.items("movie", res.Parts, names, maxCollection)
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].Released < parts[j].Released })
	return parts, nil
}

// GenreID looks a genre name up for kind.
func (t *TMDB) GenreID(ctx context.Context, kind, genre string) (int64, error) {
	names, err := t.genreNames(ctx, kind)
	if err != nil {
		return 0, err
	}
	for id, n := range names {
		if n == genre {
			return id, nil
		}
	}
	return 0, nil
}

// PersonCredits finds the best known person with that name, then lists
// the movies and shows they are credited on, best known first.
func (t *TMDB) PersonCredits(ctx context.Context, name string) ([]DiscoverItem, error) {
	b, err := t.get(ctx, "/search/person", url.Values{"query": {name}, "include_adult": {"false"}, "language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var found struct {
		Results []struct {
			ID int64 `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &found); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	if len(found.Results) == 0 || found.Results[0].ID <= 0 {
		return []DiscoverItem{}, nil
	}
	b, err = t.get(ctx, "/person/"+strconv.FormatInt(found.Results[0].ID, 10)+"/combined_credits", url.Values{"language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var cr struct {
		Cast []tmdbListItem `json:"cast"`
		Crew []tmdbListItem `json:"crew"`
	}
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	movieNames, err := t.genreNames(ctx, "movie")
	if err != nil {
		return nil, err
	}
	showNames, err := t.genreNames(ctx, "show")
	if err != nil {
		return nil, err
	}
	var movies, shows []tmdbListItem
	seen := map[string]bool{}
	for _, m := range append(cr.Cast, cr.Crew...) {
		key := m.MediaType + strconv.FormatInt(m.ID, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		switch m.MediaType {
		case "movie":
			movies = append(movies, m)
		case "tv":
			shows = append(shows, m)
		}
	}
	out := t.items("movie", movies, movieNames, len(movies))
	out = append(out, t.items("show", shows, showNames, len(shows))...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Popularity > out[j].Popularity })
	return out[:min(len(out), maxDiscover)], nil
}

// KeywordFinder finds TMDB keyword ids by name, for mood rows.
type KeywordFinder interface {
	KeywordID(ctx context.Context, name string) (int64, error)
}

// KeywordID is the id of the TMDB keyword named exactly name (any case),
// 0 when TMDB has none.
func (t *TMDB) KeywordID(ctx context.Context, name string) (int64, error) {
	b, err := t.get(ctx, "/search/keyword", url.Values{"query": {name}})
	if err != nil {
		return 0, err
	}
	var res struct {
		Results []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	for _, r := range res.Results {
		if strings.EqualFold(r.Name, name) {
			return r.ID, nil
		}
	}
	return 0, nil
}
