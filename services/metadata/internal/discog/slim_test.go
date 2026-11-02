package discog

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

type fakeSlim struct {
	artists map[string]Artist
	albums  map[string]Album
}

func (f fakeSlim) Artist(_ context.Context, id string) (Artist, bool) {
	a, ok := f.artists[id]
	return a, ok
}
func (f fakeSlim) Album(_ context.Context, id string) (Album, bool) {
	a, ok := f.albums[id]
	return a, ok
}

// The slim copy answers first and its items cost no walk; items it lacks
// still walk live MusicBrainz (the gap path).
func TestSlimFirstThenWalk(t *testing.T) {
	f := &fakeMB{groups: 3}
	slim := fakeSlim{artists: map[string]Artist{artistID: {MBID: artistID, Groups: []Group{{ID: gid(1), Title: "Slim"}}, Count: 1, Complete: true, Walked: time.Now(), Checked: time.Now()}},
		albums: map[string]Album{albumID: {MBID: albumID, Title: "Slim album", FetchedAt: time.Now()}}}
	s := open(t, f, Options{Slim: slim})
	ctx := context.Background()
	if a, ok := s.Artist(ctx, artistID); !ok || a.Groups[0].Title != "Slim" {
		t.Fatalf("artist %+v", a)
	}
	if a, ok := s.Album(ctx, albumID); !ok || a.Title != "Slim album" {
		t.Fatalf("album %+v", a)
	}
	if s.NeedsArtist(ctx, artistID) || s.NeedsAlbum(ctx, albumID) {
		t.Fatal("slim items reported as needing a walk")
	}
	if err := s.Run(ctx, ArtistKey(artistID), fetchq.P0Waiting); err != nil || len(f.calls) != 0 {
		t.Fatalf("slim artist walked: %v %v", err, f.calls)
	}
	if err := s.Run(ctx, AlbumKey(albumID), fetchq.P0Waiting); err != nil || len(f.calls) != 0 {
		t.Fatalf("slim album walked: %v %v", err, f.calls)
	}
	other := gid(42)
	if !s.NeedsArtist(ctx, other) {
		t.Fatal("gap artist not needed")
	}
	if _, err := walk(t, s, ArtistKey(other), fetchq.P0Waiting); err != nil || len(f.calls) != 1 {
		t.Fatalf("gap artist: %v %v", err, f.calls)
	}
	if a, ok := s.Artist(ctx, other); !ok || len(a.Groups) != 3 {
		t.Fatalf("gap artist stored %+v", a)
	}
}

// FromReleases makes the same album a walk makes from the same answers.
func TestFromReleasesMatchesWalk(t *testing.T) {
	chosen := "33333333-3333-3333-3333-333333333333"
	f := &fakeMB{releases: []map[string]any{
		rel("44444444-4444-4444-4444-444444444444", "Official", "US", "CD", 16, "2014-11-21"),
		rel(chosen, "Official", "US", "Digital Media", 12, "2014-11-21"),
		rel("55555555-5555-5555-5555-555555555555", "Official", "JP", "CD", 12, "2014-11-20"),
	}, tracks: map[string]any{"id": chosen, "artist-credit": []any{map[string]any{"name": "Pitbull"}}, "media": []any{map[string]any{"position": 1, "tracks": []any{
		map[string]any{"position": 1, "number": "1", "title": "Fireball", "length": 235000, "recording": map[string]any{"id": "77777777-7777-7777-7777-777777777777"}},
	}}}}}
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	s := open(t, f, Options{Now: func() time.Time { return now }, Stored: func(_ context.Context, kind, id string) ([]byte, bool) {
		return []byte(`{"source_data":{"country":"US"}}`), kind == "artist"
	}})
	if _, err := walk(t, s, AlbumKey(albumID), fetchq.P0Waiting); err != nil {
		t.Fatal(err)
	}
	walked, _ := s.Album(context.Background(), albumID)
	rels, _ := json.Marshal(f.releases)
	built, ok := FromReleases(albumID, rels, len(f.releases), "US", func(id string) ([]byte, bool) {
		b, _ := json.Marshal(f.tracks)
		return b, id == chosen
	}, now)
	if !ok {
		t.Fatal("FromReleases refused")
	}
	if !reflect.DeepEqual(walked, built) {
		t.Fatalf("walk %+v\nslim %+v", walked, built)
	}
}

// A copy with the cheap checks answers NeedsArtist and NeedsAlbum without
// building the answers (Artist and Album here would say no).
type hasSlim struct{ fakeSlim }

func (hasSlim) Has(_ context.Context, kind, id string) bool {
	return kind == "artist" && id == artistID || kind == "release-group" && id == albumID
}

func TestNeedsUsesCheapCheck(t *testing.T) {
	s := open(t, &fakeMB{groups: 1}, Options{Slim: hasSlim{}})
	ctx := context.Background()
	if s.NeedsArtist(ctx, artistID) || s.NeedsAlbum(ctx, albumID) {
		t.Fatal("held items reported as needing a walk")
	}
	if !s.NeedsArtist(ctx, gid(42)) || !s.NeedsAlbum(ctx, gid(43)) {
		t.Fatal("items the copy lacks not needed")
	}
}
