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
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// musicOrganiseSetup is a server with running Jobs, the fake MusicBrainz
// and a music Library folder holding Lab Band: First Light on the deluxe
// Pressing (three tracks), a live album the album kinds leave out, and a
// loose file.
func musicOrganiseSetup(t *testing.T) (url string, s *database.Store, lib string, folderID int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: &metadata.Static{}, Artists: apiMusic()}
	svc.Register(q)
	q.Handle(library.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Library: svc, Metadata: &metadata.Static{}}).Handler())
	t.Cleanup(srv.Close)
	lib = t.TempDir()
	for _, rel := range []string{"Lab Band/First Light (2020)/1 Song 1.flac", "Lab Band/First Light (2020)/2 Song 2.flac", "Lab Band/First Light (2020)/3 Song 3.flac",
		"Lab Band/Live Night (2021)/01 - Song 1.mp3", "Lab Band/cover.mp3"} {
		p := filepath.Join(lib, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("audio "+rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var lf struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, lib), 201), &lf)
	return srv.URL, s, lib, lf.ID
}

// route: POST /api/v1/library-folders/{id}/scan
// route: POST /api/v1/library-folders/{id}/adopt
// route: GET /api/v1/titles/{id}/rename
// route: POST /api/v1/titles/{id}/rename
// route: GET /api/v1/rename
// route: POST /api/v1/titles/{id}/delete
// route: GET /api/v1/events
// route: GET /api/v1/calendar
//
// 4.15: a music Library folder is scanned (artist, albums with the Pressing
// the files fit, files to tracks) and adopted with no file moved; its
// tracks are renamed by the music patterns, per album; a track and an
// album are deleted; the album's History and the Calendar list it.
func TestMusicOrganiseAPI(t *testing.T) {
	t.Parallel()
	url, s, lib, folderID := musicOrganiseSetup(t)
	u := url + "/api/v1"
	var q struct {
		JobID  int64 `json:"job_id"`
		DryRun bool  `json:"dry_run"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/library-folders/"+itoa(folderID)+"/scan", "", 202), &q)
	j := waitJob(t, url, q.JobID)
	b, _ := json.Marshal(j.Result)
	for _, want := range []string{`"kind":"music"`, `"status":"matched"`, `"id":"mbid:` + apiArtist + `"`, `"tracks":3`, `"status":"not_included"`,
		`"track":"track 02 Song 2"`, `"pressing":"First Light (2020, US, CD, 3 tracks)"`, `not in an album folder`} {
		if j.State != "done" || !q.DryRun || !strings.Contains(string(b), want) {
			t.Fatalf("scan lacks %s: %s %s", want, j.State, b)
		}
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM titles").Scan(&n)
	if n != 0 {
		t.Fatal("the scan wrote a title")
	}
	_ = json.Unmarshal(do(t, "POST", u+"/library-folders/"+itoa(folderID)+"/adopt", `{"folders":[{"folder":"Lab Band","id":"mbid:`+apiArtist+`"}]}`, 202), &q)
	j = waitJob(t, url, q.JobID)
	res, _ := j.Result["results"].([]any)
	if j.State != "done" || j.Result["adopted"] != float64(1) || len(res) != 1 {
		t.Fatalf("adopt %+v", j)
	}
	r, _ := res[0].(map[string]any)
	id := int64(r["title_id"].(float64))
	if r["episodes"] != float64(3) {
		t.Fatalf("adopted %+v", r)
	}
	for _, rel := range []string{"Lab Band/First Light (2020)/1 Song 1.flac", "Lab Band/cover.mp3", "Lab Band/Live Night (2021)/01 - Song 1.mp3"} {
		if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("moved: %s", rel)
		}
	}

	// Rename: per artist (with each step's album) and in the whole Library.
	var plan struct {
		Steps    []library.RenameStep `json:"steps"`
		PlanHash string               `json:"plan_hash"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/rename", u, id), "", 200), &plan)
	if len(plan.Steps) != 3 || plan.PlanHash == "" || plan.Steps[0].AlbumID == 0 || plan.Steps[0].To != "Lab Band/First Light (2020)/01 - Song 1.flac" {
		t.Fatalf("rename plan %+v", plan)
	}
	albumID := plan.Steps[0].AlbumID
	var all struct {
		Steps []library.RenameStep `json:"steps"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/rename", "", 200), &all)
	if len(all.Steps) != 3 {
		t.Fatalf("library rename %+v", all)
	}
	do(t, "POST", fmt.Sprintf("%s/titles/%d/rename", u, id), `{}`, 400)
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/titles/%d/rename", u, id), `{"plan_hash":"`+plan.PlanHash+`"}`, 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["renamed"] != float64(3) {
		t.Fatalf("rename %+v", j)
	}
	if _, err := os.Stat(filepath.Join(lib, "Lab Band", "First Light (2020)", "02 - Song 2.flac")); err != nil {
		t.Fatal(err)
	}

	// Delete: a mixed scope is refused, then one track with its file.
	do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), fmt.Sprintf(`{"album_id":%d,"track_ids":[1]}`, albumID), 400)
	do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), fmt.Sprintf(`{"album_id":%d,"never_add":true}`, albumID), 400)
	do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), `{"track_ids":[999999]}`, 400)
	var trackID int64
	_ = s.Read().QueryRow("SELECT id FROM items WHERE part_id = ? AND json_extract(numbers, '$.track') = 3", albumID).Scan(&trackID)
	var rp library.RemovePlan
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), fmt.Sprintf(`{"track_ids":[%d],"delete_files":true}`, trackID), 200), &rp)
	if rp.Applied || rp.Scope != "tracks" || len(rp.Held) != 1 || len(rp.Tracks) != 1 || rp.Tracks[0] != "First Light: track 03 Song 3" {
		t.Fatalf("track plan %+v", rp)
	}
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), fmt.Sprintf(`{"track_ids":[%d],"delete_files":true,"apply":true}`, trackID), 200), &rp)
	if !rp.Applied || len(rp.Held) != 1 {
		t.Fatalf("track delete %+v", rp)
	}
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/titles/%d/delete", u, id), fmt.Sprintf(`{"album_id":%d}`, albumID), 200), &rp)
	if rp.Scope != "album" || rp.Album != "First Light" || len(rp.Kept) != 2 || len(rp.Held) != 0 {
		t.Fatalf("album plan %+v", rp)
	}

	// The album's History: the track delete, the renames of its files.
	var ev struct {
		Events []struct {
			Kind    string
			Summary string
		} `json:"events"`
		Total int `json:"total"`
	}
	do(t, "GET", u+"/events?album=x", "", 400)
	do(t, "GET", u+"/events?album=999999", "", 400)
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/events?album=%d", u, albumID), "", 200), &ev)
	got := []string{}
	for _, e := range ev.Events {
		got = append(got, e.Summary)
	}
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "Deleted Lab Band First Light: track 03 Song 3 from the Library") || !strings.Contains(joined, "Renamed 1 Song 1.flac to 01 - Song 1.flac") ||
		strings.Contains(joined, "Adopted") {
		t.Fatalf("album history: %s", joined)
	}

	// The Calendar: First Light came out on 6 March 2020.
	var cal struct {
		Dates []library.CalendarEntry `json:"dates"`
	}
	do(t, "GET", u+"/calendar?titles=books", "", 400)
	_ = json.Unmarshal(do(t, "GET", u+"/calendar?from=2020-03-01&to=2020-03-31&titles=music", "", 200), &cal)
	if len(cal.Dates) != 1 || cal.Dates[0].Kind != "album" || cal.Dates[0].Album != "First Light" || cal.Dates[0].PartID != albumID || cal.Dates[0].Have != 2 {
		t.Fatalf("calendar %+v", cal)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/calendar?from=2020-03-01&to=2020-03-31&titles=movies", "", 200), &cal)
	if len(cal.Dates) != 0 {
		t.Fatalf("movies calendar %+v", cal)
	}

	// A rescan marks a gone track file missing.
	_ = os.Remove(filepath.Join(lib, "Lab Band", "First Light (2020)", "01 - Song 1.flac"))
	_ = json.Unmarshal(do(t, "POST", u+"/schedules/rescan_library/run", "", 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["missing"] != float64(1) {
		t.Fatalf("rescan %+v", j)
	}
}

// route: GET /api/v1/settings/naming
// route: PUT /api/v1/settings/naming
// route: POST /api/v1/settings/naming/example
//
// 4.15.1: the naming settings carry the music patterns and examples; a bad
// music pattern is refused with the reason.
func TestMusicNamingAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _ := musicOrganiseSetup(t)
	u := url + "/api/v1"
	var v struct {
		Naming  map[string]any    `json:"naming"`
		Music   map[string]string `json:"example_music"`
		Words   map[string]string `json:"words"`
		Example string            `json:"example"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/settings/naming", "", 200), &v)
	if v.Naming["track_pattern"] != "{track} - {track_title}" || v.Naming["music_read_tags"] != true || v.Music["track"] != "The Lab Lanterns/Small Hours (2025)/03 - Harbour Light.flac" ||
		v.Music["multi_disc"] != "The Lab Lanterns/Long Night (2025)/2-04 - Second Wind.flac" || v.Music["various"] != "Various Artists/Lab Mix - Volume 1 (2024)/05 - Night.mp3" || v.Words["artist_sort"] == "" {
		t.Fatalf("naming %+v", v)
	}
	_ = json.Unmarshal(do(t, "POST", u+"/settings/naming/example", `{"artist_folder_pattern":"{artist_sort}","disc_folders":true}`, 200), &v)
	if v.Music["multi_disc"] != "Lab Lanterns, The/Long Night (2025)/Disc 2/04 - Second Wind.flac" {
		t.Fatalf("example %+v", v.Music)
	}
	// {track_artist} is the album's artist on every real import today, so
	// the various-artists example shows that, never a made-up guest.
	_ = json.Unmarshal(do(t, "POST", u+"/settings/naming/example", `{"track_pattern":"{track} - {track_artist} - {track_title}"}`, 200), &v)
	if v.Music["various"] != "Various Artists/Lab Mix - Volume 1 (2024)/05 - Various Artists - Night.mp3" {
		t.Fatalf("track artist example %+v", v.Music)
	}
	if v.Naming["music_adopt_scan_limit"] != float64(200) {
		t.Fatalf("music scan limit %+v", v.Naming["music_adopt_scan_limit"])
	}
	if b := do(t, "POST", u+"/settings/naming/example", `{"track_pattern":"{track_title}"}`, 400); !strings.Contains(string(b), "must hold {track}") {
		t.Fatalf("%s", b)
	}
	do(t, "PUT", u+"/settings/naming", `{"album_folder_pattern":"{title}"}`, 400)
	_ = json.Unmarshal(do(t, "PUT", u+"/settings/naming", `{"album_folder_pattern":"{year} - {album}","music_read_tags":false}`, 200), &v)
	if v.Naming["album_folder_pattern"] != "{year} - {album}" || v.Naming["music_read_tags"] != false || v.Music["album_folder"] != "2025 - Small Hours" {
		t.Fatalf("saved %+v", v)
	}
}
