package sources

import (
	"context"
	"database/sql"
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

// A turned-down release is never picked again. Choosing it by hand needs force
// and logs the reasons.
func TestHandle_TurnedDownNeverPickedAndForceNeeded(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(br1080, time.Now().Add(-time.Hour), 10, ""), item(tv720, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	ctx := context.Background()
	sr := &Searcher{Store: f.s}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"choose": false})
	best := relByName(t, f, br1080)
	if err := sr.TurnDown(ctx, best.ID, "bad audio", domain.System); err != nil {
		t.Fatal(err)
	}
	var picked []int64
	sr.OnChosen = func(_ context.Context, _, rel int64) { picked = append(picked, rel) }
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	again := relByName(t, f, br1080)
	if again.ID == best.ID || again.State != "refused" || len(picked) != 1 || picked[0] == again.ID {
		t.Fatalf("turned down Release picked again: state %s picked %v", again.State, picked)
	}
	if relByName(t, f, tv720).State != "chosen" {
		t.Fatal("the next best should be chosen")
	}
	exec(t, f, "UPDATE slots SET state = 'wanted'")
	_, err := sr.Choose(ctx, again.ID, 0, false, domain.Actor{Kind: "user", ID: 1})
	var nf *NeedsForceError
	if !errors.As(err, &nf) || !strings.Contains(nf.Error(), "turned down for this Slot") {
		t.Fatalf("err %v", err)
	}
	ch, err := sr.Choose(ctx, again.ID, 0, true, domain.Actor{Kind: "user", ID: 1})
	if err != nil || !ch.Forced || len(ch.Warnings) == 0 || slotState(t, f).State != mediafiles.SlotChosen {
		t.Fatalf("forced choice %+v %v", ch, err)
	}
	evs, _ := f.s.RecentEvents(ctx, 5)
	if evs[0].Kind != EventChosenByHand || evs[0].Data["reasons_overridden"] == nil {
		t.Fatalf("event %+v", evs[0])
	}
	if _, err := sr.Choose(ctx, again.ID, 0, true, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy %v", err)
	}
}

// A hand choice for a title that is not out yet works, with a warning. An
// automatic search leaves the Slot alone and a hand search skips the gate.
func TestHandle_GateSkippedOnlyByHand(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, "UPDATE slots SET state = 'not_out_yet'")
	sr := &Searcher{Store: f.s}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if slotState(t, f).State != mediafiles.SlotNotOutYet || lf.hits != 0 {
		t.Fatal("an automatic search ran on a Slot not out yet")
	}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	if slotState(t, f).State != mediafiles.SlotChosen {
		t.Fatalf("hand search: %s", slotState(t, f).State)
	}
	rel := relByName(t, f, web1080)
	if !strings.Contains(fmt.Sprint(rel.Verdict["reasons"]), "gate_skipped") {
		t.Fatalf("no gate warning in %v", rel.Verdict["reasons"])
	}
	exec(t, f, "UPDATE slots SET state = 'not_out_yet'")
	ch, err := sr.Choose(context.Background(), rel.ID, 0, false, domain.System)
	if err != nil || len(ch.Warnings) != 1 || !strings.Contains(ch.Warnings[0], "gate") {
		t.Fatalf("choice %+v %v", ch, err)
	}
}

// With a file in place only a better release passes, and nothing passes once
// the file is good enough.
func TestHandle_BetterCopySearch(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(tv720, time.Now().Add(-time.Hour), 10, ""), item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	ctx := context.Background()
	addFile := func(name string) {
		if err := f.s.Write(ctx, func(tx *sql.Tx) error {
			_, _ = tx.Exec("UPDATE files SET state = 'replaced'")
			var vid int64
			_ = tx.QueryRow("SELECT version_id FROM slots WHERE id = ?", f.slotID).Scan(&vid)
			fl := &mediafiles.File{SlotIDs: []int64{f.slotID}, VersionID: vid, RelativePath: "Dune/" + name + ".mkv", AddedAt: time.Now(), Languages: []string{},
				MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
			id, err := database.Insert(tx, "files", fl, domain.System)
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'better_wanted' WHERE id = ?", id, f.slotID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	addFile("Dune.Part.Two.2024.720p.WEB-DL.x264-OLD")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if relByName(t, f, tv720).State != "refused" || relByName(t, f, web1080).State != "chosen" {
		t.Fatalf("720p tv %s, 1080p web %s", relByName(t, f, tv720).State, relByName(t, f, web1080).State)
	}
	addFile("Dune.Part.Two.2024.1080p.WEB-DL.x264-OLD")
	exec(t, f, "UPDATE slots SET state = 'better_wanted'")
	exec(t, f, "DELETE FROM releases")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if got := relByName(t, f, web1080); got.State != "refused" || !strings.Contains(fmt.Sprint(got.Verdict["reasons"]), "good_enough_already") {
		t.Fatalf("good enough copy still replaced: %s", got.State)
	}
	if slotState(t, f).State != mediafiles.SlotBetterWanted {
		t.Fatalf("slot %s", slotState(t, f).State)
	}
}

// Held and turned-down lists: paging, remove one, clear all.
func TestLists_PagingAndRemoval(t *testing.T) {
	f := seed(t, "http://unused/api")
	ctx := context.Background()
	sr := &Searcher{Store: f.s}
	var held []int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		for i := 0; i < 5; i++ {
			until := time.Now().Add(time.Duration(i+1) * time.Hour)
			st := "held"
			if i >= 3 {
				st = "turned_down"
			}
			r := &domain.Release{SourceID: f.srcID, Name: fmt.Sprintf("Dune.Part.Two.2024.1080p.WEB-DL.x264-G%d", i), LinkRef: "x", Protocol: "torrent", Flags: []string{},
				Parsed: map[string]any{}, MatchedSlotIDs: []int64{f.slotID}, Verdict: map[string]any{}, FoundBy: "feed", State: st}
			if st == "held" {
				r.HoldUntil = &until
			}
			id, err := database.Insert(tx, "releases", r, domain.System)
			if err != nil {
				return err
			}
			if st == "held" {
				held = append(held, id)
			}
		}
		_, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ? WHERE id = ?", held[0], f.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	page, total, err := sr.List(ctx, "held", 2, 0)
	if err != nil || total != 3 || len(page) != 2 || page[0].ID != held[0] || page[0].TitleName != "Dune: Part Two" || page[0].SourceName != "Test" {
		t.Fatalf("page 1: %d of %d %v", len(page), total, err)
	}
	page, _, _ = sr.List(ctx, "held", 2, 2)
	if len(page) != 1 || page[0].ID != held[2] {
		t.Fatalf("page 2: %+v", page)
	}
	if n, err := sr.RemoveHeld(ctx, held[0], domain.System); err != nil || n != 1 {
		t.Fatalf("remove %d %v", n, err)
	}
	if sl := slotState(t, f); sl.State != mediafiles.SlotWanted || sl.HeldReleaseID != nil {
		t.Fatalf("slot %s", sl.State)
	}
	if _, err := sr.RemoveHeld(ctx, held[0], domain.System); !errors.Is(err, ErrState) {
		t.Fatalf("removing twice: %v", err)
	}
	if n, _ := sr.RemoveHeld(ctx, 0, domain.System); n != 2 {
		t.Fatalf("clear all held removed %d", n)
	}
	_, total, _ = sr.List(ctx, "turned_down", 10, 0)
	if total != 2 {
		t.Fatalf("turned down %d", total)
	}
	if n, _ := sr.RemoveTurnedDown(ctx, 0, domain.System); n != 2 {
		t.Fatalf("clear turned down %d", n)
	}
}
