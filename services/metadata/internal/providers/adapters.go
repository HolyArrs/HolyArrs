package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

func validQueryID(q model.Query) bool {
	if q.Operation == model.OperationTMDB {
		return true
	}
	if q.Operation == "search" {
		return q.Provider != "tvmaze" || q.Page == 1
	}
	if q.Provider == "musicbrainz" {
		return validMBID(q.ID)
	}
	n, err := strconv.ParseInt(q.ID, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == q.ID
}

func validMBID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func requestURL(base *url.URL, q model.Query) *url.URL {
	u := *base
	v := url.Values{}
	if q.Operation == model.OperationTMDB {
		u.Path += q.Path
		u.RawQuery = q.Params
		return &u
	}
	switch q.Provider {
	case "tmdb":
		kind := "movie"
		if q.Kind == "show" {
			kind = "tv"
		}
		if q.Operation == "search" {
			u.Path += "/search/" + kind
			v.Set("query", q.Q)
			v.Set("page", strconv.Itoa(q.Page))
			if q.Region != "" && q.Kind == "movie" {
				v.Set("region", q.Region)
			}
		} else {
			u.Path += "/" + kind + "/" + q.ID
			if q.Operation == "season" {
				u.Path += "/season/" + strconv.Itoa(q.Season)
				v.Set("append_to_response", "credits,translations")
			} else if q.Kind == "movie" {
				v.Set("append_to_response", "credits,translations,release_dates")
			} else {
				v.Set("append_to_response", "credits,translations,content_ratings")
			}
		}
		if q.Language != "" {
			v.Set("language", q.Language)
		}
	case "tvmaze":
		if q.Operation == "search" {
			u.Path += "/search/shows"
			v.Set("q", q.Q)
		} else {
			u.Path += "/shows/" + q.ID
			v.Set("embed", "episodes")
		}
	case "musicbrainz":
		u.Path += "/" + q.Kind
		v.Set("fmt", "json")
		if q.Operation == "search" {
			v.Set("query", q.Q)
			v.Set("limit", strconv.Itoa(q.Limit))
			v.Set("offset", strconv.Itoa((q.Page-1)*q.Limit))
		} else {
			u.Path += "/" + strings.ToLower(q.ID)
			includes := map[string]string{
				"artist":        "aliases+artist-rels+url-rels+genres+tags+ratings",
				"release-group": "artist-credits+releases+url-rels+genres+tags+ratings",
				"release":       "artist-credits+labels+recordings+release-groups+isrcs+url-rels+genres+tags",
				"recording":     "artist-credits+releases+release-groups+isrcs+work-rels+url-rels+genres+tags+ratings",
			}
			v.Set("inc", includes[q.Kind])
		}
	}
	u.RawQuery = v.Encode()
	return &u
}

func normalize(q model.Query, b []byte) (model.Result, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var raw any
	if err := d.Decode(&raw); err != nil {
		return model.Result{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return model.Result{}, fmt.Errorf("trailing JSON")
	}
	if q.Operation == model.OperationTMDB {
		obj, ok := raw.(map[string]any)
		if !ok {
			return model.Result{}, fmt.Errorf("TMDB answer must be an object")
		}
		if q.Path == "/person/popular" {
			if list, ok := obj["results"].([]any); ok {
				kept := make([]any, 0, len(list))
				for _, it := range list {
					if m, ok := it.(map[string]any); ok && m["adult"] == true {
						continue
					}
					kept = append(kept, it)
				}
				obj["results"] = kept
				out, err := json.Marshal(obj)
				if err != nil {
					return model.Result{}, err
				}
				return model.Result{Data: out, Complete: true}, nil
			}
		}
		var c bytes.Buffer
		if err := json.Compact(&c, b); err != nil {
			return model.Result{}, err
		}
		return model.Result{Data: c.Bytes(), Complete: true}, nil
	}
	if q.Operation != "search" {
		obj, ok := raw.(map[string]any)
		if !ok {
			return model.Result{}, fmt.Errorf("detail must be object")
		}
		item, err := normalizeItem(q, obj)
		if err != nil {
			return model.Result{}, err
		}
		if q.Operation == "season" {
			season, ok := integer(obj["season_number"])
			if !ok || season != int64(q.Season) {
				return model.Result{}, fmt.Errorf("season missing or mismatched")
			}
			episodes, ok := obj["episodes"].([]any)
			if !ok {
				return model.Result{}, fmt.Errorf("missing episodes")
			}
			for _, rawEpisode := range episodes {
				episode, ok := rawEpisode.(map[string]any)
				if !ok {
					return model.Result{}, fmt.Errorf("invalid episode")
				}
				if id, ok := integer(episode["id"]); !ok || id < 1 {
					return model.Result{}, fmt.Errorf("missing episode identity")
				}
			}
		}
		out, err := json.Marshal(item)
		return model.Result{Data: out, Complete: err == nil}, err
	}
	var sourceItems []any
	pagination := map[string]any{"page": q.Page}
	next := ""
	switch q.Provider {
	case "tmdb":
		obj, ok := raw.(map[string]any)
		if !ok {
			return model.Result{}, fmt.Errorf("search must be object")
		}
		var arrayOK bool
		sourceItems, arrayOK = obj["results"].([]any)
		page, pageOK := integer(obj["page"])
		pages, pagesOK := integer(obj["total_pages"])
		total, totalOK := integer(obj["total_results"])
		if !arrayOK || !pageOK || !pagesOK || !totalOK || page != int64(q.Page) || pages < 0 || total < 0 || total < int64(len(sourceItems)) || (total > 0 && (pages < page || len(sourceItems) == 0)) || (total == 0 && len(sourceItems) > 0) {
			return model.Result{}, fmt.Errorf("incomplete search pagination")
		}
		pagination["total_pages"] = pages
		pagination["total_results"] = total
		if page < pages {
			next = strconv.FormatInt(page+1, 10)
		}
	case "tvmaze":
		var ok bool
		sourceItems, ok = raw.([]any)
		if !ok {
			return model.Result{}, fmt.Errorf("search must be array")
		}
		pagination["total_results"] = len(sourceItems)
		pagination["total_pages"] = 1
	case "musicbrainz":
		obj, ok := raw.(map[string]any)
		if !ok {
			return model.Result{}, fmt.Errorf("search must be object")
		}
		var arrayOK bool
		sourceItems, arrayOK = obj[q.Kind+"s"].([]any)
		count, countOK := integer(obj["count"])
		offset, offsetOK := integer(obj["offset"])
		if !arrayOK || !countOK || !offsetOK || count < 0 || offset != int64((q.Page-1)*q.Limit) || len(sourceItems) > q.Limit || (len(sourceItems) > 0 && offset+int64(len(sourceItems)) > count) || (offset < count && len(sourceItems) == 0) {
			return model.Result{}, fmt.Errorf("incomplete search pagination")
		}
		pagination["total_results"] = count
		pagination["offset"] = offset
		pagination["limit"] = q.Limit
		if offset+int64(len(sourceItems)) < count && len(sourceItems) > 0 {
			next = strconv.Itoa(q.Page + 1)
		}
	}
	items := make([]map[string]any, 0, len(sourceItems))
	for _, rawItem := range sourceItems {
		obj, ok := rawItem.(map[string]any)
		if !ok {
			return model.Result{}, fmt.Errorf("item must be object")
		}
		if q.Provider == "tvmaze" {
			obj, ok = obj["show"].(map[string]any)
			if !ok {
				return model.Result{}, fmt.Errorf("missing show")
			}
		}
		item, err := normalizeItem(q, obj)
		if err != nil {
			return model.Result{}, err
		}
		items = append(items, item)
	}
	out, err := json.Marshal(map[string]any{"items": items, "pagination": pagination})
	return model.Result{Data: out, Complete: err == nil, NextCursor: next}, err
}

func integer(value any) (int64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

func normalizeItem(q model.Query, obj map[string]any) (map[string]any, error) {
	var id string
	if q.Provider == "musicbrainz" {
		s, ok := obj["id"].(string)
		if !ok || !validMBID(s) {
			return nil, fmt.Errorf("missing identity")
		}
		id = strings.ToLower(s)
	} else {
		n, ok := integer(obj["id"])
		if !ok || n < 1 {
			return nil, fmt.Errorf("missing identity")
		}
		id = strconv.FormatInt(n, 10)
	}
	if q.Operation == "detail" && id != strings.ToLower(q.ID) {
		return nil, fmt.Errorf("mismatched identity")
	}
	titleField, summaryField := "title", "overview"
	if q.Kind == "show" || q.Operation == "season" {
		titleField = "name"
	}
	if q.Provider == "tvmaze" {
		titleField = "name"
		summaryField = "summary"
	}
	if q.Provider == "musicbrainz" {
		summaryField = "disambiguation"
		if q.Kind == "artist" {
			titleField = "name"
		}
	}
	for _, field := range []string{titleField, summaryField} {
		if obj[field] != nil {
			if _, ok := obj[field].(string); !ok {
				return nil, fmt.Errorf("invalid text field")
			}
		}
	}
	source := make(map[string]any)
	for _, field := range strings.Fields(sourceFields[q.Provider]) {
		if value, exists := obj[field]; exists {
			source[field] = value
		}
	}
	return map[string]any{"source": q.Provider, "source_id": id, "kind": q.Kind, "title": obj[titleField], "summary": obj[summaryField], "source_data": source}, nil
}

var sourceFields = map[string]string{
	"tmdb":        "id title original_title name original_name overview adult backdrop_path poster_path belongs_to_collection budget genres genre_ids homepage imdb_id original_language origin_country popularity production_companies production_countries release_date first_air_date last_air_date revenue runtime spoken_languages status tagline video vote_average vote_count created_by episode_run_time in_production languages last_episode_to_air next_episode_to_air networks number_of_episodes number_of_seasons seasons type air_date episode_count season_number episodes episode_number show_id still_path crew guest_stars credits translations release_dates content_ratings",
	"tvmaze":      "id url name type language genres status runtime averageRuntime premiered ended officialSite schedule rating weight network webChannel dvdCountry externals image summary updated _links _embedded",
	"musicbrainz": "id score name sort-name title disambiguation annotation type type-id gender gender-id country area begin-area end-area life-span ipi ipis isnis aliases tags genres rating relations releases release-groups release-group recordings artist-credit primary-type primary-type-id secondary-types secondary-type-ids first-release-date release-events date status status-id quality packaging packaging-id text-representation barcode asin label-info media length video isrcs work-list works artist-credit-id",
}
