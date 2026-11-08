package library

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// judgeNever says whether the judge turns the release down for a "never"
// own trait, which tells what the judge currently knows about the trait.
func judgeNever(id int64, release string) bool {
	rules := mediafiles.GoodEveryday()
	rules.Never = append(rules.Never, mediafiles.OwnTraitKey(id))
	rel := mediafiles.ReleaseFacts{Name: release, Parse: releasename.Parse(release, releasename.Hints{}), Size: 4 << 30, Protocol: "torrent"}
	m := mediafiles.MatchTitle(rel.Parse, []mediafiles.TitleRef{{ID: 1, Name: "Dune Part Two", Year: 2024, OriginalLanguage: "en", Runtime: 166}})
	v := mediafiles.Judge(rel, mediafiles.SlotSet{Match: m, SlotIDs: []int64{1}, RuntimeMinutes: 166, Out: true}, rules)
	for _, r := range v.Reasons {
		if r.Effect == "fail" && r.Code() == "hard.never:"+mediafiles.OwnTraitKey(id) {
			return true
		}
	}
	return false
}

// TestOwnTraitCRUD: names are unique ignoring case, every change reaches the
// judge, and deleting a trait takes it out of the Pick rules that use it.
func TestOwnTraitCRUD(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	const bad = "Dune.Part.Two.2024.1080p.WEB.H264.NLSubs-HOLYARRS"

	a, err := CreateOwnTrait(ctx, st, OwnTraitInput{Name: "Dual", Match: "name", Terms: []string{"NLSubs"}}, domain.System)
	if err != nil || a.ID == 0 {
		t.Fatalf("create: %+v %v", a, err)
	}
	if !judgeNever(a.ID, bad) {
		t.Fatal("the judge does not know a trait right after it was created")
	}
	if _, err := CreateOwnTrait(ctx, st, OwnTraitInput{Name: "dual", Match: "name", Terms: []string{"x"}}, domain.System); !errors.Is(err, ErrOwnTraitName) {
		t.Fatalf("duplicate name differing in case: %v", err)
	}
	if _, err := CreateOwnTrait(ctx, st, OwnTraitInput{Name: "Empty", Match: "name"}, domain.System); err == nil {
		t.Fatal("a trait without terms was accepted")
	}
	b, err := CreateOwnTrait(ctx, st, OwnTraitInput{Name: "Other", Match: "name", Terms: []string{"zzz"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateOwnTrait(ctx, st, b.ID, OwnTraitInput{Name: "DUAL", Match: "name", Terms: []string{"zzz"}}, domain.System); !errors.Is(err, ErrOwnTraitName) {
		t.Fatalf("rename onto another trait's name: %v", err)
	}
	// Keeping its own name is fine.
	if _, err := UpdateOwnTrait(ctx, st, a.ID, OwnTraitInput{Name: "dual", Match: "name", Terms: []string{"NLSubs"}}, domain.System); err != nil {
		t.Fatalf("same name on itself: %v", err)
	}
	u, err := UpdateOwnTrait(ctx, st, a.ID, OwnTraitInput{Name: "Dual2", Match: "name", Terms: []string{"FRSubs"}}, domain.System)
	if err != nil || u.Name != "Dual2" {
		t.Fatalf("update: %+v %v", u, err)
	}
	if judgeNever(a.ID, bad) || !judgeNever(a.ID, "Dune.Part.Two.2024.1080p.WEB.H264.FRSubs-HOLYARRS") {
		t.Fatal("the judge still uses the old terms after an update")
	}
	if _, err := UpdateOwnTrait(ctx, st, 9999, OwnTraitInput{Name: "Nope", Match: "name", Terms: []string{"x"}}, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("update of a missing trait: %v", err)
	}
	list, err := OwnTraits(ctx, st)
	if err != nil || len(list) != 2 || list[0].Name != "Dual2" || list[1].Name != "Other" {
		t.Fatalf("list %+v %v", list, err)
	}

	// A Pick rule that uses the trait loses it on delete; one that does not is left alone.
	body := mediafiles.GoodEveryday()
	body.Never = append(body.Never, mediafiles.OwnTraitKey(a.ID))
	used, err := CreatePickRules(ctx, st, PickRulesInput{Name: "Uses own", Kinds: []string{"movie"}, Body: body}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePickRules(ctx, st, PickRulesInput{Name: "Plain", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); err != nil {
		t.Fatal(err)
	}
	n, err := DeleteOwnTrait(ctx, st, a.ID, domain.System)
	if err != nil || n != 1 {
		t.Fatalf("delete touched %d Pick rules, %v", n, err)
	}
	var after mediafiles.PickRules
	if err := database.Get(ctx, st.Read(), "pick_rules", used.ID, &after); err != nil {
		t.Fatal(err)
	}
	for _, k := range after.Body.Never {
		if k == mediafiles.OwnTraitKey(a.ID) {
			t.Fatalf("the deleted trait is still in the rule's never list: %v", after.Body.Never)
		}
	}
	if after.Revision != used.Revision+1 {
		t.Fatalf("revision %d, want %d", after.Revision, used.Revision+1)
	}
	if judgeNever(a.ID, "Dune.Part.Two.2024.1080p.WEB.H264.FRSubs-HOLYARRS") {
		t.Fatal("the judge still knows a deleted trait")
	}
	if list, _ := OwnTraits(ctx, st); len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list after delete %+v", list)
	}
	if _, err := DeleteOwnTrait(ctx, st, a.ID, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// A Pick rule naming a trait that is gone is refused.
	body.Never = []string{mediafiles.OwnTraitKey(a.ID)}
	if _, err := CreatePickRules(ctx, st, PickRulesInput{Name: "Stale", Kinds: []string{"movie"}, Body: body}, domain.System); err == nil {
		t.Fatal("a Pick rule naming a deleted own trait was accepted")
	}
}
