package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
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
