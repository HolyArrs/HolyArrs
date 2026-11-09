package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// route: POST /api/v1/titles/{id}/versions
// route: PUT /api/v1/titles/{id}/versions/{vid}
// route: DELETE /api/v1/titles/{id}/versions/{vid}
func TestVersionsAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := lbLibSetup(t)
	var hd, uhd library.FolderView
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"HD"}`, t.TempDir()), 201), &hd)
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"4K"}`, t.TempDir()), 201), &uhd)
	var rules struct {
		PickRules []downloads.PickRulesView `json:"pick_rules"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/pick-rules", "", 200), &rules)
	var fourK, lossless int64
	for _, p := range rules.PickRules {
		if p.Name == "4K" {
			fourK = p.ID
		}
		if p.Name == "Lossless first" {
			lossless = p.ID
		}
	}
	if fourK == 0 {
		t.Fatalf("no 4K preset in %+v", rules)
	}

	// Add with a second Version.
	b := lbDo(t, "POST", srv.URL+"/api/v1/titles", fmt.Sprintf(`{"id":"tmdb:693134","library_folder_id":%d,"more_versions":[{"label":"4K","pick_rules_id":%d,"library_folder_id":%d}]}`, hd.ID, fourK, uhd.ID), 201)
	var added struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(b, &added)
	tv := added.Title
	if len(tv.Versions) != 2 || len(tv.Slots) != 2 || !tv.Versions[0].Main || tv.Versions[1].Label != "4K" ||
		tv.Versions[1].PickRulesName != "4K" || tv.Versions[1].LibraryFolderID != uhd.ID || tv.Slots[1].Version != "4K" ||
		tv.Slots[1].VersionID != tv.Versions[1].ID || !strings.HasPrefix(tv.Versions[1].FolderPath, uhd.Path) {
		t.Fatalf("added %s", b)
	}
	base := srv.URL + "/api/v1/titles/" + lbItoa(tv.ID) + "/versions"
	// Bad Versions on add are refused before anything is saved.
	lbDo(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001","more_versions":[{"label":""}]}`, 400)

	// A third Version in the same folder as the main one; a clash of labels is refused.
	b = lbDo(t, "POST", base, `{"label":"Remux"}`, 201)
	var v VersionView
	_ = json.Unmarshal(b, &v)
	if v.Label != "Remux" || v.Main || !strings.HasSuffix(v.FolderPath, "[Remux]") {
		t.Fatalf("third %s", b)
	}
	lbDo(t, "POST", base, `{"label":"remux"}`, 400)
	lbDo(t, "POST", base, `{"label":"x","pick_rules_id":999}`, 400)
	// Music Pick rules never fit a movie's Version (4.8.11 review).
	lbDo(t, "POST", base, fmt.Sprintf(`{"label":"x","pick_rules_id":%d}`, lossless), 400)
	lbDo(t, "PUT", base+"/"+lbItoa(v.ID), fmt.Sprintf(`{"pick_rules_id":%d}`, lossless), 400)
	lbDo(t, "POST", base, `nope`, 400)
	lbDo(t, "POST", srv.URL+"/api/v1/titles/999/versions", `{"label":"x"}`, 404)

	// Edit and remove.
	b = lbDo(t, "PUT", base+"/"+lbItoa(v.ID), `{"label":"Remux copy","keep_up_to_date":false}`, 200)
	if !strings.Contains(string(b), `"label":"Remux copy"`) || !strings.Contains(string(b), `"keep_up_to_date":false`) {
		t.Fatalf("edit %s", b)
	}
	lbDo(t, "PUT", base+"/999", `{"label":"y"}`, 404)
	lbDo(t, "DELETE", base+"/"+lbItoa(tv.Versions[0].ID), "", 409) // the main one stays
	lbDo(t, "DELETE", base+"/"+lbItoa(v.ID), "", 204)
	lbDo(t, "DELETE", base+"/"+lbItoa(v.ID), "", 404)
	var got TitleView
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/titles/"+lbItoa(tv.ID), "", 200), &got)
	if len(got.Versions) != 2 || len(got.Slots) != 2 {
		t.Fatalf("after remove %+v", got)
	}
}
