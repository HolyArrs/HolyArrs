package requests

import (
	"context"
	"database/sql"
	"testing"
)

// an approved ask says where it is from the server's records: the
// search its approval started, a download, an import, a decision waiting
// on a person, or why nothing is coming yet (the search's words for staff).
func TestAskSteps(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	step := func(staff bool) *Step {
		vs, err := r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
		if err != nil || len(vs) != 1 {
			t.Fatalf("views %v %v", vs, err)
		}
		r.svc.AddSteps(ctx, vs, staff)
		return vs[0].Step
	}
	exec := func(q string, args ...any) {
		if err := r.st.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	slot := "id IN (SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)"
	// Just approved: the add's chain (refresh, scan, search) is queued.
	if s := step(true); s == nil || s.Kind != "searching" {
		t.Fatalf("after approval %+v", s)
	}
	exec("UPDATE jobs SET state = 'done'")
	// Never looked for yet: its slot's reason is why it is wanted, not why nothing came.
	exec("UPDATE slots SET state = 'wanted', last_reason = 'out since 2024-03-01' WHERE "+slot, *got.TitleID)
	if s := step(true); s == nil || s.Kind != "waiting" || s.Text != "" || s.SearchedAt != nil {
		t.Fatalf("not looked for yet %+v", s)
	}
	exec("UPDATE slots SET last_reason = 'no Release passed the Pick rules (2 found, 2 refused)' WHERE "+slot, *got.TitleID)
	exec("UPDATE items SET last_searched_at = '2026-11-03T08:00:00Z' WHERE title_id = ?", *got.TitleID)
	if s := step(true); s == nil || s.Kind != "waiting" || s.Text != "no Release passed the Pick rules (2 found, 2 refused)" || s.SearchedAt == nil {
		t.Fatalf("nothing found %+v", s)
	}
	// The people who asked are told it waits, not the search's words.
	if s := step(false); s == nil || s.Kind != "waiting" || s.Text != "" {
		t.Fatalf("member's waiting %+v", s)
	}
	for _, c := range []struct{ state, want string }{
		{"searching", "searching"}, {"downloading", "downloading"}, {"importing", "importing"},
		{"needs_a_decision", "waiting_on_admin"}, {"held", "searching"}, {"not_out_yet", "not_out"},
	} {
		exec("UPDATE slots SET state = ? WHERE "+slot, c.state, *got.TitleID)
		if s := step(false); s == nil || s.Kind != c.want {
			t.Fatalf("slot %s: step %+v, want %s", c.state, s, c.want)
		}
	}
	exec("UPDATE slots SET state = 'in_library' WHERE "+slot, *got.TitleID)
	if s := step(true); s != nil {
		t.Fatalf("nothing left to get, step %+v", s)
	}
}

// a step is read for what the ask covers. ana asked season 1
// and bo season 2 of one show: bo's season downloading is not ana's step,
// and a Version nobody keeps up to date (so nobody searches) gives no step.
func TestAskStepsAreTheAsksOwn(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		if err := r.st.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	ana, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, ana.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	bo, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2})
	if err != nil || bo.Ask.ID == ana.Ask.ID {
		t.Fatalf("bo's ask %+v %v", bo, err)
	}
	got, err := r.svc.Approve(ctx, bo.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve bo %+v %v", got, err)
	}
	exec("UPDATE jobs SET state = 'done'")
	season := func(n int) string {
		return "id IN (SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id JOIN parts p ON p.id = i.part_id WHERE p.number = " + map[int]string{1: "1", 2: "2"}[n] + ")"
	}
	exec("UPDATE slots SET state = 'wanted' WHERE " + season(1))
	exec("UPDATE slots SET state = 'downloading' WHERE " + season(2))
	step := func(id int64) *Step {
		vs, _ := r.svc.Views(ctx, "a.id = ?", id)
		r.svc.AddSteps(ctx, vs, true)
		return vs[0].Step
	}
	if s := step(ana.Ask.ID); s == nil || s.Kind != "waiting" {
		t.Fatalf("ana's season 1 %+v", s)
	}
	if s := step(bo.Ask.ID); s == nil || s.Kind != "downloading" {
		t.Fatalf("bo's season 2 %+v", s)
	}
	// Nobody searches a Version that is not kept up to date: no "nothing found yet" forever.
	exec("UPDATE versions SET keep_up_to_date = 0 WHERE title_id = ?", *got.TitleID)
	if s := step(ana.Ask.ID); s != nil {
		t.Fatalf("a Version nobody searches still gives a step %+v", s)
	}
}
