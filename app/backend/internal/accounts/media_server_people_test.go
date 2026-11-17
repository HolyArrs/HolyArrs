package accounts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const ownerPlexTok = "owner-plex-token-SECRET"

// fakePlexOwner answers like plex.tv for the owner linking their account.
func fakePlexOwner(t *testing.T) *httptest.Server {
	m := http.NewServeMux()
	var n atomic.Int64
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id":%d,"code":"LINK"}`, 900+n.Add(1))
	})
	m.HandleFunc("GET /api/v2/pins/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authToken": ownerPlexTok})
	})
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Plex-Token") != ownerPlexTok {
				http.Error(w, "no", 401)
				return
			}
			h(w, r)
		}
	}
	m.HandleFunc("GET /api/v2/user", authed(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"username":"boss"}`))
	}))
	m.HandleFunc("GET /api/v2/resources", authed(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"Home","clientIdentifier":"mine-1","provides":"server","owned":true},
			{"name":"Friend","clientIdentifier":"theirs","provides":"server","owned":false},
			{"name":"Phone","clientIdentifier":"p","provides":"player","owned":true}]`))
	}))
	// Plex answers 410 here for everyone now, token or not.
	m.HandleFunc("GET /api/v2/friends", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) })
	// The list of people the owner shares with, in XML as plex.tv sends it.
	m.HandleFunc("GET /api/users", authed(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(plexUsersXML))
	}))
	s := httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(s.Close)
	return s
}

const plexUsersXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer friendlyName="myPlex" identifier="com.plexapp.plugins.myplex" totalSize="4" size="4">
  <User id="21" title="ana" username="ana" email="ana@example.test" restricted="0" home="0" thumb="https://plex.tv/users/a1/avatar">
    <Server id="1" serverId="9" machineIdentifier="mine-1" name="Home" owned="1"/>
  </User>
  <User id="22" title="Little" username="" email="" restricted="1" home="1" thumb="https://plex.tv/users/b2/avatar"/>
  <User id="23" title="elsewhere" username="elsewhere" restricted="0" home="0">
    <Server id="2" serverId="8" machineIdentifier="other" name="Theirs" owned="0"/>
  </User>
  <User id="0" title="no id"/>
</MediaContainer>`

// fakePlexPeople answers the people list with code and body; the rest of
// the owner's link works as in fakePlexOwner.
func fakePlexPeople(t *testing.T, code int, body string) *httptest.Server {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":901,"code":"LINK"}`)) })
	m.HandleFunc("GET /api/v2/pins/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authToken": ownerPlexTok})
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":1,"username":"boss"}`)) })
	m.HandleFunc("GET /api/v2/resources", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	m.HandleFunc("GET /api/v2/friends", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) })
	m.HandleFunc("GET /api/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
	s := httptest.NewServer(m)
	t.Cleanup(s.Close)
	return s
}

// The owner's Plex link reads the people list; each way it can fail says
// what happened instead of always asking to try again.
func TestPlexImportSaysWhatPlexDid(t *testing.T) {
	cases := []struct {
		name, body, want string
		code, status     int
	}{
		{"empty", `<MediaContainer size="0"></MediaContainer>`, `"people":[]`, 200, 200},
		{"gone", "", "Plex no longer offers the list of people your server is shared with", 410, 502},
		{"missing", "", "Plex no longer offers the list of people your server is shared with", 404, 502},
		{"refused", "", "Plex refused your linked Plex account; link it again", 401, 502},
		{"forbidden", "", "Plex refused your linked Plex account; link it again", 403, 502},
		{"busy", "", "Plex did not list the people your server is shared with; try again", 503, 502},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fakePlexPeople(t, c.code, c.body)
			f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
			own := ownerOf(t, f)
			own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
			if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
				t.Fatalf("link: %d %s", resp.StatusCode, out)
			}
			resp, out := own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
			if resp.StatusCode != c.status || !strings.Contains(out, c.want) {
				t.Fatalf("preview: %d %s", resp.StatusCode, out)
			}
			if c.code != 503 && strings.Contains(out, "try again") {
				t.Fatalf("asks to try again when that cannot help: %s", out)
			}
		})
	}
}

// route: POST /api/v1/auth/plex/link/start
// route: POST /api/v1/auth/plex/link/finish
// route: DELETE /api/v1/auth/plex/link
// route: POST /api/v1/auth/import/{kind}/preview
// route: POST /api/v1/auth/import/{kind}
func TestOwnerLinksPlexAndImports(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	// Plex sign-in is still off: linking works anyway, and its PIN cannot sign anyone in.
	resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp.StatusCode != 200 || !strings.Contains(out, "LINK") {
		t.Fatalf("link start: %d %s", resp.StatusCode, out)
	}
	resp, out = own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"id":"mine-1"`) || strings.Contains(out, "theirs") || strings.Contains(out, `"id":"p"`) {
		t.Fatalf("link finish: %d %s", resp.StatusCode, out)
	}
	if strings.Contains(out, ownerPlexTok) {
		t.Fatal("token in the answer")
	}
	_, st := own.do(t, f, "GET", "/api/v1/auth/settings", "")
	if !strings.Contains(st, `"plex_owner":{"can_import":true,"linked":true,"name":"boss"}`) || strings.Contains(st, ownerPlexTok) {
		t.Fatalf("settings: %s", st)
	}
	var plain int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE value LIKE '%'||?||'%'", ownerPlexTok).Scan(&plain)
	if plain != 0 {
		t.Fatal("Plex token stored outside the vault")
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":false,"server_id":"mine-1","join":"nobody"}}`)

	resp, out = own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
	if resp.StatusCode != 200 || !strings.Contains(out, `"name":"ana"`) || !strings.Contains(out, `"kid":true`) || strings.Contains(out, "elsewhere") {
		t.Fatalf("preview: %d %s", resp.StatusCode, out)
	}
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"22","kid":false}]}`); resp.StatusCode != 409 {
		t.Fatalf("restricted cleared without confirm: %d", resp.StatusCode)
	}
	// Restricted on Plex: a kid unless the request says otherwise.
	resp, out = own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"},{"id":"22"},{"id":"999"}]}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"skipped":1`) {
		t.Fatalf("import: %d %s", resp.StatusCode, out)
	}
	var role, rating string
	_ = f.s.Read().QueryRow("SELECT role, max_rating FROM users WHERE name = 'Little'").Scan(&role, &rating)
	if role != "member" || rating != "PG" {
		t.Fatalf("kid: %q %q", role, rating)
	}
	_ = f.s.Read().QueryRow("SELECT role, max_rating FROM users WHERE name = 'ana'").Scan(&role, &rating)
	if role != "member" || rating != "" {
		t.Fatalf("member: %q %q", role, rating)
	}
	// Importing again skips people already linked.
	own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
	if _, out := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"}]}`); !strings.Contains(out, `"skipped":1`) {
		t.Fatalf("again: %s", out)
	}
	// The preview names who each already-linked person is.
	if _, out := own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}"); !strings.Contains(out, `"id":"21","name":"ana","email":"ana@example.test","kid":false,"server_admin":false,"linked_to":"ana"`) {
		t.Fatalf("linked person: %s", out)
	}
	resp, out = own.do(t, f, "DELETE", "/api/v1/auth/plex/link", "")
	if resp.StatusCode != 200 || !strings.Contains(out, `"linked":false`) || !strings.Contains(out, `"can_import":false`) {
		t.Fatalf("unlink: %d %s", resp.StatusCode, out)
	}
	// The preview ran out after a while.
	f.a.mu.Lock()
	f.a.offers[ProviderPlex].expires = f.clock.t.Add(-time.Minute)
	f.a.mu.Unlock()
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"}]}`); resp.StatusCode != 409 {
		t.Fatalf("old preview: %d", resp.StatusCode)
	}
}

func TestSignInPinCannotLink(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"join":"nobody"}}`)
	resp, _ := own.do(t, f, "POST", "/api/v1/auth/plex/start", "")
	if resp.StatusCode != 200 {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 404 {
		t.Fatalf("a sign-in PIN linked: %d", resp.StatusCode)
	}
}

// route: POST /api/v1/auth/media-server/{kind}/test
func TestMediaServerAddressTest(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("GET /System/Info/Public", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ServerName":"Den","Version":"10.9.0","ProductName":"Jellyfin Server","Id":"x1"}`))
	})
	m.HandleFunc("GET /away/System/Info/Public", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	})
	ms := httptest.NewServer(m)
	t.Cleanup(ms.Close)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	test := func(u string) (int, string) {
		b, _ := json.Marshal(map[string]string{"url": u})
		resp, out := own.do(t, f, "POST", "/api/v1/auth/media-server/jellyfin/test", string(b))
		return resp.StatusCode, out
	}
	if code, out := test(ms.URL + "/"); code != 200 || !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"name":"Den"`) {
		t.Fatalf("good server: %d %s", code, out)
	}
	if code, out := test(ms.URL + "/away"); code != 200 || !strings.Contains(out, `"ok":false`) {
		t.Fatalf("redirect followed: %d %s", code, out)
	}
	notJf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "gone", 418) }))
	t.Cleanup(notJf.Close)
	if code, out := test(notJf.URL); code != 200 || !strings.Contains(out, `"ok":false`) || strings.Contains(out, "418") || strings.Contains(out, "status") {
		t.Fatalf("other answer leaks detail: %d %s", code, out)
	}
	if code, out := test("http://169.254.169.254"); code != 200 || !strings.Contains(out, `"ok":false`) {
		t.Fatalf("link-local: %d %s", code, out)
	}
	for _, bad := range []string{"file:///etc/passwd", "http://user:pw@host", "gopher://x", "http://h/?q=1"} {
		if code, _ := test(bad); code != 400 {
			t.Fatalf("%s: %d", bad, code)
		}
	}
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/media-server/plex/test", `{"url":"http://x"}`); resp.StatusCode != 404 {
		t.Fatalf("plex test: %d", resp.StatusCode)
	}
}

func TestJellyfinImportNeedsAnAdmin(t *testing.T) {
	var logouts atomic.Int32
	m := http.NewServeMux()
	m.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		switch {
		case b.Username == "admin" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"a1","Name":"admin","Policy":{"IsAdministrator":true}},"AccessToken":"tok"}`))
		case b.Username == "mum" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"m1","Name":"Mum","Policy":{"IsAdministrator":false}},"AccessToken":"tok"}`))
		default:
			http.Error(w, "no", 401)
		}
	})
	m.HandleFunc("GET /Users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "tok" {
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte(`[{"Id":"a1","Name":"admin","Policy":{"IsAdministrator":true}},{"Id":"m1","Name":"Mum","Policy":{}},{"Id":"o1","Name":"Off","Policy":{"IsDisabled":true}},{"Id":"k1","Name":"Kiddo","Policy":{"MaxParentalRating":10}}]`))
	})
	m.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, _ *http.Request) { logouts.Add(1); w.WriteHeader(204) })
	ms := httptest.NewServer(m)
	t.Cleanup(ms.Close)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`","join":"nobody"}}`)
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/import/jellyfin/preview", `{"name":"mum","password":"right"}`); resp.StatusCode != 403 {
		t.Fatalf("non-admin listed users: %d", resp.StatusCode)
	}
	resp, out := own.do(t, f, "POST", "/api/v1/auth/import/jellyfin/preview", `{"name":"admin","password":"right"}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"server_admin":true`) || strings.Contains(out, "Off") || strings.Contains(out, "right") {
		t.Fatalf("preview: %d %s", resp.StatusCode, out)
	}
	if logouts.Load() != 2 {
		t.Fatalf("sessions ended: %d", logouts.Load())
	}
	// Parental limits on the server mark a Kid, as signing in themselves would.
	if strings.Count(out, `"kid":true`) != 1 || !strings.Contains(out, "Kiddo") {
		t.Fatalf("limited person not marked a Kid: %s", out)
	}
	own.do(t, f, "POST", "/api/v1/auth/import/jellyfin", `{"people":[{"id":"a1"},{"id":"m1"},{"id":"k1"}]}`)
	var admins int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users WHERE role != 'member' AND role != 'owner'").Scan(&admins)
	if admins != 0 {
		t.Fatal("a server admin became a HolyArrs admin")
	}
	var rating string
	_ = f.s.Read().QueryRow("SELECT max_rating FROM users WHERE name = 'Kiddo'").Scan(&rating)
	if rating != "PG" {
		t.Fatalf("imported Kid limit: %q", rating)
	}
}

func TestJellyfinImportDoesNotFollowRedirects(t *testing.T) {
	var leaked atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { leaked.Add(1) }))
	t.Cleanup(elsewhere.Close)
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(ms.Close)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`","join":"nobody"}}`)
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/import/jellyfin/preview", `{"name":"admin","password":"right"}`); resp.StatusCode/100 == 2 {
		t.Fatalf("redirected import: %d", resp.StatusCode)
	}
	if leaked.Load() != 0 {
		t.Fatal("the admin password followed a redirect")
	}
}

func TestEveryoneNeedsConfirming(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	body := `{"jellyfin":{"on":true,"url":"http://192.168.1.10:8096","join":"everyone"}}`
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 409 {
		t.Fatalf("unconfirmed everyone: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/settings", strings.Replace(body, `}}`, `},"confirm_everyone":true}`, 1)); resp.StatusCode != 200 {
		t.Fatalf("confirmed: %d", resp.StatusCode)
	}
	// Already everyone: saving again needs no new confirm.
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 200 {
		t.Fatalf("resave: %d", resp.StatusCode)
	}
}

func TestMemberCannotSetUpSignIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	for _, r := range []string{"POST /api/v1/auth/plex/link/start", "POST /api/v1/auth/media-server/jellyfin/test", "POST /api/v1/auth/import/plex/preview", "POST /api/v1/auth/import/plex", "DELETE /api/v1/auth/plex/link", "PUT /api/v1/auth/settings"} {
		method, path, _ := strings.Cut(r, " ")
		if resp, _ := ana.do(t, f, method, path, `{}`); resp.StatusCode != 403 {
			t.Errorf("%s as a member: %d", r, resp.StatusCode)
		}
	}
}
