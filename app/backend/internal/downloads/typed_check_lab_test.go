//go:build lab

package downloads

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A saved qBittorrent Downloader with an old password: Test with the new one
// typed in the Change form signs in with it; the masked one sends the saved
// password. A proxy in front of the real qBittorrent sees which password each
// sign-in carries (compared here, never printed). The lab's qBittorrent skips
// sign-in for local addresses, so that is turned off for the run and back on
// after it.
func TestLabQbitTypedPassword(t *testing.T) {
	user, pass := os.Getenv("HOLYARRS_LAB_QBIT_USER"), os.Getenv("HOLYARRS_LAB_QBIT_PASS")
	if user == "" || pass == "" {
		t.Skip("HOLYARRS_LAB_QBIT_USER/PASS not set (run through oldnew.py)")
	}
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://127.0.0.1:18708"
	}
	target, _ := url.Parse(base)
	labBypass(t, base, user, pass, false)
	t.Cleanup(func() { labBypass(t, base, user, pass, true) })
	var mu sync.Mutex
	type signIn struct{ pass, answer string }
	var sent []signIn // each sign-in's password and qBittorrent's answer
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.ModifyResponse = func(r *http.Response) error {
		if r.Request.URL.Path == "/api/v2/auth/login" {
			b, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(b))
			mu.Lock()
			sent = append(sent, signIn{r.Request.Header.Get("X-Lab-Pass"), string(b)})
			mu.Unlock()
		}
		return nil
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			b, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(b))
			f, _ := url.ParseQuery(string(b))
			r.Header.Set("X-Lab-Pass", f.Get("password"))
		}
		r.Header.Del("Origin")
		r.Header.Del("Referer")
		r.Host = target.Host
		rp.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	old := "old-password-before-the-change"
	ref := int64(1)
	saved := domain.Downloader{ID: 1, Name: "qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: proxy.URL, Username: user, SecretRef: &ref, AllowLocal: true}
	m := &Manager{FromVault: func(context.Context, int64) (string, error) { return old, nil }}
	ctx := context.Background()
	for _, c := range []struct {
		name, typed, want string
	}{
		{"new password typed", pass, pass},
		{"password left masked", secrets.Masked, old},
	} {
		mu.Lock()
		sent = nil
		mu.Unlock()
		_, h, err := m.CheckTyped(ctx, saved, Input{Name: saved.Name, URL: proxy.URL, Username: &user, Secret: c.typed})
		mu.Lock()
		got := append([]signIn(nil), sent...)
		mu.Unlock()
		if len(got) == 0 {
			t.Fatalf("%s: no sign-in reached qBittorrent", c.name)
		}
		for _, g := range got {
			if g.pass != c.want {
				t.Fatalf("%s: sign-in carried another password", c.name)
			}
		}
		if c.want == pass {
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if a := got[0].answer; a == "Fails." { // 4.x says "Ok.", 5.x answers empty
				t.Fatalf("%s: qBittorrent answered %q", c.name, a)
			}
			t.Logf("%s: qBittorrent %s accepted the typed password", c.name, h.Version)
		} else {
			t.Logf("%s: the saved password was sent (answer %q)", c.name, got[0].answer)
		}
	}
}

// labBypass turns the lab qBittorrent's sign-in skip for local addresses on or off.
func labBypass(t *testing.T, base, user, pass string, on bool) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	r, err := c.PostForm(base+"/api/v2/auth/login", url.Values{"username": {user}, "password": {pass}})
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	v := "false"
	if on {
		v = "true"
	}
	r, err = c.PostForm(base+"/api/v2/app/setPreferences", url.Values{"json": {`{"bypass_local_auth":` + v + `,"bypass_auth_subnet_whitelist_enabled":` + v + `}`}})
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("set sign-in skip %v: %v", on, err)
	}
	r.Body.Close()
}
