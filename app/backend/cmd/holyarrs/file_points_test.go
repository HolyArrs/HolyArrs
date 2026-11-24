package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// route: GET /api/v1/titles/{id}/points
func TestFilePointsAPI(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	f, v, slot := apiMovieCopy(t, s, false)
	need := 1500
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		body := mediafiles.GoodEveryday()
		body.GoodEnoughPoints = &need
		body.Traits = []mediafiles.TraitPoints{{Trait: "web_dl", Points: 25}, {Trait: "remux", Points: 900}}
		if _, err := tx.Exec("UPDATE pick_rules SET body = ? WHERE id = ?", mustJSON(t, body), v.PickRulesID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET release_name = ? WHERE id = ?", "Arrival.2016.1080p.WEB-DL.DDP5.1.H.264-GRP", f.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var tid int64
	if err := s.Read().QueryRow("SELECT title_id FROM versions WHERE id = ?", v.ID).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Step             string            `json:"step"`
		StepReached      bool              `json:"step_reached"`
		Points           int               `json:"points"`
		GoodEnoughPoints *int              `json:"good_enough_points"`
		GoodEnough       bool              `json:"good_enough"`
		MeetsVersion     bool              `json:"meets_version"`
		Traits           []map[string]any  `json:"traits"`
		Names            map[string]string `json:"names"`
	}
	b := lbDo(t, "GET", srv.URL+"/api/v1/titles/"+lbItoa(tid)+"/points?slot="+lbItoa(slot.ID), "", 200)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Step == "" || !got.StepReached || got.GoodEnoughPoints == nil || *got.GoodEnoughPoints != 1500 || got.GoodEnough {
		t.Fatalf("points %s", b)
	}
	for _, tr := range got.Traits {
		if tr["trait"] == "remux" {
			t.Errorf("remux does not fit: %s", b)
		}
	}
	if got.Points != 25 || len(got.Traits) != 1 || !got.MeetsVersion {
		t.Errorf("web_dl fits: %s", b)
	}
	lbDo(t, "GET", srv.URL+"/api/v1/titles/"+lbItoa(tid)+"/points?slot=999999", "", 404)
	lbDo(t, "GET", srv.URL+"/api/v1/titles/"+lbItoa(tid)+"/points", "", 400)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
