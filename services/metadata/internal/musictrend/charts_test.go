package musictrend

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestChartFormulas(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if !near(Popularity(1, 4), 1) || !near(Popularity(4, 4), 0.25) || Popularity(1, 0) != 0 {
		t.Fatal("popularity")
	}
	if Freshness("", 14, now) != 0 || Freshness("bad-date-x", 14, now) != 0 {
		t.Fatal("unknown date must be 0")
	}
	if !near(Freshness("2026-11-01", 14, now), 1) || !near(Freshness("2026-10-22", 7, now), 1) {
		t.Fatal("future or today must be 1")
	}
	if !near(Freshness("2026-10-08", 14, now), math.Exp(-1)) {
		t.Fatal("14 days at t=14")
	}
	if !near(RankGain(0, 1, 10), 1) || RankGain(3, 5, 10) != 0 || !near(RankGain(5, 3, 10), 0.2) || RankGain(1, 1, 0) != 0 {
		t.Fatal("rank gain")
	}
	if GenreSlug("Hip-Hop/Rap") != "hip-hop-rap" || GenreSlug("R&B/Soul") != "randb-soul" || GenreSlug("Música Mexicana") != "musica-mexicana" {
		t.Fatal(GenreSlug("Hip-Hop/Rap"), GenreSlug("R&B/Soul"), GenreSlug("Música Mexicana"))
	}
	if h := GenreHue("pop"); h < 0 || h >= GenreHues || h != GenreHue("pop") {
		t.Fatal("hue")
	}
}

func chartService(t *testing.T) *Service {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "m.sqlite"), Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestChartsGenreRanksAndHistory(t *testing.T) {
	s := chartService(t)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	week := []Entry{{Name: "Old", Artist: "X", Genres: []string{"Rock"}}, {Name: "Riser", Artist: "Y", Genres: []string{"Pop"}}}
	s.keepRanks("us", chart{albums: week}, now.AddDate(0, 0, -7))
	s.keepRanks("us", chart{albums: week}, now.AddDate(0, 0, -9)) // pruned
	today := []Entry{
		{Name: "Riser", Artist: "Y", Date: "2026-10-22", Explicit: true, Genres: []string{"Pop"}},
		{Name: "Rocky", Artist: "Z", Genres: []string{"Rock"}},
		{Name: "Old", Artist: "X", Genres: []string{"Rock"}},
		{Name: "Popper", Artist: "W", Genres: []string{"Pop"}},
	}
	s.keepRanks("us", chart{albums: today}, now)
	s.apple["us"] = chart{albums: today}
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM chart_days").Scan(&n)
	if n != 2 {
		t.Fatalf("days kept %d, want 2 (9 days ago dropped)", n)
	}
	all, ok := s.Charts(context.Background(), "global", "albums", "", now)
	if !ok || len(all) != 4 || all[0].Title != "Riser" || !all[0].Explicit || !near(all[0].P, 1) || !near(all[3].P, 0.25) || !near(all[0].F14, 1) || all[1].F14 != 0 {
		t.Fatalf("%+v", all)
	}
	if all[0].RankGain == nil || !near(*all[0].RankGain, 0.25) || *all[1].RankAgo != 0 || !near(*all[1].RankGain, 0.75) {
		t.Fatalf("rank gain %+v %+v", *all[0].RankGain, *all[1].RankGain)
	}
	// Genre pages come from ListenBrainz and the slim tags only (none here).
	if rock, ok := s.Charts(context.Background(), "global", "albums", "rock", now); ok {
		t.Fatalf("genre page without ListenBrainz %+v", rock)
	}
	if got, ok := s.Charts(context.Background(), "DE", "albums", "", now); ok {
		t.Fatalf("unknown storefront with no backup must miss %+v", got)
	}
	if _, ok := s.Genres("global"); ok {
		t.Fatal("genre tiles without the list source")
	}
	// A restart loads the newest day.
	s2, err := Open(dbPathOf(t, s), Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if c := s2.apple["us"]; len(c.albums) != 4 {
		t.Fatalf("restart loaded %d", len(c.albums))
	}
}

func dbPathOf(t *testing.T, s *Service) string {
	var seq int
	var name, file string
	if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestNoHistoryNoGain(t *testing.T) {
	s := chartService(t)
	s.apple["us"] = chart{albums: []Entry{{Name: "A", Artist: "B"}}}
	all, _ := s.Charts(context.Background(), "global", "albums", "", time.Now())
	if all[0].RankGain != nil || all[0].RankAgo != nil {
		t.Fatal("rank gain needs 7 days of ranks")
	}
}

func TestSuggest(t *testing.T) {
	s := chartService(t)
	s.apple["us"] = chart{
		albums:  []Entry{{Name: "Radical Optimism", Artist: "Dua Lipa"}, {Name: "OK Computer", Artist: "Radiohead"}, {Name: "Other", Artist: "Q"}},
		songs:   []Entry{{Name: "Radio", Artist: "Lana Del Rey"}, {Name: "Radioactive", Artist: "Imagine Dragons"}, {Name: "Radar", Artist: "Britney"}, {Name: "Radiant", Artist: "Z"}},
		artists: []Entry{{Name: "Radiohead"}, {Name: "Dua Lipa"}},
	}
	if got := s.Suggest(context.Background(), "r", 10); len(got) != 0 {
		t.Fatal("one letter must answer nothing")
	}
	got := s.Suggest(context.Background(), "radi", 10)
	per := map[string]int{}
	for _, g := range got {
		per[g.Kind]++
		if !strings.Contains(strings.ToLower(g.Name), "radi") {
			t.Fatalf("bad match %+v", g)
		}
	}
	if per["song"] != 3 || per["album"] != 1 || per["artist"] != 1 {
		t.Fatalf("%v %+v", per, got)
	}
}

type fakeLists struct{}

func (fakeLists) ChartList(_ context.Context, region, kind string) (ListChart, bool) {
	if region != "us" || kind != "albums" {
		return ListChart{}, false
	}
	return ListChart{Entries: []Entry{{Name: "Listed", Artist: "L", MBID: grand, ArtistMBID: singer, Date: "2026-10-20"}, {Name: "Second", Artist: "M"}},
		Ago: []Entry{{Name: "Second", Artist: "M"}}, HaveAgo: true}, true
}

func (fakeLists) GenreTiles() []GenreTile {
	return []GenreTile{{Slug: "pop", Name: "Pop", Count: 30, Artists: []NamedArtist{{ID: singer, Name: "Chanteuse"}}}}
}

func (fakeLists) Regions() []string { return []string{"us"} }

func (fakeLists) Built() time.Time { return time.Time{} }

func TestChartsListsFirstAndAppleGenreCharts(t *testing.T) {
	s := chartService(t)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s.apple["us"] = chart{albums: []Entry{{Name: "Apple One", Artist: "A", Genres: []string{"Pop"}}, {Name: "Apple Rock", Artist: "R", Genres: []string{"Rock"}}}}
	s.SetLists(fakeLists{})
	// The list source answers first for its regions; Apple for the rest.
	us, ok := s.Charts(context.Background(), "US", "albums", "", now)
	if !ok || len(us) != 2 || us[0].Title != "Listed" || us[0].MBID != grand || us[0].RankGain == nil || *us[1].RankAgo != 1 {
		t.Fatalf("list source first %+v", us)
	}
	if songs, ok := s.Charts(context.Background(), "US", "songs", "", now); ok && len(songs) > 0 {
		t.Fatalf("no songs anywhere %+v", songs)
	}
	// Tiles are the list source's pages that have a genre chart (slim
	// tags plus ListenBrainz).
	if g, ok := s.Genres("FR"); ok {
		t.Fatalf("tiles without genre charts %+v", g)
	}
	s.lb.genres = map[string]genrePool{"pop": {albums: []Entry{{Name: "LB Pop", Artist: "P"}}}}
	g, ok := s.Genres("FR")
	if !ok || len(g) != 1 || g[0].Slug != "pop" || g[0].Count != 30 || g[0].Artists[0].ID != singer {
		t.Fatalf("genre tiles %+v", g)
	}
	pop, ok := s.Charts(context.Background(), "global", "albums", "pop", now)
	if !ok || len(pop) != 1 || pop[0].Title != "LB Pop" || pop[0].Source != SrcListenBrainz {
		t.Fatalf("genre page %+v", pop)
	}
}

func TestTrendingListsFromTheListSourceFirst(t *testing.T) {
	s := chartService(t)
	s.apple["us"] = chart{albums: []Entry{{Name: "Apple One", Artist: "A"}}, artists: []Entry{{Name: "Chanteuse", MBID: singer}}}
	s.SetLists(fakeLists{})
	if err := s.publish(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	var albums, artists struct {
		Source string `json:"source"`
		Items  []struct {
			MBID string `json:"mbid"`
		} `json:"items"`
	}
	b, _, _ := s.List("album", "US")
	_ = json.Unmarshal(b, &albums)
	b, _, _ = s.List("artist", "US")
	_ = json.Unmarshal(b, &artists)
	// Only the list source's matched album is listed; no artist chart there: Apple's.
	if albums.Source != SrcLists || len(albums.Items) != 1 || albums.Items[0].MBID != grand {
		t.Fatalf("albums %s", b)
	}
	if artists.Source != SrcApple || len(artists.Items) != 1 || artists.Items[0].MBID != singer {
		t.Fatalf("artists %+v", artists)
	}
}
