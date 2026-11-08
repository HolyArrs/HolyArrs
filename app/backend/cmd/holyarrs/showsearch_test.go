package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Show search over HTTP (stage 3.2): the existing search routes take an
// episode, a season or the whole show; Releases list per season or
// episode with episode names; a Release is turned down for one episode.
//
// route: POST /api/v1/titles/{id}/search
// route: GET /api/v1/titles/{id}/releases
// route: POST /api/v1/releases/{id}/turn-down
// route: GET /api/v1/turned-down
// route: DELETE /api/v1/turned-down/{id}
// route: GET /api/v1/searches
func TestShowSearchOverHTTP(t *testing.T) {
	t.Parallel()
	srv, s, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var add addedShow
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100"}`, 201), &add)
	id := add.Title.ID
	tu := fmt.Sprintf("%s/titles/%d", u, id)
	ctx := context.Background()

	var eps struct {
		Episodes []EpisodeView `json:"episodes"`
	}
	_ = json.Unmarshal(do(t, "GET", tu+"/episodes?season=1", "", 200), &eps)
	if len(eps.Episodes) != 2 || eps.Episodes[0].Slots[0].State != "wanted" {
		t.Fatalf("season 1 %+v", eps.Episodes)
	}
	e1, e2 := eps.Episodes[0], eps.Episodes[1]
	s1, s2 := e1.Slots[0].ID, e2.Slots[0].ID

	jobOf := func(body []byte) []domain.Job {
		var out struct {
			JobIDs []int64 `json:"job_ids"`
		}
		_ = json.Unmarshal(body, &out)
		var js []domain.Job
		for _, j := range out.JobIDs {
			var job domain.Job
			if err := database.Get(ctx, s.Read(), "jobs", j, &job); err != nil {
				t.Fatal(err)
			}
			js = append(js, job)
		}
		return js
	}

	// The whole show: one season search for season 1 (season 2 has not
	// aired, specials are not kept up to date).
	js := jobOf(do(t, "POST", tu+"/search", "", 202))
	if len(js) != 1 || js[0].Kind != jobs.KindSearchSeason || js[0].Params["kept_only"] != true || js[0].Params["by_hand"] != true {
		t.Fatalf("whole show %+v", js)
	}
	seasonJob := js[0]
	// A season to choose from.
	js = jobOf(do(t, "POST", tu+"/search", `{"season":1,"choose":false}`, 202))
	if len(js) != 1 || js[0].Kind != jobs.KindSearchSeason || js[0].Params["choose"] != false || js[0].ID == seasonJob.ID {
		t.Fatalf("season list %+v", js)
	}
	// One episode, by its episode or its Slot.
	js = jobOf(do(t, "POST", tu+"/search", fmt.Sprintf(`{"item_id":%d}`, e1.ID), 202))
	if len(js) != 1 || js[0].Kind != jobs.KindSearchSlot || js[0].Subject.ID != s1 || js[0].Params["by_hand"] != true {
		t.Fatalf("episode %+v", js)
	}
	js = jobOf(do(t, "POST", tu+"/search", fmt.Sprintf(`{"slot_id":%d,"choose":false}`, s2), 202))
	if len(js) != 1 || js[0].Subject.ID != s2 || js[0].Params["choose"] != false {
		t.Fatalf("slot %+v", js)
	}
	do(t, "POST", tu+"/search", `{"choose":false}`, 400)
	do(t, "POST", tu+"/search", `{"season":7}`, 404)
	do(t, "POST", tu+"/search", `{"slot_id":99999}`, 404)
	do(t, "POST", tu+"/search", `{"item_id":99999}`, 404)
	do(t, "POST", tu+"/search", `{"version_id":99999}`, 404)

	// A season pack the season search found, judged for both episodes.
	var relID, srcID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		srcID = src.ID
		rel := &domain.Release{SourceID: src.ID, Name: "Lab.Show.S01.1080p.WEB-DL.H.264-QUILL", LinkRef: "http://x/1.torrent", Protocol: "torrent",
			Size: 3 << 30, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{s1, s2}, FoundBy: "hand_search", JobID: &seasonJob.ID,
			State: "judged", Verdict: map[string]any{"outcome": "chosen", "passed": true, "rank": 1, "reasons": []any{
				map[string]any{"step": "match", "rule": "episode", "effect": "pass", "detail": "holds S01E01, S01E02"}}}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		relID = rel.ID
		_, err := tx.Exec("UPDATE jobs SET state = 'done', result = '{\"releases\":1,\"refused\":0,\"chosen_release_id\":null}', ended_at = ? WHERE id = ?",
			database.FormatTime(seasonJob.CreatedAt), seasonJob.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = srcID
	var list struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
	}
	_ = json.Unmarshal(do(t, "GET", tu+"/releases?season=1", "", 200), &list)
	if list.Total != 1 || list.Releases[0].Episode != "S01E01, S01E02" || !slices.Equal(list.Releases[0].SlotIDs, []int64{s1, s2}) ||
		list.Releases[0].TitleName != "Lab Show" {
		t.Fatalf("season list %+v", list)
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/releases?item_id=%d", tu, e2.ID), "", 200), &list)
	if list.Total != 1 {
		t.Fatalf("episode list %+v", list)
	}
	_ = json.Unmarshal(do(t, "GET", tu+"/releases?season=2", "", 200), &list)
	if list.Total != 0 {
		t.Fatalf("season 2 list %+v", list)
	}
	do(t, "GET", tu+"/releases?season=x", "", 400)

	// Turned down for S01E02 only.
	var v sources.ReleaseView
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/releases/%d/turn-down", u, relID), fmt.Sprintf(`{"reason":"bad audio","slot_id":%d}`, s2), 200), &v)
	if v.State != "turned_down" || v.TurnedDownFor != "S01E02" || v.SlotID == nil || *v.SlotID != s2 {
		t.Fatalf("turned down %+v", v)
	}
	do(t, "POST", fmt.Sprintf("%s/releases/%d/turn-down", u, relID), `{"slot_id":99999}`, 400)
	var td struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/turned-down", "", 200), &td)
	if td.Total != 1 || td.Releases[0].TurnedDownFor != "S01E02" || td.Releases[0].TitleName != "Lab Show" {
		t.Fatalf("turned down list %+v", td)
	}

	// Home names the season search.
	var home struct {
		Recent []sources.SearchView `json:"recent"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/searches", "", 200), &home)
	found := false
	for _, x := range home.Recent {
		found = found || x.JobID == seasonJob.ID && x.Episode == "S01" && x.TitleName == "Lab Show" && x.Version != ""
	}
	if !found {
		t.Fatalf("searches %+v", home.Recent)
	}

	do(t, "DELETE", fmt.Sprintf("%s/turned-down/%d", u, relID), "", 200)
	_ = json.Unmarshal(do(t, "GET", u+"/turned-down", "", 200), &td)
	if td.Total != 0 {
		t.Fatalf("still turned down %+v", td)
	}
}

// A whole-show search by hand leaves out a show or Version that is not
// kept up to date (3.2 review, M6).
//
// route: POST /api/v1/titles/{id}/search
func TestWholeShowSearchKeptOnly(t *testing.T) {
	t.Parallel()
	srv, s, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var add addedShow
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100"}`, 201), &add)
	tu := fmt.Sprintf("%s/titles/%d", u, add.Title.ID)
	count := func() int {
		var out struct {
			JobIDs []int64 `json:"job_ids"`
		}
		_ = json.Unmarshal(do(t, "POST", tu+"/search", "", 202), &out)
		return len(out.JobIDs)
	}
	set := func(q string) {
		if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, add.Title.ID); return err }); err != nil {
			t.Fatal(err)
		}
	}
	set("UPDATE versions SET keep_up_to_date = 0 WHERE title_id = ?")
	if n := count(); n != 0 {
		t.Fatalf("Version not kept: %d searches", n)
	}
	set("UPDATE versions SET keep_up_to_date = 1 WHERE title_id = ?")
	set("UPDATE titles SET keep_up_to_date = 0 WHERE id = ?")
	if n := count(); n != 0 {
		t.Fatalf("show not kept: %d searches", n)
	}
}
