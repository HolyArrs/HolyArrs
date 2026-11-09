package music

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// trendChart is a 20-album region chart; with ranks, each album's rank 7
// days ago is set so albums 1–3 fell, 4–13 climbed and 14–20 are new.
func trendChart(now time.Time, ranks bool) []metadata.MusicChartItem {
	var out []metadata.MusicChartItem
	for i := 1; i <= 20; i++ {
		it := metadata.MusicChartItem{Rank: i, MBID: mid('f', i), ArtistMBID: mid('e', i), Title: fmt.Sprintf("Chart album %d", i), Artist: fmt.Sprintf("Charter %d", i),
			Date: now.AddDate(0, 0, -i*3).Format("2006-01-02"), Genres: []string{"Pop"}, P: 1 - float64(i-1)/20}
		if ranks {
			ago := 0 // new entry
			switch {
			case i <= 3:
				ago = i // steady
			case i <= 13:
				ago = i + i // climbed i places
			}
			it.RankAgo = &ago
		}
		out = append(out, it)
	}
	return out
}

func trendInput(now time.Time, ranks bool) musicTrendingInput {
	in := musicTrendingInput{region: "US", albums: trendChart(now, ranks), owned: map[string]int64{mid('a', 1): 7}, follows: map[string]bool{}, listeners: map[string]int64{}}
	for i := 1; i <= 8; i++ {
		in.songs = append(in.songs, metadata.MusicChartItem{Rank: i, Title: fmt.Sprintf("Song %d", i), Artist: fmt.Sprintf("Singer %d", i), Date: now.AddDate(0, 0, -i).Format("2006-01-02"), P: 1 - float64(i-1)/8, Explicit: i == 2})
	}
	// This week: 8 uncharted albums with growing listeners, plus one charted.
	for i := 1; i <= 8; i++ {
		r := metadata.NewRelease{MBID: mid('c', i), Name: fmt.Sprintf("Week %d", i), PrimaryType: "album", ReleaseDate: now.AddDate(0, 0, -(i % 7)).Format("2006-01-02"), Artist: fmt.Sprintf("Weekly %d", i), ArtistMBID: mid('b', i)}
		in.releases = append(in.releases, r)
		in.listeners[mid('b', i)] = int64(1000 * i)
	}
	in.releases = append(in.releases, metadata.NewRelease{MBID: mid('f', 15), Name: "Chart album 15", PrimaryType: "album", ReleaseDate: now.AddDate(0, 0, -2).Format("2006-01-02"), Artist: "Charter 15", ArtistMBID: mid('e', 15)})
	// Coming: 7 albums, the Library artist's one last by date.
	for i := 1; i <= 7; i++ {
		artist := mid('d', i)
		if i == 7 {
			artist = mid('a', 1)
		}
		in.releases = append(in.releases, metadata.NewRelease{MBID: mid('9', i), Name: fmt.Sprintf("Soon %d", i), PrimaryType: "album", ReleaseDate: now.AddDate(0, 0, i).Format("2006-01-02"), Artist: "Soon", ArtistMBID: artist})
	}
	return in
}

func rowByID(rows []MusicRow, id string) *MusicRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func TestComposeMusicTrending(t *testing.T) {
	now := orderFixed
	// Fewer than 7 days of ranks: no Climbing row.
	hero, rows := composeMusicTrending(trendInput(now, false), now)
	if rowByID(rows, "np_music_climbing") != nil {
		t.Fatal("Climbing must stay hidden without 7 days of ranks")
	}
	if len(hero) != musicTrendingHeroMax {
		t.Fatalf("hero %d", len(hero))
	}
	for _, id := range []string{"np_music_tracks", "np_music_week", "np_music_top", "np_music_coming"} {
		if rowByID(rows, id) == nil {
			t.Fatalf("no %s row", id)
		}
	}
	// One chart item missing its 7-day rank still hides Climbing.
	in := trendInput(now, true)
	in.albums[5].RankAgo = nil
	if _, rows := composeMusicTrending(in, now); rowByID(rows, "np_music_climbing") != nil {
		t.Fatal("a partial history must hide Climbing")
	}

	// 7-day fixture: Climbing shows, biggest gain first, fallers left out.
	hero, rows = composeMusicTrending(trendInput(now, true), now)
	climb := rowByID(rows, "np_music_climbing")
	if climb == nil {
		t.Fatal("Climbing must show with 7 days of ranks")
	}
	seen := map[string]string{}
	for _, c := range hero {
		seen[c.Key] = "hero"
	}
	for _, row := range rows {
		for _, c := range row.Items {
			if prev, ok := seen[c.Key]; ok {
				t.Fatalf("%s in %s and %s", c.Key, prev, row.ID)
			}
			seen[c.Key] = row.ID
		}
	}
	for _, c := range climb.Items {
		if c.Rank <= 3 {
			t.Fatalf("album %d did not climb", c.Rank)
		}
		if c.Note == "" {
			t.Fatal("Climbing cards carry their move")
		}
	}
	// Climbed 13 places (ΔR 0.65); new entries count from L + 1, so place 14
	// gains (21 − 14)/20 = 0.35 and follows the climbers.
	if climb.Items[0].Rank != 13 || climb.Items[0].Note != "Up 13 · #13" {
		t.Fatalf("first climber %d %q", climb.Items[0].Rank, climb.Items[0].Note)
	}
	for _, c := range climb.Items {
		if c.Rank == 14 && c.Note != "New entry · #14" {
			t.Fatalf("new entry note %q", c.Note)
		}
	}

	// Out this week: uncharted albums under the listener median are left
	// out; the charted one stays whatever its listeners.
	week := rowByID(rows, "np_music_week")
	if week == nil {
		t.Fatal("no Out this week")
	}
	has := map[string]bool{}
	for _, c := range week.Items {
		has[c.MBID] = true
	}
	if has[mid('c', 1)] || has[mid('c', 3)] || !has[mid('c', 8)] {
		t.Fatalf("median rule: %v", has)
	}
	if !has[mid('f', 15)] && seen["album:"+mid('f', 15)] == "" {
		t.Fatal("the charted release was dropped")
	}
	// Coming soon: the Library artist's album first (0.7·[A>0] + 0.3·P).
	if c := rowByID(rows, "np_music_coming"); c == nil || c.Items[0].MBID != mid('9', 7) || !c.Items[0].Upcoming {
		t.Fatalf("coming %+v", c)
	}
	// Explicit songs keep the E mark.
	explicit := false
	for _, c := range rowByID(rows, "np_music_tracks").Items {
		explicit = explicit || c.Explicit
	}
	if !explicit {
		t.Fatal("the explicit song lost its E mark")
	}
}

// route: GET /api/v1/explore/new/music (music_rows_v6)
func TestExploreNewMusic(t *testing.T) {
	r := newMusicAskRig(t)
	r.api.s.MusicCharts = func(_ context.Context, region, kind, _ string) (metadata.MusicChart, error) {
		c := metadata.MusicChart{Region: region, Kind: kind, Items: trendChart(time.Now().UTC(), true)}
		return c, nil
	}
	musicNew = musicNewStore{data: &musicNewData{at: time.Now()}}
	t.Cleanup(func() { musicNew = musicNewStore{} })
	var m struct {
		Kid  bool       `json:"kid"`
		Rows []MusicRow `json:"rows"`
	}
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/explore/new/music?region=US", "", 200), &m); err != nil {
		t.Fatal(err)
	}
	if m.Kid || rowByID(m.Rows, "np_music_climbing") == nil {
		t.Fatalf("rows %v", m.Rows)
	}
	k := newKidRig(t)
	if code, b := k.kai.get("/api/v1/explore/new/music"); code == 200 && !strings.Contains(b, `"kid":true`) {
		t.Fatalf("kid answer %s", b)
	}
}
