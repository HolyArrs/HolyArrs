package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// botWall answers like a site behind Cloudflare: Go's default User-Agent (or
// none) gets 403, anything else gets the file.
func botWall(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		*seen = append(*seen, ua)
		if ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write(grabPayload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The .nzb or .torrent fetch HolyArrs makes for NZBGet, Transmission, Deluge
// and qBittorrent links carries the HolyArrs User-Agent, like searches do.
func TestReleaseFetchSendsHolyArrsAgent(t *testing.T) {
	var seen []string
	srv := botWall(t, &seen)
	b, err := fetchRelease(context.Background(), srv.URL+"/getnzb/1.nzb", true, addFetchMax)
	if err != nil || string(b) != string(grabPayload) {
		t.Fatalf("fetch behind a bot wall: %q %v (User-Agents %q)", b, err, seen)
	}
	if len(seen) != 1 || seen[0] != sources.UserAgent {
		t.Fatalf("User-Agents %q, want [%q]", seen, sources.UserAgent)
	}
}

// The grab-time fetch without a Source proxy (qBittorrent and SABnzbd off the
// home network, and the torrent read for Transmission and Deluge) carries it too.
func TestGrabFetchWithoutProxySendsHolyArrsAgent(t *testing.T) {
	var seen []string
	srv := botWall(t, &seen)
	fakeDNS(t, map[string]string{"nzbsite.example": hostPort(t, srv)}, true)
	b, m, err := fetchGrabLink(context.Background(), "http://nzbsite.example/getnzb/1.nzb", addFetchMax, nil, "")
	if err != nil || string(b) != string(grabPayload) || m != "" {
		t.Fatalf("grab fetch behind a bot wall: %q %q %v (User-Agents %q)", b, m, err, seen)
	}
	if link, file := prefetchTorrent(context.Background(), "http://nzbsite.example/t/1.torrent", false, ""); string(file) != string(grabPayload) {
		t.Fatalf("torrent read behind a bot wall: %q %q (User-Agents %q)", link, file, seen)
	}
	for _, ua := range seen {
		if ua != sources.UserAgent {
			t.Fatalf("User-Agents %q, want only %q", seen, sources.UserAgent)
		}
	}
}
