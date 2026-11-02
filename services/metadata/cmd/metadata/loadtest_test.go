//go:build loadtest

// Lab load test (music speed plan Phase 6), local only, never the live VM:
//
//	go test -tags loadtest -run TestLabLoad -timeout 30m ./cmd/metadata -v
//
// A real server build (store, provider client, queue, picture route with
// WebP, edge writer) runs in process behind a caching proxy that keeps
// public answers by s-maxage (the Cloudflare model, one tier). A fake
// MusicBrainz answers 503 above 1 request a second. 1,000 installs each
// open 12 artist pages with pictures (and some album pages), chosen on a
// Zipf curve over 20,000 artists; the top 1,000 are pre-stored (the P2
// shared fill, pictures with WebP made by the image worker). A 202 partial is re-asked after Retry-After, as the app does.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/store"
)

const (
	labInstalls  = 1000
	labArtists   = 20000
	labPrestored = 1000
	labAlbums    = 2
)

func labArtist(i int) string { return fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i) }
func labAlbum(i, j int) string {
	return fmt.Sprintf("%08x-%04x-4000-8000-%012x", i, j+1, i)
}

type labImages struct{ dir string }

func (l labImages) Get(_ context.Context, kind, mbid string, size int) (string, string, bool) {
	p := filepath.Join(l.dir, kind+"-"+mbid+"-"+strconv.Itoa(size)+".jpg")
	if _, err := os.Stat(p); err != nil {
		return "", "", false
	}
	return p, "v1", true
}

type edgeEntry struct {
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

func TestLabLoad(t *testing.T) {
	dir := t.TempDir()
	var mbMu sync.Mutex
	var mbLast time.Time
	var mbCalls, mb503 atomic.Int64
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mbCalls.Add(1)
		mbMu.Lock()
		ok := time.Since(mbLast) >= time.Second
		if ok {
			mbLast = time.Now()
		}
		mbMu.Unlock()
		if !ok {
			mb503.Add(1)
			w.WriteHeader(503)
			return
		}
		time.Sleep(200 * time.Millisecond)
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/artist/"):
			fmt.Fprintf(w, `{"id":%q,"name":"Lab artist","relations":[]}`, strings.TrimPrefix(p, "/artist/"))
		case p == "/release-group":
			fmt.Fprint(w, `{"release-group-count":0,"release-groups":[]}`)
		case strings.HasPrefix(p, "/release-group/"):
			fmt.Fprintf(w, `{"id":%q,"title":"Lab album","releases":[]}`, strings.TrimPrefix(p, "/release-group/"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer mb.Close()

	st, err := store.Open(filepath.Join(dir, "data"), store.Options{MaxBytes: 1 << 34, MaxEntries: 10000000})
	if err != nil {
		t.Fatal(err)
	}
	client, err := providers.New(providers.Options{Providers: map[string]providers.Config{"musicbrainz": {BaseURL: mb.URL, Interval: 1100 * time.Millisecond}}, AllowPrivate: true, Attempts: 2, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	imgDir := filepath.Join(dir, "img")
	os.MkdirAll(imgDir, 0o755)
	tpl := map[int][]byte{}
	for _, size := range []int{250, 500} {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x * y), 255})
			}
		}
		var b bytes.Buffer
		_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 82})
		tpl[size] = b.Bytes()
	}
	now := time.Now().UTC()
	put := func(kind, id, body string) {
		e := model.Entry{Key: api.MBStoreKey(kind, id), Result: model.Result{Data: json.RawMessage(body), Complete: true}, FetchedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
		if err := st.Put(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < labPrestored; i++ {
		put("artist", labArtist(i), fmt.Sprintf(`{"source":"musicbrainz","source_id":%q,"kind":"artist","title":"Lab artist","source_data":{"release-groups":[]}}`, labArtist(i)))
		ap := filepath.Join(imgDir, "artist-"+labArtist(i)+"-500.jpg")
		os.WriteFile(ap, tpl[500], 0o644)
		_, _ = imgstore.WebPOf(context.Background(), ap) // as the image worker does
		for j := 0; j < labAlbums; j++ {
			put("release-group", labAlbum(i, j), fmt.Sprintf(`{"source":"musicbrainz","source_id":%q,"kind":"release-group","title":"Lab album"}`, labAlbum(i, j)))
			bp := filepath.Join(imgDir, "album-"+labAlbum(i, j)+"-250.jpg")
			os.WriteFile(bp, tpl[250], 0o644)
			_, _ = imgstore.WebPOf(context.Background(), bp)
		}
	}

	handler := api.New(st, client, api.Options{ClientPerMinute: 100000, ClientBurst: 10000, PerMinute: 1000000, Burst: 100000, MaxConcurrent: 1000,
		Images: &api.Images{Store: labImages{imgDir}, WebP: imgstore.WebPOf, Redirect: func() bool { return true }}})
	q, err := fetchq.Open(filepath.Join(dir, "fetchq.sqlite"), fetchq.Options{Name: "musicbrainz", Rate: 0.9, Shares: [5]float64{0.6, 0.15, 0.15, 0.07, 0.03}, Do: handler.RunJob})
	if err != nil {
		t.Fatal(err)
	}
	handler.SetQueue(q)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); time.Sleep(200 * time.Millisecond); q.Close(); st.Close() }()
	go q.Run(ctx)
	origin := httptest.NewServer(handler)
	defer origin.Close()

	var edgeMu, latMu sync.Mutex
	edge := map[string]*edgeEntry{}
	var hits, misses, uncacheable atomic.Int64
	var originLat, lat []time.Duration
	originClient := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 256}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.RequestURI()
		edgeMu.Lock()
		e := edge[key]
		edgeMu.Unlock()
		if e != nil && time.Now().Before(e.expires) {
			hits.Add(1)
			for k, v := range e.header {
				w.Header()[k] = v
			}
			w.WriteHeader(e.status)
			w.Write(e.body)
			return
		}
		misses.Add(1)
		start := time.Now()
		req, _ := http.NewRequest("GET", origin.URL+key, nil)
		req.Header = r.Header.Clone()
		resp, err := originClient.Do(req)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		latMu.Lock()
		originLat = append(originLat, time.Since(start))
		latMu.Unlock()
		cc := resp.Header.Get("Cache-Control")
		if !strings.Contains(cc, "s-maxage=") || !strings.HasPrefix(cc, "public") {
			uncacheable.Add(1) // 202 partial, placeholder, negotiated
		}
		if strings.HasPrefix(cc, "public") && (resp.StatusCode == 200 || resp.StatusCode == 302) {
			for _, part := range strings.Split(cc, ",") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(part), "s-maxage="); ok {
					if n, _ := strconv.Atoi(v); n > 0 {
						edgeMu.Lock()
						edge[key] = &edgeEntry{resp.StatusCode, resp.Header.Clone(), body, time.Now().Add(time.Duration(n) * time.Second)}
						edgeMu.Unlock()
					}
				}
			}
		}
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	}))
	defer front.Close()

	var peakSys, peakHeap uint64
	var goroutinesPeak int
	stopSample := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		var m runtime.MemStats
		for {
			select {
			case <-stopSample:
				return
			case <-time.After(500 * time.Millisecond):
			}
			runtime.ReadMemStats(&m)
			peakSys, peakHeap = max(peakSys, m.Sys), max(peakHeap, m.HeapInuse)
			goroutinesPeak = max(goroutinesPeak, runtime.NumGoroutine())
		}
	}()

	zipf := rand.NewZipf(rand.New(rand.NewPCG(1, 2)), 1.1, 8, labArtists-1)
	var zmu sync.Mutex
	pick := func() int { zmu.Lock(); defer zmu.Unlock(); return int(zipf.Uint64()) }
	cl := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 2048}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var total, errs, partial, retried atomic.Int64
	get := func(path string) int {
		start := time.Now()
		resp, err := cl.Get(front.URL + path)
		d := time.Since(start)
		total.Add(1)
		latMu.Lock()
		lat = append(lat, d)
		latMu.Unlock()
		if err != nil {
			errs.Add(1)
			return 0
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			errs.Add(1)
		}
		if resp.StatusCode == 202 {
			partial.Add(1)
		}
		return resp.StatusCode
	}
	t0 := time.Now()
	var wg sync.WaitGroup
	for n := 0; n < labInstalls; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(n), 7))
			time.Sleep(time.Duration(r.IntN(60000)) * time.Millisecond)
			for page := 0; page < 12; page++ {
				a := pick()
				id := labArtist(a)
				if get("/v1/music/musicbrainz/artist/"+id) == 202 {
					time.Sleep(2 * time.Second)
					retried.Add(1)
					get("/v1/music/musicbrainz/artist/" + id)
				}
				get("/v1/img/artist/" + id + "/500.webp?v=v1")
				for j := 0; j < labAlbums; j++ {
					get("/v1/img/album/" + labAlbum(a, j) + "/250.webp?v=v1")
				}
				if page%3 == 0 {
					get("/v1/music/musicbrainz/release-group/" + labAlbum(a, 0))
				}
				time.Sleep(time.Duration(300+r.IntN(1200)) * time.Millisecond)
			}
		}(n)
	}
	wg.Wait()
	elapsed := time.Since(t0)
	close(stopSample)
	<-sampled
	pct := func(d []time.Duration, p float64) time.Duration {
		if len(d) == 0 {
			return 0
		}
		sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
		return d[int(float64(len(d)-1)*p)].Round(10 * time.Microsecond)
	}
	h, m := hits.Load(), misses.Load()
	fmt.Printf("LOAD installs=%d requests=%d elapsed=%s rps=%.0f\n", labInstalls, total.Load(), elapsed.Round(time.Second), float64(total.Load())/elapsed.Seconds())
	fmt.Printf("LOAD all p50=%s p95=%s p99=%s\n", pct(lat, .5), pct(lat, .95), pct(lat, .99))
	fmt.Printf("LOAD origin(edge miss) n=%d p50=%s p95=%s p99=%s\n", len(originLat), pct(originLat, .5), pct(originLat, .95), pct(originLat, .99))
	fmt.Printf("LOAD errors=%d (%.3f%%) partial202=%d retried=%d\n", errs.Load(), 100*float64(errs.Load())/float64(total.Load()), partial.Load(), retried.Load())
	fmt.Printf("LOAD edge hit=%d miss=%d hit_rate=%.1f%%\n", h, m, 100*float64(h)/float64(h+m))
	fmt.Printf("LOAD edge uncacheable=%d hit_rate_of_cacheable=%.1f%%\n", uncacheable.Load(), 100*float64(h)/float64(h+m-uncacheable.Load()))
	fmt.Printf("LOAD mb calls=%d 503=%d\n", mbCalls.Load(), mb503.Load())
	fmt.Printf("LOAD mem peak_sys=%dMiB peak_heap=%dMiB goroutines_peak=%d\n", peakSys>>20, peakHeap>>20, goroutinesPeak)
	if errs.Load() > total.Load()/100 {
		t.Errorf("error rate over 1%%")
	}
}
