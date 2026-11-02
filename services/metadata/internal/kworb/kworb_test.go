package kworb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	bruno     = "0du5cEVh5yTK9QJze8zA0C"
	drake     = "3TVXtAsR1Inumwj472S9r4"
	ethelCain = "0avMDS4HyoCEP6RqZJWpY2"
	brunoMB   = "afb680f2-b6eb-4cd7-a70b-a63b25c763d5"
	drakeMB   = "9fff2f8a-21e6-47de-a2b8-7f449929d43f"
	ethelMB   = "677e31f1-4035-463c-8ca4-41175440c919"
	otherMB   = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
)

func TestParseListeners(t *testing.T) {
	rows, err := ParseListeners(fixture(t, "listeners.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 33 {
		t.Fatalf("%d rows", len(rows))
	}
	b := rows[0]
	if b.Rank != 1 || b.Name != "Bruno Mars" || b.Spotify != bruno || b.Listeners != 127664459 || b.Change != -287530 || b.Peak != 1 || b.PeakListeners != 151079821 {
		t.Fatalf("first row %+v", b)
	}
	if a := rows[30]; a.Name != "Alicia Keys" || a.Peak != 49 {
		t.Fatalf("a new-peak row %+v", a)
	}
	if last := rows[len(rows)-1]; last.Rank != 2500 || last.Listeners != 5996691 {
		t.Fatalf("cutoff row %+v", last)
	}
	if _, err := ParseListeners(strings.Replace(fixture(t, "listeners.html"), "<th>PkListeners</th>", "<th>Peak listeners</th>", 1)); err != ErrLayout {
		t.Fatalf("changed header: %v", err)
	}
	if _, err := ParseListeners(fixture(t, "notfound.html")); err != ErrLayout {
		t.Fatal("an error page read as listeners")
	}
}

func TestParseArtists(t *testing.T) {
	rows, err := ParseArtists(fixture(t, "artists.html"))
	if err != nil || len(rows) != 32 {
		t.Fatalf("%d %v", len(rows), err)
	}
	d := rows[0]
	if d.Rank != 1 || d.Name != "Drake" || d.Spotify != drake || d.Total != 143564800000 || d.Daily != 54206000 {
		t.Fatalf("first row %+v", d)
	}
	if _, err := ParseArtists(strings.Replace(fixture(t, "artists.html"), "<th>Daily</th>", "<th>Today</th>", 1)); err != ErrLayout {
		t.Fatalf("changed header: %v", err)
	}
}

func TestParseArtistSongsPage(t *testing.T) {
	p, err := ParseArtist(fixture(t, "ethelcain_songs.html"))
	if err != nil || p.Total != 1230851933 || p.Daily != 1348796 || p.AsOf != "2026-10-22" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := ParseArtist(strings.Replace(fixture(t, "ethelcain_songs.html"), "<th>Total</th>", "<th>All</th>", 1)); err != ErrLayout {
		t.Fatalf("changed table: %v", err)
	}
	if _, err := ParseArtist(fixture(t, "various_songs.html")); err != ErrNoTotals {
		t.Fatalf("various artists page: %v, want ErrNoTotals", err)
	}
	// A page with totals whose date reads "unknown" keeps its totals: a
	// layout alarm, not "no totals".
	if _, err := ParseArtist(strings.Replace(fixture(t, "ethelcain_songs.html"), "Last updated: 2026/10/22", "Last updated: unknown", 1)); err != ErrLayout {
		t.Fatalf("unknown date with totals: %v, want ErrLayout", err)
	}
	if _, err := ParseArtist(fixture(t, "notfound.html")); err != ErrLayout {
		t.Fatal("the 404 page read as an artist")
	}
}

func TestParseCharts(t *testing.T) {
	c, err := ParseChart(fixture(t, "global_daily.html"), false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Day != "2026-10-25" || len(c.Rows) != 32 {
		t.Fatalf("%s %d rows", c.Day, len(c.Rows))
	}
	r := c.Rows[0]
	if r.Rank != 1 || r.Move != "=" || r.Artist != "Tate McRae" || r.Title != "Housewife" || r.ArtistID != "45dkTj5sMRSjrmBSBeiHym" || r.TrackID != "2sFepQNyl1WjpROruP8bJ0" ||
		r.Days != 10 || r.Peak != 1 || r.Streams != 6254852 || r.StreamsChange != -640884 || r.Total != 60325757 {
		t.Fatalf("first row %+v", r)
	}
	if f := c.Rows[1]; f.Title != "BbY WOW" || len(f.Featured) != 2 || f.Featured[1] != "rusowsky" {
		t.Fatalf("featured %+v", f)
	}
	if last := c.Rows[len(c.Rows)-1]; last.Rank != 200 || last.Move != "RE" || last.StreamsChange != 0 {
		t.Fatalf("last row %+v", last)
	}
	w, err := ParseChart(fixture(t, "us_weekly.html"), true)
	if err != nil || w.Day != "2026-10-22" || len(w.Rows) != 32 {
		t.Fatalf("weekly %s %d %v", w.Day, len(w.Rows), err)
	}
	if _, err := ParseChart(fixture(t, "us_weekly.html"), false); err != ErrLayout {
		t.Fatal("a weekly page read as daily")
	}
}

// site fakes kworb, MusicBrainz's URL lookup and Wikidata's query service.
type site struct {
	mu        sync.Mutex
	pages     map[string]string // path: body
	status    map[string]int    // path: status
	mbLinks   map[string]string // Spotify id: MusicBrainz id
	wdLinks   map[string]string // Spotify id: MusicBrainz id
	hits      map[string]int
	uas       []string
	mbBusy    int // MusicBrainz answers 503 this many times first
	kworbHits int
}

func (f *site) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[r.URL.Path]++
	switch {
	case strings.HasPrefix(r.URL.Path, "/spotify/"):
		f.kworbHits++
		f.uas = append(f.uas, r.UserAgent())
		if st := f.status[r.URL.Path]; st != 0 {
			w.WriteHeader(st)
			return
		}
		b, ok := f.pages[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Last-Modified", "Mon, 26 Oct 2026 14:00:28 GMT")
		_, _ = w.Write([]byte(b))
	case r.URL.Path == "/ws/2/url":
		if f.mbBusy > 0 {
			f.mbBusy--
			w.WriteHeader(503)
			return
		}
		res := r.URL.Query()["resource"]
		var urls []map[string]any
		for _, u := range res {
			if mb := f.mbLinks[strings.TrimPrefix(u, spotifyPage)]; mb != "" {
				urls = append(urls, map[string]any{"resource": u, "relations": []any{map[string]any{"target-type": "artist", "artist": map[string]any{"id": mb}}}})
			}
		}
		if len(res) == 1 {
			if len(urls) == 0 {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(urls[0])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"urls": urls})
	case r.URL.Path == "/sparql":
		q := r.URL.Query().Get("query")
		var rows []any
		for sp, mb := range f.wdLinks {
			if strings.Contains(q, `"`+sp+`"`) || strings.Contains(q, `"`+mb+`"`) {
				rows = append(rows, map[string]any{"sp": map[string]string{"value": sp}, "mb": map[string]string{"value": mb}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": rows}})
	default:
		w.WriteHeader(404)
	}
}

func (f *site) set(path, body string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if body != "" {
		f.pages[path] = body
	}
	f.status[path] = status
}

type clock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
	rnd   float64
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (c *clock) random() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rnd += 0.37
	if c.rnd >= 1 {
		c.rnd--
	}
	return c.rnd
}

func newSite(t *testing.T) *site {
	return &site{pages: map[string]string{
		"/spotify/listeners.html":                      fixture(t, "listeners.html"),
		"/spotify/artists.html":                        fixture(t, "artists.html"),
		"/spotify/artist/" + ethelCain + "_songs.html": fixture(t, "ethelcain_songs.html"),
		"/spotify/country/global_daily.html":           fixture(t, "global_daily.html"),
		"/spotify/country/us_weekly.html":              fixture(t, "us_weekly.html"),
	}, status: map[string]int{}, mbLinks: map[string]string{bruno: brunoMB, drake: drakeMB}, wdLinks: map[string]string{}, hits: map[string]int{}}
}

func rig(t *testing.T, f *site, o Options) (*Service, *clock) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	c := &clock{now: time.Date(2026, 10, 26, 15, 30, 0, 0, time.UTC)}
	o.BaseURL, o.HTTP, o.UserAgent, o.MinRows = srv.URL+"/spotify", srv.Client(), "HolyArrsMetadata/test", 10
	o.MBHTTP, o.MBURL, o.WDHTTP, o.SPARQLURL = srv.Client(), srv.URL+"/ws/2", srv.Client(), srv.URL+"/sparql"
	o.Now, o.sleep, o.rnd = c.Now, c.sleep, c.random
	s, err := Open(filepath.Join(t.TempDir(), "k.sqlite"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, c
}

func TestListsKeepLastGoodCopyAndAlert(t *testing.T) {
	f := newSite(t)
	s, _ := rig(t, f, Options{})
	ctx := context.Background()
	if err := s.RefreshListeners(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshArtists(ctx); err != nil {
		t.Fatal(err)
	}
	if st := s.Stats(); st.Listeners != 33 || st.Streamed != 32 || st.AsOf != "2026-10-26" || st.Alert != "" {
		t.Fatalf("status %+v", st)
	}
	// A changed layout, a 503 and a short page keep the kept copy; a
	// layout error raises the alert.
	f.set("/spotify/listeners.html", strings.Replace(fixture(t, "listeners.html"), "<th>Listeners</th>", "<th>Monthly</th>", 1), 0)
	if err := s.RefreshListeners(ctx); err == nil {
		t.Fatal("a changed layout was taken")
	}
	if st := s.Stats(); st.Listeners != 33 || st.Alert == "" || st.LayoutErrors != 1 {
		t.Fatalf("after a layout change %+v", st)
	}
	f.set("/spotify/listeners.html", "", 503)
	if err := s.RefreshListeners(ctx); err == nil || s.Stats().Listeners != 33 {
		t.Fatal("a 503 was taken")
	}
	f.set("/spotify/listeners.html", fixture(t, "listeners.html")[:strings.Index(fixture(t, "listeners.html"), "<tr><td>3</td>")]+"</tbody></table>", 0)
	if err := s.RefreshListeners(ctx); err == nil || s.Stats().Listeners != 33 {
		t.Fatal("a short page was taken")
	}
	if p := s.Popularity(brunoMB); p == nil || p.MonthlyListeners != 127664459 {
		t.Fatalf("figures lost after bad reads: %+v", p)
	}
	// It reads again: the alert clears.
	f.set("/spotify/listeners.html", fixture(t, "listeners.html"), 0)
	if err := s.RefreshListeners(ctx); err != nil || s.Stats().Alert != "" {
		t.Fatalf("alert not cleared: %v %+v", err, s.Stats())
	}
	for _, ua := range f.uas {
		if ua != "HolyArrsMetadata/test" {
			t.Fatalf("user agent %q", ua)
		}
	}
}

func TestChartFailureKeepsLastGoodAndMarksFailed(t *testing.T) {
	f := newSite(t)
	s, _ := rig(t, f, Options{Regions: []string{"global", "us"}})
	ctx := context.Background()
	if err := s.RefreshChart(ctx, "global", false); err != nil {
		t.Fatal(err)
	}
	st, ok := s.SongChart("global", false)
	if !ok || st.Failed || len(st.Chart.Rows) != 32 {
		t.Fatalf("%+v", st)
	}
	f.set("/spotify/country/global_daily.html", strings.Replace(fixture(t, "global_daily.html"), "<th>Days</th>", "<th>Run</th>", 1), 0)
	if err := s.RefreshChart(ctx, "global", false); err == nil {
		t.Fatal("a changed chart was taken")
	}
	st, _ = s.SongChart("global", false)
	if !st.Failed || len(st.Chart.Rows) != 32 || s.Stats().Alert == "" {
		t.Fatalf("after a layout change %+v %+v", st, s.Stats())
	}
	// No page for the region: failed, nothing kept.
	if err := s.RefreshChart(ctx, "us", false); err == nil {
		t.Fatal("a 404 chart read")
	}
	if st, ok := s.SongChart("us", false); !ok || !st.Failed || len(st.Chart.Rows) != 0 {
		t.Fatalf("missing chart %+v", st)
	}
	if err := s.RefreshChart(ctx, "us", true); err != nil {
		t.Fatal(err)
	}
}

func TestPaceTwoSeconds(t *testing.T) {
	f := newSite(t)
	s, c := rig(t, f, Options{Gap: time.Second}) // raised to 2 s
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, _, _, _, _, err := s.page(ctx, "/listeners.html", "", false); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.waits) != 2 || c.waits[0] < 2*time.Second || c.waits[1] < 2*time.Second {
		t.Fatalf("waits %v", c.waits)
	}
}

func TestDailyCaps(t *testing.T) {
	f := newSite(t)
	s, _ := rig(t, f, Options{})
	ctx := context.Background()
	_, _ = s.db.Exec(`INSERT INTO spent (day, pages, crawl) VALUES (?, ?, ?)`, s.today(), dayCap-1, crawlCap)
	// The crawl's own cap is spent: no songs page, but one list page fits.
	if _, _, _, _, _, err := s.page(ctx, "/artist/"+ethelCain+"_songs.html", "", true); err != errCap {
		t.Fatalf("crawl past its cap: %v", err)
	}
	if err := s.RefreshListeners(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshArtists(ctx); err != errCap {
		t.Fatalf("a page past the day's cap: %v", err)
	}
	if f.kworbHits != 1 {
		t.Fatalf("%d pages read", f.kworbHits)
	}
	// The crawl waits for the next day.
	if err := s.runJob(ctx, "crawl"); err != errCap {
		t.Fatalf("crawl %v", err)
	}
	if at, _ := s.dueAt("crawl"); at.UTC().Day() != 27 {
		t.Fatalf("crawl due %v, want the next day", at)
	}
}

func TestCrawlSpreadAndNeverOnTheHour(t *testing.T) {
	f := newSite(t)
	s, c := rig(t, f, Options{})
	ctx := context.Background()
	var prev time.Time
	for i := 0; i < 200; i++ {
		_, _ = s.nextJob()
		if err := s.runJob(ctx, "crawl"); err != nil {
			t.Fatal(err)
		}
		at, _ := s.dueAt("crawl")
		if at.Minute() == 0 {
			t.Fatalf("crawl on the hour: %v", at)
		}
		if gap := at.Sub(c.Now()); gap < crawlMin || gap > crawlMin+crawlSpread+time.Hour {
			t.Fatalf("crawl gap %v", gap)
		}
		if !prev.IsZero() && !at.After(prev) {
			t.Fatal("crawl times do not move on")
		}
		prev = at
		c.add(at.Sub(c.Now()))
	}
	// Daily jobs too: never on the hour.
	for job := range s.jobs() {
		if at, ok := s.dueAt(job); ok && at.Minute() == 0 {
			t.Fatalf("%s on the hour", job)
		}
	}
}

func TestNoFetchOnAskAndOpenedFirst(t *testing.T) {
	f := newSite(t)
	links := map[string][]string{ethelMB: {"https://open.spotify.com/artist/" + ethelCain}, otherMB: {"https://open.spotify.com/artist/1111111111111111111111"}}
	s, _ := rig(t, f, Options{StoredLinks: func(_ context.Context, mbid string) []string { return links[mbid] },
		Catalog: func() []string { return []string{otherMB, ethelMB} }})
	ctx := context.Background()
	// Opening an artist reads nothing: no figures yet, the field is left out.
	s.Opened(ethelMB)
	if p := s.Popularity(ethelMB); p != nil || f.kworbHits != 0 {
		t.Fatalf("asked: %+v, %d pages", p, f.kworbHits)
	}
	// The next crawl reads the opened artist first, then the catalog's.
	if err := s.crawlOne(ctx); err != nil {
		t.Fatal(err)
	}
	if f.hits["/spotify/artist/"+ethelCain+"_songs.html"] != 1 || f.kworbHits != 1 {
		t.Fatalf("crawl order %v", f.hits)
	}
	p := s.Popularity(ethelMB)
	if p == nil || p.TotalPlays != 1230851933 || p.AsOf != "2026-10-22" || p.MonthlyListeners != 0 {
		t.Fatalf("crawled figures %+v", p)
	}
	// The next one has no kworb page: nothing shown, not read again this week.
	if err := s.crawlOne(ctx); err != nil {
		t.Fatal(err)
	}
	if p := s.Popularity(otherMB); p != nil || s.Stats().NoPage != 1 || s.pageDue("1111111111111111111111") {
		t.Fatalf("no page: %+v", p)
	}
	n := f.kworbHits
	if err := s.crawlOne(ctx); err != nil || f.kworbHits != n {
		t.Fatal("a page read twice in the week")
	}
}

func TestCrawlSkipsListedArtistsAndAsksWikidata(t *testing.T) {
	f := newSite(t)
	f.wdLinks[ethelCain] = ethelMB
	s, _ := rig(t, f, Options{Catalog: func() []string { return []string{brunoMB, ethelMB} }})
	ctx := context.Background()
	if err := s.RefreshListeners(ctx); err != nil {
		t.Fatal(err)
	}
	q := s.crawlTargets(ctx)
	if len(q) != 1 || q[0] != ethelCain {
		t.Fatalf("line-up %v (the listed artist is skipped; the other found by Wikidata)", q)
	}
}

func TestMatchingByLinksOnly(t *testing.T) {
	f := newSite(t)
	rows, _ := ParseListeners(f.pages["/spotify/listeners.html"])
	f.wdLinks[rows[1].Spotify] = otherMB
	third := "c8b03190-306c-4120-bb0b-6f2ebfc06ea9"
	s, _ := rig(t, f, Options{KeptArtist: func(sp string) (string, bool) { return third, sp == rows[2].Spotify }})
	ctx := context.Background()
	if err := s.RefreshListeners(ctx); err != nil {
		t.Fatal(err)
	}
	for sp, want := range map[string]string{bruno: brunoMB, rows[1].Spotify: otherMB, rows[2].Spotify: third, rows[3].Spotify: ""} {
		if got := s.pairBySpotify(sp); got != want {
			t.Fatalf("%s paired with %q, want %q", sp, got, want)
		}
	}
	var how string
	_ = s.db.QueryRow(`SELECT how FROM pairs WHERE spotify = ?`, rows[1].Spotify).Scan(&how)
	if how != "wikidata" || f.hits["/ws/2/url"] != 1 {
		t.Fatalf("how %q, musicbrainz calls %d", how, f.hits["/ws/2/url"])
	}
	if _, err := s.matchSpotify(ctx, []string{rows[3].Spotify}); err != nil || f.hits["/ws/2/url"] != 1 {
		t.Fatal("an unmatched id was asked again within the week")
	}
}

func TestBusyMusicBrainzAskedOnceMore(t *testing.T) {
	f := newSite(t)
	f.mbBusy = 1
	s, c := rig(t, f, Options{})
	if err := s.RefreshListeners(context.Background()); err != nil {
		t.Fatal(err)
	}
	rested := false
	for _, w := range c.waits {
		rested = rested || w == mbBusyWait
	}
	if s.pairBySpotify(bruno) != brunoMB || !rested {
		t.Fatalf("pair %q, waits %v", s.pairBySpotify(bruno), c.waits)
	}
	f2 := newSite(t)
	f2.mbBusy = 2
	s2, _ := rig(t, f2, Options{})
	if err := s2.RefreshListeners(context.Background()); err == nil || s2.triedRecently("sp:"+bruno) {
		t.Fatal("a busy MusicBrainz was taken as no link")
	}
}

func TestPopularityFieldsAndRising(t *testing.T) {
	f := newSite(t)
	rows, _ := ParseListeners(f.pages["/spotify/listeners.html"])
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Change > rows[j].Change })
	f.wdLinks[rows[0].Spotify] = otherMB
	f.wdLinks[rows[1].Spotify] = ethelMB
	s, _ := rig(t, f, Options{})
	ctx := context.Background()
	if err := s.RefreshListeners(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshArtists(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s.Popularity(brunoMB))
	if string(b) != `{"monthly_listeners":127664459,"world_rank":1,"listeners_change_today":-287530,"as_of":"2026-10-26"}` {
		t.Fatalf("top artist %s", b)
	}
	// An artist only on the most streamed page gets its total plays.
	s.keepPair("4RnBFZRiMLRyZy0AzzTg2C", "89aa5ecb-59ad-46f5-b3eb-2d424e941f19", "test")
	b, _ = json.Marshal(s.Popularity("89aa5ecb-59ad-46f5-b3eb-2d424e941f19"))
	if string(b) != `{"total_plays":1423700000,"as_of":"2026-10-26"}` {
		t.Fatalf("streamed artist %s", b)
	}
	if s.Popularity("11111111-1111-4111-8111-111111111111") != nil {
		t.Fatal("figures for an artist with no known Spotify id")
	}
	body, ok := s.Rising()
	var r struct {
		AsOf  string  `json:"as_of"`
		Items []Riser `json:"items"`
	}
	if !ok || json.Unmarshal(body, &r) != nil || r.AsOf != "2026-10-26" {
		t.Fatalf("rising %s", body)
	}
	// The two linked gainers and Drake (matched by MusicBrainz); every
	// other gainer is not matched and left out.
	if len(r.Items) != 3 || r.Items[0].MBID != otherMB || r.Items[0].Name != rows[0].Name || r.Items[0].Rank != rows[0].Rank || r.Items[1].MBID != ethelMB || r.Items[2].MBID != drakeMB {
		t.Fatalf("rising %+v", r.Items)
	}
}

func TestKillSwitch(t *testing.T) {
	f := newSite(t)
	s, _ := rig(t, f, Options{On: func() bool { return false }, Regions: []string{"global"}})
	ctx := context.Background()
	for job := range s.jobs() {
		_ = s.runJob(ctx, job)
	}
	_ = s.runJob(ctx, "crawl")
	if f.kworbHits != 0 || s.Stats().On {
		t.Fatal("read while switched off")
	}
}
