package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// GET /api/v1/searches names the episode of a Slot search and the season of a
// season search through the library catalog adapters.
func TestSearchesShowSlotAndPartLabels(t *testing.T) {
	t.Parallel()
	srv, _, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var add addedShow
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100"}`, 201), &add)
	tu := fmt.Sprintf("%s/titles/%d", u, add.Title.ID)
	var eps struct {
		Episodes []EpisodeView `json:"episodes"`
	}
	_ = json.Unmarshal(do(t, "GET", tu+"/episodes?season=1", "", 200), &eps)
	if len(eps.Episodes) == 0 {
		t.Fatal("no episodes")
	}
	ids := func(b []byte) int64 {
		var out struct {
			JobIDs []int64 `json:"job_ids"`
		}
		_ = json.Unmarshal(b, &out)
		if len(out.JobIDs) != 1 {
			t.Fatalf("jobs %s", b)
		}
		return out.JobIDs[0]
	}
	slotJob := ids(do(t, "POST", tu+"/search", fmt.Sprintf(`{"slot_id":%d,"choose":false}`, eps.Episodes[0].Slots[0].ID), 202))
	partJob := ids(do(t, "POST", tu+"/search", `{"season":1,"choose":false}`, 202))

	var out struct {
		Running []sources.SearchView `json:"running"`
		Recent  []sources.SearchView `json:"recent"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/searches", "", 200), &out)
	var slotSeen, partSeen bool
	for _, v := range append(out.Running, out.Recent...) {
		switch v.JobID {
		case slotJob:
			slotSeen = true
			if v.Episode != "S01E01" || v.TitleID == nil || *v.TitleID != add.Title.ID {
				t.Fatalf("slot view %+v", v)
			}
		case partJob:
			partSeen = true
			if v.Episode != "S01" || v.TitleID == nil || *v.TitleID != add.Title.ID || v.TitleName != "Lab Show" {
				t.Fatalf("part view %+v", v)
			}
		}
	}
	if !slotSeen || !partSeen {
		t.Fatalf("missing views slot=%v part=%v: %+v", slotSeen, partSeen, out)
	}
}
