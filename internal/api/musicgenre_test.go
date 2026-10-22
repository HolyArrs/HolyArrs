package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: GET /api/v1/music/genre/{slug}
func TestMusicGenrePage(t *testing.T) {
	now := time.Now().UTC()
	r := newMusicAskRig(t)
	loadHousehold(t, r.st, now)
	r.api.s.MusicRowsV2 = true
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
}

// route: GET /api/v1/music/genre-art/{slug}
func TestMusicGenreArtRoute(t *testing.T) {
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
	b, _ := os.ReadFile("musicgenre.go")
	s := string(b)
	page := s[strings.Index(s, "func (a *API) musicGenrePage"):]
	if strings.Index(page, "a.kidMax(r)") > strings.Index(page, "a.musicGenreTiles") {
		t.Fatal("the Kid check must run before any music is read")
	}
}
