package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Pins today's music reads over parts, items, slots, versions and
// pressings (Albums, Totals, track gates) before they move to the catalog
// store and the music module.

func t10LabArtist(t *testing.T) (*Service, *database.Store, int64) {
	t.Helper()
	svc, s, _, _ := musicService(t)
	id := addLabArtist(t, svc, "all")
	return svc, s, id
}

func TestT10PinAlbumsSummaryGolden(t *testing.T) {
	_, s, id := t10LabArtist(t)
	ctx := context.Background()
	// Spread the track states over the albums: a file, a failed track, a track
	// with no slot for the main Version (NULL partner), a track not kept.
	var slots []int64
	rows, err := s.Read().Query("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.kind = 'track' AND i.title_id = ? ORDER BY s.id", id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sid int64
		_ = rows.Scan(&sid)
		slots = append(slots, sid)
	}
	rows.Close()
	if len(slots) < 4 {
		t.Fatalf("only %d track slots", len(slots))
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE id = ?", slots[0]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state = 'failed' WHERE id = ?", slots[1]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state = 'better_wanted' WHERE id = ?", slots[2]); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM slots WHERE id = ?", slots[3])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Albums(ctx, s.Read(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("albums %+v", got)
	}
	// The album summaries stay in step with the totals query.
	tot, err := Totals(ctx, s.Read(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	var tracks, inLib, better, notOut, missing int
	for _, a := range got {
		tracks += a.Tracks
		inLib += a.InLibrary
		better += a.BetterWanted
		notOut += a.NotOutYet
		missing += a.Missing
	}
	if tot.Albums != len(got) || tot.Tracks != tracks || tot.InLibrary != inLib || tot.BetterWanted != better || tot.NotOutYet != notOut || tot.Missing != missing {
		t.Fatalf("totals %+v albums: tracks %d lib %d better %d notout %d missing %d", tot, tracks, inLib, better, notOut, missing)
	}
	if tot.Failed != 1 || tot.InLibrary != 2 {
		t.Fatalf("totals %+v", tot)
	}
	b, _ := json.MarshalIndent(got, "", "  ")
	t10Golden(t, "album_summary_pin.golden", b)
	tb, _ := json.MarshalIndent(tot, "", "  ")
	t10Golden(t, "artist_totals_pin.golden", tb)
}

func TestT10PinAlbumsAndTotalsForUnknownArtist(t *testing.T) {
	_, s, _ := t10LabArtist(t)
	ctx := context.Background()
	got, err := Albums(ctx, s.Read(), 424242)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("albums %v %v", got, err)
	}
	tot, err := Totals(ctx, s.Read(), 424242, now)
	if err != nil || tot.Albums != 0 || tot.Tracks != 0 || tot.NextRelease != nil {
		t.Fatalf("totals %+v %v", tot, err)
	}
}

// OpenTrackGates opens the waiting tracks whose album is out and returns how
// many Slots it opened; a removed artist is left alone.
func TestT10PinOpenTrackGatesTwoArtists(t *testing.T) {
	svc, s, id := t10LabArtist(t)
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		// Make every track of the artist wait on a past release date, then
		// clone the artist as a removed one with the same waiting tracks.
		if _, err := tx.Exec("UPDATE slots SET state = 'not_out_yet' WHERE item_id IN (SELECT id FROM items WHERE kind = 'track' AND title_id = ?)", id); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET air_date = '2020-01-01T00:00:00.000000000Z' WHERE kind = 'track' AND title_id = ?", id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var waiting int
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE state = 'not_out_yet' AND item_id IN (SELECT id FROM items WHERE kind = 'track' AND title_id = ?)", id).Scan(&waiting)
	// The artist is removed: nothing opens.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.OpenTrackGates(ctx); err != nil || n != 0 {
		t.Fatalf("removed artist opened %d %v", n, err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'active' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	n, err := svc.OpenTrackGates(ctx)
	if err != nil || n != waiting {
		t.Fatalf("opened %d %v, want every waiting track (%d)", n, err, waiting)
	}
	var left int
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE state = 'not_out_yet' AND item_id IN (SELECT id FROM items WHERE kind = 'track' AND title_id = ?)", id).Scan(&left)
	if left != 0 {
		t.Fatalf("%d of %d tracks still waiting", left, waiting)
	}
	if n, err := svc.OpenTrackGates(ctx); err != nil || n != 0 {
		t.Fatalf("second run opened %d %v", n, err)
	}
}
