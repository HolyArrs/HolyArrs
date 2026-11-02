package musicpkg

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

const (
	art = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	rg1 = "11111111-1111-1111-1111-111111111111"
	rg2 = "22222222-2222-2222-2222-222222222222"
	rg3 = "33333333-3333-3333-3333-333333333333"
	rg4 = "44444444-4444-4444-4444-444444444444"
)

func TestKind(t *testing.T) {
	cases := []struct {
		p    string
		s    []string
		want string
	}{
		{"Album", nil, "album"}, {"EP", nil, "ep"}, {"Single", nil, "single"},
		{"Album", []string{"Live"}, "live"}, {"Album", []string{"Compilation", "Live"}, "compilation"},
		{"Album", []string{"Soundtrack"}, "soundtrack"}, {"Album", []string{"Mixtape/Street"}, "mixtape"},
		{"Other", nil, "other"}, {"", nil, "other"}, {"Broadcast", nil, "broadcast"},
	}
	for _, c := range cases {
		if got := Kind(c.p, c.s); got != c.want {
			t.Errorf("Kind(%q,%v) = %q, want %q", c.p, c.s, got, c.want)
		}
	}
}

func groups() []discog.Group {
	return []discog.Group{
		{ID: rg1, Title: "Single", PrimaryType: "Single", FirstReleaseDate: "2020-01-01"},
		{ID: rg2, Title: "Old album", PrimaryType: "Album", FirstReleaseDate: "1997-05-21"},
		{ID: rg3, Title: "New album", PrimaryType: "Album", FirstReleaseDate: "2016-05-08"},
		{ID: rg4, Title: "Live", PrimaryType: "Album", SecondaryTypes: []string{"Live"}, FirstReleaseDate: "2001-01-01"},
	}
}

func TestPickAlbumsFirstNewestFirst(t *testing.T) {
	got := Pick(groups(), Depth{Max: 3})
	want := []string{rg3, rg2, rg1}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Pick = %v, want %v", got, want)
	}
	got = Pick(groups(), Depth{Max: 10, Kinds: map[string]bool{"album": true}})
	if len(got) != 2 {
		t.Fatalf("album-only pick = %v", got)
	}
}

func sources(albums map[string]bool) Sources {
	return Sources{
		Artist: func(_ context.Context, id string) ([]byte, bool) {
			return []byte(`{"name":"Radiohead"}`), id == art
		},
		Discog: func(context.Context, string) (discog.Artist, bool) {
			return discog.Artist{MBID: art, Groups: groups(), Count: 4, Complete: true}, true
		},
		Album: func(_ context.Context, id string) (discog.Album, bool) {
			if !albums[id] {
				return discog.Album{}, false
			}
			return discog.Album{MBID: id, Edition: &discog.Edition{ID: "e-" + id}, Tracks: []discog.Track{{Disc: 1, Position: 1, Title: "T"}}, TrackCount: 1, DiscCount: 1, FetchedAt: time.Now()}, true
		},
		Pictures: func(_ context.Context, kind, id string) map[string]imgstore.Info {
			if kind == "artist" {
				return map[string]imgstore.Info{"photo": {Version: "p1"}}
			}
			if id == rg3 {
				return map[string]imgstore.Info{"cover": {Version: "c3"}}
			}
			return nil
		},
	}
}

func TestBuildCompleteAndPending(t *testing.T) {
	ctx := context.Background()
	d := Depth{Max: 10}
	p, ok := Build(ctx, sources(map[string]bool{rg1: true, rg2: true, rg3: true}), art, 1, d, time.Now())
	if !ok || p.Complete {
		t.Fatalf("ok=%v complete=%v, want ok and not complete (rg4 pending)", ok, p.Complete)
	}
	if p.Kinds["album"] != 2 || p.Kinds["single"] != 1 || p.Kinds["live"] != 1 {
		t.Fatalf("kinds = %v", p.Kinds)
	}
	if len(p.Albums) != 3 || len(p.Missing) != 1 || p.Missing[0] != "tracklists" {
		t.Fatalf("albums %d missing %v", len(p.Albums), p.Missing)
	}
	var cover bool
	for _, g := range p.ReleaseGroups {
		if g.ID == rg3 && g.Cover != nil && g.Cover.Version == "c3" && g.Tracklist == "stored" {
			cover = true
		}
		if g.ID == rg4 && g.Tracklist != "pending" {
			t.Fatalf("rg4 tracklist = %q", g.Tracklist)
		}
	}
	if !cover {
		t.Fatal("rg3 cover or tracklist missing")
	}
	full, _ := Build(ctx, sources(map[string]bool{rg1: true, rg2: true, rg3: true, rg4: true}), art, 1, d, time.Now())
	if !full.Complete || full.Hash == p.Hash {
		t.Fatalf("full complete=%v hash same=%v", full.Complete, full.Hash == p.Hash)
	}
	if _, ok := Build(ctx, sources(nil), rg1, 1, d, time.Now()); ok {
		t.Fatal("no artist answer stored should not build")
	}
}

func TestBuildHashIgnoresBuildTime(t *testing.T) {
	ctx := context.Background()
	src := sources(map[string]bool{rg1: true})
	a, _ := Build(ctx, src, art, 1, Depth{Max: 10}, time.Now())
	b, _ := Build(ctx, src, art, 7, Depth{Max: 10}, time.Now().Add(time.Hour))
	if a.Hash != b.Hash {
		t.Fatal("hash moved with build time or rank")
	}
	if a.Albums[rg1].FetchedAt.IsZero() {
		t.Fatal("hash must not clear the package's own fetched_at")
	}
}

func open(t *testing.T) *Store {
	s, err := Open(filepath.Join(t.TempDir(), "p.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func pkg(id, hash string) Package {
	return Package{SchemaVersion: 1, MBID: id, Hash: hash, BuiltAt: time.Now(), Artist: json.RawMessage(`{}`)}
}

func TestStoreVersionsMoveOnlyOnChange(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if ch, err := s.Put(ctx, pkg(rg1, "a")); err != nil || !ch {
		t.Fatalf("first put changed=%v err=%v", ch, err)
	}
	if ch, _ := s.Put(ctx, pkg(rg2, "b")); !ch {
		t.Fatal("second artist")
	}
	if ch, _ := s.Put(ctx, pkg(rg1, "a")); ch {
		t.Fatal("same hash must keep its version")
	}
	if s.Latest(ctx) != 2 {
		t.Fatalf("latest = %d", s.Latest(ctx))
	}
	if ch, _ := s.Put(ctx, pkg(rg1, "a2")); !ch {
		t.Fatal("changed hash")
	}
	// Since 2: only rg1 (now version 3).
	bodies, next, more, err := s.Page(ctx, 2, 10, 1<<20)
	if err != nil || len(bodies) != 1 || next != 3 || more {
		t.Fatalf("page since 2: %d bodies next %d more %v err %v", len(bodies), next, more, err)
	}
	var got Package
	_ = json.Unmarshal(bodies[0], &got)
	if got.MBID != rg1 || got.Version != 3 {
		t.Fatalf("got %s v%d", got.MBID, got.Version)
	}
	// Paging by one.
	b1, n1, m1, _ := s.Page(ctx, 0, 1, 1<<20)
	if len(b1) != 1 || n1 != 2 || !m1 {
		t.Fatalf("page 1: %d next %d more %v", len(b1), n1, m1)
	}
	b2, n2, m2, _ := s.Page(ctx, n1, 1, 1<<20)
	if len(b2) != 1 || n2 != 3 || m2 {
		t.Fatalf("page 2: %d next %d more %v", len(b2), n2, m2)
	}
	// A byte cap still answers at least one.
	b3, _, m3, _ := s.Page(ctx, 0, 10, 1)
	if len(b3) != 1 || !m3 {
		t.Fatalf("byte cap: %d more %v", len(b3), m3)
	}
	idx, next, more, _ := s.Index(ctx, 0, 10)
	if len(idx) != 2 || next != 3 || more || idx[1].MBID != rg1 {
		t.Fatalf("index %+v next %d more %v", idx, next, more)
	}
	if _, ok := s.Get(ctx, rg2); !ok {
		t.Fatal("get")
	}
}

func TestWantedKeptUntilBuilt(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Now()
	_ = s.Want(ctx, rg1, now)
	_ = s.Want(ctx, rg2, now.Add(-WantedFor-time.Hour))
	if w := s.Wanted(ctx, now); len(w) != 1 || w[0] != rg1 {
		t.Fatalf("wanted = %v (expired one should go)", w)
	}
	_, _ = s.Put(ctx, pkg(rg1, "x"))
	if w := s.Wanted(ctx, now); len(w) != 0 {
		t.Fatalf("built ask should be gone: %v", w)
	}
	if c := s.Counts(ctx); c.Packages != 1 || c.Wanted != 0 {
		t.Fatalf("counts %+v", c)
	}
}

// A picture fetched again unchanged leaves the package's hash alone.
func TestHashIgnoresFetchTimes(t *testing.T) {
	p := Package{Pictures: map[string]imgstore.Info{"photo": {Version: "v1", Source: "deezer", FetchedAt: "2026-10-30T01:00:00Z"}},
		ReleaseGroups: []Group{{Cover: &imgstore.Info{Version: "c1", FetchedAt: "2026-10-30T01:00:00Z"}}}}
	q := p
	q.Pictures = map[string]imgstore.Info{"photo": {Version: "v1", Source: "deezer", FetchedAt: "2026-10-31T09:00:00Z"}}
	q.ReleaseGroups = []Group{{Cover: &imgstore.Info{Version: "c1", FetchedAt: "2026-10-31T09:00:00Z"}}}
	if hash(p) != hash(q) {
		t.Fatal("a new fetch time changed the hash")
	}
	if p.Pictures["photo"].FetchedAt == "" || p.ReleaseGroups[0].Cover.FetchedAt == "" {
		t.Fatal("hash changed the package it was given")
	}
	q.Pictures = map[string]imgstore.Info{"photo": {Version: "v2"}}
	if hash(p) == hash(q) {
		t.Fatal("a new picture kept the hash")
	}
}
