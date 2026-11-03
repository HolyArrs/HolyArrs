// Package stats keeps aggregate service counters in 5-minute buckets for the
// dashboard. It never records tokens, client addresses or search text.
package stats

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	BucketSize = 5 * time.Minute
	Retention  = 7 * 24 * time.Hour
)

// Latency bin upper bounds in milliseconds; the last bin is open-ended.
var bounds = []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000}

type Options struct {
	Now  func() time.Time
	Size func() int64
	// ItemTTL drops item records older than the cache answer lifetime;
	// MaxItems caps the table (oldest removed first). Defaults 7 days, 500000.
	ItemTTL  time.Duration
	MaxItems int
	// Logf reports a failed flush or read with its cause (default log.Printf,
	// at most once a minute).
	Logf func(string, ...any)
}

type bucket struct {
	Requests       int64                       `json:"requests"`
	Successes      int64                       `json:"successes"`
	Failures       int64                       `json:"failures"`
	ClientErrors   int64                       `json:"client_errors"`
	RateLimited    int64                       `json:"rate_limited"`
	CacheHits      int64                       `json:"cache_hits"`
	CacheMisses    int64                       `json:"cache_misses"`
	LatencySumMS   float64                     `json:"latency_sum_ms"`
	Histogram      []int64                     `json:"histogram"`
	ProviderErrors map[string]map[string]int64 `json:"provider_errors"`
	ItemsAdded     map[string]int64            `json:"items_added"`
	StorageBytes   int64                       `json:"storage_bytes"`
}

func newBucket() *bucket {
	return &bucket{Histogram: make([]int64, len(bounds)+1), ProviderErrors: map[string]map[string]int64{}, ItemsAdded: map[string]int64{}}
}

func (b *bucket) merge(o *bucket) {
	b.Requests += o.Requests
	b.Successes += o.Successes
	b.Failures += o.Failures
	b.ClientErrors += o.ClientErrors
	b.RateLimited += o.RateLimited
	b.CacheHits += o.CacheHits
	b.CacheMisses += o.CacheMisses
	b.LatencySumMS += o.LatencySumMS
	for i := range b.Histogram {
		if i < len(o.Histogram) {
			b.Histogram[i] += o.Histogram[i]
		}
	}
	for p, codes := range o.ProviderErrors {
		if b.ProviderErrors[p] == nil {
			b.ProviderErrors[p] = map[string]int64{}
		}
		for c, n := range codes {
			b.ProviderErrors[p][c] += n
		}
	}
	for k, n := range o.ItemsAdded {
		b.ItemsAdded[k] += n
	}
	b.StorageBytes = max(b.StorageBytes, o.StorageBytes)
}

func (b *bucket) p95() float64 {
	total := int64(0)
	for _, n := range b.Histogram {
		total += n
	}
	if total == 0 {
		return 0
	}
	need := (total*95 + 99) / 100
	seen := int64(0)
	for i, n := range b.Histogram {
		seen += n
		if seen >= need {
			if i < len(bounds) {
				return bounds[i]
			}
			return bounds[len(bounds)-1] * 2
		}
	}
	return 0
}

type Recorder struct {
	db      *sql.DB
	opts    Options
	started time.Time
	mu      sync.Mutex
	open    map[int64]*bucket
	done    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
	// flushErr is the last flush failure with its cause ("" once one works).
	flushErr string
	logged   time.Time
}

// fail keeps a failure's cause, logs it at most once a minute, and answers
// err with the cause wrapped (the API still shows only a code). Call with
// r.mu held.
func (r *Recorder) fail(what string, err error) error {
	e := fmt.Errorf("%s: %w", what, err)
	if r.opts.Now().Sub(r.logged) >= time.Minute {
		r.logged = r.opts.Now()
		r.opts.Logf("stats: %v", e)
	}
	return e
}

func Open(path string, opts Options) (*Recorder, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.ItemTTL <= 0 {
		opts.ItemTTL = Retention
	}
	if opts.MaxItems <= 0 {
		opts.MaxItems = 500000
	}
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	// The pragmas go in the address too, so a connection the pool opens
	// again keeps its busy wait.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, errors.New("cannot open statistics database")
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=5000", "PRAGMA cache_size=-1024",
		"CREATE TABLE IF NOT EXISTS buckets (start INTEGER PRIMARY KEY, data TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS items (key TEXT PRIMARY KEY, kind TEXT NOT NULL, added INTEGER NOT NULL) WITHOUT ROWID",
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, errors.New("cannot initialize statistics database")
		}
	}
	// Item title and provider came later; older rows keep empty values.
	for _, col := range []string{"title", "provider"} {
		_, _ = db.Exec("ALTER TABLE items ADD COLUMN " + col + " TEXT NOT NULL DEFAULT ''")
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS items_by_added ON items(added)"); err != nil {
		db.Close()
		return nil, errors.New("cannot initialize statistics database")
	}
	// Lets the item count by window and kind read the index alone; only a speedup.
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS items_by_added_kind ON items(added, kind)")
	return &Recorder{db: db, opts: opts, started: opts.Now().UTC(), open: map[int64]*bucket{}, done: make(chan struct{})}, nil
}

// Run flushes buckets periodically until Close.
func (r *Recorder) Run(every time.Duration) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-r.done:
				return
			case <-t.C:
				_ = r.Flush()
			}
		}
	}()
}

func (r *Recorder) Close() error {
	var err error
	r.once.Do(func() {
		close(r.done)
		r.wg.Wait()
		err = errors.Join(r.Flush(), r.db.Close())
	})
	return err
}

func (r *Recorder) current() *bucket {
	start := r.opts.Now().UTC().Truncate(BucketSize).Unix()
	b := r.open[start]
	if b == nil {
		b = newBucket()
		r.open[start] = b
	}
	return b
}

// KindGroup maps a query kind to its dashboard group. Kinds that are not a
// movie, show or music identity (TMDB raw reads, lists) return "".
func KindGroup(kind string) string {
	switch kind {
	case "movie", "movies":
		return "movies"
	case "show", "shows":
		return "shows"
	case "artist", "release-group", "release", "recording", "music":
		return "music"
	}
	return ""
}

// Request records one API request. cache is "hit", "miss", "stale", "bypass" or empty.
func (r *Recorder) Request(status int, cache string, d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.current()
	b.Requests++
	switch {
	case status == 429:
		b.RateLimited++
	case status >= 500:
		b.Failures++
	case status >= 400:
		b.ClientErrors++
	default:
		b.Successes++
	}
	switch cache {
	case "hit", "stale":
		b.CacheHits++
	case "miss", "bypass":
		b.CacheMisses++
	}
	b.LatencySumMS += ms
	i := sort.SearchFloat64s(bounds, ms)
	b.Histogram[i]++
}

func (r *Recorder) ProviderError(provider, code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.current()
	if b.ProviderErrors[provider] == nil {
		b.ProviderErrors[provider] = map[string]int64{}
	}
	b.ProviderErrors[provider][code]++
}

// Stored records a saved answer; only answers new to the cache count as added.
func (r *Recorder) Stored(key, kind string) { r.StoredItem(key, kind, "", "") }

// StoredItem is Stored with the item's provider and display title. Answers
// whose kind has no group (searches, raw reads, lists) are not items.
func (r *Recorder) StoredItem(key, kind, provider, title string) {
	g := KindGroup(kind)
	if g == "" {
		return
	}
	if len(title) > 200 {
		title = title[:200]
	}
	if len(provider) > 40 {
		provider = provider[:40]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	res, err := r.db.Exec("INSERT OR IGNORE INTO items(key,kind,added,title,provider) VALUES(?,?,?,?,?)", key, g, r.opts.Now().Unix(), title, provider)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n == 1 {
		r.current().ItemsAdded[g]++
	}
}

// Flush merges in-memory buckets into the database and prunes expired history.
func (r *Recorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.flushLocked()
	if err != nil {
		r.flushErr = err.Error()
		return r.fail("flush", err)
	}
	r.flushErr = ""
	return nil
}

func (r *Recorder) flushLocked() error {
	if r.opts.Size != nil {
		r.current().StorageBytes = r.opts.Size()
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	for start, b := range r.open {
		merged := newBucket()
		var raw string
		if tx.QueryRow("SELECT data FROM buckets WHERE start=?", start).Scan(&raw) == nil {
			_ = json.Unmarshal([]byte(raw), merged)
			if len(merged.Histogram) != len(bounds)+1 {
				merged.Histogram = make([]int64, len(bounds)+1)
			}
			if merged.ProviderErrors == nil {
				merged.ProviderErrors = map[string]map[string]int64{}
			}
			if merged.ItemsAdded == nil {
				merged.ItemsAdded = map[string]int64{}
			}
		}
		merged.merge(b)
		data, _ := json.Marshal(merged)
		if _, err := tx.Exec("INSERT INTO buckets(start,data) VALUES(?,?) ON CONFLICT(start) DO UPDATE SET data=excluded.data", start, string(data)); err != nil {
			return fmt.Errorf("write bucket: %w", err)
		}
	}
	cutoff := r.opts.Now().Add(-Retention).Unix()
	if _, err := tx.Exec("DELETE FROM buckets WHERE start < ?", cutoff); err != nil {
		return fmt.Errorf("prune buckets: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM items WHERE added < ?", r.opts.Now().Add(-r.opts.ItemTTL).Unix()); err != nil {
		return fmt.Errorf("prune old items: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM items WHERE key IN (SELECT key FROM items ORDER BY added DESC, key DESC LIMIT -1 OFFSET ?)", r.opts.MaxItems); err != nil {
		return fmt.Errorf("prune items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	r.open = map[int64]*bucket{}
	return nil
}

type Point struct {
	Start          string                      `json:"start"`
	Requests       int64                       `json:"requests"`
	Successes      int64                       `json:"successes"`
	Failures       int64                       `json:"failures"`
	ClientErrors   int64                       `json:"client_errors"`
	RateLimited    int64                       `json:"rate_limited"`
	CacheHits      int64                       `json:"cache_hits"`
	CacheMisses    int64                       `json:"cache_misses"`
	AvgMS          float64                     `json:"avg_ms"`
	P95MS          float64                     `json:"p95_ms"`
	ProviderErrors map[string]map[string]int64 `json:"provider_errors"`
	ItemsAdded     map[string]int64            `json:"items_added"`
	StorageBytes   int64                       `json:"storage_bytes"`
}

type Snapshot struct {
	SchemaVersion    int              `json:"schema_version"`
	Status           string           `json:"status"`
	StartedAt        string           `json:"started_at"`
	GeneratedAt      string           `json:"generated_at"`
	UptimeSeconds    int64            `json:"uptime_seconds"`
	BucketSeconds    int64            `json:"bucket_seconds"`
	RetentionSeconds int64            `json:"retention_seconds"`
	WindowSeconds    int64            `json:"window_seconds"`
	Totals           Point            `json:"totals"`
	ItemsStored      map[string]int64 `json:"items_stored"`
	StorageBytes     int64            `json:"storage_bytes"`
	Series           []Point          `json:"series"`
	// FlushError is the last failed write of the figures with its cause
	// (the figures not written yet are still counted in this answer).
	FlushError string `json:"flush_error,omitempty"`
	// Trailers is trailer health (set by the API when trailers are on).
	Trailers any `json:"trailers,omitempty"`
	// Wikidata is lookups and the award-winner lists (when on).
	Wikidata any `json:"wikidata,omitempty"`
	// ServiceTop is each streaming service's list state (ok, stale,
	// key_invalid, quota_paused, last_success_at, last_error).
	ServiceTop any `json:"service_top,omitempty"`
	// Extras is each keyed extra source's state (ok, disabled_no_key).
	Extras any `json:"extras,omitempty"`
	// Refresh is the change-driven refresh state: store size, stale rows,
	// changes applied, upstream calls saved, feed states and disk warnings.
	Refresh any `json:"refresh,omitempty"`
	// InstallCountries is installs used in the last 30 days per country
	// (days, total, countries), set by the API. Counts only.
	InstallCountries any `json:"install_countries,omitempty"`
}

func point(start int64, b *bucket) Point {
	p := Point{Requests: b.Requests, Successes: b.Successes, Failures: b.Failures, ClientErrors: b.ClientErrors, RateLimited: b.RateLimited, CacheHits: b.CacheHits, CacheMisses: b.CacheMisses, P95MS: b.p95(), ProviderErrors: b.ProviderErrors, ItemsAdded: b.ItemsAdded, StorageBytes: b.StorageBytes}
	if start != 0 {
		p.Start = time.Unix(start, 0).UTC().Format(time.RFC3339)
	}
	if b.Requests > 0 {
		p.AvgMS = float64(int64(b.LatencySumMS/float64(b.Requests)*100)) / 100
	}
	return p
}

// Snapshot flushes, then returns totals and the sparse series for the window.
func (r *Recorder) Snapshot(window time.Duration) (Snapshot, error) {
	if window <= 0 || window > Retention {
		window = Retention
	}
	// A failed write never hides the figures: what is not written yet is
	// added from memory below, and the cause is shown (flush_error).
	_ = r.Flush()
	r.mu.Lock()
	flushErr := r.flushErr
	pending := map[int64]*bucket{}
	for start, b := range r.open {
		c := newBucket()
		c.merge(b)
		pending[start] = c
	}
	r.mu.Unlock()
	now := r.opts.Now().UTC()
	from := now.Add(-window).Truncate(BucketSize).Unix()
	rows, err := r.db.Query("SELECT start,data FROM buckets WHERE start >= ? ORDER BY start", from)
	if err != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		return Snapshot{}, r.fail("read buckets", err)
	}
	defer rows.Close()
	s := Snapshot{SchemaVersion: 1, Status: "ok", FlushError: flushErr, StartedAt: r.started.Format(time.RFC3339), GeneratedAt: now.Format(time.RFC3339), UptimeSeconds: int64(now.Sub(r.started).Seconds()), BucketSeconds: int64(BucketSize / time.Second), RetentionSeconds: int64(Retention / time.Second), WindowSeconds: int64(window / time.Second), Series: []Point{}, ItemsStored: map[string]int64{"movies": 0, "shows": 0, "music": 0, "total": 0}}
	total := newBucket()
	for rows.Next() {
		var start int64
		var raw sql.NullString
		if err := rows.Scan(&start, &raw); err != nil {
			r.mu.Lock()
			defer r.mu.Unlock()
			return Snapshot{}, r.fail("read bucket", err)
		}
		b := newBucket()
		if json.Unmarshal([]byte(raw.String), b) != nil {
			b = newBucket()
		}
		if len(b.Histogram) != len(bounds)+1 {
			b.Histogram = make([]int64, len(bounds)+1)
		}
		if b.ProviderErrors == nil {
			b.ProviderErrors = map[string]map[string]int64{}
		}
		if b.ItemsAdded == nil {
			b.ItemsAdded = map[string]int64{}
		}
		if p := pending[start]; p != nil {
			b.merge(p)
			delete(pending, start)
		}
		s.Series = append(s.Series, point(start, b))
		total.merge(b)
		if b.StorageBytes > 0 {
			s.StorageBytes = b.StorageBytes
		}
	}
	if err := rows.Err(); err != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		return Snapshot{}, r.fail("read buckets", err)
	}
	var left []int64
	for start := range pending {
		if start >= from {
			left = append(left, start)
		}
	}
	sort.Slice(left, func(a, b int) bool { return left[a] < left[b] })
	for _, start := range left {
		b := pending[start]
		s.Series = append(s.Series, point(start, b))
		total.merge(b)
		if b.StorageBytes > 0 {
			s.StorageBytes = b.StorageBytes
		}
	}
	s.Totals = point(0, total)
	s.Totals.StorageBytes = s.StorageBytes
	kinds, err := r.db.Query("SELECT kind,COUNT(*) FROM items GROUP BY kind")
	if err != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		return Snapshot{}, r.fail("read items", err)
	}
	defer kinds.Close()
	for kinds.Next() {
		var k string
		var n int64
		if kinds.Scan(&k, &n) == nil {
			s.ItemsStored[k] = n
			s.ItemsStored["total"] += n
		}
	}
	return s, nil
}

type Item struct {
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	AddedAt  string `json:"added_at"`
}

type ItemPage struct {
	WindowSeconds int64  `json:"window_seconds"`
	Kind          string `json:"kind"`
	Total         int64  `json:"total"`
	Offset        int    `json:"offset"`
	Limit         int    `json:"limit"`
	Items         []Item `json:"items"`
}

// Items lists items first added to the cache within window, newest first.
// group is "" for all kinds, or "movies", "shows" or "music".
func (r *Recorder) Items(window time.Duration, group string, limit, offset int) (ItemPage, error) {
	if window <= 0 || window > Retention {
		window = Retention
	}
	p := ItemPage{WindowSeconds: int64(window / time.Second), Kind: group, Offset: offset, Limit: limit, Items: []Item{}}
	from := r.opts.Now().Add(-window).Unix()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db.QueryRow("SELECT COUNT(*) FROM items WHERE added >= ? AND (?='' OR kind=?)", from, group, group).Scan(&p.Total) != nil {
		return ItemPage{}, errors.New("statistics read unavailable")
	}
	rows, err := r.db.Query("SELECT title,kind,provider,added FROM items WHERE added >= ? AND (?='' OR kind=?) ORDER BY added DESC, key DESC LIMIT ? OFFSET ?", from, group, group, limit, offset)
	if err != nil {
		return ItemPage{}, errors.New("statistics read unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var it Item
		var added int64
		if rows.Scan(&it.Title, &it.Kind, &it.Provider, &added) != nil {
			return ItemPage{}, errors.New("statistics read failed")
		}
		it.AddedAt = time.Unix(added, 0).UTC().Format(time.RFC3339)
		p.Items = append(p.Items, it)
	}
	return p, rows.Err()
}
