package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// organiseWorld adds a movie Library folder and Dune with one imported file.
func organiseWorld(t *testing.T) (url string, s *database.Store, lib string, titleID, fileID int64) {
	t.Helper()
	srv, s, _ := lbLibSetup(t)
	lib = t.TempDir()
	lbDo(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201)
	var added struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &added)
	titleID = added.Title.ID
	rel := "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv"
	if err := os.MkdirAll(filepath.Join(lib, "Dune - Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, filepath.FromSlash(rel)), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS"
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var vid, sid int64
		if err := tx.QueryRow("SELECT v.id, s.id FROM versions v JOIN slots s ON s.version_id = v.id WHERE v.title_id = ?", titleID).Scan(&vid, &sid); err != nil {
			return err
		}
		f := &mediafiles.File{SlotIDs: []int64{sid}, VersionID: vid, RelativePath: rel, Size: 5, AddedAt: time.Now(), Languages: []string{},
			MediaInfo: map[string]any{}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", f, domain.System)
		fileID = id
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, s, lib, titleID, fileID
}

func waitJob(t *testing.T, url string, id int64) domain.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var j domain.Job
		_ = json.Unmarshal(lbDo(t, "GET", url+"/api/v1/jobs/"+lbItoa(id), "", 200), &j)
		if j.State == "done" || j.State == "failed" || time.Now().After(deadline) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// route: GET /api/v1/titles/{id}/rename
// route: POST /api/v1/titles/{id}/rename
// route: GET /api/v1/jobs/{id}
func TestRenameTitleAPI(t *testing.T) {
	t.Parallel()
	url, s, lib, id, fid := organiseWorld(t)
	b := string(lbDo(t, "GET", url+"/api/v1/titles/"+lbItoa(id)+"/rename", "", 200))
	want := "Dune - Part Two (2024)/Dune - Part Two (2024) 1080p BluRay.mkv"
	if !strings.Contains(b, `"to":"`+want+`"`) {
		t.Fatalf("preview %s", b)
	}
	lbDo(t, "GET", url+"/api/v1/titles/999/rename", "", 404)
	var q struct {
		JobID int64 `json:"job_id"`
	}
	// 5.1.7 important 3: the rename needs the hash of the preview it follows.
	lbDo(t, "POST", url+"/api/v1/titles/"+lbItoa(id)+"/rename", `{}`, 400)
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/titles/"+lbItoa(id)+"/rename", `{"plan_hash":"`+planHashOf(t, b)+`"}`, 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["renamed"] != float64(1) {
		t.Fatalf("job %+v", j)
	}
	var f mediafiles.File
	_ = database.Get(context.Background(), s.Read(), "files", fid, &f)
	if f.RelativePath != want {
		t.Fatalf("file %q", f.RelativePath)
	}
	if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(want))); err != nil {
		t.Fatal(err)
	}
	lbDo(t, "GET", url+"/api/v1/jobs/99999", "", 404)
	lbDo(t, "POST", url+"/api/v1/titles/"+lbItoa(id)+"/rename", `{"file_ids":"x"}`, 400)
}

// route: GET /api/v1/rename
// route: POST /api/v1/rename
func TestRenameLibraryAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _, _ := organiseWorld(t)
	b := string(lbDo(t, "GET", url+"/api/v1/rename", "", 200))
	if !strings.Contains(b, `"title_name":"Dune: Part Two"`) || !strings.Contains(b, `"from":"Dune - Part Two (2024)/Dune.Part.Two`) {
		t.Fatalf("preview %s", b)
	}
	var q struct {
		JobID int64 `json:"job_id"`
	}
	lbDo(t, "POST", url+"/api/v1/rename", "", 400)
	lbDo(t, "POST", url+"/api/v1/rename", `{}`, 400)
	// A preview that no longer matches (the naming settings changed after
	// it) is refused and nothing moves.
	lbDo(t, "PUT", url+"/api/v1/settings/naming",
		`{"file_pattern":"{title} {year}","folder_pattern":"{title} ({year})","colon":"remove","illegal":"replace","rename_on_import":true}`, 200)
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/rename", `{"plan_hash":"`+planHashOf(t, b)+`"}`, 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "failed" || j.Error == nil || !strings.Contains(j.Error.Message, "preview again") {
		t.Fatalf("stale job %+v", j)
	}
	b = string(lbDo(t, "GET", url+"/api/v1/rename", "", 200))
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/rename", `{"plan_hash":"`+planHashOf(t, b)+`"}`, 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["renamed"] != float64(1) {
		t.Fatalf("job %+v", j)
	}
	if b := string(lbDo(t, "GET", url+"/api/v1/rename", "", 200)); !strings.HasPrefix(b, "{\"plan_hash\":") || !strings.Contains(b, "\"steps\":[]") {
		t.Fatalf("after %s", b)
	}
}

func planHashOf(t *testing.T, preview string) string {
	t.Helper()
	var p struct {
		PlanHash string `json:"plan_hash"`
	}
	if err := json.Unmarshal([]byte(preview), &p); err != nil || len(p.PlanHash) != 64 {
		t.Fatalf("no plan_hash in %s", preview)
	}
	return p.PlanHash
}

// route: GET /api/v1/settings/naming
// route: PUT /api/v1/settings/naming
func TestNamingSettingsAPI(t *testing.T) {
	t.Parallel()
	url, _, _, id, _ := organiseWorld(t)
	b := string(lbDo(t, "GET", url+"/api/v1/settings/naming", "", 200))
	if !strings.Contains(b, `"file_pattern":"{title} ({year}) {quality}"`) || !strings.Contains(b, `"example":"Dune - Part Two (2024) 1080p BluRay.mkv"`) {
		t.Fatalf("get %s", b)
	}
	b = string(lbDo(t, "PUT", url+"/api/v1/settings/naming",
		`{"file_pattern":"{title} {year} [{tmdb}]","folder_pattern":"{title} ({year})","colon":"remove","illegal":"replace","rename_on_import":true}`, 200))
	if !strings.Contains(b, `"example":"Dune Part Two 2024 [693134].mkv"`) {
		t.Fatalf("put %s", b)
	}
	lbDo(t, "PUT", url+"/api/v1/settings/naming", `{"file_pattern":"{nope}","folder_pattern":"{title}","colon":"smart","illegal":"replace"}`, 400)
	if b := string(lbDo(t, "GET", url+"/api/v1/titles/"+lbItoa(id)+"/rename", "", 200)); !strings.Contains(b, "Dune Part Two 2024 [693134].mkv") {
		t.Fatalf("preview after change %s", b)
	}
}

// route: POST /api/v1/settings/naming/example
func TestNamingExampleAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _, _ := organiseWorld(t)
	b := string(lbDo(t, "POST", url+"/api/v1/settings/naming/example",
		`{"file_pattern":"{title} [{year}]","folder_pattern":"{sort_title}","colon":"dash","illegal":"replace"}`, 200))
	if !strings.Contains(b, `"example":"Dune- Part Two [2024].mkv"`) || !strings.Contains(b, `"example_folder":"Dune- Part Two"`) {
		t.Fatalf("example %s", b)
	}
	lbDo(t, "POST", url+"/api/v1/settings/naming/example", `{"file_pattern":"{nope}","folder_pattern":"{title}","colon":"smart","illegal":"replace"}`, 400)
	// Nothing was saved.
	if b := string(lbDo(t, "GET", url+"/api/v1/settings/naming", "", 200)); !strings.Contains(b, `"file_pattern":"{title} ({year}) {quality}"`) {
		t.Fatalf("saved %s", b)
	}
}

// route: GET /api/v1/settings/files
// route: PUT /api/v1/settings/files
func TestFileRulesAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _, _ := organiseWorld(t)
	if b := string(lbDo(t, "GET", url+"/api/v1/settings/files", "", 200)); !strings.Contains(b, `"holding_days":7`) || !strings.Contains(b, `"min_free_mb":100`) {
		t.Fatalf("get %s", b)
	}
	lbDo(t, "PUT", url+"/api/v1/settings/files", `{"holding_days":14,"min_free_mb":50}`, 400)
	if b := string(lbDo(t, "PUT", url+"/api/v1/settings/files", `{"holding_days":14,"min_free_mb":200,"skip_free_space_check":false}`, 200)); !strings.Contains(b, `"holding_days":14`) {
		t.Fatalf("put %s", b)
	}
	if b := string(lbDo(t, "GET", url+"/api/v1/settings/files", "", 200)); !strings.Contains(b, `"min_free_mb":200`) {
		t.Fatalf("after %s", b)
	}
}

// route: POST /api/v1/titles/{id}/delete
func TestDeleteTitleAPI(t *testing.T) {
	t.Parallel()
	url, s, lib, id, fid := organiseWorld(t)
	path := url + "/api/v1/titles/" + lbItoa(id) + "/delete"
	b := string(lbDo(t, "POST", path, `{"delete_files":true}`, 200))
	if !strings.Contains(b, `"applied":false`) || !strings.Contains(b, `"to":".holyarrs-replaced/`) || !strings.Contains(b, `"holding_days":7`) {
		t.Fatalf("plan %s", b)
	}
	if _, err := os.Stat(filepath.Join(lib, "Dune - Part Two (2024)")); err != nil {
		t.Fatal("the plan changed the disk")
	}
	b = string(lbDo(t, "POST", path, `{"delete_files":true,"never_add":true,"apply":true}`, 200))
	if !strings.Contains(b, `"applied":true`) {
		t.Fatalf("apply %s", b)
	}
	var f mediafiles.File
	_ = database.Get(context.Background(), s.Read(), "files", fid, &f)
	if f.State != "gone" || !strings.HasPrefix(f.RelativePath, ".holyarrs-replaced/") {
		t.Fatalf("file %+v", f)
	}
	if b := string(lbDo(t, "GET", url+"/api/v1/never-add", "", 200)); !strings.Contains(b, `"693134"`) {
		t.Fatalf("never-add %s", b)
	}
	lbDo(t, "POST", path, `{}`, 409)
	lbDo(t, "POST", url+"/api/v1/titles/999/delete", "", 404)
	lbDo(t, "POST", path, `{"apply":"yes"}`, 400)
}

// route: POST /api/v1/holding/clean
func TestCleanHoldingAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _, _ := organiseWorld(t)
	var q struct {
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/holding/clean", "", 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["days_emptied"] != float64(0) {
		t.Fatalf("job %+v", j)
	}
}

// route: POST /api/v1/library-folders/{id}/scan
// route: POST /api/v1/library-folders/{id}/adopt
func TestScanAndAdoptAPI(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbLibSetup(t)
	url := srv.URL
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "Dune - Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(lib, "Dune - Part Two (2024)", "Dune.Part.Two.2024.1080p.WEB-DL.mkv")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	var lf struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201), &lf)
	var q struct {
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/scan", "", 202), &q)
	j := waitJob(t, url, q.JobID)
	b, _ := json.Marshal(j.Result)
	if j.State != "done" || !strings.Contains(string(b), `"status":"matched"`) || !strings.Contains(string(b), `"tmdb_id":"693134"`) {
		t.Fatalf("scan %s %s", j.State, b)
	}
	// The scan Job shows how far it got, done/total folders.
	var pj struct {
		Progress     float64
		ProgressNote *string
	}
	_ = json.Unmarshal(lbDo(t, "GET", url+"/api/v1/jobs/"+lbItoa(q.JobID), "", 200), &pj)
	if pj.Progress != 1 || pj.ProgressNote == nil || *pj.ProgressNote != "1/1" {
		t.Fatalf("scan progress %v %v", pj.Progress, pj.ProgressNote)
	}
	// 5.1.7 minor 9: "scan the rest" starts after the last folder read.
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/scan", `{"after":"Dune - Part Two (2024)"}`, 202), &q)
	j = waitJob(t, url, q.JobID)
	b, _ = json.Marshal(j.Result)
	if j.State != "done" || !strings.Contains(string(b), `"folders":[]`) || !strings.Contains(string(b), `"limit":5000`) {
		t.Fatalf("scan the rest %s %s", j.State, b)
	}
	lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/scan", `{"after":5}`, 400)
	lbDo(t, "POST", url+"/api/v1/library-folders/999/scan", "", 409)
	lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/adopt", `{"folders":[]}`, 400)
	lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/adopt", `{"folders":[{"folder":"x"}]}`, 400)
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/library-folders/"+lbItoa(lf.ID)+"/adopt",
		`{"folders":[{"folder":"Dune - Part Two (2024)","tmdb_id":"693134"}]}`, 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["adopted"] != float64(1) {
		t.Fatalf("adopt %+v", j)
	}
	var f mediafiles.File
	if err := s.Read().QueryRow("SELECT origin, relative_path FROM files").Scan(&f.Origin, &f.RelativePath); err != nil ||
		f.Origin != "adopted" || f.RelativePath != "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.WEB-DL.mkv" {
		t.Fatalf("file %+v %v", f, err)
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatal("the video moved")
	}
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/schedules/rescan_library/run", "", 202), &q)
	if j := waitJob(t, url, q.JobID); j.State != "done" || j.Result["folders"] != float64(1) {
		t.Fatalf("rescan %+v", j)
	}
}

// 4.2.1: the show patterns sit with the movie ones; fields left out keep
// their value, and the examples name episodes.
func TestNamingShowPatternsAPI(t *testing.T) {
	t.Parallel()
	url, _, _, _, _ := organiseWorld(t)
	b := string(lbDo(t, "GET", url+"/api/v1/settings/naming", "", 200))
	if !strings.Contains(b, `"episode_pattern":"{title} - {episode} - {episode_title} {quality}"`) ||
		!strings.Contains(b, `"episode":"Copper Valley (2025)/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv"`) ||
		!strings.Contains(b, `"multi":"Copper Valley (2025)/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv"`) {
		t.Fatalf("get %s", b)
	}
	b = string(lbDo(t, "PUT", url+"/api/v1/settings/naming", `{"episode_pattern":"{title} {episode}","season_folders":false}`, 200))
	if !strings.Contains(b, `"file_pattern":"{title} ({year}) {quality}"`) || !strings.Contains(b, `"episode":"Copper Valley (2025)/Copper Valley S01E02.mkv"`) {
		t.Fatalf("put %s", b)
	}
	lbDo(t, "PUT", url+"/api/v1/settings/naming", `{"episode_pattern":"{title} {quality}"}`, 400)
	b = string(lbDo(t, "POST", url+"/api/v1/settings/naming/example", `{"season_folder_pattern":"S{season}"}`, 200))
	if !strings.Contains(b, `"special":"Copper Valley (2025)/Specials/`) && !strings.Contains(b, `Copper Valley (2025)/Copper Valley S00E01.mkv`) {
		t.Fatalf("example %s", b)
	}
}
