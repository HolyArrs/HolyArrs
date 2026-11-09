package discover

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// keywordIDs keeps TMDB keyword ids by name (they never change), so a
// keyword is looked up once per run.
var keywordIDs sync.Map

func keywordID(ctx context.Context, kf metadata.KeywordFinder, name string) (int64, error) {
	if id, ok := keywordIDs.Load(name); ok {
		return id.(int64), nil
	}
	id, err := kf.KeywordID(ctx, name)
	if err == nil {
		keywordIDs.Store(name, id)
	}
	return id, err
}

// ListerSource maps a Request to TMDB lists.
type ListerSource struct{ L metadata.ExploreLister }

func (s ListerSource) List(ctx context.Context, q engine.Request) ([]engine.Item, error) {
	p := "movie"
	if q.Kind == "show" {
		p = "tv"
	}
	var out []metadata.DiscoverItem
	var err error
	switch q.List {
	case "trending":
		out, err = s.L.ExploreList(ctx, q.Kind, "/trending/"+p+"/week", nil)
	case "upcoming":
		path := "/movie/upcoming"
		if q.Kind == "show" {
			path = "/tv/on_the_air"
		}
		out, err = s.L.ExploreList(ctx, q.Kind, path, nil)
	case "now_playing":
		// Movies in cinemas now (New & Popular); shows have no such list.
		if q.Kind != "movie" {
			return []engine.Item{}, nil
		}
		var v url.Values
		if q.Region != "" {
			v = url.Values{"region": {q.Region}}
		}
		out, err = s.L.ExploreList(ctx, q.Kind, "/movie/now_playing", v)
	case "best":
		out, err = s.L.ExploreList(ctx, q.Kind, "/discover/"+p, metadata.BestQuery(q.Kind, time.Now()))
	case "similar":
		var recs, sim []metadata.DiscoverItem
		if recs, err = s.L.ExploreList(ctx, q.Kind, "/"+p+"/"+q.Ref+"/recommendations", nil); err == nil {
			sim, err = s.L.ExploreList(ctx, q.Kind, "/"+p+"/"+q.Ref+"/similar", nil)
		}
		out = append(recs, sim...)
	case "collection":
		cl, ok := s.L.(metadata.CollectionLister)
		if !ok {
			return []engine.Item{}, nil
		}
		out, err = cl.CollectionParts(ctx, q.Ref)
	case "person":
		var all []metadata.DiscoverItem
		all, err = s.L.PersonCredits(ctx, q.Ref)
		for _, it := range all {
			if q.Kind == "" || it.Kind == q.Kind {
				out = append(out, it)
			}
		}
	case "discover", "weekly":
		v := url.Values{"include_adult": {"false"}, "sort_by": {"popularity.desc"}}
		if q.Keywords != "" {
			kf, ok := s.L.(metadata.KeywordFinder)
			if !ok {
				return []engine.Item{}, nil
			}
			var ids []string
			for _, name := range strings.Split(q.Keywords, "|") {
				id, kerr := keywordID(ctx, kf, name)
				if kerr != nil {
					return nil, kerr
				}
				if id > 0 {
					ids = append(ids, strconv.FormatInt(id, 10))
				}
			}
			if len(ids) == 0 {
				return []engine.Item{}, nil
			}
			v.Set("with_keywords", strings.Join(ids, "|"))
		}
		if q.Genres != "" {
			v.Set("with_genres", q.Genres)
		}
		if q.Without != "" {
			v.Set("without_genres", q.Without)
		}
		if q.Lang != "" {
			v.Set("with_original_language", q.Lang)
		}
		if q.Cert != "" && q.Kind == "movie" {
			v.Set("certification_country", "US")
			v.Set("certification.lte", q.Cert)
		}
		if q.Sort == "rating" {
			v.Set("sort_by", "vote_average.desc")
		}
		if q.Sort == "recent" {
			v.Set("sort_by", "primary_release_date.desc")
			if q.Kind == "show" {
				v.Set("sort_by", "first_air_date.desc")
			}
		}
		if q.AirFrom != "" && q.Kind == "show" {
			v.Set("air_date.gte", q.AirFrom)
		}
		if q.AirTo != "" && q.Kind == "show" {
			v.Set("air_date.lte", q.AirTo)
		}
		if q.Genre != "" {
			id, gerr := s.L.GenreID(ctx, q.Kind, q.Genre)
			if gerr != nil {
				return nil, gerr
			}
			if id == 0 {
				return []engine.Item{}, nil
			}
			v.Set("with_genres", strconv.FormatInt(id, 10))
		}
		date := "primary_release_date"
		if q.Kind == "show" {
			date = "first_air_date"
		} else if q.Types != "" {
			// Regional release dates of those types (digital, cinema).
			date = "release_date"
			v.Set("with_release_type", q.Types)
			if q.Region != "" {
				v.Set("region", q.Region)
			}
		}
		if q.From != "" {
			v.Set(date+".gte", q.From)
		}
		if q.To != "" {
			v.Set(date+".lte", q.To)
		}
		if q.Services != "" && q.Region != "" {
			v.Set("with_watch_providers", q.Services)
			v.Set("watch_region", q.Region)
			v.Set("with_watch_monetization_types", "flatrate")
		}
		if q.MinVote > 0 {
			v.Set("vote_average.gte", strconv.FormatFloat(q.MinVote, 'f', 1, 64))
		}
		if q.MinVotes > 0 {
			v.Set("vote_count.gte", strconv.Itoa(q.MinVotes))
		}
		out, err = s.L.ExploreList(ctx, q.Kind, "/discover/"+p, v)
	default:
		return nil, errors.New("explore: unknown list " + q.List)
	}
	if err != nil {
		return nil, err
	}
	items := make([]engine.Item, 0, len(out))
	for _, d := range out {
		if q.MaxVotes > 0 && d.VoteCount > q.MaxVotes {
			continue
		}
		items = append(items, engine.Item{TMDBID: d.TMDBID, Kind: d.Kind, Name: d.Name, Year: d.Year, Poster: d.Poster, Backdrop: d.Backdrop,
			Overview: d.Overview, Genres: d.Genres, Pop: d.Popularity, Vote: d.VoteAverage, Votes: d.VoteCount, Released: d.Released})
	}
	return items, nil
}
