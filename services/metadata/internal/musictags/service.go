package musictags

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Options configure the catalog job.
type Options struct {
	HTTP   *http.Client // ListenBrainz (budgeted)
	MBHTTP *http.Client // MusicBrainz (paced with every other MusicBrainz call)
	LBRoot string       // default https://api.listenbrainz.org/1
	MBRoot string       // default https://musicbrainz.org/ws/2
	// UserAgent names the service on every call.
	UserAgent string
	// Charts answers today's country charts.
	Charts func(ctx context.Context) []ChartRow
	// Shelves answers the list shelves' items (nil: none).
	Shelves func(ctx context.Context) []ShelfItem
	// MBPerRun caps MusicBrainz calls that miss the cache in one run
	// (default 800: the first fill spreads over three days); -1 means none.
	MBPerRun int
	// MBGap is the extra wait after each MusicBrainz call, leaving pacing
	// slots to live lookups (default 1 s).
	MBGap time.Duration
	Now   func() time.Time
}

// ErrBudget is a call skipped because the run's MusicBrainz cap is spent.
var ErrBudget = errors.New("musictags: MusicBrainz budget for this run is spent")

// Service builds and serves the bundle.
type Service struct {
	db   *sql.DB
	o    Options
	path string

	mu    sync.RWMutex
	body  []byte
	etag  string
	built time.Time
	// tried is the last build (published or refused); refused says why the
	// last one was not published ("" when it was).
	tried   time.Time
	refused string
	items   int

	run  sync.Mutex
	seen []string // URLs called upstream (tests)

	albumsTag string
	albums    []AlbumRef
	artists   []string
}

// AlbumRef is one catalog album: its release group and names.
type AlbumRef struct {
	MBID, Artist, Title string
	Year                int // first release year (0 unknown)
}

// Albums answers the bundle's albums: charting in the most countries first,
// then the most played (the order their covers are stored ahead in).
func (s *Service) Albums() []AlbumRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.albumsTag == s.etag && s.albums != nil || len(s.body) == 0 {
		return s.albums
	}
	var d struct {
		Items []struct {
			ID      string         `json:"id"`
			Kind    string         `json:"k"`
			Title   string         `json:"t"`
			Artist  string         `json:"ar"`
			Listens int64          `json:"l"`
			Date    string         `json:"d"`
			Charts  map[string]int `json:"c"`
		} `json:"items"`
	}
	if json.Unmarshal(s.body, &d) != nil {
		return s.albums
	}
	items := d.Items[:0]
	var artists []string
	for _, it := range d.Items {
		if it.Kind == "a" && it.ID != "" {
			items = append(items, it)
		}
		if it.Kind == "r" && it.ID != "" {
			artists = append(artists, it.ID)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if len(items[i].Charts) != len(items[j].Charts) {
			return len(items[i].Charts) > len(items[j].Charts)
		}
		return items[i].Listens > items[j].Listens
	})
	out := make([]AlbumRef, len(items))
	for i, it := range items {
		y := 0
		if len(it.Date) >= 4 {
			y, _ = strconv.Atoi(it.Date[:4])
		}
		out[i] = AlbumRef{MBID: it.ID, Artist: it.Artist, Title: it.Title, Year: y}
	}
	s.albums, s.artists, s.albumsTag = out, artists, s.etag
	return out
}

// Artists answers the bundle's artists (MusicBrainz ids, bundle order).
func (s *Service) Artists() []string {
	s.Albums()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.artists
}

// Open opens the store and loads the last bundle.
func Open(path string, o Options) (*Service, error) {
	if o.LBRoot == "" {
		o.LBRoot = "https://api.listenbrainz.org/1"
	}
	if o.MBRoot == "" {
		o.MBRoot = "https://musicbrainz.org/ws/2"
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	if o.MBHTTP == nil {
		o.MBHTTP = o.HTTP
	}
	if o.MBPerRun == 0 {
		o.MBPerRun = 800
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS mt_cache (url TEXT PRIMARY KEY, body BLOB NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS mt_bundle (id INTEGER PRIMARY KEY CHECK (id = 1), version TEXT NOT NULL, built INTEGER NOT NULL, body BLOB NOT NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	s := &Service{db: db, o: o, path: path}
	var v string
	var at int64
	var body []byte
	if err := db.QueryRow(`SELECT version, built, body FROM mt_bundle WHERE id = 1`).Scan(&v, &at, &body); err == nil {
		s.body, s.etag, s.built = body, `"`+v+`"`, time.Unix(at, 0)
	}
	return s, nil
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

// Pools answers the bundle and its ETag.
func (s *Service) Pools() ([]byte, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.body, s.etag, len(s.body) > 0
}

// Built answers when the bundle was built.
func (s *Service) Built() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.built
}

// ttl is how long a cached answer is good for, by kind of call.
func ttl(u string) time.Duration {
	switch {
	case strings.Contains(u, "/release-group?artist="), strings.Contains(u, "/release-group?query="):
		return 7 * 24 * time.Hour
	case strings.Contains(u, "/metadata/release_group"):
		return 30 * 24 * time.Hour
	case strings.Contains(u, "/popularity/"):
		return 6 * 24 * time.Hour
	}
	return 20 * time.Hour
}

type cacheFetch struct {
	s      *Service
	client *http.Client
	mb     bool
	misses int
}

func (f *cacheFetch) Get(ctx context.Context, u string) ([]byte, error) {
	return f.do(ctx, http.MethodGet, u, nil)
}

func (f *cacheFetch) Post(ctx context.Context, u string, body []byte) ([]byte, error) {
	return f.do(ctx, http.MethodPost, u, body)
}

func (f *cacheFetch) do(ctx context.Context, method, u string, body []byte) ([]byte, error) {
	key := u
	if body != nil {
		h := sha1.Sum(body)
		key = u + "#" + hex.EncodeToString(h[:])
	}
	now := f.s.o.Now()
	var b []byte
	var at int64
	if err := f.s.db.QueryRowContext(ctx, `SELECT body, at FROM mt_cache WHERE url = ?`, key).Scan(&b, &at); err == nil && now.Sub(time.Unix(at, 0)) < ttl(u) {
		return b, nil
	}
	if f.mb {
		if f.s.o.MBPerRun < 0 || f.misses >= f.s.o.MBPerRun {
			return nil, ErrBudget
		}
		f.misses++
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.s.o.UserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	f.s.seen = append(f.s.seen, u)
	resp, err := f.client.Do(req)
	if f.mb && f.s.o.MBGap > 0 {
		select {
		case <-time.After(f.s.o.MBGap):
		case <-ctx.Done():
		}
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		b = []byte("null")
	case resp.StatusCode != http.StatusOK:
		return nil, errors.New("musictags: " + method + " answered " + resp.Status)
	}
	_, _ = f.s.db.ExecContext(ctx, `INSERT INTO mt_cache (url, body, at) VALUES (?, ?, ?) ON CONFLICT(url) DO UPDATE SET body = excluded.body, at = excluded.at`, key, b, now.Unix())
	return b, nil
}

// Refresh collects, builds and stores a new bundle.
func (s *Service) Refresh(ctx context.Context) error {
	s.run.Lock()
	defer s.run.Unlock()
	rs, err := Recipes()
	if err != nil {
		return err
	}
	lb := &cacheFetch{s: s, client: s.o.HTTP}
	mb := &cacheFetch{s: s, client: s.o.MBHTTP, mb: true}
	now0 := s.o.Now()
	src, err := Collect(ctx, lb, mb, Config{LB: s.o.LBRoot, MB: s.o.MBRoot, Keep: 10, Charts: s.o.Charts, Shelves: s.o.Shelves, Log: log.Printf,
		SearchTags: func(src *Source) []string { return SmallKeyTags(src, rs, now0, 300, 3) }})
	if err != nil {
		return err
	}
	b := Build(src, rs, s.o.Now())
	body, err := json.Marshal(b)
	if err != nil {
		return err
	}
	now := s.o.Now()
	prev := s.publishedItems()
	s.mu.Lock()
	s.tried = now
	s.mu.Unlock()
	if prev > 0 && float64(len(b.Items)) < float64(prev)*(1-MaxShrink) {
		// A build that lost more than MaxShrink of the items is a broken
		// collection (a source down), not the catalog changing: the
		// published bundle stays, and the next day's build tries again.
		why := fmt.Sprintf("bundle %s has %d items, %d published: more than %.0f%% fewer, kept the published one", b.Version, len(b.Items), prev, MaxShrink*100)
		s.mu.Lock()
		s.refused = why
		s.mu.Unlock()
		log.Printf("musictags: refused: %s", why)
		return ErrShrunk
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO mt_bundle (id, version, built, body) VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET version = excluded.version, built = excluded.built, body = excluded.body`, b.Version, now.Unix(), body); err != nil {
		return err
	}
	// Old cache rows (past every ttl) are dropped.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM mt_cache WHERE at < ?`, now.Add(-31*24*time.Hour).Unix())
	s.mu.Lock()
	s.body, s.etag, s.built = body, `"`+b.Version+`"`, now
	s.refused, s.items = "", len(b.Items)
	s.mu.Unlock()
	log.Printf("musictags: bundle %s: %d items, %d keys, %d bytes, %d MusicBrainz calls", b.Version, len(b.Items), len(b.Keys), len(body), mb.misses)
	return nil
}

// Run builds on start when the bundle is older than 20 hours, then every
// day after 06:30 UTC (after the chart and listen refreshes).
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for {
		now := s.o.Now().UTC()
		s.mu.RLock()
		tried := s.tried
		s.mu.RUnlock()
		due := s.Built().IsZero() || now.Sub(s.Built()) > 20*time.Hour && now.Sub(tried) > 20*time.Hour
		at := time.Date(now.Year(), now.Month(), now.Day(), 6, 30, 0, 0, time.UTC)
		if !due && now.After(at) && s.Built().Before(at) && tried.Before(at) {
			due = true
		}
		if due {
			if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
				log.Printf("musictags: refresh: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// MaxShrink is the largest drop in items a new bundle may have against the
// published one; a bigger drop is refused (ErrShrunk).
var MaxShrink = 0.10

// ErrShrunk: the new bundle lost more than MaxShrink of the items and was
// not published.
var ErrShrunk = errors.New("musictags: bundle shrank too much, not published")

// publishedItems counts the published bundle's items (0 when none).
func (s *Service) publishedItems() int {
	s.mu.RLock()
	n, body := s.items, s.body
	s.mu.RUnlock()
	if n > 0 || len(body) == 0 {
		return n
	}
	var b struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(body, &b) != nil {
		return 0
	}
	s.mu.Lock()
	s.items = len(b.Items)
	s.mu.Unlock()
	return len(b.Items)
}

// PublishStatus is the last build's outcome for /v1/stats: the published
// bundle's items and when, and why the last build was refused, if it was.
func (s *Service) PublishStatus() map[string]any {
	n := s.publishedItems()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]any{"items": n, "built": s.built.UTC().Format(time.RFC3339), "max_shrink": MaxShrink}
	if !s.tried.IsZero() {
		out["last_build"] = s.tried.UTC().Format(time.RFC3339)
	}
	if s.refused != "" {
		out["refused"] = s.refused
	}
	return out
}
