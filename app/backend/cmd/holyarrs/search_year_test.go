package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Search words often end in the year of the title ("Strike 2017"). A source
// asked for every word finds almost nothing then, so the year is taken off
// the words and given to the source as a year.

// route: GET /api/v1/titles/lookup
func TestLookupTakesTheYearOffTheWords(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	for term, want := range map[string]string{
		"Dune 2024":      `"name":"Dune: Part Two"`, // the year belongs to the year
		"Dune":           `"name":"Dune: Part Two"`, // the words on their own answer as before
		"Dune 1999":      `"movies":[]`,             // no Dune of that year
		"2024":           `"movies":[]`,             // a year alone is a name, not a year to take off
		"Dune 1990 2024": `"movies":[]`,             // the words without their year are still the words
		"Dune (2024)":    `"name":"Dune: Part Two"`, // the year in brackets is a year too
	} {
		b := string(do(t, "GET", srv.URL+"/api/v1/titles/lookup?term="+strings.ReplaceAll(term, " ", "+"), "", 200))
		if !strings.Contains(b, want) {
			t.Errorf("%s: %s", term, b)
		}
	}
	// A year asked for on its own is the one used.
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/lookup?term=Dune&year=2024", "", 200))
	if !strings.Contains(b, `"name":"Dune: Part Two"`) {
		t.Fatalf("an explicit year: %s", b)
	}
}

// route: GET /api/v1/shows/lookup
func TestShowLookupTakesTheYearOffTheWords(t *testing.T) {
	t.Parallel()
	r := newShowAskRig(t)
	var look struct {
		Shows []showResult `json:"shows"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/shows/lookup?term=Lab+Show+2025", "", 200), &look)
	if len(look.Shows) != 1 || look.Shows[0].Name != "Lab Show" {
		t.Fatalf("a show with its year in the words: %+v", look.Shows)
	}
	// The words on their own answer as before.
	look.Shows = nil
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/shows/lookup?term=Lab+Show", "", 200), &look)
	if len(look.Shows) != 1 || look.Shows[0].Name != "Lab Show" {
		t.Fatalf("a show on its own: %+v", look.Shows)
	}
	// A show of another year is still found, after the one of this year.
	look.Shows = nil
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/shows/lookup?term=Lab+Show+1999", "", 200), &look)
	if len(look.Shows) != 1 || look.Shows[0].Name != "Lab Show" {
		t.Fatalf("a show of another year: %+v", look.Shows)
	}
}

// route: GET /api/v1/search/suggest
func TestSuggestFindsAMovieWithItsYear(t *testing.T) {
	t.Parallel()
	r, _ := suggestRig(t, 0)
	got := suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=Radio+Days+2003&kind=movie", "", 200))
	for _, it := range got.Items {
		if it.Kind == "movie" && it.Name == "Radio Days 3" {
			return
		}
	}
	t.Fatalf("a movie with its year in the words: %+v", got.Items)
}
