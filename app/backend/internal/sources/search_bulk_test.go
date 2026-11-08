package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestBulkSearch_SkipGatePicks checks that a bulk search with the gate setting off may pick for a Slot not out yet, and its verdict says the gate was skipped.
func TestBulkSearch_SkipGatePicks(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, "UPDATE slots SET state = 'not_out_yet'")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"bulk": true, "skip_gate": true})
	if st := slotState(t, f).State; st != mediafiles.SlotChosen {
		t.Fatalf("slot %s", st)
	}
	if r := relByName(t, f, web1080); !strings.Contains(fmt.Sprint(r.Verdict["reasons"]), "gate_skipped") {
		t.Fatalf("no gate warning: %v", r.Verdict["reasons"])
	}
}

// TestChoose_OnNeedsADecision checks that choosing for a Slot that cannot take a pick is busy, not a failure.
// A Slot that needs a decision does take a hand choice.
func TestChoose_OnNeedsADecision(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	id := addRelease(t, f, tv720, "judged")
	exec(t, f, "UPDATE slots SET state = 'needs_a_decision'")
	if _, err := (&Searcher{Store: f.s}).Choose(context.Background(), id, 0, true, domain.System); err != nil {
		t.Fatalf("err %v", err)
	}
	if st := slotState(t, f).State; st != mediafiles.SlotChosen {
		t.Fatalf("slot %s", st)
	}
	exec(t, f, "UPDATE slots SET state = 'importing'")
	if _, err := (&Searcher{Store: f.s}).Choose(context.Background(), id, 0, true, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v", err)
	}
}

// TestHandSearch_FromNeedsADecision checks that searching again on a Slot that needs a decision is a hand search.
func TestHandSearch_FromNeedsADecision(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, "UPDATE slots SET state = 'needs_a_decision'")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"bulk": true})
	if st := slotState(t, f).State; st != mediafiles.SlotNeedsADecision {
		t.Fatalf("a bulk search moved it: %s", st)
	}
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	if st := slotState(t, f).State; st != mediafiles.SlotChosen {
		t.Fatalf("slot %s", st)
	}
}

// TestHandSearch_FromNeedsADecisionFindsNothing checks that a search again that finds nothing leaves the Slot needing a decision.
func TestHandSearch_FromNeedsADecisionFindsNothing(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf())
	f := seed(t, lf.url(t))
	exec(t, f, "UPDATE slots SET state = 'needs_a_decision'")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	if sl := slotState(t, f); sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || !strings.HasPrefix(*sl.LastReason, "searched again") {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
}

// TestFeed_LooksAgainAtNamesOthersStored checks that a feed check looks again at a name another search stored and passed over, and skips names it stored itself.
func TestFeed_LooksAgainAtNamesOthersStored(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	addRelease(t, f, web1080, "passed_over")
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, rel int64) { chosen = append(chosen, rel) }}
	runKind(t, f, sr, jobs.KindFeedCheck, nil, nil)
	if len(chosen) != 1 {
		t.Fatalf("feed did not look again: %v", chosen)
	}
}

// TestBulkSearch_CappedAndStaggered checks that a bulk search asked by a person stops at the per-run cap and spreads its searches out in time.
func TestBulkSearch_CappedAndStaggered(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	exec(t, f, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main)
		SELECT created_at, created_by, updated_at, updated_by, title_id, '4K', pick_rules_id, library_folder_id, 'Dune 4K', 1, 0 FROM versions LIMIT 1`)
	exec(t, f, `INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
		SELECT created_at, created_by, updated_at, updated_by, item_id, (SELECT max(id) FROM versions), 'wanted' FROM slots LIMIT 1`)
	exec(t, f, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', 'bulk_search_cap', '1', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`)
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, map[string]any{"which": "missing"})
	if j.Result["queued"] != float64(1) {
		t.Fatalf("result %v", j.Result)
	}
	exec(t, f, "DELETE FROM settings WHERE key = 'bulk_search_cap'")
	exec(t, f, "DELETE FROM jobs WHERE kind = 'search_slot'")
	// The queued searches do nothing here, so a worker that runs one never moves the run times the handler set.
	before := time.Now().UTC().Add(-time.Second)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, map[string]any{"which": "missing"}, func(q *jobs.Queue) {
		q.Handle(jobs.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
	})
	js, _ := database.Select[domain.Job](context.Background(), f.s.Read(), "jobs", "kind = 'search_slot' ORDER BY id")
	if len(js) != 2 {
		t.Fatalf("%d searches queued", len(js))
	}
	if js[0].RunAfter.Before(before) || js[1].RunAfter.Sub(js[0].RunAfter) < bulkStagger-time.Second {
		t.Fatalf("searches not spread: %v then %v", js[0].RunAfter, js[1].RunAfter)
	}
}
