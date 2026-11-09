package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// route: GET /api/v1/library-folders
// route: POST /api/v1/library-folders
// route: PUT /api/v1/library-folders/{id}
// route: DELETE /api/v1/library-folders/{id}
// route: POST /api/v1/library-folders/{id}/check
func TestLibraryFoldersAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	dir := t.TempDir()
	do(t, "POST", srv.URL+"/api/v1/library-folders", `{"path":"movies","kind":"movie"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, filepath.Join(dir, "nope")), 400)
	var lf library.FolderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies"}`, dir), 201), &lf)
	if lf.ID == 0 || lf.Health != "ok" || lf.Label != "Movies" {
		t.Fatalf("folder %+v", lf)
	}
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, dir), 409)
	b := string(do(t, "PUT", srv.URL+"/api/v1/library-folders/"+itoa(lf.ID), `{"label":"Films"}`, 200))
	if !strings.Contains(b, `"label":"Films"`) {
		t.Fatalf("edit %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/library-folders", "", 200))
	if !strings.Contains(b, `"Films"`) {
		t.Fatalf("list %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	do(t, "DELETE", srv.URL+"/api/v1/library-folders/"+itoa(lf.ID), "", 409)
	_ = os.Remove(dir) // fails while not empty; make it gone instead
	_ = os.RemoveAll(dir)
	b = string(do(t, "POST", srv.URL+"/api/v1/library-folders/"+itoa(lf.ID)+"/check", "", 200))
	if !strings.Contains(b, `"health":"missing"`) {
		t.Fatalf("check %s", b)
	}
	do(t, "DELETE", srv.URL+"/api/v1/library-folders/999", "", 404)
}

// route: POST /api/v1/titles
// route: GET /api/v1/never-add
// route: POST /api/v1/never-add
// route: DELETE /api/v1/never-add/{id}
func TestAddTitleAPI(t *testing.T) {
	t.Parallel()
	srv, s, searched := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 409) // no folder yet
	dir := t.TempDir()
	var lf library.FolderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, dir), 201), &lf)

	// Add with search on add: refresh, then scan, then search, in that order.
	b := do(t, "POST", srv.URL+"/api/v1/titles", fmt.Sprintf(`{"id":"tmdb:693134","library_folder_id":%d,"search_on_add":true}`, lf.ID), 201)
	var added struct {
		Title TitleView `json:"title"`
		JobID int64     `json:"job_id"`
	}
	_ = json.Unmarshal(b, &added)
	if added.Title.ReleaseGate != "released" || added.Title.GateDriver != "digital" || added.Title.Slots[0].State != "wanted" || added.JobID == 0 {
		t.Fatalf("added %s", b)
	}
	select {
	case id := <-searched:
		if id != added.Title.Slots[0].ID {
			t.Fatalf("searched %d", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no search after add")
	}
	js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "1=1 ORDER BY id")
	if len(js) != 3 || js[0].Kind != "refresh_title" || js[1].Kind != "scan_title" || js[2].Kind != "search_slot" {
		t.Fatalf("jobs %+v", js)
	}

	// Duplicate refused with the existing Title.
	b = do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"imdb:tt15239678"}`, 409)
	if !strings.Contains(string(b), fmt.Sprintf(`"title_id":%d`, added.Title.ID)) {
		t.Fatalf("duplicate %s", b)
	}
	// Never-add refused.
	do(t, "POST", srv.URL+"/api/v1/never-add", `{"tmdb_id":"900001","name":"The Lantern Keepers","reason":"not for us"}`, 201)
	b = do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001"}`, 409)
	if !strings.Contains(string(b), "Never-add") {
		t.Fatalf("never-add %s", b)
	}
	var na struct {
		NeverAdd []library.NeverAddView `json:"never_add"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/never-add", "", 200), &na)
	do(t, "DELETE", srv.URL+"/api/v1/never-add/"+itoa(na.NeverAdd[0].ID), "", 204)
	// Bad choices refused.
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001","release_gate":"soon"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001","pick_rules_id":999}`, 400)
	// An unhealthy folder is refused with the reason.
	_ = os.RemoveAll(dir)
	b = do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001"}`, 409)
	if !strings.Contains(string(b), `"health":"missing"`) || !strings.Contains(string(b), `"reason":"the folder does not exist`) {
		t.Fatalf("unhealthy %s", b)
	}
}

// route: GET /api/v1/titles/{id}
// route: PUT /api/v1/titles/{id}
// route: POST /api/v1/titles/{id}/move
func TestEditTitleAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, t.TempDir()), 201)
	var added struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001"}`, 201), &added)
	id := itoa(added.Title.ID)
	if added.Title.Slots[0].State != "not_out_yet" || !strings.Contains(added.Title.Slots[0].Reason, "2031-05-02") {
		t.Fatalf("future movie %+v", added.Title)
	}
	b := string(do(t, "PUT", srv.URL+"/api/v1/titles/"+id+"?dry_run=1", `{"release_gate":"announced"}`, 200))
	if !strings.Contains(b, `"dry_run":true`) || !strings.Contains(b, `"to":"wanted"`) {
		t.Fatalf("dry run %s", b)
	}
	var got TitleView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/"+id, "", 200), &got)
	if got.ReleaseGate != "released" || got.Slots[0].State != "not_out_yet" {
		t.Fatalf("dry run saved %+v", got)
	}
	b = string(do(t, "PUT", srv.URL+"/api/v1/titles/"+id, `{"release_gate":"announced","keep_up_to_date":false}`, 200))
	if !strings.Contains(b, `"release_gate":"announced"`) || !strings.Contains(b, `"state":"wanted"`) || !strings.Contains(b, `"keep_up_to_date":false`) {
		t.Fatalf("edit %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/titles/"+id, `{"gate_delay_days":-1}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/titles/999", `{}`, 404)
	do(t, "POST", srv.URL+"/api/v1/titles/"+id+"/move", `{"library_folder_id":999}`, 409)
}

// route: GET /api/v1/titles/lookup
func TestLookupAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, t.TempDir()), 201)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	for term, want := range map[string]string{
		"dune":            `"title_id":1`,
		"tmdb:693134":     `"name":"Dune: Part Two"`,
		"IMDB:tt15239678": `"title_id":1`,
		"tmdb:12345":      `"movies":[]`,
		"lantern":         `"never_add":false`,
	} {
		b := string(do(t, "GET", srv.URL+"/api/v1/titles/lookup?term="+strings.ReplaceAll(term, " ", "+"), "", 200))
		if !strings.Contains(b, want) {
			t.Errorf("%s: %s", term, b)
		}
	}
	do(t, "GET", srv.URL+"/api/v1/titles/lookup", "", 400)
}

// route: GET /api/v1/settings/release-gate
// route: PUT /api/v1/settings/release-gate
func TestReleaseGateSettingAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	b := string(do(t, "GET", srv.URL+"/api/v1/settings/release-gate", "", 200))
	if !strings.Contains(b, `"release_gate":"released"`) || !strings.Contains(b, `"gate_delay_days":0`) || !strings.Contains(b, `"bulk_search_respects_gate":true`) {
		t.Fatalf("defaults %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/release-gate", `{"release_gate":"in_cinemas","gate_delay_days":1,"bulk_search_respects_gate":true}`, 200)
	do(t, "PUT", srv.URL+"/api/v1/settings/release-gate", `{"release_gate":"later"}`, 400)
	b = string(do(t, "GET", srv.URL+"/api/v1/settings/release-gate", "", 200))
	if !strings.Contains(b, `"in_cinemas"`) {
		t.Fatalf("after change %s", b)
	}
}

// route: POST /api/v1/titles/{id}/refresh
// route: POST /api/v1/titles/refresh
// route: GET /api/v1/calendar
// route: GET /api/v1/artwork/{name}
func TestRefreshAndCalendarAPI(t *testing.T) {
	t.Parallel()
	srv, s, _ := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, t.TempDir()), 201)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001"}`, 201)
	b := string(do(t, "POST", srv.URL+"/api/v1/titles/1/refresh", "", 202))
	if !strings.Contains(b, "job_ids") {
		t.Fatalf("refresh one %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/titles/99/refresh", "", 404)
	do(t, "POST", srv.URL+"/api/v1/titles/refresh", `{"title_ids":[1,2]}`, 202)
	do(t, "POST", srv.URL+"/api/v1/titles/refresh", `{"title_ids":[77]}`, 404)
	var all struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles/refresh", "", 202), &all)
	if len(all.JobIDs) != 3 {
		t.Fatalf("refresh all queues the movie, show and artist refresh: %v", all.JobIDs)
	}
	if js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "kind = 'refresh_shows'"); len(js) != 1 {
		t.Fatalf("refresh all should queue one refresh_shows Job: %+v", js)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "kind = 'refresh_titles'")
		if len(js) == 1 && js[0].State == "done" {
			if js[0].Result["refreshed"] != float64(2) {
				t.Fatalf("refresh all %+v", js[0].Result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("refresh all never finished: %+v", js)
		}
		time.Sleep(10 * time.Millisecond)
	}

	b = string(do(t, "GET", srv.URL+"/api/v1/calendar?from=2024-03-01&to=2024-05-01", "", 200))
	if !strings.Contains(b, `"kind":"cinema"`) || !strings.Contains(b, `"kind":"digital"`) || strings.Contains(b, "Lantern") {
		t.Fatalf("calendar %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/calendar?from=2031-02-01", "", 200))
	if !strings.Contains(b, "The Lantern Keepers") {
		t.Fatalf("calendar default range %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/calendar?from=2024-13-01", "", 400)
	do(t, "GET", srv.URL+"/api/v1/calendar?from=2024-05-01&to=2024-03-01", "", 400)
	do(t, "GET", srv.URL+"/api/v1/artwork/nothing.jpg", "", 404)
}

// route: GET /api/v1/pick-rules
func TestPickRulesListAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	var got struct {
		PickRules []downloads.PickRulesView `json:"pick_rules"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/pick-rules", "", 200), &got)
	if len(got.PickRules) == 0 || got.PickRules[0].ID == 0 || got.PickRules[0].Name == "" {
		t.Fatalf("pick rules %+v", got)
	}
}

func TestTitleViewGateAndVersion(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	var lf library.FolderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies"}`, t.TempDir()), 201), &lf)
	var future, out struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:900001"}`, 201), &future)
	v := future.Title
	if v.GateOpen || v.Stage != "announced" || !strings.Contains(v.GateReason, "2031-05-02") {
		t.Fatalf("future gate %+v", v)
	}
	if v.LibraryFolderID != lf.ID || v.FolderPath == "" || v.PickRulesID == 0 || v.PickRulesName == "" {
		t.Fatalf("version fields %+v", v)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134","keep_up_to_date":false}`, 201), &out)
	if !out.Title.GateOpen || out.Title.Stage != "released" || !strings.Contains(out.Title.GateReason, "Keep up to date is off") {
		t.Fatalf("released gate %+v", out.Title)
	}
	do(t, "PUT", srv.URL+"/api/v1/titles/"+itoa(out.Title.ID), `{"keep_up_to_date":true}`, 200)
	var got TitleView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(out.Title.ID), "", 200), &got)
	if !strings.Contains(got.GateReason, "open") {
		t.Fatalf("open gate reason %q", got.GateReason)
	}
}
