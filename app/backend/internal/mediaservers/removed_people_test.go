package mediaservers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// peopleWorld adds the owner and three members, each linked to an account
// on a kind server: the owner and "gone" to accounts the server no longer
// has, "ana" (u1) and "kid" (u2) to accounts it still has.
func peopleWorld(t *testing.T, w *world, kind string) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, p := range [][3]string{{"boss", "owner", "u9"}, {"gone", "member", "u7"}, {"ana", "member", "u1"}, {"kid", "member", "u2"}} {
			id := int64(i + 1)
			if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, state)
				VALUES (?, '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', ?, ?, '', 'active')`, id, p[0], p[1]); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at)
				VALUES (?, ?, ?, ?, '2026-01-01T00:00:00Z')`, id, kind, p[2], p[0]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// signInWay turns on the kind sign-in way with the server at url.
func signInWay(t *testing.T, w *world, kind, url string) {
	t.Helper()
	if kind == "emby" {
		t.Setenv("HOLYARRS_FLAGS", "emby_signin") // Emby sign-in is held back by default
	}
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		for k, v := range map[string]string{"sign_in_" + kind + "_on": "true", "sign_in_" + kind + "_url": url} {
			if err := database.PutSetting(tx, "this_app", k, v, domain.System, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// connected adds srv as a library server and the kind sign-in way on it.
func connected(t *testing.T, w *world, kind, url string) {
	t.Helper()
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "Home", Kind: kind, URL: url, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	signInWay(t, w, kind, url)
}

// usersServer answers /Users with body, with the token check of a real one.
func usersServer(t *testing.T, body string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users" {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = rw.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func stateOf(t *testing.T, w *world, name string) (state, reason string) {
	t.Helper()
	if err := w.s.Read().QueryRow("SELECT state, off_reason FROM users WHERE name = ?", name).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	return state, reason
}

func removedEvents(t *testing.T, w *world) int {
	t.Helper()
	var n int
	if err := w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'account_changed' AND summary LIKE '%removed on the%'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func allOn(t *testing.T, w *world, why string) {
	t.Helper()
	for _, n := range []string{"boss", "gone", "ana", "kid"} {
		if s, _ := stateOf(t, w, n); s != "active" {
			t.Fatalf("%s: %s turned off", why, n)
		}
	}
}

// People removed on the sign-in server are turned off here (never the
// owner), with the reason kept and a History event; people the server still
// has are left alone.
func TestRemovedOnTheServerTurnsOff(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			w := newWorld(t)
			ctx := context.Background()
			connected(t, w, kind, jellyHistoryServer(t, kind, nil).URL)
			peopleWorld(t, w, kind)
			l := &Linker{Store: w.s, Vault: w.v}
			if err := l.CheckRemovedPeople(ctx); err != nil {
				t.Fatal(err)
			}
			if s, why := stateOf(t, w, "gone"); s != "disabled" || why != kind {
				t.Fatalf("gone: %s %q", s, why)
			}
			for _, n := range []string{"boss", "ana", "kid"} {
				if s, _ := stateOf(t, w, n); s != "active" {
					t.Fatalf("%s: %s", n, s)
				}
			}
			if n := removedEvents(t, w); n != 1 {
				t.Fatalf("history events: %d", n)
			}
			if err := l.CheckRemovedPeople(ctx); err != nil {
				t.Fatal(err)
			}
			if n := removedEvents(t, w); n != 1 {
				t.Fatalf("history events after a second check: %d", n)
			}
		})
	}
}

// An answer that lists nobody, or would turn off more than half the people
// linked to the server, changes nothing.
func TestRemovedCheckDoubtfulAnswersChangeNothing(t *testing.T) {
	for name, body := range map[string]string{
		"null":        `null`,
		"empty":       `[]`,
		"all missing": `[{"Id":"x1","Name":"stranger"}]`,
		"most":        `[{"Id":"u1","Name":"ana"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			connected(t, w, "jellyfin", usersServer(t, body).URL)
			peopleWorld(t, w, "jellyfin")
			l := &Linker{Store: w.s, Vault: w.v}
			_ = l.CheckRemovedPeople(context.Background())
			allOn(t, w, name)
			if n := removedEvents(t, w); n != 0 {
				t.Fatalf("history events: %d", n)
			}
		})
	}
}

// A server that cannot be reached changes nothing.
func TestRemovedCheckUnreachableChangesNothing(t *testing.T) {
	w := newWorld(t)
	srv := jellyHistoryServer(t, "jellyfin", nil)
	connected(t, w, "jellyfin", srv.URL)
	peopleWorld(t, w, "jellyfin")
	srv.Close()
	l := &Linker{Store: w.s, Vault: w.v}
	_ = l.CheckRemovedPeople(context.Background())
	allOn(t, w, "server down")
}

// With no library server connected, or the sign-in way on another server
// than the connected one, there is no check: people signing in on A are
// never turned off because they are missing on B.
func TestRemovedCheckOnlyOnTheSignInServer(t *testing.T) {
	w := newWorld(t)
	peopleWorld(t, w, "emby")
	l := &Linker{Store: w.s, Vault: w.v}
	signInWay(t, w, "emby", "http://127.0.0.1:1")
	if err := l.CheckRemovedPeople(context.Background()); err != nil {
		t.Fatal(err)
	}
	allOn(t, w, "no library server")
	b := jellyHistoryServer(t, "emby", nil)
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "B", Kind: "emby", URL: b.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	_ = l.CheckRemovedPeople(context.Background())
	allOn(t, w, "sign-in on another server")
}

// Someone the owner turns back on stays on while their account is still
// missing on the server, with no new History event.
func TestRemovedTurnedBackOnStaysOn(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	connected(t, w, "emby", jellyHistoryServer(t, "emby", nil).URL)
	peopleWorld(t, w, "emby")
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.CheckRemovedPeople(ctx); err != nil {
		t.Fatal(err)
	}
	// What the owner's "turn back on" stores (see accounts' updateUserRow).
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET state = 'active', off_reason = 'kept:' || off_reason WHERE name = 'gone'")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := l.CheckRemovedPeople(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if s, _ := stateOf(t, w, "gone"); s != "active" {
		t.Fatalf("turned off again: %s", s)
	}
	if n := removedEvents(t, w); n != 1 {
		t.Fatalf("history events: %d", n)
	}
}
