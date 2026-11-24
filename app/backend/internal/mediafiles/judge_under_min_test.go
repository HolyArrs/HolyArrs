package mediafiles

import (
	"slices"
	"testing"
	"time"
)

// TestReplaceUnderMin: with the switch on, a file below its step's smallest
// size is worth replacing by a Release of the same step or higher; with it
// off the old apps' order stands and size plays no part.
func TestReplaceUnderMin(t *testing.T) {
	const web1080 = "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-GRP"
	const web720 = "The.Lantern.Keepers.2021.720p.WEB-DL.DDP5.1.H.264-GRP"
	cases := []struct {
		name    string
		on      bool
		curSize int64
		release string
		want    string // the better reason
	}{
		{"on, small file, same step", true, 300 * mb, web1080, "better.under_min"},
		{"off, small file, same step", false, 300 * mb, web1080, "better.good_enough_already"},
		{"on, file big enough", true, 2 * gb, web1080, "better.good_enough_already"},
		{"on, small file, lower step", true, 300 * mb, web720, "better.lower_step"},
		{"on, file size not known", true, 0, web1080, "better.good_enough_already"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rules := GoodEveryday()
			rules.ReplaceUnderMin = c.on
			cur := CurrentOf(web1080, c.curSize, time.Now().Add(-48*time.Hour), rules)
			rel := release(row{Name: c.release})
			v := Judge(rel, setFor(t, rel, &cur), rules)
			got := append(codes(v, "pass"), codes(v, "fail")...)
			if !slices.Contains(got, c.want) {
				t.Fatalf("want %s, got %v", c.want, got)
			}
		})
	}
}

func TestFileUnderMin(t *testing.T) {
	rules := GoodEveryday() // every step: 0.5 GB an hour and up
	cases := []struct {
		on      bool
		step    string
		size    int64
		runtime int
		want    bool
	}{
		{true, "1080p_web", 300 * mb, 120, true},
		{true, "1080p_web", 2 * gb, 120, false},
		{false, "1080p_web", 300 * mb, 120, false},
		{true, "1080p_web", 0, 120, false},
		{true, "1080p_web", 300 * mb, 0, false},
		{true, "", 300 * mb, 120, false},
	}
	for _, c := range cases {
		rules.ReplaceUnderMin = c.on
		if got := FileUnderMin(rules, c.step, c.size, c.runtime); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}
