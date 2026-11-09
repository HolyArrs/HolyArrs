package music

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/music/genre/{slug}
func TestMusicGenrePage(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	r := newMusicAskRig(t)
	loadHousehold(t, r.st, now)
	r.api.s.MusicCharts, r.api.s.MusicGenres = fakeCharts(now)

	r.ana.call("GET", "/api/v1/music/genre/Pop!", "", 400)
	r.ana.call("GET", "/api/v1/music/genre/polka?region=US", "", 404)
	var m struct {
		musicRowsAnswer
		Tile MusicCard `json:"tile"`
	}
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/music/genre/pop?region=US", "", 200), &m); err != nil {
		t.Fatal(err)
	}
	if m.Tile.Name != "Pop" || m.Tile.Hue == nil || m.Tile.Image != "" {
		t.Fatalf("tile %+v", m.Tile)
	}
	if m.Hero == nil || m.Hero.Kind != "album" {
		t.Fatalf("hero %+v", m.Hero)
	}
	if m.row("genre_top") == nil || m.row("genre_artists") == nil {
		t.Fatalf("rows %+v", m.Rows)
	}
	seen := map[string]bool{m.Hero.Key: true}
	explicit := false
	for _, row := range m.Rows {
		for _, c := range row.Items {
			if seen[c.Key] {
				t.Fatalf("%s twice (%s)", c.Key, row.ID)
			}
			seen[c.Key] = true
			explicit = explicit || c.Explicit
			if row.ID == "genre_artists" && (c.Kind != "artist" || c.TitleID != 0) {
				t.Fatalf("popular artist %+v", c)
			}
		}
	}
	if !explicit && !m.Hero.Explicit {
		for _, c := range m.Alts {
			explicit = explicit || c.Explicit
		}
	}
	if !explicit {
		t.Fatal("the explicit chart album lost its E mark")
	}
	// Lab Band charts in Pop and is in the Library: one of your artists.
	yours := m.row("genre_yours")
	if yours != nil {
		for _, c := range yours.Items {
			if c.TitleID == 0 {
				t.Fatalf("your artist without a title %+v", c)
			}
		}
	}
}

// Tile art: the server's tile link becomes the app's picture route, and
// with none the tile stays a flat hue.
func TestMusicGenreArtLink(t *testing.T) {
	t.Parallel()
	if got := musicGenreArt(metadata.MusicGenre{Slug: "pop"}); got != "" {
		t.Fatalf("flat: %q", got)
	}
	if got := musicGenreArt(metadata.MusicGenre{Slug: "pop", Art: "/v1/music/genres/tile/pop.jpg?v=ab12"}); got != "/api/v1/music/genre-art/pop?v=ab12" {
		t.Fatalf("art: %q", got)
	}
	if got := musicGenreArt(metadata.MusicGenre{Slug: "pop", Art: "https://elsewhere.example/x.jpg"}); got != "" {
		t.Fatalf("outside link: %q", got)
	}
	if !genreMatch("hip-hop", "hip-hop-rap") || genreMatch("pop", "k-pop") || musicSlug("R&B/Soul") != "randb-soul" {
		t.Fatal("genre names")
	}
	if got := musicGenreArts(metadata.MusicGenre{Slug: "pop", Arts: []string{"/v1/music/genres/tile/pop.jpg?v=a1", "/v1/music/genres/tile/pop.2.jpg?v=b2", "https://x/y.jpg"}}); len(got) != 2 || got[0] != "/api/v1/music/genre-art/pop?v=a1" || got[1] != "/api/v1/music/genre-art/pop?v=b2&n=1" {
		t.Fatalf("arts: %v", got)
	}
}

// route: GET /api/v1/music/genre-art/{slug}
func TestMusicGenreArtRoute(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	calls := 0
	r.api.s.MusicGenreTile = func(_ context.Context, slug, v string) ([]byte, error) {
		calls++
		if slug != "pop" {
			return nil, metadata.ErrNotFound
		}
		return []byte("\xff\xd8jpeg"), nil
	}
	if b := r.ana.call("GET", "/api/v1/music/genre-art/pop?v=1", "", 200); string(b) != "\xff\xd8jpeg" {
		t.Fatalf("%q", b)
	}
	r.ana.call("GET", "/api/v1/music/genre-art/pop?v=1", "", 200)
	r.ana.call("GET", "/api/v1/music/genre-art/rock?v=1", "", 404)
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

func TestMusicGenreKidFirst(t *testing.T) {
	t.Parallel()
	b, _ := os.ReadFile("genre.go")
	s := string(b)
	page := s[strings.Index(s, "func (h *Handlers) musicGenrePage"):]
	if strings.Index(page, "h.KidMax(r)") > strings.Index(page, "h.musicGenreTiles") {
		t.Fatal("the Kid check must run before any music is read")
	}
}

// A genre page warms the art (photo, wide photo, logo) of every artist and
// album it lists, like every other music row.
func TestMusicGenrePageWarmsArt(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	r := newMusicAskRig(t)
	loadHousehold(t, r.st, now)
	r.api.s.MusicCharts, r.api.s.MusicGenres = fakeCharts(now)
	var mu sync.Mutex
	warmed := map[ArtWarmKey]bool{}
	r.api.SetArtWarmDo(func(_ context.Context, k ArtWarmKey) {
		mu.Lock()
		warmed[k] = true
		mu.Unlock()
	})
	var m musicRowsAnswer
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/music/genre/pop?region=US", "", 200), &m); err != nil {
		t.Fatal(err)
	}
	want := map[ArtWarmKey]bool{}
	for _, row := range m.Rows {
		for _, c := range row.Items {
			if c.ArtistMBID != "" {
				want[ArtWarmKey{Kind: "artist", MBID: c.ArtistMBID}] = true
			}
		}
	}
	if len(want) == 0 {
		t.Fatal("no artists listed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		missing := 0
		for k := range want {
			if !warmed[k] {
				missing++
			}
		}
		mu.Unlock()
		if missing == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d listed artists not warmed", missing, len(want))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// With music_lists_v1 a genre page adds the tile's editorial picks, its
// artists and the region's charts filtered to the genre: 8 rows or more.
func TestMusicGenrePageListRows(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	r := newMusicAskRig(t)
	r.api.s.MusicCharts, r.api.s.MusicGenres = fakeCharts(now)
	r.api.s.MusicShelves = fakeShelves
	r.api.musicx.mu.Lock()
	r.api.musicx.genres = map[string]musicGenreEntry{}
	for i := 0; i < 40; i++ {
		r.api.musicx.genres[mid('6', i)] = musicGenreEntry{genres: []string{"pop"}, at: now}
	}
	r.api.musicx.mu.Unlock()
	var m musicRowsAnswer
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/music/genre/pop?region=US", "", 200), &m); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"genre_picks", "genre_to_know", "genre_rising", "genre_charts"} {
		if m.row(id) == nil {
			t.Fatalf("no %s row: %d rows", id, len(m.Rows))
		}
	}
	if len(m.Rows) < 8 {
		t.Fatalf("%d rows", len(m.Rows))
	}
	seen := map[string]bool{}
	for _, row := range m.Rows {
		if row.Reason == "" {
			t.Fatalf("%s without a reason", row.ID)
		}
		for _, c := range row.Items {
			if seen[c.Key] {
				t.Fatalf("%s twice", c.Key)
			}
			seen[c.Key] = true
		}
	}
}
