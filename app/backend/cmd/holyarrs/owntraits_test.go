package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Your own traits: made, changed and removed here, judged at once,
// and taken out of every Pick rule when removed.
//
// route: GET /api/v1/own-traits
// route: POST /api/v1/own-traits
// route: PUT /api/v1/own-traits/{id}
// route: DELETE /api/v1/own-traits/{id}
func TestOwnTraitsMadeJudgedAndRemoved(t *testing.T) {
	srv, _, _ := lbLibSetup(t)
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })

	var tr downloads.OwnTraitView
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/own-traits", `{"name":" Favourite group ","match":"group","terms":["HOLYARRS",""," /^lantern$/ "]}`, 201), &tr)
	if tr.ID == 0 || tr.Key != mediafiles.OwnTraitKey(tr.ID) || tr.Name != "Favourite group" || !slices.Equal(tr.Terms, []string{"HOLYARRS", "/^lantern$/"}) {
		t.Fatalf("create %+v", tr)
	}
	lbDo(t, "POST", srv.URL+"/api/v1/own-traits", `{"name":"favourite GROUP","match":"name","terms":["x"]}`, 409)

	// A Pick rule gives it points and must-have; the judge sees it at once.
	b := mediafiles.GoodEveryday()
	b.Traits = append(b.Traits, mediafiles.TraitPoints{Trait: tr.Key, Points: 300})
	b.MustHave = []string{tr.Key}
	var p lbPickRulesDetail
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/pick-rules", lbRuleJSON(t, "Mine", []string{"movie"}, b), 201), &p)
	name := "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-HOLYARRS"
	if got := mediafiles.CurrentOf(name, 0, time.Time{}, p.Body).Points; got < 300 {
		t.Fatalf("points with the trait = %d, want 300 or more", got)
	}

	// A change of terms is judged at once too.
	path := fmt.Sprintf("%s/api/v1/own-traits/%d", srv.URL, tr.ID)
	_ = json.Unmarshal(lbDo(t, "PUT", path, `{"name":"Favourite group","match":"group","terms":["LANTERN"]}`, 200), &tr)
	if got := mediafiles.CurrentOf(name, 0, time.Time{}, p.Body).Points; got >= 300 {
		t.Fatalf("points after the change = %d, want under 300", got)
	}

	var list struct {
		OwnTraits []downloads.OwnTraitView `json:"own_traits"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/own-traits", "", 200), &list)
	if len(list.OwnTraits) != 1 || list.OwnTraits[0].Terms[0] != "LANTERN" {
		t.Fatalf("list %+v", list)
	}

	// Removing it takes it out of the Pick rule (points and must-have).
	var del struct {
		Changed int `json:"pick_rules_changed"`
	}
	_ = json.Unmarshal(lbDo(t, "DELETE", path, "", 200), &del)
	if del.Changed != 1 {
		t.Fatalf("pick rules changed = %d, want 1", del.Changed)
	}
	var after lbPickRulesDetail
	_ = json.Unmarshal(lbDo(t, "GET", fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID), "", 200), &after)
	if len(after.Body.MustHave) != 0 || slices.ContainsFunc(after.Body.Traits, func(x mediafiles.TraitPoints) bool { return x.Trait == tr.Key }) || after.Revision != 2 {
		t.Fatalf("pick rules after delete %+v", after)
	}
	lbDo(t, "DELETE", path, "", 404)
	lbDo(t, "PUT", path, `{"name":"x","match":"name","terms":["x"]}`, 404)
	lbDo(t, "DELETE", srv.URL+"/api/v1/own-traits/abc", "", 400)

	// Pick rules cannot name an own trait that does not exist; the error
	// names the list that holds it.
	var bad struct {
		Field string `json:"field"`
	}
	_ = json.Unmarshal(lbDo(t, "PUT", fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID), lbRuleJSON(t, "Mine", []string{"movie"}, b), 400), &bad)
	if bad.Field != "body.traits" {
		t.Fatalf("field %q, want body.traits", bad.Field)
	}
	b.Traits = b.Traits[:len(b.Traits)-1]
	_ = json.Unmarshal(lbDo(t, "PUT", fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID), lbRuleJSON(t, "Mine", []string{"movie"}, b), 400), &bad)
	if bad.Field != "body.must_have" {
		t.Fatalf("field %q, want body.must_have", bad.Field)
	}

	// A removed trait's id is never given again, so an old key cannot
	// come back as another trait.
	var next downloads.OwnTraitView
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/own-traits", `{"name":"Next","match":"name","terms":["x"]}`, 201), &next)
	if next.ID == tr.ID {
		t.Fatalf("id %d given again", next.ID)
	}
}

func TestOwnTraitFieldErrors(t *testing.T) {
	srv, _, _ := lbLibSetup(t)
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	for _, c := range []struct{ body, field string }{
		{`{"name":"","match":"name","terms":["x"]}`, "name"},
		{`{"name":"A","match":"title","terms":["x"]}`, "match"},
		{`{"name":"A","match":"name","terms":[" "]}`, "terms"},
		{`{"name":"A","match":"name","terms":["/(open/"]}`, "terms"},
	} {
		var got struct {
			Field string `json:"field"`
		}
		_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/own-traits", c.body, 400), &got)
		if got.Field != c.field {
			t.Errorf("%s: field %q, want %q", c.body, got.Field, c.field)
		}
	}
}
