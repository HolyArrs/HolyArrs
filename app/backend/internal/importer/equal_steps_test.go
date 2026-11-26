package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Steps the rules rank as one (equal_steps) must not refuse each other at
// import, the same way the pick check lets either replace the other.
func TestLieOfHonoursEqualSteps(t *testing.T) {
	grouped := mediafiles.GoodEveryday()
	grouped.EqualSteps = [][]string{{"1080p_web", "1080p_bluray"}}
	plain := mediafiles.GoodEveryday()
	const web = "City.of.Darkness.2024.1080p.AMZN.WEB-DL.DDP5.1.H264-R3MiX"
	const bluray = "City.of.Darkness.2024.1080p.BluRay.x264-YTS"
	const dvd = "City.of.Darkness.2024.DVDRip.x264-GRP"
	cases := []struct {
		name, file, old string
		rules           mediafiles.PickRulesBody
		refused         bool
	}{
		{"web over bluray, grouped", web, bluray, grouped, false},
		{"bluray over web, grouped", bluray, web, grouped, false},
		{"web over bluray, not grouped", web, bluray, plain, true},
		{"dvd over bluray, grouped", dvd, bluray, grouped, true},
		{"bluray over dvd, grouped", bluray, dvd, grouped, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			why := lieOf(c.file, c.file, true, c.old, c.rules, nil)
			if got := strings.Contains(why, "lower than the copy in place"); got != c.refused {
				t.Fatalf("refused=%v, want %v (%q)", got, c.refused, why)
			}
		})
	}
}
