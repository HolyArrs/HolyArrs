package sources

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A Release found by an album's Choose a release search whose name gives no
// album can still be picked with force for that album's tracks.
func TestChoose_AlbumSearchReleaseWithNoAlbumInName(t *testing.T) {
	const odd = "Velvet Cartography Low Tide Atlas DE 2019 FLAC"
	ms := &musicServer{answer: func(url.Values) []string { return []string{odd} }}
	f := seedArtist(t, ms.start(t))
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, map[string]any{"choose": false, "by_hand": true})
	r := relByName(t, f.fixture, odd)
	if len(r.MatchedSlotIDs) != 0 {
		t.Fatalf("the name should not match an album here: %v", r.MatchedSlotIDs)
	}
	ctx := context.Background()
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, r.ID, 0, false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("without force: %v, want a force question", err)
	}
	ch, err := sr.Choose(ctx, r.ID, 0, true, domain.System)
	if err != nil || !ch.Forced || len(ch.SlotIDs) != len(ltaTracks) {
		t.Fatalf("with force: %+v %v", ch, err)
	}
	for _, l := range ltaTracks {
		if !slices.Contains(ch.SlotIDs, f.slots[l]) {
			t.Errorf("%s not picked", l)
		}
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Errorf("%s: %s", l, st)
		}
	}
	if st := f.state(t, "Harbour Songs 1-1"); st != mediafiles.SlotNotOutYet {
		t.Errorf("other album touched: %s", st)
	}
}
