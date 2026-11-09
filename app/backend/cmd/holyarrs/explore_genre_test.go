package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getJSON(t *testing.T, url string, want int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s: %d, want %d", url, resp.StatusCode, want)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func (f *recordingTMDB) asked(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, part) {
			n++
		}
	}
	return n
}

// route: GET /api/v1/explore/top
// One ranked row per chip: All my services merges the picked services by
// popularity (top 10, each once), one service (Netflix too) asks TMDB for
// that service; lists are kept a week.
func TestExploreTopOnYourServices(t *testing.T) {
	t.Parallel()
	a, _, f := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	base := srv.URL + "/api/v1/explore/top"

	all := getJSON(t, base+"?type=all&chip=all&services=8|337&region=US", 200)
	items := all["items"].([]any)
	if all["source"] != "popular" || len(items) != 10 {
		t.Fatalf("all chip %v", all)
	}
	seen := map[string]bool{}
	for i, it := range items {
		m := it.(map[string]any)
		if m["rank"] != float64(i+1) {
			t.Fatalf("rank %v at %d", m["rank"], i)
		}
		k := m["kind"].(string) + m["tmdb_id"].(string)
		if seen[k] {
			t.Fatalf("%s twice", k)
		}
		seen[k] = true
	}
	if f.asked("with_watch_providers=8%7C337") != 2 {
		t.Fatalf("both kinds not asked with both services: %v", f.urls)
	}
	getJSON(t, base+"?type=all&chip=all&services=8|337&region=US", 200)
	if f.asked("with_watch_providers=8%7C337") != 2 {
		t.Fatal("weekly list not kept")
	}

	one := getJSON(t, base+"?type=movies&chip=337&services=8|337&region=US", 200)
	if one["source"] != "popular" || f.asked("with_watch_providers=337") != 1 || f.asked("discover/tv?") != 1 {
		t.Fatalf("one service %v %v", one, f.urls)
	}

	// Netflix is like every other service: the daily list on Netflix.
	nf := getJSON(t, base+"?type=shows&chip=8&services=8|337&region=US", 200)
	if nf["source"] != "popular" || nf["week"] != nil || len(nf["items"].([]any)) != 10 {
		t.Fatalf("netflix chip %v %v", nf, f.urls)
	}

	// In a genre the Netflix chip is TMDB popularity on Netflix within it.
	g := getJSON(t, base+"?type=movies&chip=8&services=8&region=US&genre=18", 200)
	if g["source"] != "popular" || f.asked("with_genres=18") == 0 {
		t.Fatalf("genre chip %v", g)
	}
	empty := getJSON(t, base+"?type=movies&chip=all&region=US", 200)
	if len(empty["items"].([]any)) != 0 {
		t.Fatal("no services picked should be empty")
	}
	for _, bad := range []string{"?type=movies&chip=all&region=usa", "?type=movies&chip=8|9&region=US", "?type=all&chip=8&region=US&genre=18", "?type=movies&chip=8&region=US&genre=999", "?type=x&region=US"} {
		getJSON(t, base+bad, 400)
	}
}

// route: GET /api/v1/explore/genre
// A genre page: Trending, Best new, Because you have (a Library title in
// the genre), Top-rated classics and mood rows, each from TMDB discover
// with the genre; a title once per page; hero from the trending row.
func TestExploreGenrePage(t *testing.T) {
	t.Parallel()
	a, s, f := exploreRig(t)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET genres = '["Drama"]' WHERE kind = 'movie'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	pg := getJSON(t, srv.URL+"/api/v1/explore/genre?type=movies&genre=18", 200)
	if g := pg["genre"].(map[string]any); g["name"] != "Drama" || g["kind"] != "movie" {
		t.Fatalf("genre %v", g)
	}
	ids := map[string]string{}
	seen := map[string]bool{}
	for _, r := range pg["rows"].([]any) {
		row := r.(map[string]any)
		ids[row["id"].(string)] = row["title"].(string)
		for _, it := range row["items"].([]any) {
			k := it.(map[string]any)["tmdb_id"].(string)
			if seen[k] {
				t.Fatalf("%s twice on the page", k)
			}
			seen[k] = true
		}
	}
	if ids["genre_trending"] != "Trending drama" || ids["genre_because"] != "Because you have M11" {
		t.Fatalf("rows %v", ids)
	}
	if len(pg["hero"].([]any)) == 0 {
		t.Fatal("no hero")
	}
	if f.asked("with_genres=18") < 3 || f.asked("/movie/11/similar") != 1 {
		t.Fatalf("lists %v", f.urls)
	}
	for _, bad := range []string{"?type=all&genre=18", "?type=movies", "?type=movies&genre=10759", "?type=shows&genre=x"} {
		getJSON(t, srv.URL+"/api/v1/explore/genre"+bad, 400)
	}
}
