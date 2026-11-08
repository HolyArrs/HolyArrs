package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
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
	now := database.FormatTime(r.clock)
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', '100', ?)", r.ana.UserID, now); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.ana.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
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

func TestFollowLibraryMovieWithoutFileTellsOnImport(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tid, _, err := r.svc.Library.Add(ctx, movie("Paper Harbour", 2023, "900002"), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET rating = 'R' WHERE id = ?", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ready := func(uid int64) int {
		var n int
		if err := r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE user_id = ? AND kind = 'ready' AND title_id = ?", uid, tid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// ana follows by TMDB id, luis by the title's bell and then stops; a Kid over the limit cannot follow.
	if err := r.svc.FollowMovieTMDB(ctx, r.ana, "900002"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.FollowTitle(ctx, r.luis, tid); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Unfollow(ctx, r.luis.UserID, tid); err != nil {
		t.Fatal(err)
	}
	kid := r.bo
	kid.MaxRating = "PG"
	if err := r.svc.FollowTitle(ctx, kid, tid); !errors.Is(err, ErrAboveLimit) {
		t.Fatalf("kid follows an R movie: %v", err)
	}
	if n, err := r.svc.TellMovieFollows(ctx); err != nil || n != 0 {
		t.Fatalf("told %d before the file is in (%v)", n, err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if ready(r.ana.UserID) != 1 || ready(r.luis.UserID) != 0 || ready(r.bo.UserID) != 0 {
		t.Fatalf("ready messages ana %d luis %d bo %d", ready(r.ana.UserID), ready(r.luis.UserID), ready(r.bo.UserID))
	}
	if r.fake.count(alerts.MomentReady) != 1 {
		t.Fatalf("%d Ready alerts", r.fake.count(alerts.MomentReady))
	}
	if m := r.fake.last(); m.Moment != alerts.MomentReady || len(m.People) != 1 || m.People[0].Name != "ana" {
		t.Fatalf("alert %+v", m)
	}
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); !on {
		t.Fatal("ana still follows after being told")
	}
	// bo follows once the file is in: a later import or upgrade tells nobody.
	if err := r.svc.FollowTitle(ctx, r.bo, tid); err != nil {
		t.Fatal(err)
	}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'better_wanted' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if ready(r.ana.UserID) != 1 || ready(r.bo.UserID) != 0 || r.fake.count(alerts.MomentReady) != 1 {
		t.Fatalf("told again: ana %d bo %d alerts %d", ready(r.ana.UserID), ready(r.bo.UserID), r.fake.count(alerts.MomentReady))
	}
}
