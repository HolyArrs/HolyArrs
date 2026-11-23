package mediaservers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fakeSubsonic plays a Subsonic-style server with user "luis" and password
// pw. tokenOK false plays an LDAP-backed server that only takes the
// password itself. It counts library scans.
func fakeSubsonic(t *testing.T, pw string, tokenOK bool) (*httptest.Server, func() (scans, calls int)) {
	t.Helper()
	var mu sync.Mutex
	scans, calls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fail := func(code int) {
			fmt.Fprintf(w, `{"subsonic-response":{"status":"failed","version":"1.16.1","error":{"code":%d,"message":"no"}}}`, code)
		}
		if q.Get("u") != "luis" || q.Get("c") != "HolyArrs" || q.Get("f") != "json" {
			fail(10)
			return
		}
		if q.Get("t") != "" {
			if !tokenOK {
				fail(41)
				return
			}
			sum := md5.Sum([]byte(pw + q.Get("s")))
			if q.Get("t") != hex.EncodeToString(sum[:]) || len(q.Get("s")) < 6 {
				fail(40)
				return
			}
		} else if q.Get("p") != "enc:"+hex.EncodeToString([]byte(pw)) {
			fail(40)
			return
		}
		switch r.URL.Path {
		case "/rest/ping":
			fmt.Fprint(w, `{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome","serverVersion":"0.58.0"}}`)
		case "/rest/startScan":
			scans++
			fmt.Fprint(w, `{"subsonic-response":{"status":"ok","version":"1.16.1","scanStatus":{"scanning":true}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() (int, int) { mu.Lock(); defer mu.Unlock(); return scans, calls }
}

func TestSubsonicTestAndScan(t *testing.T) {
	for _, tokenOK := range []bool{true, false} {
		t.Run(fmt.Sprintf("token=%v", tokenOK), func(t *testing.T) {
			w := newWorld(t)
			srv, count := fakeSubsonic(t, "s3cret pw", tokenOK)
			// An LDAP-backed server needs the owner's Plain password choice.
			plain := !tokenOK
			m, err := Add(context.Background(), w.s, w.v, Input{Name: "Navidrome", Kind: "subsonic", URL: srv.URL, Username: " luis ", Token: "s3cret pw", PlainPassword: &plain}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			got, err := l.Test(context.Background(), m)
			if err != nil || got != "Navidrome 0.58.0" {
				t.Fatalf("test = %q, %v", got, err)
			}
			// One scan for a whole batch of music folders.
			err = l.Tell(context.Background(), m, []Update{{Path: "/music/A/One", Change: Created, Kind: "artist"}, {Path: "/music/B/Two", Change: Created, Kind: "artist"}})
			if err != nil {
				t.Fatal(err)
			}
			if scans, _ := count(); scans != 1 {
				t.Fatalf("scans = %d, want 1", scans)
			}
		})
	}
}

// A music server only hears about music, never films or shows.
func TestSubsonicOnlyHearsMusic(t *testing.T) {
	w := newWorld(t)
	srv, count := fakeSubsonic(t, "pw", true)
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "Navidrome", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	l.queue(context.Background(), "/movies/Arrival (2016)", Created, "movie")
	l.Flush(context.Background())
	if _, calls := count(); calls != 0 {
		t.Fatalf("calls for a film = %d", calls)
	}
	l.queue(context.Background(), "/music/A/One", Created, "artist")
	l.queue(context.Background(), "/movies/Arrival (2016)", Created, "movie")
	l.Flush(context.Background())
	if scans, _ := count(); scans != 1 {
		t.Fatalf("scans = %d, want 1", scans)
	}
}

func TestSubsonicWrongPasswordAndInput(t *testing.T) {
	w := newWorld(t)
	srv, _ := fakeSubsonic(t, "right", true)
	ctx := context.Background()
	if _, err := Add(ctx, w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Token: "right"}, domain.System); !errors.Is(err, ErrNoUser) || !IsInputError(err) {
		t.Fatalf("no user name: %v", err)
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "wrong"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	if _, err := l.Test(ctx, m); doctor.Of(err) != doctor.Auth {
		t.Fatalf("wrong password = %v", err)
	}
	// A new name needs its password; an empty change keeps both.
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "N", Username: "other"}, domain.System); !errors.Is(err, ErrNoUser) {
		t.Fatalf("name without password: %v", err)
	}
	if m, err = Change(ctx, w.s, w.v, m.ID, Input{Name: "N", Username: "luis", Token: "right"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if m, err = Change(ctx, w.s, w.v, m.ID, Input{Name: "Navidrome"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Test(ctx, m); err != nil {
		t.Fatalf("kept sign-in: %v", err)
	}
}

// Another app at the address is said plainly.
func TestSubsonicWrongApp(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>hi</html>") }))
	t.Cleanup(srv.Close)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "N", Kind: "subsonic", URL: srv.URL, Username: "luis", Token: "pw"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Linker{Store: w.s, Vault: w.v}).Test(context.Background(), m); doctor.Of(err) != doctor.WrongApp {
		t.Fatalf("wrong app = %v", err)
	}
}
