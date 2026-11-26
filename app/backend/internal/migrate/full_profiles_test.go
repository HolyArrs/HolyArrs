package migrate

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// TestPlanBringsUnscoredProfileWithSizes: a profile that scores no
// formats still becomes its own Pick rules with exactly its qualities and
// cutoff, and the app's size table comes along per step.
func TestPlanBringsUnscoredProfileWithSizes(t *testing.T) {
	sonarr := &Snapshot{Kind: Sonarr, Address: "http://sonarr:8989"}
	sonarr.QualityProfiles = []Profile{{ID: 4, Name: "HD-720p only", Cutoff: "WEBDL-720p", UpgradeAllowed: true,
		Qualities: []string{"HDTV-720p", "WEBDL-720p"}, CutoffQualities: []string{"WEBDL-720p"}}}
	sonarr.QualitySizes = []library.OldQualitySize{
		{Quality: "HDTV-720p", Min: 3, Max: 0}, {Quality: "WEBDL-720p", Min: 5, Max: 40},
		{Quality: "WEBDL-1080p", Min: 0, Max: 0}, {Quality: "Unknown", Min: 1, Max: 2},
	}
	p := BuildPlan([]Input{{1, sonarr}}, Choices{})
	it := item(t, p, GroupRules, "pick_rules:1:sonarr:4")
	b := it.Rules.Body
	if b == nil || it.Rules.Name != "HD-720p only" {
		t.Fatalf("rules item %+v", it)
	}
	var allowed []string
	for _, s := range b.Steps {
		if s.Allowed {
			allowed = append(allowed, s.Quality)
		}
	}
	if len(allowed) == 0 || slices.ContainsFunc(allowed, func(s string) bool { return s[:4] == "1080" || s[:4] == "2160" }) {
		t.Fatalf("allowed %v", allowed)
	}
	const mib = 1 << 20
	for step, l := range b.SizeLimits {
		if !l.PerMinute || l.Max != 0 && l.Max < l.Min {
			t.Fatalf("limit %s %+v", step, l)
		}
	}
	if l := b.SizeLimits[library.QualityStep("WEBDL-720p")]; l.Min != 5*mib || l.Max != 40*mib {
		t.Fatalf("720p web %+v", l)
	}
	if l := b.SizeLimits[library.QualityStep("HDTV-720p")]; l.Min != 3*mib || l.Max < 1<<40 {
		t.Fatalf("720p tv %+v", l)
	}
	if l, ok := b.SizeLimits[library.QualityStep("WEBDL-1080p")]; !ok || l.Min != 0 || l.Max != 0 {
		t.Fatalf("1080p web %+v %v", l, ok)
	}
	for _, n := range it.Rules.Notes {
		if n == "its size limits are not brought; set them under Pick rules" {
			t.Fatalf("notes %v", it.Rules.Notes)
		}
	}
}

// TestPlanNoFormatsKeepsPresetMatch: owners who turn format import off
// still get the ready-made match.
func TestPlanNoFormatsKeepsPresetMatch(t *testing.T) {
	sonarr := &Snapshot{Kind: Sonarr}
	sonarr.QualityProfiles = []Profile{{ID: 4, Name: "HD", Cutoff: "WEBDL-720p", Qualities: []string{"WEBDL-720p"}}}
	p := BuildPlan([]Input{{1, sonarr}}, Choices{NoFormats: true})
	if it := item(t, p, GroupRules, "pick_rules:1:sonarr:4"); it.Rules.Body != nil {
		t.Fatalf("own rules with NoFormats %+v", it.Rules)
	}
}
