package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// cookieSite answers the .torrent only to a request that carries the cookie.
func cookieSite(t *testing.T, host string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "uid=1; pass=secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write(grabPayload)
	}))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{host: hostPort(t, srv)}, true)
}

func cookieHeader(context.Context, domain.Source) (http.Header, error) {
	return http.Header{"Cookie": {"uid=1; pass=secret"}}, nil
}

// A site that hands out .torrent files only with the sign-in cookie: HolyArrs
// fetches the file with the cookie and sends the file, never the cookie.
func TestGrabFetchesACookieSiteFileItself(t *testing.T) {
	cookieSite(t, "ipt.example")
	w, m := grabWorld(t, "http://ipt.example/download.php/1/x.torrent", nil)
	w.exec(t, "UPDATE sources SET url = ?", "http://ipt.example")
	m.SourceHeader = cookieHeader
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["torrent"] != string(grabPayload) || w.fake.added[0]["urls"] != "" {
		t.Fatalf("added %+v, want the file", w.fake.added)
	}
}

// The cookie goes only to the Source's own site, not to a link elsewhere.
func TestCookieStaysOnTheSourceSite(t *testing.T) {
	cookieSite(t, "other.example")
	if _, _, err := fetchGrabLinkHeader(context.Background(), "http://other.example/x.torrent", addFetchMax, nil, "", sourceHeaderFor("http://ipt.example", "http://other.example/x.torrent", http.Header{"Cookie": {"uid=1; pass=secret"}})); err == nil {
		t.Fatal("the cookie reached another site")
	}
	if b, _, err := fetchGrabLinkHeader(context.Background(), "http://other.example/x.torrent", addFetchMax, nil, "", sourceHeaderFor("http://other.example", "http://other.example/x.torrent", http.Header{"Cookie": {"uid=1; pass=secret"}})); err != nil || string(b) != string(grabPayload) {
		t.Fatalf("own site: %q %v", b, err)
	}
}

// Transmission and the other torrent Downloaders get the file too.
func TestPrefetchSendsTheCookie(t *testing.T) {
	cookieSite(t, "ipt.example")
	if _, file := prefetchTorrentHeader(context.Background(), "http://ipt.example/x.torrent", false, "", http.Header{"Cookie": {"uid=1; pass=secret"}}); string(file) != string(grabPayload) {
		t.Fatalf("file %q", file)
	}
}
