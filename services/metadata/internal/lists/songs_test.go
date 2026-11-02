package lists

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSongChartsFromTheChartReaderFirst(t *testing.T) {
	src := &fakeSource{}
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	charts := map[string]SongChart{
		"global": {Day: "2026-10-25", At: now.Add(-time.Hour), Rows: []SongRow{
			{Rank: 1, Move: "=", Title: "Housewife", Artist: "Tate McRae", ArtistID: "45dkTj5sMRSjrmBSBeiHym", TrackID: "t1", Days: 10, Peak: 1, Streams: 6254852},
			{Rank: 2, Move: "+3", Title: "BbY WOW", Artist: "KAROL G", TrackID: "t2", Days: 75, Peak: 1, Streams: 4606007},
			{Rank: 3, Move: "NEW", Title: "Fresh", Artist: "New One", TrackID: "t3", Days: 1, Peak: 3},
		}},
		"us": {Failed: true},
	}
	asked := 0
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, Now: func() time.Time { return now },
		Songs: func(region string) (SongChart, bool) { c, ok := charts[region]; return c, ok },
		Tracks: func(_ context.Context, ids []string) (map[string]TrackAlbum, error) {
			asked++
			out := map[string]TrackAlbum{}
			for _, id := range ids {
				if id == "t1" {
					out[id] = TrackAlbum{ID: "a1", Name: "So Close To What", Type: "album", ReleaseDate: "2025-02-21", Image: "img1"}
				}
			}
			return out, nil
		}})
	ctx := context.Background()
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	es, _ := s.latest("songs:global")
	if len(es) != 3 {
		t.Fatalf("global songs %+v", es)
	}
	h := es[0]
	if h.Album != "So Close To What" || h.AlbumType != "album" || h.Image != "img1" || h.Status != "same" || h.Prev != 1 || h.PeakDate != "2026-10-25" || h.EntryDate != "2026-10-16" || h.ExtArtist != "45dkTj5sMRSjrmBSBeiHym" {
		t.Fatalf("first song %+v", h)
	}
	if es[1].Prev != 5 || es[1].Status != "up" || es[2].Status != "new" || es[2].Prev != 0 {
		t.Fatalf("moves %+v %+v", es[1], es[2])
	}
	// The chart reader's region is never asked of the source; its failed
	// region is (the backup); regions it has not read yet wait.
	src.mu.Lock()
	n := src.n
	src.mu.Unlock()
	var global, us int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM lists_source WHERE key = 'songs:global' AND hash != ''`).Scan(&global)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM lists_source WHERE key = 'songs:us' AND hash != ''`).Scan(&us)
	if global != 1 || us != 1 || n == 0 {
		t.Fatalf("stored global %d, us %d (backup), source calls %d", global, us, n)
	}
	if es, _ := s.latest("songs:gb"); len(es) != 0 {
		t.Fatal("a region the chart reader has not read was asked of the source")
	}
	// The song's album is matched like any album, and its track is kept.
	if _, ok, _ := s.matcher.Known(ctx, "album", "So Close To What", "Tate McRae"); !ok {
		t.Fatal("the song's album was not matched")
	}
	if got := s.albumsOf(ctx, []string{"t1"}); got["t1"].Name != "So Close To What" || asked != 1 {
		t.Fatalf("track not kept: %v, %d calls", got, asked)
	}
	// The shelves show the album for the charted song.
	b, _, _ := s.Shelves("global")
	var sh Shelves
	_ = json.Unmarshal(b, &sh)
	top := sh.Blocks["top_now"]
	if len(top) == 0 || top[0].Title != "So Close To What" || top[0].Song != "Housewife" {
		t.Fatalf("top_now %+v", top)
	}
}

func TestPeakDateCarriesAcrossDays(t *testing.T) {
	src := &fakeSource{}
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	s, _ := openTest(t, src, Options{Now: func() time.Time { return now }, Songs: func(string) (SongChart, bool) { return SongChart{}, false }})
	day1 := SongChart{Day: "2026-10-24", Rows: []SongRow{{Rank: 1, Move: "=", Title: "Top", Artist: "A", Peak: 1}}}
	b, _ := json.Marshal(s.songEntries(context.Background(), "songs:global", day1))
	s.store(Source{Key: "songs:global"}, b)
	day2 := SongChart{Day: "2026-10-25", Rows: []SongRow{{Rank: 2, Move: "-1", Title: "Top", Artist: "A", Peak: 1}}}
	if es := s.songEntries(context.Background(), "songs:global", day2); es[0].PeakDate != "2026-10-24" {
		t.Fatalf("peak date %+v", es[0])
	}
}

func TestSongChartsLeaveThePlanBudget(t *testing.T) {
	with, without := planWith(900, true), Plan(900)
	if c := PlanCost(onlyBudgeted(with)); c > 900*9/10 {
		t.Fatalf("budgeted cost %d over the cap", c)
	}
	songs := 0
	for _, src := range with {
		if src.Kind == KindSongs {
			songs++
		}
	}
	if songs != len(TopRegions) {
		t.Fatalf("%d song sources, want every region (%d)", songs, len(TopRegions))
	}
	if len(with) <= len(without) {
		t.Fatalf("the freed budget was not spent: %d sources, before %d", len(with), len(without))
	}
	if !strings.HasPrefix(with[0].Key, "songs:") {
		t.Fatalf("plan order %v", with[0])
	}
}

// onlyBudgeted drops the song charts (a backup outside the budget).
func onlyBudgeted(p []Source) []Source {
	var out []Source
	for _, s := range p {
		if s.Kind != KindSongs {
			out = append(out, s)
		}
	}
	return out
}
