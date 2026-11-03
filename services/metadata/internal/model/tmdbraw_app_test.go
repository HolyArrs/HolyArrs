package model

import (
	"net/url"
	"testing"
)

// The app's movie and show detail reads (internal/metadata tmdb.go and
// tmdbtv.go) go through this proxy; if their append values ever stop
// passing, every detail read in every install answers 400.
func TestAppDetailReadsPassTheTMDBProxy(t *testing.T) {
	for path, v := range map[string]url.Values{
		"/movie/194": {"append_to_response": {"release_dates,images,credits,translations"}, "include_image_language": {"en,null"}},
		"/tv/71446":  {"append_to_response": {"external_ids,images,credits,content_ratings,translations"}, "include_image_language": {"en,null"}},
	} {
		if _, err := TMDBRead(path, v); err != nil {
			t.Errorf("%s %v: %v", path, v, err)
		}
	}
	// alternative_titles is allowed too, for when the app asks for it again.
	if _, err := TMDBRead("/movie/194", url.Values{"append_to_response": {"release_dates,images,credits,alternative_titles,translations"}}); err != nil {
		t.Errorf("alternative_titles: %v", err)
	}
}
