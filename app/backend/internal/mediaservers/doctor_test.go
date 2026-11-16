package mediaservers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestMediaServerTestNamesEachCause(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/key/"):
			w.WriteHeader(http.StatusUnauthorized)
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
		{"name not found", "http://nas.invalid:8096", true, doctor.DNS},
		{"refused", closed, true, doctor.Refused},
		{"self-signed certificate", secure.URL, true, doctor.TLS},
		{"https on an http port", strings.Replace(plain.URL, "http://", "https://", 1), true, doctor.UseHTTP},
		{"http on an https port", strings.Replace(secure.URL, "https://", "http://", 1), true, doctor.UseHTTPS},
		{"token refused", plain.URL + "/key", true, doctor.Auth},
		{"wrong path", plain.URL + "/nothing", true, doctor.NotFound},
		{"another app", plain.URL + "/page", true, doctor.WrongApp},
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
