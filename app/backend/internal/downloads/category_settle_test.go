package downloads

import (
	"encoding/json"
	"slices"
	"testing"
)

// route: GET /api/v1/downloaders/{id}/categories
func TestAddPicksClientCategories(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key, "tv", "Movies", "software")
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key,
		"allow_local_network": true}), 201), &d)
	if d.CategoryShows != "tv" || d.CategoryMovies != "Movies" || d.CategoryMusic != "" {
		t.Fatalf("picks %+v", d)
	}
	// Music has no fit: it falls back to the main one, which is made.
	if got := cats(); !slices.Contains(got, "holyarrs") || len(got) != 5 {
		t.Fatalf("cats %v", got)
	}
	var out struct {
		Listed     bool              `json:"listed"`
		Categories []string          `json:"categories"`
		Picks      map[string]string `json:"picks"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/categories", "", 200), &out)
	if !out.Listed || slices.Contains(out.Categories, "*") || out.Picks["show"] != "tv" || out.Picks["movie"] != "Movies" || out.Picks["artist"] != "" {
		t.Fatalf("list %+v", out)
	}
}

func TestAddAllFitMakesNothing(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key, "tv-sonarr", "radarr", "lidarr")
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key,
		"allow_local_network": true}), 201), &d)
	if d.CategoryShows != "tv-sonarr" || d.CategoryMovies != "radarr" || d.CategoryMusic != "lidarr" {
		t.Fatalf("picks %+v", d)
	}
	if got := cats(); len(got) != 4 {
		t.Fatalf("made one it did not need: %v", got)
	}
}

func TestEditKeepsChosenKinds(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key, "tv")
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key,
		"allow_local_network": true, "category_shows": "", "category_movies": "", "category_music": ""}), 201), &d)
	if d.CategoryShows != "tv" {
		t.Fatalf("add picks %+v", d)
	}
	// The user picked the main category for shows: an edit keeps that.
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/downloaders/"+itoa(d.ID), body(map[string]any{"name": "SAB", "category_shows": "", "category_movies": "films", "category_music": ""}), 200), &d)
	if d.CategoryShows != "" || d.CategoryMovies != "films" {
		t.Fatalf("edit %+v", d)
	}
	if got := cats(); !slices.Contains(got, "films") || !slices.Contains(got, "tv") {
		t.Fatalf("cats %v", got)
	}
}

func TestUnreachableAddStillSaves(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "Gone", "kind": "sabnzbd", "url": "http://127.0.0.1:1", "secret": key,
		"allow_local_network": true}), 201)
}
