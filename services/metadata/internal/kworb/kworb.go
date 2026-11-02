// Package kworb reads kworb.net's public Spotify pages on this server only,
// on a fixed schedule, and keeps what it read: the top 2,500 artists by
// monthly listeners and the top 3,000 by all-time plays once a day, the
// daily song chart of every kept region once a day and its weekly chart
// once a week, and a slow crawl of artists' songs pages (total plays) for
// the catalog's other artists, each a week apart. Nothing is ever read
// because someone asked: an artist page opened is only noted, and moves
// that artist to the front of the next crawl. If we don't have it, we
// don't have it.
//
// One reader does it all: a page at most every 2 s, at randomised times
// (never on the hour), at most 1,200 pages a day, the server's own
// User-Agent and a kill switch. A failed read or an unknown layout keeps
// the last good copy, never marks anything missing, and raises an alert in
// the status. The ids on the pages are Spotify ids, matched to MusicBrainz
// ids by links only (match.go). No key or account is needed.
package kworb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// BaseURL is kworb's Spotify section.
const BaseURL = "https://kworb.net/spotify"

// Reader limits.
const (
	minGap   = 2 * time.Second
	dayCap   = 1200 // pages a day, every kind
	crawlCap = 1000 // songs pages a day
)

// errOff: the kill switch is off; nothing is read.
var errOff = errors.New("kworb: switched off")

// errCap: the day's page cap is spent.
var errCap = errors.New("kworb: daily page cap reached")

// Options configure the reader.
type Options struct {
	BaseURL   string // default BaseURL
	HTTP      *http.Client
	UserAgent string
	// Gap is the wait between page reads (at least 2 s).
	Gap time.Duration
	// On is the kill switch (control provider "kworb"); nil: on.
	On func() bool
	// Regions are the chart regions read ("global" and lower-case ISO
	// countries).
	Regions []string
	// Catalog answers the catalog's artists (MusicBrainz ids, most
	// popular first) for the crawl.
	Catalog func() []string
	// MinRows is the fewest rows the listeners and artists pages must
	// have to replace the kept copy (default 2,000).
	MinRows int
	// MBHTTP reaches MusicBrainz (paced with every other MusicBrainz call)
	// at MBURL; WDHTTP reaches Wikidata's query service at SPARQLURL. nil:
	// that source is not asked.
	MBHTTP, WDHTTP   *http.Client
	MBURL, SPARQLURL string
	// KeptSpotify answers the Spotify id the picture chains matched for an
	// artist from its MusicBrainz link; KeptArtist the reverse. Matches
	// they made by a name search are never answered.
	KeptSpotify func(mbid string) (string, bool)
	KeptArtist  func(spotify string) (string, bool)
	// StoredLinks answers the url-relations of the artist's stored
	// MusicBrainz answer (no call).
	StoredLinks func(ctx context.Context, mbid string) []string
	Now         func() time.Time
	// sleep waits and rnd draws in [0, 1) (tests fake both).
	sleep func(ctx context.Context, d time.Duration) error
	rnd   func() float64
}

// Popularity is what the artist answer carries (data.popularity); every
// field is left out when unknown. The top 2,500 get their listeners, rank
// and the day's change; any other artist we hold plays for its total.
type Popularity struct {
	MonthlyListeners int64  `json:"monthly_listeners,omitempty"`
	WorldRank        int    `json:"world_rank,omitempty"`
	ListenersChange  *int64 `json:"listeners_change_today,omitempty"`
	TotalPlays       int64  `json:"total_plays,omitempty"`
	// AsOf is kworb's date for the figures (YYYY-MM-DD).
	AsOf string `json:"as_of,omitempty"`
}

// Riser is one card of the rising row.
type Riser struct {
	MBID      string `json:"mbid"`
	Name      string `json:"name"`
	Listeners int64  `json:"listeners"`
	Gain      int64  `json:"gain"`
	Rank      int    `json:"rank"`
}

// Status is the reader's health for /v1/stats.
type Status struct {
	On            bool   `json:"on"`
	Listeners     int    `json:"listeners_rows"`
	Streamed      int    `json:"artists_rows"`
	Charts        int    `json:"charts"`
	AsOf          string `json:"as_of,omitempty"`
	Matched       int    `json:"matched"`
	PagesToday    int    `json:"pages_today"`
	CrawlToday    int    `json:"crawl_today"`
	Reads         int64  `json:"reads"`
	NotModified   int64  `json:"not_modified"`
	Failures      int64  `json:"failures"`
	LayoutErrors  int64  `json:"layout_errors"`
	NoPage        int64  `json:"no_page"`
	MatchFailures int64  `json:"match_failures"`
	// Alert names the last page whose layout did not read, kept until that
	// page reads again.
	Alert     string `json:"alert,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type topList struct {
	AsOf string     `json:"as_of"`
	Rows []Listener `json:"rows"`
	ETag string     `json:"etag,omitempty"`
}

type streamedList struct {
	AsOf string     `json:"as_of"`
	Rows []Streamed `json:"rows"`
}

// ChartState is a region's kept chart and how its last read went: Failed
// is true when the last read failed or did not parse (the kept Chart, if
// any, is older).
type ChartState struct {
	Chart  Chart     `json:"chart"`
	At     time.Time `json:"at"`
	Failed bool      `json:"failed"`
}

// Service keeps what the reader read and the id pairs.
type Service struct {
	db *sql.DB
	o  Options

	paceMu sync.Mutex
	next   time.Time

	mu       sync.RWMutex
	top      topList
	bySp     map[string]Listener
	streamed streamedList
	bySt     map[string]Streamed
	rising   []byte
	// queue is the crawl's line-up, made again every rebuildFor.
	queue   []string
	queueAt time.Time

	stMu sync.Mutex
	st   Status
}

// Open opens (or creates) the store at path and loads the kept lists.
func Open(path string, o Options) (*Service, error) {
	if o.BaseURL == "" {
		o.BaseURL = BaseURL
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Gap < minGap {
		o.Gap = minGap
	}
	if o.MinRows <= 0 {
		o.MinRows = 2000
	}
	if o.MBURL == "" {
		o.MBURL = MusicBrainzURL
	}
	if o.SPARQLURL == "" {
		o.SPARQLURL = SPARQLURL
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.rnd == nil {
		o.rnd = rand.Float64
	}
	if o.sleep == nil {
		o.sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-t.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS kept (k TEXT PRIMARY KEY, body BLOB NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS pages (spotify TEXT PRIMARY KEY, found INTEGER NOT NULL, total INTEGER NOT NULL, as_of TEXT NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS pairs (spotify TEXT PRIMARY KEY, mbid TEXT NOT NULL, how TEXT NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS pairs_mbid ON pairs(mbid);
CREATE TABLE IF NOT EXISTS tried (k TEXT PRIMARY KEY, until INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS opened (mbid TEXT PRIMARY KEY, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS due (job TEXT PRIMARY KEY, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS spent (day TEXT PRIMARY KEY, pages INTEGER NOT NULL, crawl INTEGER NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, o: o}
	var t topList
	if s.loadKept("listeners", &t) {
		s.setTop(t)
	}
	var a streamedList
	if s.loadKept("artists", &a) {
		s.setStreamed(a)
	}
	return s, nil
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

func (s *Service) on() bool { return s.o.On == nil || s.o.On() }

func (s *Service) count(f func(*Status)) {
	s.stMu.Lock()
	f(&s.st)
	s.stMu.Unlock()
}

func (s *Service) fail(err error) {
	s.count(func(st *Status) { st.Failures++; st.LastError = err.Error() })
}

// layout counts a page that did not read and raises the alert.
func (s *Service) layout(page string) {
	s.count(func(st *Status) {
		st.LayoutErrors++
		st.Alert = page + ": unknown layout, last good copy kept"
		st.LastError = st.Alert
	})
}

// readOK clears the alert a page raised once it reads again.
func (s *Service) readOK(page string) {
	s.count(func(st *Status) {
		if strings.HasPrefix(st.Alert, page+":") {
			st.Alert = ""
		}
	})
}

func (s *Service) loadKept(k string, out any) bool {
	var b []byte
	if s.db.QueryRow(`SELECT body FROM kept WHERE k = ?`, k).Scan(&b) != nil {
		return false
	}
	return json.Unmarshal(b, out) == nil
}

func (s *Service) keep(k string, v any) {
	if b, err := json.Marshal(v); err == nil {
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO kept (k, body, at) VALUES (?, ?, ?)`, k, b, s.o.Now().Unix())
	}
}

func (s *Service) today() string { return s.o.Now().UTC().Format("2006-01-02") }

// spentToday answers the pages read today (every kind, and the crawl's).
func (s *Service) spentToday() (pages, crawl int) {
	_ = s.db.QueryRow(`SELECT pages, crawl FROM spent WHERE day = ?`, s.today()).Scan(&pages, &crawl)
	return pages, crawl
}

// pace waits until the next page read may start.
func (s *Service) pace(ctx context.Context) error {
	s.paceMu.Lock()
	now := s.o.Now()
	at := s.next
	if at.Before(now) {
		at = now
	}
	s.next = at.Add(s.o.Gap)
	s.paceMu.Unlock()
	if d := at.Sub(now); d > 0 {
		return s.o.sleep(ctx, d)
	}
	return nil
}

// page reads one kworb page inside the day's cap. ok is false on a 404;
// etag (when set) asks for the page only when it changed (notModified).
func (s *Service) page(ctx context.Context, path, etag string, crawl bool) (body, newTag, lastMod string, ok, notModified bool, err error) {
	if !s.on() {
		return "", "", "", false, false, errOff
	}
	pages, crawled := s.spentToday()
	if pages >= dayCap || crawl && crawled >= crawlCap {
		return "", "", "", false, false, errCap
	}
	if err := s.pace(ctx); err != nil {
		return "", "", "", false, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.o.BaseURL+path, nil)
	if err != nil {
		return "", "", "", false, false, err
	}
	req.Header.Set("User-Agent", s.o.UserAgent)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	c := 0
	if crawl {
		c = 1
	}
	_, _ = s.db.Exec(`INSERT INTO spent (day, pages, crawl) VALUES (?, 1, ?) ON CONFLICT(day) DO UPDATE SET pages = pages + 1, crawl = crawl + excluded.crawl`, s.today(), c)
	_, _ = s.db.Exec(`DELETE FROM spent WHERE day < ?`, s.o.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02"))
	s.count(func(st *Status) { st.Reads++ })
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		return "", "", "", false, false, errors.New("kworb: request failed")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		s.count(func(st *Status) { st.NotModified++ })
		return "", etag, "", true, true, nil
	case http.StatusNotFound:
		return "", "", "", false, false, nil
	case http.StatusOK:
	default:
		return "", "", "", false, false, &statusErr{"kworb", resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", "", false, false, err
	}
	return string(b), resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), true, false, nil
}

// dayOf is an HTTP date as YYYY-MM-DD (UTC), else "".
func dayOf(h string) string {
	t, err := http.ParseTime(h)
	if err != nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func (s *Service) setTop(t topList) {
	by := make(map[string]Listener, len(t.Rows))
	for _, l := range t.Rows {
		by[l.Spotify] = l
	}
	s.mu.Lock()
	s.top, s.bySp = t, by
	s.mu.Unlock()
	s.buildRising()
	s.count(func(st *Status) { st.Listeners, st.AsOf = len(t.Rows), t.AsOf })
}

func (s *Service) setStreamed(a streamedList) {
	by := make(map[string]Streamed, len(a.Rows))
	for _, r := range a.Rows {
		by[r.Spotify] = r
	}
	s.mu.Lock()
	s.streamed, s.bySt = a, by
	s.mu.Unlock()
	s.count(func(st *Status) { st.Streamed = len(a.Rows) })
}

// covered reports a Spotify id on either list page.
func (s *Service) covered(sp string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, a := s.bySp[sp]
	_, b := s.bySt[sp]
	return a || b
}

// Popularity answers an artist's kept figures (nil when none are kept): the
// listeners page for the top 2,500, else the most streamed page, else the
// artist's crawled songs page. Nothing is read here.
func (s *Service) Popularity(mbid string) *Popularity {
	sp := s.pairByMBID(strings.ToLower(mbid))
	if sp == "" {
		return nil
	}
	s.mu.RLock()
	l, top := s.bySp[sp]
	a, streamed := s.bySt[sp]
	topAsOf, stAsOf := s.top.AsOf, s.streamed.AsOf
	s.mu.RUnlock()
	switch {
	case top:
		change := l.Change
		return &Popularity{MonthlyListeners: l.Listeners, WorldRank: l.Rank, ListenersChange: &change, AsOf: topAsOf}
	case streamed:
		return &Popularity{TotalPlays: a.Total, AsOf: stAsOf}
	}
	var found int
	var total int64
	var day string
	if s.db.QueryRow(`SELECT found, total, as_of FROM pages WHERE spotify = ?`, sp).Scan(&found, &total, &day) != nil || found == 0 {
		return nil
	}
	return &Popularity{TotalPlays: total, AsOf: day}
}

// Opened notes that an artist page was opened: the artist goes to the
// front of the next crawl. Nothing is read.
func (s *Service) Opened(mbid string) {
	mbid = strings.ToLower(mbid)
	if !mbidRe.MatchString(mbid) {
		return
	}
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO opened (mbid, at) VALUES (?, ?)`, mbid, s.o.Now().Unix())
}

// Rising answers the rising row's JSON; false before a listeners page was
// ever read.
func (s *Service) Rising() ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rising, s.rising != nil
}

// SongChart answers a region's kept daily (or weekly) chart and how its
// last read went; false when the region was never read.
func (s *Service) SongChart(region string, weekly bool) (ChartState, bool) {
	var c ChartState
	return c, s.loadKept(chartKey(region, weekly), &c)
}

func chartKey(region string, weekly bool) string {
	if weekly {
		return "chart:" + region + ":weekly"
	}
	return "chart:" + region + ":daily"
}

// Stats answers the reader's health.
func (s *Service) Stats() Status {
	s.stMu.Lock()
	st := s.st
	s.stMu.Unlock()
	st.On = s.on()
	st.PagesToday, st.CrawlToday = s.spentToday()
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM kept WHERE k LIKE 'chart:%'`).Scan(&st.Charts)
	return st
}

// TopArtists answers the matched MusicBrainz ids of the top artists by
// monthly listeners (at most n, in rank order). Nothing is read upstream.
func (s *Service) TopArtists(n int) []string {
	s.mu.RLock()
	rows := s.top.Rows
	s.mu.RUnlock()
	var out []string
	for _, l := range rows {
		if len(out) >= n {
			break
		}
		if mb := s.pairBySpotify(l.Spotify); mb != "" {
			out = append(out, mb)
		}
	}
	return out
}

// RisingArtists answers the rising row's artists.
func (s *Service) RisingArtists() []string {
	b, ok := s.Rising()
	if !ok {
		return nil
	}
	var d struct {
		Items []Riser `json:"items"`
	}
	_ = json.Unmarshal(b, &d)
	out := make([]string, 0, len(d.Items))
	for _, r := range d.Items {
		out = append(out, r.MBID)
	}
	return out
}

// OpenedArtists answers the artists whose page any install opened within
// d, latest first.
func (s *Service) OpenedArtists(d time.Duration) []string {
	rows, err := s.db.Query(`SELECT mbid FROM opened WHERE at >= ? ORDER BY at DESC`, s.o.Now().Add(-d).Unix())
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var m string
		if rows.Scan(&m) == nil {
			out = append(out, m)
		}
	}
	return out
}
