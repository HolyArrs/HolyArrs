package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

func id(n int) string { return fmt.Sprintf("%08d-0000-4000-8000-000000000000", n) }

// Charts read spotify81 first, Apple Music when it has nothing, and
// ListenBrainz as the backup; each item says where it came from.
func TestChartSourceOrder(t *testing.T) {
	s := chartService(t)
	ctx, now := context.Background(), time.Now()
	s.lb.albums = []Entry{{MBID: id(1), Name: "LB Album", Artist: "B", ArtistMBID: id(2)}}
	s.lb.songs = []Entry{{Name: "LB Song", Artist: "B", ArtistMBID: id(2)}}

	// Nothing but ListenBrainz.
	got, ok := s.Charts(ctx, "US", "albums", "", now)
	if !ok || len(got) != 1 || got[0].Source != SrcListenBrainz {
		t.Fatalf("backup %+v", got)
	}
	// Apple Music before ListenBrainz.
	s.apple["us"] = chart{albums: []Entry{{Name: "Apple Album", Artist: "A"}}}
	got, _ = s.Charts(ctx, "US", "albums", "", now)
	if len(got) != 1 || got[0].Source != SrcApple {
		t.Fatalf("apple %+v", got)
	}
	// spotify81 before Apple Music.
	s.SetLists(fakeLists{})
	got, _ = s.Charts(ctx, "US", "albums", "", now)
	if len(got) == 0 || got[0].Source != SrcLists || got[0].Title != "Listed" {
		t.Fatalf("lists %+v", got)
	}
	// A region with no list chart and no Apple songs falls to ListenBrainz.
	got, _ = s.Charts(ctx, "US", "songs", "", now)
	if len(got) != 1 || got[0].Source != SrcListenBrainz || got[0].Title != "LB Song" {
		t.Fatalf("songs backup %+v", got)
	}
}

// The trending lists use the same order and tag every item; bodies kept
// from before the tags are not served again.
func TestTrendingListsSourceOrderAndOldBodies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.sqlite")
	s, err := Open(path, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.db.Exec(`INSERT INTO lists (kind, region, body) VALUES ('album', 'global', '{"schema_version":1,"items":[{"mbid":"old"}]}')`)
	s.lb.albums = []Entry{{MBID: id(1), Name: "LB Album", Artist: "B", ArtistMBID: id(2)}}
	s.lb.artists = []Entry{{MBID: id(2), Name: "B"}}
	if err := s.publish(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	b, _, ok := s.List("album", "global")
	if !ok || !strings.Contains(string(b), `"source":"listenbrainz"`) {
		t.Fatalf("lb album list %s", b)
	}
	s.Close()
	_, _ = func() (any, error) {
		db, err := openDB(path)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		return db.Exec(`UPDATE lists SET body = '{"schema_version":1,"items":[]}' WHERE kind = 'artist'`)
	}()
	s2, err := Open(path, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, _, ok := s2.List("artist", "global"); ok {
		t.Fatal("old untagged list served after restart")
	}
	if _, _, ok := s2.List("album", "global"); !ok {
		t.Fatal("tagged list lost")
	}
}

// Genre pages: ListenBrainz's lists sorted by the slim copy's tags (the
// release group's, else the artist's).
func TestGenrePagesFromSlimTagsAndListenBrainz(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			w.WriteHeader(404)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/artists"):
			fmt.Fprintf(w, `{"payload":{"artists":[{"artist_mbid":"%s","artist_name":"Rocker"}]}}`, id(10))
		case strings.HasSuffix(r.URL.Path, "/release-groups"):
			fmt.Fprintf(w, `{"payload":{"release_groups":[{"release_group_mbid":"%s","release_group_name":"Loud","artist_name":"Rocker","artist_mbids":["%s"]},{"release_group_mbid":"%s","release_group_name":"Untagged","artist_name":"Popper","artist_mbids":["%s"]}]}}`, id(1), id(10), id(2), id(20))
		case strings.HasSuffix(r.URL.Path, "/recordings"):
			fmt.Fprintf(w, `{"payload":{"recordings":[{"track_name":"Hook","artist_name":"Popper","artist_mbids":["%s"]}]}}`, id(20))
		}
	}))
	defer srv.Close()
	tags := map[string][]string{"release-group|" + id(1): {"hard rock"}, "artist|" + id(20): {"dance-pop", "synth-pop"}}
	s, err := Open(filepath.Join(t.TempDir(), "m.sqlite"), Options{ListenBrainzURL: srv.URL + "/1/stats/sitewide", Pause: -1, Search: &fakeSearch{},
		Tags: func(_ context.Context, kind, mbid string) []string { return tags[kind+"|"+mbid] }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.refreshLB(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, now := context.Background(), time.Now()
	rock, ok := s.Charts(ctx, "global", "albums", "rock", now)
	if !ok || len(rock) != 1 || rock[0].Title != "Loud" || rock[0].Source != SrcListenBrainz || rock[0].GenreSource != SrcDump {
		t.Fatalf("rock %+v", rock)
	}
	pop, _ := s.Charts(ctx, "global", "albums", "pop", now)
	if len(pop) != 1 || pop[0].Title != "Untagged" {
		t.Fatalf("pop by artist tags %+v", pop)
	}
	songs, _ := s.Charts(ctx, "global", "songs", "dance", now)
	if len(songs) != 1 || songs[0].Title != "Hook" {
		t.Fatalf("dance songs %+v", songs)
	}
	g, ok := s.Genres("global")
	if !ok || len(g) < 3 {
		t.Fatalf("genres %+v", g)
	}
}

// fakePictures is a picture store: Fetch stores from the table, or
// records a miss, or fails (busy) the first time.
type fakePictures struct {
	mu      sync.Mutex
	have    map[string]string // kind|id -> source
	missed  map[string]bool
	answer  map[string]string // kind|id -> source a fetch finds ("" miss)
	busy    map[string]int    // fails this many times first
	fetches int
	bigNow  int // fetches whose big size would be made inside the fetch
}

func (f *fakePictures) Provider(_ context.Context, kind, id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.have[kind+"|"+id]
	return s, ok
}
func (f *fakePictures) Missed(_ context.Context, kind, id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.missed[kind+"|"+id]
}
func (f *fakePictures) Fetch(ctx context.Context, kind, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	if !imgstore.IsBigLater(ctx) {
		f.bigNow++
	}
	k := kind + "|" + id
	if f.busy[k] > 0 {
		f.busy[k]--
		return errors.New("busy")
	}
	src, ok := f.answer[k]
	if !ok || src == "" {
		f.missed[k] = true
		return errors.New("no picture")
	}
	f.have[k] = src
	return nil
}

func newPrefetch(pics *fakePictures, chart []string, similar map[string][]string, albums map[string][]string) *Prefetch {
	return &Prefetch{Pictures: pics, Workers: 2, Retries: 3,
		Artists: func(context.Context) []string { return chart },
		Similar: func(_ context.Context, a string) []string { return similar[a] },
		Albums:  func(_ context.Context, a string) []string { return albums[a] }}
}

// Coverage: every chart artist plus its top 5 similar artists, and all
// their albums, are fetched; counts per source are kept.
func TestPrefetchCoverage(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	chart := []string{id(1), id(2), "not-an-id"}
	similar := map[string][]string{id(1): {id(11), id(12), id(13), id(14), id(15), id(16)}, id(2): {id(11)}}
	albums := map[string][]string{id(1): {id(101), id(102)}, id(11): {id(111)}, id(16): {id(161)}}
	for _, a := range []string{id(1), id(2), id(11), id(12), id(13), id(14), id(15)} {
		pics.answer["artist|"+a] = "applemusic"
	}
	pics.answer["artist|"+id(15)] = "deezer" // overflow
	pics.answer["album|"+id(101)] = "applemusic"
	pics.answer["album|"+id(102)] = "caa"
	pics.answer["album|"+id(111)] = "" // no source has it: a recorded miss
	pics.busy["artist|"+id(2)] = 1     // busy once, fetched on the next try
	p := newPrefetch(pics, chart, similar, albums)
	c := p.Pass(context.Background())
	if c.ChartArtists != 2 || c.Artists != 7 {
		t.Fatalf("members %d chart, %d all (want 2, 7: the 6th similar is left out)", c.ChartArtists, c.Artists)
	}
	if _, ok := pics.have["artist|"+id(16)]; ok {
		t.Fatal("6th similar artist fetched")
	}
	if c.Albums != 3 || c.AlbumCovers != 2 || c.AlbumMisses != 1 {
		t.Fatalf("albums %+v", c)
	}
	if c.ArtistPhotos != 7 || c.PhotoSources["applemusic"] != 6 || c.PhotoSources["deezer"] != 1 || c.CoverSources["caa"] != 1 {
		t.Fatalf("sources %+v %+v", c.PhotoSources, c.CoverSources)
	}
	if !c.Ready || !p.Ready() {
		t.Fatalf("gate closed with full coverage: %+v", c)
	}
	// A second pass fetches nothing: everything is stored or missed.
	before := pics.fetches
	p.Pass(context.Background())
	if pics.fetches != before {
		t.Fatalf("second pass fetched %d", pics.fetches-before)
	}
}

// The gate stays closed (charts hidden) while any artist lacks a photo or
// any album has neither a cover nor a recorded miss.
func TestGateHiddenUntilItPasses(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	chart := []string{id(1)}
	albums := map[string][]string{id(1): {id(101)}}
	pics.answer["artist|"+id(1)] = "applemusic"
	pics.busy["album|"+id(101)] = 10 // the cover source stays busy
	p := newPrefetch(pics, chart, nil, albums)
	if p.Ready() {
		t.Fatal("gate open before any pass")
	}
	c := p.Pass(context.Background())
	if c.Ready || p.Ready() || len(c.Open) != 1 {
		t.Fatalf("gate open with an album still open: %+v", c)
	}
	// An artist whose photo source stays busy (no miss recorded) keeps it
	// closed.
	pics.busy = map[string]int{"artist|" + id(2): 10}
	pics.answer["album|"+id(101)] = "applemusic"
	chart = append(chart, id(2))
	p.Artists = func(context.Context) []string { return chart }
	c = p.Pass(context.Background())
	if c.Ready || len(c.Open) != 1 || c.Open[0] != id(2) {
		t.Fatalf("gate open with an artist not tried by every source: %+v", c)
	}
	// Every source tried, miss recorded: covered, and listed for the audit.
	pics.busy = map[string]int{}
	if c = p.Pass(context.Background()); !c.Ready || !p.Ready() || c.ArtistMisses != 1 || len(c.NoPhoto) != 1 || c.NoPhoto[0] != id(2) {
		t.Fatalf("recorded photo miss should pass and be listed: %+v", c)
	}
	// No chart artists at all (wiped stores): closed.
	p.Artists = func(context.Context) []string { return nil }
	if c = p.Pass(context.Background()); c.Ready {
		t.Fatal("gate open with no charts")
	}
	b, _ := json.Marshal(c)
	if !strings.Contains(string(b), `"ready":false`) {
		t.Fatalf("coverage json %s", b)
	}
}
