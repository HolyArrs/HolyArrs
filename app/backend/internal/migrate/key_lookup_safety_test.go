package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyLookupRouter serves the key lookup with plex as the stored Plex token.
func keyLookupRouter(t *testing.T, plex string) *webserver.Router {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false },
		PlexToken: func(context.Context) string { return plex }}
	h.Routes(r)
	return r
}

func postKeyLookup(r http.Handler, via string, body map[string]any) *httptest.ResponseRecorder {
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: via})
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/key-lookup", bytes.NewReader(b)).WithContext(ctx))
	return rec
}

// The answer to the browser never holds the password or Plex token sent.
func TestKeyLookupAnswerHoldsNoSignIn(t *testing.T) {
	bazarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(bazarr.Close)
	arr := fakeArr("forms")
	t.Cleanup(arr.Close)
	seerr := fakeSeerr()
	t.Cleanup(seerr.Close)
	r := keyLookupRouter(t, testPlex)
	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"arr", map[string]any{"kind": "radarr", "url": arr.URL, "username": testUser, "password": testPass}},
		{"arr wrong", map[string]any{"kind": "radarr", "url": arr.URL, "username": testUser, "password": testPass + "x"}},
		{"bazarr", map[string]any{"kind": "bazarr", "url": bazarr.URL, "username": testUser, "password": testPass}},
		{"plex", map[string]any{"kind": "seerr", "url": seerr.URL, "use_plex": true}},
	} {
		c.body["allow_local_network"] = true
		rec := postKeyLookup(r, "session", c.body)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
		if out := rec.Body.String(); strings.Contains(out, testPass) || strings.Contains(out, testPlex) {
			t.Errorf("%s: a sign-in went back to the browser: %s", c.name, out)
		}
	}
}

// An API key may not send the stored Plex token anywhere.
func TestKeyLookupPlexNeedsSession(t *testing.T) {
	var hits atomic.Int32
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(app.Close)
	r := keyLookupRouter(t, testPlex)
	rec := postKeyLookup(r, "api_key", map[string]any{"kind": "seerr", "url": app.URL, "allow_local_network": true, "use_plex": true})
	if rec.Code != http.StatusForbidden || hits.Load() != 0 || strings.Contains(rec.Body.String(), testPlex) {
		t.Fatalf("api key with use_plex: %d hits %d %s", rec.Code, hits.Load(), rec.Body)
	}
	// Without use_plex an API key still looks a key up.
	if rec := postKeyLookup(r, "api_key", map[string]any{"kind": "seerr", "url": app.URL, "allow_local_network": true}); rec.Code != http.StatusOK {
		t.Fatalf("api key without plex: %d", rec.Code)
	}
}

// A redirect is never followed: the key on the far side is not fetched.
func TestKeyLookupRefusesRedirects(t *testing.T) {
	var far atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		far.Add(1)
		_, _ = w.Write([]byte(`{"apiKey":"` + lookupKey + `"}`))
	}))
	t.Cleanup(other.Close)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(app.Close)
	if r := LookupKeyWith(context.Background(), Radarr, app.URL, true, KeyLogin{Username: testUser, Password: testPass}); r.Key != "" {
		t.Fatalf("followed a redirect: %+v", r)
	}
	if far.Load() != 0 {
		t.Fatalf("the other host was reached %d times", far.Load())
	}
}

// The lookup's cookie jar keeps and sends cookies for its one host only.
func TestHostJarOneHost(t *testing.T) {
	home, _ := url.Parse("http://10.0.0.5:7878/login")
	away, _ := url.Parse("http://evil.example:7878/")
	j := &hostJar{host: home.Host}
	j.SetCookies(home, []*http.Cookie{{Name: "a", Value: "1"}})
	j.SetCookies(away, []*http.Cookie{{Name: "b", Value: "2"}})
	if got := j.Cookies(away); len(got) != 0 {
		t.Fatalf("cookies sent to another host: %v", got)
	}
	got := j.Cookies(home)
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("home cookies %v, want only a", got)
	}
}

// What the app says when it refuses never reaches the browser, and the log
// holds only a fixed reason (never the login typed).
func TestKeyLookupHidesAppErrorText(t *testing.T) {
	const remote = "REMOTE-ERROR-TEXT"
	email := "someone" + "@example.org"
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/local" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"` + remote + ` no user ` + email + `"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(app.Close)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r := keyLookupRouter(t, "")
	rec := postKeyLookup(r, "session", map[string]any{"kind": "overseerr", "url": app.URL, "allow_local_network": true, "username": email, "password": testPass})
	if out := rec.Body.String(); strings.Contains(out, remote) || strings.Contains(out, email) || !strings.Contains(out, keyWrongLogin) {
		t.Fatalf("answer: %s", out)
	}
	if l := logs.String(); strings.Contains(l, remote) || strings.Contains(l, email) || !strings.Contains(l, "refused") {
		t.Fatalf("log: %s", l)
	}
	if got := appReason([]byte(`{"error":"Invalid credentials for ` + email + `"}`)); got != "invalid credentials" {
		t.Fatalf("known reason: %q", got)
	}
}
