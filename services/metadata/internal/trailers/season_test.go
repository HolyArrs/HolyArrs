package trailers

import (
	"context"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

type seasonTMDB struct{ paths []string }

func (f *seasonTMDB) Fetch(_ context.Context, q model.Query) (model.Result, error) {
	f.paths = append(f.paths, q.Path)
	switch q.Path {
	case "/tv/1416/videos", "/tv/1416/season/22":
		return model.Result{Data: []byte(`{"results":[],"videos":{"results":[]}}`), Complete: true}, nil
	case "/tv/1416":
		return model.Result{Data: []byte(`{"seasons":[{"season_number":0},{"season_number":21},{"season_number":22}]}`), Complete: true}, nil
	case "/tv/1416/season/21":
		return model.Result{Data: []byte(`{"videos":{"results":[{"key":"s21trailer1","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"}]}}`), Complete: true}, nil
	case "/tv/7/videos", "/tv/7":
		return model.Result{Data: []byte(`{"results":[],"seasons":[]}`), Complete: true}, nil
	}
	return model.Result{}, &model.FetchError{Code: "upstream_status", Status: 404}
}

// A show with no trailer of its own takes its newest season's trailer.
func TestShowTakesNewestSeasonTrailer(t *testing.T) {
	f := &seasonTMDB{}
	s := open(t, Options{Fetcher: f})
	p, err := s.Pick(context.Background(), "tv", 1416, "en", "tester")
	if err != nil || p.Key != "s21trailer1" {
		t.Fatalf("%+v %v %v", p, err, f.paths)
	}
}

// No trailer anywhere is asked again after 15 minutes, not after 7 days.
func TestNoTrailerRetriedSoon(t *testing.T) {
	f := &seasonTMDB{}
	now := time.Now()
	s := open(t, Options{Fetcher: f, Now: func() time.Time { return now }})
	_, _ = s.Pick(context.Background(), "tv", 7, "en", "tester")
	n := len(f.paths)
	now = now.Add(16 * time.Minute)
	_, _ = s.Pick(context.Background(), "tv", 7, "en", "tester")
	if len(f.paths) == n {
		t.Fatal("empty kept past 15 minutes")
	}
}
