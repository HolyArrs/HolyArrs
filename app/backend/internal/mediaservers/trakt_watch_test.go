package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeTraktHistory plays Trakt: the device sign-in, and the signed-in
// person's watched movies (The Matrix twice, one movie not in the Library)
// and shows (Severance 1x1 and 1x2, read from its progress as Trakt
// answers now; a show not in the Library is never asked about). good says whether Trakt still takes
// the token.
func fakeTraktHistory(t *testing.T, good *atomic.Bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "dev", "user_code": "CODE", "verification_url": "https://trakt.tv/activate", "expires_in": 600, "interval": 1})
			return
		case "/oauth/device/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "refresh_token": "ref", "expires_in": 86400})
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("trakt-api-key") != "cid" || !good.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/users/settings":
			_, _ = w.Write([]byte(`{"user":{"username":"ana_t"}}`))
		case "/sync/watched/movies":
			_, _ = w.Write([]byte(`[{"plays":2,"last_watched_at":"2026-11-01T20:00:00.000Z","movie":{"title":"The Matrix","ids":{"trakt":481,"imdb":"tt0133093","tmdb":603}}},
				{"plays":1,"movie":{"title":"Elsewhere","ids":{"tmdb":999999}}}]`))
		case "/sync/watched/shows":
			// As Trakt answers now: no seasons, so the episodes come from
			// the show's progress.
			_, _ = w.Write([]byte(`[{"plays":2,"show":{"title":"Severance","ids":{"trakt":143,"tvdb":100,"tmdb":95396}}},
				{"plays":5,"show":{"title":"Elsewhere","ids":{"trakt":555,"tvdb":999999}}}]`))
		case "/shows/143/progress/watched":
			_, _ = w.Write([]byte(`{"seasons":[{"number":0,"episodes":[{"number":1,"completed":false,"stats":{"play_count":0}}]},
				{"number":1,"episodes":[{"number":1,"completed":true,"last_watched_at":"2026-11-02T20:00:00.000Z","stats":{"play_count":1}},
				{"number":2,"completed":true,"stats":{"play_count":1}},{"number":3,"completed":false,"stats":{"play_count":0}}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestWatchHistoryFromTrakt: with no media server at all, the history of
// the person who signed in with Trakt is read for the Library titles and
// shows in their Watched marks, in Watched by anyone and in the summary.
// A token Trakt refuses ends the sign-in and drops the Trakt rows.
func TestWatchHistoryFromTrakt(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	movie, show := watchTitles(t, w)
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
			(7, 't', '{}', 't', '{}', 'Ana B', 'member', ''), (9, 't', '{}', 't', '{}', 'Bo', 'member', 'x')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	good := &atomic.Bool{}
	good.Store(true)
	srv := fakeTraktHistory(t, good)
	auth := &metadata.TraktAuth{Secrets: w.v, Base: srv.URL}
	by := database.Actor{Kind: "user", ID: 7}
	if err := w.v.SetSetting(ctx, metadata.TraktKeyGroup, metadata.TraktKeyName, "cid", by); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetSecret(ctx, "secret", by); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Trakt: auth}
	// Watch history off: Trakt is not read.
	if _, err := auth.StartSignIn(ctx, 7, by); err != nil {
		t.Fatal(err)
	}
	if s, err := auth.CheckSignIn(ctx); s != metadata.TraktSignedIn || err != nil {
		t.Fatalf("sign-in %q %v", s, err)
	}
	if err := l.ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mine(t, w, 7); got.Enabled || len(got.Movies) != 0 {
		t.Fatalf("read while off: %+v", got)
	}
	watchOn(t, w, true)
	if err := l.ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	got := mine(t, w, 7)
	if !got.Linked || !slices.Equal(got.Movies, []int64{movie}) || got.Shows[show][1] != 2 {
		t.Fatalf("Ana's watched %+v", got)
	}
	if bo := mine(t, w, 9); bo.Linked || len(bo.Movies) != 0 || len(bo.Shows) != 0 {
		t.Fatalf("Bo got Ana's Trakt history: %+v", bo)
	}
	anyone, err := WatchedByAnyone(ctx, w.s.Read())
	if err != nil || !slices.Equal(anyone.Titles, []int64{movie, show}) {
		t.Fatalf("watched by anyone %+v %v", anyone, err)
	}
	sum, err := Watched(ctx, w.s.Read())
	if err != nil || len(sum.People) != 1 || sum.ReadAt == nil {
		t.Fatalf("summary %+v %v", sum, err)
	}
	if p := sum.People[0]; p.Server != "Trakt" || p.ServerKind != "trakt" || p.Name != "ana_t" || !p.Linked || p.UserName != "Ana B" ||
		p.Movies != 1 || p.Shows != 1 || p.Episodes != 2 || p.Plays != 4 || p.LastWatchedAt == nil {
		t.Fatalf("Trakt line %+v", p)
	}
	// Revoked at Trakt: the read fails, the sign-in ends, the rows go.
	good.Store(false)
	if err := l.ReadWatchHistory(ctx); err == nil {
		t.Fatal("a refused token was not an error")
	}
	if auth.Status(ctx).SignedIn {
		t.Fatal("still signed in after Trakt refused the token")
	}
	if got := mine(t, w, 7); len(got.Movies) != 0 || len(got.Shows) != 0 {
		t.Fatalf("Trakt rows kept after the sign-in ended: %+v", got)
	}
	// Signed out, the next read asks Trakt nothing and is no error.
	if err := l.ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
}

// Turning watch history off forgets the Trakt rows too.
func TestWatchHistoryOffForgetsTrakt(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	movie, _ := watchTitles(t, w)
	watchOn(t, w, true)
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'Ana', 'member', '')`); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceTraktWatchState(ctx, w.s, 7, "ana_t", []watchRow{{title: movie, kind: "movie", plays: 1}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := mine(t, w, 7); len(got.Movies) != 1 {
		t.Fatalf("before %+v", got)
	}
	watchOn(t, w, false)
	var n int
	if err := w.s.Read().QueryRow("SELECT count(*) FROM trakt_watch_state").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after off: %d %v", n, err)
	}
}
