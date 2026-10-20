package explore

import (
	"testing"
	"time"
)

// Monday 19 Oct 2026, mid-afternoon.
var freshNow = time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)

func ago(days int) time.Time { return freshNow.AddDate(0, 0, -days) }

func text(b *Fresh) string {
	if b == nil {
		return ""
	}
	return b.Text
}

func TestFreshRules(t *testing.T) {
	cases := []struct {
		name string
		f    FreshFacts
		want string
	}{
		{"new episode", FreshFacts{Kind: "show", InLibrary: true, LastEpisode: ago(6)}, "New Episode"},
		{"episode too old", FreshFacts{Kind: "show", InLibrary: true, LastEpisode: ago(7)}, ""},
		{"episode not in library", FreshFacts{Kind: "show", LastEpisode: ago(1)}, ""},
		{"new season", FreshFacts{Kind: "show", SeasonStart: ago(13)}, "New Season"},
		{"season too old", FreshFacts{Kind: "show", SeasonStart: ago(14)}, ""},
		{"recently added", FreshFacts{Kind: "movie", InLibrary: true, Added: ago(6)}, "Recently Added"},
		{"added too old", FreshFacts{Kind: "movie", InLibrary: true, Added: ago(8)}, ""},
		{"top 10", FreshFacts{Kind: "movie", Top10Rank: 3}, "Top 10"},
		{"rank 11", FreshFacts{Kind: "movie", Top10Rank: 11}, ""},
		{"coming this week", FreshFacts{Kind: "movie", Released: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)}, "Coming Friday"},
		{"coming in 7 days", FreshFacts{Kind: "movie", Released: freshNow.AddDate(0, 0, 7)}, "Coming Monday"},
		{"premieres later", FreshFacts{Kind: "show", Released: time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)}, "Premieres Nov 24"},
		{"season coming", FreshFacts{Kind: "show", NextSeason: freshNow.AddDate(0, 2, 0), NextSeasonNumber: 3}, "Season 3 coming"},
		{"season date passed", FreshFacts{Kind: "show", NextSeason: ago(1), NextSeasonNumber: 3}, ""},
		{"new release", FreshFacts{Kind: "movie", Released: ago(29)}, "New Release"},
		{"release too old", FreshFacts{Kind: "movie", Released: ago(30)}, ""},
		{"show is no release", FreshFacts{Kind: "show", Released: ago(3)}, ""},
		{"music added", FreshFacts{Kind: "album", Music: true, InLibrary: true, Added: ago(2)}, "Recently Added"},
		{"music release", FreshFacts{Kind: "album", Music: true, Released: ago(2)}, "New Release"},
		{"music no top 10", FreshFacts{Kind: "album", Music: true, Top10Rank: 1}, ""},
		{"nothing", FreshFacts{Kind: "movie"}, ""},
	}
	for _, c := range cases {
		if got := text(PickFresh(c.f, freshNow)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFreshPriority(t *testing.T) {
	all := FreshFacts{Kind: "show", InLibrary: true, LastEpisode: ago(1), SeasonStart: ago(2), Added: ago(1), Top10Rank: 1,
		Released: freshNow.AddDate(0, 0, 3), NextSeason: freshNow.AddDate(0, 1, 0), NextSeasonNumber: 4}
	steps := []struct {
		drop func(*FreshFacts)
		want string
	}{
		{func(*FreshFacts) {}, "New Episode"},
		{func(f *FreshFacts) { f.LastEpisode = time.Time{} }, "New Season"},
		{func(f *FreshFacts) { f.SeasonStart = time.Time{} }, "Recently Added"},
		{func(f *FreshFacts) { f.Added = time.Time{} }, "Top 10"},
		{func(f *FreshFacts) { f.Top10Rank = 0 }, "Coming Thursday"},
		{func(f *FreshFacts) { f.Released = time.Time{} }, "Season 4 coming"},
	}
	f := all
	for _, s := range steps {
		s.drop(&f)
		if got := text(PickFresh(f, freshNow)); got != s.want {
			t.Fatalf("got %q, want %q", got, s.want)
		}
	}
	m := FreshFacts{Kind: "movie", Top10Rank: 2, Released: ago(3)}
	if b := PickFresh(m, freshNow); b == nil || b.Kind != FreshTop10 || b.Rank != 2 {
		t.Fatalf("Top 10 outranks New Release: %+v", b)
	}
}

func TestParseDay(t *testing.T) {
	for _, s := range []string{"2026-10-19", "2026-10-19T08:00:00.000000000Z", "2026-10-19T08:00:00Z"} {
		if d := ParseDay(s); d.Year() != 2026 || d.Day() != 19 {
			t.Errorf("%s: %v", s, d)
		}
	}
	if !ParseDay("").IsZero() || !ParseDay("soon").IsZero() {
		t.Error("bad dates must be zero")
	}
}
