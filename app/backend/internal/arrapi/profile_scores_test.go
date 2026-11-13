package arrapi

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Tiered scores as Profilarr sends them (hundreds of thousands) become the
// same points, and saving the set unchanged from the Pick rules screen
// keeps every one, so the list still answers the scores as sent.
func TestProfiles_LargeScoresKeptThroughSave(t *testing.T) {
	h := newHookRig(t)
	want := map[string]int{"Tier 1": 943000, "Tier 2": 861000, "Banned": -999999}
	fmts, names := map[int64]int{}, map[int64]string{}
	for n, score := range want {
		st, out := h.do("radarr", "POST", "customformat", formatBody(0, n, false, spec(n, "ReleaseGroupSpecification", false, false, "value", n[:4])))
		id := int64(decodeObj(t, st, out, 201)["id"].(float64))
		fmts[id], names[id] = score, n
	}
	st, out := h.do("radarr", "POST", "qualityprofile", radarrProfile(0, "1080p Compact", fmts, names))
	id := int64(decodeObj(t, st, out, 201)["id"].(float64))

	points := func() map[string]int {
		got := map[string]int{}
		for _, tp := range h.rules(id).Body.Traits {
			for fid, n := range names {
				if tp.Trait == mediafiles.OwnTraitKey(h.formatTrait("radarr", fid)) {
					got[n] = tp.Points
				}
			}
		}
		return got
	}
	check := func(when string) {
		t.Helper()
		got := points()
		if len(got) != len(want) {
			t.Fatalf("%s: traits %v", when, got)
		}
		for n, w := range want {
			if got[n] != w {
				t.Fatalf("%s: %s has %d points, want %d", when, n, got[n], w)
			}
		}
	}
	check("after sync")

	// The screen sends the set back as it read it.
	p := h.rules(id)
	if _, err := library.UpdatePickRules(context.Background(), h.st, id, library.PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: p.Body}, database.Actor{Kind: "user", ID: 1}); err != nil {
		t.Fatalf("save unchanged: %v", err)
	}
	check("after save")
	for _, f := range byName(h.list("radarr", "qualityprofile"), "1080p Compact")["formatItems"].([]any) {
		m := f.(map[string]any)
		if n := names[int64(m["format"].(float64))]; n != "" && int(m["score"].(float64)) != want[n] {
			t.Fatalf("list: %s scored %v", n, m["score"])
		}
	}
}
