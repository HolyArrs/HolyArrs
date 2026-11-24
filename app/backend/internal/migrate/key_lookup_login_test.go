package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Test sign-ins are built by joining parts so no secret scan mistakes them.
var (
	testUser = "lab" + "user"
	testPass = "pa" + "ss-" + "word"
	testPlex = "plex" + "-tok" + "en"
	seerrKey = "MTY" + "zNzA" + "wMDAwMDAwMGFiY2Rl" + "Zg=="
)

// fakeArr is a Radarr-like app whose sign-in is method: "forms", "basic" or "none".
func fakeArr(method string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			_ = r.ParseForm()
			if method == "forms" && r.PostForm.Get("username") == testUser && r.PostForm.Get("password") == testPass {
				http.SetCookie(w, &http.Cookie{Name: "RadarrAuth", Value: "ok", Path: "/"})
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/login?loginFailed=true", http.StatusFound)
		case "/initialize.json":
			ok := method == "none"
			if c, err := r.Cookie("RadarrAuth"); err == nil && c.Value == "ok" && method == "forms" {
				ok = true
			}
			if u, p, has := r.BasicAuth(); has && method == "basic" && u == testUser && p == testPass {
				ok = true
			}
			switch {
			case ok:
				_, _ = w.Write([]byte(`{"apiKey":"` + lookupKey + `"}`))
			case method == "basic":
				w.Header().Set("WWW-Authenticate", `Basic realm="Radarr"`)
				w.WriteHeader(http.StatusUnauthorized)
			default:
				http.Redirect(w, r, "/login", http.StatusFound)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestLookupKeyArrSignIns(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	good := KeyLogin{Username: testUser, Password: testPass}
	bad := KeyLogin{Username: testUser, Password: "wrong"}
	ctx := context.Background()
	for _, method := range []string{"forms", "basic", "none"} {
		srv := fakeArr(method)
		if method != "none" {
			if r := LookupKeyWith(ctx, Radarr, srv.URL, true, KeyLogin{}); !r.NeedLogin || r.Key != "" {
				t.Errorf("%s without login: %+v", method, r)
			}
			if r := LookupKeyWith(ctx, Radarr, srv.URL, true, bad); r.Problem != keyWrongLogin || r.Key != "" {
				t.Errorf("%s wrong login: %+v", method, r)
			}
		}
		if r := LookupKeyWith(ctx, Sonarr, srv.URL, true, good); r.Key != lookupKey {
			t.Errorf("%s good login: %q", method, r.Problem)
		}
		srv.Close()
	}
	if r := LookupKeyWith(ctx, Radarr, "http://127.0.0.1:1", true, KeyLogin{}); r.Problem != keyUnreachable {
		t.Errorf("unreachable: %+v", r)
	}
	if strings.Contains(logs.String(), lookupKey) || strings.Contains(logs.String(), testPass) {
		t.Error("the key or the password reached the log")
	}
}

func TestLookupKeyBazarrSignIns(t *testing.T) {
	key := strings.Repeat("cd", 16)
	page := `<html><script>window.Bazarr = {"apiKey": "` + key + `"}</script></html>`
	ctx := context.Background()
	for _, method := range []string{"forms", "basic"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/system/account":
				_ = r.ParseForm()
				if method == "forms" && r.URL.Query().Get("action") == "login" && r.PostForm.Get("username") == testUser && r.PostForm.Get("password") == testPass {
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
			case "/":
				if method == "basic" {
					if u, p, has := r.BasicAuth(); !has || u != testUser || p != testPass {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					_, _ = w.Write([]byte(page))
					return
				}
				if c, err := r.Cookie("session"); err == nil && c.Value == "ok" {
					_, _ = w.Write([]byte(page))
					return
				}
				_, _ = w.Write([]byte(`<html><script>window.Bazarr = {"apiKey": ""}</script></html>`))
			}
		}))
		if r := LookupKeyWith(ctx, Bazarr, srv.URL, true, KeyLogin{}); !r.NeedLogin {
			t.Errorf("%s without login: %+v", method, r)
		}
		if r := LookupKeyWith(ctx, Bazarr, srv.URL, true, KeyLogin{Username: testUser, Password: "x"}); r.Problem != keyWrongLogin {
			t.Errorf("%s wrong login: %+v", method, r)
		}
		if r := LookupKeyWith(ctx, Bazarr, srv.URL, true, KeyLogin{Username: testUser, Password: testPass}); r.Key != key {
			t.Errorf("%s good login: %q", method, r.Problem)
		}
		srv.Close()
	}
}

// fakeSeerr signs in admin@x (an admin) and user@x (not) by email, and the
// test Plex token as the admin.
func fakeSeerr() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		switch r.URL.Path {
		case "/api/v1/auth/local":
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["password"] != testPass || (in["email"] != "admin@x" && in["email"] != "user@x") {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "connect.sid", Value: in["email"], Path: "/"})
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/api/v1/settings/public":
			_, _ = w.Write([]byte(`{"mediaServerType":2}`))
		case "/api/v1/auth/jellyfin":
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["username"] != testUser || in["password"] != testPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "connect.sid", Value: "admin@x", Path: "/"})
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/api/v1/auth/plex":
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["authToken"] != testPlex {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "connect.sid", Value: "admin@x", Path: "/"})
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/api/v1/settings/main":
			c, err := r.Cookie("connect.sid")
			switch {
			case err != nil:
				w.WriteHeader(http.StatusUnauthorized)
			case c.Value != "admin@x":
				w.WriteHeader(http.StatusForbidden)
			default:
				_, _ = w.Write([]byte(`{"apiKey":"` + seerrKey + `","applicationTitle":"Seerr"}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestKeyLookupSeerr(t *testing.T) {
	app := fakeSeerr()
	t.Cleanup(app.Close)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		if out := logs.String(); strings.Contains(out, testPass) || strings.Contains(out, testPlex) || strings.Contains(out, seerrKey) {
			t.Error("a sign-in or the key reached the log")
		}
	})
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	plex := ""
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false },
		PlexToken: func(context.Context) string { return plex }}
	h.Routes(r)
	owner := webserver.ContextWith(httptest.NewRequest("GET", "/", nil).Context(), webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	type answer struct {
		Key           string `json:"key"`
		NeedLogin     bool   `json:"need_login"`
		PlexAvailable bool   `json:"plex_available"`
		Problem       string `json:"problem"`
		MediaServer   string `json:"media_server"`
	}
	ask := func(kind string, extra map[string]any) answer {
		body := map[string]any{"kind": kind, "url": app.URL, "allow_local_network": true}
		for k, v := range extra {
			body[k] = v
		}
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/key-lookup", bytes.NewReader(b)).WithContext(owner))
		if strings.Contains(rec.Body.String(), testPlex) || strings.Contains(rec.Body.String(), testPass) {
			t.Fatal("a sign-in went back to the browser")
		}
		var out answer
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		return out
	}
	if a := ask("seerr", map[string]any{"use_media_server": true, "username": testUser, "password": testPass}); a.Key != seerrKey {
		t.Errorf("jellyfin sign-in: %q", a.Problem)
	}
	if a := ask("seerr", map[string]any{"use_media_server": true, "username": testUser, "password": "x"}); a.Problem != keyWrongLogin {
		t.Errorf("jellyfin wrong: %+v", a)
	}
	if a := ask("overseerr", nil); a.MediaServer != "" {
		t.Errorf("overseerr offers no media server sign-in: %+v", a)
	}
	if a := ask("seerr", nil); !a.NeedLogin || a.PlexAvailable || a.Key != "" || a.MediaServer != "jellyfin" {
		t.Errorf("no login: %+v", a)
	}
	if a := ask("overseerr", map[string]any{"username": "admin@x", "password": testPass}); a.Key != seerrKey {
		t.Errorf("admin: %q", a.Problem)
	}
	if a := ask("seerr", map[string]any{"username": "user@x", "password": testPass}); a.Problem != keyNotAdmin || a.Key != "" {
		t.Errorf("not admin: %+v", a)
	}
	if a := ask("seerr", map[string]any{"username": "admin@x", "password": "x"}); a.Problem != keyWrongLogin {
		t.Errorf("wrong login: %+v", a)
	}
	if a := ask("seerr", map[string]any{"use_plex": true}); !a.NeedLogin {
		t.Errorf("plex without a token: %+v", a)
	}
	plex = testPlex
	if a := ask("seerr", nil); !a.NeedLogin || !a.PlexAvailable {
		t.Errorf("plex offered: %+v", a)
	}
	if a := ask("seerr", map[string]any{"use_plex": true}); a.Key != seerrKey {
		t.Errorf("plex sign-in: %q", a.Problem)
	}
	if a := ask("radarr", map[string]any{"use_plex": true}); a.PlexAvailable {
		t.Error("plex is offered only for Seerr")
	}
}
