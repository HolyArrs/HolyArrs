package migrate

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// bigSeries is a Sonarr-shaped show list of about target bytes, with the
// pictures, other names, overviews and statistics a real one carries.
func bigSeries(target int) []byte {
	var b bytes.Buffer
	writeSeries(&b, target)
	return b.Bytes()
}

// writeSeries writes bigSeries to w as it goes, so a test server never
// holds the whole answer (a held answer would skew the heap measured).
func writeSeries(w io.Writer, target int) {
	b := &countWriter{w: bufio.NewWriterSize(w, 64<<10)}
	defer b.w.(*bufio.Writer).Flush()
	b.WriteString("[")
	ov := strings.Repeat("A plot line about the show. ", 25)
	for i := 1; b.Len() < target; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		var alts, imgs, seasons []string
		for a := 0; a < 8; a++ {
			alts = append(alts, fmt.Sprintf(`{"title":"Other name %d-%d","seasonNumber":-1}`, i, a))
		}
		for _, k := range []string{"banner", "poster", "fanart", "clearlogo"} {
			imgs = append(imgs, fmt.Sprintf(`{"coverType":"%s","url":"/MediaCover/%d/%s.jpg","remoteUrl":"https://img.example/%d/%s/abcdef0123456789.jpg"}`, k, i, k, i, k))
		}
		for s := 0; s < 8; s++ {
			seasons = append(seasons, fmt.Sprintf(`{"seasonNumber":%d,"monitored":%t,"statistics":{"episodeFileCount":10,"sizeOnDisk":12345678901,"releaseGroups":["GRP"]}}`, s, s%2 == 0))
		}
		fmt.Fprintf(b, `{"id":%d,"title":"Show %d","sortTitle":"show %d","overview":"%s","images":[%s],"seasons":[%s],"year":2010,"path":"/tv/Show %d","qualityProfileId":1,"seasonFolder":true,"monitored":true,"monitorNewItems":"all","tvdbId":%d,"tvMazeId":%d,"tmdbId":%d,"imdbId":"tt%07d","seriesType":"anime","rootFolderPath":"/tv/","genres":["Drama"],"tags":[1],"added":"2021-01-01T00:00:00Z","ratings":{"votes":1234,"value":8.1},"statistics":{"seasonCount":8,"sizeOnDisk":98765432100},"languageProfileId":2,"alternateTitles":[%s]}`,
			i, i, i, ov, strings.Join(imgs, ","), strings.Join(seasons, ","), i, 70000+i, i, i, i, strings.Join(alts, ","))
	}
	b.WriteString("]")
}

type countWriter struct {
	w io.Writer
	n int
}

func (c *countWriter) Write(p []byte) (int, error) { n, err := c.w.Write(p); c.n += n; return n, err }
func (c *countWriter) WriteString(s string)        { _, _ = c.Write([]byte(s)) }
func (c *countWriter) Len() int                    { return c.n }

// titleRows reads the title list at /x as the builder sees it.
func titleRows(c *apiClient) ([]map[string]any, error) {
	ts, err := c.getTitles(context.Background(), "/x")
	var rows []map[string]any
	for i := range ts {
		rows = append(rows, ts[i].row())
	}
	return rows, err
}

func listServer(t *testing.T, body []byte) *apiClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	c, err := newAPIClient(Source{Kind: Sonarr, URL: srv.URL, AllowLocal: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestListSameTitles: for every app's title shape, the slim rows read entry
// by entry build the same Titles as the whole answer did.
func TestListSameTitles(t *testing.T) {
	shapes := map[Kind]string{
		Sonarr: string(bigSeries(20_000)),
		Radarr: `[{"id":3,"title":"M","year":1999,"path":"/m/M","rootFolderPath":"/m/","monitored":true,"added":"2020","tags":[2],"qualityProfileId":4,"tmdbId":603,"imdbId":"tt0133093","minimumAvailability":"released","images":[{"url":"x"}],"overview":"o",
			"movieFile":{"id":8,"relativePath":"M.mkv","quality":{"quality":{"id":7,"name":"Bluray-1080p"}},"mediaInfo":{"videoCodec":"x265"}}}]`,
		Lidarr: `[{"id":5,"artistName":"A","foreignArtistId":"11111111-2222-3333-4444-555555555555","artistMetadataId":9,"monitored":true,"monitorNewItems":"new","metadataProfileId":2,"qualityProfileId":1,"path":"/music/A","rootFolderPath":"/music/","tags":[],"added":"2020",
			"nextAlbum":{"title":"x","images":[]},"lastAlbum":{"title":"y"},"overview":"o","statistics":{"albumCount":3}}]`,
	}
	for kind, body := range shapes {
		c := listServer(t, []byte(body))
		var whole any
		if err := c.getJSON(context.Background(), "/x", nil, &whole); err != nil {
			t.Fatal(err)
		}
		slim, err := titleRows(c)
		if err != nil {
			t.Fatal(err)
		}
		r := arrReader{kind: kind}
		labels := func(any) []string { return nil }
		for i, e := range whole.([]any) {
			full := lower(e.(map[string]any))
			a, b := r.title(full, nil, labels), r.title(slim[i], nil, labels)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("%s: titles differ\nwhole %+v\nslim  %+v", kind, a, b)
			}
			if kind == Radarr && !reflect.DeepEqual(qualityName(mapOf(full["moviefile"])["quality"], nil), qualityName(mapOf(slim[i]["moviefile"])["quality"], nil)) {
				t.Fatal("movie file quality differs")
			}
		}
	}
}

// TestListHostile: broken and hostile answers fail plainly.
func TestListHostile(t *testing.T) {
	cases := map[string]struct {
		body string
		lim  Limits
		want error
	}{
		"not a list": {body: `{"id":1}`, want: ErrBadAnswer},
		"cut off":    {body: `[{"id":1},{"id":`, want: ErrBadAnswer},
		"not json":   {body: `[{"id":1} nope`, want: ErrBadAnswer},
		"deep":       {body: "[" + strings.Repeat(`{"a":`, 100) + "1" + strings.Repeat("}", 100) + "]", want: ErrBadAnswer},
		"too big":    {body: "[" + strings.Repeat(`{"id":1},`, 2000) + `{"id":1}]`, lim: Limits{MaxList: 4096}, want: ErrTooBig},
		"many rows":  {body: "[" + strings.Repeat(`{"id":1},`, 50) + `{"id":1}]`, lim: Limits{MaxRows: 10}, want: ErrTooBig},
		"empty":      {body: `[]`},
		"skips junk": {body: `[1,"x",null,{"id":2}]`},
		// One entry is never bigger than a whole answer may be; many small ones are fine.
		"huge entry":  {body: `[{"id":1},{"id":2,"title":"` + strings.Repeat("x", 20000) + `"}]`, lim: Limits{MaxAnswer: 4096}, want: ErrTooBig},
		"huge scalar": {body: `[{"id":1},"` + strings.Repeat("x", 20000) + `"]`, lim: Limits{MaxAnswer: 4096}, want: ErrTooBig},
		"many small":  {body: "[" + strings.Repeat(`{"id":1,"tags":[1,2]},`, 2000) + `{"id":1}]`, lim: Limits{MaxAnswer: 4096}},
		"huge string": {body: `[{"id":1,"title":"` + strings.Repeat(`\"[{`, 5000) + `"}]`, lim: Limits{MaxAnswer: 4096}, want: ErrTooBig},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
		c, _ := newAPIClient(Source{Kind: Sonarr, URL: srv.URL, AllowLocal: true, Limits: tc.lim}, nil)
		rows, err := titleRows(c)
		srv.Close()
		if tc.want == nil {
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if name == "skips junk" && len(rows) != 1 {
				t.Fatalf("%s: rows %v", name, rows)
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: want %v, got %v", name, tc.want, err)
		}
	}
	// A declared size past the cap is refused before reading.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write([]byte("["))
	}))
	defer srv.Close()
	c, _ := newAPIClient(Source{Kind: Sonarr, URL: srv.URL, AllowLocal: true, Limits: Limits{MaxList: 4096}}, nil)
	if _, err := titleRows(c); !errors.Is(err, ErrTooBig) {
		t.Fatalf("declared size: %v", err)
	}
}

type heapPeak struct {
	max  atomic.Uint64
	stop chan struct{}
	done chan struct{}
}

func watchHeap() *heapPeak {
	p := &heapPeak{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(p.done)
		var m runtime.MemStats
		for {
			select {
			case <-p.stop:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > p.max.Load() {
				p.max.Store(m.HeapAlloc)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	return p
}

func heapNow() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// measureList reads body with the old whole-answer way (old) or the entry
// by entry reader, and returns rows, peak and held heap above the start (MB).
func measureList(t *testing.T, mb int, old bool) (int, int64, int64) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeSeries(w, mb<<20) }))
	defer srv.Close()
	c, err := newAPIClient(Source{Kind: Sonarr, URL: srv.URL, AllowLocal: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.lim.MaxAnswer = 1 << 30
	b0 := heapNow()
	p := watchHeap()
	var rows []map[string]any
	var slim []slimTitle
	if old {
		var got any
		if err = c.getJSON(context.Background(), "/api/v3/series", nil, &got); err == nil {
			for _, e := range got.([]any) {
				rows = append(rows, lower(e.(map[string]any)))
			}
		}
		got = nil
	} else {
		slim, err = c.getTitles(context.Background(), "/api/v3/series")
	}
	if err != nil {
		t.Fatal(err)
	}
	held := heapNow()
	close(p.stop)
	<-p.done
	runtime.KeepAlive(rows)
	runtime.KeepAlive(slim)
	return len(rows) + len(slim), int64(p.max.Load()-b0) >> 20, (int64(held) - int64(b0)) >> 20
}

// TestListBoundedRAM: a 35 MB show list (7,700 shows) is read with a
// small, bounded heap. HOLYARRS_BIG_LIST=1 adds a 200 MB list and the old
// whole-answer way for comparison.
func TestListBoundedRAM(t *testing.T) {
	sizes := []int{35}
	big := os.Getenv("HOLYARRS_BIG_LIST") != ""
	if big {
		sizes = append(sizes, 200)
	}
	for _, mb := range sizes {
		if big {
			n, peak, held := measureList(t, mb, true)
			t.Logf("before: %d MB answer, %d shows: peak heap +%d MB, held +%d MB", mb, n, peak, held)
		}
		n, peak, held := measureList(t, mb, false)
		t.Logf("after:  %d MB answer, %d shows: peak heap +%d MB, held +%d MB", mb, n, peak, held)
		// The heap grows with the shows kept (about 1.3 KB each), not the answer.
		if limit := int64(40 + mb); peak > limit {
			t.Fatalf("%d MB answer took +%d MB of heap, want at most %d", mb, peak, limit)
		}
	}
}
