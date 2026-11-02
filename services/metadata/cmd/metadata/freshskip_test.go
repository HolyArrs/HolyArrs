package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// A fetched answer queues its picture only when it is not stored fresh:
// re-checks of stored pictures are the refresh loop's (P4), so they never
// crowd the P1 line ahead of pictures not stored yet.
func TestOnFetchedQueuesOnlyPicturesNotFresh(t *testing.T) {
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
	fresh := map[string]bool{}
	sp := &speed{imgs: st, imgq: q, fresh: func(_ context.Context, kind, mbid string) bool { return fresh[kind+":"+mbid] }}
	ctx := context.Background()
	fresh[imgstore.KindAlbum+":"+id] = true
	sp.onFetched("release-group", id)
	if s, _ := q.State(ctx, "img:album:"+id); s != "" {
		t.Fatalf("fresh picture queued: %q", s)
	}
	if err := sp.runImage(ctx, "img:album:"+id, fetchq.P1Finish); err != nil {
		t.Fatalf("fresh picture job: %v", err)
	}
	fresh[imgstore.KindAlbum+":"+id] = false
	sp.onFetched("release-group", id)
	if s, _ := q.State(ctx, "img:album:"+id); s != "queued" {
		t.Fatalf("missing picture not queued: %q", s)
	}
}
