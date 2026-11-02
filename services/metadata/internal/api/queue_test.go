package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

const queuedArtist = "a74b1b7f-71a5-4011-9441-d0b5e4122711"

func queued(t *testing.T, delay time.Duration) (*Server, *fetchq.Queue, *atomic.Int64) {
	t.Helper()
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	calls := &atomic.Int64{}
	f := fetchFunc(func(c context.Context, q model.Query) (model.Result, error) {
		calls.Add(1)
		time.Sleep(delay)
		return model.Result{Data: json.RawMessage(`{"name":"Synthetic"}`), Complete: true}, nil
	})
	var fetched atomic.Int64
	s := New(m, f, Options{Burst: 100, QueueWait: 300 * time.Millisecond, OnFetched: func(string, string) { fetched.Add(1) }})
	q, err := fetchq.Open(filepath.Join(t.TempDir(), "q.sqlite"), fetchq.Options{Rate: 50, Do: s.RunJob})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	s.SetQueue(q)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.Run(ctx)
	return s, q, calls
}

func TestQueuedMissAnswersWithinWait(t *testing.T) {
	s, _, calls := queued(t, 0)
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "miss" {
		t.Fatalf("fast miss %d %s", w.Code, w.Body)
	}
	if w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, ""); w.Code != 200 || calls.Load() != 1 {
		t.Fatalf("second ask refetched: %d calls", calls.Load())
	}
}

func TestQueuedSlowMissIsPartialAndNoStore(t *testing.T) {
	s, _, _ := queued(t, 2*time.Second)
	started := time.Now()
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 202 || w.Header().Get("Retry-After") == "" || w.Header().Get("Cache-Control") != "no-store" || time.Since(started) > time.Second {
		t.Fatalf("slow miss %d %v in %s", w.Code, w.Header(), time.Since(started))
	}
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["partial"] != true {
		t.Fatalf("not partial %s", w.Body)
	}
}

func TestPrefetchMissMakesOnlyP4(t *testing.T) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	s := New(m, fetchFunc(func(context.Context, model.Query) (model.Result, error) { return model.Result{}, nil }), Options{Burst: 100})
	rec := &recordingQueue{}
	s.SetQueue(rec)
	r := httptest.NewRequest("GET", "/v1/music/musicbrainz/artist/"+queuedArtist, nil)
	r.Header.Set("X-Fetch", "ahead")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 202 || len(rec.p) != 1 || rec.p[0] != fetchq.P4Deep || rec.wait {
		t.Fatalf("prefetch %d %v wait=%v", w.Code, rec.p, rec.wait)
	}
}

type recordingQueue struct {
	p    []int
	wait bool
}

func (r *recordingQueue) Enqueue(_ context.Context, _ string, p int, wait bool) <-chan error {
	r.p = append(r.p, p)
	r.wait = r.wait || wait
	return nil
}

// posQueue answers a fixed state and place in line for every key.
type posQueue struct {
	recordingQueue
	state string
	pos   int
}

func (q *posQueue) Position(context.Context, string) (string, int) { return q.state, q.pos }

func TestQueuedAnswerSaysPlaceInLine(t *testing.T) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	s := New(m, fetchFunc(func(context.Context, model.Query) (model.Result, error) { return model.Result{}, nil }), Options{Burst: 100})
	for _, c := range []struct {
		state, want, msg string
		pos              int
	}{{"queued", "queued", "Queued, number 7 in line", 7}, {"running", "fetching", "Getting this from the server…", 0}, {"", "fetching", "Getting this from the server…", 0}} {
		s.SetQueue(&posQueue{state: c.state, pos: c.pos})
		r := httptest.NewRequest("GET", "/v1/music/musicbrainz/artist/"+queuedArtist, nil)
		r.Header.Set("X-Fetch", "ahead")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != 202 || body["state"] != c.want || body["message"] != c.msg {
			t.Fatalf("%q: %d %s", c.state, w.Code, w.Body)
		}
		if pos, _ := body["position"].(float64); int(pos) != c.pos {
			t.Fatalf("%q: position %v", c.state, body["position"])
		}
	}
}

func TestQueuedMissWithRealQueueHasPosition(t *testing.T) {
	s, _, _ := queued(t, 2*time.Second)
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 202 || (body["state"] != "fetching" && body["state"] != "queued") || body["message"] == nil {
		t.Fatalf("slow miss %d %s", w.Code, w.Body)
	}
}

func TestLiveMusicBrainzAnswerIsTagged(t *testing.T) {
	s, _, _ := queued(t, 0)
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	var body struct {
		Data struct {
			Tag map[string]any `json:"source_tag"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || body.Data.Tag["source"] != "mb-live" || body.Data.Tag["fetched_at"] == nil || body.Data.Tag["ref"] != queuedArtist {
		t.Fatalf("live answer tag %d %s", w.Code, w.Body)
	}
}

func TestLiveTagNeverServesEgressAddress(t *testing.T) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	var kept []srctag.Tag
	s := New(m, fetchFunc(func(c context.Context, q model.Query) (model.Result, error) {
		return model.Result{Data: json.RawMessage(`{"name":"Synthetic"}`), Complete: true}, nil
	}), Options{Burst: 100, LiveTag: func(kind, id string, t srctag.Tag) { kept = append(kept, t) }})
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "via_ip") || !strings.Contains(w.Body.String(), `"mb-live"`) {
		t.Fatalf("served tag %d %s", w.Code, w.Body)
	}
	if len(kept) != 1 || kept[0].Source != "mb-live" || kept[0].Ref != queuedArtist || kept[0].FetchedAt.IsZero() {
		t.Fatalf("internal tag %+v", kept)
	}
}

// Security M1: with the slim copy loaded, an anonymous ask for an id it
// lacks is answered 404 at once and never queued; the answer is kept a
// short while; a credentialed ask still goes to the queue.
func TestAnonymousSlimMissNeverQueued(t *testing.T) {
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	var checks atomic.Int64
	s := New(m, fetchFunc(func(context.Context, model.Query) (model.Result, error) { return model.Result{}, nil }), Options{Burst: 100,
		SlimMissing: func(context.Context, string, string) bool { checks.Add(1); return true }})
	rq := &recordingQueue{}
	s.SetQueue(rq)
	for i := 0; i < 2; i++ {
		if w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, ""); w.Code != 404 {
			t.Fatalf("anonymous miss %d %s", w.Code, w.Body)
		}
	}
	if len(rq.p) != 0 {
		t.Fatalf("anonymous miss queued %v", rq.p)
	}
	if checks.Load() != 1 {
		t.Fatalf("negative cache not used: %d checks", checks.Load())
	}
	_ = req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "synthetic-test-token")
	if len(rq.p) != 1 {
		t.Fatalf("credentialed miss not queued: %v", rq.p)
	}
}
