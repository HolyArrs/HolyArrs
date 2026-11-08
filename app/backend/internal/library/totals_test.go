package library

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TestAlbumsBatchMatchesOne: the batched Albums answers what AlbumOf
// answers album by album, and Totals adds up to the same counts.
func TestAlbumsBatchMatchesOne(t *testing.T) {
	svc, s, _, _ := musicService(t)
	id := addLabArtist(t, svc, AlbumsAll)
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE slots SET state = 'in_library' WHERE id = (SELECT min(id) FROM slots)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sums, err := Albums(ctx, s.Read(), id)
	if err != nil || len(sums) == 0 {
		t.Fatalf("albums %v %v", sums, err)
	}
	parts, err := database.Select[mediafiles.Part](ctx, s.Read(), "parts", "title_id = ? AND kind = 'album'", id)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]mediafiles.Part{}
	for _, p := range parts {
		byID[p.ID] = p
	}
	var want ArtistTotals
	want.Albums = len(sums)
	for _, a := range sums {
		one, err := AlbumOf(ctx, s.Read(), byID[a.ID])
		if err != nil || !reflect.DeepEqual(one, a) {
			t.Fatalf("batched %+v, one by one %+v (%v)", a, one, err)
		}
		if a.Included {
			want.Included++
			if a.ReleaseDate != nil && a.ReleaseDate.After(now) && (want.NextRelease == nil || a.ReleaseDate.Before(*want.NextRelease)) {
				d := *a.ReleaseDate
				want.NextRelease = &d
			}
		}
		want.Tracks += a.Tracks
		want.InLibrary += a.InLibrary
		want.Missing += a.Missing
		want.NotOutYet += a.NotOutYet
		want.BetterWanted += a.BetterWanted
	}
	if want.InLibrary == 0 || want.Missing == 0 {
		t.Fatalf("test data too thin: %+v", want)
	}
	got, err := Totals(ctx, s.Read(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	if (got.NextRelease == nil) != (want.NextRelease == nil) || (got.NextRelease != nil && !got.NextRelease.Equal(*want.NextRelease)) {
		t.Fatalf("next release %v, want %v", got.NextRelease, want.NextRelease)
	}
	got.NextRelease, want.NextRelease = nil, nil
	if got != want {
		t.Fatalf("totals %+v, want %+v", got, want)
	}
}
