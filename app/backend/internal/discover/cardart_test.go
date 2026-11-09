package discover

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// noImages lists no images and counts every outside call.
type noImages struct {
	*metadata.Static
	calls atomic.Int32
}

func (n *noImages) CardImages(context.Context, string, string, string) (metadata.CardImages, error) {
	n.calls.Add(1)
	return metadata.CardImages{}, nil
}

// Wide card art: picked once per title and language, kept in card_art,
// read on page answers without going outside, and not fetched again while fresh.
func TestCardArtCached(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &noImages{Static: metadata.Lab()}
	c := &CardArt{Store: func() *database.Store { return s }, Metadata: func() metadata.Provider { return f }, Gap: time.Millisecond}
	ctx := context.Background()
	pool := &engine.Pool{Rows: []engine.Row{{Items: []engine.Item{{Kind: "movie", TMDBID: "7", Backdrop: "b.jpg"}, {Kind: "artist", TMDBID: "9"}}}}}
	wait := func() {
		for i := 0; i < 500; i++ {
			if !c.Running() {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("card art filler did not finish")
	}
	c.QueuePool(ctx, pool)
	wait()
	if c.Fetched() != 1 {
		t.Fatalf("one image list call expected, got %d", c.Fetched())
	}
	c.QueuePool(ctx, pool)
	wait()
	if c.Fetched() != 1 {
		t.Fatalf("a fresh pick was fetched again: %d calls", c.Fetched())
	}
	before := f.calls.Load()
	items := []ItemView{{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "7"}}}
	c.Fill(httptest.NewRequest("GET", "/x", nil), items)
	if f.calls.Load() != before {
		t.Fatal("filling a page answer went outside")
	}
	// The fake lists no images: the pick keeps the known backdrop (name variant).
	if items[0].Art != "b.jpg" || items[0].Logo != "" || items[0].HasTitle {
		t.Fatalf("pick: %+v", items[0].CardArt)
	}
}
