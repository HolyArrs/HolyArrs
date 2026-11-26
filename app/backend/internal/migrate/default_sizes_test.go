package migrate

import (
	"sort"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// startingTable is a fresh app's size table, as its API answers it.
func startingTable(kind Kind) []library.OldQualitySize {
	var out []library.OldQualitySize
	for q, d := range startingSizes[kind] {
		out = append(out, library.OldQualitySize{Quality: q, Min: d.min, Max: d.max})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Quality < out[j].Quality })
	return out
}

// plainSonarr is a Sonarr with one profile that allows exactly the
// Standard preset's qualities and scores no formats.
func plainSonarr(sizes []library.OldQualitySize) *Snapshot {
	return &Snapshot{Kind: Sonarr, Address: "http://sonarr:8989", QualitySizes: sizes,
		QualityProfiles: []Profile{{ID: 4, Name: "HD-720p/1080p", Cutoff: "WEBDL-1080p", UpgradeAllowed: true,
			Qualities:       []string{"HDTV-720p", "WEBDL-720p", "Bluray-720p", "HDTV-1080p", "WEBDL-1080p", "Bluray-1080p"},
			CutoffQualities: []string{"WEBDL-1080p"}}}}
}

// An untouched size table keeps the ready-made preset; a changed one
// comes over as Pick rules named after the profile, with its sizes.
func TestChangedSizeTableMakesOwnRules(t *testing.T) {
	p := BuildPlan([]Input{{1, plainSonarr(startingTable(Sonarr))}}, Choices{})
	it := item(t, p, GroupRules, "pick_rules:1:sonarr:4")
	if it.Rules.Body != nil || it.Rules.Preset != "Standard" {
		t.Fatalf("untouched table: want the Standard preset, got %+v", it)
	}

	sizes := startingTable(Sonarr)
	for i := range sizes {
		if sizes[i].Quality == "WEBDL-1080p" {
			sizes[i].Min, sizes[i].Max = 10, 60
		}
	}
	p = BuildPlan([]Input{{1, plainSonarr(sizes)}}, Choices{})
	it = item(t, p, GroupRules, "pick_rules:1:sonarr:4")
	if it.Rules.Body == nil || it.Rules.Name != "HD-720p/1080p" {
		t.Fatalf("changed table: want own Pick rules, got %+v", it)
	}
	l := it.Rules.Body.SizeLimits["1080p_web"]
	const mib = 1 << 20
	if !l.PerMinute || l.Min != 4*mib || l.Max != 130*mib {
		t.Fatalf("1080p_web limit %+v (WEBRip-1080p keeps the wider range)", l)
	}
	if _, ok := it.Rules.Body.SizeLimits["*"]; ok {
		t.Fatal("the preset's one range should not stay")
	}
}

func TestSizesChanged(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		rows []library.OldQualitySize
		want bool
	}{
		{"fresh radarr", Radarr, startingTable(Radarr), false},
		{"fresh sonarr", Sonarr, startingTable(Sonarr), false},
		{"no table", Radarr, nil, false},
		{"renamed row", Radarr, []library.OldQualitySize{{Quality: "My 1080p", Min: 9, Max: 9}}, false},
		{"radarr max set", Radarr, []library.OldQualitySize{{Quality: "Bluray-1080p", Min: 0, Max: 400}}, true},
		{"sonarr min raised", Sonarr, []library.OldQualitySize{{Quality: "HDTV-720p", Min: 5, Max: 125}}, true},
		{"rounding", Sonarr, []library.OldQualitySize{{Quality: "Unknown", Min: 1, Max: 199.90001}}, false},
	}
	for _, c := range cases {
		if got := sizesChanged(c.kind, c.rows); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
