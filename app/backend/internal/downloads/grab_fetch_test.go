package downloads

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var grabPayload = []byte("d4:infod6:lengthi1e4:name7:lab.mkvee")

func TestGrabFetchRefusesNamesThatLeadHome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(grabPayload) }))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(hostPort(t, srv))
	fakeDNS(t, map[string]string{"loop.example": "127.0.0.1:" + port, "nas.example": "10.20.30.40:" + port, "router.example": "192.168.1.1:" + port}, false)
	for _, name := range []string{"loop.example", "nas.example", "router.example"} {
		if _, _, err := fetchGrabLink(context.Background(), "http://"+name+"/x.torrent", addFetchMax, nil, ""); !errors.Is(err, errLinkLocal) {
			t.Errorf("%s: err = %v, want the local-address refusal", name, err)
		}
	}
}

func TestGrabFetchRefusesRedirectsHome(t *testing.T) {
	inside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(grabPayload) }))
	t.Cleanup(inside.Close)
	_, port, _ := net.SplitHostPort(hostPort(t, inside))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-ip":
			http.Redirect(w, r, "http://10.0.0.7:"+port+"/x.torrent", http.StatusFound)
		case "/to-name":
			http.Redirect(w, r, "http://rebind.example:"+port+"/x.torrent", http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"public.example": hostPort(t, srv), "rebind.example": "127.0.0.2:" + port}, true)
	for _, p := range []string{"/to-ip", "/to-name"} {
		if _, _, err := fetchGrabLink(context.Background(), "http://public.example"+p, addFetchMax, nil, ""); !errors.Is(err, errLinkLocal) {
			t.Errorf("%s: err = %v, want the local-address refusal", p, err)
		}
	}
}

func TestGrabFetchPublicNameAndMagnetRedirect(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=lab"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file":
			_, _ = w.Write(grabPayload)
		case "/moved":
			http.Redirect(w, r, "/file", http.StatusFound)
		case "/magnet":
			w.Header().Set("Location", magnet)
			w.WriteHeader(http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"public.example": hostPort(t, srv)}, true)
	for _, p := range []string{"/file", "/moved"} {
		b, m, err := fetchGrabLink(context.Background(), "http://public.example"+p, addFetchMax, nil, "")
		if err != nil || string(b) != string(grabPayload) || m != "" {
			t.Errorf("%s: %q %q %v", p, b, m, err)
		}
	}
	if b, m, err := fetchGrabLink(context.Background(), "http://public.example/magnet", addFetchMax, nil, ""); err != nil || b != nil || m != magnet {
		t.Errorf("magnet redirect: %q %q %v", b, m, err)
	}
}

func TestGrabHandsQbittorrentTheFileNotTheAddress(t *testing.T) {
	h, err := torrentFileHash(grabPayload)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(grabPayload) }))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"public.example": hostPort(t, srv)}, true)
	w, m := grabWorld(t, "http://public.example/x.torrent?apikey=k", nil)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["urls"] != "" || w.fake.added[0]["torrent"] != string(grabPayload) || w.fake.added[0]["category"] != "holyarrs" {
		t.Fatalf("added %+v, want the file and no address", w.fake.added)
	}
	if d := w.download(t, id); d.ExternalID != h {
		t.Fatalf("external id %q, want the file's hash %q", d.ExternalID, h)
	}
}

func TestGrabRefusesALinkThatLeadsHome(t *testing.T) {
	fakeDNS(t, map[string]string{"nas.example": "10.1.2.3:80"}, false)
	w, m := grabWorld(t, "http://nas.example/x.torrent", nil)
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || !strings.Contains(err.Error(), "local or blocked") {
		t.Fatalf("err = %v, want the local-address refusal", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("qBittorrent got %+v", w.fake.added)
	}
}

func TestGrabLocalSourceStillHandsOverTheAddress(t *testing.T) {
	w, m := setup(t)
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["urls"] != "http://fake-indexer/dune-part-two.torrent" {
		t.Fatalf("added %+v", w.fake.added)
	}
}

func TestSabnzbdAddUploadsTheFile(t *testing.T) {
	var mode, nzb, name, cat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("mode") {
		case "get_cats":
			_, _ = w.Write([]byte(`{"categories":["*","movies"]}`))
		case "queue", "history":
			_, _ = w.Write([]byte(`{"queue":{"slots":[]},"history":{"slots":[]}}`))
		case "addfile", "addurl":
			mode, name, cat = q.Get("mode"), q.Get("nzbname"), q.Get("cat")
			if f, _, err := r.FormFile("name"); err == nil {
				b, _ := io.ReadAll(f)
				nzb = string(b)
			}
			_, _ = w.Write([]byte(`{"status":true,"nzo_ids":["SABnzbd_nzo_1"]}`))
		}
	}))
	t.Cleanup(srv.Close)
	s := NewSabnzbd(srv.URL, "key", nil)
	id, err := s.Add(context.Background(), AddRequest{URL: "http://indexer.example/get?id=1", File: []byte("<nzb/>"), Name: "Lab.Movie.2024", Category: "movies"})
	if err != nil || id != "SABnzbd_nzo_1" {
		t.Fatalf("add = %q, %v", id, err)
	}
	if mode != "addfile" || nzb != "<nzb/>" || name != "Lab.Movie.2024" || cat != "movies" {
		t.Fatalf("mode %q nzb %q name %q cat %q", mode, nzb, name, cat)
	}
}

func TestGrabProxySourceUsesItsOwnTransport(t *testing.T) {
	publicFileServer(t, grabPayload)
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE sources SET proxy_url = 'socks5://proxy.example:1080'")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || len(w.fake.added) != 0 {
		t.Fatalf("err %v, added %+v: want a refusal and nothing sent", err, w.fake.added)
	}
	used := 0
	m.SourceTransport = func(context.Context, domain.Source) (http.RoundTripper, error) {
		used++
		d := outbound.PolicyDialer(grabAllow)
		return &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, network, grabDial(addr))
		}}, nil
	}
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if used != 1 || len(w.fake.added) != 1 || w.fake.added[0]["torrent"] != string(grabPayload) || w.fake.added[0]["urls"] != "" {
		t.Fatalf("used %d, added %+v", used, w.fake.added)
	}
}

func TestGrabTorrentWithTrailingNewline(t *testing.T) {
	publicFileServer(t, append(append([]byte{}, grabPayload...), "\r\n"...))
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["torrent"] != string(grabPayload) {
		t.Fatalf("added %+v", w.fake.added)
	}
}

func TestGrabNonTorrentAnswerIsTriedAgain(t *testing.T) {
	publicFileServer(t, []byte("<html>download limit reached</html>"))
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	_, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	var fe interface{ Final() bool }
	if err == nil || (errors.As(err, &fe) && fe.Final()) {
		t.Fatalf("err = %v, want a retryable error", err)
	}
}

func TestGrabAdoptsATorrentQbittorrentAlreadyHas(t *testing.T) {
	h, _ := torrentFileHash(grabPayload)
	publicFileServer(t, grabPayload)
	w, m := grabWorld(t, "http://public.example/x.torrent", nil)
	w.fake.set(h, map[string]any{"name": "lab.mkv", "category": "holyarrs", "state": "downloading", "progress": 0.1})
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("added again: %+v", w.fake.added)
	}
	if d := w.download(t, id); d.ExternalID != h {
		t.Fatalf("external id %q, want %q", d.ExternalID, h)
	}
}

func fakeDNS(t *testing.T, names map[string]string, publicHere bool) {
	t.Helper()
	oldDial, oldAllow := grabDial, grabAllow
	grabDial = func(addr string) string {
		host, _, _ := net.SplitHostPort(addr)
		if to, ok := names[host]; ok {
			return to
		}
		return addr
	}
	if publicHere {
		grabAllow = func(ip net.IP) bool { return ip.Equal(net.IPv4(127, 0, 0, 1)) || outbound.PublicOnly(ip) }
	}
	t.Cleanup(func() { grabDial, grabAllow = oldDial, oldAllow })
}

func hostPort(t *testing.T, s *httptest.Server) string {
	t.Helper()
	u, _ := url.Parse(s.URL)
	return u.Host
}

func grabWorld(t *testing.T, link string, infoHash *string) (*world, *Manager) {
	t.Helper()
	w, m := setup(t)
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE sources SET allow_local_network = 0"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE releases SET link_ref = ?, info_hash = ? WHERE id = ?", link, infoHash, w.relID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, m
}

func publicFileServer(t *testing.T, body []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"public.example": hostPort(t, srv)}, true)
}
