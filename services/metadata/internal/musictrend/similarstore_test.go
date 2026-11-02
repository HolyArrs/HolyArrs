package musictrend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Similar artists and artist data survive a restart: a new Similar on the
// same file answers from storage without asking the source; failures are
// never kept.
func TestSimilarAndArtistDataKeptAcrossRestart(t *testing.T) {
	var calls atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "/popularity/top-release-groups-for-artist/"+artistA):
			_, _ = w.Write([]byte(`[{"artist":{"artists":[{"artist_mbid":"` + artistA + `","name":"A"}]},"release_group":{"name":"G","date":"2020","type":"Album"},"release_group_mbid":"` + groupA + `","total_user_count":3,"tag":{"artist":[{"artist_mbid":"` + artistA + `","count":2,"tag":"pop"}]}}]`))
		case strings.Contains(r.URL.Path, "/popularity/"):
			w.WriteHeader(404)
		case r.URL.Query().Get("artist_mbids") == artistA:
			_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
		case r.URL.Query().Get("artist_mbids") == artistB:
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer src.Close()
	path := filepath.Join(t.TempDir(), "similar.sqlite")
	now := time.Unix(1_800_000_000, 0)
	open := func() *Similar {
		s := &Similar{URL: src.URL, APIURL: src.URL, UserAgent: "test-agent", HTTP: src.Client(), Now: func() time.Time { return now }}
		if err := s.OpenStore(path); err != nil {
			t.Fatal(err)
		}
		s.SetKnown(func(id string) bool { return id == artistA || id == artistB })
		return s
	}
	ctx := context.Background()
	s := open()
	body, _, err := s.Artists(ctx, artistA)
	if err != nil || !strings.Contains(string(body), artistB) {
		t.Fatalf("first ask %s %v", body, err)
	}
	if _, _, err := s.Artists(ctx, artistB); !errors.Is(err, ErrNoSimilar) {
		t.Fatalf("none: %v", err)
	}
	if _, _, err := s.Artists(ctx, singer); err == nil {
		t.Fatal("a failing source answered")
	}
	d, err := s.Data(ctx, artistA)
	if err != nil || d == nil || len(d.Albums) == 0 {
		t.Fatalf("data %+v %v", d, err)
	}
	if _, err := s.Data(ctx, artistB); !errors.Is(err, ErrNoSimilar) {
		t.Fatalf("no data: %v", err)
	}
	if c := s.StoreCounts(); c["similar"] != 2 || c["artist_data"] != 2 {
		t.Fatalf("kept %v (the failure must not be)", c)
	}
	s.CloseStore()

	before := calls.Load()
	now = now.Add(30 * time.Minute) // within the hour a "none" is kept
	s2 := open()
	defer s2.CloseStore()
	body2, cache, err := s2.Artists(ctx, artistA)
	if err != nil || string(body2) != string(body) || cache != "hit" {
		t.Fatalf("after restart %s %q %v", body2, cache, err)
	}
	if _, _, err := s2.Artists(ctx, artistB); !errors.Is(err, ErrNoSimilar) {
		t.Fatalf("kept none after restart: %v", err)
	}
	if d2, err := s2.Data(ctx, artistA); err != nil || d2 == nil || d2.Albums[0].MBID != d.Albums[0].MBID {
		t.Fatalf("data after restart %+v %v", d2, err)
	}
	if calls.Load() != before {
		t.Fatalf("a restart asked the source %d times", calls.Load()-before)
	}
	// Past the week a kept answer is asked again.
	now = now.Add(8 * 24 * time.Hour)
	if _, _, err := s2.Artists(ctx, artistA); err != nil || calls.Load() == before {
		t.Fatalf("expired answer not refreshed: %v", err)
	}
}

// A "none" for an id MusicBrainz is not known to have stays in memory.
func TestUnknownMissNotKept(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/popularity/") {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer src.Close()
	s := &Similar{URL: src.URL, APIURL: src.URL, UserAgent: "t", HTTP: src.Client()}
	if err := s.OpenStore(filepath.Join(t.TempDir(), "s.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer s.CloseStore()
	s.SetKnown(func(id string) bool { return id == artistA })
	ctx := context.Background()
	for _, id := range []string{artistA, artistB, singer} {
		if _, _, err := s.Artists(ctx, id); !errors.Is(err, ErrNoSimilar) {
			t.Fatal(err)
		}
		if _, err := s.Data(ctx, id); !errors.Is(err, ErrNoSimilar) {
			t.Fatal(err)
		}
	}
	if c := s.StoreCounts(); c["similar"] != 1 || c["artist_data"] != 1 {
		t.Fatalf("kept %v, want only the known artist", c)
	}
	// The kept "none" rows carry a source and a fetch time (audit
	// 2026-10-29: 259 untagged none rows).
	var tag, dsrc string
	var at, dat int64
	if err := s.db.QueryRow(`SELECT src, fetched_at FROM similar WHERE mbid=?`, artistA).Scan(&tag, &at); err != nil || tag != SimilarNoneSrc || at <= 0 {
		t.Fatalf("similar none tag %q %d %v", tag, at, err)
	}
	if err := s.db.QueryRow(`SELECT src, fetched_at FROM artist_data WHERE mbid=?`, artistA).Scan(&dsrc, &dat); err != nil || dsrc != "listenbrainz" || dat <= 0 {
		t.Fatalf("artist data none tag %q %d %v", dsrc, dat, err)
	}
	// Still answered from memory.
	if _, cache, err := s.Artists(ctx, artistB); !errors.Is(err, ErrNoSimilar) || cache != "hit" {
		t.Fatalf("memory miss %q %v", cache, err)
	}
}

// The prune drops rows past their lifetime (a none after MissTTL, a list
// after TTL) and the cap keeps each table to twice Max, newest first.
func TestStorePruneAndCap(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := &Similar{Max: 2, Now: func() time.Time { return now }}
	if err := s.OpenStore(filepath.Join(t.TempDir(), "s.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer s.CloseStore()
	s.SetKnown(func(string) bool { return true })
	s.mu.Lock()
	s.entries = map[string]similarEntry{}
	s.keepSimilarLocked(artistA, similarEntry{body: []byte(`{}`), at: now.Add(-2 * time.Hour)})
	s.keepSimilarLocked(artistB, similarEntry{err: ErrNoSimilar, at: now.Add(-2 * time.Hour)})
	s.keepSimilarLocked(singer, similarEntry{body: []byte(`{}`), at: now.Add(-8 * 24 * time.Hour)})
	s.keepDataLocked(artistA, nil, now.Add(-2*time.Hour))
	s.keepDataLocked(artistB, &ArtistData{MBID: artistB}, now.Add(-2*time.Hour))
	s.mu.Unlock()
	s.PruneStore()
	var left []string
	rows, _ := s.db.Query(`SELECT mbid FROM similar ORDER BY mbid`)
	for rows.Next() {
		var m string
		rows.Scan(&m)
		left = append(left, m)
	}
	rows.Close()
	if len(left) != 1 || left[0] != artistA {
		t.Fatalf("similar left %v, want the fresh list only", left)
	}
	if c := s.StoreCounts(); c["artist_data"] != 1 {
		t.Fatalf("artist data left %v", c)
	}
	// Cap: 2*Max = 4 rows, the newest kept.
	s.mu.Lock()
	for i := range 10 {
		id := fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		s.keepSimilarLocked(id, similarEntry{body: []byte(`{}`), at: now.Add(time.Duration(i) * time.Second)})
	}
	s.capLocked()
	s.mu.Unlock()
	if c := s.StoreCounts(); c["similar"] != 4 {
		t.Fatalf("cap kept %d rows, want 4", c["similar"])
	}
	var newest string
	s.db.QueryRow(`SELECT mbid FROM similar ORDER BY at DESC LIMIT 1`).Scan(&newest)
	if !strings.HasPrefix(newest, "00000009") {
		t.Fatalf("newest kept %s", newest)
	}
}

func TestDropSourceOnlyThatSource(t *testing.T) {
	s := &Similar{}
	if err := s.OpenStore(filepath.Join(t.TempDir(), "similar.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer s.CloseStore()
	s.mu.Lock()
	if s.entries == nil {
		s.entries = map[string]similarEntry{}
	}
	s.keepSimilarLocked("a", similarEntry{body: []byte(`{"source":"applemusic"}`), at: time.Now()})
	s.keepSimilarLocked("b", similarEntry{body: []byte(`{"source":"listenbrainz"}`), at: time.Now()})
	s.mu.Unlock()
	if s.DropSource("b", "applemusic") != nil {
		t.Fatal("dropped a ListenBrainz list")
	}
	if b := s.DropSource("a", "applemusic"); string(b) != `{"source":"applemusic"}` {
		t.Fatalf("dropped %s", b)
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM similar`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows left %d", n)
	}
}

// Expire makes a kept list due (memory and storage): the next ask goes to
// the source again.
func TestSimilarExpire(t *testing.T) {
	var calls atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("artist_mbids") == artistA {
			_, _ = w.Write([]byte(`[{"artist_mbid":"` + artistB + `","name":"B","score":9}]`))
			return
		}
		w.WriteHeader(404)
	}))
	defer src.Close()
	path := filepath.Join(t.TempDir(), "similar.sqlite")
	s := &Similar{URL: src.URL, APIURL: src.URL, UserAgent: "test-agent", HTTP: src.Client()}
	if err := s.OpenStore(path); err != nil {
		t.Fatal(err)
	}
	defer s.CloseStore()
	s.SetKnown(func(string) bool { return true })
	ctx := context.Background()
	if _, _, err := s.Artists(ctx, artistA); err != nil {
		t.Fatal(err)
	}
	if _, cache, _ := s.Artists(ctx, artistA); cache != "hit" {
		t.Fatalf("not kept: %q", cache)
	}
	s.Expire(artistA)
	before := calls.Load()
	if _, cache, err := s.Artists(ctx, artistA); err != nil || cache != "miss" || calls.Load() == before {
		t.Fatalf("expired list not asked again: %q %v", cache, err)
	}
}
