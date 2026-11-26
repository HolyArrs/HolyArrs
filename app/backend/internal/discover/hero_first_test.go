package discover

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// logoImages lists one backdrop and one logo per title and counts calls.
type logoImages struct {
	*metadata.Static
	calls atomic.Int32
}

func (l *logoImages) CardImages(_ context.Context, _, id, _ string) (metadata.CardImages, error) {
	l.calls.Add(1)
	en := "en"
	return metadata.CardImages{
		Backdrops: []metadata.CardImage{{Path: "/b" + id + ".jpg"}},
		Logos:     []metadata.CardImage{{Path: "/l" + id + ".png", Lang: &en}},
	}, nil
}

// On a fresh install no card pick is stored yet. The hero's titles must get
// theirs on the first answer (fetched now), not after the paced filler,
// and a stored pick is never fetched again.
func TestFillNowFetchesMissingHeroArt(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &logoImages{Static: metadata.Lab()}
	c := &CardArt{Store: func() *database.Store { return s }, Metadata: func() metadata.Provider { return f }}
	items := []ItemView{
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "7", Backdrop: "b7.jpg"}},
		{DiscoverItem: metadata.DiscoverItem{Kind: "show", TMDBID: "8", Backdrop: "b8.jpg"}},
	}
	r := httptest.NewRequest("GET", "/x", nil)
	c.FillNow(r, items, 8, 5*time.Second)
	for _, v := range items {
		if !HeroArtReady(v) {
			t.Fatalf("hero art not ready on first answer: %+v", v.CardArt)
		}
	}
	before := f.calls.Load()
	again := []ItemView{{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "7"}}}
	c.FillNow(r, again, 8, 5*time.Second)
	if f.calls.Load() != before || again[0].Logo == "" {
		t.Fatalf("stored pick fetched again or not read: calls %d -> %d", before, f.calls.Load())
	}
}

// Titles with art but no known trailer yet top up the hero, so a fresh
// install (no trailers looked up yet) still gets one; the trailer joins later.
func TestHeroReadyArtOnlyTopsUp(t *testing.T) {
	t.Parallel()
	art := metadata.CardArt{Logo: "l.png", Art: "a.jpg"}
	vs := []ItemView{
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "1"}, CardArt: art},
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "2"}},
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "3"}, CardArt: art, TrailerKey: "k"},
	}
	got := HeroReady(vs, 6)
	if len(got) != 2 || got[0].TMDBID != "3" || got[1].TMDBID != "1" {
		t.Fatalf("hero ready %+v", got)
	}
}

// On a slow server no logo may have come in yet: the hero still shows the
// titles that have a backdrop (their name stands in for the logo), never
// nothing.
func TestHeroReadyFallsBackToBackdrops(t *testing.T) {
	t.Parallel()
	vs := []ItemView{
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "1"}},
		{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "2", Backdrop: "b.jpg"}},
	}
	got := HeroReady(vs, 6)
	if len(got) != 1 || got[0].TMDBID != "2" {
		t.Fatalf("hero ready %+v", got)
	}
}
