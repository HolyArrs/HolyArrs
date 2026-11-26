package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// route: GET /api/v1/titles/{id}/releases
// The list says when the Pick rules changed after the search, so the sheet
// can say its verdicts are out of date.
func TestReleasesListRulesChanged(t *testing.T) {
	t.Parallel()
	u, s, _, titleID, _, _ := searchWorld(t)
	var page struct {
		RulesChanged bool `json:"rules_changed"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases", "", 200), &page)
	if page.RulesChanged {
		t.Fatal("fresh search marked out of date")
	}
	later := database.FormatTime(time.Now().Add(time.Hour))
	run := func(q string, args ...any) {
		t.Helper()
		if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	// A grab or a folder move saves the Version but leaves its rules alone.
	run("UPDATE versions SET updated_at = ? WHERE title_id = ?", later, titleID)
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases", "", 200), &page)
	if page.RulesChanged {
		t.Fatal("a Version save without a rules change marked the list out of date")
	}
	// The Version switched to other Pick rules than the search judged with.
	run(`UPDATE jobs SET result = (SELECT json_object('version_id', id, 'pick_rules_id', pick_rules_id) FROM versions WHERE title_id = ? LIMIT 1)
		WHERE id IN (SELECT job_id FROM releases)`, titleID)
	var was int64
	if err := s.Read().QueryRow("SELECT pick_rules_id FROM versions WHERE title_id = ? LIMIT 1", titleID).Scan(&was); err != nil {
		t.Fatal(err)
	}
	run(`INSERT INTO pick_rules (created_at, created_by, updated_at, updated_by, name, kinds, body)
		SELECT created_at, created_by, created_at, updated_by, name || ' copy', kinds, body FROM pick_rules
		WHERE id = (SELECT pick_rules_id FROM versions WHERE title_id = ? LIMIT 1)`, titleID)
	run("UPDATE versions SET pick_rules_id = (SELECT max(id) FROM pick_rules) WHERE title_id = ?", titleID)
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases", "", 200), &page)
	if !page.RulesChanged {
		t.Fatal("Version moved to other Pick rules, list not marked")
	}
	run("UPDATE versions SET pick_rules_id = ? WHERE title_id = ?", was, titleID)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE pick_rules SET updated_at = ? WHERE id IN (SELECT pick_rules_id FROM versions WHERE title_id = ?)", later, titleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases", "", 200), &page)
	if !page.RulesChanged {
		t.Fatal("rules changed after the search, list not marked")
	}
}
