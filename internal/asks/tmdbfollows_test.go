package asks

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/alerts"
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

func TestFollowMovieByTMDBLandsOnce(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	kid := r.bo
	kid.MaxRating = "PG"
	r.svc.Metadata.(*metadata.Static).Movies[1].Rating = "R"
	if err := r.svc.FollowMovieTMDB(ctx, kid, "900002"); !errors.Is(err, ErrAboveLimit) {
		t.Fatalf("kid follows an R movie: %v", err)
	}
	if err := r.svc.FollowMovieTMDB(ctx, r.ana, "abc"); !errors.Is(err, ErrBadTMDBID) {
		t.Fatalf("bad id: %v", err)
	}
	if err := r.svc.FollowMovieTMDB(ctx, r.ana, "900002"); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.svc.IsFollowingMovieTMDB(ctx, r.ana.UserID, "900002"); !on {
		t.Fatal("not following")
	}
	if on, _ := r.svc.IsFollowingMovieTMDB(ctx, r.luis.UserID, "900002"); on {
		t.Fatal("a movie follow is per person")
	}
	tid, _, err := r.svc.Library.Add(ctx, movie("Paper Harbour", 2023, "900002"), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.svc.LandTMDBFollows(ctx, tid); err != nil || n != 1 {
		t.Fatalf("moved %d %v", n, err)
	}
	if on, _ := r.svc.IsFollowingMovieTMDB(ctx, r.ana.UserID, "900002"); !on {
		t.Fatal("the follow should go on as the Library title once the movie lands")
	}
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); !on {
		t.Fatal("the landed movie is followed as its Library title")
	}
	ready := func() int {
		var msgs int
		if err := r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE user_id = ? AND kind = 'ready' AND title_id = ?", r.ana.UserID, tid).Scan(&msgs); err != nil {
			t.Fatal(err)
		}
		return msgs
	}
	// Added is not ready: no word until the movie's file is in.
	if n, err := r.svc.TellMovieFollows(ctx); err != nil || n != 0 || ready() != 0 || r.fake.count(alerts.MomentReady) != 0 {
		t.Fatalf("told %d before the file is in (%v), %d messages", n, err, ready())
	}
	if n, _ := r.svc.LandTMDBFollows(ctx, tid); n != 0 {
		t.Fatal("moved twice")
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The ready check after the import tells ana once.
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if ready() != 1 {
		t.Fatalf("%d ready messages", ready())
	}
	if m := r.fake.last(); m.Moment != alerts.MomentReady || len(m.People) != 1 || m.People[0].Name != "ana" || m.Subject != "Paper Harbour is in the Library" {
		t.Fatalf("alert %+v", m)
	}
	if n, _ := r.svc.TellMovieFollows(ctx); n != 0 || ready() != 1 {
		t.Fatal("told twice")
	}
	if err := r.svc.UnfollowMovieTMDB(ctx, r.ana.UserID, "900002"); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); on {
		t.Fatal("unfollow by TMDB id ends the Library follow")
	}
	// A movie in the Library is followed as its title.
	if err := r.svc.FollowMovieTMDB(ctx, r.luis, "900002"); err != nil {
		t.Fatalf("in the Library: %v", err)
	}
	if on, _ := r.svc.IsFollowing(ctx, r.luis.UserID, tid); !on {
		t.Fatal("following a Library movie by TMDB id follows the title")
	}
}
