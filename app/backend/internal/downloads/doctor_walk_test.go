package downloads

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// serveTCP answers each connection on a fresh 127.0.0.1 port with h.
func serveTCP(t *testing.T, h func(c net.Conn)) string {
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
			go func() { defer c.Close(); h(c) }()
		}
	}()
	return l.Addr().String()
}

// checkCause runs the background check on a Downloader at rawURL and
// returns the cause health shows afterwards.
func checkCause(t *testing.T, rawURL string) (doctor.Cause, string) {
	t.Helper()
	w, m := setup(t)
	ctx := context.Background()
	d := &domain.Downloader{Name: "Walk", Adapter: "qbittorrent", Protocol: "torrent", URL: rawURL, Priority: 2, AllowLocal: true}
	w.addDownloader(t, d)
	if _, err := m.Check(ctx, *d); err == nil {
		t.Fatal("check passed")
	}
	var got domain.Downloader
	if err := database.Get(ctx, w.s.Read(), "downloaders", d.ID, &got); err != nil {
		t.Fatal(err)
	}
	hv := HealthOf(got, time.Now())
	return doctor.Cause(hv.Cause), got.LastError
}

// An https:// address on qBittorrent's plain port, which waits on a TLS hello
// without a word: the background check names http too, and soon.
func TestBackgroundCheckNamesHTTPOnASilentPort(t *testing.T) {
	old := tlsWait
	tlsWait = 300 * time.Millisecond
	t.Cleanup(func() { tlsWait = old })
	addr := serveTCP(t, func(c net.Conn) {
		b := make([]byte, 1)
		if _, err := c.Read(b); err != nil {
			return
		}
		if b[0] == 0x16 {
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = io.Copy(io.Discard, c)
			return
		}
		_, _ = c.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
	})
	start := time.Now()
	c, last := checkCause(t, "https://"+addr)
	if c != doctor.UseHTTP {
		t.Fatalf("cause %q, last error %q", c, last)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("took %s", took)
	}
}

// An http:// address on a TLS-only port that hangs up on plain words: the
// background check names https, as Test does.
func TestBackgroundCheckNamesHTTPSOnATLSPort(t *testing.T) {
	sec := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(sec.Close)
	addr := serveTCP(t, func(c net.Conn) {
		b := make([]byte, 1)
		if _, err := c.Read(b); err != nil || b[0] != 0x16 {
			return
		}
		up, err := net.Dial("tcp", sec.Listener.Addr().String())
		if err != nil {
			return
		}
		defer up.Close()
		_, _ = up.Write(b)
		go func() { _, _ = io.Copy(up, c) }()
		_, _ = io.Copy(c, up)
	})
	if c, last := checkCause(t, "http://"+addr); c != doctor.UseHTTPS {
		t.Fatalf("cause %q, last error %q", c, last)
	}
}

// qBittorrent makes a category on the first add, so a missing one passes.
func TestQbitCheckPassesAMissingCategory(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	w.fake.listCats = true
	w.fake.cats = []string{"movies"}
	var d domain.Downloader
	if err := database.Get(ctx, w.s.Read(), "downloaders", 1, &d); err != nil {
		t.Fatal(err)
	}
	d.Category = "tv-missing"
	if _, err := m.Check(ctx, d); err != nil {
		t.Fatalf("check without the category: %v", err)
	}
	d.Category = "movies"
	if _, err := m.Check(ctx, d); err != nil {
		t.Fatalf("check with the category: %v", err)
	}
}

// Another HolyArrs on the port turns down the key with its own plain answer;
// that is another app, not a wrong key.
func TestQbitNamesAnotherHolyArrs(t *testing.T) {
	srv := httptest.NewServer(webserver.SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webserver.Fail(w, http.StatusUnauthorized, "sign in first")
	})))
	t.Cleanup(srv.Close)
	_, err := NewQbittorrent(srv.URL, nil).Health(context.Background())
	if doctor.Of(err) != doctor.WrongApp {
		t.Fatalf("cause %q (%v)", doctor.Of(err), err)
	}
}
