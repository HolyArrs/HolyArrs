// Package lists keeps world music lists (daily song charts, weekly album
// and artist charts, new releases) from one paid list source, matched to
// MusicBrainz ids and served as shelves of albums and artists. Only this
// server calls the source, on a schedule and inside a monthly request cap,
// so the cost never grows with installs. The source's brand and ids never
// leave this package: shelves carry MusicBrainz ids and our own words.
package lists

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	_ "modernc.org/sqlite"
)

// DefaultHost is the list source's API host (allow-listed for the brand
// grep: it is a host name, never shown or logged).
const DefaultHost = "spotify81.p.rapidapi.com"

// DefaultPlanLimit is the source's plan: 1,000 requests a period.
const DefaultPlanLimit = 1000

// QuotaStale is how long a quota seen without a reset time is kept.
const QuotaStale = 24 * time.Hour

// QuotaShare is the most of the plan the server may spend (half), so the
// rest stays for checks by hand and retries.
const QuotaShare = 0.5

// DefaultMonthCap is QuotaShare of the free plan.
const DefaultMonthCap = 500

// BrowserUA is sent on every call (the source rejects plain clients).
const BrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// Options configure the collector.
type Options struct {
	// ExactArtist resolves a row's own artist id ("spotify", the row's
	// ext_artist) to a MusicBrainz artist exactly ("" when it does not);
	// it beats the name match. Stored rows only: nothing is asked.
	ExactArtist func(ctx context.Context, source, id, name string, codes []string) string
	// SharedName reports a name several MusicBrainz artists carry: with no
	// exact evidence such a row links to no artist.
	SharedName func(ctx context.Context, name string) bool
	Key        string
	Host       string // default DefaultHost
	// BaseURL overrides https://<Host> (tests).
	BaseURL string
	HTTP    *http.Client
	// MonthCap is the most calls in a calendar month (default
	// DefaultMonthCap, 500); DayCap the most in a UTC day (default
	// MonthCap/30 rounded up, 17, so the month's calls last the month).
	MonthCap int
	DayCap   int
	// PlanLimit is the source's requests per period (default
	// DefaultPlanLimit). MonthCap is held to QuotaShare of it, and the
	// source's own quota headers stop calls once QuotaShare of the period
	// is used, whoever spent it.
	PlanLimit int
	// Pace is the wait between calls (default 600 ms); MatchPace the wait
	// between MusicBrainz searches (default 2 s, a low lane).
	Pace      time.Duration
	MatchPace time.Duration
	// MatchMax is the most new searches in one run (default 400).
	MatchMax int
	Search   musictrend.Searcher
	// Link answers the MusicBrainz id linked to a source artist id (one
	// MusicBrainz call); tried before a name search. nil: names only.
	Link func(ctx context.Context, kind, extID string) (string, error)
	// Every is how often due sources are checked (default 30 min).
	Every time.Duration
	// Prefetch queues one new shelf member for pictures and videos
	// (kind "artist" or "release-group").
	Prefetch func(kind, mbid string)
	// RGInfo answers a release group's first release date from stored
	// MusicBrainz answers (no call); nil or "": unknown.
	RGInfo func(mbid string) string
	// Fresh answers ListenBrainz's fresh releases for the new-release
	// shelf (no call; nil: the US list only).
	Fresh func() []FreshAlbum
	// Songs answers a region's song chart kept by the chart reader (no
	// call); with it the source's own song charts are only the backup and
	// are left out of the plan's budget. Tracks answers songs' albums from
	// the Spotify API (songs.go). nil: the source's song charts only.
	Songs  func(region string) (SongChart, bool)
	Tracks func(ctx context.Context, ids []string) (map[string]TrackAlbum, error)
	Now    func() time.Time
	Log    func(format string, args ...any)
}

// Service is the collector and the shelves it serves.
type Service struct {
	db      *sql.DB
	opts    Options
	plan    []Source
	matcher *musictrend.Matcher
	runMu   sync.Mutex
	mu      sync.RWMutex
	shelves map[string]served // by region
	known   map[string]bool   // members already prefetched
	last    time.Time
	calls   int // calls this process (tests)
	backlog bool
	linked  int // matches made by link this process
	// rematched are the songs already logged as re-matched.
	rematched map[string]bool
	backoff   time.Time
}

type served struct {
	body []byte
	etag string
}

// Open opens (or creates) the store at path and loads the last shelves.
func Open(path string, o Options) (*Service, error) {
	if o.Host == "" {
		o.Host = DefaultHost
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://" + o.Host
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if o.PlanLimit <= 0 {
		o.PlanLimit = DefaultPlanLimit
	}
	if o.MonthCap <= 0 {
		o.MonthCap = DefaultMonthCap
	}
	o.MonthCap = min(o.MonthCap, int(float64(o.PlanLimit)*QuotaShare))
	if o.DayCap <= 0 {
		// The month's calls spread over the month: a bigger day cap let
		// four days spend it (Oct 2026: 478 of 500 by the 29th).
		o.DayCap = max(1, (o.MonthCap+29)/30)
	}
	if o.Pace < 0 {
		o.Pace = 0
	} else if o.Pace == 0 {
		o.Pace = 600 * time.Millisecond
	}
	if o.MatchPace < 0 {
		o.MatchPace = 0
	} else if o.MatchPace == 0 {
		// Matching calls are priority calls (providers.WithPriority): the
		// gate gives them every other slot while names wait.
		o.MatchPace = 500 * time.Millisecond
	}
	if o.MatchMax <= 0 {
		o.MatchMax = 900
	}
	if o.Every <= 0 {
		o.Every = 30 * time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = log.Printf
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS lists_calls (day TEXT PRIMARY KEY, n INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lists_source (key TEXT PRIMARY KEY, fetched_at INTEGER NOT NULL, hash TEXT NOT NULL, version INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lists_day (key TEXT NOT NULL, day TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(key, day)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lists_season (key TEXT PRIMARY KEY, season TEXT NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS matches (kind TEXT NOT NULL, name TEXT NOT NULL, artist TEXT NOT NULL, mbid TEXT NOT NULL, artist_mbid TEXT NOT NULL, matched_at INTEGER NOT NULL, PRIMARY KEY(kind, name, artist)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS tracks (id TEXT PRIMARY KEY, body BLOB NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lists_quota (id INTEGER PRIMARY KEY CHECK (id = 1), lim INTEGER NOT NULL, remaining INTEGER NOT NULL, reset_at INTEGER NOT NULL, seen_at INTEGER NOT NULL);`)
	if err == nil {
		err = srctag.Stamp(db, srctag.Spec{Table: "lists_day", Key: []string{"key", "day"}, Body: "body", Src: "'spotify81'", Known: "CAST(strftime('%s', day) AS INTEGER)"})
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, opts: o, plan: planWith(o.MonthCap, o.Songs != nil), shelves: map[string]served{}, known: map[string]bool{}}
	_, _ = db.Exec(`ALTER TABLE matches ADD COLUMN first_date TEXT NOT NULL DEFAULT ''`)
	s.rematched = map[string]bool{}
	s.matcher = &musictrend.Matcher{DB: db}
	if o.Search != nil {
		s.matcher.Search = honestSearch{o.Search}
	}
	s.rebuild(context.Background(), false)
	return s, nil
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

// Sources is the plan in force for the month cap.
func (s *Service) Sources() []Source { return append([]Source{}, s.plan...) }

func (s *Service) today() string { return s.opts.Now().UTC().Format("2006-01-02") }

// Used answers the calls spent this calendar month and today.
func (s *Service) Used() (month, today int) {
	d := s.today()
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(n),0) FROM lists_calls WHERE day LIKE ?`, d[:7]+"%").Scan(&month)
	_ = s.db.QueryRow(`SELECT COALESCE(n,0) FROM lists_calls WHERE day = ?`, d).Scan(&today)
	return month, today
}

// Quota is the source's own count for the current period, from the
// headers of the last answer (Limit 0: not seen yet).
type Quota struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	SeenAt    time.Time `json:"seen_at"`
}

// Used answers the period's calls by the source's count.
func (q Quota) Used() int { return q.Limit - q.Remaining }

// Quota answers the last quota the source reported; a period that has
// reset since counts as unseen.
func (s *Service) Quota() Quota {
	var q Quota
	var reset, seen int64
	if s.db.QueryRow(`SELECT lim, remaining, reset_at, seen_at FROM lists_quota WHERE id = 1`).Scan(&q.Limit, &q.Remaining, &reset, &seen) != nil {
		return Quota{}
	}
	q.ResetAt, q.SeenAt = time.Unix(reset, 0), time.Unix(seen, 0)
	if !q.ResetAt.IsZero() && reset > 0 && s.opts.Now().After(q.ResetAt) {
		return Quota{}
	}
	// An answer without a reset header never expires by itself; it is
	// taken as stale a day after it was seen, so a high count cannot stop
	// every later call.
	if reset <= 0 && s.opts.Now().Sub(q.SeenAt) > QuotaStale {
		return Quota{}
	}
	return q
}

// keepQuota records the quota headers of one answer.
func (s *Service) keepQuota(h http.Header) {
	lim, err1 := strconv.Atoi(strings.TrimSpace(h.Get("x-ratelimit-requests-limit")))
	rem, err2 := strconv.Atoi(strings.TrimSpace(h.Get("x-ratelimit-requests-remaining")))
	if err1 != nil || err2 != nil || lim <= 0 {
		return
	}
	now := s.opts.Now()
	var reset int64
	if sec, err := strconv.ParseInt(strings.TrimSpace(h.Get("x-ratelimit-requests-reset")), 10, 64); err == nil && sec > 0 {
		reset = now.Add(time.Duration(sec) * time.Second).Unix()
	}
	_, _ = s.db.Exec(`INSERT INTO lists_quota (id, lim, remaining, reset_at, seen_at) VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET lim = excluded.lim, remaining = excluded.remaining, reset_at = excluded.reset_at, seen_at = excluded.seen_at`, lim, rem, reset, now.Unix())
}

// canSpend reports whether one more call fits both caps and keeps the
// source's period at QuotaShare or less.
func (s *Service) canSpend() bool {
	m, d := s.Used()
	if m >= s.opts.MonthCap || d >= s.opts.DayCap {
		return false
	}
	if q := s.Quota(); q.Limit > 0 && float64(q.Used()+1) > float64(q.Limit)*QuotaShare {
		return false
	}
	return true
}

func (s *Service) spend() {
	_, _ = s.db.Exec(`INSERT INTO lists_calls (day, n) VALUES (?, 1) ON CONFLICT(day) DO UPDATE SET n = n + 1`, s.today())
	_, _ = s.db.Exec(`DELETE FROM lists_calls WHERE day < ?`, s.opts.Now().UTC().AddDate(0, -3, 0).Format("2006-01-02"))
	s.calls++
}

var errLimited = errors.New("lists: rate limited")

// fetch makes one counted call. The key is sent only as a header and is
// never logged.
func (s *Service) fetch(ctx context.Context, src Source) ([]Entry, []byte, error) {
	if src.Tile.Key == SeasonTile.Key {
		src.Tile.Category = SeasonTiles[ActiveSeason(s.opts.Now())]
	}
	path := src.Path()
	if src.Kind == KindList {
		path = s.listPath(src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opts.BaseURL+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", BrowserUA)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-rapidapi-key", s.opts.Key)
	req.Header.Set("x-rapidapi-host", s.opts.Host)
	s.spend()
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("lists: %s: request failed", src.Key)
	}
	defer resp.Body.Close()
	s.keepQuota(resp.Header)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, nil, errLimited
	}
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("lists: %s answered %d", src.Key, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	var es []Entry
	switch src.Kind {
	case KindNew:
		es, err = ParseNew(strings.NewReader(string(b)))
		es = FreshReleases(es, s.opts.Now())
	case KindCategory:
		es, err = ParseCategory(strings.NewReader(string(b)))
	case KindList:
		es, err = ParsePlaylist(strings.NewReader(string(b)))
	default:
		es, err = ParseChart(strings.NewReader(string(b)))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lists: %s: unreadable answer", src.Key)
	}
	if len(es) == 0 {
		return nil, nil, fmt.Errorf("lists: %s: empty answer", src.Key)
	}
	body, _ := json.Marshal(es)
	return es, body, nil
}

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:12])
}

// listPath is a tile list's request path: list number Slot of its stored
// category page, or "" while the page is not read yet.
func (s *Service) listPath(src Source) string {
	es, _ := s.latest("category:" + src.Tile.Key)
	if src.Slot >= len(es) {
		return ""
	}
	return ListPath(es[src.Slot].ExtAlbum)
}

// due reports whether a source's period has passed since its last good read.
// The seasonal slot is due only while a paid season with a known page is
// on, and at once when the stored read is another season's.
func (s *Service) due(src Source) bool {
	if src.Tile.Key == SeasonTile.Key {
		on := ActiveSeason(s.opts.Now())
		if on == "" || SeasonTiles[on] == "" && src.Kind == KindCategory {
			return false
		}
		if s.season(src.Key) != on {
			return true
		}
	}
	var at int64
	if s.db.QueryRow(`SELECT fetched_at FROM lists_source WHERE key = ?`, src.Key).Scan(&at) != nil {
		return true
	}
	return s.opts.Now().Sub(time.Unix(at, 0)) >= src.Every-30*time.Minute
}

// store keeps one read: unchanged entries (same hash) only touch the time.
func (s *Service) store(src Source, body []byte) (changed bool) {
	h := hashOf(body)
	now := s.opts.Now()
	var old string
	_ = s.db.QueryRow(`SELECT hash FROM lists_source WHERE key = ?`, src.Key).Scan(&old)
	_, _ = s.db.Exec(`INSERT INTO lists_source (key, fetched_at, hash, version) VALUES (?, ?, ?, 1)
ON CONFLICT(key) DO UPDATE SET fetched_at = excluded.fetched_at, version = version + (CASE WHEN hash = excluded.hash THEN 0 ELSE 1 END), hash = excluded.hash`, src.Key, now.Unix(), h)
	if old == h {
		return false
	}
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO lists_day (key, day, body) VALUES (?, ?, ?)`, src.Key, s.today(), body)
	_, _ = s.db.Exec(`DELETE FROM lists_day WHERE day < ?`, now.UTC().AddDate(0, 0, -28).Format("2006-01-02"))
	return true
}

// failRetry is how long a source that failed waits before its next try,
// so a list the source cannot answer never spends a call each run.
const failRetry = 6 * time.Hour

// failed marks a failed read: the source is due again after failRetry.
func (s *Service) failed(src Source) {
	at := s.opts.Now().Add(failRetry - src.Every).Unix()
	_, _ = s.db.Exec(`INSERT INTO lists_source (key, fetched_at, hash, version) VALUES (?, ?, '', 0)
ON CONFLICT(key) DO UPDATE SET fetched_at = excluded.fetched_at`, src.Key, at)
}

// season answers which season a seasonal source last read.
func (s *Service) season(key string) string {
	var v string
	_ = s.db.QueryRow(`SELECT season FROM lists_season WHERE key = ?`, key).Scan(&v)
	return v
}

func (s *Service) setSeason(key, season string) {
	_, _ = s.db.Exec(`INSERT INTO lists_season (key, season) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET season = excluded.season`, key, season)
}

// Season answers the season the seasonal list block holds ("" when none
// is on or none is stored yet).
func (s *Service) Season() string {
	on := ActiveSeason(s.opts.Now())
	if on != "" && s.season("list:season:0") == on {
		return on
	}
	return ""
}

// Refresh reads every due source in plan order inside the caps, matches
// new names to MusicBrainz, and rebuilds the shelves when anything changed.
func (s *Service) Refresh(ctx context.Context) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.opts.Now().Before(s.backoff) {
		return nil
	}
	read, changed, skipped := 0, 0, 0
	var firstErr error
	for _, src := range s.plan {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !s.due(src) || src.Kind == KindList && s.listPath(src) == "" {
			continue
		}
		if src.Kind == KindSongs && s.opts.Songs != nil {
			done, ch := s.songsFirst(ctx, src)
			if ch {
				changed++
			}
			if done {
				continue
			}
		}
		if !s.canSpend() {
			skipped++
			continue
		}
		if read > 0 && s.opts.Pace > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.opts.Pace):
			}
		}
		_, body, err := s.fetch(ctx, src)
		read++
		if errors.Is(err, errLimited) {
			s.backoff = s.opts.Now().Add(5 * time.Minute)
			firstErr = err
			break
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.failed(src)
			continue
		}
		if src.Tile.Key == SeasonTile.Key {
			s.setSeason(src.Key, ActiveSeason(s.opts.Now()))
		}
		if s.store(src, body) {
			changed++
		}
	}
	matched := s.matchNew(ctx)
	if changed > 0 || matched > 0 || s.last.IsZero() {
		s.rebuild(ctx, true)
	}
	if read > 0 || skipped > 0 {
		m, _ := s.Used()
		s.opts.Log("lists: read %d, changed %d, skipped over cap %d, matched %d, month %d/%d", read, changed, skipped, matched, m, s.opts.MonthCap)
	}
	return firstErr
}

// Run refreshes on start and then every Every.
func (s *Service) Run(ctx context.Context) {
	for {
		_ = s.Refresh(ctx)
		wait := s.opts.Every
		if s.Backlog() {
			// Names still to match: go on after a short rest.
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// latest reads a source's newest stored entries (and its day).
func (s *Service) latest(key string) ([]Entry, string) {
	var b []byte
	var d string
	if s.db.QueryRow(`SELECT day, body FROM lists_day WHERE key = ? ORDER BY day DESC LIMIT 1`, key).Scan(&d, &b) != nil {
		return nil, ""
	}
	var es []Entry
	_ = json.Unmarshal(b, &es)
	return es, d
}

// at reads a source's entries stored on or before day.
func (s *Service) at(key, day string) []Entry {
	var b []byte
	if s.db.QueryRow(`SELECT body FROM lists_day WHERE key = ? AND day <= ? ORDER BY day DESC LIMIT 1`, key, day).Scan(&b) != nil {
		return nil
	}
	var es []Entry
	_ = json.Unmarshal(b, &es)
	return es
}

// matchBatch is how many new matches pass between shelf rebuilds.
const matchBatch = 40

// matchLimits are the ranks that get a MusicBrainz match per kind.
var matchLimits = map[Kind]int{KindSongs: 100, KindAlbums: 100, KindArtists: 60, KindNew: 40, KindList: 60}

// Backlog reports names still waiting for a match (the shared fill gives
// matching a bigger share of MusicBrainz meanwhile).
func (s *Service) Backlog() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backlog
}

// matchNew searches names not matched yet, best ranks first across all
// regions, up to MatchMax searches in one run. Its MusicBrainz calls are
// priority calls: at least half of the shared slot while names wait.
func (s *Service) matchNew(ctx context.Context) int {
	ctx = providers.WithPriority(ctx)
	if s.opts.Search == nil {
		return 0
	}
	type job struct {
		rank                    float64
		kind, name, artist, ext string
	}
	var jobs []job
	seen := map[string]bool{}
	regionAt := map[string]int{}
	for _, src := range s.plan {
		if _, ok := regionAt[src.Region]; !ok {
			regionAt[src.Region] = len(regionAt)
		}
	}
	for _, src := range s.plan {
		es, _ := s.latest(src.Key)
		// Best ranks first, the biggest regions a little ahead.
		weight := 1 + 0.25*float64(regionAt[src.Region])
		for _, e := range es {
			if e.Rank > matchLimits[src.Kind] {
				continue
			}
			add := func(kind, name, artist, ext string) {
				if kind == "album" {
					if _, _, fresh := s.matcher.Known(ctx, kind, name, artist); !fresh {
						// New names are matched without edition junk.
						name = cleanTitle(name)
					}
				}
				k := kind + "|" + name + "|" + artist
				if name == "" || seen[k] {
					return
				}
				seen[k] = true
				if _, _, fresh := s.matcher.Known(ctx, kind, name, artist); !fresh {
					jobs = append(jobs, job{float64(e.Rank) * weight, kind, name, artist, ext})
				}
			}
			switch src.Kind {
			case KindAlbums, KindNew:
				add("album", e.Title, e.Artist, "")
			case KindList:
				if isAlbum(e) {
					add("album", e.Album, e.Artist, "")
				} else if e.Rank <= 30 {
					// A song on its own single: its single's release group.
					add("album", e.Title, e.Artist, "")
				}
			case KindSongs:
				if isAlbum(e) && e.Rank <= matchLimits[KindSongs] {
					// The song's album, from the Spotify API.
					add("album", e.Album, e.Artist, "")
				}
				if e.Rank <= 50 {
					// The song's single (for songs without an album, and to
					// re-check a new song matched to an old album).
					add("album", e.Title, e.Artist, "")
				}
			case KindCategory:
				continue
			}
			add("artist", e.Artist, "", e.ExtArtist)
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].rank < jobs[j].rank })
	s.mu.Lock()
	s.backlog = len(jobs) > 0
	s.mu.Unlock()
	n := 0
	for _, j := range jobs {
		if n >= s.opts.MatchMax || ctx.Err() != nil {
			break
		}
		if n > 0 && s.opts.MatchPace > 0 {
			select {
			case <-ctx.Done():
				return n
			case <-time.After(s.opts.MatchPace):
			}
		}
		if !s.matchLink(ctx, j.kind, j.name, j.artist, j.ext) {
			if c, ok := s.matcher.Match(ctx, j.kind, j.name, j.artist); ok && c.Date != "" {
				_, _ = s.db.ExecContext(ctx, "UPDATE matches SET first_date = ? WHERE kind = ? AND name = ? AND artist = ?", c.Date, j.kind, j.name, j.artist)
			}
		}
		n++
		if n%matchBatch == 0 {
			// Shelves fill as names match (best ranks first), not only
			// after the whole backlog.
			s.rebuild(ctx, true)
		}
	}
	s.mu.Lock()
	s.backlog = len(jobs) > n
	s.mu.Unlock()
	return n
}

// matchLink keeps the MusicBrainz id linked to the source's own id when
// there is one (exact, no name guessing); false: fall back to names.
func (s *Service) matchLink(ctx context.Context, kind, name, artist, ext string) bool {
	if s.opts.Link == nil || ext == "" {
		return false
	}
	id, err := s.opts.Link(ctx, kind, ext)
	if err != nil || !mbidRe.MatchString(id) {
		return false
	}
	_, _ = s.db.ExecContext(ctx, "INSERT OR REPLACE INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES (?, ?, ?, ?, '', ?)", kind, name, artist, id, time.Now().Unix())
	s.linked++
	return true
}

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// lookup answers a stored match without searching.
func (s *Service) lookup(ctx context.Context) Lookup {
	// One rebuild asks the same (kind, name, artist) many times (the global
	// lists once per region), so answers, misses too, are kept for this call.
	type answer struct {
		mbid, art string
		ok        bool
	}
	var mu sync.Mutex
	memo := map[[3]string]answer{}
	return func(kind, name, artist string) (string, string, bool) {
		key := [3]string{kind, name, artist}
		mu.Lock()
		a, hit := memo[key]
		mu.Unlock()
		if !hit {
			a.mbid, a.art, a.ok = s.knownMatch(ctx, kind, name, artist)
			mu.Lock()
			memo[key] = a
			mu.Unlock()
		}
		return a.mbid, a.art, a.ok
	}
}

// knownMatch is one uncached lookup of a stored match.
func (s *Service) knownMatch(ctx context.Context, kind, name, artist string) (string, string, bool) {
	c, ok, _ := s.matcher.Known(ctx, kind, name, artist)
	if !ok && kind == "album" {
		if t := cleanTitle(name); t != name {
			c, ok, _ = s.matcher.Known(ctx, kind, t, artist)
		}
	}
	if !ok || !mbidRe.MatchString(c.MBID) {
		return "", "", false
	}
	return c.MBID, c.ArtistMBID, true
}

// Regions are the regions with a stored song chart (global first).
func (s *Service) Regions() []string {
	var out []string
	for _, src := range s.plan {
		if src.Kind == KindSongs {
			if es, _ := s.latest(src.Key); len(es) > 0 {
				out = append(out, src.Region)
			}
		}
	}
	return out
}

// rebuild makes every region's shelves from the stored entries and queues
// members not seen before for pictures and videos.
func (s *Service) rebuild(ctx context.Context, prefetch bool) {
	now := s.opts.Now()
	in := Inputs{Now: now, Lookup: s.lookup(ctx), Ago: map[string][]Entry{}, Lists: map[string][]Entry{}, Date: s.dates(), Rematch: s.logRematch}
	if s.opts.Fresh != nil {
		in.Fresh = s.opts.Fresh()
	}
	asOf := ""
	for _, src := range s.plan {
		es, d := s.latest(src.Key)
		if len(es) == 0 {
			continue
		}
		in.Lists[src.Key] = es
		if d > asOf {
			asOf = d
		}
		if src.Kind == KindSongs {
			in.Ago[src.Key] = s.at(src.Key, now.UTC().AddDate(0, 0, -7).Format("2006-01-02"))
		}
	}
	in.AsOf = asOf
	regions := map[string]bool{"global": true}
	for _, src := range s.plan {
		regions[src.Region] = true
	}
	next := map[string]served{}
	var fresh []Item
	for r := range regions {
		sh := Build(r, in)
		sh.Season = s.Season()
		if sh.Empty() {
			continue
		}
		b, err := json.Marshal(sh)
		if err != nil {
			continue
		}
		h := sha256.Sum256(b)
		next[r] = served{b, `"` + hex.EncodeToString(h[:16]) + `"`}
		for _, l := range sh.Blocks {
			fresh = append(fresh, l...)
		}
	}
	s.mu.Lock()
	s.shelves = next
	s.last = now
	var queue [][2]string
	for _, it := range fresh {
		for _, m := range it.members() {
			if !s.known[m[1]] {
				s.known[m[1]] = true
				queue = append(queue, m)
			}
		}
	}
	s.mu.Unlock()
	if prefetch && s.opts.Prefetch != nil {
		for _, m := range queue {
			s.opts.Prefetch(m[0], m[1])
		}
	}
}

// dates answers release groups' first release dates: from the matches,
// else from stored MusicBrainz answers (once per mbid per build).
func (s *Service) dates() func(mbid string) string {
	known := map[string]string{}
	if rows, err := s.db.Query(`SELECT mbid, first_date FROM matches WHERE first_date != ''`); err == nil {
		for rows.Next() {
			var id, d string
			if rows.Scan(&id, &d) == nil {
				known[id] = d
			}
		}
		rows.Close()
	}
	asked := map[string]bool{}
	return func(mbid string) string {
		if d, ok := known[mbid]; ok || asked[mbid] || s.opts.RGInfo == nil {
			return d
		}
		asked[mbid] = true
		d := s.opts.RGInfo(mbid)
		known[mbid] = d
		return d
	}
}

// logRematch logs a song charting now that was shown as an old album and
// now shows as its single or its artist (once per song and process).
func (s *Service) logRematch(song, artist, from, to string) {
	k := song + "|" + artist
	s.mu.Lock()
	seen := s.rematched[k]
	s.rematched[k] = true
	s.mu.Unlock()
	if !seen {
		s.opts.Log("lists: re-matched %q by %s from a %s album to %s", song, artist, from, to)
	}
}

// SetFresh sets the fresh-releases reader (call before Run).
func (s *Service) SetFresh(f func() []FreshAlbum) { s.opts.Fresh = f }

// SetRGInfo sets the stored release-group date reader (call before Run).
func (s *Service) SetRGInfo(f func(mbid string) string) { s.opts.RGInfo = f }

// Shelves answers a region's shelves JSON and ETag; a region without its
// own chart gets the global shelves.
func (s *Service) Shelves(region string) ([]byte, string, bool) {
	region = strings.ToLower(region)
	if region == "" {
		region = "global"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.shelves[region]
	if !ok {
		v, ok = s.shelves["global"]
	}
	return v.body, v.etag, ok
}

// Members lists every shelf album and artist (for the shared fill:
// pictures, artist details and videos ahead).
func (s *Service) Members(context.Context) []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	var out []Member
	regions := make([]string, 0, len(s.shelves))
	for r := range s.shelves {
		regions = append(regions, r)
	}
	sort.Strings(regions)
	for _, r := range regions {
		var sh Shelves
		if json.Unmarshal(s.shelves[r].body, &sh) != nil {
			continue
		}
		for _, name := range blockNames(sh.Blocks) {
			for _, it := range sh.Blocks[name] {
				for _, m := range it.members() {
					if !seen[m[1]] {
						seen[m[1]] = true
						out = append(out, Member{Kind: m[0], MBID: m[1]})
					}
				}
			}
		}
	}
	return out
}

// Member is one album ("release-group") or artist to fetch ahead.
type Member struct{ Kind, MBID string }

// Status is the collector's state for /v1/stats.
type Status struct {
	MonthCap   int      `json:"month_cap"`
	DayCap     int      `json:"day_cap"`
	UsedMonth  int      `json:"used_month"`
	UsedToday  int      `json:"used_today"`
	PlanCost   int      `json:"plan_month_cost"`
	Sources    int      `json:"sources"`
	Regions    []string `json:"regions"`
	LastBuild  string   `json:"last_build,omitempty"`
	Matched    int      `json:"matched"`
	NotMatched int      `json:"not_matched"`
	// TrackAlbums is how many charted songs have their album kept.
	TrackAlbums int `json:"track_albums"`
	// Quota is the source's own count for the period; QuotaShare the most
	// of it the server spends.
	Quota      Quota   `json:"quota"`
	QuotaShare float64 `json:"quota_share"`
}

// Status answers the budget used and the shelves' state.
func (s *Service) Status(context.Context) Status {
	m, d := s.Used()
	st := Status{MonthCap: s.opts.MonthCap, DayCap: s.opts.DayCap, UsedMonth: m, UsedToday: d, PlanCost: PlanCost(s.plan), Sources: len(s.plan), Regions: s.Regions(), Quota: s.Quota(), QuotaShare: QuotaShare}
	if st.Regions == nil {
		st.Regions = []string{}
	}
	s.mu.RLock()
	if !s.last.IsZero() {
		st.LastBuild = s.last.UTC().Format(time.RFC3339)
	}
	s.mu.RUnlock()
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM matches WHERE mbid != ''`).Scan(&st.Matched)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM matches WHERE mbid = ''`).Scan(&st.NotMatched)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&st.TrackAlbums)
	return st
}

// SetPrefetch sets the hook that queues new shelf members (call before Run).
func (s *Service) SetPrefetch(f func(kind, mbid string)) { s.opts.Prefetch = f }

// SetExactArtist sets Options.ExactArtist after Open.
func (s *Service) SetExactArtist(f func(ctx context.Context, source, id, name string, codes []string) string) {
	s.opts.ExactArtist = f
}

// SetSharedName sets Options.SharedName after Open.
func (s *Service) SetSharedName(f func(ctx context.Context, name string) bool) {
	s.opts.SharedName = f
}
