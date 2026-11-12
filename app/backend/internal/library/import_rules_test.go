package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestImportPickRulesAgain: importing the same profile again updates the
// Pick rules and traits in place (keeping the tags set here), a name taken
// by other Pick rules gets the app added, and imported traits keep their
// tests read-only.
func TestImportPickRulesAgain(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	if _, err := CreatePickRules(ctx, st, PickRulesInput{Name: "HD", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); err != nil {
		t.Fatal(err)
	}
	in := ImportRulesInput{App: "radarr", ProfileKey: "radarr:host:1", Name: "HD", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday(),
		Traits: []ImportedTrait{{SourceID: "radarr:host:5", Name: "x265", Specs: []mediafiles.OwnSpec{{Name: "x265", Kind: "name", Value: "x265"}}, Points: 50}}}
	first, err := ImportPickRules(ctx, st, in, domain.System)
	if err != nil || !first.Created || first.PickRules.Name != "HD (radarr)" || len(first.NewTraits) != 1 {
		t.Fatalf("first %+v %v", first, err)
	}
	b := first.PickRules.Body
	b.Tags = []string{"4k"}
	if _, err := UpdatePickRules(ctx, st, first.PickRules.ID, PickRulesInput{Name: first.PickRules.Name, Kinds: first.PickRules.Kinds, Body: b}, domain.System); err != nil {
		t.Fatal(err)
	}
	in.Traits[0].Points = 80
	again, err := ImportPickRules(ctx, st, in, domain.System)
	if err != nil || again.Created || again.PickRules.ID != first.PickRules.ID || len(again.NewTraits) != 0 || again.Updated != 1 {
		t.Fatalf("again %+v %v", again, err)
	}
	if tags := again.PickRules.Body.Tags; len(tags) != 1 || tags[0] != "4k" || again.PickRules.Body.Traits[0].Points != 80 {
		t.Fatalf("again body %+v", again.PickRules.Body)
	}
	ts, _ := database.Select[mediafiles.OwnTrait](ctx, st.Read(), "own_traits", "source = 'import'")
	if len(ts) != 1 {
		t.Fatalf("%d imported traits", len(ts))
	}
	if _, err := UpdateOwnTrait(ctx, st, ts[0].ID, OwnTraitInput{Name: "X", Match: "name", Terms: []string{"x"}}, domain.System); err == nil {
		t.Fatal("an imported trait's tests were changed")
	}
}
