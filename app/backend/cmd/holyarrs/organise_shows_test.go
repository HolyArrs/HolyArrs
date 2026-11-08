package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// showOrganiseSetup is a server with running Jobs, the fake show sources
// and a show Library folder holding the lab show with three episodes.
func showOrganiseSetup(t *testing.T) (url string, s *database.Store, lib string, folderID int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tmdb := &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": apiShow("tmdb", "100"), "tvdb:200": apiShow("tmdb", "100")}}
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: &metadata.Static{}, Shows: metadata.ShowSources{"tmdb": tmdb}}
	svc.Register(q)
	q.Handle(library.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Queue: q, Library: svc, Metadata: &metadata.Static{}}), config{}))
	t.Cleanup(srv.Close)
	lib = t.TempDir()
	for _, rel := range []string{"Lab Show (2025)/Season 1/Lab.Show.S01E01.1080p.WEB-DL-GRP.mkv", "Lab Show (2025)/Season 1/Lab.Show.S01E02.1080p.WEB-DL-GRP.mkv",
		"Lab Show (2025)/Specials/Lab.Show.S00E01.720p.HDTV-GRP.mkv", "Lab Show (2025)/Extras/interview.mkv"} {
		p := filepath.Join(lib, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("video "+rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var lf struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show","label":"Shows"}`, lib), 201), &lf)
	return srv.URL, s, lib, lf.ID
}

// adoptLabShow scans the show folder and adopts the lab show; it returns
// the new show's id.
func adoptLabShow(t *testing.T, url string, folderID int64) int64 {
	t.Helper()
	var q struct {
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(do(t, "POST", url+"/api/v1/library-folders/"+itoa(folderID)+"/adopt",
		`{"folders":[{"folder":"Lab Show (2025)","id":"tmdb:100"}]}`, 202), &q)
	j := waitJob(t, url, q.JobID)
	res, _ := j.Result["results"].([]any)
	if j.State != "done" || j.Result["adopted"] != float64(1) || len(res) != 1 {
		t.Fatalf("adopt %+v", j)
	}
	r, _ := res[0].(map[string]any)
	return int64(r["title_id"].(float64))
}

// 5.2.2: a show Library folder is scanned (a dry run listing each file's
// episodes) and adopted with no file moved.
func TestScanAndAdoptShowsAPI(t *testing.T) {
	t.Parallel()
	url, s, lib, folderID := showOrganiseSetup(t)
	var q struct {
		JobID  int64 `json:"job_id"`
		DryRun bool  `json:"dry_run"`
	}
	_ = json.Unmarshal(do(t, "POST", url+"/api/v1/library-folders/"+itoa(folderID)+"/scan", "", 202), &q)
	j := waitJob(t, url, q.JobID)
	b, _ := json.Marshal(j.Result)
	for _, want := range []string{`"kind":"show"`, `"status":"matched"`, `"id":"tmdb:100"`, `"episodes":["S01E01"]`, `"episodes":["S00E01"]`, `"unknown":1`} {
		if j.State != "done" || !q.DryRun || !strings.Contains(string(b), want) {
			t.Fatalf("scan lacks %s: %s %s", want, j.State, b)
		}
	}
	if n := count41(t, s, "titles"); n != 0 {
		t.Fatalf("the scan added %d titles", n)
	}
	do(t, "POST", url+"/api/v1/library-folders/"+itoa(folderID)+"/adopt", `{"folders":[{"folder":"Lab Show (2025)"}]}`, 400)
	id := adoptLabShow(t, url, folderID)
	var eps struct {
		Episodes []EpisodeView `json:"episodes"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/titles/"+itoa(id)+"/episodes", "", 200), &eps)
	in := 0
	for _, e := range eps.Episodes {
		for _, sl := range e.Slots {
			if sl.State == "in_library" || sl.State == "better_wanted" {
				in++
			}
		}
	}
	if in != 3 {
		t.Fatalf("episodes in the library %d: %+v", in, eps)
	}
	for _, rel := range []string{"Lab Show (2025)/Season 1/Lab.Show.S01E01.1080p.WEB-DL-GRP.mkv", "Lab Show (2025)/Extras/interview.mkv"} {
		if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s moved", rel)
		}
	}
}

func count41(t *testing.T, s *database.Store, table string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// 5.2.3: POST /titles/{id}/delete takes a season or episode_ids for part
// of a show; the dry run is what the delete does.
func TestDeleteShowPartsAPI(t *testing.T) {
	t.Parallel()
	url, _, lib, folderID := showOrganiseSetup(t)
	id := adoptLabShow(t, url, folderID)
	u := url + "/api/v1/titles/" + itoa(id) + "/delete"
	do(t, "POST", u, `{"season":1,"episode_ids":[1]}`, 400)
	do(t, "POST", u, `{"season":9}`, 400)
	do(t, "POST", u, `{"season":1,"never_add":true}`, 400)
	var dry, done library.RemovePlan
	_ = json.Unmarshal(do(t, "POST", u, `{"season":1,"delete_files":true}`, 200), &dry)
	if dry.Applied || dry.Scope != "season" || len(dry.Held) != 2 || len(dry.Episodes) != 2 {
		t.Fatalf("dry %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(lib, "Lab Show (2025)", "Season 1", "Lab.Show.S01E01.1080p.WEB-DL-GRP.mkv")); err != nil {
		t.Fatal("the dry run moved a file")
	}
	_ = json.Unmarshal(do(t, "POST", u, `{"season":1,"delete_files":true,"apply":true}`, 200), &done)
	if !done.Applied || len(done.Held) != 2 || done.Held[0].To != dry.Held[0].To {
		t.Fatalf("done %+v", done)
	}
	if _, err := os.Stat(filepath.Join(lib, "Lab Show (2025)", "Specials", "Lab.Show.S00E01.720p.HDTV-GRP.mkv")); err != nil {
		t.Fatal("a special moved")
	}
	// The whole show, kept files, on Never-add.
	_ = json.Unmarshal(do(t, "POST", u, `{"never_add":true,"apply":true}`, 200), &done)
	if done.Scope != "title" || !done.Applied {
		t.Fatalf("show %+v", done)
	}
	do(t, "POST", u, `{"season":0}`, 409)
}

// route: GET /api/v1/settings/alerts
// route: PUT /api/v1/settings/alerts
// 5.2.5: one Alert per pack import by default, or one per episode.
func TestAlertSettingsAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _ := showOrganiseSetup(t)
	u := url + "/api/v1/settings/alerts"
	if b := string(do(t, "GET", u, "", 200)); !strings.Contains(b, `"pack_alerts":"one"`) || !strings.Contains(b, `"each_limit":25`) {
		t.Fatalf("get %s", b)
	}
	do(t, "PUT", u, `{"pack_alerts":"some"}`, 400)
	do(t, "PUT", u, `{"pack_alerts":"each"}`, 200)
	if b := string(do(t, "PUT", u, `{}`, 200)); !strings.Contains(b, `"pack_alerts":"each"`) {
		t.Fatalf("left out %s", b)
	}
	do(t, "PUT", u, `{"each_limit":501}`, 400)
	if b := string(do(t, "PUT", u, `{"each_limit":10}`, 200)); !strings.Contains(b, `"each_limit":10`) || !strings.Contains(b, `"pack_alerts":"each"`) {
		t.Fatalf("limit %s", b)
	}
}

// 5.2.6: History of one episode: the Events of its Slots (its own, or a
// pack's slot_ids) and those naming it; each show row names its episodes.
func TestEpisodeHistoryAPI(t *testing.T) {
	t.Parallel()
	url, s, _, folderID := showOrganiseSetup(t)
	id := adoptLabShow(t, url, folderID)
	ctx := context.Background()
	slot := map[string]int64{}
	item := map[string]int64{}
	rows, err := s.Read().Query(`SELECT s.id, i.id, json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode') FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sid, iid int64
		var se, ep int
		_ = rows.Scan(&sid, &iid, &se, &ep)
		k := fmt.Sprintf("S%02dE%02d", se, ep)
		slot[k], item[k] = sid, iid
	}
	rows.Close()
	add := func(kind string, slotID *int64, data map[string]any) {
		if err := s.AddEvent(ctx, &domain.Event{Kind: kind, TitleID: &id, SlotID: slotID, Summary: kind, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	s1, s2 := slot["S01E01"], slot["S01E02"]
	add("download_sent", &s1, map[string]any{"slot_ids": []int64{s1, s2}})
	add("import_done", &s1, map[string]any{"slot_ids": []int64{s1, s2}, "placed_slot_ids": []int64{s1}})
	add("needs_decision", &s2, nil)
	add("season_changed", nil, map[string]any{"item_id": item["S01E02"]})
	add("episodes_removed", nil, map[string]any{"item_ids": []int64{item["S02E01"]}})
	var page struct {
		Events  []shows.EventView `json:"events"`
		Total   int               `json:"total"`
		Episode string            `json:"episode"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/events?episode="+itoa(item["S01E02"]), "", 200), &page)
	kinds := map[string]string{}
	for _, e := range page.Events {
		kinds[e.Kind] = e.Episode
	}
	if page.Episode != "S01E02 Second" || page.Total != 4 || kinds["download_sent"] != "S01E01-E02, 2 episodes" || kinds["import_done"] != "S01E01 Pilot" ||
		kinds["needs_decision"] != "S01E02 Second" || kinds["season_changed"] != "S01E02 Second" {
		t.Fatalf("episode history %d %v", page.Total, kinds)
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/events?title="+itoa(id)+"&kind=episodes_removed", "", 200), &page)
	if page.Total != 1 || page.Events[0].Episode != "S02E01 Return" {
		t.Fatalf("delete row %+v", page.Events)
	}
	do(t, "GET", url+"/api/v1/events?episode=x", "", 400)
	do(t, "GET", url+"/api/v1/events?episode=99999", "", 400)
}

// 5.2.7: the calendar lists episodes with movies, or only one of them.
func TestCalendarShowsAPI(t *testing.T) {
	t.Parallel()
	url, _, _, folderID := showOrganiseSetup(t)
	adoptLabShow(t, url, folderID)
	var c struct {
		Titles string                  `json:"titles"`
		Dates  []library.CalendarEntry `json:"dates"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/calendar?from=2025-01-01&to=2025-02-01", "", 200), &c)
	if c.Titles != "all" || len(c.Dates) != 2 || c.Dates[0].Kind != "episode" || c.Dates[0].EpisodeName != "Pilot" || !c.Dates[0].Timed ||
		(c.Dates[0].State != "in_library" && c.Dates[0].State != "better_wanted") {
		t.Fatalf("calendar %+v", c)
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/calendar?from=2025-01-01&to=2025-02-01&titles=movies", "", 200), &c)
	if len(c.Dates) != 0 {
		t.Fatalf("movies only %+v", c.Dates)
	}
	do(t, "GET", url+"/api/v1/calendar?titles=films", "", 400)
}
