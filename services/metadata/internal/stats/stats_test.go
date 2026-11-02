package stats

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBucketsAggregateAndSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 19, 12, 3, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r, err := Open(filepath.Join(dir, "stats.sqlite"), Options{Now: clock, Size: func() int64 { return 4096 }})
	if err != nil {
		t.Fatal(err)
	}
	r.Request(200, "hit", 3*time.Millisecond)
	r.Request(200, "miss", 40*time.Millisecond)
	r.Request(503, "", 900*time.Millisecond)
	r.Request(429, "", time.Millisecond)
	r.Request(404, "", time.Millisecond)
	r.ProviderError("tmdb", "upstream_status")
	r.Stored("k1", "movie")
	r.Stored("k1", "movie")
	r.Stored("k2", "artist")
	now = now.Add(5 * time.Minute)
	r.Request(200, "hit", 2*time.Millisecond)
	r.Stored("k3", "show")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = Open(filepath.Join(dir, "stats.sqlite"), Options{Now: clock, Size: func() int64 { return 8192 }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s, err := r.Snapshot(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tot := s.Totals
	if tot.Requests != 6 || tot.Successes != 3 || tot.Failures != 1 || tot.RateLimited != 1 || tot.ClientErrors != 1 || tot.CacheHits != 2 || tot.CacheMisses != 1 {
		t.Fatalf("totals %+v", tot)
	}
	if tot.ProviderErrors["tmdb"]["upstream_status"] != 1 || tot.ItemsAdded["movies"] != 1 || tot.ItemsAdded["music"] != 1 || tot.ItemsAdded["shows"] != 1 {
		t.Fatalf("errors/items %+v", tot)
	}
	if s.ItemsStored["total"] != 3 || s.StorageBytes != 8192 {
		t.Fatalf("stored %+v %d", s.ItemsStored, s.StorageBytes)
	}
	if len(s.Series) != 2 || s.Series[0].Requests != 5 || s.Series[0].Start != "2026-10-19T12:00:00Z" || s.Series[0].P95MS != 1000 {
		t.Fatalf("series %+v", s.Series)
	}
	if s.BucketSeconds != 300 || s.RetentionSeconds != 7*86400 {
		t.Fatal("bucket policy")
	}
}

func TestOldBucketsPruned(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	r, err := Open(filepath.Join(t.TempDir(), "stats.sqlite"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Request(200, "hit", time.Millisecond)
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	r.Request(200, "hit", time.Millisecond)
	s, err := r.Snapshot(30 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Series) != 1 || s.Totals.Requests != 1 {
		t.Fatalf("not pruned %+v", s.Series)
	}
}

func TestItemsPrunedByAgeAndCapped(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	r, err := Open(filepath.Join(t.TempDir(), "stats.sqlite"), Options{Now: func() time.Time { return now }, ItemTTL: 24 * time.Hour, MaxItems: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Stored("old", "movie")
	now = now.Add(48 * time.Hour)
	for _, k := range []string{"a", "b", "c", "d"} {
		r.Stored(k, "show")
		now = now.Add(time.Second)
	}
	s, err := r.Snapshot(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if s.ItemsStored["total"] != 3 || s.ItemsStored["movies"] != 0 {
		t.Fatalf("items %+v", s.ItemsStored)
	}
}

func TestOnlyTitlesCountAsItemsAndListPages(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	r, err := Open(filepath.Join(t.TempDir(), "stats.sqlite"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.StoredItem("old", "movie", "tmdb", "Old Film")
	now = now.Add(3 * time.Hour)
	r.StoredItem("raw", "", "tmdb", "")
	r.StoredItem("list", "similar", "listenbrainz", "")
	r.StoredItem("m1", "movie", "tmdb", "Film One")
	now = now.Add(time.Minute)
	r.StoredItem("s1", "show", "tvmaze", "Show One")
	now = now.Add(time.Minute)
	r.StoredItem("a1", "release-group", "musicbrainz", "Album One")
	s, err := r.Snapshot(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if s.Totals.ItemsAdded["movies"] != 1 || s.Totals.ItemsAdded["shows"] != 1 || s.Totals.ItemsAdded["music"] != 1 || s.ItemsStored["total"] != 4 {
		t.Fatalf("added %+v stored %+v", s.Totals.ItemsAdded, s.ItemsStored)
	}
	p, err := r.Items(time.Hour, "", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || len(p.Items) != 2 || p.Items[0].Title != "Album One" || p.Items[1].Kind != "shows" || p.Items[1].Provider != "tvmaze" {
		t.Fatalf("page %+v", p)
	}
	p, _ = r.Items(time.Hour, "", 2, 2)
	if len(p.Items) != 1 || p.Items[0].Title != "Film One" {
		t.Fatalf("page 2 %+v", p)
	}
	p, _ = r.Items(24*time.Hour, "movies", 10, 0)
	if p.Total != 2 || p.Items[1].Title != "Old Film" {
		t.Fatalf("movies %+v", p)
	}
}

func TestItemsCountUsesCoveringIndex(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "stats.sqlite"), Options{Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var id, parent, unused int
	var plan string
	if err := r.db.QueryRow("EXPLAIN QUERY PLAN SELECT COUNT(*) FROM items WHERE added >= ? AND (?='' OR kind=?)", 0, "music", "music").Scan(&id, &parent, &unused, &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "COVERING INDEX items_by_added_kind") {
		t.Fatalf("plan %q", plan)
	}
}
