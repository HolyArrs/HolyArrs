package model

import (
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// OperationTMDB is a read of one allowlisted TMDB v3 path, answered with
// TMDB's own JSON. It serves the app's lists, discover, people, ratings and
// find lookups without the app holding a TMDB key.
const OperationTMDB = "tmdb"

var tmdbPaths = []*regexp.Regexp{
	regexp.MustCompile(`^/search/(movie|tv|person|keyword)$`),
	regexp.MustCompile(`^/(movie|tv|person|collection|list)/[1-9][0-9]{0,11}$`),
	regexp.MustCompile(`^/tv/[1-9][0-9]{0,11}/season/[0-9]{1,4}$`),
	regexp.MustCompile(`^/movie/[1-9][0-9]{0,11}/(credits|release_dates|recommendations|similar|images|external_ids|keywords|videos|reviews)$`),
	regexp.MustCompile(`^/tv/[1-9][0-9]{0,11}/(credits|aggregate_credits|content_ratings|recommendations|similar|images|external_ids|keywords|videos|reviews)$`),
	regexp.MustCompile(`^/person/[1-9][0-9]{0,11}/(movie_credits|tv_credits|combined_credits|images|external_ids)$`),
	regexp.MustCompile(`^/person/(popular|changes)$`),
	regexp.MustCompile(`^/find/(tt[0-9]{1,12}|[1-9][0-9]{0,11})$`),
	regexp.MustCompile(`^/genre/(movie|tv)/list$`),
	regexp.MustCompile(`^/certification/(movie|tv)/list$`),
	regexp.MustCompile(`^/trending/(all|movie|tv|person)/(day|week)$`),
	regexp.MustCompile(`^/movie/(popular|upcoming|now_playing|top_rated|changes)$`),
	regexp.MustCompile(`^/tv/(popular|on_the_air|airing_today|top_rated|changes)$`),
	regexp.MustCompile(`^/discover/(movie|tv)$`),
	regexp.MustCompile(`^/watch/providers/(movie|tv)$`),
	regexp.MustCompile(`^/(movie|tv)/[1-9][0-9]{0,11}/watch/providers$`),
}

var tmdbParams = map[string]*regexp.Regexp{
	"query":                         regexp.MustCompile(`^[^\x00-\x1f]{1,256}$`),
	"page":                          regexp.MustCompile(`^([1-9]|[1-9][0-9]|[1-4][0-9]{2}|500)$`),
	"language":                      regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`),
	"region":                        regexp.MustCompile(`^[A-Z]{2}$`),
	"include_adult":                 regexp.MustCompile(`^false$`),
	"year":                          regexp.MustCompile(`^[12][0-9]{3}$`),
	"primary_release_year":          regexp.MustCompile(`^[12][0-9]{3}$`),
	"first_air_date_year":           regexp.MustCompile(`^[12][0-9]{3}$`),
	"append_to_response":            regexp.MustCompile(`^(credits|aggregate_credits|release_dates|content_ratings|images|recommendations|similar|external_ids|keywords|translations|videos)(,(credits|aggregate_credits|release_dates|content_ratings|images|recommendations|similar|external_ids|keywords|translations|videos)){0,9}$`),
	"include_image_language":        regexp.MustCompile(`^[a-z,]{1,32}$`),
	"include_video_language":        regexp.MustCompile(`^[a-z]{2}(,[a-z]{2}){0,2}(,null)?$`),
	"external_source":               regexp.MustCompile(`^(imdb_id|tvdb_id)$`),
	"with_genres":                   regexp.MustCompile(`^[0-9]{1,8}([,|][0-9]{1,8}){0,9}$`),
	"without_genres":                regexp.MustCompile(`^[0-9]{1,8}([,|][0-9]{1,8}){0,9}$`),
	"with_keywords":                 regexp.MustCompile(`^[1-9][0-9]{0,8}([,|][1-9][0-9]{0,8}){0,19}$`),
	"sort_by":                       regexp.MustCompile(`^[a-z_.]{1,40}$`),
	"start_date":                    regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"end_date":                      regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"with_original_language":        regexp.MustCompile(`^[a-z]{2}$`),
	"certification_country":         regexp.MustCompile(`^[A-Z]{2}$`),
	"certification":                 regexp.MustCompile(`^[A-Za-z0-9+-]{1,12}$`),
	"certification.lte":             regexp.MustCompile(`^[A-Za-z0-9+-]{1,12}$`),
	"vote_count.gte":                regexp.MustCompile(`^[0-9]{1,7}$`),
	"primary_release_date.gte":      regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"primary_release_date.lte":      regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"first_air_date.gte":            regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"first_air_date.lte":            regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"with_watch_providers":          regexp.MustCompile(`^[1-9][0-9]{0,6}(\|[1-9][0-9]{0,6}){0,19}$`),
	"vote_average.gte":              regexp.MustCompile(`^(10|[0-9])(\.[0-9])?$`),
	"watch_region":                  regexp.MustCompile(`^[A-Z]{2}$`),
	"with_watch_monetization_types": regexp.MustCompile(`^flatrate$`),
	// New & Popular: shows with an episode airing in a window; movies by
	// their (regional) digital or cinema release dates.
	"air_date.gte":      regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"air_date.lte":      regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"release_date.gte":  regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"release_date.lte":  regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`),
	"with_release_type": regexp.MustCompile(`^[1-6]([|,][1-6]){0,5}$`),
}

// TMDBRead checks a TMDB path and its parameters against the allowlist and
// returns the canonical (sorted, encoded) parameter string.
func TMDBRead(path string, v url.Values) (string, error) {
	if len(path) > 128 || strings.Contains(path, "//") || strings.Contains(path, "..") {
		return "", errors.New("unsupported TMDB path")
	}
	ok := false
	for _, re := range tmdbPaths {
		if re.MatchString(path) {
			ok = true
			break
		}
	}
	if !ok {
		return "", errors.New("unsupported TMDB path")
	}
	if path == "/person/popular" {
		if p := v.Get("page"); p != "" && !regexp.MustCompile(`^[1-8]$`).MatchString(p) {
			return "", errors.New("unsupported TMDB parameter")
		}
	}
	keys := make([]string, 0, len(v))
	for k, values := range v {
		re := tmdbParams[k]
		if re == nil || len(values) != 1 || !re.MatchString(values[0]) {
			return "", errors.New("unsupported TMDB parameter")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k) + "=" + url.QueryEscape(v.Get(k)))
	}
	if b.Len() > 1024 {
		return "", errors.New("unsupported TMDB parameter")
	}
	return b.String(), nil
}
