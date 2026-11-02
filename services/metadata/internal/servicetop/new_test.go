package servicetop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// changesAPI answers /changes with two new titles (one repeated, one
// without a TMDB id) and /shows/top with a small list.
type changesAPI struct{ changes, tops atomic.Int64 }

func (f *changesAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `[{"tmdbId":"movie/1","title":"Top"}]`
	if strings.HasSuffix(r.URL.Path, "/changes") {
		f.changes.Add(1)
		q := r.URL.Query()
		if q.Get("change_type") != "new" || q.Get("catalogs") == "" || q.Get("from") == "" {
			return &http.Response{StatusCode: 400, Status: "400", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		ts := strconv.FormatInt(time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC).Unix(), 10)
		old := strconv.FormatInt(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Unix(), 10)
		body = `{"changes":[{"showId":"a","timestamp":` + ts + `},{"showId":"a","timestamp":` + ts + `},{"showId":"b","timestamp":` + ts + `},{"showId":"c","timestamp":` + old + `}],
"shows":{"a":{"tmdbId":"tv/7","title":"Fresh"},"b":{"tmdbId":"","title":"No id"},"c":{"tmdbId":"movie/9","title":"Old"}},"hasMore":false}`
	} else {
		f.tops.Add(1)
	}
	h := http.Header{}
	h.Set("X-Quota-Used", "1")
	h.Set("X-Quota-Granted", "1000")
	h.Set("X-Quota-Reset", "2026-11-01T00:00:00Z")
	return &http.Response{StatusCode: 200, Status: "200", Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestNewListsOncePerDay(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	api := &changesAPI{}
	path := filepath.Join(t.TempDir(), "s.sqlite")
	open := func() *Service {
		s, err := Open(path, Options{BaseURL: "https://sa.test", Keys: []string{"k"}, New: true, TMDB: &fakeTMDB{}, HTTP: &http.Client{Transport: api}, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	s.Refresh(context.Background())
	sa := int64(0)
	for _, p := range Providers {
		if p.SA != "" {
			sa++
		}
	}
	if api.changes.Load() != sa {
		t.Fatalf("changes calls = %d, want %d", api.changes.Load(), sa)
	}
	body, _, _ := s.List("US")
	var out struct {
		Services []struct {
			Service string    `json:"service"`
			New     []NewItem `json:"new"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Services) == 0 || len(out.Services[0].New) != 1 || out.Services[0].New[0].TMDBID != 7 || out.Services[0].New[0].Kind != "tv" || out.Services[0].New[0].PosterPath == "" {
		t.Fatalf("new = %+v", out.Services)
	}
	// Same day, and after a restart: no more calls.
	now = now.Add(6 * time.Hour)
	s.Refresh(context.Background())
	s.Close()
	s = open()
	defer s.Close()
	s.Refresh(context.Background())
	if api.changes.Load() != sa {
		t.Fatalf("changes calls = %d after same-day refresh, want %d", api.changes.Load(), sa)
	}
	now = now.Add(19 * time.Hour)
	s.Refresh(context.Background())
	if api.changes.Load() != 2*sa {
		t.Fatalf("changes calls = %d next day, want %d", api.changes.Load(), 2*sa)
	}
}

func TestNewListsStopBeforeTopQuota(t *testing.T) {
	r := newRig(t)
	r.api.granted = 100
	r.s.opts.New = true
	// 85 used: under the 90% top stop, over the 80% new stop.
	r.s.saveKey(keyID("k1"), keyState{used: 85, granted: 100, reset: r.now.Add(240 * time.Hour)})
	if b := r.s.keyBlockAt("k1", r.s.opts.NewStop); b != StateQuotaPaused {
		t.Fatalf("new block = %q", b)
	}
	if b := r.s.keyBlock("k1"); b != "" {
		t.Fatalf("top block = %q", b)
	}
}
