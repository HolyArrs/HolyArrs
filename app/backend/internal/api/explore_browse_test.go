package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// route: GET /api/v1/explore/browse/section
// route: GET /api/v1/explore/browse/category
// Browse sections answer tile lists with unique backdrops, refuse unknown
// sections, and a category view lists its titles.
func TestSearchBrowseSectionsAndCategory(t *testing.T) {
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	get := func(path string, want int) map[string]any {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s = %d, want %d", path, resp.StatusCode, want)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	br := get("/api/v1/explore/browse?type=shows", 200)
	if s, ok := br["sections"].([]any); !ok || len(s) != 7 {
		t.Fatalf("sections %v", br["sections"])
	}
	used := map[string]bool{}
	for _, sec := range []string{"genres", "moods", "eras"} {
		got := get("/api/v1/explore/browse/section?type=shows&section="+sec, 200)
		tiles, ok := got["tiles"].([]any)
		if !ok {
			t.Fatalf("%s tiles %v", sec, got)
		}
		for _, x := range tiles {
			b := x.(map[string]any)["backdrop"].(string)
			if b == "" || used[b] {
				t.Fatalf("backdrop %q empty or repeated", b)
			}
			used[b] = true
		}
	}
	get("/api/v1/explore/browse/section?type=shows&section=nope", 400)
	get("/api/v1/explore/browse/section?type=nope&section=moods", 400)
	c := get("/api/v1/explore/browse/category?type=movies&id=action", 200)
	if _, ok := c["items"].([]any); !ok || c["name"] != "Action" {
		t.Fatalf("category %v", c)
	}
	get("/api/v1/explore/browse/category?type=movies&id=limited", 404)
	get("/api/v1/explore/browse/category?type=movies&id=nope", 404)
}

// The warm-up builds Genres for every tab first, then each section in page order.
func TestBrowseWarmOrder(t *testing.T) {
	got := browseWarmOrder([]string{"all", "movies", "shows"})
	if len(got) != 21 {
		t.Fatalf("jobs %d", len(got))
	}
	for i, sw := range []string{"all", "movies", "shows"} {
		if got[i] != (browseJob{sw, "genres"}) {
			t.Fatalf("job %d = %v", i, got[i])
		}
	}
	if got[3].section != "moods" || got[20] != (browseJob{"shows", "collections"}) {
		t.Fatalf("order %v", got)
	}
}

// The shared cache is per tab and rating limit: a Kid's maximum rating never
// shares an adult's sections.
func TestBrowseShareKeyedByKidMax(t *testing.T) {
	a := &API{}
	open, pg, pg13 := a.browseShare("all", false, ""), a.browseShare("all", true, "PG"), a.browseShare("all", true, "PG-13")
	if open == pg || pg == pg13 || a.browseShare("all", true, "PG") != pg || a.browseShare("movies", false, "") == open {
		t.Fatal("shares not keyed by tab, kid and maximum rating")
	}
	ls := a.browseLimits()
	if ls[0] != (browseLimit{false, ""}) || len(ls) != 3 {
		t.Fatalf("limits %v", ls)
	}
}

// A section still building answers at once with building, never blocking.
func TestSearchBrowseSectionBuilding(t *testing.T) {
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	old := browseWait
	browseWait = 20 * time.Millisecond
	defer func() { browseWait = old }()
	sh := a.browseShare("shows", false, "")
	hold := make(chan struct{})
	sh.mu.Lock()
	sh.busy["genres"] = hold
	sh.mu.Unlock()
	start := time.Now()
	resp, err := http.Get(srv.URL + "/api/v1/explore/browse/section?type=shows&section=moods")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if out["building"] != true || time.Since(start) > 5*time.Second {
		t.Fatalf("answer %v after %v", out, time.Since(start))
	}
	sh.mu.Lock()
	delete(sh.busy, "genres")
	sh.mu.Unlock()
	close(hold)
	// Built in the background: asking again soon answers tiles.
	for i := 0; i < 100; i++ {
		resp, err = http.Get(srv.URL + "/api/v1/explore/browse/section?type=shows&section=moods")
		if err != nil {
			t.Fatal(err)
		}
		out = map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if out["building"] == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("still building: %v", out)
}
