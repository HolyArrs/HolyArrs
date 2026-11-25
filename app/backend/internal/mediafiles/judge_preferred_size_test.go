package mediafiles

import (
	"testing"
)

// TestPreferredSizeBreaksTie: two Releases equal on everything else; the one
// closer to the step's preferred size wins, and without a preferred size the
// one closer to the middle of the band wins, as before.
func TestPreferredSizeBreaksTie(t *testing.T) {
	const a = "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-GRPA"
	const b = "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.264-GRPB"
	perMin := func(gbHour float64) int64 { return int64(gbHour * float64(gb) / 60) }
	cases := []struct {
		name      string
		preferred int64
		winner    string
	}{
		{"preferred 4 GB an hour: the bigger copy", perMin(4), a},
		{"no preferred: the copy nearer the middle", 0, b},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rules := GoodEveryday()
			rules.SizeLimits = map[string]SizeLimit{"*": {Min: perMin(0.5), Max: perMin(6), Preferred: c.preferred, PerMinute: true}}
			ra := release(row{Name: a, Size: 7800 << 20})
			rb := release(row{Name: b, Size: 6400 << 20})
			va, vb := Judge(ra, setFor(t, ra, nil), rules), Judge(rb, setFor(t, rb, nil), rules)
			if !va.Passed || !vb.Passed {
				t.Fatalf("both should pass:\n%s\n%s", dump(va), dump(vb))
			}
			vs := []Verdict{va, vb}
			Rank([]ReleaseFacts{ra, rb}, vs, rules)
			loser := vs[1]
			if c.winner == b {
				loser = vs[0]
			}
			if loser.Outcome != "passed_over" {
				t.Fatalf("want %s to win:\n%s\n%s", c.winner, dump(vs[0]), dump(vs[1]))
			}
		})
	}
}

func TestPreferredSizeInsideBand(t *testing.T) {
	cases := []struct {
		l  SizeLimit
		ok bool
	}{
		{SizeLimit{Min: 10, Max: 100, Preferred: 50}, true},
		{SizeLimit{Min: 10, Max: 100}, true},
		{SizeLimit{Min: 10, Max: 100, Preferred: 5}, false},
		{SizeLimit{Min: 10, Max: 100, Preferred: 101}, false},
		{SizeLimit{Min: 10, Max: 1 << 50, Preferred: 500}, true}, // no largest size
		{SizeLimit{Min: 10, Max: 100, Preferred: -1}, false},
	}
	for _, c := range cases {
		b := GoodEveryday()
		b.SizeLimits = map[string]SizeLimit{"*": c.l}
		if err := CheckPickRules("Mine", []string{"movie"}, b); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.l, err)
		}
	}
}
