package musicdata

import (
	"context"
	"errors"
	"testing"
)

func TestDeezerArtistKeptAndServedStale(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.artist(artistA, "Frank Ocean", "https://www.deezer.com/artist/42")
	g.f.set("/deezer/artist/42", map[string]any{"id": 42, "name": "Frank Ocean", "nb_fan": 900, "picture_xl": "https://d.test/xl"})
	g.f.set("/deezer/artist/42/top", map[string]any{"data": []any{map[string]any{"title": "Lost", "duration": 234, "explicit_lyrics": true, "album": map[string]any{"title": "Channel Orange"}}}})
	g.f.set("/deezer/artist/42/albums", map[string]any{"data": []any{map[string]any{"id": 7, "title": "Blonde", "explicit_lyrics": true, "release_date": "2016-08-20"}}})
	a, err := g.s.DeezerArtist(ctx, artistA)
	if err != nil || a.ID != 42 || a.Fans != 900 || a.Picture != "https://d.test/xl" || len(a.Top) != 1 || !a.Top[0].Explicit || len(a.Albums) != 1 || !a.Albums[0].Explicit || a.Albums[0].ID != 7 {
		t.Fatalf("artist: %+v %v", a, err)
	}
	// Kept: a second ask makes no call.
	if _, err := g.s.DeezerArtist(ctx, artistA); err != nil || g.f.hit("/deezer/artist/42/albums") != 1 {
		t.Fatalf("kept answer asked Deezer again: %d %v", g.f.hit("/deezer/artist/42/albums"), err)
	}
	// Past its time and Deezer failing: the kept copy is served.
	if _, err := g.s.db.Exec(`UPDATE answers SET until = 0 WHERE k = ?`, "dz-artist:"+artistA); err != nil {
		t.Fatal(err)
	}
	g.f.set("/deezer/artist/42", 500)
	if a, err := g.s.DeezerArtist(ctx, artistA); err != nil || a.ID != 42 {
		t.Fatalf("stale copy not served: %+v %v", a, err)
	}
}

func TestDeezerArtistNoneAndLater(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	// Not stored yet (no MusicBrainz answer): later, never kept as none.
	if _, err := g.s.DeezerArtist(ctx, artistB); !errors.Is(err, ErrLater) {
		t.Fatalf("unknown artist: %v", err)
	}
	// Known, no Deezer match: none.
	g.artist(artistA, "Nobody")
	g.f.set("/deezer/search/artist?Nobody", map[string]any{"data": []any{}})
	if _, err := g.s.DeezerArtist(ctx, artistA); !errors.Is(err, ErrNone) {
		t.Fatalf("no match: %v", err)
	}
}

func TestDeezerAlbum(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.f.set("/deezer/album/7", map[string]any{"id": 7, "label": "Boys Don't Cry", "explicit_lyrics": true,
		"tracks": map[string]any{"data": []any{map[string]any{"title": "Nikes", "explicit_lyrics": true}, map[string]any{"title": "Ivy", "explicit_lyrics": false}}}})
	a, err := g.s.DeezerAlbum(ctx, 7)
	if err != nil || a.Label != "Boys Don't Cry" || !a.Explicit || len(a.Tracks) != 2 || !a.Tracks[0].Explicit || a.Tracks[1].Explicit {
		t.Fatalf("album: %+v %v", a, err)
	}
	if _, err := g.s.DeezerAlbum(ctx, 7); err != nil || g.f.hit("/deezer/album/7") != 1 {
		t.Fatalf("kept album asked again: %v", err)
	}
	g.f.set("/deezer/album/8", map[string]any{"error": map[string]any{"code": 800}})
	if _, err := g.s.DeezerAlbum(ctx, 8); !errors.Is(err, ErrNone) {
		t.Fatalf("missing album: %v", err)
	}
	g.f.set("/deezer/album/9", 500)
	if _, err := g.s.DeezerAlbum(ctx, 9); !errors.Is(err, ErrLater) {
		t.Fatalf("failing album: %v", err)
	}
}
