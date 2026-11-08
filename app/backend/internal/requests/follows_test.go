package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

func TestFollowShowGetsNewEpisodeNotices(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	tid, _, err := shows.New(r.svc.Library).AddShow(ctx, "tmdb:100", shows.ShowAddOptions{Seasons: "all"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Follow(ctx, r.bo, tid); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Follow(ctx, r.bo, tid); err != nil { // twice is fine
		t.Fatal(err)
	}
	// Following adds no ask and changes nothing in the Library.
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks").Scan(&n)
	if n != 0 {
		t.Fatalf("%d asks", n)
	}
	fs, err := r.svc.Following(ctx, r.bo.UserID)
	if err != nil || len(fs) != 1 || fs[0].TitleID != tid || fs[0].Name != "Lab Show" || fs[0].Kind != "show" {
		t.Fatalf("following %+v %v", fs, err)
	}
	if on, _ := r.svc.IsFollowing(ctx, r.bo.UserID, tid); !on {
		t.Fatal("not following")
	}
	var slot int64
	_ = r.st.Read().QueryRow(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND i.kind = 'episode' ORDER BY s.id LIMIT 1`, tid).Scan(&slot)
	r.svc.WatchEpisodes()
	if err := r.st.AddEvent(ctx, &domain.Event{Kind: "import_done", TitleID: &tid, Data: map[string]any{"placed_slot_ids": []int64{slot}}}); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	r.clock = r.clock.Add(61 * time.Minute)
	if n, err := r.svc.SendEpisodeNotices(ctx); err != nil || n != 1 {
		t.Fatalf("sent %d %v", n, err)
	}
	r.svc.Wait()
	m := r.fake.last()
	if m.Moment != alerts.MomentNewEpisodes || len(m.People) != 1 || m.People[0].Name != "bo" {
		t.Fatalf("alert %+v", m)
	}
	_ = r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE kind = 'new_episodes' AND user_id = ? AND title_id = ?", r.bo.UserID, tid).Scan(&n)
	if n != 1 {
		t.Fatalf("bo's messages %d", n)
	}
	// Unfollowed: no more notices.
	if err := r.svc.Unfollow(ctx, r.bo.UserID, tid); err != nil {
		t.Fatal(err)
	}
	if fs, _ := r.svc.Following(ctx, r.bo.UserID); len(fs) != 0 {
		t.Fatalf("still following %+v", fs)
	}
}

func TestFollowRefusals(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	tid, _, err := shows.New(r.svc.Library).AddShow(ctx, "tmdb:100", shows.ShowAddOptions{Seasons: "all"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET rating = 'TV-MA' WHERE id = ?", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kid := r.ana
	kid.MaxRating = "PG"
	if err := r.svc.Follow(ctx, kid, tid); !errors.Is(err, ErrAboveLimit) {
		t.Fatalf("kid follow: %v", err)
	}
	if err := r.svc.Follow(ctx, r.bo, 99999); !errors.Is(err, ErrNotFollowable) {
		t.Fatalf("missing: %v", err)
	}
}

func TestFollowArtistGetsNewAlbumNotices(t *testing.T) {
	r := musicRig(t)
	ctx := context.Background()
	tid, _, err := r.svc.Library.AddArtist(ctx, "mbid:"+mArtist, library.ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Follow(ctx, r.bo, tid); err != nil {
		t.Fatal(err)
	}
	// One track of First Light arrives.
	var slots []int64
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		var partID, versionID int64
		if err := tx.QueryRow("SELECT id FROM parts WHERE title_id = ? AND name = 'First Light'", tid).Scan(&partID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM versions WHERE title_id = ? LIMIT 1", tid).Scan(&versionID); err != nil {
			return err
		}
		one := 1
		it := &mediafiles.Item{TitleID: tid, PartID: &partID, Kind: "track", Numbers: mediafiles.Numbers{Track: &one}, SourceIDs: map[string]string{}}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		id, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: versionID, State: mediafiles.SlotWanted}, domain.System)
		slots = append(slots, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r.svc.WatchEpisodes()
	if err := r.st.AddEvent(ctx, &domain.Event{Kind: "import_done", TitleID: &tid, Data: map[string]any{"placed_slot_ids": slots}}); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	r.clock = r.clock.Add(61 * time.Minute)
	if n, err := r.svc.SendAlbumNotices(ctx); err != nil || n != 1 {
		t.Fatalf("sent %d %v", n, err)
	}
	r.svc.Wait()
	m := r.fake.last()
	if m.Moment != alerts.MomentNewAlbums || m.Subject != "A new album from Lab Band" || len(m.People) != 1 || m.People[0].Name != "bo" {
		t.Fatalf("alert %+v", m)
	}
	if n, _ := r.svc.SendAlbumNotices(ctx); n != 0 {
		t.Fatalf("again %d", n)
	}
}

func TestAskersFollowOnceApproved(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.FollowAsk(ctx, r.ana, res.Ask.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	// Bo unticks Follow.
	if err := r.svc.FollowAsk(ctx, r.bo, res.Ask.ID, false); err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tid := *a.TitleID
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); !on {
		t.Fatal("the asker follows once the ask is approved")
	}
	if on, _ := r.svc.IsFollowing(ctx, r.bo.UserID, tid); on {
		t.Fatal("an asker who unticked Follow does not follow")
	}
	if on, _ := r.svc.IsFollowing(ctx, r.luis.UserID, tid); on {
		t.Fatal("the approver does not follow")
	}
	// Ticking Follow later on an approved ask follows straight away.
	if err := r.svc.FollowAsk(ctx, r.bo, res.Ask.ID, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.svc.IsFollowing(ctx, r.bo.UserID, tid); !on {
		t.Fatal("follow on an approved ask starts now")
	}
	// Titles added by hand can be followed too (movies included).
	if err := r.svc.FollowAdded(ctx, r.luis, tid, 99999); err != nil {
		t.Fatal(err)
	}
	if fs, _ := r.svc.Following(ctx, r.luis.UserID); len(fs) != 1 || fs[0].Kind != "movie" {
		t.Fatalf("luis follows %+v", fs)
	}
}
