package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
)

// Each signed-in person is answered their own watched titles and nobody
// else's, whatever the address asks for.
// route: GET /api/v1/my/watched
func TestMyWatchedOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`, 200)
	var movie, show int64
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []struct {
			into *int64
			sql  string
		}{
			{&movie, `INSERT INTO titles(created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added) VALUES ('t', '{}', 't', '{}', 'movie', 'Seen Movie', 'seen movie', 'local', 'by_hand')`},
			{&show, `INSERT INTO titles(created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added) VALUES ('t', '{}', 't', '{}', 'show', 'Seen Show', 'seen show', 'local', 'by_hand')`},
		} {
			res, err := tx.Exec(q.sql)
			if err != nil {
				return err
			}
			*q.into, _ = res.LastInsertId()
		}
		for _, q := range []string{
			`INSERT INTO media_servers(id, created_at, created_by, updated_at, updated_by, name, kind, url) VALUES (1, '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'Jelly', 'jellyfin', 'http://jelly.test')`,
			`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?1, 'jellyfin', 'ua', 'ana', 't'), (?2, 'jellyfin', 'ub', 'bo', 't')`,
			`INSERT INTO watch_state(media_server_id, account_key, account_name, title_id, kind, season, episode, read_at) VALUES
				(1, 'ua', 'ana', ?3, 'movie', 0, 0, 't'), (1, 'ub', 'bo', ?4, 'episode', 1, 1, 't'), (1, 'ub', 'bo', ?4, 'episode', 1, 2, 't')`,
		} {
			if _, err := tx.Exec(q, r.ana.id, r.bo.id, movie, show); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ask := func(s *signedIn, path string) mediaservers.MyWatched {
		t.Helper()
		var out mediaservers.MyWatched
		if err := json.Unmarshal(s.call("GET", path, "", 200), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := ask(r.ana, "/api/v1/my/watched"); !got.Enabled || !got.Linked || !reflect.DeepEqual(got.Movies, []int64{movie}) || len(got.Shows) != 0 {
		t.Fatalf("ana %+v", got)
	}
	if got := ask(r.bo, "/api/v1/my/watched?user=2&user_id=2"); !got.Linked || len(got.Movies) != 0 || !reflect.DeepEqual(got.Shows, map[int64]map[int]int{show: {1: 2}}) {
		t.Fatalf("bo %+v", got)
	}
	// The owner signs in with a password only.
	if got := ask(r.owner, "/api/v1/my/watched"); !got.Enabled || got.Linked || len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("owner %+v", got)
	}
	// Signed out, nothing is answered.
	(&signedIn{t: t, base: r.owner.base, c: newClient()}).call("GET", "/api/v1/my/watched", "", 401)

	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":false}`, 200)
	if got := ask(r.ana, "/api/v1/my/watched"); got.Enabled || got.Linked || len(got.Movies) != 0 {
		t.Fatalf("off %+v", got)
	}
}

// Only the owner and admins may ask what anybody watched.
// route: GET /api/v1/library/watched-by-anyone
func TestWatchedByAnyoneOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`, 200)
	var out mediaservers.AnyoneWatched
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/library/watched-by-anyone", "", 200), &out); err != nil || !out.Enabled || out.Titles == nil {
		t.Fatalf("owner %+v %v", out, err)
	}
	r.ana.call("GET", "/api/v1/library/watched-by-anyone", "", 403)
}

// The clean-up list names who watched what and when, so it is gated like
// watched-by-anyone: a person who only manages the Library is refused.
// route: GET /api/v1/library/watched-cleanup
func TestWatchedCleanupOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`, 200)
	var out mediaservers.WatchedCleanup
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/library/watched-cleanup?days=7", "", 200), &out); err != nil || !out.Enabled || out.Days != 7 || out.Found == nil {
		t.Fatalf("owner %+v %v", out, err)
	}
	r.ana.call("GET", "/api/v1/library/watched-cleanup", "", 403)
	r.owner.call("PUT", "/api/v1/users/"+itoa(r.ana.id), `{"perms":["ask","manage_library"]}`, 200)
	r.ana.call("GET", "/api/v1/library/watched-cleanup", "", 403)
}
