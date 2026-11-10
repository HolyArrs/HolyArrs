package library

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library/trashtest"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestApplyTrash: applying a profile makes synced traits with its points,
// keeps the person's own points, and a second sync updates in place.
func TestApplyTrash(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	c := NewTrashGuides()
	c.Raw, c.API = trashtest.Serve(t, trashtest.Files)

	mine, err := CreateOwnTrait(ctx, st, OwnTraitInput{Name: "HEVC", Match: "name", Terms: []string{"hevc"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	body := mediafiles.GoodEveryday()
	body.Traits = append(body.Traits, mediafiles.TraitPoints{Trait: mediafiles.OwnTraitKey(mine.ID), Points: 7})
	p, err := CreatePickRules(ctx, st, PickRulesInput{Name: "Mine", Kinds: []string{"show"}, Body: body}, domain.System)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ApplyTrash(ctx, st, c, p.ID, "sonarr", "p1", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Updated != 0 || len(res.Skipped) != 1 {
		t.Fatalf("first sync: %+v", res)
	}
	b := res.PickRules.Body
	if b.Synced == nil || b.Synced.ProfileID != "p1" || b.MinPoints != 0 || b.GoodEnoughPoints == nil || *b.GoodEnoughPoints != 10000 || b.UpgradeMinPoints != 1 {
		t.Fatalf("rules: %+v", b)
	}
	points := map[string]int{}
	for _, tp := range b.Traits {
		points[tp.Trait] = tp.Points
	}
	if points[mediafiles.OwnTraitKey(mine.ID)] != 7 {
		t.Fatal("the person's own points were lost")
	}
	ts, _ := OwnTraits(ctx, st)
	names := map[string]bool{}
	for _, tr := range ts {
		names[tr.Name] = true
	}
	if !names["HEVC (shows)"] || !names["LQ"] {
		t.Fatalf("names: %v", names)
	}

	again, err := ApplyTrash(ctx, st, c, p.ID, "sonarr", "p1", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if again.Added != 0 || again.Updated != 3 {
		t.Fatalf("re-sync: %+v", again)
	}
	if ts2, _ := OwnTraits(ctx, st); len(ts2) != len(ts) {
		t.Fatalf("re-sync made %d traits, had %d", len(ts2), len(ts))
	}
	if len(again.PickRules.Body.Traits) != len(b.Traits) {
		t.Fatalf("re-sync points list %d, was %d", len(again.PickRules.Body.Traits), len(b.Traits))
	}

	// A synced trait cannot be edited by hand.
	var synced mediafiles.OwnTrait
	for _, tr := range ts {
		if tr.Source == "trash" {
			synced = tr
		}
	}
	if _, err := UpdateOwnTrait(ctx, st, synced.ID, OwnTraitInput{Name: "X", Match: "name", Terms: []string{"x"}}, domain.System); err == nil {
		t.Fatal("a synced trait was edited")
	}
	if _, err := ApplyTrash(ctx, st, c, p.ID, "sonarr", "nope", domain.System); err != ErrTrashProfile {
		t.Fatalf("unknown profile: %v", err)
	}
}
