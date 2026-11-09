package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock that only moves when someone sleeps on it.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	// slots are the moments callers were let through, in call order.
	slots []time.Time
}

func newClock() *fakeClock { return &fakeClock{now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *fakeClock) SleepUntil(ctx context.Context, t time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.After(c.now) {
		c.now = t
	}
	c.slots = append(c.slots, t)
	return nil
}

// memCache is a Cache in memory.
type memCache struct {
	mu   sync.Mutex
	rows map[string]memRow
}

type memRow struct {
	body    []byte
	expires time.Time
}

func (m *memCache) Get(_ context.Context, key string, now time.Time) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[key]
	if !ok || !now.Before(r.expires) {
		return nil, false, nil
	}
	return r.body, true, nil
}

func (m *memCache) Put(_ context.Context, key string, body []byte, _, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]memRow{}
	}
	m.rows[key] = memRow{body: body, expires: expires}
	return nil
}

const (
	mbArtistID = "11111111-1111-1111-1111-111111111111"
	mbAlbumA   = "22222222-2222-2222-2222-222222222222"
	mbAlbumB   = "33333333-3333-3333-3333-333333333333"
	mbLive     = "44444444-4444-4444-4444-444444444444"
	mbRel1     = "55555555-5555-5555-5555-555555555551"
	mbRel2     = "55555555-5555-5555-5555-555555555552"
)

// fakeMB is a fake MusicBrainz server. It records each request with the
// fake clock's time and can answer 503 a set number of times first.
type fakeMB struct {
	clock *fakeClock
	mu    sync.Mutex
	hits  []string
	at    []time.Time
	ua    []string
	busy  int    // answer 503 this many times
	after string // Retry-After on a 503
}

func (f *fakeMB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits = append(f.hits, r.URL.Path+"?"+r.URL.RawQuery)
	f.at = append(f.at, f.clock.Now())
	f.ua = append(f.ua, r.UserAgent())
	busy := f.busy > 0
	if busy {
		f.busy--
	}
	f.mu.Unlock()
	if busy {
		if f.after != "" {
			w.Header().Set("Retry-After", f.after)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	if q.Get("fmt") != "json" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/ws/2/artist" && q.Get("query") != "":
		if strings.Contains(strings.ToLower(q.Get("query")), "lab") {
			fmt.Fprintf(w, `{"artists":[{"id":%q,"name":"Lab Band","sort-name":"Band, Lab","type":"Group","country":"GB","score":100,"disambiguation":"test band","life-span":{"ended":false}},
				{"id":"not-an-id","name":"Broken"}]}`, mbArtistID)
			return
		}
		fmt.Fprint(w, `{"artists":[]}`)
	case r.URL.Path == "/ws/2/artist/"+mbArtistID && q.Get("inc") == "genres artist-rels":
		fmt.Fprintf(w, `{"id":%q,"name":"Lab Band","genres":[{"name":"Rock","count":2},{"name":"art pop","count":5},{"name":"zero","count":0}],
			"relations":[{"type":"member of band","artist":{"id":%q,"name":"Singer"}},{"type":"tribute","artist":{"id":%q,"name":"Cover Band"}},{"type":"collaboration","artist":{"id":"bad","name":"Bad"}}]}`, mbArtistID, mbAlbumA, mbAlbumB)
	case r.URL.Path == "/ws/2/release-group" && strings.Contains(q.Get("query"), "secondarytype:soundtrack"):
		fmt.Fprintf(w, `{"release-groups":[{"id":%q,"title":"Lab Movie (Original Soundtrack)","score":100,"first-release-date":"2020-05-01"},
			{"id":%q,"title":"Lab Movie 2","score":100,"first-release-date":"1990"},{"id":%q,"title":"Other","score":100,"first-release-date":"2020"}]}`, mbAlbumA, mbAlbumB, mbLive)
	case r.URL.Path == "/ws/2/artist/"+mbArtistID:
		fmt.Fprintf(w, `{"id":%q,"name":"Lab Band","sort-name":"Band, Lab","type":"Group","life-span":{"ended":true}}`, mbArtistID)
	case r.URL.Path == "/ws/2/release-group" && q.Get("artist") == mbArtistID:
		// Two pages: the server sends two albums, then the third.
		if q.Get("offset") == "0" {
			fmt.Fprintf(w, `{"release-group-count":3,"release-groups":[
				{"id":%q,"title":"First Light","primary-type":"Album","secondary-types":[],"first-release-date":"2020-03-06"},
				{"id":%q,"title":"Side Step","primary-type":"EP","secondary-types":[],"first-release-date":"2021"}]}`, mbAlbumA, mbAlbumB)
			return
		}
		fmt.Fprintf(w, `{"release-group-count":3,"release-groups":[
			{"id":%q,"title":"Live at the Lab","primary-type":"Album","secondary-types":["Live"],"first-release-date":"2022-05","disambiguation":"tour"}]}`, mbLive)
	case r.URL.Path == "/ws/2/release" && q.Get("release-group") == mbAlbumA:
		if q.Get("inc") != "media recordings" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"release-count":2,"releases":[
			{"id":%q,"title":"First Light","status":"Official","country":"XW","date":"2020-03-06","media":[
				{"position":1,"format":"Digital Media","tracks":[
					{"id":"aaaaaaaa-0000-0000-0000-000000000001","position":1,"title":"Dawn","length":200000,"recording":{"id":"bbbbbbbb-0000-0000-0000-000000000001"}},
					{"id":"aaaaaaaa-0000-0000-0000-000000000002","position":2,"title":"Noon","length":null,"recording":{"id":"bbbbbbbb-0000-0000-0000-000000000002","length":180000}}]}]},
			{"id":%q,"title":"First Light","status":"Official","country":"US","date":"2020","disambiguation":"deluxe","media":[
				{"position":1,"format":"CD","tracks":[
					{"id":"aaaaaaaa-0000-0000-0000-000000000011","position":1,"title":"Dawn","recording":{"id":"bbbbbbbb-0000-0000-0000-000000000001"}}]},
				{"position":2,"format":"CD","tracks":[
					{"id":"aaaaaaaa-0000-0000-0000-000000000012","position":1,"title":"Noon","recording":{"id":"bbbbbbbb-0000-0000-0000-000000000002"}},
					{"id":"aaaaaaaa-0000-0000-0000-000000000013","position":2,"title":"Dusk","recording":{"id":"bbbbbbbb-0000-0000-0000-000000000003"}}]}]}]}`, mbRel1, mbRel2)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func mbSetup(t *testing.T) (*MusicBrainz, *fakeMB, *fakeClock, *memCache) {
	t.Helper()
	clock := newClock()
	fake := &fakeMB{clock: clock}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cache := &memCache{}
	mb := &MusicBrainz{BaseURL: func(context.Context) string { return srv.URL + "/" }, UserAgent: UserAgentFor("1.2.3"),
		Limiter: &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}, Cache: cache}
	return mb, fake, clock, cache
}

// gapsAtLeast fails when two requests were sent less than d apart.
func gapsAtLeast(t *testing.T, at []time.Time, d time.Duration) {
	t.Helper()
	for i := 1; i < len(at); i++ {
		if g := at[i].Sub(at[i-1]); g < d {
			t.Fatalf("requests %d and %d only %v apart: %v", i-1, i, g, at)
		}
	}
}

func TestMusicBrainzSearchByName(t *testing.T) {
	mb, fake, _, _ := mbSetup(t)
	got, err := mb.SearchArtists(context.Background(), "Lab: Band")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].MBID != mbArtistID || got[0].Name != "Lab Band" || got[0].Type != "group" || got[0].Status != "running" ||
		got[0].Disambiguation != "test band" || got[0].Score != 100 || got[0].Albums != nil {
		t.Fatalf("search %+v", got)
	}
	// The search words are escaped for the search syntax, and HolyArrs names itself.
	if !strings.Contains(fake.hits[0], `query=Lab%5C%3A+Band`) || fake.ua[0] != "HolyArrs/1.2.3 ( https://github.com/HolyArrs/HolyArrs )" {
		t.Fatalf("request %q ua %q", fake.hits[0], fake.ua[0])
	}
	if got, err := mb.SearchArtists(context.Background(), "nobody"); err != nil || len(got) != 0 {
		t.Fatalf("no match %+v %v", got, err)
	}
	if got, err := mb.SearchArtists(context.Background(), "  "); err != nil || len(got) != 0 || len(fake.hits) != 2 {
		t.Fatalf("empty search %+v %v, %d requests", got, err, len(fake.hits))
	}
}

func TestMusicBrainzArtistAndAlbums(t *testing.T) {
	mb, fake, _, _ := mbSetup(t)
	a, err := mb.Artist(context.Background(), strings.ToUpper(mbArtistID))
	if err != nil {
		t.Fatal(err)
	}
	if a.MBID != mbArtistID || a.Status != "ended" || len(a.Albums) != 3 {
		t.Fatalf("artist %+v", a)
	}
	byID := map[string]Album{}
	for _, al := range a.Albums {
		byID[al.MBID] = al
	}
	if al := byID[mbAlbumA]; al.Name != "First Light" || al.PrimaryType != "album" || al.ReleaseDate != "2020-03-06" || len(al.SecondaryTypes) != 0 {
		t.Fatalf("album A %+v", al)
	}
	if al := byID[mbAlbumB]; al.PrimaryType != "ep" || al.ReleaseDate != "2021" {
		t.Fatalf("album B %+v", al)
	}
	if al := byID[mbLive]; al.SecondaryTypes[0] != "live" || al.ReleaseDate != "2022-05" || al.Disambiguation != "tour" {
		t.Fatalf("live album %+v", al)
	}
	// One request for the artist and one per page of albums.
	if len(fake.hits) != 3 || !strings.Contains(fake.hits[2], "offset=2") {
		t.Fatalf("requests %v", fake.hits)
	}
	if _, err := mb.Artist(context.Background(), "99999999-9999-9999-9999-999999999999"); !errors.Is(err, ErrArtistNotFound) {
		t.Fatalf("unknown artist: %v", err)
	}
	n := len(fake.hits)
	if _, err := mb.Artist(context.Background(), "../etc"); !errors.Is(err, ErrArtistNotFound) || len(fake.hits) != n {
		t.Fatalf("a bad id must not be sent: %v", err)
	}
}

func TestMusicBrainzPressingsWithTracks(t *testing.T) {
	mb, _, _, _ := mbSetup(t)
	ps, err := mb.Pressings(context.Background(), mbAlbumA)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("pressings %+v", ps)
	}
	d, dx := ps[0], ps[1]
	if d.MBID != mbRel1 || d.Status != "official" || d.Country != "XW" || d.Date != "2020-03-06" || len(d.Formats) != 1 || d.Formats[0] != "Digital Media" || len(d.Tracks) != 2 {
		t.Fatalf("digital %+v", d)
	}
	if tr := d.Tracks[1]; tr.Name != "Noon" || tr.LengthMS != 180000 || tr.Disc != 1 || tr.Number != 2 || tr.Position != 2 || tr.RecordingID != "bbbbbbbb-0000-0000-0000-000000000002" {
		t.Fatalf("track length from the recording %+v", tr)
	}
	if dx.Disambiguation != "deluxe" || len(dx.Formats) != 2 || len(dx.Tracks) != 3 {
		t.Fatalf("deluxe %+v", dx)
	}
	if tr := dx.Tracks[2]; tr.Disc != 2 || tr.Number != 2 || tr.Position != 3 || tr.Name != "Dusk" {
		t.Fatalf("second disc track %+v", tr)
	}
}

// TestMusicBrainzOneRequestASecond: back-to-back lookups are spaced at
// least a second apart on the clock, and callers sharing one limiter take
// distinct moments a second apart.
func TestMusicBrainzOneRequestASecond(t *testing.T) {
	mb, fake, clock, _ := mbSetup(t)
	ctx := NoCache(context.Background())
	for i := 0; i < 3; i++ {
		if _, err := mb.Artist(ctx, mbArtistID); err != nil {
			t.Fatal(err)
		}
	}
	if len(fake.at) != 9 {
		t.Fatalf("requests %d", len(fake.at))
	}
	gapsAtLeast(t, fake.at, time.Second)
	if total := fake.at[8].Sub(fake.at[0]); total < 8*time.Second {
		t.Fatalf("9 requests in %v", total)
	}

	// Many callers at once on one shared limiter.
	clock.mu.Lock()
	clock.slots = nil
	clock.mu.Unlock()
	other := &MusicBrainz{BaseURL: mb.BaseURL, Limiter: mb.Limiter}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := mb
			if i%2 == 1 {
				c = other
			}
			_, _ = c.SearchArtists(ctx, fmt.Sprintf("lab %d", i))
		}(i)
	}
	wg.Wait()
	clock.mu.Lock()
	slots := append([]time.Time(nil), clock.slots...)
	clock.mu.Unlock()
	sort.Slice(slots, func(i, j int) bool { return slots[i].Before(slots[j]) })
	if len(slots) != 8 {
		t.Fatalf("slots %d", len(slots))
	}
	gapsAtLeast(t, slots, time.Second)
}

// TestMusicBrainzBacksOffWhenBusy: 503 answers are tried again after 2
// then 4 seconds (or Retry-After), and give up with ErrMusicBusy.
func TestMusicBrainzBacksOffWhenBusy(t *testing.T) {
	mb, fake, _, _ := mbSetup(t)
	fake.busy = 2
	got, err := mb.SearchArtists(context.Background(), "lab")
	if err != nil || len(got) != 1 {
		t.Fatalf("after two 503s: %+v %v", got, err)
	}
	if len(fake.at) != 3 {
		t.Fatalf("requests %d", len(fake.at))
	}
	if g1, g2 := fake.at[1].Sub(fake.at[0]), fake.at[2].Sub(fake.at[1]); g1 != 2*time.Second || g2 != 4*time.Second {
		t.Fatalf("back-off gaps %v %v", g1, g2)
	}

	// Retry-After is honoured.
	fake.busy, fake.after = 1, "7"
	if _, err := mb.SearchArtists(NoCache(context.Background()), "lab"); err != nil {
		t.Fatal(err)
	}
	if g := fake.at[4].Sub(fake.at[3]); g != 7*time.Second {
		t.Fatalf("Retry-After gap %v", g)
	}

	// Still busy after every try.
	fake.busy, fake.after = 10, ""
	before := len(fake.at)
	if _, err := mb.SearchArtists(NoCache(context.Background()), "lab"); !errors.Is(err, ErrMusicBusy) {
		t.Fatalf("busy: %v", err)
	}
	if n := len(fake.at) - before; n != 4 {
		t.Fatalf("tries %d, want 4", n)
	}
	// The next request also waits out the back-off, whoever sends it.
	fake.busy = 0
	before = len(fake.at)
	if _, err := mb.Artist(context.Background(), mbArtistID); err != nil {
		t.Fatal(err)
	}
	if g := fake.at[before].Sub(fake.at[before-1]); g < 8*time.Second {
		t.Fatalf("the next caller went %v after the last busy answer", g)
	}
}

// TestMusicBrainzCache: a second lookup is answered from the cache until
// the expiry, a refresh (NoCache) asks the server and stores the fresh
// answer, and 404s are not cached.
func TestMusicBrainzCache(t *testing.T) {
	mb, fake, clock, cache := mbSetup(t)
	mb.TTL = func(context.Context) time.Duration { return time.Hour }
	ctx := context.Background()
	if _, err := mb.Pressings(ctx, mbAlbumA); err != nil {
		t.Fatal(err)
	}
	if _, err := mb.Pressings(ctx, mbAlbumA); err != nil || len(fake.hits) != 1 {
		t.Fatalf("cache hit: %d requests, %v", len(fake.hits), err)
	}
	if _, err := mb.Pressings(NoCache(ctx), mbAlbumA); err != nil || len(fake.hits) != 2 {
		t.Fatalf("refresh skips the cache: %d requests, %v", len(fake.hits), err)
	}
	// Past the expiry the server is asked again.
	clock.mu.Lock()
	clock.now = clock.now.Add(61 * time.Minute)
	clock.mu.Unlock()
	if _, err := mb.Pressings(ctx, mbAlbumA); err != nil || len(fake.hits) != 3 {
		t.Fatalf("expired: %d requests, %v", len(fake.hits), err)
	}
	for k := range cache.rows {
		if !strings.HasPrefix(k, "musicbrainz:"+strings.TrimPrefix(strings.TrimSuffix(mb.BaseURL(ctx), "/"), "http://")+"/ws/2/release?") {
			t.Fatalf("cache key %q", k)
		}
	}
	_, _ = mb.Artist(ctx, "99999999-9999-9999-9999-999999999999")
	_, _ = mb.Artist(ctx, "99999999-9999-9999-9999-999999999999")
	if len(fake.hits) != 5 {
		t.Fatalf("a 404 must not be cached: %d requests", len(fake.hits))
	}
}

// TestMusicBrainzBaseURL: the base URL is read on every request, so a
// changed setting (a mirror) takes effect at once; empty is the public server.
func TestMusicBrainzBaseURL(t *testing.T) {
	mb, fake, _, _ := mbSetup(t)
	clock := newClock()
	mirror := &fakeMB{clock: clock}
	srv := httptest.NewServer(mirror)
	defer srv.Close()
	base := mb.BaseURL(context.Background())
	use := base
	mb.BaseURL = func(context.Context) string { return use }
	if _, err := mb.SearchArtists(context.Background(), "lab"); err != nil || len(fake.hits) != 1 {
		t.Fatal(err)
	}
	use = srv.URL
	if _, err := mb.SearchArtists(NoCache(context.Background()), "lab"); err != nil || len(mirror.hits) != 1 {
		t.Fatalf("mirror not asked: %v", err)
	}
	if got := (&MusicBrainz{}).base(context.Background()); got != DefaultMusicBrainzURL {
		t.Fatalf("default base %q", got)
	}
}

func TestMusicBrainzArtistGenres(t *testing.T) {
	mb, _, _, _ := mbSetup(t)
	got, err := mb.ArtistGenres(context.Background(), strings.ToUpper(mbArtistID))
	if err != nil || strings.Join(got, ",") != "art pop,rock" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := mb.ArtistGenres(context.Background(), "nope"); err == nil {
		t.Fatal("bad id looked up")
	}
}

func TestMusicBrainzLinksAndSoundtracks(t *testing.T) {
	mb, _, _, _ := mbSetup(t)
	_, links, err := mb.ArtistGenresLinks(context.Background(), mbArtistID)
	if err != nil || len(links) != 1 || links[0].Name != "Singer" || links[0].MBID != mbAlbumA {
		t.Fatalf("%+v %v", links, err)
	}
	st, err := mb.SoundtrackFor(context.Background(), "Lab Movie", 2020)
	if err != nil || len(st) != 1 || st[0].MBID != mbAlbumA {
		t.Fatalf("%+v %v", st, err)
	}
}
