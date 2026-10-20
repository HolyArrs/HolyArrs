package asks

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestKidFollowByTMDBKeepsTheLimit(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	mature, kids, unrated := askShow(), askShow(), askShow()
	mature.Rating, kids.Rating, unrated.Rating = "TV-MA", "TV-G", ""
	r.svc.Library.Shows = metadata.ShowSources{"tmdb": &fakeShows{shows: map[string]metadata.Show{"tmdb:101": mature, "tmdb:102": kids, "tmdb:103": unrated}}}
	kid := r.ana
	kid.MaxRating = "PG"
	for _, id := range []string{"101", "103"} {
		if _, err := r.svc.FollowShowTMDB(ctx, kid, id); !errors.Is(err, ErrAboveLimit) {
			t.Fatalf("kid follows %s: %v", id, err)
		}
		if on, _ := r.svc.IsFollowingShowTMDB(ctx, kid.UserID, id); on {
			t.Fatalf("kid follows %s", id)
		}
	}
	if _, err := r.svc.FollowShowTMDB(ctx, kid, "102"); err != nil {
		t.Fatalf("kid follows a TV-G show: %v", err)
	}
	if _, err := r.svc.FollowShowTMDB(ctx, r.bo, "101"); err != nil {
		t.Fatalf("no limit: %v", err)
	}
}

func TestLandingDropsFollowersOverTheirLimit(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	if _, err := r.svc.FollowShowTMDB(ctx, r.bo, "100"); err != nil {
		t.Fatal(err)
	}
	// ana became a Kid after following.
	now := store.FormatTime(r.clock)
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', '100', ?)", r.ana.UserID, now); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.ana.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tid, _, err := r.svc.Library.AddShow(ctx, "tmdb:100", library.ShowAddOptions{Seasons: "all"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET rating = 'TV-MA' WHERE id = ?", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.svc.LandTMDBFollows(ctx, tid); err != nil || n != 1 {
		t.Fatalf("told %d %v", n, err)
	}
	r.svc.Wait()
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); on {
		t.Fatal("over-limit follower moved to the title")
	}
	if on, _ := r.svc.IsFollowing(ctx, r.bo.UserID, tid); !on {
		t.Fatal("bo not moved")
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE user_id = ?", r.ana.UserID).Scan(&n)
	if n != 0 {
		t.Fatalf("ana got %d messages", n)
	}
	if m := r.fake.last(); len(m.People) != 1 || m.People[0].Name != "bo" {
		t.Fatalf("alert %+v", m)
	}
	_ = r.st.Read().QueryRow("SELECT count(*) FROM tmdb_follows").Scan(&n)
	if n != 0 {
		t.Fatalf("%d waiting follows left", n)
	}
}
