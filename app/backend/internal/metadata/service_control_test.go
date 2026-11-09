package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlReadsSwitchAndOldServer(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/control" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("the switch is public; no install token is sent")
		}
		_, _ = w.Write([]byte(`{"prefetch":"slow","prebuild_min_hours":4,"max_rps":0.5,"providers":{"caa":false},"queue":{"p2":true}}`))
	})
	c, err := s.Control(context.Background())
	if err != nil || c.Prefetch != "slow" || c.PrebuildMinHours != 4 || c.MaxRPS != 0.5 || c.Providers["caa"] {
		t.Fatalf("control = %+v, %v", c, err)
	}
	old := testService(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, err := old.Control(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an older server without the switch must answer ErrNotFound, got %v", err)
	}
	if got := (Control{Prefetch: "weird", PrebuildMinHours: 99}).Normal(); got.Prefetch != "on" || got.PrebuildMinHours != 0 {
		t.Fatalf("normal = %+v", got)
	}
}

func artistEnvelope(id, data string) string {
	now := time.Now().UTC()
	return fmt.Sprintf(`{"schema_version":1,"source":"musicbrainz","source_id":%q,"fetched_at":%q,"expires_at":%q,"completeness":"complete","stale":false,"data":%s}`,
		id, now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), data)
}

func TestArtistVideosByFeature(t *testing.T) {
	const id = "5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	var body atomic.Value
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(artistEnvelope(id, body.Load().(string))))
	})
	body.Store(`{"source":"musicbrainz","source_id":"` + id + `","kind":"artist","title":"A","source_data":{"id":"` + id + `"},"videos":[{"youtube_id":"dQw4w9WgXcQ","most_replayed":63.5,"duration":212},"bad id","hTWKbfoikeg",{"id":"hTWKbfoikeg"}]}`)
	v, err := s.ArtistVideos(context.Background(), id)
	if err != nil || len(v) != 2 || v[0].ID != "dQw4w9WgXcQ" || v[0].MostReplayed != 63.5 || v[1].ID != "hTWKbfoikeg" {
		t.Fatalf("videos = %+v, %v", v, err)
	}
	body.Store(`{"source":"musicbrainz","source_id":"` + id + `","kind":"artist","source_data":{"videos":["dQw4w9WgXcQ"]}}`)
	if v, err := s.ArtistVideos(context.Background(), id); err != nil || len(v) != 1 {
		t.Fatalf("source_data videos = %+v, %v", v, err)
	}
	body.Store(`{"source":"musicbrainz","source_id":"` + id + `","kind":"artist","source_data":{}}`)
	if v, err := s.ArtistVideos(context.Background(), id); err != nil || len(v) != 0 {
		t.Fatalf("an older server has no videos: %+v, %v", v, err)
	}
}

func TestAheadHeaderAndRetryAfterHold(t *testing.T) {
	var calls, ahead atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Fetch") == "ahead" {
			ahead.Add(1)
		}
		w.Header().Set("Retry-After", "0")
		if calls.Load() >= 2 {
			w.Header().Set("Retry-After", "120")
		}
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ctx := Ahead(context.Background())
	if _, err := s.Art(ctx, "movie", "603"); !errors.Is(err, ErrServiceBusy) {
		t.Fatalf("art = %v", err)
	}
	if ahead.Load() != 1 {
		t.Fatalf("fetch-ahead requests carry X-Fetch: ahead")
	}
	_, _ = s.Art(ctx, "movie", "603")
	n := calls.Load()
	if until := s.HeldUntil(); time.Until(until) < 100*time.Second {
		t.Fatalf("Retry-After 120 must hold fetch-ahead, until %v", until)
	}
	if _, err := s.Art(ctx, "movie", "603"); !errors.Is(err, ErrServiceBusy) || calls.Load() != n {
		t.Fatalf("held fetch-ahead must not reach the server: %v, calls %d→%d", err, n, calls.Load())
	}
	// A person's own request is never held.
	_, _ = s.Art(context.Background(), "movie", "603")
	if calls.Load() != n+1 {
		t.Fatalf("a person's request must still be sent")
	}
}
