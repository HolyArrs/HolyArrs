package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
)

// A fresh install has no card pictures stored yet. The first Explore and
// New & Popular answers must still carry a hero (its pictures fetched on
// that answer), not only after a reload once the paced filler got there.
func TestHeroOnFirstVisit(t *testing.T) {
	t.Parallel()
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/images"):
			_, _ = w.Write([]byte(`{"backdrops":[{"file_path":"/b.jpg","iso_639_1":null,"vote_average":5}],"logos":[{"file_path":"/l.png","iso_639_1":"en","vote_average":5}]}`))
		case strings.HasSuffix(r.URL.Path, "/genre/movie/list"), strings.HasSuffix(r.URL.Path, "/genre/tv/list"):
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"}]}`))
		default:
			var parts []string
			base := len(r.URL.String()) * 7
			for i := 0; i < 30; i++ {
				id := 1000 + (base+i*13)%900
				parts = append(parts, fmt.Sprintf(`{"id":%d,"title":"M%d","name":"S%d","release_date":"2026-09-01","first_air_date":"2026-09-01","genre_ids":[18],"popularity":%d,"vote_average":7.5,"vote_count":400,"backdrop_path":"/bd%d.jpg","poster_path":"/p%d.jpg"}`, id, id, id, 100-i, id, id))
			}
			_, _ = w.Write([]byte(`{"results":[` + strings.Join(parts, ",") + `]}`))
		}
	}))
	t.Cleanup(tmdb.Close)
	p := &metadata.TMDB{BaseURL: tmdb.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
	_, s := discoverRig(t, p)
	a := buildHandlers(services{Store: s, Metadata: p, Version: "test"})
	a.explore.Paced = &engine.Paced{Source: discover.ListerSource{L: p}, Gap: time.Millisecond}
	// The paced filler stands still, as on a fresh install minutes before it
	// reaches the hero's titles.
	a.explore.Cards.Gap = time.Hour
	if err := a.explore.BuildPool(context.Background(), 1, "shows"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	get := func(path string) map[string]any {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, resp)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	h := get("/api/v1/explore/head?type=shows")
	if len(h["head"].([]any)) == 0 {
		t.Fatalf("no rows: %v", h)
	}
	// The full hero, not just the one title the paced filler reached.
	if hero, _ := h["hero"].([]any); len(hero) < 2 {
		t.Fatalf("Explore: %d hero titles on the first visit", len(hero))
	}
	var np map[string]any
	for i := 0; i < 50; i++ {
		np = get("/api/v1/explore/new?type=all&page=0")
		if np["building"] != true {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if rows, _ := np["rows"].([]any); len(rows) > 0 {
		if hero, _ := np["hero"].([]any); len(hero) == 0 {
			t.Fatal("New & Popular: no hero on the first visit")
		}
	}
}
