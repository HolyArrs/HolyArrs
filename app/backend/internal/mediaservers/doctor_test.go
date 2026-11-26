package mediaservers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func TestMediaServerTestNamesEachCause(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/key/"):
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasPrefix(r.URL.Path, "/sonarr/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<html><head><title>Sonarr</title></head></html>`))
		case strings.HasPrefix(r.URL.Path, "/nginx/"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("The plain HTTP request was sent to HTTPS port"))
		case strings.HasPrefix(r.URL.Path, "/page/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><body>Some other app</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer secure.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close()

	w := newWorld(t)
	cases := []struct {
		name  string
		url   string
		local bool
		want  doctor.Cause
	}{
		// The trailing dot makes the name absolute: a DNS search domain with a
		// wildcard record would otherwise turn the name into one that resolves.
		{"name not found", "http://nas.invalid.:8096", true, doctor.DNS},
		{"refused", closed, true, doctor.Refused},
		{"self-signed certificate", secure.URL, true, doctor.TLS},
		{"https on an http port", strings.Replace(plain.URL, "http://", "https://", 1), true, doctor.UseHTTP},
		{"http on an https port", strings.Replace(secure.URL, "https://", "http://", 1), true, doctor.UseHTTPS},
		{"token refused", plain.URL + "/key", true, doctor.Auth},
		{"wrong path", plain.URL + "/nothing", true, doctor.NotFound},
		{"another app", plain.URL + "/page", true, doctor.WrongApp},
		{"another app named", plain.URL + "/sonarr", true, doctor.WrongApp},
		{"nginx https port", plain.URL + "/nginx", true, doctor.UseHTTPS},
		{"https on a plain port that drops TLS", "https://" + picky(t), true, doctor.UseHTTP},
		{"local network policy", plain.URL, false, doctor.Blocked},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			local := tc.local
			m, err := Add(ctx, w.s, w.v, Input{Name: "Doctor " + string(rune('a'+i)), Kind: "jellyfin", URL: tc.url, Token: "x", AllowLocal: &local}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			_, err = l.Test(ctx, m)
			if got := doctor.Of(err); got != tc.want {
				t.Fatalf("cause %q, want %q (error %v)", got, tc.want, err)
			}
			if err.Error() != doctor.Line(tc.want) {
				t.Fatalf("text %q", err.Error())
			}
			// The stored last error reads as the same cause on the health row.
			m, _ = getServer(ctx, w.s.Read(), m.ID)
			if got := doctor.OfText(m.LastError); got != tc.want {
				t.Fatalf("stored %q reads as %q", m.LastError, got)
			}
		})
	}
}

func TestPlexTestNamesAnotherApp(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body>Sonarr</body></html>"))
	}))
	defer page.Close()
	w := newWorld(t)
	ctx := context.Background()
	local := true
	m, err := Add(ctx, w.s, w.v, Input{Name: "Plex page", Kind: "plex", URL: page.URL, Token: "x", AllowLocal: &local}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&Linker{Store: w.s, Vault: w.v}).Test(ctx, m)
	if got := doctor.Of(err); got != doctor.WrongApp {
		t.Fatalf("cause %q (error %v)", got, err)
	}
}

// picky is a plain http port that hangs up on a TLS hello without a word but
// answers plain http.
func picky(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				b := make([]byte, 1)
				if _, err := c.Read(b); err != nil || b[0] == 0x16 {
					return
				}
				_, _ = c.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}()
		}
	}()
	return l.Addr().String()
}

// After a few failures the outbound breaker holds calls back; the Test line
// stays plain words and the stored cause stays the real one.
func TestMediaServerTestWhileHeldBackIsPlain(t *testing.T) {
	outbound.Reset()
	t.Cleanup(outbound.Reset)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close()
	w := newWorld(t)
	ctx := context.Background()
	local := true
	m, err := Add(ctx, w.s, w.v, Input{Name: "Held back", Kind: "jellyfin", URL: closed, Token: "x", AllowLocal: &local}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	lk := &Linker{Store: w.s, Vault: w.v}
	held := false
	for range 12 {
		_, err = lk.Test(ctx, m)
		if errors.Is(err, outbound.ErrSuppressed) {
			held = true
			break
		}
	}
	if !held {
		t.Skip("the breaker never held the calls back")
	}
	if strings.Contains(err.Error(), "outside service") || strings.Contains(err.Error(), "retrying") {
		t.Fatalf("raw breaker words on the Test line: %q", err.Error())
	}
	m, _ = getServer(ctx, w.s.Read(), m.ID)
	if got := doctor.OfText(m.LastError); got != doctor.Refused {
		t.Fatalf("stored %q reads as %q", m.LastError, got)
	}
	// A stored cause nobody knows still gives a translated line.
	markError(ctx, w.s, m.ID, "something odd")
	m, _ = getServer(ctx, w.s.Read(), m.ID)
	_, err = lk.Test(ctx, m)
	if !errors.Is(err, outbound.ErrSuppressed) {
		t.Skip("the breaker let the call through")
	}
	if err.Error() != doctor.Line(doctor.Timeout) {
		t.Fatalf("held line %q is not a known line", err.Error())
	}
}

// Jellyfin answers Plex's paths with an empty 404 that names only Kestrel.
func TestPlexAtJellyfinNamesAnotherApp(t *testing.T) {
	jf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Kestrel")
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer jf.Close()
	w := newWorld(t)
	ctx := context.Background()
	local := true
	m, err := Add(ctx, w.s, w.v, Input{Name: "Plex at Jellyfin", Kind: "plex", URL: jf.URL, Token: "x", AllowLocal: &local}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&Linker{Store: w.s, Vault: w.v}).Test(ctx, m)
	if got := doctor.Of(err); got != doctor.WrongApp {
		t.Fatalf("cause %q (error %v)", got, err)
	}
}
