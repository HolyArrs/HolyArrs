package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseLBSimilar(t *testing.T) {
	body := `[{"artist_mbid":"` + artistB + `","name":"B","score":9,"reference_mbid":"` + artistA + `"},
	{"artist_mbid":"` + artistA + `","name":"Self","score":8,"reference_mbid":"` + artistA + `"},
	{"artist_mbid":"bad","name":"Bad","score":7},
	{"artist_mbid":"` + artistB + `","name":"Again","score":6},
	{"artist_mbid":"` + singer + `","name":" ","score":5},
	{"artist_mbid":"` + grand + `","name":"Other ref","score":4,"reference_mbid":"` + singer + `"},
	{"artist_mbid":"` + strings.ToUpper(groupA) + `","name":"G","score":3}]`
	got, err := ParseLBSimilar(strings.NewReader(body), artistA)
	if err != nil || len(got) != 2 || got[0].MBID != artistB || got[0].Score != 9 || got[1].MBID != groupA {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := ParseLBSimilar(strings.NewReader(`{"x":1}`), artistA); err == nil {
		t.Fatal("odd answer accepted")
	}
}

func TestSimilarCachesAndReports(t *testing.T) {
	var calls atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "test-agent" || r.URL.Query().Get("algorithm") == "" {
			t.Errorf("bad request %s", r.URL)
		}
		switch r.URL.Query().Get("artist_mbids") {
		case artistA:
			_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
		case artistB:
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer src.Close()
	now := time.Unix(1_800_000_000, 0)
	var reports atomic.Int32
	s := &Similar{URL: src.URL, UserAgent: "test-agent", HTTP: src.Client(), Max: 2, Now: func() time.Time { return now }, Report: func(source, code string) {
		if source == "listenbrainz" && code == "similar_failed" {
			reports.Add(1)
		}
	}}
	ctx := context.Background()
	b, cache, err := s.Artists(ctx, strings.ToUpper(artistA))
	var got struct {
		Source  string          `json:"source"`
		Artists []SimilarArtist `json:"artists"`
	}
	if err != nil || cache != "miss" || json.Unmarshal(b, &got) != nil || got.Source != "listenbrainz" || len(got.Artists) != 1 || got.Artists[0].Name != "B" {
		t.Fatalf("%s %s %v", b, cache, err)
	}
	if _, cache, _ := s.Artists(ctx, artistA); cache != "hit" || calls.Load() != 1 {
		t.Fatalf("not cached: %s %d", cache, calls.Load())
	}
	if _, _, err := s.Artists(ctx, artistB); !errors.Is(err, ErrNoSimilar) {
		t.Fatalf("empty answer %v", err)
	}
	if _, _, err := s.Artists(ctx, grand); err == nil || errors.Is(err, ErrNoSimilar) || reports.Load() != 1 {
		t.Fatalf("failure %v %d", err, reports.Load())
	}
	if _, _, err := s.Artists(ctx, "nope"); !errors.Is(err, ErrNoSimilar) {
		t.Fatal("bad id fetched")
	}
	if len(s.entries) > 2 {
		t.Fatalf("cache over its cap: %d", len(s.entries))
	}
	now = now.Add(8 * 24 * time.Hour)
	before := calls.Load()
	if _, cache, _ := s.Artists(ctx, artistA); cache != "miss" || calls.Load() != before+1 {
		t.Fatal("old answer kept past a week")
	}
}

func TestSimilarServesStaleWhenRefetchFails(t *testing.T) {
	var down atomic.Bool
	var calls atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if down.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
	}))
	defer src.Close()
	now := time.Unix(1_800_000_000, 0)
	s := &Similar{URL: src.URL, UserAgent: "test-agent", HTTP: src.Client(), Now: func() time.Time { return now }}
	ctx := context.Background()
	first, _, err := s.Artists(ctx, artistA)
	if err != nil {
		t.Fatal(err)
	}
	down.Store(true)
	now = now.Add(8 * 24 * time.Hour)
	b, cache, err := s.Artists(ctx, artistA)
	if err != nil || cache != "stale" || string(b) != string(first) {
		t.Fatalf("stale: %s %s %v", b, cache, err)
	}
	// Kept for the failure lifetime: no new upstream call right away.
	if _, cache, err := s.Artists(ctx, artistA); err != nil || cache != "hit" || calls.Load() != 2 {
		t.Fatalf("kept: %s %v %d", cache, err, calls.Load())
	}
}

func TestSimilarAppleFirstThenListenBrainz(t *testing.T) {
	// ListenBrainz knows B for A (a name hint for Apple's "B").
	lb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("artist_mbids") {
		case artistA, singer:
			_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer lb.Close()
	db, err := openDB(t.TempDir() + "/m.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	search := &fakeSearch{}
	var appleErr atomic.Bool
	apple := map[string][]NamedArtist{
		artistA: {{ID: "1", Name: "Chanteuse"}, {ID: "2", Name: "b"}, {ID: "3", Name: "Nobody Known"}},
		singer:  {},
	}
	s := &Similar{URL: lb.URL, HTTP: lb.Client(), Names: &Matcher{DB: db, Search: search}, NamePace: -1,
		Apple: func(_ context.Context, seed string) ([]NamedArtist, error) {
			if appleErr.Load() {
				return nil, errors.New("apple: answered 503")
			}
			return apple[seed], nil
		}}
	ctx := context.Background()
	// Chanteuse and "Nobody Known" are not matched yet: B (by ListenBrainz's
	// name) answers now, kept short while the rest are searched.
	b, _, err := s.Artists(ctx, artistA)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Source  string          `json:"source"`
		Artists []SimilarArtist `json:"artists"`
	}
	_ = json.Unmarshal(b, &got)
	if got.Source != "applemusic" || len(got.Artists) != 1 || got.Artists[0].MBID != artistB || got.Artists[0].Score != 2 {
		t.Fatalf("first answer %+v", got)
	}
	for i := 0; i < 200 && func() bool { s.nameMu.Lock(); defer s.nameMu.Unlock(); return s.nameRun }(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	s.entries = nil // the short answer has run out
	b, _, _ = s.Artists(ctx, artistA)
	_ = json.Unmarshal(b, &got)
	if len(got.Artists) != 2 || got.Artists[0].MBID != singer || got.Artists[0].Score != 3 || got.Artists[1].MBID != artistB {
		t.Fatalf("after matching %+v", got)
	}
	// Apple has none: ListenBrainz answers.
	b, _, err = s.Artists(ctx, singer)
	_ = json.Unmarshal(b, &got)
	if err != nil || got.Source != "listenbrainz" {
		t.Fatalf("apple none: %s %v", b, err)
	}
	// Apple fails: ListenBrainz answers, kept only an hour.
	appleErr.Store(true)
	s.entries = nil
	b, _, err = s.Artists(ctx, artistA)
	_ = json.Unmarshal(b, &got)
	if err != nil || got.Source != "listenbrainz" || !s.entries[artistA].short {
		t.Fatalf("apple failed: %s %v", b, err)
	}
}

func TestSimilarPendingIsNeverKept(t *testing.T) {
	db, err := openDB(t.TempDir() + "/m.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	block := make(chan struct{})
	s := &Similar{URL: "http://127.0.0.1:1", Names: &Matcher{DB: db, Search: blockSearch(block)},
		Apple: func(context.Context, string) ([]NamedArtist, error) {
			return []NamedArtist{{ID: "9", Name: "Unknown"}}, nil
		}}
	defer close(block)
	if _, _, err := s.Artists(context.Background(), artistA); !errors.Is(err, ErrPending) {
		t.Fatalf("got %v", err)
	}
	if _, kept := s.entries[artistA]; kept {
		t.Fatal("a pending answer was kept")
	}
}

type blockSearch chan struct{}

func (b blockSearch) Search(ctx context.Context, _, _, _ string) ([]Candidate, error) {
	select {
	case <-b:
	case <-ctx.Done():
	}
	return nil, errors.New("closed")
}
