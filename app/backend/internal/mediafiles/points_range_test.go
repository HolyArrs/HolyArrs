package mediafiles

import (
	"math"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Points take the full 32-bit range the old apps keep scores in: the
// bound itself passes on every field, one past it is refused.
func TestCheckPickRules_PointsRange(t *testing.T) {
	b := GoodEveryday()
	b.Traits = []TraitPoints{{Trait: "hevc", Points: MaxPoints}, {Trait: "av1", Points: -MaxPoints}, {Trait: "remux", Points: 943000}}
	b.MinPoints, b.UpgradeMinPoints = -MaxPoints, MaxPoints
	g := MaxPoints
	b.GoodEnoughPoints = &g
	if err := CheckPickRules("x", []string{"movie"}, b); err != nil {
		t.Fatalf("bound refused: %v", err)
	}
	if math.MaxInt == MaxPoints {
		return // int is 32 bits here: one past the bound cannot be held at all
	}
	past := MaxPoints
	past++
	for name, set := range map[string]func(*PickRulesBody){
		"trait":       func(b *PickRulesBody) { b.Traits[0].Points = past },
		"min":         func(b *PickRulesBody) { b.MinPoints = -past },
		"upgrade":     func(b *PickRulesBody) { b.UpgradeMinPoints = past },
		"good enough": func(b *PickRulesBody) { b.GoodEnoughPoints = &past },
	} {
		c := GoodEveryday()
		c.Traits = []TraitPoints{{Trait: "hevc", Points: 1}}
		set(&c)
		err := CheckPickRules("x", []string{"movie"}, c)
		if err == nil || !strings.Contains(err.Error(), "-2147483647 to 2147483647") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Many large scores add up without wrapping around, stopping at the bound
// either way, and the upgrade gain between two large totals stays right.
func TestSumPoints_NoOverflow(t *testing.T) {
	rules := GoodEveryday()
	rules.Traits = []TraitPoints{{Trait: "hevc", Points: MaxPoints}, {Trait: "web_dl", Points: MaxPoints}, {Trait: "ddp_audio", Points: MaxPoints}}
	p := releasename.Parse("Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.265-GRP", hintsFor(rules))
	if got := sumPoints(p, rules); got != MaxPoints {
		t.Fatalf("sum up: %d", got)
	}
	for i := range rules.Traits {
		rules.Traits[i].Points = -MaxPoints
	}
	if got := sumPoints(p, rules); got != -MaxPoints {
		t.Fatalf("sum down: %d", got)
	}
	for i := range rules.Traits {
		rules.Traits[i].Points = 943000
	}
	if got := sumPoints(p, rules); got != 3*943000 {
		t.Fatalf("plain sum: %d", got)
	}
	if !gainOver(MaxPoints, -MaxPoints, 1) || gainOver(-MaxPoints, MaxPoints, 1) || !gainOver(944000, 943000, 1000) || gainOver(943999, 943000, 1000) {
		t.Fatal("gain")
	}
}
