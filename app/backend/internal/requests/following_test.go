package requests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The follows list shows every follow, with the address that stops it: Library
// titles, movies and shows followed by TMDB id before they are in the Library,
// and albums. A TMDB follow whose movie has landed is listed once, as the title.
func TestFollowingListsEveryKind(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := r.svc.FollowMovieTMDB(ctx, r.ana, "900002"); err != nil {
		t.Fatal(err)
	}
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', '4242', '2026-11-01T00:00:00Z')", r.ana.UserID); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO album_follows (user_id, mbid, name, artist, release_date, created_at) VALUES (?, 'mb-1', 'Night Drive', 'The Lanterns', '2027-01-01', '2026-11-01T00:00:00Z')", r.ana.UserID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	byKind := func() map[string]Followed {
		fs, err := r.svc.Following(ctx, r.ana.UserID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]Followed{}
		for _, f := range fs {
			if _, dup := out[f.Kind]; dup {
				t.Fatalf("%s listed twice: %+v", f.Kind, fs)
			}
			out[f.Kind] = f
		}
		return out
	}
	got := byKind()
	if m := got["movie"]; m.TMDBID != "900002" || m.TitleID != 0 || m.Unfollow != "/metadata/movie/900002/follow" {
		t.Fatalf("movie by TMDB id: %+v", m)
	}
	if s := got["show"]; s.TMDBID != "4242" || s.Unfollow != "/metadata/show/4242/follow" {
		t.Fatalf("show by TMDB id: %+v", s)
	}
	if a := got["album"]; a.MBID != "mb-1" || a.Name != "Night Drive" || a.Artist != "The Lanterns" || a.Unfollow != "/metadata/album/mb-1/follow" {
		t.Fatalf("album: %+v", a)
	}
	if fs, _ := r.svc.Following(ctx, r.luis.UserID); len(fs) != 0 {
		t.Fatalf("follows are per person: %+v", fs)
	}

	// The movie lands: it is listed once, as its Library title.
	tid, _, err := r.svc.Library.Add(ctx, movie("Paper Harbour", 2023, "900002"), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.LandTMDBFollows(ctx, tid); err != nil {
		t.Fatal(err)
	}
	got = byKind()
	if m := got["movie"]; m.TitleID != tid || m.Name != "Paper Harbour" || m.TMDBID != "" || m.Unfollow != fmt.Sprintf("/titles/%d/follow", tid) {
		t.Fatalf("landed movie: %+v", m)
	}
}
