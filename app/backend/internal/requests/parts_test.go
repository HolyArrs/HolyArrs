package requests

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestStoredEpisodeListsOfAShowAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{2})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := r.svc.AskParts(ctx, res.Ask.ID); len(p.Seasons) != 0 {
		t.Fatalf("stored before asked to: %+v", p)
	}
	if err := r.svc.StoreParts(ctx, res.Ask.ID); err != nil {
		t.Fatal(err)
	}
	p, err := r.svc.AskParts(ctx, res.Ask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Seasons["1"]) != 2 || len(p.Seasons["2"]) != 2 || p.Seasons["2"][0].Name != "Return" {
		t.Fatalf("seasons %+v", p.Seasons)
	}
}

func TestStoredSongListsOfAnArtistAsk(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	fa := r.svc.Library.Artists.(*fakeArtists)
	fa.pressings = map[string][]metadata.Pressing{mFirst: {
		{MBID: "p2", Status: "official", Disambiguation: "deluxe", Tracks: []metadata.Track{{Disc: 1, Number: 1, Name: "A"}, {Disc: 1, Number: 2, Name: "B"}, {Disc: 1, Number: 3, Name: "Bonus"}}},
		{MBID: "p1", Status: "official", Date: "2010", Tracks: []metadata.Track{{Disc: 1, Number: 1, Name: "A", RecordingID: "r1"}, {Disc: 1, Number: 2, Name: "B", RecordingID: "r2"}}},
	}}
	res, err := r.svc.AskArtist(ctx, r.ana, "mbid:"+mArtist, []string{mFirst, mLive})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.StoreParts(ctx, res.Ask.ID); err != nil {
		t.Fatal(err)
	}
	p, err := r.svc.AskParts(ctx, res.Ask.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The plain edition's songs; an album with no songs yet is left out.
	if len(p.Albums) != 1 || len(p.Albums[mFirst]) != 2 || p.Albums[mFirst][1].RecordingID != "r2" {
		t.Fatalf("albums %+v", p.Albums)
	}
}
