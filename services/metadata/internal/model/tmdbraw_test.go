package model

import (
	"net/url"
	"testing"
)

func TestTMDBReadWatchProviders(t *testing.T) {
	ok := []struct {
		path string
		q    url.Values
	}{
		{"/watch/providers/movie", url.Values{"watch_region": {"US"}, "language": {"en-US"}}},
		{"/watch/providers/tv", url.Values{"watch_region": {"GB"}}},
		{"/discover/movie", url.Values{"with_watch_providers": {"8|337|1899"}, "watch_region": {"US"}, "with_watch_monetization_types": {"flatrate"}, "sort_by": {"popularity.desc"}}},
		{"/discover/tv", url.Values{"with_watch_providers": {"8"}, "watch_region": {"DE"}}},
		{"/discover/movie", url.Values{"primary_release_date.gte": {"2024-10-19"}, "primary_release_date.lte": {"2026-10-19"}, "vote_average.gte": {"7.5"}, "vote_count.gte": {"300"}, "sort_by": {"vote_average.desc"}}},
		{"/discover/tv", url.Values{"first_air_date.gte": {"2024-10-19"}, "first_air_date.lte": {"2026-10-19"}, "vote_average.gte": {"10"}}},
	}
	for _, c := range ok {
		if _, err := TMDBRead(c.path, c.q); err != nil {
			t.Fatalf("%s %v: %v", c.path, c.q, err)
		}
	}
	bad := []struct {
		path string
		q    url.Values
	}{
		{"/watch/providers/person", nil},
		{"/watch/providers/regions", nil},
		{"/discover/movie", url.Values{"with_watch_providers": {"8,337"}}},
		{"/discover/movie", url.Values{"with_watch_providers": {"0"}}},
		{"/discover/movie", url.Values{"with_watch_providers": {"8|"}}},
		{"/discover/movie", url.Values{"with_watch_providers": {"abc"}}},
		{"/discover/movie", url.Values{"watch_region": {"us"}}},
		{"/discover/movie", url.Values{"watch_region": {"USA"}}},
		{"/discover/movie", url.Values{"with_watch_monetization_types": {"rent"}}},
		{"/discover/movie", url.Values{"vote_average.gte": {"11"}}},
		{"/discover/movie", url.Values{"vote_average.gte": {"7.55"}}},
		{"/discover/movie", url.Values{"vote_average.gte": {"-1"}}},
		{"/discover/movie", url.Values{"vote_count.gte": {"abc"}}},
		{"/discover/movie", url.Values{"primary_release_date.gte": {"2024-1-1"}}},
		{"/discover/tv", url.Values{"first_air_date.lte": {"tomorrow"}}},
		{"/discover/movie", url.Values{"with_watch_monetization_types": {"flatrate|free"}}},
	}
	for _, c := range bad {
		if _, err := TMDBRead(c.path, c.q); err == nil {
			t.Fatalf("accepted %s %v", c.path, c.q)
		}
	}
}

func TestTMDBReadTitleWatchProviders(t *testing.T) {
	for _, p := range []string{"/movie/550/watch/providers", "/tv/1396/watch/providers"} {
		if _, err := TMDBRead(p, nil); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	for _, p := range []string{"/person/1/watch/providers", "/movie/0/watch/providers", "/movie/abc/watch/providers", "/movie/550/watch/providers/", "/movie/550/watch/provider", "/movie/550/watch/providers/extra", "/movie/1234567890123/watch/providers", "/collection/1/watch/providers"} {
		if _, err := TMDBRead(p, nil); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}

func TestTMDBReadKeywordsAndPersonCredits(t *testing.T) {
	for _, p := range []string{"/movie/550/keywords", "/tv/1396/keywords", "/person/287/movie_credits", "/person/66633/tv_credits"} {
		if _, err := TMDBRead(p, url.Values{"language": {"en-US"}}); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	for _, p := range []string{"/person/1/keywords", "/movie/0/keywords", "/movie/550/keywords/", "/movie/550/keyword", "/movie/287/movie_credits", "/person/abc/tv_credits", "/person/287/movie_credits/extra", "/collection/1/keywords"} {
		if _, err := TMDBRead(p, nil); err == nil {
			t.Fatalf("%s: want error", p)
		}
	}
}

func TestTMDBReadKeywordSearchAndDiscover(t *testing.T) {
	if _, err := TMDBRead("/search/keyword", url.Values{"query": {"found footage"}}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"9663", "9663|12339", "9663,12339"} {
		if _, err := TMDBRead("/discover/movie", url.Values{"with_genres": {"27"}, "with_keywords": {v}, "sort_by": {"popularity.desc"}}); err != nil {
			t.Fatalf("with_keywords=%s: %v", v, err)
		}
	}
	for _, v := range []string{"", "0", "abc", "1;2", "1||2", "1234567890"} {
		if _, err := TMDBRead("/discover/tv", url.Values{"with_keywords": {v}}); err == nil {
			t.Fatalf("with_keywords=%q accepted", v)
		}
	}
	if _, err := TMDBRead("/search/company", url.Values{"query": {"x"}}); err == nil {
		t.Fatal("search/company accepted")
	}
}

func TestTMDBReadReviews(t *testing.T) {
	for _, p := range []string{"/movie/550/reviews", "/tv/1396/reviews"} {
		if _, err := TMDBRead(p, url.Values{"language": {"en-US"}, "page": {"2"}}); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	for _, p := range []string{"/person/287/reviews", "/movie/0/reviews", "/movie/550/reviews/", "/review/abc", "/collection/1/reviews"} {
		if _, err := TMDBRead(p, nil); err == nil {
			t.Fatalf("%s: want error", p)
		}
	}
}

func TestTMDBReadPersonPopular(t *testing.T) {
	for _, p := range []string{"1", "5", "8"} {
		if _, err := TMDBRead("/person/popular", url.Values{"page": {p}, "language": {"en-US"}}); err != nil {
			t.Fatalf("page %s: %v", p, err)
		}
	}
	if _, err := TMDBRead("/person/popular", nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"0", "9", "500", "x"} {
		if _, err := TMDBRead("/person/popular", url.Values{"page": {p}}); err == nil {
			t.Fatalf("accepted page %s", p)
		}
	}
}

func TestTMDBReadNewWindows(t *testing.T) {
	ok := []struct {
		path string
		v    url.Values
	}{
		{"/discover/tv", url.Values{"air_date.gte": {"2026-10-07"}, "air_date.lte": {"2026-10-21"}, "sort_by": {"popularity.desc"}}},
		{"/discover/movie", url.Values{"release_date.gte": {"2026-09-06"}, "release_date.lte": {"2026-10-21"}, "with_release_type": {"4|3"}, "region": {"US"}}},
		{"/discover/movie", url.Values{"with_watch_providers": {"8|337"}, "watch_region": {"US"}, "with_release_type": {"4"}}},
	}
	for _, c := range ok {
		if _, err := TMDBRead(c.path, c.v); err != nil {
			t.Fatalf("%s %v: %v", c.path, c.v, err)
		}
	}
	for _, v := range []url.Values{{"air_date.gte": {"2026-1-1"}}, {"with_release_type": {"7"}}, {"with_release_type": {"4|"}}, {"release_date.lte": {"soon"}}} {
		if _, err := TMDBRead("/discover/movie", v); err == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}
