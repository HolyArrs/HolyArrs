package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

const slimArtist = "b1111111-1111-4111-8111-111111111111"

// slimServer has a slim copy holding slimArtist only, and a live fetcher
// that counts calls.
func slimServer(t *testing.T) (*Server, *atomic.Int64) {
	t.Helper()
	m := &memoryStore{entries: map[string]model.Entry{}, authorized: true}
	calls := &atomic.Int64{}
	f := fetchFunc(func(c context.Context, q model.Query) (model.Result, error) {
		calls.Add(1)
		return model.Result{Data: json.RawMessage(`{"name":"Live"}`), Complete: true}, nil
	})
	s := New(m, f, Options{Burst: 100, QueueWait: time.Second, Slim: func(_ context.Context, kind, mbid string) ([]byte, string, bool) {
		if kind == "artist" && mbid == slimArtist {
			return []byte(`{"source":"musicbrainz","kind":"artist","title":"Slim","source_data":{"name":"Slim"},"source_tag":{"source":"mb-dump","ref":"20261025-001001"}}`), "20261025-001001", true
		}
		return nil, "", false
	}})
	q, err := fetchq.Open(filepath.Join(t.TempDir(), "q.sqlite"), fetchq.Options{Rate: 50, Do: s.RunJob})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	s.SetQueue(q)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.Run(ctx)
	return s, calls
}

func TestSlimAnswersFirst(t *testing.T) {
	s, calls := slimServer(t)
	w := req(s, "/v1/music/musicbrainz/artist/"+slimArtist, "")
	if w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "slim" || calls.Load() != 0 {
		t.Fatalf("slim item: %d %s cache=%q calls=%d", w.Code, w.Body, w.Header().Get("X-Metadata-Cache"), calls.Load())
	}
	var body struct {
		FetchedAt time.Time `json:"fetched_at"`
		Data      struct {
			Title string         `json:"title"`
			Tag   map[string]any `json:"source_tag"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Title != "Slim" || body.Data.Tag["source"] != "mb-dump" || body.FetchedAt.Format("20060102") != "20261025" {
		t.Fatalf("slim body %s", w.Body)
	}
	if b, ok := s.StoredData(context.Background(), "artist", slimArtist); !ok || len(b) == 0 {
		t.Fatal("StoredData skipped the slim copy")
	}
	if err := s.RunJob(context.Background(), QueueKey("artist", slimArtist), fetchq.P2Shared); err != nil || calls.Load() != 0 {
		t.Fatalf("queue job for a slim item: err=%v calls=%d", err, calls.Load())
	}
}

func TestSlimMissingFallsBackToQueue(t *testing.T) {
	s, calls := slimServer(t)
	w := req(s, "/v1/music/musicbrainz/artist/"+queuedArtist, "")
	if w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "miss" || calls.Load() != 1 {
		t.Fatalf("gap item: %d %s cache=%q calls=%d", w.Code, w.Body, w.Header().Get("X-Metadata-Cache"), calls.Load())
	}
	if _, ok := s.StoredData(context.Background(), "artist", queuedArtist); !ok {
		t.Fatal("gap item not stored from the live answer")
	}
	// Searches never go to the slim copy.
	if w := req(s, "/v1/music/musicbrainz/artist?q=x", ""); w.Header().Get("X-Metadata-Cache") == "slim" {
		t.Fatal("search answered from the slim copy")
	}
}

func TestRevalidateSkipsSlimRow(t *testing.T) {
	s, calls := slimServer(t)
	b, _ := json.Marshal(model.Query{Provider: "musicbrainz", Kind: "artist", Operation: "detail", ID: slimArtist})
	if err := s.Revalidate(context.Background(), refresh.Item{Key: "k", Query: string(b)}); err != nil || calls.Load() != 0 {
		t.Fatalf("revalidate slim row: err=%v calls=%d", err, calls.Load())
	}
}
