package sources

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestFeedCheck_RacingHandChoice checks that when a person chooses a Release while a feed check judges the same Slot, the check skips that Slot and the job still finishes.
func TestFeedCheck_RacingHandChoice(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	hand := addRelease(t, f, tv720, "judged")
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, rel int64) { chosen = append(chosen, rel) }}
	beforeSettle = func(slotID int64) {
		if _, err := sr.Choose(context.Background(), hand, slotID, false, domain.Actor{Kind: "user", ID: 1}); err != nil {
			t.Errorf("hand choice: %v", err)
		}
	}
	t.Cleanup(func() { beforeSettle = nil })
	j := runKind(t, f, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if len(chosen) != 1 || chosen[0] != hand {
		t.Fatalf("chosen %v, want only the hand choice %d", chosen, hand)
	}
	if st := relByName(t, f, web1080).State; st == "chosen" {
		t.Fatal("the feed Release is chosen too")
	}
}

// TestFeedCheck_SlotAlreadyDownloading checks the case where the Slot started downloading between judging and settling.
func TestFeedCheck_SlotAlreadyDownloading(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	called := 0
	sr := &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) { called++ }}
	beforeSettle = func(int64) { exec(t, f, "UPDATE slots SET state = 'downloading'") }
	t.Cleanup(func() { beforeSettle = nil })
	j := runKind(t, f, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" || called != 0 || slotState(t, f).State != mediafiles.SlotDownloading {
		t.Fatalf("job %s chosen %d slot %s", j.State, called, slotState(t, f).State)
	}
	if st := relByName(t, f, web1080).State; st == "chosen" {
		t.Fatal("feed Release chosen for a downloading Slot")
	}
}

// TestSettle_RefusesAnotherRelease checks that settling refuses a Slot already chosen for another Release and accepts the same Release again.
func TestSettle_RefusesAnotherRelease(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	sr := &Searcher{Store: f.s}
	a, b := addRelease(t, f, tv720, "judged"), addRelease(t, f, br1080, "judged")
	if err := sr.settleChosen(ctx, f.slotID, a, tv720, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := sr.settleChosen(ctx, f.slotID, b, br1080, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Release: %v", err)
	}
	if err := sr.settleHeld(ctx, f.slotID, b, br1080, time.Now().Add(time.Hour), domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("hold over a chosen Slot: %v", err)
	}
	if err := sr.settleChosen(ctx, f.slotID, a, tv720, domain.System); err != nil {
		t.Fatalf("same Release again: %v", err)
	}
	var st string
	_ = f.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", b).Scan(&st)
	if st == "chosen" {
		t.Fatal("second Release marked chosen")
	}
}

// addRelease stores a judged Release for the fixture's Slot.
func addRelease(t *testing.T, f fixture, name, state string) int64 {
	t.Helper()
	var id int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: f.srcID, Name: name, LinkRef: "x", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{f.slotID}, Verdict: map[string]any{}, FoundBy: "hand_search", State: state}
		var err error
		id, err = database.Insert(tx, "releases", r, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
