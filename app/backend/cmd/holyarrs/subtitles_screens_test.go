package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// route: POST /api/v1/subtitle-sites/{id}/test
// A site that needs your account is refused until its details are given;
// the details go to the vault, never back out, and the test says in plain
// words whether the site took them.
func TestSubtitleSiteAccountAndTest(t *testing.T) {
	t.Parallel()
	srv, s := dlServer(t)
	u := srv.URL + "/api/v1"
	site := fakesite.Lab()
	site.Key = "xxxx-xxxx-xxxx"
	fake := httptest.NewServer(site)
	t.Cleanup(fake.Close)
	limits := `"allow_local_network": true, "limits": {"gap_ms": 1, "per_window": 100, "window_seconds": 3600}`

	var kinds struct {
		Kinds []struct {
			Kind         string   `json:"kind"`
			NeedsAccount bool     `json:"needs_account"`
			Account      []string `json:"account"`
		} `json:"kinds"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitle-sites/kinds", "", 200), &kinds)
	last := kinds.Kinds[len(kinds.Kinds)-1]
	if last.Kind != "fake_account" || !last.NeedsAccount || len(last.Account) != 1 || last.Account[0] != "api_key" {
		t.Fatalf("%+v", kinds)
	}
	// Refused without the account, and a site without accounts takes none.
	b := do(t, "POST", u+"/subtitle-sites", `{"kind": "fake_account", "url": "`+fake.URL+`", `+limits+`}`, 409)
	if !strings.Contains(string(b), "needs your account") {
		t.Fatalf("%s", b)
	}
	do(t, "POST", u+"/subtitle-sites", `{"kind": "fake", "url": "`+fake.URL+`", "api_key": "x", `+limits+`}`, 400)

	// A wrong key is saved, and the test says the site refused it.
	var row struct {
		ID         int64 `json:"id"`
		HasAccount bool  `json:"has_account"`
	}
	b = do(t, "POST", u+"/subtitle-sites", `{"kind": "fake_account", "url": "`+fake.URL+`", "api_key": "wrong", `+limits+`}`, 201)
	decodeJSON(t, b, &row)
	if !row.HasAccount || strings.Contains(string(b), "wrong") {
		t.Fatalf("%s", b)
	}
	var res struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	decodeJSON(t, do(t, "POST", u+"/subtitle-sites/"+itoa(row.ID)+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Message, "refused the account") {
		t.Fatalf("%+v", res)
	}
	// The test never pauses the site.
	var state string
	if err := s.Read().QueryRow("SELECT state FROM subtitle_sites WHERE id = ?", row.ID).Scan(&state); err != nil || state != "enabled" {
		t.Fatalf("state %q %v", state, err)
	}
	// A change without details keeps the stored ones; new details replace them.
	do(t, "PUT", u+"/subtitle-sites/"+itoa(row.ID), `{"name": "Lab account", "url": "`+fake.URL+`", `+limits+`}`, 200)
	decodeJSON(t, do(t, "POST", u+"/subtitle-sites/"+itoa(row.ID)+"/test", "", 200), &res)
	if res.OK {
		t.Fatalf("the old key changed: %+v", res)
	}
	do(t, "PUT", u+"/subtitle-sites/"+itoa(row.ID), `{"name": "Lab account", "url": "`+fake.URL+`", "api_key": "xxxx-xxxx-xxxx", `+limits+`}`, 200)
	decodeJSON(t, do(t, "POST", u+"/subtitle-sites/"+itoa(row.ID)+"/test", "", 200), &res)
	if !res.OK || !strings.Contains(res.Message, "answered") {
		t.Fatalf("%+v", res)
	}
	// The key is sealed: not in the sites table or the list.
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = 'subtitle_site_account'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("secrets %d %v", n, err)
	}
	if l := do(t, "GET", u+"/subtitle-sites", "", 200); strings.Contains(string(l), "xxxx-xxxx-xxxx") {
		t.Fatalf("key leaked: %s", l)
	}
	// Moving to another address needs the details again.
	do(t, "PUT", u+"/subtitle-sites/"+itoa(row.ID), `{"name": "Lab account", "url": "http://10.9.9.9:1", `+limits+`}`, 400)
	do(t, "POST", u+"/subtitle-sites/9999/test", "", 404)
	// Deleting the site removes its sealed details.
	do(t, "DELETE", u+"/subtitle-sites/"+itoa(row.ID), "", 204)
	if err := s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = 'subtitle_site_account'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("secrets after delete %d %v", n, err)
	}
}

// The missing list narrows to one movie or show and names them for the
// filter; Search all says which Jobs to follow; sync defaults are settings.
func TestSubtitleMissingFiltersAndJobs(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	u := srv.URL + "/api/v1"
	titleID, fileID, _ := subtitleMovie(t, s)
	fake := httptest.NewServer(fakesite.Lab())
	t.Cleanup(fake.Close)
	do(t, "POST", u+"/subtitle-sites", `{"kind": "fake", "url": "`+fake.URL+`", "allow_local_network": true, "limits": {"gap_ms": 1, "per_window": 100, "window_seconds": 3600}}`, 201)
	do(t, "PUT", u+"/titles/"+itoa(titleID)+"/subtitle-wish", `{"languages": [{"language": "de"}]}`, 200)
	do(t, "POST", u+"/files/"+itoa(fileID)+"/subtitles/choices", "", 200) // looks: German is wanted

	var miss struct {
		Missing []struct {
			FileID  int64 `json:"file_id"`
			ItemID  int64 `json:"item_id"`
			TitleID int64 `json:"title_id"`
		} `json:"missing"`
		Total  int `json:"total"`
		Titles []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"titles"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitles/missing?title="+itoa(titleID), "", 200), &miss)
	if miss.Total != 1 || miss.Missing[0].FileID != fileID || miss.Missing[0].ItemID == 0 || len(miss.Titles) != 1 || miss.Titles[0].Name != "Big Buck Bunny" {
		t.Fatalf("%+v", miss)
	}
	decodeJSON(t, do(t, "GET", u+"/subtitles/missing?title=9999", "", 200), &miss)
	if miss.Total != 0 {
		t.Fatalf("%+v", miss)
	}
	do(t, "GET", u+"/subtitles/missing?title=x", "", 400)
	var q struct {
		Queued int     `json:"queued"`
		JobIDs []int64 `json:"job_ids"`
	}
	decodeJSON(t, do(t, "POST", u+"/subtitles/missing/search", `{"title_id": `+itoa(titleID)+`}`, 202), &q)
	if q.Queued != 1 || len(q.JobIDs) != 1 {
		t.Fatalf("%+v", q)
	}
	var kind string
	if err := s.Read().QueryRowContext(context.Background(), "SELECT kind FROM jobs WHERE id = ?", q.JobIDs[0]).Scan(&kind); err != nil || kind != "subtitle_search" {
		t.Fatalf("job %q %v", kind, err)
	}
	decodeJSON(t, do(t, "POST", u+"/subtitles/search", `{"file_id": `+itoa(fileID)+`}`, 202), &q)
	if len(q.JobIDs) != 1 {
		t.Fatalf("%+v", q)
	}

	// Sync defaults.
	var set map[string]any
	decodeJSON(t, do(t, "GET", u+"/settings/subtitles", "", 200), &set)
	if set["sync_from_fps"] != 25.0 || set["sync_to_fps"] != 23.976 || set["sync_step_ms"] != 500.0 {
		t.Fatalf("%v", set)
	}
	do(t, "PUT", u+"/settings/subtitles", `{"sync_to_fps": 200}`, 400)
	do(t, "PUT", u+"/settings/subtitles", `{"sync_step_ms": 5}`, 400)
	decodeJSON(t, do(t, "PUT", u+"/settings/subtitles", `{"sync_from_fps": 23.976, "sync_to_fps": 25, "sync_step_ms": 100}`, 200), &set)
	if set["sync_from_fps"] != 23.976 || set["sync_to_fps"] != 25.0 || set["sync_step_ms"] != 100.0 {
		t.Fatalf("%v", set)
	}
}
