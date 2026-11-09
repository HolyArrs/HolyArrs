package accounts

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// route: POST /api/v1/auth/setup/plex/start
// route: POST /api/v1/auth/setup/plex/finish
func TestSetupOwnerWithPlex(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	c := client(t)
	_, st := call(t, c, "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(st, `"setup_ways":["password","plex","jellyfin","emby"]`) {
		t.Fatalf("state before setup: %s", st)
	}
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup/plex/start", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(out, "ABCD") || !strings.Contains(out, "/auth#?") {
		t.Fatalf("start: %d %s", resp.StatusCode, out)
	}
	// A setup PIN never signs anyone in through the normal Plex way.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("setup pin on the sign-in route: %d", resp.StatusCode)
	}
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup/plex/finish", `{"id":77}`, nil); resp.StatusCode != 202 {
		t.Fatalf("waiting: %d", resp.StatusCode)
	}
	f.clock.add(2 * time.Second)
	p.confirmed.Store(true)
	resp, out = call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup/plex/finish", `{"id":77}`, nil)
	if resp.StatusCode != 201 || !strings.Contains(out, `"csrf"`) {
		t.Fatalf("finish: %d %s", resp.StatusCode, out)
	}
	_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	if !strings.Contains(me, `"name":"kidplex"`) || !strings.Contains(me, `"role":"owner"`) || !strings.Contains(me, `"auth_method":"plex"`) {
		t.Fatalf("me: %s", me)
	}
	id, _, ok := f.a.owner(context.Background())
	if !ok {
		t.Fatal("no owner")
	}
	var hash string
	if err := f.s.Read().QueryRow("SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil || hash != "" {
		t.Fatalf("owner password %q %v", hash, err)
	}
	if tok, err := f.a.PlexToken(context.Background(), id); err != nil || tok != plexTok {
		t.Fatalf("owner plex token %q %v", tok, err)
	}
	if po := f.a.plexOwner(context.Background()); po["linked"] != true || po["can_import"] != true || po["name"] != "kidplex" {
		t.Fatalf("plex owner %v", po)
	}
	if !f.a.way(context.Background(), ProviderPlex).On {
		t.Fatal("plex way not turned on")
	}
	_, st = call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if strings.Contains(st, "setup_ways") || !strings.Contains(st, `"needs_setup":false`) || !strings.Contains(st, `"plex"`) {
		t.Fatalf("state after setup: %s", st)
	}
	// The owner exists: both setup routes refuse.
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/plex/start", "", nil); resp.StatusCode != 409 {
		t.Fatalf("start after setup: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/plex/finish", `{"id":77}`, nil); resp.StatusCode != 409 {
		t.Fatalf("finish after setup: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"other","password":"`+pw+`"}`, nil); resp.StatusCode != 409 {
		t.Fatalf("password setup after plex setup: %d", resp.StatusCode)
	}
}

// fakeSetupServer answers AuthenticateByName like Jellyfin or Emby, with an
// admin "boss" and a plain "mum".
func fakeSetupServer(t *testing.T) *httptest.Server {
	m := http.NewServeMux()
	m.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "MediaBrowser ") {
			http.Error(w, "auth header", 400)
			return
		}
		var b struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		switch {
		case b.Username == "boss" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"admin1","Name":"Boss","Policy":{"IsAdministrator":true}},"AccessToken":"tok"}`))
		case b.Username == "mum" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"mum1","Name":"Mum","Policy":{"IsAdministrator":false}},"AccessToken":"tok"}`))
		default:
			http.Error(w, "Error processing request.", 401)
		}
	})
	m.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	s := httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(s.Close)
	return s
}

// route: POST /api/v1/auth/setup/media-server/{kind}
func TestSetupOwnerWithMediaServer(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			ms := fakeSetupServer(t)
			gone := httptest.NewServer(http.NotFoundHandler())
			goneURL := gone.URL
			gone.Close()
			f := gated(t, Options{})
			try := func(c *http.Client, u, name, pass string) (int, string) {
				b, _ := json.Marshal(map[string]string{"url": u, "name": name, "password": pass})
				resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup/media-server/"+kind, string(b), nil)
				return resp.StatusCode, out
			}
			if code, out := try(client(t), "file:///etc", "boss", "right"); code != 400 {
				t.Fatalf("odd address: %d %s", code, out)
			}
			if code, out := try(client(t), goneURL, "boss", "right"); code != 502 || !strings.Contains(out, "could not reach") {
				t.Fatalf("unreachable: %d %s", code, out)
			}
			if code, out := try(client(t), ms.URL, "boss", "wrong"); code != 401 || !strings.Contains(out, "wrong name or password") {
				t.Fatalf("wrong password: %d %s", code, out)
			}
			if code, out := try(client(t), ms.URL, "mum", "right"); code != 403 || !strings.Contains(out, "admin account") {
				t.Fatalf("not an admin: %d %s", code, out)
			}
			if _, _, ok := f.a.owner(context.Background()); ok {
				t.Fatal("a refused try made the owner")
			}
			// No scheme and a trailing slash are fine.
			c := client(t)
			if code, out := try(c, strings.TrimPrefix(ms.URL, "http://")+"/", "boss", "right"); code != 201 {
				t.Fatalf("setup: %d %s", code, out)
			}
			_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
			if !strings.Contains(me, `"name":"Boss"`) || !strings.Contains(me, `"role":"owner"`) || !strings.Contains(me, `"auth_method":"`+kind+`"`) {
				t.Fatalf("me: %s", me)
			}
			if v := f.a.way(context.Background(), kind); !v.On || v.URL != ms.URL || v.Join != JoinNobody {
				t.Fatalf("way %+v, want on at %s", v, ms.URL)
			}
			var n int
			if err := f.s.Read().QueryRow("SELECT count(*) FROM user_links l JOIN users u ON u.id = l.user_id WHERE u.role = 'owner' AND l.provider = ? AND l.external_id = 'admin1'", kind).Scan(&n); err != nil || n != 1 {
				t.Fatalf("owner link %d %v", n, err)
			}
			// Signing in later goes through the normal way.
			b, _ := json.Marshal(map[string]string{"name": "boss", "password": "right"})
			if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/"+kind, string(b), nil); resp.StatusCode != 200 {
				t.Fatalf("sign-in after setup: %d %s", resp.StatusCode, out)
			}
			if code, _ := try(client(t), ms.URL, "boss", "right"); code != 409 {
				t.Fatalf("second setup: %d", code)
			}
		})
	}
}

func TestSetupMediaServerLimitsTries(t *testing.T) {
	ms := fakeSetupServer(t)
	f := gated(t, Options{})
	b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": "wrong"})
	for i := range setupTries {
		if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/jellyfin", string(b), nil); resp.StatusCode != 401 {
			t.Fatalf("try %d: %d", i, resp.StatusCode)
		}
	}
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/jellyfin", string(b), nil)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("after %d wrong tries: %d", setupTries, resp.StatusCode)
	}
	f.clock.add(setupWindow + time.Second)
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/jellyfin", string(b), nil); resp.StatusCode != 401 {
		t.Fatalf("after the wait: %d", resp.StatusCode)
	}
}

func TestServerAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.10:8096":         "http://192.168.1.10:8096",
		" http://jf.local:8096/ ":   "http://jf.local:8096",
		"HTTPS://media.example.com": "https://media.example.com",
		"http://nas:8096/emby/":     "http://nas:8096/emby",
		"":                          "",
		"ftp://nas":                 "",
		"http://u:p@nas:8096":       "",
		"http://nas:8096/?x=1":      "",
	} {
		got, ok := serverAddress(in)
		if got != want || ok != (want != "") {
			t.Errorf("serverAddress(%q) = %q %t, want %q", in, got, ok, want)
		}
	}
}

// route: GET /api/v1/auth/setup/discover
func TestSetupDiscover(t *testing.T) {
	f := gated(t, Options{})
	// A fake network: one Jellyfin and one Emby server answer their own message.
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := l.ReadFromUDP(buf)
			if err != nil {
				return
			}
			switch strings.ToLower(string(buf[:n])) {
			case "who is jellyfinserver?":
				_, _ = l.WriteToUDP([]byte(`{"Address":"http://192.168.1.20:8096","Id":"j","Name":"Den","EndpointAddress":null}`), from)
				_, _ = l.WriteToUDP([]byte(`not json`), from)
			case "who is embyserver?":
				_, _ = l.WriteToUDP([]byte(`{"Address":"http://192.168.1.30:8097/","Id":"e","Name":"Attic"}`), from)
			}
		}
	}()
	f.a.discoverTo, f.a.discoverWait = l.LocalAddr().String(), 400*time.Millisecond
	resp, out := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/setup/discover", "", nil)
	var got struct {
		Servers []foundServer `json:"servers"`
	}
	jsonOf(t, out, &got)
	// Each address takes the IP the answer came from, not the one it claims.
	want := []foundServer{{"emby", "Attic", "http://127.0.0.1:8097"}, {"jellyfin", "Den", "http://127.0.0.1:8096"}}
	if resp.StatusCode != 200 || len(got.Servers) != 2 || got.Servers[0] != want[0] || got.Servers[1] != want[1] {
		t.Fatalf("discover: %d %s", resp.StatusCode, out)
	}
	// Nothing answers: an empty list, never an error.
	quiet, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f.a.discoverTo = quiet.LocalAddr().String()
	t.Cleanup(func() { _ = quiet.Close() })
	resp, out = call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/setup/discover", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(out, `"servers":[]`) {
		t.Fatalf("quiet network: %d %s", resp.StatusCode, out)
	}
	ownerOf(t, f)
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/setup/discover", "", nil); resp.StatusCode != 404 {
		t.Fatalf("discover after setup: %d", resp.StatusCode)
	}
}
