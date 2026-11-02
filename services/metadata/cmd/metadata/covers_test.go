package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// A cover not stored yet answers "later" at once and queues the fetch at
// P0; the page open never waits on the outside source.
func TestCoverMissNeverWaits(t *testing.T) {
	dir := t.TempDir()
	st, err := imgstore.Open(imgstore.Options{Dir: filepath.Join(dir, "img")})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, err := fetchq.Open(filepath.Join(dir, "q.sqlite"), fetchq.Options{Name: "images", Rate: 1, Do: func(context.Context, string, int) error { return nil }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	c := &storeCovers{sp: &speed{imgs: st, imgq: q}}
	const id = "11111111-2222-3333-4444-555555555555"
	t0 := time.Now()
	_, cache, err := c.Album(context.Background(), id, "", "")
	if !errors.Is(err, api.ErrCoverLater) || cache != "miss" {
		t.Fatalf("%s %v", cache, err)
	}
	if d := time.Since(t0); d > 500*time.Millisecond {
		t.Fatalf("waited %v", d)
	}
	if s, _ := q.State(context.Background(), "img:"+imgstore.KindAlbum+":"+id); s != "queued" {
		t.Fatalf("not queued: %q", s)
	}
}

// Security M2: an anonymous cover ask for a release group the slim copy
// lacks is answered "no cover" with nothing queued; one it holds is queued
// at P4 (not P0), under the anonymous caller.
func TestAnonymousCoverAsk(t *testing.T) {
	dir := t.TempDir()
	st, err := imgstore.Open(imgstore.Options{Dir: filepath.Join(dir, "img")})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, err := fetchq.Open(filepath.Join(dir, "q.sqlite"), fetchq.Options{Name: "images", Rate: 1, Do: func(context.Context, string, int) error { return nil }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	const known, unknown = "11111111-2222-3333-4444-555555555555", "99999999-2222-3333-4444-555555555555"
	c := &storeCovers{sp: &speed{imgs: st, imgq: q, slimAbsent: func(_ context.Context, kind, id string) bool {
		return kind == "release-group" && id == unknown
	}}}
	ctx := api.WithCaller(context.Background(), "anon")
	if _, _, err := c.Album(ctx, unknown, "", ""); !errors.Is(err, api.ErrNoCover) {
		t.Fatalf("unknown id: %v", err)
	}
	if q.Has(ctx, "img:"+imgstore.KindAlbum+":"+unknown) {
		t.Fatal("unknown id queued")
	}
	if _, _, err := c.Album(ctx, known, "", ""); !errors.Is(err, api.ErrCoverLater) {
		t.Fatalf("known id: %v", err)
	}
	if q.Ready(ctx, fetchq.P3Refresh) != 0 || q.Ready(ctx, fetchq.P4Deep) != 1 {
		t.Fatal("anonymous cover not queued at P4")
	}
}

func TestSlimMissing(t *testing.T) {
	ctx := context.Background()
	if slimMissing(ctx, fakeSlim{date: ""}, "artist", "x") {
		t.Fatal("no copy loaded: never missing")
	}
	if !slimMissing(ctx, fakeSlim{date: "d"}, "artist", "x") {
		t.Fatal("absent id not reported")
	}
	if slimMissing(ctx, fakeSlim{date: "d", has: true}, "artist", "x") || slimMissing(ctx, fakeSlim{date: "d"}, "label", "x") {
		t.Fatal("held id or a kind the copy lacks reported")
	}
}

type fakeSlim struct {
	date string
	has  bool
}

func (f fakeSlim) Date() string                             { return f.date }
func (f fakeSlim) Has(context.Context, string, string) bool { return f.has }
