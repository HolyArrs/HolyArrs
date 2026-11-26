package downloads

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// diesThenAnswers is a transport whose first round trip fails the way a
// pooled connection that qBittorrent closed does: EOF, with no answer read.
// Later trips go to the real transport. Counting the trips is how the tests
// tell "the client retried once" from "the transport retried for us".
type diesThenAnswers struct {
	trips int32
	rt    http.RoundTripper
}

func (d *diesThenAnswers) RoundTrip(r *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&d.trips, 1) == 1 {
		return nil, io.EOF
	}
	return d.rt.RoundTrip(r)
}

// diesAlways fails every round trip, for the downloader that is really down.
type diesAlways struct{ trips int32 }

func (d *diesAlways) RoundTrip(r *http.Request) (*http.Response, error) {
	atomic.AddInt32(&d.trips, 1)
	return nil, io.EOF
}

// A Downloader that closes one idle connection must not be reported as
// unreachable: the health check reads app/version again and succeeds.
func TestHealthRetriesOnceAfterADeadConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/version" {
			t.Errorf("asked for %q, want app/version", r.URL.Path)
		}
		_, _ = io.WriteString(w, "v5.0.0")
	}))
	defer srv.Close()

	tr := &diesThenAnswers{rt: http.DefaultTransport}
	q := NewQbittorrentKey(srv.URL, "a-key", &http.Client{Transport: tr})

	h, err := q.Health(context.Background())
	if err != nil {
		t.Fatalf("Health after one dead connection: %v, want no error", err)
	}
	if h.Version != "v5.0.0" {
		t.Errorf("version %q, want v5.0.0", h.Version)
	}
	if got := atomic.LoadInt32(&tr.trips); got != 2 {
		t.Errorf("round trips %d, want 2 (the dead one and the one that worked)", got)
	}
}

// A POST must never be retried: torrents/add sent twice would add the torrent
// twice, so a dead connection there has to stay an error.
func TestAPostIsNotRetriedAfterADeadConnection(t *testing.T) {
	tr := &diesAlways{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("%s %s reached the server, want nothing to arrive", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	q := NewQbittorrentKey(srv.URL, "a-key", &http.Client{Transport: tr})

	if _, err := q.call(context.Background(), http.MethodPost, "torrents/add",
		url.Values{"urls": {"magnet:?xt=urn:btih:abc"}}); err == nil {
		t.Fatal("POST to a dead connection: no error, want the EOF passed through")
	}
	if got := atomic.LoadInt32(&tr.trips); got != 1 {
		t.Errorf("round trips %d, want 1 (a POST is never sent twice)", got)
	}
}

// One retry, not a loop: a Downloader that is genuinely down still fails, and
// it fails on the second try so a broken client cannot pin the app.
func TestADownDownloaderStillFailsAfterOneRetry(t *testing.T) {
	tr := &diesAlways{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	q := NewQbittorrentKey(srv.URL, "a-key", &http.Client{Transport: tr})

	_, err := q.Health(context.Background())
	if err == nil {
		t.Fatal("Health against a downloader that never answers: no error")
	}
	if !strings.Contains(err.Error(), "qbittorrent app/version") {
		t.Errorf("error %q, want it to name app/version", err)
	}
	if got := atomic.LoadInt32(&tr.trips); got != 2 {
		t.Errorf("round trips %d, want exactly 2", got)
	}
}

// A cancelled context is the caller giving up, not a dead connection, so it
// must not be sent again.
func TestACancelledContextIsNotRetried(t *testing.T) {
	tr := &diesAlways{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	q := NewQbittorrentKey(srv.URL, "a-key", &http.Client{Transport: tr})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := q.Health(ctx); err == nil {
		t.Fatal("Health with a cancelled context: no error")
	}
	if got := atomic.LoadInt32(&tr.trips); got > 1 {
		t.Errorf("round trips %d, want at most 1 when the caller already gave up", got)
	}
}
