package lists

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// brand is built so this file itself does not trip a brand grep.
var brand = strings.ToLower("Spot" + "ify")

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseFixtures(t *testing.T) {
	songs, err := ParseChart(strings.NewReader(string(readFixture(t, "t200.json"))))
	if err != nil || len(songs) != 60 {
		t.Fatalf("songs %d %v", len(songs), err)
	}
	if songs[0].Rank != 1 || songs[0].Title == "" || songs[0].Artist == "" || songs[0].Streams <= 0 || len(songs[0].Image) != 40 {
		t.Fatalf("song row %+v", songs[0])
	}
	albums, _ := ParseChart(strings.NewReader(string(readFixture(t, "alb.json"))))
	if len(albums) != 60 || albums[0].ExtAlbum == "" || albums[0].AlbumType != "album" {
		t.Fatalf("albums %+v", albums[0])
	}
	artists, _ := ParseChart(strings.NewReader(string(readFixture(t, "art.json"))))
	if len(artists) != 40 || artists[0].Artist == "" || artists[0].Title != "" {
		t.Fatalf("artists %+v", artists[0])
	}
	nr, err := ParseNew(strings.NewReader(string(readFixture(t, "newrel.json"))))
	if err != nil || len(nr) != 20 || nr[0].ReleaseDate == "" || nr[0].AlbumType == "" {
		t.Fatalf("new %d %+v %v", len(nr), nr, err)
	}
}

func TestPlanFitsFreeCapAndGrowsOnPro(t *testing.T) {
	free := Plan(900)
	if c := PlanCost(free); c > 810 || c < 600 {
		t.Fatalf("free plan cost %d", c)
	}
	// Daily song charts for the top countries come first.
	for i, r := range TopRegions[:firstWave] {
		if free[i].Kind != KindSongs || free[i].Region != r || free[i].Every != daily {
			t.Fatalf("slot %d = %+v, want daily songs %s", i, free[i], r)
		}
	}
	pro := Plan(6000)
	if len(pro) <= len(free)*2 || PlanCost(pro) > 5400 {
		t.Fatalf("pro plan %d sources, cost %d", len(pro), PlanCost(pro))
	}
	all := map[string]bool{}
	for _, s := range pro {
		all[s.Key] = true
	}
	for _, r := range TopRegions {
		if !all["songs:"+r] {
			t.Fatalf("pro plan misses daily chart of %s", r)
		}
	}
}

// fakeSource serves the fixtures and counts calls.
type fakeSource struct {
	mu    sync.Mutex
	n     int
	ua    string
	key   string
	limit bool
	// quota, when set, is the period limit reported in the headers, with
	// used calls spent before the test.
	quota, used int
}

func (f *fakeSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.n++
	f.ua, f.key = r.Header.Get("User-Agent"), r.Header.Get("x-rapidapi-key")
	limit := f.limit
	if f.quota > 0 {
		w.Header().Set("x-ratelimit-requests-limit", fmt.Sprint(f.quota))
		w.Header().Set("x-ratelimit-requests-remaining", fmt.Sprint(f.quota-f.used-f.n))
		w.Header().Set("x-ratelimit-requests-reset", "2300000")
	}
	f.mu.Unlock()
	if limit {
		w.WriteHeader(429)
		return
	}
	name := map[string]string{"/top_200_tracks": "t200.json", "/top_albums": "alb.json", "/top_artists": "art.json", "/browse/new-releases": "new.json", "/playlist_tracks": "playlist.json"}[r.URL.Path]
	if strings.HasPrefix(r.URL.Path, "/browse/categories/") {
		name = "category.json"
	}
	if name == "new.json" {
		name = "newrel.json"
	}
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		w.WriteHeader(404)
		return
	}
	_, _ = w.Write(b)
}

// fakeSearch answers every name with a stable made-up MBID.
type fakeSearch struct{ n int }

func fakeID(s string) string {
	h := fnv.New128a()
	h.Write([]byte(s))
	b := h.Sum(nil)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (f *fakeSearch) Search(_ context.Context, kind, name, artist string) ([]musictrend.Candidate, error) {
	f.n++
	if kind == "artist" {
		return []musictrend.Candidate{{MBID: fakeID("artist|" + name), Name: name}}, nil
	}
	return []musictrend.Candidate{{MBID: fakeID("album|" + name + "|" + artist), Name: name, Artists: []string{artist}, ArtistMBID: fakeID("artist|" + artist)}}, nil
}

func openTest(t *testing.T, src *fakeSource, o Options) (*Service, *[]string) {
	t.Helper()
	srv := httptest.NewServer(src)
	t.Cleanup(srv.Close)
	var logs []string
	o.BaseURL, o.Key = srv.URL, "test-key"
	o.Pace, o.MatchPace = -1, -1
	if o.Search == nil {
		o.Search = &fakeSearch{}
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC) }
	}
	o.Log = func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	s, err := Open(filepath.Join(t.TempDir(), "lists.sqlite"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, &logs
}

func TestRefreshCapsSkipsUnchangedAndServesShelves(t *testing.T) {
	src := &fakeSource{}
	var queued []string
	s, logs := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, MatchMax: 5000, Prefetch: func(kind, id string) { queued = append(queued, kind+":"+id) }})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if src.n != len(s.Sources()) || !strings.Contains(src.ua, "Mozilla/5.0") || src.key != "test-key" {
		t.Fatalf("calls %d of %d, ua %q", src.n, len(s.Sources()), src.ua)
	}
	m, d := s.Used()
	if m != src.n || d != src.n {
		t.Fatalf("used %d/%d, calls %d", m, d, src.n)
	}
	// Nothing is due again the same day: no calls.
	before := src.n
	_ = s.Refresh(context.Background())
	if src.n != before {
		t.Fatalf("refetched %d", src.n-before)
	}
	body, etag, ok := s.Shelves("US")
	if !ok || etag == "" {
		t.Fatal("no shelves")
	}
	var sh Shelves
	if err := json.Unmarshal(body, &sh); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"top_now", "albums_week", "artists_week", "climbing", "new_friday", "singles_artists", "long_runners"} {
		if len(sh.Blocks[name]) == 0 {
			t.Fatalf("block %s empty", name)
		}
	}
	if strings.Contains(strings.ToLower(string(body)), brand) || strings.Contains(string(body), "ext_") || strings.Contains(string(body), "scdn") {
		t.Fatal("brand or source ids in shelves JSON")
	}
	for _, l := range *logs {
		if strings.Contains(strings.ToLower(l), brand) || strings.Contains(l, "test-key") {
			t.Fatalf("log leaks: %q", l)
		}
	}
	if len(queued) == 0 {
		t.Fatal("nothing queued for pictures")
	}
	// Unknown region: global shelves.
	if _, e2, ok := s.Shelves("zz"); !ok || e2 == "" {
		t.Fatal("no fallback shelves")
	}
	st := s.Status(context.Background())
	if st.UsedMonth != src.n || st.MonthCap != 900 || len(st.Regions) == 0 {
		t.Fatalf("status %+v", st)
	}
}

func TestMonthCapStopsCalls(t *testing.T) {
	src := &fakeSource{}
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 3})
	_ = s.Refresh(context.Background())
	if src.n != 3 {
		t.Fatalf("day cap: %d calls", src.n)
	}
	// The first calls are the top daily charts.
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM lists_source WHERE key IN ('songs:global', 'songs:us', 'songs:gb')`).Scan(&n)
	if n != 3 {
		t.Fatalf("first sources were not the top daily charts: %d", n)
	}
	src2 := &fakeSource{}
	s2, _ := openTest(t, src2, Options{MonthCap: 300, DayCap: 1000})
	// 298 calls already spent earlier this month.
	_, _ = s2.db.Exec(`INSERT INTO lists_calls (day, n) VALUES ('2026-10-02', 298)`)
	_ = s2.Refresh(context.Background())
	if src2.n != 2 {
		t.Fatalf("month cap: %d calls", src2.n)
	}
}

func TestRateLimitBacksOff(t *testing.T) {
	src := &fakeSource{limit: true}
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 100})
	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("no error on 429")
	}
	if src.n != 1 {
		t.Fatalf("kept calling after 429: %d", src.n)
	}
	_ = s.Refresh(context.Background())
	if src.n != 1 {
		t.Fatal("no back-off")
	}
}

func TestStoreSkipsUnchangedHash(t *testing.T) {
	s, _ := openTest(t, &fakeSource{}, Options{})
	src := Source{Key: "songs:us", Kind: KindSongs, Region: "us", Every: daily}
	if !s.store(src, []byte(`[1]`)) {
		t.Fatal("first store not a change")
	}
	if s.store(src, []byte(`[1]`)) {
		t.Fatal("same hash counted as change")
	}
	if !s.store(src, []byte(`[2]`)) {
		t.Fatal("new hash not a change")
	}
}

func TestBuildSongsToAlbumsAndArtists(t *testing.T) {
	look := func(kind, name, artist string) (string, string, bool) {
		if kind == "album" && name != "Big Album" {
			return "", "", false // no single in MusicBrainz
		}
		if kind == "artist" {
			return fakeID("artist|" + name), "", true
		}
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	now := time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)
	in := Inputs{Now: now, Lookup: look, Lists: map[string][]Entry{
		"albums:us": {{Rank: 1, Title: "Big Album", Artist: "A", Image: "img1", ReleaseDate: "2026-10-20"}},
		"songs:us": {
			{Rank: 1, Title: "s1", Artist: "A", Image: "img1", Prev: 3, Streams: 100},
			{Rank: 2, Title: "s2", Artist: "A", Image: "img1", Prev: 2, Streams: 50},
			{Rank: 3, Title: "s3", Artist: "A", Image: "img1", Prev: 1, Streams: 10},
			{Rank: 4, Title: "single", Artist: "B", Image: "imgX", Status: "new"},
		},
	}}
	sh := Build("us", in)
	top := sh.Blocks["top_now"]
	if len(top) != 2 || top[0].Kind != "album" || top[0].Title != "Big Album" || top[0].Streams != 160 {
		t.Fatalf("top %+v", top)
	}
	if top[1].Kind != "artist" || top[1].Song != "single" {
		t.Fatalf("single not an artist: %+v", top[1])
	}
	if len(sh.Blocks["climbing"]) != 1 || sh.Blocks["climbing"][0].M <= 0 {
		t.Fatalf("climbing %+v", sh.Blocks["climbing"])
	}
	if len(sh.Blocks["new_entries"]) == 0 || len(sh.Blocks["big_new_albums"]) != 1 {
		t.Fatalf("new %+v", sh.Blocks)
	}
	// Unmatched names are left out.
	in.Lookup = func(string, string, string) (string, string, bool) { return "", "", false }
	if !Build("us", in).Empty() {
		t.Fatal("unmatched items shown")
	}
}

func TestScores(t *testing.T) {
	if p := pop(1, 200, 0, 0); p != 1 {
		t.Fatal(p)
	}
	if p := pop(200, 200, 0, 0); p != 0.005 {
		t.Fatal(p)
	}
	if m := momentum(Entry{Rank: 10, Prev: 20}, 50, 200); m <= 0 || m > 1 {
		t.Fatal(m)
	}
	if m := momentum(Entry{Rank: 10, Prev: 5}, 5, 200); m != 0 {
		t.Fatal(m)
	}
	if m := momentum(Entry{Rank: 20, Status: "new"}, 0, 200); m != 0.9 {
		t.Fatal(m)
	}
}

func TestShelvesFillWhileMatching(t *testing.T) {
	src := &fakeSource{}
	srch := &fakeSearch{}
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, MatchMax: matchBatch, Search: srch})
	_ = s.Refresh(context.Background())
	if srch.n != matchBatch {
		t.Fatalf("searches %d", srch.n)
	}
	if _, _, ok := s.Shelves("us"); !ok {
		t.Fatal("no shelves after the first batch")
	}
}

func TestFailedSourceWaitsBeforeRetry(t *testing.T) {
	src := &fakeSource{}
	now := time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, Now: func() time.Time { return now }})
	bad := Source{Key: "songs:zz", Kind: KindSongs, Region: "zz", Every: daily}
	s.failed(bad)
	if s.due(bad) {
		t.Fatal("failed source due at once")
	}
	now = now.Add(failRetry)
	if !s.due(bad) {
		t.Fatal("failed source not due after the retry wait")
	}
}

// A new-release list one or two years old (the source's non-US countries)
// never shows as new: every region shows the US list, released within
// NewDays, and stale entries are dropped even from a stored list.
func TestStaleNewReleasesNeverShow(t *testing.T) {
	look := func(kind, name, artist string) (string, string, bool) {
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	in := Inputs{Now: now, Lookup: look, Lists: map[string][]Entry{
		"new:us": {
			{Rank: 1, Title: "This Friday", Artist: "A", ReleaseDate: "2026-10-23"},
			{Rank: 2, Title: "Last Year", Artist: "B", ReleaseDate: "2025-03-14"},
		},
		"new:gb": {{Rank: 1, Title: "Old GB", Artist: "C", ReleaseDate: "2025-03-07"}},
		"new:de": {{Rank: 1, Title: "Old DE", Artist: "D", ReleaseDate: "2024-05-10"}},
	}}
	for _, region := range []string{"us", "gb", "de", "jp"} {
		got := Build(region, in).Blocks["new_friday"]
		if len(got) != 1 || got[0].Title != "This Friday" {
			t.Fatalf("%s new_friday %+v", region, got)
		}
	}
	if f := FreshReleases(in.Lists["new:us"], now); len(f) != 1 || f[0].Title != "This Friday" {
		t.Fatalf("fresh %+v", f)
	}
	// ListenBrainz's fresh releases follow the US list, most listened
	// first; not out yet, too old or already shown: left out.
	in.Fresh = []FreshAlbum{
		{MBID: "00000000-0000-4000-8000-00000000000a", Title: "Quiet", Artist: "Q", Date: "2026-10-22", Listens: 3},
		{MBID: "00000000-0000-4000-8000-00000000000b", Title: "Loud (Deluxe Edition)", Artist: "L", Date: "2026-10-20", Listens: 90},
		{MBID: "00000000-0000-4000-8000-00000000000c", Title: "Soon", Artist: "S", Date: "2026-11-06", Listens: 500},
		{MBID: "00000000-0000-4000-8000-00000000000d", Title: "Old", Artist: "O", Date: "2026-09-01", Listens: 500},
		{MBID: fakeID("album|This Friday|A"), Title: "This Friday", Artist: "A", Date: "2026-10-23", Listens: 999},
	}
	for _, region := range []string{"us", "jp"} {
		got := Build(region, in).Blocks["new_friday"]
		if len(got) != 3 || got[0].Title != "This Friday" || got[1].Title != "Loud" || got[2].Title != "Quiet" || got[2].Rank != 3 || got[0].P != 1 || got[2].P >= got[1].P {
			t.Fatalf("%s new_friday with fresh %+v", region, got)
		}
	}
	for _, src := range Plan(900) {
		if src.Kind == KindNew && src.Region != "us" {
			t.Fatalf("%s still read", src.Key)
		}
	}
}

// A pre-order (dated after today, even tomorrow) never shows as new.
func TestPreOrdersNeverNew(t *testing.T) {
	look := func(kind, name, artist string) (string, string, bool) {
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	now := time.Date(2026, 10, 29, 22, 0, 0, 0, time.UTC)
	in := Inputs{Now: now, Lookup: look, Lists: map[string][]Entry{"new:us": {
		{Rank: 1, Title: "Tomorrow", Artist: "T", ReleaseDate: "2026-10-30"},
		{Rank: 2, Title: "Today", Artist: "A", ReleaseDate: "2026-10-29"},
	}}}
	got := Build("us", in).Blocks["new_friday"]
	if len(got) != 1 || got[0].Title != "Today" {
		t.Fatalf("new_friday %+v", got)
	}
	if f := FreshReleases(in.Lists["new:us"], now); len(f) != 1 || f[0].Title != "Today" {
		t.Fatalf("fresh %+v", f)
	}
}
