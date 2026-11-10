package downloads

import (
	"bytes"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func prefetchServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	fakeDNS(t, map[string]string{"indexer.example": hostPort(t, srv)}, true)
}

func TestPrefetchMagnetRedirectReachesTransmission(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:" + hash + "&dn=lab"
	prefetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", magnet)
		w.WriteHeader(http.StatusMovedPermanently)
	})
	link, file := prefetchTorrent(context.Background(), "http://indexer.example/1/download?apikey=k&link=x", false, "")
	if file != nil || linkHash(link) != hash {
		t.Fatalf("got link %q, file %d bytes", link, len(file))
	}
	c, f := newTransmissionHarness(t, true)
	r := c.release
	r.URL, r.InfoHash = link, linkHash(link)
	if _, err := c.engine.Add(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.lastAdd["filename"].(string); linkHash(got) != hash {
		t.Fatalf("transmission got filename %q", got)
	}
}

func TestPrefetchTorrentFileReachesTransmission(t *testing.T) {
	prefetchServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(grabPayload) })
	link, file := prefetchTorrent(context.Background(), "http://indexer.example/x.torrent", false, "")
	if !bytes.Equal(file, grabPayload) || link != "http://indexer.example/x.torrent" {
		t.Fatalf("got link %q, file %q", link, file)
	}
	c, f := newTransmissionHarness(t, true)
	r := c.release
	r.URL, r.InfoHash, r.File = "http://127.0.0.1:1/must-not-fetch.torrent", "", file
	_, _ = c.engine.Add(context.Background(), r) // the fake answers with its own hash
	if got, _ := f.lastAdd["metainfo"].(string); got != base64.StdEncoding.EncodeToString(grabPayload) {
		t.Fatalf("transmission got metainfo %q", got)
	}
}

func TestPrefetchFailureKeepsLink(t *testing.T) {
	prefetchServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	const raw = "http://indexer.example/x.torrent"
	if link, file := prefetchTorrent(context.Background(), raw, false, ""); link != raw || file != nil {
		t.Fatalf("got link %q, file %d bytes", link, len(file))
	}
}

func TestGrabHomeSourceSendsTheMagnetToQbittorrent(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:" + hash + "&dn=lab"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", magnet)
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(hostPort(t, srv))
	fakeDNS(t, map[string]string{"prowlarr.example": hostPort(t, srv)}, false)
	w, m := setup(t)
	base := "http://prowlarr.example:" + port
	w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", base+"/1/api")
	w.exec(t, "UPDATE releases SET link_ref = ? WHERE id = ?", base+"/1/download?apikey=k&link=x", w.relID)
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || linkHash(w.fake.added[0]["urls"]) != hash {
		t.Fatalf("added %+v, want the magnet", w.fake.added)
	}
}

func TestGrabRefusedAddLeavesNoSentDownload(t *testing.T) {
	w, m := setup(t)
	w.fake.addFails = true
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v, want the refusal", err)
	}
	var n int
	if err := w.s.Read().QueryRow("SELECT COUNT(*) FROM downloads WHERE state = 'sent'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("sent downloads = %d, %v", n, err)
	}
}
