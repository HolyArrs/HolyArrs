// Package servicetop keeps a "top today" list per streaming service and
// region from the Streaming Availability API (Movie of the Night). Every
// good fetch is kept as a snapshot; a failed or suspect fetch serves the
// newest good snapshot instead. Peacock and Tubi, which the API does not cover, are
// TMDB's most popular titles on it.
package servicetop

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// BaseURL is the Streaming Availability API.
const BaseURL = "https://api.movieofthenight.com/v4"

// Attribution must be shown wherever these lists are shown publicly.
const Attribution = "Streaming data by Streaming Availability (Movie of the Night)"

// Sources of a list.
const (
	SourceSA      = "streaming-availability"
	SourcePopular = "tmdb-popular"
)

// States of a service, for /v1/stats.
const (
	StateOK          = "ok"
	StateStale       = "stale"
	StateKeyInvalid  = "key_invalid"
	StateQuotaPaused = "quota_paused"
)

const (
	// staleAfter marks a served snapshot stale (it is still served).
	staleAfter = 14 * 24 * time.Hour
	// keepFor is how long snapshots are kept (the newest good one always).
	keepFor = 30 * 24 * time.Hour
	// emptyRepeats is how many empty answers in a row are believed.
	emptyRepeats = 3
	// tries is how many attempts a failing fetch gets before it waits
	// for the next round; retryGap is the first wait, doubling.
	tries    = 3
	retryGap = 10 * time.Minute
)

// Provider is one streaming service: its Streaming Availability id ("" when
// the API has no list for it) and its TMDB watch provider id.
type Provider struct {
	ID, Name, SA string
	// TMDB is the watch provider id; 0 means look it up live by Name.
	TMDB int
}

// Providers is the served set, in display order.
var Providers = []Provider{
	{"netflix", "Netflix", "netflix", 8},
	{"prime", "Prime Video", "prime", 9},
	{"disney", "Disney+", "disney", 337},
	{"apple", "Apple TV+", "apple", 350},
	{"hbo", "Max", "hbo", 1899},
	{"hulu", "Hulu", "hulu", 15},
	{"paramount", "Paramount+", "paramount", 531},
	{"crunchyroll", "Crunchyroll", "crunchyroll", 283},
	{"starz", "Starz", "starz", 43},
	{"peacock", "Peacock", "", 386},
	{"tubi", "Tubi", "", 0},
}

// Item is one ranked title.
type Item struct {
	Rank       int    `json:"rank"`
	TMDBID     int64  `json:"tmdb_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path,omitempty"`
}

// TMDBReader fills Peacock, Tubi and posters.
type TMDBReader interface {
	// Popular is TMDB discover with with_watch_providers, by popularity.
	// free drops the flat-rate filter (for ad-supported services).
	Popular(ctx context.Context, provider int, region string, free bool) ([]Item, error)
	// ProviderID finds a watch provider's id by name in a region.
	ProviderID(ctx context.Context, name, region string) (int, error)
	// Poster finds a title's TMDB poster path.
	Poster(ctx context.Context, kind string, id int64) (string, error)
}

// Options configure the job.
type Options struct {
	BaseURL string
	// Keys are tried in order; empty ones are skipped. A key is passed
	// over once rejected (401/403), and paused at Stop of its quota or on
	// 429 until its quota resets.
	Keys    []string
	Regions []string
	HTTP    *http.Client
	TMDB    TMDBReader
	// Every is how often each list is fetched (default 12 h).
	Every time.Duration
	// Stop is the share of a key's monthly quota at which calls stop
	// (default 0.9).
	Stop   float64
	Report func(code string, n int)
	Now    func() time.Time
	// New also keeps each service's newly added titles (the API's changes,
	// change_type new, last 14 days): one call per service and region a day,
	// skipped once a key has spent NewStop of its quota (default 0.8), so
	// the top lists always come first.
	New     bool
	NewStop float64
}

type list struct {
	body []byte
	etag string
}

// track is one service's fetch state in a region.
type track struct {
	next      time.Time
	failures  int
	empties   int
	lastOK    time.Time
	lastError string
	state     string
}

// Service serves every region's lists as ready JSON with a strong ETag.
type Service struct {
	db     *sql.DB
	opts   Options
	run    sync.Mutex
	mu     sync.Mutex
	lists  atomic.Pointer[map[string]list]
	tracks map[string]*track
	pruned time.Time
	// newNext is when each region|service's new list is next due.
	newNext map[string]time.Time
	// Calls counts Streaming Availability requests (for tests).
	Calls atomic.Int64
}

// Open opens (or creates) the store at path and serves the newest good
// snapshots at once, before any fetch.
func Open(path string, o Options) (*Service, error) {
	if o.BaseURL == "" {
		o.BaseURL = BaseURL
	}
	if o.Every <= 0 {
		o.Every = 12 * time.Hour
	}
	if o.Stop <= 0 || o.Stop > 1 {
		o.Stop = 0.9
	}
	if o.NewStop <= 0 || o.NewStop > o.Stop {
		o.NewStop = 0.8
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Report == nil {
		o.Report = func(string, int) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if len(o.Regions) == 0 {
		o.Regions = []string{"US"}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS snapshots (region TEXT NOT NULL, service TEXT NOT NULL, fetched_at INTEGER NOT NULL, source TEXT NOT NULL, items BLOB NOT NULL, PRIMARY KEY(region, service, fetched_at)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS keys (key_id TEXT PRIMARY KEY, used INTEGER NOT NULL, granted INTEGER NOT NULL, reset_at INTEGER NOT NULL, paused_until INTEGER NOT NULL, invalid INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS newlists (region TEXT NOT NULL, service TEXT NOT NULL, fetched_at INTEGER NOT NULL, items BLOB NOT NULL, PRIMARY KEY(region, service)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS posters (kind TEXT NOT NULL, tmdb_id INTEGER NOT NULL, poster_path TEXT NOT NULL, PRIMARY KEY(kind, tmdb_id)) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	// The first build kept one list per service in "lists"; carry it over.
	_, _ = db.Exec("INSERT OR IGNORE INTO snapshots SELECT region, service, fetched_at, source, items FROM lists WHERE source IN ('streaming-availability', 'tmdb-popular')")
	s := &Service{db: db, opts: o, tracks: map[string]*track{}, newNext: map[string]time.Time{}}
	for _, r := range o.Regions {
		for _, p := range Providers {
			if at, _, ok := s.newList(r, p.ID); ok {
				s.newNext[r+"|"+p.ID] = at.Add(24 * time.Hour)
			}
			t := &track{state: StateOK}
			if sn, ok := s.newest(r, p.ID); ok {
				t.next, t.lastOK = sn.fetched.Add(o.Every), sn.fetched
			}
			s.tracks[r+"|"+p.ID] = t
		}
	}
	s.rebuild()
	return s, nil
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

// List returns a region's JSON and ETag.
func (s *Service) List(region string) ([]byte, string, bool) {
	l, ok := (*s.lists.Load())[region]
	return l.body, l.etag, ok
}

// ---- keys and quota ----

// keyID names a key by a short hash, never the key itself.
func keyID(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:6])
}

type keyState struct {
	used, granted      int
	reset, pausedUntil time.Time
	invalid, known     bool
}

func (s *Service) keyState(id string) keyState {
	var k keyState
	var reset, paused, invalid int64
	if s.db.QueryRow("SELECT used, granted, reset_at, paused_until, invalid FROM keys WHERE key_id = ?", id).Scan(&k.used, &k.granted, &reset, &paused, &invalid) == nil {
		k.known, k.reset, k.pausedUntil, k.invalid = true, time.Unix(reset, 0), time.Unix(paused, 0), invalid != 0
	}
	return k
}

func (s *Service) saveKey(id string, k keyState) {
	inv := 0
	if k.invalid {
		inv = 1
	}
	_, _ = s.db.Exec("INSERT OR REPLACE INTO keys (key_id, used, granted, reset_at, paused_until, invalid) VALUES (?, ?, ?, ?, ?, ?)", id, k.used, k.granted, k.reset.Unix(), k.pausedUntil.Unix(), inv)
}

// keyBlock says why a key cannot be used now ("" when it can). Pauses and
// the quota stop end by themselves when the quota resets.
func (s *Service) keyBlock(key string) string { return s.keyBlockAt(key, s.opts.Stop) }

func (s *Service) keyBlockAt(key string, stop float64) string {
	k := s.keyState(keyID(key))
	now := s.opts.Now()
	switch {
	case !k.known:
		return ""
	case k.invalid:
		return StateKeyInvalid
	case now.Before(k.pausedUntil):
		return StateQuotaPaused
	case k.granted > 0 && now.Before(k.reset) && float64(k.used) >= stop*float64(k.granted):
		return StateQuotaPaused
	}
	return ""
}

// ---- fetching ----

// errKind sorts a failed fetch.
type fetchErr struct {
	state string // StateKeyInvalid, StateQuotaPaused, or "" (retry)
	msg   string
}

func (e *fetchErr) Error() string { return e.msg }

type saShow struct {
	TMDBID string `json:"tmdbId"`
	Title  string `json:"title"`
}

// clean drops titles without a TMDB id and repeats (the best rank kept),
// numbering the rest in their order.
func clean(shows []saShow) []Item {
	items := make([]Item, 0, len(shows))
	seen := map[string]bool{}
	for _, sh := range shows {
		kind, idText, ok := strings.Cut(sh.TMDBID, "/")
		n, err := strconv.ParseInt(idText, 10, 64)
		if !ok || err != nil || n <= 0 || (kind != "movie" && kind != "tv") || seen[sh.TMDBID] {
			continue
		}
		seen[sh.TMDBID] = true
		items = append(items, Item{Rank: len(items) + 1, TMDBID: n, Kind: kind, Title: sh.Title})
	}
	return items
}

// top asks the API for one service's list, trying each usable key.
func (s *Service) top(ctx context.Context, region, service string) ([]Item, error) {
	body, err := s.get(ctx, "/shows/top", url.Values{"country": {strings.ToLower(region)}, "service": {service}}, s.opts.Stop)
	if err != nil {
		return nil, err
	}
	var shows []saShow
	if json.Unmarshal(body, &shows) != nil {
		return nil, &fetchErr{"", "malformed answer"}
	}
	return clean(shows), nil
}

// get calls the API with the first usable key (one under stop of its
// quota) and returns the answer body.
func (s *Service) get(ctx context.Context, path string, q url.Values, stop float64) ([]byte, error) {
	blocked := StateKeyInvalid
	for _, key := range s.opts.Keys {
		if key == "" {
			continue
		}
		if b := s.keyBlockAt(key, stop); b != "" {
			if b == StateQuotaPaused {
				blocked = b
			}
			continue
		}
		u := s.opts.BaseURL + path + "?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, &fetchErr{"", "bad request"}
		}
		req.Header.Set("X-API-Key", key)
		req.Header.Set("Accept", "application/json")
		s.Calls.Add(1)
		resp, err := s.opts.HTTP.Do(req)
		if err != nil {
			return nil, &fetchErr{"", "network error or timeout"}
		}
		id := keyID(key)
		k := s.keyState(id)
		used, e1 := strconv.Atoi(resp.Header.Get("X-Quota-Used"))
		granted, e2 := strconv.Atoi(resp.Header.Get("X-Quota-Granted"))
		reset, e3 := time.Parse(time.RFC3339, resp.Header.Get("X-Quota-Reset"))
		if e3 != nil {
			reset = s.opts.Now().AddDate(0, 1, 0)
		}
		if e1 == nil && e2 == nil {
			k.used, k.granted, k.reset = used, granted, reset
		}
		switch {
		case resp.StatusCode == 401 || resp.StatusCode == 403:
			resp.Body.Close()
			k.invalid = true
			s.saveKey(id, k)
			log.Printf("servicetop: key %s rejected (%d)", id, resp.StatusCode)
			s.opts.Report("key_invalid", 1)
			continue
		case resp.StatusCode == 429:
			resp.Body.Close()
			k.pausedUntil = reset
			s.saveKey(id, k)
			log.Printf("servicetop: key %s paused until %s (429)", id, reset.UTC().Format(time.RFC3339))
			blocked = StateQuotaPaused
			continue
		}
		s.saveKey(id, k)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, &fetchErr{"", "answered " + resp.Status}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, &fetchErr{"", "network error or timeout"}
		}
		return body, nil
	}
	return nil, &fetchErr{blocked, "no usable key: " + blocked}
}

// ---- snapshots ----

type snapshot struct {
	fetched time.Time
	source  string
	items   []Item
}

// newest is the newest good snapshot: readable and with items, unless the
// newest readable one is an empty list that was believed.
func (s *Service) newest(region, service string) (snapshot, bool) {
	rs, err := s.db.Query("SELECT fetched_at, source, items FROM snapshots WHERE region = ? AND service = ? ORDER BY fetched_at DESC LIMIT 200", region, service)
	if err != nil {
		return snapshot{}, false
	}
	defer rs.Close()
	first := true
	for rs.Next() {
		var sn snapshot
		var at int64
		var b []byte
		if rs.Scan(&at, &sn.source, &b) != nil || json.Unmarshal(b, &sn.items) != nil {
			continue
		}
		sn.fetched = time.Unix(at, 0)
		if len(sn.items) > 0 || first {
			return sn, true
		}
		first = false
	}
	return snapshot{}, false
}

func (s *Service) save(region, service, source string, items []Item) {
	if items == nil {
		items = []Item{}
	}
	b, _ := json.Marshal(items)
	_, _ = s.db.Exec("INSERT OR REPLACE INTO snapshots (region, service, fetched_at, source, items) VALUES (?, ?, ?, ?, ?)", region, service, s.opts.Now().Unix(), source, b)
}

// prune drops snapshots older than keepFor, never a service's newest good one.
func (s *Service) prune() {
	cut := s.opts.Now().Add(-keepFor).Unix()
	for _, r := range s.opts.Regions {
		for _, p := range Providers {
			keep := int64(-1)
			if sn, ok := s.newest(r, p.ID); ok {
				keep = sn.fetched.Unix()
			}
			_, _ = s.db.Exec("DELETE FROM snapshots WHERE region = ? AND service = ? AND fetched_at < ? AND fetched_at != ?", r, p.ID, cut, keep)
		}
	}
}

// posters fills missing poster paths from the poster table, then TMDB.
func (s *Service) posters(ctx context.Context, items []Item) {
	for i := range items {
		it := &items[i]
		if it.PosterPath != "" {
			continue
		}
		if s.db.QueryRow("SELECT poster_path FROM posters WHERE kind = ? AND tmdb_id = ?", it.Kind, it.TMDBID).Scan(&it.PosterPath) == nil || s.opts.TMDB == nil {
			continue
		}
		if p, err := s.opts.TMDB.Poster(ctx, it.Kind, it.TMDBID); err == nil {
			it.PosterPath = p
			_, _ = s.db.Exec("INSERT OR REPLACE INTO posters (kind, tmdb_id, poster_path) VALUES (?, ?, ?)", it.Kind, it.TMDBID, p)
		}
	}
}

// ---- refresh ----

// Refresh fetches every list that is due, each service on its own. Only one
// refresh runs at a time; a second call while one runs returns at once.
func (s *Service) Refresh(ctx context.Context) {
	if !s.run.TryLock() {
		return
	}
	defer s.run.Unlock()
	now := s.opts.Now()
	for _, region := range s.opts.Regions {
		for _, p := range Providers {
			if ctx.Err() != nil {
				return
			}
			s.mu.Lock()
			t := s.tracks[region+"|"+p.ID]
			due := !now.Before(t.next)
			s.mu.Unlock()
			if due {
				s.fetchOne(ctx, region, p, t)
			}
		}
	}
	if s.opts.New {
		for _, region := range s.opts.Regions {
			for _, p := range Providers {
				if ctx.Err() != nil {
					return
				}
				s.mu.Lock()
				due := p.SA != "" && !now.Before(s.newNext[region+"|"+p.ID])
				s.mu.Unlock()
				if due {
					s.fetchNew(ctx, region, p, now)
				}
			}
		}
	}
	if now.Sub(s.pruned) >= 24*time.Hour {
		s.prune()
		s.pruned = now
	}
	s.rebuild()
}

func (s *Service) fetchOne(ctx context.Context, region string, p Provider, t *track) {
	now := s.opts.Now()
	var items []Item
	var err error
	source := SourceSA
	if p.SA == "" {
		source = SourcePopular
		if s.opts.TMDB == nil {
			return
		}
		id := p.TMDB
		if id == 0 {
			id, err = s.opts.TMDB.ProviderID(ctx, p.Name, region)
		}
		if err == nil {
			items, err = s.opts.TMDB.Popular(ctx, id, region, p.TMDB == 0)
		}
		if err != nil {
			err = &fetchErr{"", "tmdb: " + err.Error()}
		}
	} else {
		items, err = s.top(ctx, region, p.SA)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		var fe *fetchErr
		errors.As(err, &fe)
		t.lastError = fe.msg
		log.Printf("servicetop: %s %s: %s", region, p.ID, fe.msg)
		if fe.state != "" {
			// Bad key or paused quota: no more calls this round; the key
			// check runs again next round (a reset quota resumes).
			t.state, t.failures, t.next = fe.state, 0, now.Add(time.Hour)
			return
		}
		t.state = StateOK
		t.failures++
		s.opts.Report("fetch_failed", 1)
		if t.failures < tries {
			t.next = now.Add(retryGap * time.Duration(1<<(t.failures-1)))
		} else {
			t.failures, t.next = 0, now.Add(s.opts.Every)
		}
		return
	}
	t.failures, t.state, t.next = 0, StateOK, now.Add(s.opts.Every)
	old, had := s.newest(region, p.ID)
	if len(items) == 0 && had && len(old.items) > 0 {
		t.empties++
		if t.empties < emptyRepeats {
			t.lastError = "empty list (" + strconv.Itoa(t.empties) + " in a row), kept the last good one"
			log.Printf("servicetop: %s %s: %s", region, p.ID, t.lastError)
			return
		}
	}
	t.empties = 0
	if had && len(items) > 0 && len(items)*2 < len(old.items) {
		log.Printf("servicetop: %s %s: partial list, %d titles against %d before", region, p.ID, len(items), len(old.items))
	}
	s.mu.Unlock()
	s.posters(ctx, items)
	s.mu.Lock()
	s.save(region, p.ID, source, items)
	t.lastOK, t.lastError = now, ""
}

// ---- output ----

type serviceOut struct {
	Service    string    `json:"service"`
	Name       string    `json:"name"`
	FetchedAt  string    `json:"fetched_at"`
	AgeSeconds int64     `json:"age_seconds"`
	Stale      bool      `json:"stale"`
	Source     string    `json:"source"`
	Items      []Item    `json:"items"`
	New        []NewItem `json:"new,omitempty"`
}

func (s *Service) rebuild() {
	now := s.opts.Now()
	built := map[string]list{}
	for _, region := range s.opts.Regions {
		out := []serviceOut{}
		for _, p := range Providers {
			sn, ok := s.newest(region, p.ID)
			if !ok || len(sn.items) == 0 {
				continue
			}
			age := now.Sub(sn.fetched)
			_, fresh, _ := s.newList(region, p.ID)
			out = append(out, serviceOut{p.ID, p.Name, sn.fetched.UTC().Format(time.RFC3339), int64(age / time.Second), age > staleAfter, sn.source, sn.items, fresh})
		}
		if len(out) == 0 {
			continue
		}
		b, err := json.Marshal(map[string]any{"schema_version": 1, "region": region, "attribution": Attribution, "services": out})
		if err != nil {
			continue
		}
		h := sha256.Sum256(b)
		built[region] = list{b, `"` + hex.EncodeToString(h[:16]) + `"`}
	}
	s.lists.Store(&built)
}

// ServiceStats is one service's state for /v1/stats.
type ServiceStats struct {
	Region        string `json:"region"`
	Service       string `json:"service"`
	State         string `json:"state"`
	Source        string `json:"source"`
	Items         int    `json:"items"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// Stats lists every service's state: ok, stale (newest good snapshot over
// 14 days old), key_invalid or quota_paused.
func (s *Service) Stats() any {
	now := s.opts.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []ServiceStats{}
	for _, region := range s.opts.Regions {
		for _, p := range Providers {
			t := s.tracks[region+"|"+p.ID]
			st := ServiceStats{Region: region, Service: p.ID, State: t.state, LastError: t.lastError, Source: SourceSA}
			if p.SA == "" {
				st.Source = SourcePopular
			}
			sn, ok := s.newest(region, p.ID)
			if ok {
				st.Items = len(sn.items)
				st.LastSuccessAt = sn.fetched.UTC().Format(time.RFC3339)
			}
			if st.State == StateOK && (!ok || now.Sub(sn.fetched) > staleAfter) {
				st.State = StateStale
			}
			out = append(out, st)
		}
	}
	return out
}

// Run refreshes what is due every five minutes until ctx ends.
func (s *Service) Run(ctx context.Context) {
	for {
		s.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

// ---- newly added ----

// NewItem is one title newly added to a service.
type NewItem struct {
	TMDBID     int64  `json:"tmdb_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path,omitempty"`
	AddedAt    string `json:"added_at"`
}

// newWindow is how far back the new lists look.
const newWindow = 14 * 24 * time.Hour

type saChange struct {
	ShowID    string `json:"showId"`
	Timestamp int64  `json:"timestamp"`
}

type saChanges struct {
	Changes []saChange        `json:"changes"`
	Shows   map[string]saShow `json:"shows"`
}

// cleanNew keeps one item per title (its newest addition), in answer
// order, dropping titles without a TMDB id.
func cleanNew(c saChanges) []NewItem {
	out := []NewItem{}
	seen := map[string]bool{}
	for _, ch := range c.Changes {
		sh, ok := c.Shows[ch.ShowID]
		kind, idText, cut := strings.Cut(sh.TMDBID, "/")
		n, err := strconv.ParseInt(idText, 10, 64)
		if !ok || !cut || err != nil || n <= 0 || (kind != "movie" && kind != "tv") || seen[sh.TMDBID] {
			continue
		}
		seen[sh.TMDBID] = true
		out = append(out, NewItem{TMDBID: n, Kind: kind, Title: sh.Title, AddedAt: time.Unix(ch.Timestamp, 0).UTC().Format(time.RFC3339)})
	}
	return out
}

// fetchNew reads one page of a service's new titles (newest first). A
// failure waits an hour; a good answer (even empty) waits a day.
func (s *Service) fetchNew(ctx context.Context, region string, p Provider, now time.Time) {
	k := region + "|" + p.ID
	q := url.Values{"country": {strings.ToLower(region)}, "change_type": {"new"}, "item_type": {"show"}, "catalogs": {p.SA}, "order_direction": {"desc"}, "from": {strconv.FormatInt(now.Add(-newWindow).Unix(), 10)}}
	body, err := s.get(ctx, "/changes", q, s.opts.NewStop)
	var c saChanges
	if err == nil && json.Unmarshal(body, &c) != nil {
		err = &fetchErr{"", "malformed answer"}
	}
	if err != nil {
		log.Printf("servicetop: %s %s new: %s", region, p.ID, err.Error())
		s.mu.Lock()
		s.newNext[k] = now.Add(time.Hour)
		s.mu.Unlock()
		return
	}
	items := cleanNew(c)
	if len(items) > 40 {
		items = items[:40]
	}
	tops := make([]Item, len(items))
	for i, it := range items {
		tops[i] = Item{TMDBID: it.TMDBID, Kind: it.Kind}
	}
	s.posters(ctx, tops)
	for i := range items {
		items[i].PosterPath = tops[i].PosterPath
	}
	b, _ := json.Marshal(items)
	_, _ = s.db.Exec("INSERT OR REPLACE INTO newlists (region, service, fetched_at, items) VALUES (?, ?, ?, ?)", region, p.ID, now.Unix(), b)
	s.mu.Lock()
	s.newNext[k] = now.Add(24 * time.Hour)
	s.mu.Unlock()
}

// newList is a service's kept new list, only items still in the window.
func (s *Service) newList(region, service string) (time.Time, []NewItem, bool) {
	var at int64
	var b []byte
	if s.db.QueryRow("SELECT fetched_at, items FROM newlists WHERE region = ? AND service = ?", region, service).Scan(&at, &b) != nil {
		return time.Time{}, nil, false
	}
	var items []NewItem
	if json.Unmarshal(b, &items) != nil {
		return time.Time{}, nil, false
	}
	cut := s.opts.Now().Add(-newWindow)
	kept := []NewItem{}
	for _, it := range items {
		if t, err := time.Parse(time.RFC3339, it.AddedAt); err == nil && t.After(cut) {
			kept = append(kept, it)
		}
	}
	return time.Unix(at, 0), kept, true
}
