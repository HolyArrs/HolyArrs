package mediaservers

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// authSeen is what a fake Subsonic server saw.
type authSeen struct {
	mu       sync.Mutex
	plain    int      // calls that carried the password itself
	token    int      // calls that carried the salted token
	salts    []string // every salt sent
	inURL    int      // calls with a sign-in in the address
	posts    int      // form POSTs
	extAsked int
}

// authFake plays a Subsonic server for user "luis" with password pw.
// tokenOK false refuses the salted token with code 41 (LDAP); formPost
// says it lists the OpenSubsonic formPost extension.
func authFake(t *testing.T, pw string, tokenOK, formPost bool) (*httptest.Server, *authSeen) {
	t.Helper()
	seen := &authSeen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.mu.Lock()
		defer seen.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/getOpenSubsonicExtensions" {
			seen.extAsked++
			ext := `[]`
			if formPost {
				ext = `[{"name":"formPost","versions":[1]}]`
			}
			fmt.Fprintf(w, `{"subsonic-response":{"status":"ok","version":"1.16.1","openSubsonic":true,"openSubsonicExtensions":%s}}`, ext)
			return
		}
		q := r.URL.Query()
		if q.Get("u") != "" || q.Get("t") != "" || q.Get("p") != "" {
			seen.inURL++
		}
		if r.Method == http.MethodPost {
			seen.posts++
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		_ = r.ParseForm()
		f := r.Form
		fail := func(code int) {
			fmt.Fprintf(w, `{"subsonic-response":{"status":"failed","version":"1.16.1","error":{"code":%d,"message":"no"}}}`, code)
		}
		if f.Get("u") != "luis" {
			fail(10)
			return
		}
		if f.Get("p") != "" {
			seen.plain++
			if f.Get("p") != "enc:"+hex.EncodeToString([]byte(pw)) {
				fail(40)
				return
			}
		} else {
			seen.token++
			seen.salts = append(seen.salts, f.Get("s"))
			if !tokenOK {
				fail(41)
				return
			}
			sum := md5.Sum([]byte(pw + f.Get("s")))
			if f.Get("t") != hex.EncodeToString(sum[:]) {
				fail(40)
				return
			}
		}
		fmt.Fprint(w, `{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome","serverVersion":"0.58.0"}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

// The salted token is the default, with a fresh salt on every call.
func TestSubsonicTokenByDefault(t *testing.T) {
	w := newWorld(t)
	srv, seen := authFake(t, "pw", true, false)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if m.PlainPassword {
		t.Fatal("plain password on by default")
	}
	l := &Linker{Store: w.s, Vault: w.v}
	for i := 0; i < 2; i++ {
		if _, err := l.Test(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
	if seen.plain != 0 || seen.token != 2 || seen.salts[0] == seen.salts[1] || len(seen.salts[0]) < 16 {
		t.Fatalf("plain %d token %d salts %v", seen.plain, seen.token, seen.salts)
	}
}

// A 41/42 refusal of the token never makes HolyArrs send the password on
// its own: the owner is told to turn Plain password on.
func TestSubsonicNoPlainRetry(t *testing.T) {
	for _, code := range []int{41, 42} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			w := newWorld(t)
			var mu sync.Mutex
			var plain, calls int
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				_ = r.ParseForm()
				if r.URL.Path == "/rest/getOpenSubsonicExtensions" {
					fmt.Fprint(rw, `{"subsonic-response":{"status":"ok","openSubsonicExtensions":[]}}`)
					return
				}
				calls++
				if r.Form.Get("p") != "" {
					plain++
				}
				fmt.Fprintf(rw, `{"subsonic-response":{"status":"failed","error":{"code":%d}}}`, code)
			}))
			t.Cleanup(srv.Close)
			m, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			_, err = l.Test(context.Background(), m)
			if !errors.Is(err, errTokenRefused) || !strings.Contains(err.Error(), "Plain password") {
				t.Fatalf("err = %v", err)
			}
			if err := l.Tell(context.Background(), m, []Update{{Path: "/music/A", Change: Created, Kind: "artist"}}); !errors.Is(err, errTokenRefused) {
				t.Fatalf("scan err = %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if plain != 0 || calls != 2 {
				t.Fatalf("plain retries %d, calls %d", plain, calls)
			}
		})
	}
}

// Plain password only when the owner turned it on, and then it signs in.
func TestSubsonicPlainWhenTurnedOn(t *testing.T) {
	w := newWorld(t)
	srv, seen := authFake(t, "pw", false, false)
	on := true
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw", PlainPassword: &on}, domain.System)
	if err != nil || !m.PlainPassword {
		t.Fatalf("%v %v", m.PlainPassword, err)
	}
	m = mustGet(t, w, m.ID)
	if !m.PlainPassword || !ViewOf(m).PlainPassword {
		t.Fatal("plain password not kept")
	}
	if _, err := (&Linker{Store: w.s, Vault: w.v}).Test(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if seen.plain != 1 || seen.token != 0 {
		t.Fatalf("plain %d token %d", seen.plain, seen.token)
	}
	// Another kind never keeps it.
	j, err := Add(context.Background(), w.s, w.v, Input{Name: "J", Kind: "jellyfin", URL: srv.URL, Token: "k", PlainPassword: &on}, domain.System)
	if err != nil || j.PlainPassword {
		t.Fatalf("jellyfin plain %v %v", j.PlainPassword, err)
	}
}

// The password itself never goes over plain http to an outside host.
func TestSubsonicPlainNeverOverOutsideHTTP(t *testing.T) {
	for raw, ok := range map[string]bool{
		"http://navidrome.example.com": false, "http://8.8.8.8:4533": false,
		"https://navidrome.example.com": true, "http://192.168.1.5:4533": true, "http://127.0.0.1:4533": true,
		"http://localhost:4533": true, "http://navidrome:4533": true, "http://music.lan": true, "http://[::1]:4533": true,
	} {
		if plainSafe(raw) != ok {
			t.Errorf("plainSafe(%s) = %v", raw, !ok)
		}
	}
	w := newWorld(t)
	on := true
	_, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: "http://music.example.com", Username: "luis", Token: "pw", PlainPassword: &on}, domain.System)
	if !errors.Is(err, ErrPlainHTTP) || !IsInputError(err) {
		t.Fatalf("outside http: %v", err)
	}
	// A stored server that somehow has it is still refused before sending.
	m := domain.MediaServer{Kind: "subsonic", URL: "http://music.example.com", PlainPassword: true}
	if _, err := (&Linker{Store: w.s, Vault: w.v}).subsonicCall(context.Background(), m, "ping"); !errors.Is(err, ErrPlainHTTP) {
		t.Fatalf("stored outside http: %v", err)
	}
}

// With OpenSubsonic formPost the sign-in goes in a POST body, never the
// address; without it the address carries it. Asked once per address.
func TestSubsonicFormPost(t *testing.T) {
	for _, fp := range []bool{true, false} {
		t.Run(fmt.Sprint(fp), func(t *testing.T) {
			w := newWorld(t)
			srv, seen := authFake(t, "pw", true, fp)
			m, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			for i := 0; i < 2; i++ {
				if _, err := l.Test(context.Background(), m); err != nil {
					t.Fatal(err)
				}
			}
			if seen.extAsked != 1 {
				t.Fatalf("extensions asked %d times", seen.extAsked)
			}
			if fp && (seen.posts != 2 || seen.inURL != 0) {
				t.Fatalf("posts %d, sign-in in address %d", seen.posts, seen.inURL)
			}
			if !fp && (seen.posts != 0 || seen.inURL != 2) {
				t.Fatalf("posts %d, address %d", seen.posts, seen.inURL)
			}
		})
	}
}

// No error, log line or stored last error holds the address's query, the
// salted token or the password, and no API view returns the password.
func TestSubsonicSecretsNeverShown(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	w := newWorld(t)
	ctx := context.Background()
	const pw = "hunter2-very-secret"
	on := true
	srv, _ := authFake(t, pw, true, false)
	m, err := Add(ctx, w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: pw, PlainPassword: &on}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close() // down: a raw request error would name the address
	l := &Linker{Store: w.s, Vault: w.v, Delay: 1}
	_, terr := l.Test(ctx, m)
	if terr == nil {
		t.Fatal("down server passed")
	}
	l.queue(ctx, "/music/A/One", Created, "artist")
	l.Flush(ctx)
	m = mustGet(t, w, m.ID)
	if m.LastError == "" {
		t.Fatal("no last error stored")
	}
	view, _ := json.Marshal(ViewOf(m))
	hexPW := hex.EncodeToString([]byte(pw))
	for _, where := range []string{terr.Error(), logs.String(), m.LastError, string(view)} {
		for _, bad := range []string{pw, hexPW, "enc:", "u=luis", "&t=", "&s=", "/rest/"} {
			if strings.Contains(where, bad) {
				t.Fatalf("%q shows %q", where, bad)
			}
		}
	}
	// withoutAddress drops every wrapped url.Error, however deep.
	ue := &url.Error{Op: "Get", URL: srv.URL + "/rest/ping?u=luis&p=enc:" + hexPW, Err: errors.New("connection refused")}
	if e := withoutAddress(fmt.Errorf("x: %w", ue)); strings.Contains(e.Error(), hexPW) {
		t.Fatalf("kept address: %v", e)
	}
}
