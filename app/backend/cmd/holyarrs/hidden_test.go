package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// TestHiddenTitlesLeaveBrowsingOnly: a household hide takes a title out of
// browsing for everyone on the next request, while its link, the Library
// and a manager's add search still find it; a person's hide is theirs
// alone; only owners and admins hide; Show again brings it back.
func TestHiddenTitlesLeaveBrowsingOnly(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	var hide struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(k.owner.call("POST", "/api/v1/hidden", `{"kind": "movie", "tmdb_id": "4101", "name": "Gentle Meadow"}`, 201), &hide)
	k.owner.call("POST", "/api/v1/hidden", fmt.Sprintf(`{"kind": "movie", "tmdb_id": "4111", "user_id": %d}`, k.kai.id), 201)
	k.ana.call("POST", "/api/v1/hidden", `{"kind": "movie", "tmdb_id": "4102"}`, 403)
	k.kai.call("GET", "/api/v1/hidden", "", 403)

	sameNames(t, "member trending", namesIn(t, string(k.ana.call("GET", "/api/v1/discover?row=trending", "", 200)), "items"),
		"Dark Alley", "Sunny Pond", "Nameless Reel", "Iron Verdict", "Night Courier", "Blood Coast", "Puppet Lane", "Cold Precinct", "Harbor Friends")
	sameNames(t, "kid trending", namesIn(t, string(k.kai.call("GET", "/api/v1/discover?row=trending", "", 200)), "items"),
		"Puppet Lane", "Harbor Friends")

	// The link still opens, marked for the viewer.
	if b := string(k.ana.call("GET", "/api/v1/metadata/movie/4101", "", 200)); !strings.Contains(b, `"hidden":true`) || strings.Contains(b, `"hides"`) {
		t.Fatalf("member title page: %s", b)
	}
	if b := string(k.owner.call("GET", "/api/v1/metadata/movie/4101", "", 200)); !strings.Contains(b, `"hides":[{`) {
		t.Fatalf("owner title page: %s", b)
	}
	// The Library keeps it.
	if got := namesIn(t, string(k.ana.call("GET", "/api/v1/titles", "", 200)), "titles"); !slices.Contains(got, "Gentle Meadow") {
		t.Fatalf("Library lost the hidden title: %v", got)
	}
	// Add search: the owner finds it marked; a member does not see it.
	var found struct {
		Movies []struct {
			Name   string `json:"name"`
			Hidden bool   `json:"hidden"`
		} `json:"movies"`
	}
	_ = json.Unmarshal(k.owner.call("GET", "/api/v1/titles/lookup?term=any", "", 200), &found)
	marked := false
	for _, m := range found.Movies {
		if m.Name == "Gentle Meadow" {
			marked = m.Hidden
		}
	}
	if !marked {
		t.Fatalf("owner lookup: %+v", found.Movies)
	}
	if got := namesIn(t, string(k.ana.call("GET", "/api/v1/titles/lookup?term=any", "", 200)), "movies"); slices.Contains(got, "Gentle Meadow") {
		t.Fatalf("member lookup shows the hidden title: %v", got)
	}

	k.owner.call("DELETE", fmt.Sprintf("/api/v1/hidden/%d", hide.ID), "", 204)
	if got := namesIn(t, string(k.ana.call("GET", "/api/v1/discover?row=trending", "", 200)), "items"); !slices.Contains(got, "Gentle Meadow") {
		t.Fatalf("after Show again: %v", got)
	}
}

// route: GET /api/v1/search/suggest
// A hidden title, and every title of a hidden genre, leave the suggestions;
// the genres come with the name search answer.
func TestHiddenLeavesSuggestions(t *testing.T) {
	t.Parallel()
	r, _ := suggestRig(t, 0)
	r.api.s.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "Radio Fear", Year: 2001, IDs: map[string]string{"tmdb": "920001"}, Genres: []string{"Horror", "Thriller"}},
		{Name: "Radio Days", Year: 2002, IDs: map[string]string{"tmdb": "920002"}, Genres: []string{"Comedy"}},
		{Name: "Radio Star", Year: 2003, IDs: map[string]string{"tmdb": "920003"}, Genres: []string{"Drama"}},
	}}
	r.owner.call("POST", "/api/v1/hidden", `{"kind": "movie", "genre": "Horror"}`, 201)
	r.owner.call("POST", "/api/v1/hidden", `{"kind": "movie", "tmdb_id": "920002"}`, 201)
	var got []string
	for _, it := range suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=radio&kind=movie", "", 200)).Items {
		got = append(got, it.Name)
	}
	if !slices.Equal(got, []string{"Radio Star"}) {
		t.Fatalf("suggestions = %v, want only Radio Star", got)
	}
}

// route: GET /api/v1/explore/strip
// A hide reaches the Explore strip on the next request: the strip kept for
// the day is not reused, so no tile keeps a hidden title's picture.
func TestHiddenLeavesExploreStrip(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
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
	strip := func() []map[string]any {
		resp, err := http.Get(srv.URL + "/api/v1/explore/strip?type=shows")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Tiles   []map[string]any `json:"tiles"`
			Partial bool             `json:"partial"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != 200 || out.Partial {
			t.Fatalf("strip = %d %+v %v", resp.StatusCode, out, err)
		}
		return out.Tiles
	}
	from := ""
	for _, x := range strip() {
		if x["section"] != "genres" {
			from, _ = x["from"].(string)
			break
		}
	}
	kind, id, ok := strings.Cut(from, ":")
	if !ok {
		t.Fatalf("no section tile names its title: %q", from)
	}
	resp, err := http.Post(srv.URL+"/api/v1/hidden", "application/json", strings.NewReader(fmt.Sprintf(`{"kind": %q, "tmdb_id": %q}`, kind, id)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("hide = %d", resp.StatusCode)
	}
	for _, x := range strip() {
		if x["from"] == from {
			t.Fatalf("tile %v still shows the hidden %s", x["name"], from)
		}
	}
}

// route: GET /api/v1/shows/lookup
// Show add search: the owner finds a hidden show marked, a member does not
// see it by name, and its id (a link) still finds it.
func TestHiddenShowLookup(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	k.owner.call("POST", "/api/v1/hidden", `{"kind": "show", "tmdb_id": "4121", "name": "Harbor Friends"}`, 201)
	var found struct {
		Shows []struct {
			Name   string `json:"name"`
			Hidden bool   `json:"hidden"`
		} `json:"shows"`
	}
	_ = json.Unmarshal(k.owner.call("GET", "/api/v1/shows/lookup?term=any", "", 200), &found)
	marked := false
	for _, s := range found.Shows {
		marked = marked || (s.Name == "Harbor Friends" && s.Hidden)
	}
	if !marked {
		t.Fatalf("owner show lookup: %+v", found.Shows)
	}
	if got := namesIn(t, string(k.ana.call("GET", "/api/v1/shows/lookup?term=any", "", 200)), "shows"); slices.Contains(got, "Harbor Friends") {
		t.Fatalf("member show lookup shows the hidden show: %v", got)
	}
	if got := namesIn(t, string(k.ana.call("GET", "/api/v1/shows/lookup?term=tmdb:4121", "", 200)), "shows"); !slices.Contains(got, "Harbor Friends") {
		t.Fatalf("member lookup by id lost the show: %v", got)
	}
}
