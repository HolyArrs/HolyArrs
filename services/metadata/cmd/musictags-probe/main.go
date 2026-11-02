// Command musictags-probe builds a music catalog bundle from live data on
// a workstation (Phase 0 of the row templates plan): listen ranks and top
// albums per artist from ListenBrainz, charts from our own metadata API.
// Raw answers are cached on disk, so a re-run costs no calls.
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictags"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"

	_ "modernc.org/sqlite"
)

type fetcher struct {
	client *http.Client
	cache  string
	every  time.Duration
	last   time.Time
	calls  int
	// export, when set, receives every answer under the server cache's key
	// (url, or url#sha1(body) for posts), so a server starts warm.
	export *sql.DB
}

func (f *fetcher) keep(key string, b []byte) {
	if f.export != nil {
		_, _ = f.export.Exec(`INSERT INTO mt_cache (url, body, at) VALUES (?, ?, ?) ON CONFLICT(url) DO UPDATE SET body = excluded.body, at = excluded.at`, key, b, time.Now().Unix())
	}
}

// Get and Post make the fetcher a musictags.Fetcher.
func (f *fetcher) Get(_ context.Context, u string) ([]byte, error) { return f.get(u) }
func (f *fetcher) Post(_ context.Context, u string, b []byte) ([]byte, error) {
	return f.post(u, b)
}

func (f *fetcher) get(u string) ([]byte, error) {
	b, err := f.get1(u)
	if err == nil {
		f.keep(u, b)
	}
	return b, err
}

func (f *fetcher) get1(u string) ([]byte, error) {
	h := sha1.Sum([]byte(u))
	path := filepath.Join(f.cache, hex.EncodeToString(h[:]))
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	for try := 0; try < 5; try++ {
		if d := f.every - time.Since(f.last); d > 0 {
			time.Sleep(d)
		}
		f.last = time.Now()
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "HolyArrs-metadata-probe/0.1 ( https://holyarrs-api.coderluii.dev )")
		resp, err := f.client.Do(req)
		f.calls++
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			time.Sleep(time.Duration(5*(try+1)) * time.Second)
			continue
		}
		if resp.StatusCode == 404 || resp.StatusCode == 204 {
			b = []byte("null")
		} else if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%s: %s", u, resp.Status)
		}
		_ = os.WriteFile(path, b, 0o644)
		return b, nil
	}
	return nil, fmt.Errorf("%s: gave up", u)
}

// post sends a JSON body; the answer is cached by URL and body.
func (f *fetcher) post(u string, body []byte) ([]byte, error) {
	b, err := f.post1(u, body)
	if err == nil {
		bh := sha1.Sum(body)
		f.keep(u+"#"+hex.EncodeToString(bh[:]), b)
	}
	return b, err
}

func (f *fetcher) post1(u string, body []byte) ([]byte, error) {
	h := sha1.Sum(append([]byte(u+"\n"), body...))
	path := filepath.Join(f.cache, hex.EncodeToString(h[:]))
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	for try := 0; try < 5; try++ {
		if d := f.every - time.Since(f.last); d > 0 {
			time.Sleep(d)
		}
		f.last = time.Now()
		req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
		req.Header.Set("User-Agent", "HolyArrs-metadata-probe/0.1 ( https://holyarrs-api.coderluii.dev )")
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.client.Do(req)
		f.calls++
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			time.Sleep(time.Duration(5*(try+1)) * time.Second)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%s: %s", u, resp.Status)
		}
		_ = os.WriteFile(path, b, 0o644)
		return b, nil
	}
	return nil, fmt.Errorf("%s: gave up", u)
}

func main() {
	out := flag.String("out", ".", "output folder (bundle.json, stats.json, cache/)")
	api := flag.String("api", "https://holyarrs-api.coderluii.dev", "our metadata API")
	lb := flag.String("lb", "https://api.listenbrainz.org/1", "ListenBrainz API root")
	mbRoot := flag.String("mb", "https://musicbrainz.org/ws/2", "MusicBrainz API root")
	maxArtists := flag.Int("artists", 0, "cap on seed artists (0: all)")
	keep := flag.Int("keep", 10, "albums kept per artist")
	export := flag.String("export", "", "write a warm musictags.sqlite cache here (ListenBrainz and MusicBrainz answers)")
	flag.Parse()
	cache := filepath.Join(*out, "cache")
	_ = os.MkdirAll(cache, 0o755)
	f := &fetcher{client: &http.Client{Timeout: 60 * time.Second}, cache: cache, every: 400 * time.Millisecond}
	ours := &fetcher{client: f.client, cache: cache, every: 50 * time.Millisecond}
	start := time.Now()
	charts := func(context.Context) []musictags.ChartRow {
		var out []musictags.ChartRow
		for _, st := range musictrend.Storefronts() {
			for _, kind := range []string{"albums", "songs"} {
				b, err := ours.get(*api + "/v1/music/charts?region=" + st + "&kind=" + kind)
				if err != nil {
					continue
				}
				var c struct {
					Items []musictrend.ChartItem `json:"items"`
				}
				if json.Unmarshal(b, &c) == nil {
					out = append(out, musictags.ChartRow{Country: st, Kind: kind, Items: c.Items})
				}
			}
		}
		return out
	}
	mb := &fetcher{client: f.client, cache: cache, every: 1100 * time.Millisecond}
	if *export != "" {
		db, err := sql.Open("sqlite", *export)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mt_cache (url TEXT PRIMARY KEY, body BLOB NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID`); err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		f.export, mb.export = db, db
	}
	shelves := func(context.Context) []musictags.ShelfItem {
		var out []musictags.ShelfItem
		for _, region := range []string{"global", "us", "gb", "de", "br", "mx", "ca", "au", "fr", "es", "it", "nl", "se", "jp", "in", "kr"} {
			b, err := ours.get(*api + "/v1/music/lists/shelves?region=" + region)
			if err != nil {
				continue
			}
			var sh struct {
				Blocks map[string][]struct {
					Kind       string `json:"kind"`
					MBID       string `json:"mbid"`
					ArtistMBID string `json:"artist_mbid"`
					Title      string `json:"title"`
					Artist     string `json:"artist_name"`
					Date       string `json:"release_date"`
				} `json:"blocks"`
			}
			if json.Unmarshal(b, &sh) != nil {
				continue
			}
			for _, l := range sh.Blocks {
				for _, it := range l {
					out = append(out, musictags.ShelfItem{Kind: it.Kind, MBID: it.MBID, ArtistMBID: it.ArtistMBID, Title: it.Title, Artist: it.Artist, Date: it.Date})
				}
			}
		}
		return out
	}
	recipes, err := musictags.Recipes()
	if err != nil {
		log.Fatal(err)
	}
	src, err := musictags.Collect(context.Background(), f, mb, musictags.Config{LB: *lb, MB: *mbRoot, Keep: *keep, MaxArtists: *maxArtists, Charts: charts, Shelves: shelves, Log: log.Printf,
		SearchTags: func(src *musictags.Source) []string { return musictags.SmallKeyTags(src, recipes, time.Now(), 300, 3) }})
	if err != nil {
		log.Fatal(err)
	}
	rs, err := musictags.Recipes()
	if err != nil {
		log.Fatal(err)
	}
	bundle := musictags.Build(src, rs, time.Now())
	b, _ := json.Marshal(bundle)
	_ = os.WriteFile(filepath.Join(*out, "bundle.json"), b, 0o644)
	stats := map[string][2]int{}
	for k, l := range bundle.Keys {
		var s [2]int
		for _, m := range l {
			if bundle.Items[m.I].Kind == "a" {
				s[0]++
			} else {
				s[1]++
			}
		}
		stats[k] = s
	}
	sb, _ := json.MarshalIndent(stats, "", " ")
	_ = os.WriteFile(filepath.Join(*out, "stats.json"), sb, 0o644)
	log.Printf("bundle: %d items, %d keys, %d bytes, version %s, %d calls, %s", len(bundle.Items), len(bundle.Keys), len(b), bundle.Version, f.calls+ours.calls, time.Since(start).Round(time.Second))
}
