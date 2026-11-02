package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
)

type fakeArt struct{}

func (fakeArt) Art(_ context.Context, kind, id string) ([]byte, string, error) {
	if id == "404" {
		return nil, "miss", extras.ErrNotFound
	}
	return []byte(`{"kind":"` + kind + `","id":"` + id + `","logo":"https://a/l.png","source":"fanart.tv"}`), "miss", nil
}
func (fakeArt) Status() extras.Status { return extras.Status{State: extras.StateOK} }

func TestExtrasRoutes(t *testing.T) {
	s, _ := registering(Options{Registration: true, Burst: 100, Fanart: fakeArt{}, TrustedProxies: testProxy()})
	if w := req(s, "/v1/art/movie/603", ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	if w := req(s, "/v1/art/movie/603", "synthetic-test-token"); w.Code != 200 || !strings.Contains(w.Body.String(), "l.png") {
		t.Fatalf("art %d %s", w.Code, w.Body)
	}
	tok := registerToken(t, s, "203.0.113.41")
	if w := reqFrom(s, "/v1/art/music/5b11f4ce-a62d-471e-81fc-a69a8278c7da", tok, "203.0.113.41"); w.Code != 200 {
		t.Fatalf("registered %d", w.Code)
	}
	if w := reqFrom(s, "/v1/art/artist/5b11f4ce-a62d-471e-81fc-a69a8278c7da", tok, "203.0.113.41"); w.Code != 200 {
		t.Fatalf("artist alias %d", w.Code)
	}
	if w := req(s, "/v1/art/tv/404", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("none %d", w.Code)
	}
	// Ratings has no key here: off.
	if w := req(s, "/v1/ratings/movie/603", "synthetic-test-token"); w.Code != 404 || !strings.Contains(w.Body.String(), "source_disabled") {
		t.Fatalf("disabled %d %s", w.Code, w.Body)
	}
	for _, q := range []string{"/v1/art/show/1", "/v1/art/movie/01", "/v1/art/music/abc", "/v1/art/movie/1?x=1", "/v1/ratings/music/1", "/v1/ratings/movie/x"} {
		if w := req(s, q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s %d", q, w.Code)
		}
	}
}

func TestStatsShowsExtras(t *testing.T) {
	var snap struct {
		Extras map[string]extras.Status `json:"extras"`
	}
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 100, Stats: rec, StatsToken: syntheticReader, Fanart: fakeArt{}})
	w := req(s, "/v1/stats", syntheticReader)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &snap) != nil || snap.Extras["fanart"].State != "ok" || snap.Extras["mdblist"].State != "disabled_no_key" {
		t.Fatalf("stats %d %s", w.Code, w.Body)
	}
}

// slowArt answers once released; it records whether its context was still
// alive when the answer came.
type slowArt struct {
	release chan struct{}
	calls   *int32
	alive   *int32
}

func (s slowArt) Art(ctx context.Context, kind, id string) ([]byte, string, error) {
	atomic.AddInt32(s.calls, 1)
	<-s.release
	if ctx.Err() == nil {
		atomic.AddInt32(s.alive, 1)
	}
	return []byte(`{"kind":"music","id":"` + id + `","logo":"https://a/l.png","backgrounds":["https://a/b.jpg"],"source":"fanart.tv"}`), "miss", nil
}
func (slowArt) Status() extras.Status { return extras.Status{State: extras.StateOK} }

// A cold artist's art is fetched right away and kept even when the first
// asker stops waiting: it gets 202 with a short Retry-After, the next ask
// gets the art.
func TestColdArtFetchesNowAndKeepsGoing(t *testing.T) {
	old := artWait
	artWait = 30 * time.Millisecond
	defer func() { artWait = old }()
	var calls, alive int32
	src := slowArt{release: make(chan struct{}), calls: &calls, alive: &alive}
	s, _, _ := setup(Options{Burst: 100, Fanart: src})
	const p = "/v1/art/music/5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	w := req(s, p, "synthetic-test-token")
	if w.Code != 202 || w.Header().Get("Retry-After") != "1" || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("cold %d %q %q", w.Code, w.Header().Get("Retry-After"), w.Header().Get("Cache-Control"))
	}
	close(src.release)
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&alive) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt32(&alive) != 1 {
		t.Fatal("the fetch was cut off with the first request")
	}
	if w = req(s, p, "synthetic-test-token"); w.Code != 200 || !strings.Contains(w.Body.String(), "b.jpg") {
		t.Fatalf("after %d %s", w.Code, w.Body)
	}
}
