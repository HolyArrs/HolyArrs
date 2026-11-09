package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// route: GET /api/v1/titles
// The brief list carries the same titles, names, posters, dates and slot
// states as the full list, in the same order, and nothing else.
func TestTitlesBriefMatchesFull(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	var full struct {
		Titles []TitleView `json:"titles"`
	}
	var brief struct {
		Titles []mediafiles.TitleBrief `json:"titles"`
	}
	fullRaw := do(t, "GET", srv.URL+"/api/v1/titles", "", 200)
	briefRaw := do(t, "GET", srv.URL+"/api/v1/titles?brief=1", "", 200)
	if err := json.Unmarshal(fullRaw, &full); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(briefRaw, &brief); err != nil {
		t.Fatal(err)
	}
	if len(brief.Titles) != len(full.Titles) || len(full.Titles) == 0 {
		t.Fatalf("brief %d titles, full %d", len(brief.Titles), len(full.Titles))
	}
	for i, f := range full.Titles {
		b := brief.Titles[i]
		if b.ID != f.ID || b.Kind != f.Kind || b.Name != f.Name || !slices.Equal(b.Artwork, f.Artwork) {
			t.Fatalf("title %d: brief %+v, full id %d %s %q %v", i, b, f.ID, f.Kind, f.Name, f.Artwork)
		}
		sameDate := func(a, b *time.Time) bool { return (a == nil) == (b == nil) && (a == nil || a.Equal(*b)) }
		if !sameDate(b.CinemaDate, f.CinemaDate) || !sameDate(b.DigitalDate, f.DigitalDate) || !sameDate(b.PhysicalDate, f.PhysicalDate) {
			t.Fatalf("title %d dates: brief %v %v %v, full %v %v %v", i, b.CinemaDate, b.DigitalDate, b.PhysicalDate, f.CinemaDate, f.DigitalDate, f.PhysicalDate)
		}
		var states []string
		for _, s := range f.Slots {
			states = append(states, s.State)
		}
		var got []string
		for _, s := range b.Slots {
			got = append(got, s.State)
		}
		if !slices.Equal(got, states) {
			t.Fatalf("title %d slot states: brief %v, full %v", i, got, states)
		}
	}
	var keys []map[string]json.RawMessage
	var wrap struct {
		Titles json.RawMessage `json:"titles"`
	}
	_ = json.Unmarshal(briefRaw, &wrap)
	_ = json.Unmarshal(wrap.Titles, &keys)
	for _, k := range keys {
		if _, ok := k["versions"]; ok {
			t.Fatal("brief list carries versions")
		}
	}
	if len(briefRaw) >= len(fullRaw) {
		t.Fatalf("brief %d bytes, full %d", len(briefRaw), len(fullRaw))
	}
	// Its own ETag answers 304 like the full list's.
	resp, err := http.Get(srv.URL + "/api/v1/titles?brief=1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if tag := resp.Header.Get("ETag"); tag != "" {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/titles?brief=1", nil)
		req.Header.Set("If-None-Match", tag)
		again, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		again.Body.Close()
		if again.StatusCode != http.StatusNotModified {
			t.Fatalf("If-None-Match: %d, want 304", again.StatusCode)
		}
	}
}
