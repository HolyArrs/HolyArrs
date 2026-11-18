package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// mineWorld has a Plex and a Jellyfin server with watch history on, the
// movie and show of watchTitles, and three people: 7 (ana) linked on both
// servers, 8 (Rae R) linked to Plex by a plex.tv id, 9 (bo) with no link.
func mineWorld(t *testing.T) (w *world, movie, show int64) {
	t.Helper()
	w = newWorld(t)
	ctx := context.Background()
	movie, show = watchTitles(t, w)
	watchOn(t, w, true)
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		for _, m := range []*domain.MediaServer{{Name: "Plex", Kind: "plex", URL: "http://plex.test", Enabled: true}, {Name: "Jelly", Kind: "jellyfin", URL: "http://jelly.test", Enabled: true}} {
			if err := insertServer(tx, m, domain.System); err != nil {
				return err
			}
		}
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
				(7, 't', '{}', 't', '{}', 'Ana B', 'member', ''), (8, 't', '{}', 't', '{}', 'Luis R', 'member', ''), (9, 't', '{}', 't', '{}', 'Bo', 'member', 'x')`,
			`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES
				(7, 'plex', '22', 'ana', 't'), (7, 'jellyfin', 'u1', 'ana', 't'), (8, 'plex', '9876543', 'Luis', 't')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, movie, show
}

// seen writes one watch_state row; a movie has season and episode 0.
func seen(t *testing.T, w *world, server, account, name string, title int64, season, episode int) {
	t.Helper()
	kind := "episode"
	if season == 0 && episode == 0 {
		kind = "movie"
	}
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO watch_state(media_server_id, account_key, account_name, title_id, kind, season, episode, plays, read_at)
			VALUES ((SELECT id FROM media_servers WHERE name = ?), ?, ?, ?, ?, ?, ?, 1, 't')`, server, account, name, title, kind, season, episode)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mine(t *testing.T, w *world, user int64) MyWatched {
	t.Helper()
	out, err := WatchedBy(context.Background(), w.s.Read(), user)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A person gets what their own linked accounts watched, an episode seen on
// two servers counted once, and nothing of anybody else's.
func TestWatchedByOwnAccountsOnly(t *testing.T) {
	w, movie, show := mineWorld(t)
	seen(t, w, "Plex", "22", "ana", show, 1, 1)
	seen(t, w, "Plex", "22", "ana", show, 1, 2)
	seen(t, w, "Jelly", "u1", "ana", show, 1, 2)
	seen(t, w, "Jelly", "u1", "ana", show, 2, 1)
	seen(t, w, "Jelly", "u1", "ana", movie, 0, 0)
	// Somebody else's rows on the same servers.
	seen(t, w, "Plex", "33", "kid", show, 3, 1)
	seen(t, w, "Jelly", "u2", "kid", show, 1, 3)

	got := mine(t, w, 7)
	want := MyWatched{Enabled: true, Linked: true, Movies: []int64{movie}, Shows: map[int64]map[int]int{show: {1: 2, 2: 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ana %+v, want %+v", got, want)
	}

	// Rae R is linked, and watched nothing.
	if got := mine(t, w, 8); !got.Enabled || !got.Linked || len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("luis %+v", got)
	}
	// Bo signs in with a password only: no link, so nothing, and it says so.
	if got := mine(t, w, 9); !got.Enabled || got.Linked || len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("bo %+v", got)
	}
	// Nobody.
	if got := mine(t, w, 0); got.Linked || len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("nobody %+v", got)
	}
}

// The answer always carries its lists, so a page never reads a missing one.
func TestWatchedByAnswersEmptyLists(t *testing.T) {
	w, _, _ := mineWorld(t)
	b, err := json.Marshal(mine(t, w, 9))
	if err != nil || string(b) != `{"enabled":true,"linked":false,"movies":[],"shows":{}}` {
		t.Fatalf("%s %v", b, err)
	}
}

// The Plex server's owner is account 1 there; the one person whose Plex
// name matches gets its rows, and nobody else does.
func TestWatchedByPlexOwnerByName(t *testing.T) {
	w, movie, _ := mineWorld(t)
	seen(t, w, "Plex", "1", "luis", movie, 0, 0)
	if got := mine(t, w, 8); !reflect.DeepEqual(got.Movies, []int64{movie}) {
		t.Fatalf("the owner %+v", got)
	}
	if got := mine(t, w, 7); len(got.Movies) != 0 {
		t.Fatalf("ana got the owner's rows %+v", got)
	}
	// The same name on a Jellyfin account 1 is not the Plex owner.
	seen(t, w, "Jelly", "1", "ana", movie, 0, 0)
	if got := mine(t, w, 7); len(got.Movies) != 0 {
		t.Fatalf("a Jellyfin account 1 linked by name %+v", got)
	}
}

// Two people with the same Plex name: account 1 belongs to neither.
func TestWatchedByPlexOwnerNameSharedByTwo(t *testing.T) {
	w, movie, _ := mineWorld(t)
	seen(t, w, "Plex", "1", "luis", movie, 0, 0)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (9, 'plex', '555', 'LUIS', 't')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, u := range []int64{8, 9} {
		if got := mine(t, w, u); len(got.Movies) != 0 {
			t.Fatalf("person %d got account 1's rows %+v", u, got)
		}
	}
}

// With watch history off nothing is answered, even from rows a read that
// was still going left behind.
func TestWatchedByOff(t *testing.T) {
	w, movie, _ := mineWorld(t)
	seen(t, w, "Plex", "22", "ana", movie, 0, 0)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, watchGroup, watchKey, "off", domain.System, time.Now().UTC())
	}); err != nil {
		t.Fatal(err)
	}
	if got := mine(t, w, 7); got.Enabled || got.Linked || len(got.Movies) != 0 {
		t.Fatalf("off %+v", got)
	}
}

// The route answers for the person the request is signed in as, and reads
// nobody from the address.
func TestMyWatchedRouteIsTheCallersOwn(t *testing.T) {
	w, movie, show := mineWorld(t)
	seen(t, w, "Plex", "22", "ana", movie, 0, 0)
	seen(t, w, "Plex", "33", "kid", show, 1, 1)
	h := &Handlers{Store: w.s}
	ask := func(user int64, path string) MyWatched {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if user != 0 {
			r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: user}))
		}
		rec := httptest.NewRecorder()
		h.myWatched(rec, r)
		var out MyWatched
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%d %s %v", rec.Code, rec.Body.String(), err)
		}
		return out
	}
	if got := ask(7, "/api/v1/my/watched"); !reflect.DeepEqual(got.Movies, []int64{movie}) || len(got.Shows) != 0 {
		t.Fatalf("ana %+v", got)
	}
	if got := ask(9, "/api/v1/my/watched?user=7&user_id=7&person=7"); got.Linked || len(got.Movies) != 0 {
		t.Fatalf("bo asked for ana's %+v", got)
	}
	if got := ask(0, "/api/v1/my/watched"); got.Linked || len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("no caller %+v", got)
	}
}

// "Watched by anyone" lists every title any account watched, linked to a
// person here or not, each once, and nothing while watch history is off.
func TestWatchedByAnyone(t *testing.T) {
	w, movie, show := mineWorld(t)
	seen(t, w, "Jelly", "stranger", "stranger", movie, 0, 0)
	seen(t, w, "Plex", "22", "ana", movie, 0, 0)
	seen(t, w, "Plex", "22", "ana", show, 1, 1)
	got, err := WatchedByAnyone(context.Background(), w.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || !reflect.DeepEqual(got.Titles, []int64{movie, show}) {
		t.Fatalf("got %+v", got)
	}
	watchOn(t, w, false)
	if got, _ := WatchedByAnyone(context.Background(), w.s.Read()); got.Enabled || len(got.Titles) != 0 {
		t.Fatalf("off %+v", got)
	}
}
