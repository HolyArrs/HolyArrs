package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Test-only Plex sign-ins: each fake PIN hands out the one the test chose.
const (
	anaPlex    = "ana-plex-cred-1"
	anaPlexNew = "ana-plex-cred-2"
	anaPlexTwo = "ana-plex-cred-other" // a second Plex account of Ana's
)

// plexPeople answers like plex.tv for several people linking their accounts.
type plexPeople struct {
	srv  *httptest.Server
	mu   sync.Mutex
	next string           // the sign-in the next PIN confirms with
	pins map[int64]string // PIN id -> its sign-in
	n    atomic.Int64
	// onUser, when set, runs while Plex is asked who the person is.
	onUser func()
}

func newPlexPeople(t *testing.T) *plexPeople {
	p := &plexPeople{pins: map[int64]string{}}
	who := map[string]string{
		anaPlex:    `{"id":21,"username":"ana-on-plex"}`,
		anaPlexNew: `{"id":21,"username":"ana-on-plex"}`,
		anaPlexTwo: `{"id":22,"username":"ana-second"}`,
	}
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) {
		id := 500 + p.n.Add(1)
		p.mu.Lock()
		p.pins[id] = p.next
		p.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"id":%d,"code":"MINE"}`, id)
	})
	m.HandleFunc("GET /api/v2/pins/{id}", func(w http.ResponseWriter, r *http.Request) {
		var id int64
		_, _ = fmt.Sscan(r.PathValue("id"), &id)
		p.mu.Lock()
		got := p.pins[id]
		p.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"authToken": got})
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		hook := p.onUser
		p.mu.Unlock()
		if hook != nil {
			hook()
		}
		u, ok := who[r.Header.Get("X-Plex-Token")]
		if !ok {
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte(u))
	})
	p.srv = httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(p.srv.Close)
	return p
}

// start begins a My account link that Plex confirms with cred; it returns the PIN id.
func (p *plexPeople) start(t *testing.T, f *fixture, by person, cred string) string {
	t.Helper()
	p.mu.Lock()
	p.next = cred
	p.mu.Unlock()
	resp, out := by.do(t, f, "POST", "/api/v1/auth/plex/my-link/start", "")
	if resp.StatusCode != 200 || !strings.Contains(out, "MINE") {
		t.Fatalf("my-link start: %d %s", resp.StatusCode, out)
	}
	var v struct {
		ID int64 `json:"id"`
	}
	jsonOf(t, out, &v)
	return fmt.Sprintf(`{"id":%d}`, v.ID)
}

func keptPlex(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = ?", plexTokenKind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func plexTokenOf(t *testing.T, f *fixture, userID int64) string {
	t.Helper()
	got, err := f.a.PlexToken(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// route: GET /api/v1/auth/plex/my-link
// route: POST /api/v1/auth/plex/my-link/start
// route: POST /api/v1/auth/plex/my-link/finish
// route: DELETE /api/v1/auth/plex/my-link
func TestMemberLinksPlexFromMyAccount(t *testing.T) {
	p := newPlexPeople(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	bob := join(t, f, invite(t, f, own, `{"role":"member"}`), "bob")
	anaID := userID(t, f, "ana")
	if _, out := ana.do(t, f, "GET", "/api/v1/auth/plex/my-link", ""); !strings.Contains(out, `"linked":false`) {
		t.Fatalf("before: %s", out)
	}
	// Plex sign-in is off: linking works anyway (for the watchlist).
	resp, out := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, ana, anaPlex))
	if resp.StatusCode != 200 || !strings.Contains(out, `"name":"ana-on-plex"`) || strings.Contains(out, anaPlex) {
		t.Fatalf("finish: %d %s", resp.StatusCode, out)
	}
	if got := plexTokenOf(t, f, anaID); got != anaPlex {
		t.Fatalf("kept %q", got)
	}
	for _, q := range []string{"SELECT count(*) FROM secrets WHERE value LIKE '%'||?||'%'", "SELECT count(*) FROM settings WHERE value LIKE '%'||?||'%'",
		"SELECT count(*) FROM user_links WHERE external_name LIKE '%'||?||'%' OR external_id LIKE '%'||?||'%'"} {
		var n int
		args := []any{anaPlex}
		if strings.Count(q, "?") == 2 {
			args = append(args, anaPlex)
		}
		if err := f.s.Read().QueryRow(q, args...).Scan(&n); err != nil || n != 0 {
			t.Fatalf("Plex sign-in in the clear (%s): %d %v", q, n, err)
		}
	}
	if _, out := ana.do(t, f, "GET", "/api/v1/auth/plex/my-link", ""); !strings.Contains(out, `"kept":true`) || !strings.Contains(out, `"linked":true`) {
		t.Fatalf("after: %s", out)
	}
	// Linking again with a newer sign-in replaces the kept one.
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, ana, anaPlexNew)); resp.StatusCode != 200 {
		t.Fatalf("again: %d %s", resp.StatusCode, out)
	}
	if got, n := plexTokenOf(t, f, anaID), keptPlex(t, f); got != anaPlexNew || n != 1 {
		t.Fatalf("replaced: %q, %d kept", got, n)
	}
	// Bob cannot take Ana's Plex account.
	if resp, _ := bob.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, bob, anaPlex)); resp.StatusCode != 409 {
		t.Fatalf("taken: %d", resp.StatusCode)
	}
	// A My account PIN never signs anyone in.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"join":"everyone"},"confirm_everyone":true}`)
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/plex/finish", p.start(t, f, ana, anaPlex)); resp.StatusCode != 404 {
		t.Fatalf("a link PIN signed in: %d", resp.StatusCode)
	}
	// A turned-off person's watchlist is not read.
	own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", anaID), `{"state":"disabled"}`)
	if got := plexTokenOf(t, f, anaID); got != "" {
		t.Fatalf("turned off: %q", got)
	}
	own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", anaID), `{"state":"active"}`)
	ana = signIn(t, f, "ana")
	if resp, _ := ana.do(t, f, "DELETE", "/api/v1/auth/plex/my-link", ""); resp.StatusCode != 204 {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	if got, n := plexTokenOf(t, f, anaID), keptPlex(t, f); got != "" || n != 0 {
		t.Fatalf("after unlink: %q, %d kept", got, n)
	}
	if resp, _ := ana.do(t, f, "DELETE", "/api/v1/auth/plex/my-link", ""); resp.StatusCode != 404 {
		t.Fatalf("unlink twice: %d", resp.StatusCode)
	}
}

func TestPlexSignInKeepsItsToken(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("sign-in: %d %s", code, out)
	}
	id := userID(t, f, "kidplex")
	if got := plexTokenOf(t, f, id); got != plexTok {
		t.Fatalf("kept %q", got)
	}
	// The same sign-in again keeps one copy, not two.
	if _, code, _ := plexSignIn(t, f, p); code != 200 || keptPlex(t, f) != 1 {
		t.Fatalf("again: %d, %d kept", code, keptPlex(t, f))
	}
	// Without a password, unlinking would leave no way in.
	_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, me, &v)
	if resp, _ := call(t, c, "DELETE", f.srv.URL+"/api/v1/auth/plex/my-link", "", map[string]string{CSRFHeader: v.CSRF}); resp.StatusCode != 403 && resp.StatusCode != 409 {
		t.Fatalf("only way in unlinked: %d", resp.StatusCode)
	}
	if got := plexTokenOf(t, f, id); got != plexTok {
		t.Fatalf("still kept: %q", got)
	}
	// The owner setting a new password ends the outside links and their kept sign-ins.
	if resp, out := own.do(t, f, "POST", "/api/v1/users/"+itoa(id)+"/password", `{"password":"another long test passphrase"}`); resp.StatusCode != 204 {
		t.Fatalf("set password: %d %s", resp.StatusCode, out)
	}
	if got, n := plexTokenOf(t, f, id), keptPlex(t, f); got != "" || n != 0 {
		t.Fatalf("after a new password: %q, %d kept", got, n)
	}
}

func TestOwnerPlexLinkCountsForTheOwner(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	ownerID := userID(t, f, "luis")
	if got := plexTokenOf(t, f, ownerID); got != ownerPlexTok {
		t.Fatalf("owner kept %q", got)
	}
	if f.a.PlexClientID(context.Background()) == "" {
		t.Fatal("no client id")
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/plex/link", ""); resp.StatusCode != 200 {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	if got, n := plexTokenOf(t, f, ownerID), keptPlex(t, f); got != "" || n != 0 {
		t.Fatalf("after unlink: %q, %d kept", got, n)
	}
}

// signInWith signs in with Plex in a fresh browser, Plex confirming with cred.
func (p *plexPeople) signInWith(t *testing.T, f *fixture, cred string) (*http.Client, int) {
	t.Helper()
	p.mu.Lock()
	p.next = cred
	p.mu.Unlock()
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("plex start: %d %s", resp.StatusCode, out)
	}
	var v struct {
		ID int64 `json:"id"`
	}
	jsonOf(t, out, &v)
	resp, _ = call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", fmt.Sprintf(`{"id":%d}`, v.ID), nil)
	return c, resp.StatusCode
}

// A password change that lands while Plex is being asked wins: the link is not made.
func TestPlexLinkLosesToAPasswordChangeMeanwhile(t *testing.T) {
	p := newPlexPeople(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	anaID := userID(t, f, "ana")
	pin := p.start(t, f, ana, anaPlex)
	p.mu.Lock()
	p.onUser = func() {
		if resp, out := own.do(t, f, "POST", "/api/v1/users/"+itoa(anaID)+"/password", `{"password":"another long test passphrase"}`); resp.StatusCode != 204 {
			t.Errorf("set password: %d %s", resp.StatusCode, out)
		}
	}
	p.mu.Unlock()
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", pin); resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Fatalf("linked after the password changed: %d %s", resp.StatusCode, out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM user_links WHERE user_id = ?", anaID).Scan(&n)
	if n != 0 || keptPlex(t, f) != 0 {
		t.Fatalf("%d links, %d kept", n, keptPlex(t, f))
	}
}

// Linking another Plex account signs out the devices the old one signed in.
func TestPlexLinkToAnotherAccountSignsOutTheOld(t *testing.T) {
	p := newPlexPeople(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, ana, anaPlex)); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"join":"nobody"}}`)
	tv, code := p.signInWith(t, f, anaPlex)
	if code != 200 {
		t.Fatalf("plex sign-in: %d", code)
	}
	if resp, me := call(t, tv, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != 200 || !strings.Contains(me, `"name":"ana"`) {
		t.Fatalf("me: %d %s", resp.StatusCode, me)
	}
	// The same account again keeps that device signed in.
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, ana, anaPlexNew)); resp.StatusCode != 200 {
		t.Fatalf("relink: %d", resp.StatusCode)
	}
	if resp, _ := call(t, tv, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != 200 {
		t.Fatalf("same account signed the device out: %d", resp.StatusCode)
	}
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/plex/my-link/finish", p.start(t, f, ana, anaPlexTwo)); resp.StatusCode != 200 || !strings.Contains(out, "ana-second") {
		t.Fatalf("other account: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, tv, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != 401 {
		t.Fatalf("old Plex device still signed in: %d", resp.StatusCode)
	}
	if resp, _ := ana.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 200 {
		t.Fatalf("password device signed out: %d", resp.StatusCode)
	}
	if got := plexTokenOf(t, f, userID(t, f, "ana")); got != anaPlexTwo || keptPlex(t, f) != 1 {
		t.Fatalf("kept %q (%d)", got, keptPlex(t, f))
	}
}

// The owner's Settings link is what Plex friends are read with: its token
// and chosen server, and nothing once it is unlinked.
func TestPlexOwnerLinkForFriends(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	ctx := context.Background()
	if tok, server := f.a.PlexOwnerLink(ctx); tok != "" || server != "" {
		t.Fatalf("before linking: %q %q", tok, server)
	}
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":false,"server_id":"mine-1","join":"nobody"}}`)
	if tok, server := f.a.PlexOwnerLink(ctx); tok != ownerPlexTok || server != "mine-1" {
		t.Fatalf("linked: %q %q", tok, server)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/auth/plex/link", ""); resp.StatusCode != 200 {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	if tok, _ := f.a.PlexOwnerLink(ctx); tok != "" {
		t.Fatalf("after unlink: %q", tok)
	}
}
