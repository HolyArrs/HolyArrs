package mediafiles

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Group names come from real release titles; the match is the whole group,
// in any case, never a part of it.
func TestGroupBlocked(t *testing.T) {
	tests := []struct {
		name    string
		release string
		blocked []string
		want    bool
	}{
		{"exact", "The.Last.of.Us.S01E01.1080p.WEB.H264-CAKES", []string{"CAKES"}, true},
		{"any case", "Andor.S02E03.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", []string{"flux"}, true},
		{"bracket group", "[SubsPlease] Frieren - 05 (1080p) [3A1B2C3D].mkv", []string{"subsplease"}, true},
		{"spaces around entry", "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.Atmos.H.264-FLUX", []string{"  FLUX "}, true},
		{"one of many", "Oppenheimer.2023.1080p.BluRay.x264-SPARKS", []string{"YIFY", "sparks"}, true},
		{"longer group is not a match", "Shogun.2024.S01E01.1080p.WEB.h264-ETHEL", []string{"ETH"}, false},
		{"shorter group is not a match", "The.Bear.S03E01.1080p.WEB.h264-ETHEL", []string{"ETHELS"}, false},
		{"title word is not the group", "Fargo.S05E01.1080p.WEB.H264-CAKES", []string{"Fargo"}, false},
		{"empty list", "Severance.S02E01.1080p.WEB.H264-SuccessfulCrab", nil, false},
		{"empty entry", "Severance.S02E01.1080p.WEB.H264-SuccessfulCrab", []string{""}, false},
		{"no group in the name", "Some Movie 2024 1080p", []string{""}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := releasename.Parse(tc.release, releasename.Hints{})
			if got := GroupBlocked(tc.blocked, p.Group.Value); got != tc.want {
				t.Fatalf("GroupBlocked(%q, group %q) = %v, want %v", tc.blocked, p.Group.Value, got, tc.want)
			}
		})
	}
}

func TestAddBlockedGroup(t *testing.T) {
	tests := []struct {
		name  string
		have  []string
		group string
		want  []string
		added bool
	}{
		{"new", nil, "FLUX", []string{"FLUX"}, true},
		{"same in other case", []string{"flux"}, "FLUX", []string{"flux"}, false},
		{"trimmed", []string{"CAKES"}, " SPARKS ", []string{"CAKES", "SPARKS"}, true},
		{"empty", []string{"CAKES"}, " ", []string{"CAKES"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, added := AddBlockedGroup(tc.have, tc.group)
			if added != tc.added || strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("AddBlockedGroup(%q, %q) = %q, %v; want %q, %v", tc.have, tc.group, got, added, tc.want, tc.added)
			}
		})
	}
}

func TestJudgeRefusesBlockedGroup(t *testing.T) {
	tests := []struct {
		release string
		blocked []string
		refused bool
	}{
		{"Example.Movie.2025.1080p.WEB-DL.H265-FLUX", []string{"flux"}, true},
		{"Example.Movie.2025.1080p.WEB-DL.H265-FLUXX", []string{"flux"}, false},
		{"Example.Movie.2025.1080p.WEB-DL.H265-FLUX", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.release, func(t *testing.T) {
			rules := GoodEveryday()
			rules.BlockedGroups = tc.blocked
			title := TitleRef{ID: 1, Name: "Example Movie", Year: 2025}
			p := releasename.Parse(tc.release, releasename.Hints{})
			v := Judge(ReleaseFacts{Name: tc.release, Parse: p, Size: 4 << 30}, SlotSet{
				Match: Match{Title: &title, Conf: releasename.Sure, Detail: "test match"}, SlotIDs: []int64{1}, RuntimeMinutes: 120, Out: true,
			}, rules)
			if got := hasFail(v, "hard", "blocked_group"); got != tc.refused {
				t.Fatalf("blocked_group refusal = %v, want %v; reasons %+v", got, tc.refused, v.Reasons)
			}
		})
	}
}

func TestCheckBlockedGroups(t *testing.T) {
	ok := GoodEveryday()
	ok.BlockedGroups = []string{"FLUX", "CAKES"}
	if err := CheckPickRules("x", []string{"movie"}, ok); err != nil {
		t.Fatalf("good list refused: %v", err)
	}
	for name, list := range map[string][]string{
		"empty":    {" "},
		"repeated": {"FLUX", "flux"},
		"too long": {strings.Repeat("a", 201)},
	} {
		b := GoodEveryday()
		b.BlockedGroups = list
		if err := CheckPickRules("x", []string{"movie"}, b); err == nil {
			t.Fatalf("%s list was taken", name)
		}
	}
}
