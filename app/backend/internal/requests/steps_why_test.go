package requests

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// the person who asked is told what kind of wait it is, without the
// search's own words: releases were found and the rules turned every one
// down, or nothing is out yet and when it is expected.
func TestAskStepSaysWhyItWaits(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	r.svc.Now = func() time.Time { return now }
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	step := func() *Step {
		vs, err := r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
		if err != nil || len(vs) != 1 {
			t.Fatalf("views %v %v", vs, err)
		}
		r.svc.AddSteps(ctx, vs, false)
		return vs[0].Step
	}
	exec := func(q string, args ...any) {
		if err := r.st.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	slot := "id IN (SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)"
	exec("UPDATE jobs SET state = 'done'")
	exec("UPDATE items SET last_searched_at = '2026-11-03T08:00:00Z' WHERE title_id = ?", *got.TitleID)

	// Found, and every one turned down: said so, with none of the search's words.
	exec("UPDATE slots SET state = 'wanted', last_reason = 'no Release passed the Pick rules (2 found, 2 refused)' WHERE "+slot, *got.TitleID)
	if s := step(); s == nil || s.Kind != "waiting" || s.Why != "rules" || s.Text != "" {
		t.Fatalf("turned down by the rules %+v", s)
	}
	// Nothing found at all is not the rules' doing.
	exec("UPDATE slots SET last_reason = 'no Release passed the Pick rules (0 found, 0 refused)' WHERE "+slot, *got.TitleID)
	if s := step(); s == nil || s.Kind != "waiting" || s.Why != "" {
		t.Fatalf("nothing found %+v", s)
	}
	// Nor is a search that could not run.
	exec("UPDATE slots SET last_reason = 'no torrent Downloader is set up' WHERE "+slot, *got.TitleID)
	if s := step(); s == nil || s.Why != "" {
		t.Fatalf("blocked search %+v", s)
	}

	// Not out yet: the day it is expected, when one is known and still ahead.
	exec("UPDATE slots SET state = 'not_out_yet' WHERE "+slot, *got.TitleID)
	exec("UPDATE titles SET gate_opens_at = '2027-03-12T00:00:00Z' WHERE id = ?", *got.TitleID)
	want := time.Date(2027, 3, 12, 0, 0, 0, 0, time.UTC)
	if s := step(); s == nil || s.Kind != "not_out" || s.OutAt == nil || !s.OutAt.Equal(want) {
		t.Fatalf("not out, date known %+v", s)
	}
	exec("UPDATE titles SET gate_opens_at = '2026-10-01T00:00:00Z' WHERE id = ?", *got.TitleID)
	if s := step(); s == nil || s.Kind != "not_out" || s.OutAt != nil {
		t.Fatalf("not out, date passed %+v", s)
	}
	exec("UPDATE titles SET gate_opens_at = NULL WHERE id = ?", *got.TitleID)
	if s := step(); s == nil || s.Kind != "not_out" || s.OutAt != nil {
		t.Fatalf("not out, no date %+v", s)
	}
}

func TestTurnedDown(t *testing.T) {
	for reason, want := range map[string]bool{
		"no Release passed the Pick rules (2 found, 2 refused)":                                         true,
		"no Release passed the Pick rules for it (14 found, 14 refused)":                                true,
		"searched again and no Release passed the Pick rules (3 found, 3 refused)":                      true,
		"the season search found nothing for it (1 found, 1 refused); searching the episode on its own": true,
		"no Release passed the Pick rules (0 found, 0 refused)":                                         false,
		"no torrent Downloader is set up":                                                               false,
		"out since 2024-03-01":                                                                          false,
		"":                                                                                              false,
	} {
		if got := turnedDown(reason); got != want {
			t.Errorf("turnedDown(%q) = %v, want %v", reason, got, want)
		}
	}
}
