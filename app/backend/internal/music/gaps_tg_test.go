package music

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// countSource counts package-page asks.
type countSource struct{ n atomic.Int32 }

func (c *countSource) MusicPackages(_ context.Context, since int64, _ int) (metadata.PackagePage, error) {
	c.n.Add(1)
	return metadata.PackagePage{SchemaVersion: 1, Since: since, Next: since}, nil
}
func (c *countSource) WantMusicPackage(context.Context, string) error { return nil }

// startMusicSync does nothing with Music off, fills the default hooks with
// Music on, keeps a preset Pictures hook, and is safe without Packages.
func TestStartMusicSyncSetsHooksOnlyWhenMusicOn(t *testing.T) {
	st := openStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := &countSource{}
	p := &PackageSync{Store: st, Source: src}
	a := newTestAPI(Server{Store: st, Packages: p})
	a.PartIsOn = func(parts.Part) bool { return false }
	a.startMusicSync(ctx)
	time.Sleep(50 * time.Millisecond)
	if p.Pictures != nil || p.OnSynced != nil {
		t.Fatal("Music off: hooks were set")
	}
	if n := src.n.Load(); n != 0 {
		t.Fatalf("Music off: source asked %d times", n)
	}

	a.PartIsOn = func(parts.Part) bool { return true }
	a.startMusicSync(ctx)
	if p.Pictures == nil || p.OnSynced == nil {
		t.Fatal("Music on: default hooks not set")
	}

	var preset atomic.Bool
	p2 := &PackageSync{Store: st, Source: &countSource{}, Pictures: func(context.Context, string, string, string, string) bool {
		preset.Store(true)
		return true
	}}
	b := newTestAPI(Server{Store: st, Packages: p2})
	b.PartIsOn = func(parts.Part) bool { return true }
	b.startMusicSync(ctx)
	p2.Pictures(ctx, "artist", "x", "1200", "v")
	if !preset.Load() {
		t.Fatal("preset Pictures hook was overwritten")
	}

	c := newTestAPI(Server{Store: st})
	c.PartIsOn = func(parts.Part) bool { return true }
	c.startMusicSync(ctx) // no Packages: must not panic
}

func TestInFlightTakeGive(t *testing.T) {
	var f inFlight
	if !f.take(1, 2) || !f.take(1, 2) {
		t.Fatal("first two takes refused")
	}
	if f.take(1, 2) {
		t.Fatal("third take over the limit allowed")
	}
	if !f.take(2, 2) {
		t.Fatal("limit is not per person")
	}
	f.give(1)
	if !f.take(1, 2) {
		t.Fatal("take after give refused")
	}
	f.give(3) // never taken: must not go negative
	if !f.take(3, 1) || f.take(3, 1) {
		t.Fatal("give of an unknown person broke its count")
	}
}

// askableAlbums marks what the Library keeps and leaves out, and sorts oldest first.
func TestAskableAlbumsMarksKeptAndLeftOut(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	const kept, left, other = "aaaaaaaa-0000-4000-8000-000000000001", "aaaaaaaa-0000-4000-8000-000000000002", "aaaaaaaa-0000-4000-8000-000000000003"
	var tid int64
	err := st.Write(ctx, func(tx *sql.Tx) error {
		ts, by := "2026-01-01T00:00:00Z", `{"kind":"system"}`
		res, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state, keep_up_to_date)
			VALUES (?, ?, ?, ?, 'artist', 'A', 'A', '{"musicbrainz":"b8a7c51f-362c-4dcb-a259-000000000001"}', 'musicbrainz', 'by_hand', 'active', 1)`, ts, by, ts, by)
		if err != nil {
			return err
		}
		tid, _ = res.LastInsertId()
		for _, r := range []struct {
			mbid     string
			included int
		}{{kept, 1}, {left, 0}} {
			if _, err := tx.Exec(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, name, release_type, release_date, release_date_precision, source_ids, secondary_types, included, keep_up_to_date)
				VALUES (?, ?, ?, ?, ?, 'album', 'x', 'Album', '2020-01-01T00:00:00Z', 'day', ?, '[]', ?, 1)`, ts, by, ts, by, tid, `{"musicbrainz":"`+r.mbid+`"}`, r.included); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := newTestAPI(Server{Store: st})
	got, err := a.askableAlbums(ctx, tid, []metadata.Album{
		{MBID: other, Name: "C", ReleaseDate: "2022"},
		{MBID: left, Name: "B", ReleaseDate: "2021"},
		{MBID: kept, Name: "A", ReleaseDate: "2020"},
	})
	if err != nil || len(got) != 3 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got[0].MBID != kept || got[1].MBID != left || got[2].MBID != other {
		t.Fatalf("order %v %v %v", got[0].MBID, got[1].MBID, got[2].MBID)
	}
	if !got[0].Kept || got[0].LeftOut {
		t.Fatalf("kept album %+v", got[0])
	}
	if !got[1].LeftOut {
		t.Fatalf("left-out album %+v", got[1])
	}
	if got[2].Kept || got[2].LeftOut || got[2].SecondaryTypes == nil {
		t.Fatalf("unknown album %+v", got[2])
	}
	if _, err := a.askableAlbums(ctx, 0, []metadata.Album{{MBID: kept}}); err != nil {
		t.Fatalf("no title: %v", err)
	}
}
