package downloads

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A Prowlarr NZB link answers with a redirect to the indexer on another host.
// NZBGet's send follows it like the SABnzbd and qBittorrent paths do, with the
// same address, scheme and redirect checks.
func TestNZBGetFollowsARedirectToTheIndexer(t *testing.T) {
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<?xml version=\"1.0\"?><nzb></nzb>"))
	}))
	t.Cleanup(indexer.Close)
	_, port, _ := net.SplitHostPort(hostPort(t, indexer))
	prowlarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/indexer":
			http.Redirect(w, r, "http://indexer.example:"+port+"/getnzb/1.nzb", http.StatusMovedPermanently)
		case "/home":
			http.Redirect(w, r, "http://10.0.0.7:"+port+"/getnzb/1.nzb", http.StatusMovedPermanently)
		case "/rebind":
			http.Redirect(w, r, "http://rebind.example:"+port+"/getnzb/1.nzb", http.StatusMovedPermanently)
		case "/file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusMovedPermanently)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	}))
	t.Cleanup(prowlarr.Close)
	fakeDNS(t, map[string]string{"indexer.example": hostPort(t, indexer), "rebind.example": "10.0.0.8:" + port}, true)

	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), ownSource(AddRequest{URL: prowlarr.URL + "/indexer", Name: "Redirected", Category: "tv",
		OperationKey: "redirected", AllowLocal: true}))
	if err != nil || id == "" || f.adds != 1 {
		t.Fatalf("redirect to the indexer: id=%q adds=%d err=%v", id, f.adds, err)
	}
	for _, p := range []string{"/home", "/rebind", "/file", "/loop"} {
		_, err := n.Add(context.Background(), ownSource(AddRequest{URL: prowlarr.URL + p, Name: "Refused" + p[1:], Category: "tv",
			OperationKey: "refused" + p, AllowLocal: true}))
		if err == nil {
			t.Errorf("%s: a redirect that must be refused was followed", p)
		}
	}
	if f.adds != 1 {
		t.Fatalf("refused redirects reached NZBGet: adds=%d", f.adds)
	}
}

// The size limit still holds after a redirect.
func TestNZBGetRedirectKeepsTheSizeLimit(t *testing.T) {
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.CopyN(w, zeroReader{}, nzbgetMaxNZB+1)
	}))
	t.Cleanup(indexer.Close)
	prowlarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://indexer.example/getnzb/1.nzb", http.StatusMovedPermanently)
	}))
	t.Cleanup(prowlarr.Close)
	fakeDNS(t, map[string]string{"indexer.example": hostPort(t, indexer)}, true)
	f, base := startNZBGet(t)
	if _, err := NewNZBGet(base, f.user, f.pass, nil).Add(context.Background(), ownSource(AddRequest{URL: prowlarr.URL + "/x", Name: "Big",
		Category: "tv", OperationKey: "big", AllowLocal: true})); err == nil || f.adds != 0 {
		t.Fatalf("oversized file: adds=%d err=%v", f.adds, err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
