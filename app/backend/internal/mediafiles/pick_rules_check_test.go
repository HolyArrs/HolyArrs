package mediafiles

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestCheckPickRules(t *testing.T) {
	video := []string{"movie", "show"}
	music := []string{"music"}
	if err := CheckPickRules(GoodEverydayName, video, GoodEveryday()); err != nil {
		t.Fatalf("Standard: %v", err)
	}
	if err := CheckPickRules(FourKName, []string{"movie"}, FourK()); err != nil {
		t.Fatalf("4K: %v", err)
	}
	if err := CheckPickRules(LosslessFirstName, music, LosslessFirst()); err != nil {
		t.Fatalf("Lossless first: %v", err)
	}
	cases := []struct {
		name  string
		rule  string
		kinds []string
		edit  func(*PickRulesBody)
		field string
	}{
		{"empty name", " ", video, nil, "name"},
		{"long name", strings.Repeat("a", 201), video, nil, "name"},
		{"no kinds", "x", nil, nil, "kinds"},
		{"four kinds", "x", []string{"movie", "show", "music", "movie"}, nil, "kinds"},
		{"unknown kind", "x", []string{"book"}, nil, "kinds"},
		{"repeated kind", "x", []string{"movie", "movie"}, nil, "kinds"},
		{"music mixed", "x", []string{"music", "movie"}, nil, "kinds"},
		{"no steps", "x", video, func(b *PickRulesBody) { b.Steps = nil }, "body.steps"},
		{"music step on video", "x", video, func(b *PickRulesBody) { b.Steps[0].Quality = "lossless" }, "body.steps"},
		{"repeated step", "x", video, func(b *PickRulesBody) { b.Steps[1].Quality = b.Steps[0].Quality }, "body.steps"},
		{"nothing allowed", "x", video, func(b *PickRulesBody) {
			for i := range b.Steps {
				b.Steps[i].Allowed = false
			}
		}, "body.steps"},
		{"good enough not listed", "x", video, func(b *PickRulesBody) { b.GoodEnoughAt = "lossless" }, "body.good_enough_at"},
		{"good enough not allowed", "x", video, func(b *PickRulesBody) { b.GoodEnoughAt = "cam" }, "body.good_enough_at"},
		{"unknown trait", "x", video, func(b *PickRulesBody) { b.Traits = append(b.Traits, TraitPoints{Trait: "glitter"}) }, "body.traits"},
		{"music trait on video", "x", video, func(b *PickRulesBody) { b.Traits = append(b.Traits, TraitPoints{Trait: "flac"}) }, "body.traits"},
		{"points too high", "x", video, func(b *PickRulesBody) { b.Traits = []TraitPoints{{Trait: "hevc", Points: pastPoints}} }, "body.traits"},
		{"repeated trait", "x", video, func(b *PickRulesBody) {
			b.Traits = []TraitPoints{{Trait: "hevc", Points: 1}, {Trait: "hevc", Points: 2}}
		}, "body.traits"},
		{"safety trait in must have", "x", video, func(b *PickRulesBody) { b.MustHave = []string{"sample"} }, "body.must_have"},
		{"safety trait in never is fine", "x", video, func(b *PickRulesBody) { b.Never = []string{"sample", "cam"} }, ""},
		{"unknown never", "x", video, func(b *PickRulesBody) { b.Never = []string{"glitter"} }, "body.never"},
		{"min points", "x", video, func(b *PickRulesBody) { b.MinPoints = -pastPoints }, "body.min_points"},
		{"upgrade points", "x", video, func(b *PickRulesBody) { b.UpgradeMinPoints = pastPoints }, "body.upgrade_min_points"},
		{"good enough points", "x", video, func(b *PickRulesBody) { n := pastPoints; b.GoodEnoughPoints = &n }, "body.good_enough_points"},
		{"size min above max", "x", video, func(b *PickRulesBody) { b.SizeLimits = map[string]SizeLimit{"*": {Min: 5, Max: 4}} }, "body.size_limits"},
		{"size negative", "x", video, func(b *PickRulesBody) { b.SizeLimits = map[string]SizeLimit{"*": {Min: -1, Max: 4}} }, "body.size_limits"},
		{"size too big", "x", video, func(b *PickRulesBody) { b.SizeLimits = map[string]SizeLimit{"*": {Max: 1<<50 + 1}} }, "body.size_limits"},
		{"size unknown step", "x", video, func(b *PickRulesBody) { b.SizeLimits = map[string]SizeLimit{"lossless": {Max: 4}} }, "body.size_limits"},
		{"prefer", "x", video, func(b *PickRulesBody) { b.Protocol.Prefer = "ftp" }, "body.protocol.prefer"},
		{"torrent wait", "x", video, func(b *PickRulesBody) { b.Protocol.WaitMinutesTorrent = 10081 }, "body.protocol.wait_minutes_torrent"},
		{"usenet wait", "x", video, func(b *PickRulesBody) { b.Protocol.WaitMinutesUsenet = -1 }, "body.protocol.wait_minutes_usenet"},
		{"language", "x", video, func(b *PickRulesBody) { b.Language.Wanted = []string{"xx"} }, "body.language.wanted"},
		{"repeated language", "x", video, func(b *PickRulesBody) { b.Language.Wanted = []string{"en", "en"} }, "body.language.wanted"},
		{"newer languages", "x", video, func(b *PickRulesBody) { b.Language.Wanted = []string{"pl", "nl", "pt", "zh", "uk"} }, ""},
		{"order", "x", video, func(b *PickRulesBody) { b.Order = "random" }, "body.order"},
		{"proper mode", "x", video, func(b *PickRulesBody) { b.ProperPolicy.Mode = "always" }, "body.proper_policy.mode"},
		{"proper age", "x", video, func(b *PickRulesBody) { b.ProperPolicy.MaxAgeDays = 36501 }, "body.proper_policy.max_age_days"},
		{"video step on music", "x", music, func(b *PickRulesBody) { b.Steps[0].Quality = "1080p_web" }, "body.steps"},
	}
	for _, c := range cases {
		b := GoodEveryday()
		if len(c.kinds) == 1 && c.kinds[0] == "music" {
			b = LosslessFirst()
		}
		if c.edit != nil {
			c.edit(&b)
		}
		err := CheckPickRules(c.rule, c.kinds, b)
		if c.field == "" {
			if err != nil {
				t.Errorf("%s: want ok, got %v", c.name, err)
			}
			continue
		}
		var fe *FieldError
		if !errors.As(err, &fe) {
			t.Errorf("%s: want a FieldError on %s, got %v", c.name, c.field, err)
			continue
		}
		if fe.Field != c.field || fe.Text == "" || fe.Error() != fe.Text {
			t.Errorf("%s: got field %q text %q, want field %q", c.name, fe.Field, fe.Text, c.field)
		}
	}
}

// pastPoints is one past the points bound (where int is 64 bits; a 32-bit
// int cannot hold it, so there it stays at the bound).
var pastPoints = int(min(int64(math.MaxInt), int64(MaxPoints)+1))
