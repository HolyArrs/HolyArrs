package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// newShowAskRig is the ask rig with a show Library folder and a fake TMDB
// show source that knows the lab show by its TMDB and TVDB ids.
func newShowAskRig(t *testing.T) *askRig {
	t.Helper()
	r := newAskRig(t)
	sh := apiShow("tmdb", "100")
	r.asks.Library.Shows = metadata.ShowSources{"tmdb": &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": sh, "tvdb:200": sh}}}
	r.owner.call("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show","label":"Shows"}`, t.TempDir()), 201)
	return r
}

// route: POST /api/v1/asks
// route: GET /api/v1/asks
// route: GET /api/v1/shows/lookup
// route: PUT /api/v1/settings/asks
func TestShowAskOverHTTP(t *testing.T) {
	t.Parallel()
	r := newShowAskRig(t)

	// Bad show asks are refused with the reason.
	r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:100","seasons":[9]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"netflix:1"}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:1","seasons":[1]}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:404"}`, 404)

	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:100","seasons":[1]}`, 201))
	if a.Kind != "show" || a.State != "asked" || a.SeasonWords != "season 1" || a.Ref != "tmdb:100" || !a.Mine {
		t.Fatalf("ask %+v", a)
	}
	// bo asks by the TVDB id: he joins.
	var joined struct {
		Ask    requests.View `json:"ask"`
		Joined bool          `json:"joined"`
	}
	_ = json.Unmarshal(r.bo.call("POST", "/api/v1/asks", `{"kind":"show","id":"tvdb:200","seasons":[1]}`, 200), &joined)
	// A member sees everyone who asked.
	if !joined.Joined || joined.Ask.ID != a.ID || len(joined.Ask.People) != 2 {
		t.Fatalf("join %+v", joined)
	}
	// The show search tells ana she asked already.
	var look struct {
		Shows []showResult `json:"shows"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/shows/lookup?term=tmdb:100", "", 200), &look)
	if len(look.Shows) != 1 || look.Shows[0].Ask == nil || !look.Shows[0].Ask.Mine || look.Shows[0].Ask.ID != a.ID || len(look.Shows[0].Ask.Seasons) != 1 {
		t.Fatalf("lookup %+v", look)
	}
	// Staff see it under kind=show; movies only lists none.
	var list struct {
		Asks []requests.View `json:"asks"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/asks?kind=show", "", 200), &list)
	if len(list.Asks) != 1 {
		t.Fatalf("show asks %+v", list)
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/asks?kind=movie", "", 200), &list)
	if len(list.Asks) != 0 {
		t.Fatalf("movie asks %+v", list)
	}
	r.owner.call("GET", "/api/v1/asks?kind=album", "", 400)

	// A member may not approve; the owner approves and the show is added.
	r.ana.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", a.ID), `{}`, 403)
	got := askOf(t, r.owner.call("POST", fmt.Sprintf("/api/v1/asks/%d/approve", a.ID), `{"note":"season 1 coming"}`, 200))
	if got.State != "approved" || got.TitleID == nil || got.DecidedBy == nil || got.DecidedBy.Name != "luis" {
		t.Fatalf("approved %+v", got)
	}
	var title struct {
		Kind string          `json:"kind"`
		Asks []requests.View `json:"asks"`
	}
	_ = json.Unmarshal(r.owner.call("GET", fmt.Sprintf("/api/v1/titles/%d", *got.TitleID), "", 200), &title)
	if title.Kind != "show" || len(title.Asks) != 1 || len(title.Asks[0].People) != 2 {
		t.Fatalf("title %+v", title)
	}
	// History names who asked and who approved.
	b := string(r.owner.call("GET", "/api/v1/events?kind=ask_*", "", 200))
	for _, w := range []string{"ana asked for Lab Show (2025), season 1", "bo also asked for", "approved by luis"} {
		if !strings.Contains(b, w) {
			t.Fatalf("history lacks %q: %s", w, b)
		}
	}
	// bo's in-app message says it was approved.
	if m := string(r.bo.call("GET", "/api/v1/messages", "", 200)); !strings.Contains(m, "Lab Show (2025), season 1 was approved by an admin") {
		t.Fatalf("messages %s", m)
	}

	// Settings: a partial change keeps the rest.
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":3}`, 200)
	var set requests.Settings
	_ = json.Unmarshal(r.owner.call("PUT", "/api/v1/settings/asks", `{"shows_per_week":2}`, 200), &set)
	if set.MoviesPerWeek != 3 || set.ShowsPerWeek != 2 || set.MusicPerWeek != requests.DefaultPerWeek || !set.SearchOnApprove {
		t.Fatalf("settings %+v", set)
	}
	r.owner.call("PUT", "/api/v1/settings/asks", `{"show_library_folder_id":1}`, 400)
}

// route: POST /api/v1/asks
func TestShowAskFutureOnlyOverHTTP(t *testing.T) {
	t.Parallel()
	r := newShowAskRig(t)
	// Future seasons only takes no seasons list.
	r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:100","seasons":[1],"future_only":true}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:1","future_only":true}`, 400)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"kind":"show","id":"tmdb:100","future_only":true}`, 201))
	if !a.FutureOnly || a.SeasonWords != "future seasons only" {
		t.Fatalf("ask %+v", a)
	}
}
