package refresh

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

func openTracker(t *testing.T, now *time.Time) *Tracker {
	t.Helper()
	tr, err := Open(filepath.Join(t.TempDir(), "refresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	tr.Now = func() time.Time { return *now }
	return tr
}

func TestTrackerMarksAppliesAndMeasuresLag(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tr := openTracker(t, &now)
	tr.Record(ctx, "k1", `{}`, "tmdb:movie:1")
	tr.Record(ctx, "k2", `{}`, "tmdb:movie:2")
	if n, _ := tr.MarkRefs(ctx, []string{"tmdb:movie:1", "tmdb:tv:9"}); n != 1 {
		t.Fatalf("marked %d", n)
	}
	if _, stale, ok := tr.State(ctx, "k1"); !ok || !stale {
		t.Fatal("k1 not stale")
	}
	p, _ := tr.Pending(ctx, now.Add(-time.Hour), 10)
	if len(p) != 1 || p[0].Key != "k1" {
		t.Fatalf("pending %+v", p)
	}
	now = now.Add(40 * time.Minute)
	tr.Record(ctx, "k1", `{}`, "tmdb:movie:1")
	st := tr.Snapshot(ctx)
	if st.Stale != 0 || st.ChangesApplied != 1 || st.MedianChangeToApplied != 2400 || st.StoreItems != 2 {
		t.Fatalf("stats %+v", st)
	}
}

type pagedFetcher struct {
	pages map[string][][]int64
	fail  string
	calls int
}

func (f *pagedFetcher) Fetch(_ context.Context, q model.Query) (model.Result, error) {
	f.calls++
	kind := strings.Split(q.Path, "/")[1]
	if kind == f.fail {
		return model.Result{}, &model.FetchError{Code: "upstream_unavailable"}
	}
	page := 1
	fmt.Sscanf(q.Params[strings.Index(q.Params, "page=")+5:], "%d", &page)
	pages := f.pages[kind]
	var results []map[string]int64
	if page <= len(pages) {
		for _, id := range pages[page-1] {
			results = append(results, map[string]int64{"id": id})
		}
	}
	b, _ := json.Marshal(map[string]any{"results": results, "total_pages": len(pages), "page": page})
	return model.Result{Data: b, Complete: true}, nil
}

func TestTMDBPollerPagesIntersectsAndKeepsCursorOnFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tr := openTracker(t, &now)
	tr.Record(ctx, "movie", `{}`, "tmdb:movie:3")
	tr.Record(ctx, "show", `{}`, "tmdb:tv:7")
	tr.Record(ctx, "other", `{}`, "tmdb:movie:99")
	f := &pagedFetcher{pages: map[string][][]int64{"movie": {{1, 2}, {3}, {4}}, "tv": {{7}}, "person": {}}}
	p := &TMDBPoller{Tracker: tr, Fetcher: f, Now: func() time.Time { return now }}
	if n, err := p.Poll(ctx); err != nil || n != 0 {
		t.Fatalf("first run %d %v", n, err)
	}
	now = now.Add(time.Hour)
	f.fail = "tv"
	if _, err := p.Poll(ctx); err == nil {
		t.Fatal("failure not reported")
	}
	if tr.Get(ctx, tmdbCursor) != "2026-10-20" {
		t.Fatalf("cursor moved on failure: %s", tr.Get(ctx, tmdbCursor))
	}
	f.fail = ""
	f.calls = 0
	if n, err := p.Poll(ctx); err != nil || n != 2 {
		t.Fatalf("marked %d %v", n, err)
	}
	if f.calls != 5 { // 3 movie pages, 1 tv, 1 person
		t.Fatalf("calls %d", f.calls)
	}
	if _, stale, _ := tr.State(ctx, "other"); stale {
		t.Fatal("unchanged row marked")
	}
	if tr.Get(ctx, tmdbCursor) != "2026-10-21" {
		t.Fatal("cursor not advanced")
	}
}

func TestTMDBPollerLongOutageMarksEverythingStale(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tr := openTracker(t, &now)
	now = now.Add(-25 * 24 * time.Hour)
	tr.Record(ctx, "a", `{}`, "tmdb:movie:1")
	tr.Record(ctx, "m", `{}`, "musicbrainz:x")
	now = now.Add(25 * 24 * time.Hour)
	tr.Set(ctx, tmdbCursor, "2026-10-01")
	f := &pagedFetcher{}
	p := &TMDBPoller{Tracker: tr, Fetcher: f, Now: func() time.Time { return now }}
	if n, err := p.Poll(ctx); err != nil || n != 1 || f.calls != 0 {
		t.Fatalf("recovery %d %v calls %d", n, err, f.calls)
	}
	if _, stale, _ := tr.State(ctx, "m"); stale {
		t.Fatal("music row marked by TMDB recovery")
	}
	if tr.Get(ctx, tmdbCursor) != "2026-10-21" {
		t.Fatal("cursor not reset")
	}
}

func packet(t *testing.T) []byte {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	add := func(name, body string) {
		w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body))})
		w.Write([]byte(body))
	}
	// Data first: the parser must not depend on file order.
	add("mbdump/dbmirror_pendingdata", "1\tt\t\"id\"='5' \n1\tf\t\"id\"='5' \"gid\"='11111111-1111-1111-1111-111111111111' \"name\"='A' \n2\tf\t\"id\"='6' \"gid\"='22222222-2222-2222-2222-222222222222' \n")
	add("mbdump/dbmirror_pending", "1\t\"musicbrainz\".\"artist\"\tu\t1\n2\t\"musicbrainz\".\"editor\"\tu\t1\n")
	w.Close()
	return buf.Bytes()
}

func TestMBFeedAppliesPacketsAndIsOffWithoutToken(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tr := openTracker(t, &now)
	tr.Record(ctx, "artist", `{}`, "musicbrainz:11111111-1111-1111-1111-111111111111")
	tr.Record(ctx, "other", `{}`, "musicbrainz:22222222-2222-2222-2222-222222222222")
	var seenToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenToken = r.URL.Query().Get("token")
		if r.URL.Path == "/info" {
			fmt.Fprint(w, `{"last_packet":"replication-101.tar.bz2"}`)
			return
		}
		w.Write(packet(t))
	}))
	defer srv.Close()
	off := &MBFeed{Tracker: tr, HTTP: srv.Client(), InfoURL: srv.URL + "/info", PacketURL: srv.URL + "/p/%d", Now: time.Now}
	if n, err := off.Poll(ctx); n != 0 || err != nil || seenToken != "" {
		t.Fatal("feed ran without a token")
	}
	f := &MBFeed{Tracker: tr, HTTP: srv.Client(), Token: fakeMB, InfoURL: srv.URL + "/info", PacketURL: srv.URL + "/p/%d", Now: time.Now}
	tr.Set(ctx, "feed:musicbrainz", "failed earlier")
	if n, err := f.Poll(ctx); n != 0 || err != nil || tr.Get(ctx, mbCursor) != "101" || !strings.HasPrefix(tr.Get(ctx, "feed:musicbrainz"), "ok ") {
		t.Fatal("first poll should seed the cursor and mark the feed ok")
	}
	tr.Set(ctx, mbCursor, "100")
	n, err := f.Poll(ctx)
	if err != nil || n != 1 || seenToken != fakeMB {
		t.Fatalf("marked %d %v", n, err)
	}
	if _, stale, _ := tr.State(ctx, "other"); stale {
		t.Fatal("non-entity table change applied")
	}
	if tr.Get(ctx, mbCursor) != "101" {
		t.Fatal("cursor not advanced")
	}
}

func TestMBFeedErrorsCarryNoToken(t *testing.T) {
	now := time.Now()
	tr := openTracker(t, &now)
	f := &MBFeed{Tracker: tr, HTTP: &http.Client{Timeout: time.Second}, Token: fakeMB, InfoURL: "http://127.0.0.1:1/info", Now: time.Now}
	_, err := f.Poll(context.Background())
	if err == nil || strings.Contains(err.Error(), fakeMB) {
		t.Fatalf("error %v", err)
	}
}

func TestAgeTTL(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		age  time.Duration
		want time.Duration
	}{{24 * time.Hour, 3 * time.Hour}, {200 * 24 * time.Hour, 12 * time.Hour}, {3 * 365 * 24 * time.Hour, 7 * 24 * time.Hour}} {
		got := AgeTTL(now.Add(-c.age), now, 3*time.Hour, 12*time.Hour, 7*24*time.Hour)
		if got < c.want*9/10 || got > c.want*11/10 {
			t.Fatalf("age %v: %v", c.age, got)
		}
	}
	if got := AgeTTL(time.Time{}, now, time.Hour, 2*time.Hour, 10*time.Hour); got < 9*time.Hour {
		t.Fatalf("unknown release %v", got)
	}
}

func TestRefOf(t *testing.T) {
	for q, want := range map[model.Query]string{
		{Provider: "tmdb", Operation: "detail", Kind: "movie", ID: "42"}:                     "tmdb:movie:42",
		{Provider: "tmdb", Operation: "season", Kind: "show", ID: "7", Season: 1}:            "tmdb:tv:7",
		{Provider: "tmdb", Operation: model.OperationTMDB, Path: "/person/5/images"}:         "tmdb:person:5",
		{Provider: "tmdb", Operation: model.OperationTMDB, Path: "/movie/5/watch/providers"}: "",
		{Provider: "tmdb", Operation: model.OperationTMDB, Path: "/trending/all/day"}:        "",
		{Provider: "musicbrainz", Operation: "detail", Kind: "artist", ID: "abc"}:            "musicbrainz:abc",
	} {
		if got := RefOf(q); got != want {
			t.Fatalf("%+v: %q", q, got)
		}
	}
}

type revs struct{ n int }

func (r *revs) CheckRevisions(context.Context) (int, error) { r.n++; return 0, nil }

func TestRunnerRefetchesWithinBudgetBackstopsEvictsAndWarns(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tr := openTracker(t, &now)
	start := now
	now = start.Add(-200 * 24 * time.Hour)
	tr.Record(ctx, "forgotten", `{}`, "")
	now = start.Add(-20 * 24 * time.Hour)
	tr.Record(ctx, "old", `{}`, "")
	now = start
	for i := range 5 {
		tr.Record(ctx, fmt.Sprint("hot", i), `{}`, fmt.Sprint("tmdb:movie:", i))
	}
	tr.MarkRefs(ctx, []string{"tmdb:movie:0", "tmdb:movie:1", "tmdb:movie:2"})
	var refetched []string
	var evicted []string
	r := &Runner{Tracker: tr, Budget: 3, Now: func() time.Time { return now }, DiskPath: t.TempDir(), DiskWarning: 1e-9, Logf: func(string, ...any) {},
		Refetch: func(_ context.Context, it Item) error {
			refetched = append(refetched, it.Key)
			return tr.Record(ctx, it.Key, it.Query, it.Ref)
		},
		Evict: func(_ context.Context, keys []string) error { evicted = append(evicted, keys...); return nil }}
	if n := r.Once(ctx); n != 3 {
		t.Fatalf("refetched %d", n)
	}
	if strings.Join(refetched, ",") != "hot0,hot1,hot2" && len(refetched) != 3 {
		t.Fatalf("order %v", refetched)
	}
	refetched = nil
	r.Once(ctx)
	if len(refetched) != 1 || refetched[0] != "old" {
		t.Fatalf("backstop %v", refetched)
	}
	if len(evicted) != 1 || evicted[0] != "forgotten" {
		t.Fatalf("evicted %v", evicted)
	}
	st := r.Snapshot(ctx)
	if len(st.Warnings) == 0 || st.Disk == nil || st.Feeds["musicbrainz"] != "disabled_no_token" {
		t.Fatalf("snapshot %+v", st)
	}
	r.EvictAfter = -1
	evicted = nil
	now = now.Add(365 * 24 * time.Hour)
	r.Once(ctx)
	if len(evicted) != 0 {
		t.Fatal("eviction ran while off")
	}
}

func TestRunnerStopsAtSpentBudget(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	tr := openTracker(t, &now)
	for i := range 3 {
		tr.Record(ctx, fmt.Sprint(i), `{}`, fmt.Sprint("tmdb:tv:", i))
	}
	tr.MarkRefs(ctx, []string{"tmdb:tv:0", "tmdb:tv:1", "tmdb:tv:2"})
	calls := 0
	r := &Runner{Tracker: tr, Logf: func(string, ...any) {}, Refetch: func(context.Context, Item) error { calls++; return errors.New("upstream_budget") }}
	r.Once(ctx)
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
}

// fakeMB stands in for the MetaBrainz access value in tests.
var fakeMB = strings.Repeat("z", 12)
