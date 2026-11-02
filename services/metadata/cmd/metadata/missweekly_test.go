package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// A picture no source had is never asked again by a page open or a fetch
// (the cover route answers "none"); the weekly pass alone re-asks it.
func TestMissedPictureOnlyWeekly(t *testing.T) {
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
	const id = "11111111-2222-3333-4444-555555555555"
	missed := map[string]bool{"album:" + id: true}
	due := []imgstore.Key{}
	sp := &speed{imgs: st, imgq: q,
		hasMiss:   func(_ context.Context, kind, mbid string) bool { return missed[kind+":"+mbid] },
		missesDue: func(context.Context, int) []imgstore.Key { return due }}
	ctx := context.Background()
	key := "img:album:" + id
	sp.wantImage("release-group", id, fetchq.P1Finish)
	sp.onFetched("release-group", id)
	if _, _, err := (&storeCovers{sp: sp}).Album(ctx, id, "", ""); !errors.Is(err, api.ErrNoCover) {
		t.Fatalf("cover of a miss: %v", err)
	}
	if sp.imageNow(ctx, imgstore.KindAlbum, id) {
		t.Fatal("imageNow fetched a miss")
	}
	if s, _ := q.State(ctx, key); s != "" {
		t.Fatalf("miss queued by a page open: %q", s)
	}
	sp.queueDueMisses(ctx)
	if s, _ := q.State(ctx, key); s != "" {
		t.Fatalf("miss queued before its week: %q", s)
	}
	due = []imgstore.Key{{Kind: imgstore.KindAlbum, ID: id}}
	sp.queueDueMisses(ctx)
	if s, _ := q.State(ctx, key); s != "queued" {
		t.Fatalf("weekly pass did not queue: %q", s)
	}
}
