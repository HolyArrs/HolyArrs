package downloads

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// doctorServers are the fake apps on 127.0.0.1 every cause is shown with.
type doctorServers struct {
	plain, secure, slow, page, other, nginx *httptest.Server
	closed, hangUp, picky                   string
}

func startDoctorServers(t *testing.T) doctorServers {
	t.Helper()
	var s doctorServers
	s.plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/key/"):
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	s.secure = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	s.slow = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	// Another app's web page, which answers every path.
	s.page = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body>Some other app</body></html>"))
	}))
	// Another app (here Sonarr) that turns down the key on every path.
	s.other = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("<html><head><title>Sonarr</title></head></html>"))
	}))
	// nginx's answer to plain http on its https port.
	s.nginx = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "nginx")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<center>The plain HTTP request was sent to HTTPS port</center>"))
	}))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.closed = "http://" + l.Addr().String()
	l.Close()
	// A port that takes the connection and closes it without a word, as
	// qBittorrent does to an https:// call on its plain http port.
	h, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := h.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	s.hangUp = h.Addr().String()
	t.Cleanup(func() { h.Close() })
	// A plain http port that hangs up on a TLS hello without a word but
	// answers plain http, the way qBittorrent's web UI does.
	pk, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := pk.Accept()
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
	s.picky = pk.Addr().String()
	t.Cleanup(func() { pk.Close() })
	t.Cleanup(func() {
		s.plain.Close()
		s.secure.Close()
		s.slow.Close()
		s.page.Close()
		s.other.Close()
		s.nginx.Close()
	})
	return s
}

func TestDownloaderTestNamesEachCause(t *testing.T) {
	s := startDoctorServers(t)
	https := func(u string) string { return strings.Replace(u, "https://", "http://", 1) }
	cases := []struct {
		name  string
		base  string
		local bool
		quick bool
		want  doctor.Cause
	}{
		{"name not found", "http://nas.invalid:8080", true, false, doctor.DNS},
		{"refused", s.closed, true, false, doctor.Refused},
		{"timeout", s.slow.URL, true, true, doctor.Timeout},
		{"self-signed certificate", s.secure.URL, true, false, doctor.TLS},
		{"https on an http port", strings.Replace(s.plain.URL, "http://", "https://", 1), true, false, doctor.UseHTTP},
		{"http on an https port", https(s.secure.URL), true, false, doctor.UseHTTPS},
		{"key refused", s.plain.URL + "/key", true, false, doctor.Auth},
		{"wrong path", s.plain.URL + "/nothing", true, false, doctor.NotFound},
		{"another app", s.page.URL, true, false, doctor.WrongApp},
		{"another app turns down the key", s.other.URL, true, false, doctor.WrongApp},
		{"nginx: plain http on its https port", s.nginx.URL, true, false, doctor.UseHTTPS},
		{"https on a plain port that drops TLS", "https://" + s.picky, true, false, doctor.UseHTTP},
		{"https on a port that hangs up", "https://" + s.hangUp, true, false, doctor.Unknown},
		{"http on a port that hangs up", "http://" + s.hangUp, true, false, doctor.Unknown},
		{"local network policy", s.plain.URL, false, false, doctor.Blocked},
	}
	for _, adapter := range []string{"qbittorrent", "sabnzbd"} {
		for _, tc := range cases {
			t.Run(adapter+"/"+tc.name, func(t *testing.T) {
				h := Client(tc.base, tc.local)
				if tc.quick {
					h.Timeout = 300 * time.Millisecond
				}
				var err error
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if adapter == "qbittorrent" {
					_, err = NewQbittorrentKey(tc.base, "key", h).Health(ctx)
				} else {
					_, err = NewSabnzbd(tc.base, sabKey, h).Health(ctx)
				}
				if err == nil {
					t.Fatal("the test passed")
				}
				cause, text := explainDownloader(err, tc.base, true)
				if cause != tc.want {
					t.Fatalf("cause %q, want %q (error %v)", cause, tc.want, err)
				}
				if tc.want != doctor.Unknown && (adapter == "sabnzbd" || tc.want != doctor.Auth) {
					if text != doctor.Line(tc.want) {
						t.Fatalf("text %q", text)
					}
				}
				// The stored last error reads the same on the health row.
				d := downloaderWithError(err, tc.base)
				if strings.Contains(tc.name, "hangs up") || strings.Contains(tc.name, "drops TLS") {
					// A hang-up in the background is as likely a restart as a
					// wrong port, so the row does not blame the setting.
					if hv := HealthOf(d, time.Now()); hv.Cause != "" {
						t.Fatalf("health blames the setting: %+v", hv)
					}
					return
				}
				if hv := HealthOf(d, time.Now()); hv.Text != doctor.Line(tc.want) || hv.Cause != string(tc.want) {
					t.Fatalf("health %+v (last error %q)", hv, d.LastError)
				}
			})
		}
	}
}

func TestDownloaderTestNamesMissingCategory(t *testing.T) {
	cause, text := explainDownloader(fmt.Errorf("%w (tv)", ErrNoCategory), "http://sab:8080", false)
	if cause != doctor.Category || !strings.Contains(text, "tv") || doctor.OfText(text) != doctor.Category {
		t.Fatalf("%q %q", cause, text)
	}
}

func TestDownloaderUnknownErrorSaysTheLogHasIt(t *testing.T) {
	cause, text := explainDownloader(fmt.Errorf("qbittorrent app/version: 502 Bad Gateway"), "http://qbit:8080", false)
	if cause != doctor.Unknown || strings.Contains(text, "502") || !strings.Contains(text, "log") {
		t.Fatalf("%q %q", cause, text)
	}
}

// downloaderWithError is a Downloader whose last check failed with err, as
// downloaderFailed stores it.
func downloaderWithError(err error, rawURL string) domain.Downloader {
	return domain.Downloader{State: "unreachable", URL: rawURL, LastError: firstLine(err.Error())}
}
