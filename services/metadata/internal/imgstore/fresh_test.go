package imgstore

import (
	"context"
	"testing"
)

// Fresh: a stored picture whose re-check is not due yet; a missing one is
// never fresh.
func TestFreshUntilRecheckDue(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde (Deluxe)", "Frank Ocean", 3000))
	ctx := context.Background()
	if g.s.Fresh(ctx, KindAlbum, albumID) {
		t.Fatal("missing picture fresh")
	}
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if !g.s.Fresh(ctx, KindAlbum, albumID) {
		t.Fatal("just stored picture not fresh")
	}
	if _, err := g.s.db.Exec(`UPDATE images SET recheck=1 WHERE kind=? AND mbid=?`, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	g.s.forget(KindAlbum, albumID)
	if g.s.Fresh(ctx, KindAlbum, albumID) {
		t.Fatal("picture due a re-check still fresh")
	}
}
