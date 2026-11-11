package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library/trashtest"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TRaSH Guides: profiles are listed, applying one scores Releases through
// synced traits, and a second sync makes no new traits.
//
// route: GET /api/v1/trash/profiles
// route: POST /api/v1/pick-rules/{id}/trash
func TestTrashProfileApplied(t *testing.T) {
	srv, st, _ := lbLibSetup(t)
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	old := downloads.TrashClient
	c := library.NewTrashGuides()
	c.Raw, c.API = trashtest.Serve(t, trashtest.Files)
	downloads.TrashClient = c
	t.Cleanup(func() { downloads.TrashClient = old })

	var list struct {
		Profiles []struct{ ID, Name string } `json:"profiles"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/trash/profiles?app=sonarr", "", 200), &list)
	if len(list.Profiles) != 1 || list.Profiles[0].ID != "p1" {
		t.Fatalf("profiles: %+v", list)
	}
	lbDo(t, "GET", srv.URL+"/api/v1/trash/profiles?app=lidarr", "", 400)

	var p mediafiles.PickRules
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/pick-rules", lbRuleJSON(t, "Synced", []string{"show"}, mediafiles.GoodEveryday()), 201), &p)
	path := fmt.Sprintf("%s/api/v1/pick-rules/%d/trash", srv.URL, p.ID)
	var res struct {
		Added   int `json:"added"`
		Updated int `json:"updated"`
		Scored  []struct {
			Name   string `json:"name"`
			Points int    `json:"points"`
		} `json:"scored"`
		Skipped []struct {
			Name  string   `json:"name"`
			Kinds []string `json:"kinds"`
		} `json:"skipped"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", path, `{"app":"sonarr","profile_id":"p1"}`, 200), &res)
	if res.Added != 3 || len(res.Scored) != 3 || len(res.Skipped) != 1 || res.Skipped[0].Kinds[0] != "language" {
		t.Fatalf("first sync: %+v", res)
	}
	if n := recheckJobs(t, st, p.ID); n != 1 {
		t.Fatalf("re-check jobs after applying a profile: %d, want 1", n)
	}
	_ = json.Unmarshal(lbDo(t, "POST", path, `{"app":"sonarr","profile_id":"p1"}`, 200), &res)
	if res.Added != 0 || res.Updated != 3 {
		t.Fatalf("second sync: %+v", res)
	}
	lbDo(t, "POST", path, `{"app":"sonarr","profile_id":"nope"}`, 404)
	lbDo(t, "POST", path, `{"app":"lidarr","profile_id":"p1"}`, 400)

	var traits struct {
		OwnTraits []downloads.OwnTraitView `json:"own_traits"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/own-traits", "", 200), &traits)
	synced := 0
	for _, tr := range traits.OwnTraits {
		if tr.Source == "trash" {
			synced++
			lbDo(t, "PUT", fmt.Sprintf("%s/api/v1/own-traits/%d", srv.URL, tr.ID), `{"name":"x","match":"name","terms":["x"]}`, 400)
		}
	}
	if synced != 3 {
		t.Fatalf("synced traits: %d", synced)
	}
}
