package migrate

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// formatsSnapshot is a Radarr profile scoring three formats: one plain
// pattern, one with a language and a year test, one with an indexer flag
// (which HolyArrs cannot test).
func withFormats(radarr, _, _ *Snapshot) {
	radarr.Address = "http://radarr:7878"
	radarr.CustomFormats = []Named{
		{ID: 1, Name: "x265", Items: []map[string]any{{"name": "x265", "kind": "ReleaseTitleSpecification", "value.value": `[xh][ ._-]?265|\bHEVC\b`, "required": true}}},
		{ID: 2, Name: "German 90s", Items: []map[string]any{
			{"name": "German", "kind": "LanguageSpecification", "value.value": float64(4), "language": "German"},
			{"name": "90s", "kind": "YearSpecification", "value.min": float64(1990), "value.max": float64(1999)}}},
		{ID: 3, Name: "Freeleech", Items: []map[string]any{{"name": "FL", "kind": "IndexerFlagSpecification", "value.value": float64(1)}}},
		{ID: 4, Name: "Unscored", Items: []map[string]any{{"name": "x", "kind": "ReleaseTitleSpecification", "value.value": "x"}}},
	}
	radarr.QualityProfiles = []Profile{{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p", UpgradeAllowed: true,
		Qualities:       []string{"WEBDL-2160p", "Bluray-2160p", "Remux-2160p"},
		CutoffQualities: []string{"Remux-2160p"},
		MinScore:        -100, CutoffScore: 20000, MinUpgradeScore: 10,
		Scores: map[string]int{"x265": 150, "German 90s": -500, "Freeleech": 20}}}
}

// TestPlanBringsFormats: a scored profile becomes its own Pick rules with
// a trait per format it can test, the same points (cut to the bound), and
// its score settings; a format with a test kind HolyArrs lacks is listed.
func TestPlanBringsFormats(t *testing.T) {
	radarr := &Snapshot{Kind: Radarr}
	withFormats(radarr, nil, nil)
	p := BuildPlan([]Input{{1, radarr}}, Choices{})
	it := item(t, p, GroupRules, "pick_rules:1:radarr:2")
	rp := it.Rules
	if rp.Body == nil || rp.Name != "Ultra-HD" || it.Detail != "becomes Pick rules Ultra-HD · 2 traits" {
		t.Fatalf("rules item %+v", it)
	}
	b := rp.Body
	var allowed []string
	for _, s := range b.Steps {
		if s.Allowed {
			allowed = append(allowed, s.Quality)
		}
	}
	if !slices.Equal(allowed, []string{"2160p_web", "2160p_bluray", "2160p_remux"}) || b.GoodEnoughAt != "2160p_remux" {
		t.Fatalf("steps %v good %s", allowed, b.GoodEnoughAt)
	}
	if b.MinPoints != -100 || *b.GoodEnoughPoints != 10000 || b.UpgradeMinPoints != 10 || b.Order != "quality_then_points" {
		t.Fatalf("points %d %d %d %s", b.MinPoints, *b.GoodEnoughPoints, b.UpgradeMinPoints, b.Order)
	}
	got := map[string]int{}
	for _, tr := range rp.Traits {
		got[tr.Name] = tr.Points
		if !strings.HasPrefix(tr.SourceID, "radarr:radarr:7878:") {
			t.Errorf("source id %q", tr.SourceID)
		}
	}
	if len(got) != 2 || got["x265"] != 150 || got["German 90s"] != -500 {
		t.Fatalf("traits %v", got)
	}
	for _, tr := range rp.Traits {
		if tr.Name == "German 90s" && (tr.Specs[0].Kind != "language" || tr.Specs[0].Value != "de" || tr.Specs[1].Kind != "year" || tr.Specs[1].Value != "1990-1999") {
			t.Fatalf("German 90s specs %+v", tr.Specs)
		}
	}
	if len(rp.Skipped) != 1 || rp.Skipped[0].Name != "Freeleech" || !strings.Contains(rp.Skipped[0].Reason, "indexer flag") {
		t.Fatalf("skipped %+v", rp.Skipped)
	}
	if n, ok := notMoved(p, "radarr custom_formats[3]"); !ok || !strings.Contains(n.Why, "nothing in the old app gives it points") {
		t.Fatalf("unscored format %+v %v", n, ok)
	}
	// Turned off, the profile is a ready-made match as before.
	off := item(t, BuildPlan([]Input{{1, radarr}}, Choices{NoFormats: true}), GroupRules, "pick_rules:1:radarr:2")
	if off.Rules.Body != nil || off.Rules.Preset != "4K" {
		t.Fatalf("formats off %+v", off.Rules)
	}
}

// TestProfileUpgradeOrder: quality first, then points, then the least
// gain, as the old apps check an upgrade.
func TestProfileUpgradeOrder(t *testing.T) {
	radarr := &Snapshot{Kind: Radarr}
	withFormats(radarr, nil, nil)
	b := *item(t, BuildPlan([]Input{{1, radarr}}, Choices{}), GroupRules, "pick_rules:1:radarr:2").Rules.Body
	b.GoodEnoughPoints = nil
	b.GoodEnoughAt = "2160p_remux"
	b.Traits = []mediafiles.TraitPoints{{Trait: "hevc", Points: 150}}
	b.UpgradeMinPoints = 200
	cases := []struct {
		name, cur, rel string
		want           bool
	}{
		{"a higher step wins with fewer points", "Film.1999.2160p.WEB-DL.H265-X", "Film.1999.2160p.BluRay.Remux.H264-Y", true},
		{"same step, a gain under the least step", "Film.1999.2160p.BluRay.Remux.H264-X", "Film.1999.2160p.BluRay.Remux.H265-Y", false},
		{"a lower step never wins on points", "Film.1999.2160p.BluRay.Remux.H264-X", "Film.1999.2160p.WEB-DL.H265-Y", false},
	}
	for _, c := range cases {
		cur := mediafiles.CurrentOf(c.cur, 0, time.Time{}, b)
		p := releasename.Parse(c.rel, releasename.Hints{})
		if got := mediafiles.Gains(mediafiles.ReleaseFacts{Name: c.rel, Parse: p}, b, &cur); got != c.want {
			t.Errorf("%s: %v, want %v (cur %+v)", c.name, got, c.want, cur)
		}
	}
}

// TestMoveImportsFormatsOnce: the move makes the Pick rules and traits,
// the title uses them, a second read of the same app updates in place
// (no copies), and undo removes them.
func TestMoveImportsFormatsOnce(t *testing.T) {
	l := newMoveLabWith(t, withFormats)
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "pick_rules:"+strconv.FormatInt(l.snaps[0], 10)+":radarr:2"); got.Result != ResCreated {
		t.Fatalf("move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM own_traits WHERE source = 'import'"); n != 2 {
		t.Fatalf("%d imported traits", n)
	}
	var pr []mediafiles.PickRules
	pr, err := database.Select[mediafiles.PickRules](context.Background(), l.st.Read(), "pick_rules", "name = 'Ultra-HD'")
	if err != nil || len(pr) != 1 || len(pr[0].Body.Traits) != 2 || pr[0].Body.Synced == nil || pr[0].Body.Synced.From != "import" {
		t.Fatalf("pick rules %+v %v", pr, err)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE pick_rules_id = ?", pr[0].ID); n == 0 {
		t.Fatal("the title does not use the imported Pick rules")
	}
	// Moving again brings nothing twice.
	l.move(t, Choices{})
	if n := l.count(t, "SELECT count(*) FROM own_traits WHERE source = 'import'"); n != 2 {
		t.Fatalf("after a second move %d traits", n)
	}
	if n := l.count(t, "SELECT count(*) FROM pick_rules WHERE json_extract(body, '$.synced.from') = 'import'"); n != 1 {
		t.Fatalf("after a second move %d imported Pick rules", n)
	}
	// Undo: a title the move added stays in the Library as removed, so the
	// Pick rules it used stay, and so do their traits.
	u := l.undo(t, mv.ID)
	if u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	key := "pick_rules:" + strconv.FormatInt(l.snaps[0], 10) + ":radarr:2"
	if ln := lineOf(t, u.Report, key); ln.Result != ResKept {
		t.Fatalf("undo of the Pick rules %+v", ln)
	}
	if n := l.count(t, "SELECT count(*) FROM own_traits WHERE source = 'import'"); n != 2 {
		t.Fatalf("after undo %d traits", n)
	}
}

// TestUndoRemovesUnusedImport: with nothing using them, undo removes the
// imported Pick rules and their traits.
func TestUndoRemovesUnusedImport(t *testing.T) {
	l := newMoveLabWith(t, func(r, b, s *Snapshot) {
		withFormats(r, b, s)
		r.Titles = nil
	})
	mv := l.move(t, Choices{})
	if n := l.count(t, "SELECT count(*) FROM own_traits WHERE source = 'import'"); n != 2 {
		t.Fatalf("%d imported traits", n)
	}
	if u := l.undo(t, mv.ID); u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	if n := l.count(t, "SELECT count(*) FROM own_traits WHERE source = 'import'") + l.count(t, "SELECT count(*) FROM pick_rules WHERE name = 'Ultra-HD'"); n != 0 {
		t.Fatalf("after undo %d rows left", n)
	}
}
