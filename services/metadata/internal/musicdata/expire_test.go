package musicdata

import (
	"context"
	"testing"
)

// Expire makes a kept answer due: the next read asks the sources again.
func TestExpire(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.s.keep("top:"+artistA, TopSongs{Source: "deezer", Songs: []Song{{Title: "x"}}}, keepTopSongs)
	var top TopSongs
	if found, _ := g.s.cached(ctx, "top:"+artistA, &top); !found {
		t.Fatal("not kept")
	}
	g.s.Expire(ctx, "top:"+artistA)
	if found, _ := g.s.cached(ctx, "top:"+artistA, &top); found {
		t.Fatal("still kept")
	}
}

// Retag asks again for "none" answers kept before tagging (no fetch
// time), so they too carry a source and a fetch date.
func TestRetagNoneAnswers(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.artist(artistA, "Nobody")
	g.f.set("/spotify/v1/search?Nobody", map[string]any{"artists": map[string]any{"items": []any{}}})
	g.f.set("/v1/catalog/us/search?Nobody", map[string]any{"results": map[string]any{}})
	g.f.set("/deezer/search/artist?Nobody", map[string]any{"data": []any{}})
	g.s.keep("top:"+artistA, nil, keepTopSongs)
	if _, err := g.s.db.Exec(`UPDATE answers SET fetched_at=0, src='musicbrainz' WHERE k=?`, "top:"+artistA); err != nil {
		t.Fatal(err)
	}
	if n := g.s.Retag(ctx, 0); n != 1 {
		t.Fatalf("asked %d, want 1", n)
	}
	var src string
	var at int64
	if err := g.s.db.QueryRow(`SELECT src, fetched_at FROM answers WHERE k=?`, "top:"+artistA).Scan(&src, &at); err != nil {
		t.Fatal(err)
	}
	if src == "" || at <= 0 {
		t.Fatalf("src %q fetched_at %d", src, at)
	}
	if n := g.s.Retag(ctx, 0); n != 0 {
		t.Fatalf("asked a none answer again: %d", n)
	}
}
