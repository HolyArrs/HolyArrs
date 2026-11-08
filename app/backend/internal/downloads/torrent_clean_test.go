package downloads

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCleanTorrentDropsHomeAddresses(t *testing.T) {
	in := homeTorrent()
	want, err := torrentFileHash(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := safeTorrent(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := torrentFileHash(out); err != nil || got != want {
		t.Fatalf("hash %q %v, want %q", got, err, want)
	}
	s := string(out)
	for _, gone := range []string{"10.0.0.1", "192.168.1.1", "localhost", "nas.local", "2130706433", "3:::1", "192.168.7.7", "10.9.9.9"} {
		if strings.Contains(s, gone) {
			t.Errorf("%s left in %q", gone, s)
		}
	}
	for _, kept := range []string{"tracker.example", "seed.example", "router.example", "[2001:4860::1]", "lab.mkv"} {
		if !strings.Contains(s, kept) {
			t.Errorf("%s dropped from %q", kept, s)
		}
	}
	if _, _, err := bdecode(out, 0, 0); err != nil || strings.Contains(s, bs("announce")+bs("http")) {
		t.Fatalf("bad cleaned file %q %v", s, err)
	}
}

func TestCleanTorrentLeavesAPublicFileAsItIs(t *testing.T) {
	out, err := safeTorrent(grabPayload)
	if err != nil || !bytes.Equal(out, grabPayload) {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := safeTorrent([]byte("<html>")); err == nil {
		t.Fatal("non-torrent passed")
	}
}

func TestGrabUploadsTheCleanedTorrent(t *testing.T) {
	in := homeTorrent()
	h, _ := torrentFileHash(in)
	publicFileServer(t, in)
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || strings.Contains(w.fake.added[0]["torrent"], "10.0.0.1") || !strings.Contains(w.fake.added[0]["torrent"], "seed.example") {
		t.Fatalf("added %+v", w.fake.added)
	}
	if d := w.download(t, id); d.ExternalID != h {
		t.Fatalf("external id %q, want %q", d.ExternalID, h)
	}
}

func TestGrabRefusalOfAHomeLinkIsFinal(t *testing.T) {
	fakeDNS(t, map[string]string{"nas.example": "10.1.2.3:80"}, false)
	w, m := grabWorld(t, "http://nas.example/x.torrent", nil)
	_, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	var fe interface{ Final() bool }
	if err == nil || !errors.As(err, &fe) || !fe.Final() || !strings.Contains(err.Error(), "May reach the local network") {
		t.Fatalf("err = %v, want a final refusal naming the switch", err)
	}
}

func TestTrackerWithAnOddAuthorityIsRefused(t *testing.T) {
	for _, v := range []string{"http://x.example.com:80?@192.168.1.1/announce", "udp://x.example.com:6969#@10.0.0.1", `http://a.example\@10.0.0.1/`} {
		if trackerAllowed(v, false) {
			t.Errorf("tracker %s allowed", v)
		}
	}
	if !trackerAllowed("https://tracker.example/announce?passkey=abc", false) || !trackerAllowed("udp://tracker.example:1337", false) {
		t.Error("plain public trackers refused")
	}
}

func TestGrabProxyRefusalIsTriedAgain(t *testing.T) {
	publicFileServer(t, grabPayload)
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE sources SET proxy_url = 'socks5://proxy.example:1080'")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m.SourceTransport = func(context.Context, domain.Source) (http.RoundTripper, error) {
		return &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: outbound.ErrAddressRefused}
		}}, nil
	}
	_, err = m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	var fe interface{ Final() bool }
	if err == nil || (errors.As(err, &fe) && fe.Final()) {
		t.Fatalf("err = %v, want a retryable error", err)
	}
}

func TestHomeHostOddBytesAndDots(t *testing.T) {
	for _, h := range []string{"localhost\x00", "nas.local\x00.example.com", "metadata.google.internal\x00", "tracker.example\x7f", "a b.example", "nas。local", "router．local", "127．0．0．1"} {
		if !homeHost(h) {
			t.Errorf("homeHost(%q) = false", h)
		}
	}
	if homeHost("tracker.example.com") || homeHost("router.example") {
		t.Error("public names refused")
	}
	in := []byte("d" + bs("info") + "d" + bs("length") + "i1e" + bs("name") + bs("lab.mkv") + bs("piece length") + "i16384e" + bs("pieces") + bs(strings.Repeat("a", 20)) + "e" +
		bs("nodes") + "l" + "l" + bs("localhost\x00") + "i6881ee" + "l" + bs("nas.local\x00") + "i6881ee" + "l" + bs("dht.example") + "i6881ee" + "e" + "e")
	out, err := safeTorrent(in)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); strings.Contains(s, "localhost") || strings.Contains(s, "nas.local") || !strings.Contains(s, "dht.example") {
		t.Fatalf("cleaned %q", s)
	}
}

func TestFetchReleaseRefusalIsFinal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(grabPayload) }))
	t.Cleanup(srv.Close)
	_, err := fetchRelease(context.Background(), srv.URL+"/x.torrent", false, addFetchMax)
	var fe interface{ Final() bool }
	if !errors.Is(err, errLinkLocal) || !errors.As(err, &fe) || !fe.Final() {
		t.Fatalf("err = %v, want the final local-address refusal", err)
	}
}

func bs(s string) string { return strconv.Itoa(len(s)) + ":" + s }

func homeTorrent() []byte {
	return []byte("d" +
		bs("announce") + bs("http://10.0.0.1/announce") +
		bs("announce-list") + "l" +
		"l" + bs("http://192.168.1.1/a") + bs("https://tracker.example/a") + "e" +
		"l" + bs("udp://localhost:1/x") + "e" +
		"l" + bs("http://[2001:4860::1]/a") + bs("http://2130706433/a") + "e" +
		"l" + bs("http://x.example.com:80?@192.168.7.7/announce") + bs("udp://x.example.com:6969#@10.9.9.9") + "e" +
		"e" +
		bs("httpseeds") + "l" + bs("http://nas.local/x") + "e" +
		bs("info") + "d" + bs("length") + "i1e" + bs("name") + bs("lab.mkv") + bs("piece length") + "i16384e" + bs("pieces") + bs(strings.Repeat("a", 20)) + "e" +
		bs("nodes") + "l" + "l" + bs("192.168.1.1") + "i6881ee" + "l" + bs("router.example") + "i6881ee" + "l" + bs("::1") + "i1ee" + "e" +
		bs("url-list") + "l" + bs("http://10.0.0.1/cgi-bin/x/") + bs("https://seed.example/x/") + "e" +
		"e")
}
