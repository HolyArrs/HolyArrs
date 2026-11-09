package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// route: GET /api/v1/explore/strip
// The strip never waits for Browse sections (partial while they build),
// then answers at most 18 categories within the section and near-duplicate
// limits, and drops a category a row below already names.
func TestExploreStripRoute(t *testing.T) {
	t.Parallel()
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
	get("/api/v1/explore/strip?type=nope", 400)
	// Nothing built for Movies: the answer does not wait.
	first := get("/api/v1/explore/strip?type=movies", 200)
	if _, ok := first["tiles"].([]any); !ok || first["partial"] != true {
		t.Fatalf("tiles %v", first)
	}
	// The built sections: every category with enough titles that have art.
	share := a.disc.BrowseShare("shows", false, "")
	n := 0
	for _, s := range engine.BrowseSections {
		var res []engine.CategoryResult
		for _, c := range engine.SectionCategories(s.ID, "shows", false, time.Now()) {
			var items []engine.Item
			for i := 0; i < engine.BrowseMin+2; i++ {
				n++
				items = append(items, engine.Item{TMDBID: strconv.Itoa(n), Kind: "show", Backdrop: "/s" + strconv.Itoa(n), Pop: float64(1000 - n%997), Genres: []string{"Drama"}})
			}
			res = append(res, engine.CategoryResult{Cat: c, Items: items})
		}
		share.Put(s.ID, res)
	}
	got := get("/api/v1/explore/strip?type=shows", 200)
	if got["partial"] != false {
		t.Fatalf("partial after build: %v", got["partial"])
	}
	tiles := got["tiles"].([]any)
	if len(tiles) == 0 || len(tiles) > engine.StripMax {
		t.Fatalf("%d tiles", len(tiles))
	}
	per := map[string]int{}
	names := map[string]bool{}
	for _, x := range tiles {
		m := x.(map[string]any)
		per[m["section"].(string)]++
		n := m["name"].(string)
		if names[n] {
			t.Fatalf("%s twice", n)
		}
		names[n] = true
	}
	for s, n := range per {
		if n > 3 {
			t.Fatalf("section %s has %d", s, n)
		}
	}
	first0 := tiles[0].(map[string]any)["name"].(string)
	again := get("/api/v1/explore/strip?type=shows&row="+url.QueryEscape("Popular "+first0+" tonight"), 200)
	for _, x := range again["tiles"].([]any) {
		if x.(map[string]any)["name"] == first0 && len(again["tiles"].([]any)) < len(tiles) {
			t.Fatalf("%s kept though a row names it", first0)
		}
	}
	if n := again["tiles"].([]any); len(n) > 0 && n[0].(map[string]any)["name"] == first0 {
		t.Fatalf("%s still leads though a row names it", first0)
	}
}

// route: POST /api/v1/explore/strip/counts
// route: DELETE /api/v1/explore/strip/counts
// Shown and opened counts add up per category and day; bad bodies are
// refused; Reset my recommendations clears the member's rows.
func TestExploreStripCountsRoute(t *testing.T) {
	t.Parallel()
	a, s, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	post := func(body string) int {
		resp, err := http.Post(srv.URL+"/api/v1/explore/strip/counts", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for body, want := range map[string]int{
		`{"event":"shown","categories":["heists","sci-fi","heists"]}`: 204,
		`{"event":"open","categories":["heists"]}`:                    204,
		`{"event":"open","categories":["heists","space"]}`:            400,
		`{"event":"play","categories":["heists"]}`:                    400,
		`{"event":"shown","categories":["DROP TABLE"]}`:               400,
		`{"event":"shown","categories":[]}`:                           400,
		`nope`:                                                        400,
	} {
		if got := post(body); got != want {
			t.Errorf("%s = %d, want %d", body, got, want)
		}
	}
	var shown, opened, n int
	if err := s.Read().QueryRow("SELECT shown, opened FROM explore_category_counts WHERE category_id = 'heists'").Scan(&shown, &opened); err != nil {
		t.Fatal(err)
	}
	if shown != 1 || opened != 1 {
		t.Fatalf("heists shown %d opened %d", shown, opened)
	}
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/explore/strip/counts", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset = %d", resp.StatusCode)
	}
	_ = s.Read().QueryRow("SELECT count(*) FROM explore_category_counts").Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows after reset", n)
	}
}
