package requests

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A movie follow made before the person got a rating limit is dropped when
// the movie lands in the Library above that limit: no follow, no ask, no
// message. Followers without a limit still move over.
func TestTMDBFollowSkipsOverRatedLibraryTitle(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, p := range []Person{r.ana, r.luis} {
		if err := r.svc.FollowMovieTMDB(ctx, p, "900002"); err != nil {
			t.Fatal(err)
		}
	}
	// ana became a Kid after following.
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG-13' WHERE id = ?", r.ana.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := r.svc.LandTMDBFollows(ctx, tid); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if on, _ := r.svc.IsFollowing(ctx, r.ana.UserID, tid); on {
		t.Fatal("over-limit follower moved to the title")
	}
	if on, _ := r.svc.IsFollowingMovieTMDB(ctx, r.ana.UserID, "900002"); on {
		t.Fatal("over-limit follow kept waiting")
	}
	if on, _ := r.svc.IsFollowing(ctx, r.luis.UserID, tid); !on {
		t.Fatal("follower without a limit not moved")
	}
	var n int
	if err := r.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ?", r.ana.UserID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ana has %d asks (%v)", n, err)
	}
	if err := r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE user_id = ?", r.ana.UserID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ana got %d messages (%v)", n, err)
	}
}
