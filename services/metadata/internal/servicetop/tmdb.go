package servicetop

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Fetcher is the server's provider client.
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}

// TMDB reads fallbacks and posters through the server's own paced client.
type TMDB struct{ Fetcher Fetcher }

func (t TMDB) read(ctx context.Context, path string, v url.Values, out any) error {
	params, err := model.TMDBRead(path, v)
	if err != nil {
		return err
	}
	res, err := t.Fetcher.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: path, Params: params, Page: 1, Limit: 20})
	if err != nil {
		return err
	}
	return json.Unmarshal(res.Data, out)
}

type discover struct {
	Results []struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		Name       string `json:"name"`
		PosterPath string `json:"poster_path"`
	} `json:"results"`
}

// Popular takes the most popular films and series on a provider in a
// region (flat-rate only) and alternates them, film first.
func (t TMDB) Popular(ctx context.Context, provider int, region string, free bool) ([]Item, error) {
	lists := map[string][]Item{}
	for _, kind := range []string{"movie", "tv"} {
		var d discover
		v := url.Values{"with_watch_providers": {strconv.Itoa(provider)}, "watch_region": {region}, "sort_by": {"popularity.desc"}}
		if !free {
			v.Set("with_watch_monetization_types", "flatrate")
		}
		if err := t.read(ctx, "/discover/"+kind, v, &d); err != nil {
			return nil, err
		}
		for _, r := range d.Results {
			if len(lists[kind]) < 10 {
				lists[kind] = append(lists[kind], Item{TMDBID: r.ID, Kind: kind, Title: r.Title + r.Name, PosterPath: r.PosterPath})
			}
		}
	}
	var out []Item
	for i := 0; i < 10; i++ {
		for _, kind := range []string{"movie", "tv"} {
			if i < len(lists[kind]) {
				it := lists[kind][i]
				it.Rank = len(out) + 1
				out = append(out, it)
			}
		}
	}
	return out, nil
}

// ProviderID finds a watch provider's id by name in a region (movie list).
func (t TMDB) ProviderID(ctx context.Context, name, region string) (int, error) {
	var d struct {
		Results []struct {
			ID   int    `json:"provider_id"`
			Name string `json:"provider_name"`
		} `json:"results"`
	}
	if err := t.read(ctx, "/watch/providers/movie", url.Values{"watch_region": {region}}, &d); err != nil {
		return 0, err
	}
	for _, r := range d.Results {
		if strings.EqualFold(r.Name, name) || strings.EqualFold(r.Name, name+" TV") {
			return r.ID, nil
		}
	}
	return 0, errors.New("watch provider not found: " + name)
}

// Poster reads a title's poster path.
func (t TMDB) Poster(ctx context.Context, kind string, id int64) (string, error) {
	var d struct {
		PosterPath string `json:"poster_path"`
	}
	err := t.read(ctx, "/"+kind+"/"+strconv.FormatInt(id, 10), url.Values{}, &d)
	return d.PosterPath, err
}
