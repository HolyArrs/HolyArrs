package metadata

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
)

func TestWikidataReadsServer(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/v1/wikidata/title/tv/1396?":
			_, _ = w.Write([]byte(`{"qid":"Q1079","tallies":[{"family":"emmy","wins":15,"nominations":40}],"awards":[{"name":"Primetime Emmy Award for Outstanding Drama Series","family":"emmy","won":true,"year":2013}]}`))
		case "/v1/wikidata/person/31?":
			_, _ = w.Write([]byte(`{"qid":"Q2263","born":"1956-07-09","summary":{"extract":"An actor.","url":"https://en.wikipedia.org/wiki/Tom_Hanks","license":"CC BY-SA 4.0"}}`))
		case "/v1/wikidata/person/33?":
			_, _ = w.Write([]byte(`{"qid":"Q3","summary":{"extract":"x","url":"https://en.wikipedia.org/wiki/X","license_url":"javascript:alert(1)"}}`))
		case "/v1/wikidata/person/34?":
			_, _ = w.Write([]byte(`{"qid":"Q4","summary":{"extract":"x","url":"javascript:alert(1)//en.wikipedia.org/"}}`))
		case "/v1/wikidata/person/32?":
			_, _ = w.Write([]byte(`{"qid":"Q1","summary":{"extract":"x","url":"https://evil.test/"}}`))
		case "/v1/wikidata/award-winners?kind=movie":
			_, _ = w.Write([]byte(`{"kind":"movie","results":[{"tmdb":13,"wins":6},{"tmdb":603,"wins":4}]}`))
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	e, err := s.WikiTitle(ctx, "show", "1396")
	if err != nil || e.QID != "Q1079" || e.Tallies[0].Wins != 15 || len(e.Awards) != 1 {
		t.Fatalf("%+v %v", e, err)
	}
	p, err := s.WikiPerson(ctx, "31", "")
	if err != nil || p.Summary == nil || p.Born != "1956-07-09" {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := s.WikiPerson(ctx, "32", ""); p.Summary != nil {
		t.Fatal("kept a bio that is not from Wikipedia")
	}
	if p, _ := s.WikiPerson(ctx, "33", ""); p.Summary == nil || p.Summary.LicenseURL != "https://creativecommons.org/licenses/by-sa/4.0/" {
		t.Fatalf("license link not replaced: %+v", p.Summary)
	}
	if p, _ := s.WikiPerson(ctx, "34", ""); p.Summary != nil {
		t.Fatal("kept a javascript: page link")
	}
	if _, err := s.WikiTitle(ctx, "movie", "1"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.WikiTitle(ctx, "album", "1"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatal(err)
	}
	w, err := s.AwardWinners(ctx, "movie")
	if err != nil || len(w) != 2 || w[0].TMDB != 13 {
		t.Fatalf("%+v %v", w, err)
	}
}

func TestAwardBadges(t *testing.T) {
	got := AwardBadges([]WikiTally{{Family: "bafta", Nominations: 1}, {Family: "oscar", Wins: 2}, {Family: "grammy", Wins: 1}, {Family: "other", Wins: 9}})
	if !slices.Equal(got, []string{"2 Oscars", "Grammy winner", "BAFTA nominee"}) {
		t.Fatal(got)
	}
}
