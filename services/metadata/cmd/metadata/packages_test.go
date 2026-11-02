package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicpkg"
	"github.com/CoderLuii/holyarrs-metadata/internal/sharedfill"
)

func pkArtist(i int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", i) }

// A pass covers the shared lists' artists and the asks, queues the walks a
// package still lacks (artist answer, release groups, picked track lists;
// never past the cap, never twice), and builds a package for each artist
// whose answer is stored: partial until its track lists are stored.
func TestPackagePass(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	var calls int
	q, err := fetchq.Open(filepath.Join(dir, "q.sqlite"), fetchq.Options{Do: func(context.Context, string, int) error { return nil }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	d, err := discog.Open(filepath.Join(dir, "d.sqlite"), discog.Options{Logf: func(string, ...any) {}, Get: func(_ context.Context, path string, _ url.Values, out any) error {
		calls++
		return json.Unmarshal([]byte(`{"release-group-count":2,"release-group-offset":0,"release-groups":[
{"id":"11111111-1111-1111-1111-111111111111","title":"A","primary-type":"Album","secondary-types":[],"first-release-date":"2001"},
{"id":"22222222-2222-2222-2222-222222222222","title":"L","primary-type":"Album","secondary-types":["Live"],"first-release-date":"2002"}]}`), out)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// Artist 0 has its release groups stored already.
	if err := d.Run(ctx, discog.ArtistKey(pkArtist(0)), fetchq.P2Shared); err != nil {
		t.Fatal(err)
	}
	var ms []sharedfill.Member
	for i := range 3 {
		ms = append(ms, sharedfill.Member{Kind: "artist", MBID: pkArtist(i)})
	}
	fill := &sharedfill.Filler{Lists: map[string]func(context.Context) []sharedfill.Member{"charts": func(context.Context) []sharedfill.Member { return ms }}, Enqueue: func(context.Context, sharedfill.Member) {}}
	fill.Once(ctx)
	st, err := musicpkg.Open(filepath.Join(dir, "p.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sp := &speed{mbq: q, disc: d, fill: fill}
	pk := &packager{sp: sp, store: st, src: musicpkg.Sources{
		Artist: func(_ context.Context, id string) ([]byte, bool) {
			return []byte(`{"name":"n"}`), id == pkArtist(0) || id == pkArtist(1)
		},
		Discog: d.Artist, Album: d.Album,
		Pictures: func(context.Context, string, string) map[string]imgstore.Info { return nil },
	}}
	// An app server's ask comes before the lists.
	if err := pk.want(ctx, pkArtist(9)); err != nil {
		t.Fatal(err)
	}
	if s, _ := q.State(ctx, api.QueueKey("artist", pkArtist(9))); s != "queued" {
		t.Fatalf("ask did not queue the artist: %q", s)
	}
	cov := pk.coverage(ctx)
	if len(cov) != 4 || cov[0].mbid != pkArtist(9) || cov[0].source != "wanted" {
		t.Fatalf("coverage %+v", cov)
	}
	old := pkgQueue
	pkgQueue = 3
	defer func() { pkgQueue = old }()
	pk.once(ctx)
	p := pk.lastPass()
	// Artist 0: two track lists; artist 1: release groups; artist 2: answer
	// and release groups (one over the cap); the ask was queued already.
	if p.Covered != 4 || p.Built != 2 || p.Partial != 2 || p.NoArtist != 2 || p.OverCap != 2 || p.Queued["tracklists"] != 2 || p.Queued["release_groups"] != 1 {
		t.Fatalf("pass %+v", p)
	}
	body, ok := st.Get(ctx, pkArtist(0))
	if !ok {
		t.Fatal("artist 0 package not stored")
	}
	var got musicpkg.Package
	_ = json.Unmarshal(body, &got)
	if got.Complete || got.Kinds["album"] != 1 || got.Kinds["live"] != 1 || len(got.ReleaseGroups) != 2 || got.ReleaseGroups[0].Tracklist != "pending" {
		t.Fatalf("package %+v", got)
	}
	// Second pass: nothing queued twice (only what the cap held back); the
	// packages keep their versions.
	latest := st.Latest(ctx)
	pk.once(ctx)
	p = pk.lastPass()
	if p.Queued["tracklists"] != 0 || p.Queued["release_groups"] != 1 || p.Queued["artist"] != 1 || p.Changed != 0 || st.Latest(ctx) != latest {
		t.Fatalf("second pass %+v latest %d->%d", p, latest, st.Latest(ctx))
	}
}

// An ask is deduplicated against the queue, queued below the shared lists,
// rejects a bad id, and asks past the per-pass cap wait for a later pass.
func TestPackageWantHardened(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	q, err := fetchq.Open(filepath.Join(dir, "q.sqlite"), fetchq.Options{Do: func(context.Context, string, int) error { return nil }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	st, err := musicpkg.Open(filepath.Join(dir, "p.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pk := &packager{sp: &speed{mbq: q}, store: st}
	if err := pk.want(ctx, "not-an-id"); err == nil {
		t.Fatal("bad id accepted")
	}
	for range 3 {
		if err := pk.want(ctx, pkArtist(1)); err != nil {
			t.Fatal(err)
		}
	}
	if ok := q.Has(ctx, api.QueueKey("artist", pkArtist(1))); !ok {
		t.Fatal("not queued")
	}
	old := pkgWantPerPass
	pkgWantPerPass = 2
	defer func() { pkgWantPerPass = old }()
	_ = pk.want(ctx, pkArtist(2))
	_ = pk.want(ctx, pkArtist(3))
	if cov := pk.coverage(ctx); len(cov) != 2 {
		t.Fatalf("cap: %d asks covered", len(cov))
	}
	if pk.ranked(pkArtist(1)) {
		t.Fatal("unranked ask must get light depth")
	}
}
