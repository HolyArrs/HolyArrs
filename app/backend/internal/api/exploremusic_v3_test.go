package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// v3Fixed is the fixed clock of the Phase 3 golden (ties hash the date).
var v3Fixed = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// fakeArtistSource answers the metadata server's artist batches for the
// household fixture: each band has nine similar artists, rock and pop
// votes, three back albums with listeners and three guest spots.
func fakeArtistSource(calls *int) (func(context.Context, []string) (map[string]int64, error), func(context.Context, []string) (map[string][]metadata.MusicSimilarScore, error), func(context.Context, []string) ([]metadata.MusicArtistData, error)) {
	band := func(id string) int {
		var k int
		if _, err := fmt.Sscanf(id, "a000000%d-", &k); err != nil || k < 1 || k > 3 {
			return 0
		}
		return k
	}
	listeners := func(_ context.Context, ids []string) (map[string]int64, error) {
		*calls++
		out := map[string]int64{}
		for i, id := range ids {
			out[id] = int64(1000 * (len(id) + i%7))
		}
		return out, nil
	}
	similar := func(_ context.Context, seeds []string) (map[string][]metadata.MusicSimilarScore, error) {
		*calls++
		out := map[string][]metadata.MusicSimilarScore{}
		for _, s := range seeds {
			k := band(s)
			if k == 0 {
				continue
			}
			for i := 1; i <= 9; i++ {
				out[s] = append(out[s], metadata.MusicSimilarScore{MBID: mid('b', 10*k+i), Name: fmt.Sprintf("Near %d-%d", k, i), S: 1 - 0.08*float64(i-1), Source: "listenbrainz"})
			}
		}
		return out, nil
	}
	data := func(_ context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		*calls++
		var out []metadata.MusicArtistData
		for _, id := range ids {
			d := metadata.MusicArtistData{MBID: id}
			if k := band(id); k > 0 {
				d.Genres = []metadata.MusicGenreVote{{Name: "Rock", Count: 10 - k}, {Name: "pop", Count: 2 + k}}
				for j := 0; j < 3; j++ {
					d.Albums = append(d.Albums, metadata.MusicTopAlbum{MBID: fmt.Sprintf("c00001%d%d-0000-4000-8000-0000000001%d%d", k, j, k, j), Title: fmt.Sprintf("Back %d-%d", k, j), Type: "album", Listeners: int64(5000 * (3 - j)), Artists: []string{id}})
				}
				for j := 1; j <= 3; j++ {
					d.Albums = append(d.Albums, metadata.MusicTopAlbum{MBID: mid('d', 10*k+j), Title: fmt.Sprintf("Guest %d-%d", k, j), Type: "single", Listeners: int64(100 * j), Artists: []string{mid('e', 10*k+j), id}, ArtistNames: []string{fmt.Sprintf("Host %d-%d", k, j), "Band"}})
				}
			} else if strings.HasPrefix(id, "b") {
				d.Genres = []metadata.MusicGenreVote{{Name: "rock", Count: 3}}
				if id[len(id)-1]%2 == 0 {
					d.Genres = []metadata.MusicGenreVote{{Name: "jazz", Count: 3}}
				}
				d.Albums = []metadata.MusicTopAlbum{{MBID: "f" + id[1:], Title: "Best of " + id[:8], Type: "album", Listeners: int64(len(id) * 100), Artists: []string{id}, ArtistNames: []string{"Near"}}}
			}
			out = append(out, d)
		}
		return out, nil
	}
	return listeners, similar, data
}

func v3Rig(t *testing.T) (*askRig, *int) {
	r := newMusicAskRig(t)
	loadHousehold(t, r.st, v3Fixed)
	r.api.s.MusicRowsV2, r.api.s.MusicRowsV3 = true, true
	r.api.musicv2.now = func() time.Time { return v3Fixed }
	charts, genres := fakeCharts(v3Fixed)
	r.api.s.MusicGenres = genres
	// Genre charts: their own albums (the region chart otherwise).
	r.api.s.MusicCharts = func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error) {
		c, err := charts(ctx, region, kind, genre)
		if genre == "" {
			return c, err
		}
		for i := range c.Items {
			c.Items[i].MBID = mid('9', 50+i)
			c.Items[i].Title = genre + " chart " + fmt.Sprint(i+1)
			c.Items[i].Explicit = false
		}
		return c, err
	}
	calls := 0
	r.api.s.MusicListeners, r.api.s.MusicSimilarSet, r.api.s.MusicArtistData = fakeArtistSource(&calls)
	return r, &calls
}

// route: GET /api/v1/explore/music (music_rows_v3)
func TestExploreMusicV3Household(t *testing.T) {
	r, calls := v3Rig(t)
	first := r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if string(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200)) != string(first) {
		t.Fatal("two reads on one day must be byte-identical")
	}
	var m musicRowsAnswer
	_ = json.Unmarshal(first, &m)
	got := musicShape(m)
	const path = "testdata/explore_music_v3.golden"
	if os.Getenv("HOLYARRS_WRITE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Fatalf("v3 golden differs:\n%s\nwant:\n%s", got, want)
	}
	for _, id := range []string{"new_from", "genres", "since_have", "near_taste", "genre_shelf", "guest_spots"} {
		if m.row(id) == nil {
			t.Fatalf("no %s row:\n%s", id, got)
		}
	}
	// One shared dedup across every row (and the hero).
	seen := map[string]string{m.Hero.Key: "hero"}
	for _, row := range m.Rows {
		for _, c := range row.Items {
			if c.Kind == "album" {
				if prev, ok := seen[c.Key]; ok {
					t.Fatalf("%s in %s and %s", c.Key, prev, row.ID)
				}
				seen[c.Key] = row.ID
			}
		}
	}
	// Explicit chart albums keep the E mark.
	explicit := false
	for _, row := range m.Rows {
		for _, c := range row.Items {
			explicit = explicit || (c.MBID == apiAlbum && c.Explicit)
		}
	}
	if !explicit {
		t.Fatal("the explicit chart album lost its E mark")
	}
	// Genres lead with the household's top genre (rock, then pop).
	if g := m.row("genres"); g.Items[0].Name != "Rock" || g.Items[1].Name != "Pop" {
		t.Fatalf("genre order %s %s", g.Items[0].Name, g.Items[1].Name)
	}

	// An ask moves only layer 3: the asked card sinks to its row's end.
	before := *calls
	r.ana.call("POST", "/api/v1/asks", `{"kind":"artist","id":"mbid:`+apiArtist+`","albums":["`+apiAlbum+`"]}`, 201)
	var m2 musicRowsAnswer
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200), &m2)
	if *calls != before {
		t.Fatal("an ask must not rebuild layer 2")
	}
	for _, row := range m2.Rows {
		if row.ID == "your_asks" {
			continue
		}
		for i, c := range row.Items {
			if c.MBID == apiAlbum && (c.State != "asked" || i != len(row.Items)-1) {
				t.Fatalf("%s: asked card at %d of %d, state %q", row.ID, i, len(row.Items), c.State)
			}
		}
	}
}

func TestExploreMusicV3FlagOff(t *testing.T) {
	// v3 off: the Phase 1 rows only, and no artist batches read.
	r, calls := v3Rig(t)
	r.api.s.MusicRowsV3 = false
	var m musicRowsAnswer
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200), &m)
	phase1 := map[string]bool{"new_from": true, "your_asks": true, "genres": true, "fresh_songs": true, "missing": true, "top_albums": true, "coming_soon": true}
	for _, row := range m.Rows {
		if !phase1[row.ID] {
			t.Fatalf("v3 off showed %s", row.ID)
		}
	}
	if *calls != 0 {
		t.Fatal("v3 off must not read the artist batches")
	}
}

func TestExploreMusicV3Since(t *testing.T) {
	// Yesterday's seeds are not seeded again today.
	r, _ := v3Rig(t)
	var today musicRowsAnswer
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200), &today)
	seeds := map[string]bool{}
	for _, row := range today.Rows {
		if row.Type == "since_have" {
			seeds[row.ID] = true
		}
	}
	if len(seeds) == 0 {
		t.Fatal("no Since you have rows")
	}
	r.api.musicv2.now = func() time.Time { return v3Fixed.AddDate(0, 0, 1) }
	var next musicRowsAnswer
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200), &next)
	for _, row := range next.Rows {
		if row.Type == "since_have" && seeds[row.ID] {
			t.Fatalf("%s seeded two days running", row.ID)
		}
	}
}

// TestExploreMusicV3Cost is the cost metric: layer 2 for 50 members, layer
// 1 warm, in at most 2 s.
func TestExploreMusicV3Cost(t *testing.T) {
	r, _ := v3Rig(t)
	r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200) // warms layer 1
	ctx := context.Background()
	lib, err := r.api.musicLibrary(ctx, v3Fixed)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for member := int64(1); member <= 50; member++ {
		acts, err := r.api.musicActsOf(ctx, member)
		if err != nil {
			t.Fatal(err)
		}
		if ord := r.api.buildMusicV3(ctx, member, "US", lib, acts, v3Fixed); len(ord.Rows) == 0 {
			t.Fatal("no rows")
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("50 members took %v", d)
	}
	t.Logf("layer 2 for 50 members: %v", time.Since(start))
}
