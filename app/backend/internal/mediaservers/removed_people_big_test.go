package mediaservers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// A big server: 3,000 users of about 1.5 KB each (past one answer's usual
// size cap), with one linked member missing, who is turned off.
func TestRemovedCheckReadsABigUserList(t *testing.T) {
	w := newWorld(t)
	peopleWorld(t, w, "jellyfin")
	pad := strings.Repeat("x", 1500)
	var b strings.Builder
	b.WriteString("[")
	ids := []string{"u9", "u1", "u2"}
	for i := 0; i < 3000; i++ {
		id := fmt.Sprintf("z%d", i)
		if i < len(ids) {
			id = ids[i]
		}
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"Id":%q,"Policy":{"Pad":%q}}`, id, pad)
	}
	b.WriteString("]")
	connected(t, w, "jellyfin", usersServer(t, b.String()).URL)
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.CheckRemovedPeople(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s, _ := stateOf(t, w, "gone"); s != "disabled" {
		t.Fatalf("gone: %s", s)
	}
}

// unread is the jellyfin people check's warning ("" while it reads).
func unread(t *testing.T, w *world) (string, bool) {
	t.Helper()
	msg, ok := accounts.UnreadUserLists(context.Background(), w.s.Read())["jellyfin"]
	return msg, ok
}

// A user list that cannot be read three runs in a row is said once, and
// reading it again is said once too.
func TestRemovedCheckSaysWhenTheListCannotBeRead(t *testing.T) {
	w := newWorld(t)
	var bad atomic.Bool
	bad.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if bad.Load() {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = rw.Write([]byte(`[{"Id":"u9"},{"Id":"u7"},{"Id":"u1"},{"Id":"u2"}]`))
	}))
	t.Cleanup(srv.Close)
	connected(t, w, "jellyfin", srv.URL)
	peopleWorld(t, w, "jellyfin")
	l := &Linker{Store: w.s, Vault: w.v}
	t.Cleanup(outbound.Reset)
	for i := 0; i < 2; i++ {
		outbound.Reset() // each run is a fresh try, not the retry wait
		_ = l.CheckRemovedPeople(context.Background())
	}
	if msg, _ := unread(t, w); msg != "" {
		t.Fatalf("warned after two runs: %q", msg)
	}
	for i := 0; i < 3; i++ {
		outbound.Reset() // each run is a fresh try, not the retry wait
		_ = l.CheckRemovedPeople(context.Background())
	}
	if msg, _ := unread(t, w); !strings.HasPrefix(msg, "Can't check for people removed on the Jellyfin server") {
		t.Fatalf("warning: %q", msg)
	}
	bad.Store(false)
	for i := 0; i < 2; i++ {
		outbound.Reset() // each run is a fresh try, not the retry wait
		_ = l.CheckRemovedPeople(context.Background())
	}
	if msg, ok := unread(t, w); msg != "" || !ok {
		t.Fatalf("still warning after reading again: %q %v", msg, ok)
	}
	allOn(t, w, "after reading again")
}

// A standing warning goes, with one "Working again", once there is nothing
// left to read: the sign-in way is off or its server is no longer connected.
func TestRemovedCheckWarningClearsWhenNothingToRead(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	connected(t, w, "jellyfin", srv.URL)
	peopleWorld(t, w, "jellyfin")
	l := &Linker{Store: w.s, Vault: w.v}
	t.Cleanup(outbound.Reset)
	for i := 0; i < 3; i++ {
		outbound.Reset()
		_ = l.CheckRemovedPeople(context.Background())
	}
	if msg, _ := unread(t, w); !strings.HasPrefix(msg, "Can't check for people removed on the Jellyfin server") {
		t.Fatalf("warning: %q", msg)
	}
	signInWay(t, w, "jellyfin", "http://127.0.0.1:1") // the way now points elsewhere
	for i := 0; i < 2; i++ {
		_ = l.CheckRemovedPeople(context.Background())
	}
	if msg, ok := unread(t, w); ok {
		t.Fatalf("the check stayed with nothing to read: %q %v", msg, ok)
	}
}

// With sign-in on but no connected server to read, the people check goes.
func TestRemovedCheckRowGoesWithoutAServer(t *testing.T) {
	w := newWorld(t)
	srv := jellyHistoryServer(t, "jellyfin", nil)
	connected(t, w, "jellyfin", srv.URL)
	peopleWorld(t, w, "jellyfin")
	l := &Linker{Store: w.s, Vault: w.v}
	t.Cleanup(outbound.Reset)
	if err := l.CheckRemovedPeople(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A failed read once made the row.
	if err := accounts.NoteUserListRead(context.Background(), w.s, "jellyfin", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := unread(t, w); !ok {
		t.Fatal("no row after a failed read")
	}
	signInWay(t, w, "jellyfin", "http://127.0.0.1:1") // no connected server there
	if err := l.CheckRemovedPeople(context.Background()); err != nil {
		t.Fatal(err)
	}
	if msg, ok := unread(t, w); ok {
		t.Fatalf("row stayed: %q", msg)
	}
}

// Removing the connected server, or changing its address, drops the
// failed-read count at once, so adding it back shows no old warning.
func TestServerChangeForgetsUnreadCount(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	srv := jellyHistoryServer(t, "jellyfin", nil)
	fail3 := func() {
		for i := 0; i < 3; i++ {
			if err := accounts.NoteUserListRead(ctx, w.s, "jellyfin", false); err != nil {
				t.Fatal(err)
			}
		}
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	signInWay(t, w, "jellyfin", srv.URL)
	fail3()
	if err := Remove(ctx, w.s, w.v, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := unread(t, w); ok {
		t.Fatal("removed: the count stayed")
	}
	if m, err = Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	fail3()
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL + "/jf", Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, ok := unread(t, w); ok {
		t.Fatal("new address: the count stayed")
	}
}

// A removal or change that went through is not reported as failed because
// the people check's count could not be cleared afterwards.
func TestServerChangeSucceedsWhenTheCountCannotClear(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	srv := jellyHistoryServer(t, "jellyfin", nil)
	if err := accounts.NoteUserListRead(ctx, w.s, "jellyfin", false); err != nil {
		t.Fatal(err)
	}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TRIGGER no_count_clear BEFORE UPDATE ON settings WHEN NEW.key LIKE 'removed_check_fails_%'
			BEGIN SELECT RAISE(ABORT, 'refused'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL + "/jf", Token: tok}, domain.System); err != nil {
		t.Fatalf("change: %v", err)
	}
	if err := Remove(ctx, w.s, w.v, m.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
}
