package accounts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Backup files hold the whole database, password hashes included: only the
// owner may download or delete them.
func TestBackupFilesOwnerOnly(t *testing.T) {
	for _, p := range []string{"GET /api/v1/backups/{name}", "DELETE /api/v1/backups/{name}"} {
		if RoutePerms[p] != webserver.NeedOwner {
			t.Errorf("%s needs %q, want owner", p, RoutePerms[p])
		}
	}
}

// Behind a trusted proxy every visitor gets their own sign-in budget.
func TestSignInLimitsUseForwardedAddress(t *testing.T) {
	_, n, _ := net.ParseCIDR("10.0.0.1/32")
	a := &Auth{proxies: []*net.IPNet{n}}
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := a.clientKey(r); got != "ip:203.0.113.7" {
		t.Fatalf("key %q, want the forwarded address", got)
	}
}

// A correct password must not hand back free two-factor guesses.
func TestPasswordDoesNotResetCodeBackOff(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	secret, _ := turnOnTwoFactor(t, f, own)
	c := client(t)
	tk := ticketFor(t, f, c, "luis")
	for range ticketTries {
		call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"000000"}`, nil)
	}
	f.clock.add(2 * time.Second)
	tk = ticketFor(t, f, c, "luis")
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"000000"}`, nil)
	resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"`+codeNow(t, f, secret, 0)+`"}`, nil)
	if resp.StatusCode != 429 {
		t.Fatalf("code right after a fresh ticket: %d, want 429", resp.StatusCode)
	}
}

// Very long names are refused before they reach the back-off map.
func TestLongSignInNameNotRemembered(t *testing.T) {
	f := setup(t, Options{})
	name := strings.Repeat("x", 5000)
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	f.a.mu.Lock()
	defer f.a.mu.Unlock()
	for k := range f.a.fails {
		if len(k) > 200 {
			t.Fatalf("long key kept: %d bytes", len(k))
		}
	}
}

// Changing your own password ends your API keys too.
func TestPasswordChangeEndsKeys(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"script"}`); resp.StatusCode/100 != 2 {
		t.Fatalf("key: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"another long passphrase"}`); resp.StatusCode != 204 {
		t.Fatalf("change: %d %s", resp.StatusCode, out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM api_keys").Scan(&n)
	if n != 0 {
		t.Fatalf("%d keys still work after the password change", n)
	}
}

// An invite gives only the permissions its maker still holds.
func TestInviteTrimsPermsMakerLost(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	adm := join(t, f, invite(t, f, own, `{"role":"admin","perms":["ask","settings","approve"]}`), "adm")
	tok := invite(t, f, adm, `{"role":"member","perms":["ask","settings"]}`)
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","approve"]' WHERE name = 'adm'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ana := join(t, f, tok, "ana")
	_, me := ana.do(t, f, "GET", "/api/v1/auth/me", "")
	if strings.Contains(me, `"settings"`) {
		t.Fatalf("invite kept a permission its maker lost: %s", me)
	}
}

// pngOfSize is a tiny PNG whose header claims w by h.
func pngOfSize(t *testing.T, w, h int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	binary.BigEndian.PutUint32(b[16:], uint32(w))
	binary.BigEndian.PutUint32(b[20:], uint32(h))
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// A picture big enough to need hundreds of MB to decode is refused up front.
func TestAvatarRefusesHugePicture(t *testing.T) {
	_, err := makeAvatar(pngOfSize(t, 6000, 5000))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err %v, want too large", err)
	}
}

// A server the joiner owns is not a share of the owner's server.
func TestPlexJoinIgnoresOwnServer(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":77,"code":"ABCD"}`))
	})
	m.HandleFunc("GET /api/v2/pins/77", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id":77,"authToken":%q}`, plexTok)
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":4242,"uuid":"u","username":"faker","email":"f@example.test"}`))
	})
	m.HandleFunc("GET /api/v2/resources", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"clientIdentifier":"machine-1","provides":"server","owned":true}]`))
	})
	p := httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(p.Close)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	c := client(t)
	call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("joined through a copied server id: %d %s", resp.StatusCode, out)
	}
}

// With sign-in skipped at home, importing someone as a Kid is refused.
func TestImportKidRefusedWhileSkipping(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":false,"server_id":"mine-1","join":"nobody"}}`)
	own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
	f.a.skipLocal.Store(true)
	resp, out := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"22"}]}`)
	if resp.StatusCode != 409 {
		t.Fatalf("kid import while skipping: %d %s", resp.StatusCode, out)
	}
	if !strings.Contains(out, "skipping sign-in") {
		t.Fatalf("refused for another reason: %s", out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users WHERE name = 'Little'").Scan(&n)
	if n != 0 {
		t.Fatal("kid account made")
	}
}

// A Plex pin can only be finished by the browser that started it, even when
// another visitor shares its address (both come from 127.0.0.1 here, as
// everyone does behind a proxy HolyArrs was not told about).
func TestPlexPinNeedsItsStartersCookie(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	victim := client(t)
	if resp, out := call(t, victim, "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil); resp.StatusCode != 200 {
		t.Fatalf("start: %d %s", resp.StatusCode, out)
	}
	p.confirmed.Store(true)
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("another browser finished the pin: %d %s", resp.StatusCode, out)
	}
	if resp, out := call(t, victim, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 200 {
		t.Fatalf("starter could not finish: %d %s", resp.StatusCode, out)
	}
}
