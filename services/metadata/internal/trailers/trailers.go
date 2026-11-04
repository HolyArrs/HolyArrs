// Package trailers picks each title's trailer (a YouTube key from TMDB's
// videos) and keeps it healthy: a daily job re-checks the keys people
// actually ask for with YouTube's oEmbed answer (no video is fetched),
// marks removed, private or blocked keys broken and moves on to the next
// candidate. The checks are spread over the day (one every Pause, with
// jitter, at most DailyCap a day, most asked first, backing off on 429).
// Video addresses, storyboards and the "most replayed" graph come from
// HolyArrs's own reader (heatmap.go); nothing here runs another program.
package trailers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Fetcher is the server's paced provider client.
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}

// Status of a key.
const (
	StatusOK        = "ok"
	StatusBroken    = "broken"
	StatusUnchecked = "unchecked"
)

// ErrNoTrailer: TMDB lists no usable trailer or teaser for the title.
var ErrNoTrailer = errors.New("no trailer")

// Options configure the service.
type Options struct {
	Fetcher Fetcher
	// HTTP is used for the oEmbed checks (default 10 s timeout).
	HTTP *http.Client
	// OEmbedURL is YouTube's oEmbed endpoint.
	OEmbedURL string
	// Every is how often the health job runs (default 24 h).
	Every time.Duration
	// Top is how many of the most asked-for titles are re-checked (default 500).
	Top int
	// Pause spaces the oEmbed checks (default 30 s, plus up to half again
	// as jitter).
	Pause time.Duration
	// DailyCap is the most oEmbed checks a day (default 500).
	DailyCap int
	// Fresh is how long a title's TMDB videos are kept (default 7 days).
	Fresh time.Duration
	// AskCap is how many distinct titles one asker (a credential or a
	// network) may add to the "most asked" counts a day (default 50).
	AskCap int
	Now    func() time.Time
}

// Pick is the answer of GET /v1/trailers/{movie|tv}/{tmdb_id}.
type Pick struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	CheckedAt string `json:"checked_at"`
	Status    string `json:"status"`
}

// Stats are the trailer numbers in /v1/stats.
type Stats struct {
	Checked     int64  `json:"checked"`
	Broken      int64  `json:"broken"`
	Replaced    int64  `json:"replaced"`
	ChecksToday     int64  `json:"checks_today"`
	LastRun         string `json:"last_run"`
	Titles          int64  `json:"titles"`
}

type candidate struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Language string `json:"language"`
}

// Service keeps the picks and key health in its own SQLite file.
type Service struct {
	db   *sql.DB
	opts Options
	mu   sync.Mutex           // one health run at a time
	skip map[string]time.Time // keys with an unknown answer, retried later
}

func Open(path string, o Options) (*Service, error) {
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if o.OEmbedURL == "" {
		o.OEmbedURL = "https://www.youtube.com/oembed"
	}
	if o.Every <= 0 {
		o.Every = 24 * time.Hour
	}
	if o.Top <= 0 {
		o.Top = 500
	}
	if o.Pause < 0 {
		o.Pause = 0
	} else if o.Pause == 0 {
		o.Pause = 30 * time.Second
	}
	if o.DailyCap <= 0 {
		o.DailyCap = 500
	}
	if o.Fresh <= 0 {
		o.Fresh = 7 * 24 * time.Hour
	}
	if o.AskCap <= 0 {
		o.AskCap = 50
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS titles (kind TEXT NOT NULL, tmdb_id INTEGER NOT NULL, lang TEXT NOT NULL, candidates TEXT NOT NULL, fetched_at INTEGER NOT NULL, requests INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(kind, tmdb_id, lang)) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS titles_requests ON titles(requests DESC);
CREATE TABLE IF NOT EXISTS keys (key TEXT PRIMARY KEY, status TEXT NOT NULL, checked_at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS asks (who TEXT NOT NULL, kind TEXT NOT NULL, tmdb_id INTEGER NOT NULL, lang TEXT NOT NULL, day INTEGER NOT NULL, PRIMARY KEY(who, kind, tmdb_id, lang)) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS asks_day ON asks(day);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Service{db: db, opts: o, skip: map[string]time.Time{}}, nil
}

func (s *Service) Close() error { return s.db.Close() }

var youTubeKey = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

// rank orders TMDB's videos: official Trailer, Trailer, official Teaser,
// Teaser; the asked language before English before the rest; TMDB's own
// order otherwise. Clips and non-YouTube videos are left out.
func rank(results []struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Site     string `json:"site"`
	Type     string `json:"type"`
	Official bool   `json:"official"`
	Language string `json:"iso_639_1"`
}, lang string) []candidate {
	type scored struct {
		c candidate
		s int
	}
	var list []scored
	seen := map[string]bool{}
	for _, v := range results {
		if v.Site != "YouTube" || !youTubeKey.MatchString(v.Key) || seen[v.Key] {
			continue
		}
		sc := 0
		switch v.Type {
		case "Trailer":
			sc = 100
		case "Teaser":
			sc = 50
		default:
			continue
		}
		if v.Official {
			sc += 10
		}
		switch l := strings.ToLower(v.Language); {
		case l == lang:
			sc += 5
		case l == "en" || l == "":
			sc += 2
		}
		seen[v.Key] = true
		list = append(list, scored{candidate{v.Key, v.Name, strings.ToLower(v.Language)}, sc})
	}
	// Stable insertion sort: keeps TMDB's order among equals.
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].s > list[j-1].s; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	out := make([]candidate, 0, len(list))
	for _, x := range list {
		out = append(out, x.c)
	}
	return out
}

// ValidKind maps the route's kind to TMDB's path part.
func ValidKind(kind string) bool { return kind == "movie" || kind == "tv" }

// Pick answers one title's trailer in lang (two letters). The ask counts
// toward the title's popularity once a day per asker (who: a credential
// or a network), and each asker adds at most AskCap titles a day, so one
// caller cannot steer which keys the health job checks. When TMDB cannot
// be read (budget spent, outage), older stored videos are used.
func (s *Service) Pick(ctx context.Context, kind string, id int64, lang, who string) (Pick, error) {
	if !ValidKind(kind) || id <= 0 || len(lang) != 2 {
		return Pick{}, ErrNoTrailer
	}
	now := s.opts.Now()
	var raw string
	var fetched int64
	err := s.db.QueryRowContext(ctx, "SELECT candidates, fetched_at FROM titles WHERE kind=? AND tmdb_id=? AND lang=?", kind, id, lang).Scan(&raw, &fetched)
	var cands []candidate
	stored := err == nil && json.Unmarshal([]byte(raw), &cands) == nil
	n := 0
	if s.countAsk(ctx, who, kind, id, lang, now) {
		n = 1
	}
	keep := s.opts.Fresh
	if stored && len(cands) == 0 {
		// No trailer found: asked again soon (cache audit short-retry rule).
		keep = min(keep, emptyKeep)
	}
	if stored && now.Sub(time.Unix(fetched, 0)) < keep {
		_, _ = s.db.ExecContext(ctx, "UPDATE titles SET requests = requests + ? WHERE kind=? AND tmdb_id=? AND lang=?", n, kind, id, lang)
		return s.choose(ctx, cands)
	}
	fresh, err := s.videos(ctx, kind, id, lang)
	if err != nil {
		if stored && !errors.Is(err, ErrNoTrailer) {
			_, _ = s.db.ExecContext(ctx, "UPDATE titles SET requests = requests + ? WHERE kind=? AND tmdb_id=? AND lang=?", n, kind, id, lang)
			return s.choose(ctx, cands)
		}
		return Pick{}, err
	}
	b, _ := json.Marshal(fresh)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO titles (kind, tmdb_id, lang, candidates, fetched_at, requests) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, tmdb_id, lang) DO UPDATE SET candidates = excluded.candidates, fetched_at = excluded.fetched_at, requests = titles.requests + excluded.requests`, kind, id, lang, string(b), now.Unix(), n); err != nil {
		return Pick{}, err
	}
	return s.choose(ctx, fresh)
}

// countAsk reports whether this ask adds to the title's count: the first
// today from who, within who's AskCap distinct titles today.
func (s *Service) countAsk(ctx context.Context, who, kind string, id int64, lang string, now time.Time) bool {
	day := now.Unix() / 86400
	_, _ = s.db.ExecContext(ctx, "DELETE FROM asks WHERE day < ?", day)
	var seen, today int
	if s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asks WHERE who=? AND kind=? AND tmdb_id=? AND lang=?", who, kind, id, lang).Scan(&seen) != nil || seen > 0 {
		return false
	}
	if s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asks WHERE who=?", who).Scan(&today) != nil || today >= s.opts.AskCap {
		return false
	}
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO asks (who, kind, tmdb_id, lang, day) VALUES (?, ?, ?, ?, ?)", who, kind, id, lang, day)
	return err == nil
}

// choose returns the first candidate whose key is not broken.
func (s *Service) choose(ctx context.Context, cands []candidate) (Pick, error) {
	for _, c := range cands {
		st, at := s.keyStatus(ctx, c.Key)
		if st == StatusBroken {
			continue
		}
		p := Pick{Key: c.Key, Name: c.Name, Language: c.Language, Status: st}
		if at > 0 {
			p.CheckedAt = time.Unix(at, 0).UTC().Format(time.RFC3339)
		}
		return p, nil
	}
	return Pick{}, ErrNoTrailer
}

func (s *Service) keyStatus(ctx context.Context, key string) (string, int64) {
	var st string
	var at int64
	if s.db.QueryRowContext(ctx, "SELECT status, checked_at FROM keys WHERE key=?", key).Scan(&st, &at) != nil {
		return StatusUnchecked, 0
	}
	return st, at
}

// videos reads the title's videos in lang, English and no language.
func (s *Service) videos(ctx context.Context, kind string, id int64, lang string) ([]candidate, error) {
	path := "/" + kind + "/" + strconv.FormatInt(id, 10) + "/videos"
	langs := lang
	if lang != "en" {
		langs += ",en"
	}
	params, err := model.TMDBRead(path, url.Values{"include_video_language": {langs + ",null"}})
	if err != nil {
		return nil, err
	}
	res, err := s.opts.Fetcher.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: path, Params: params, Page: 1, Limit: 20})
	if err != nil {
		if fe := (*model.FetchError)(nil); errors.As(err, &fe) && fe.Status == http.StatusNotFound {
			return nil, ErrNoTrailer
		}
		return nil, err
	}
	var body struct {
		Results []struct {
			Key      string `json:"key"`
			Name     string `json:"name"`
			Site     string `json:"site"`
			Type     string `json:"type"`
			Official bool   `json:"official"`
			Language string `json:"iso_639_1"`
		} `json:"results"`
	}
	if err := json.Unmarshal(res.Data, &body); err != nil {
		return nil, err
	}
	out := rank(body.Results, lang)
	if len(out) == 0 && kind == "tv" {
		out = s.seasonVideos(ctx, id, langs, lang)
	}
	return out, nil
}

// emptyKeep is how long "no trailer" stands before TMDB is asked again.
const emptyKeep = 15 * time.Minute

// seasonWalk caps how many seasons a show without its own trailer tries.
const seasonWalk = 5

// seasonVideos walks a show's seasons newest first and answers the first
// season's ranked trailers (Grey's Anatomy has none at show level).
func (s *Service) seasonVideos(ctx context.Context, id int64, langs, lang string) []candidate {
	show := "/tv/" + strconv.FormatInt(id, 10)
	res, err := s.opts.Fetcher.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: show, Page: 1, Limit: 20})
	if err != nil {
		return nil
	}
	var sh struct {
		Seasons []struct {
			Number int `json:"season_number"`
		} `json:"seasons"`
	}
	_ = json.Unmarshal(res.Data, &sh)
	var nums []int
	for _, x := range sh.Seasons {
		if x.Number > 0 {
			nums = append(nums, x.Number)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(nums)))
	for i, n := range nums {
		if i >= seasonWalk || ctx.Err() != nil {
			break
		}
		path := show + "/season/" + strconv.Itoa(n)
		params, err := model.TMDBRead(path, url.Values{"append_to_response": {"videos"}, "include_video_language": {langs + ",null"}})
		if err != nil {
			return nil
		}
		r, err := s.opts.Fetcher.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: path, Params: params, Page: 1, Limit: 20})
		if err != nil {
			continue
		}
		var body struct {
			Videos struct {
				Results []struct {
					Key      string `json:"key"`
					Name     string `json:"name"`
					Site     string `json:"site"`
					Type     string `json:"type"`
					Official bool   `json:"official"`
					Language string `json:"iso_639_1"`
				} `json:"results"`
			} `json:"videos"`
		}
		if json.Unmarshal(r.Data, &body) != nil {
			continue
		}
		if out := rank(body.Videos.Results, lang); len(out) > 0 {
			return out
		}
	}
	return nil
}

// check asks oEmbed about one key: 200 is fine; 400, 401, 403 and 404 are
// removed, private or blocked (broken); anything else is not known, and
// limited says YouTube asked us to slow down (429).
func (s *Service) check(ctx context.Context, key string) (status string, known, limited bool) {
	q := url.Values{"url": {"https://www.youtube.com/watch?v=" + key}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opts.OEmbedURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", false, false
	}
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return "", false, false
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return StatusOK, true, false
	case 400, 401, 403, 404:
		return StatusBroken, true, false
	case 429:
		return "", false, true
	}
	return "", false, false
}

func (s *Service) meta(key string) string {
	var v string
	_ = s.db.QueryRow("SELECT v FROM meta WHERE k=?", key).Scan(&v)
	return v
}

func (s *Service) setMeta(key, v string) {
	_, _ = s.db.Exec("INSERT INTO meta (k, v) VALUES (?, ?) ON CONFLICT (k) DO UPDATE SET v = excluded.v", key, v)
}

func (s *Service) addMeta(key string, n int64) {
	old, _ := strconv.ParseInt(s.meta(key), 10, 64)
	s.setMeta(key, strconv.FormatInt(old+n, 10))
}

// next finds the next key due a check: for the most asked-for titles in
// order, the current (first not broken) key when it was never checked or
// was checked over 20 hours ago. more says another candidate follows it.
func (s *Service) next(ctx context.Context, now time.Time) (key string, more bool, err error) {
	rows, err := s.db.QueryContext(ctx, "SELECT candidates FROM titles ORDER BY requests DESC LIMIT ?", s.opts.Top)
	if err != nil {
		return "", false, err
	}
	var lists [][]candidate
	for rows.Next() {
		var raw string
		var c []candidate
		if rows.Scan(&raw) == nil && json.Unmarshal([]byte(raw), &c) == nil && len(c) > 0 {
			lists = append(lists, c)
		}
	}
	rows.Close()
	for _, cands := range lists {
		for i, c := range cands {
			st, at := s.keyStatus(ctx, c.Key)
			if st == StatusBroken {
				continue
			}
			if until, ok := s.skip[c.Key]; ok && now.Before(until) {
				break
			}
			if st == StatusUnchecked || now.Sub(time.Unix(at, 0)) > 20*time.Hour {
				return c.Key, i+1 < len(cands), nil
			}
			break // this title's current key is fine
		}
	}
	return "", false, nil
}

// day returns today's check count (UTC) before adding n.
func (s *Service) day(now time.Time, n int64) int64 {
	d := now.UTC().Format("2006-01-02")
	if s.meta("checks_day") != d {
		s.setMeta("checks_day", d)
		s.setMeta("checks_today", "0")
	}
	old, _ := strconv.ParseInt(s.meta("checks_today"), 10, 64)
	if n != 0 {
		s.setMeta("checks_today", strconv.FormatInt(old+n, 10))
	}
	return old
}

// CheckNext checks one due key with oEmbed. did is false when nothing is
// due or today's cap is reached; limited is true on a 429.
func (s *Service) CheckNext(ctx context.Context) (did, limited bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opts.Now()
	if s.day(now, 0) >= int64(s.opts.DailyCap) {
		return false, false, nil
	}
	key, more, err := s.next(ctx, now)
	if err != nil || key == "" {
		return false, false, err
	}
	got, known, limited := s.check(ctx, key)
	if !known {
		s.skip[key] = now.Add(time.Hour) // keep the key; try again later
		return true, limited, nil
	}
	delete(s.skip, key)
	s.day(now, 1)
	s.addMeta("checked", 1)
	_, _ = s.db.ExecContext(ctx, "INSERT INTO keys (key, status, checked_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET status = excluded.status, checked_at = excluded.checked_at", key, got, now.Unix())
	if got == StatusBroken && more {
		s.addMeta("replaced", 1) // the next candidate takes over
	}
	return true, false, nil
}

// Daily runs the once-a-day part: it notes the run.
func (s *Service) Daily(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setMeta("last_run", s.opts.Now().UTC().Format(time.RFC3339))
}

// Refresh checks every due key at once (no pacing, still capped), then
// runs Daily. Run paces the checks instead.
func (s *Service) Refresh(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		did, _, err := s.CheckNext(ctx)
		if err != nil {
			return err
		}
		if !did {
			break
		}
	}
	s.Daily(ctx)
	return nil
}

// Stats reads the numbers for /v1/stats.
func (s *Service) Stats() Stats {
	st := Stats{LastRun: s.meta("last_run")}
	if s.meta("checks_day") == s.opts.Now().UTC().Format("2006-01-02") {
		st.ChecksToday, _ = strconv.ParseInt(s.meta("checks_today"), 10, 64)
	}
	st.Checked, _ = strconv.ParseInt(s.meta("checked"), 10, 64)
	st.Replaced, _ = strconv.ParseInt(s.meta("replaced"), 10, 64)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM keys WHERE status = ?", StatusBroken).Scan(&st.Broken)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM titles").Scan(&st.Titles)
	return st
}

// Run paces the key checks (one every Pause plus jitter; after a 429 the
// wait doubles from 1 min up to 1 h) and runs Daily every Every (first
// run 1 min after start, so a restart does not wait a day).
func (s *Service) Run(ctx context.Context) {
	go func() {
		backoff := time.Minute
		for {
			wait := s.opts.Pause + rand.N(s.opts.Pause/2+1)
			did, limited, err := s.CheckNext(ctx)
			switch {
			case limited:
				wait, backoff = backoff, min(backoff*2, time.Hour)
			case !did || err != nil:
				wait = max(wait, 5*time.Minute) // nothing due, cap reached or no database
			default:
				backoff = time.Minute
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}()
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		s.Daily(ctx)
		wait = s.opts.Every
	}
}
