package migrate

import "testing"

// Sonarr's multi-episode styles, read as numbers (API and backup) or as
// names, land on a HolyArrs style. The two ranges write the same name:
// Range (4) is S01E01-03, our short_range; Prefixed Range (5) is
// S01E01-E03, our range.
func TestMultiEpisodeStyleFromSonarr(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{0.0, "each"}, {"extend", "each"}, {1.0, "each_dash"}, {"duplicate", "each_dash"},
		{2.0, "each"}, {"repeat", "each"}, {3.0, "each_dash"}, {"scene", "each_dash"},
		{4.0, "short_range"}, {"range", "short_range"}, {"Range", "short_range"},
		{5.0, "range"}, {"prefixedRange", "range"}, {"PrefixedRange", "range"},
	}
	for _, c := range cases {
		p := BuildPlan([]Input{{ID: 1, Snap: &Snapshot{Kind: Sonarr, Naming: map[string]any{"multiEpisodeStyle": c.in}}}}, Choices{})
		if got := item(t, p, GroupNaming, "naming:sonarr").Naming["multi_episode_style"]; got != c.want {
			t.Errorf("style %v = %q, want %q", c.in, got, c.want)
		}
	}
}
