package extras

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

// MDBList answers outside ratings. Each title's answer is stored for good
// (Open; the permanent data) and answered from the store at once; one that
// is due is refreshed once in the background, on ask only, never on a
// schedule: 2 days after a release under 30 days old, 14 days under a year,
// else 90 days (10% jitter). A real "not found" is kept 7 days; a failure
// (429, 5xx, a timeout, the spent day budget) keeps the stored copy and is
// tried again an hour later. Off without Key. Calls go through the server's
// upstream budget ("mdblist", 600 a day).
type MDBList struct {
	Key       string
	URL       string
	UserAgent string
	HTTP      *http.Client
	Now       func() time.Time

	db *sql.DB

	mu       sync.Mutex
	flights  map[string]*ratingsFlight
	bg       chan struct{}
	failures int64
	lastErr  string
}

type ratingsFlight struct {
	done chan struct{}
	body []byte
	err  error
}

// Ratings due times.
const (
	ratingsFresh  = 2 * 24 * time.Hour
	ratingsRecent = 14 * 24 * time.Hour
	ratingsOld    = 90 * 24 * time.Hour
	ratingsMiss   = 7 * 24 * time.Hour
	ratingsRetry  = time.Hour
)

// Score is one outside rating. Votes is 0 when the source gives none.
type Score struct {
	Source string  `json:"source"`
	Value  float64 `json:"value"`
	Max    float64 `json:"max"`
	Votes  int64   `json:"votes,omitempty"`
}

// Ratings is every known outside score for one title.
type Ratings struct {
	Kind   string  `json:"kind"`
	ID     string  `json:"id"`
	IMDbID string  `json:"imdb_id,omitempty"`
	Scores []Score `json:"scores"`
	Via    string  `json:"via"`
	// Released is the release date MDBList gives (YYYY-MM-DD), when known.
	Released string `json:"released,omitempty"`
}

// Open opens (or creates) the ratings store at path.
func (m *MDBList) Open(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS ratings(k TEXT PRIMARY KEY, body BLOB, fetched INTEGER NOT NULL, due INTEGER NOT NULL)`); err != nil {
		db.Close()
		return err
	}
	m.db = db
	return nil
}

// Close closes the store.
func (m *MDBList) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	return m.db.Close()
}

func (m *MDBList) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// dueAfter is a found answer's refresh time by its release age.
func dueAfter(body []byte, now time.Time) time.Duration {
	var r Ratings
	_ = json.Unmarshal(body, &r)
	released, _ := time.Parse("2006-01-02", r.Released)
	return refresh.AgeTTL(released, now, ratingsFresh, ratingsRecent, ratingsOld)
}

// Enabled reports whether the key is set.
func (m *MDBList) Enabled() bool { return m != nil && m.Key != "" }

// Status is the source state for /v1/stats.
func (m *MDBList) Status() Status {
	if !m.Enabled() {
		return Status{State: StateDisabledNoKey}
	}
	s := Status{State: StateOK}
	if m.db != nil {
		_ = m.db.QueryRow(`SELECT COUNT(*) FROM ratings WHERE body IS NOT NULL`).Scan(&s.Cached)
	}
	m.mu.Lock()
	s.Failures, s.LastError = m.failures, m.lastErr
	m.mu.Unlock()
	return s
}

// Ratings answers the JSON body for a TMDB movie or show id: the stored
// copy at once ("hit"; a due one is refreshed in the background), else
// one fetch ("miss").
func (m *MDBList) Ratings(ctx context.Context, kind, id string) ([]byte, string, error) {
	if !m.Enabled() {
		return nil, "", ErrDisabled
	}
	key := kind + ":" + id
	now := m.now()
	var body []byte
	var due int64
	stored := m.db != nil && m.db.QueryRowContext(ctx, `SELECT body, due FROM ratings WHERE k=?`, key).Scan(&body, &due) == nil
	if stored {
		if now.Unix() >= due {
			m.background(kind, id)
		}
		if body == nil {
			return nil, "hit", ErrNotFound
		}
		return body, "hit", nil
	}
	b, err := m.load(ctx, kind, id)
	return b, "miss", err
}

// Due reports whether a title's ratings are not stored yet or due again
// (a stored "not found" counts until it is due).
func (m *MDBList) Due(ctx context.Context, kind, id string) bool {
	if !m.Enabled() || m.db == nil {
		return false
	}
	var due int64
	if m.db.QueryRowContext(ctx, `SELECT due FROM ratings WHERE k=?`, kind+":"+id).Scan(&due) != nil {
		return true
	}
	return m.now().Unix() >= due
}

// Fetch fetches and stores one title's ratings now (the nightly
// prefetch); false when the call failed (the stored copy stays).
func (m *MDBList) Fetch(ctx context.Context, kind, id string) bool {
	m.mu.Lock()
	before := m.failures
	m.mu.Unlock()
	_, err := m.load(ctx, kind, id)
	m.mu.Lock()
	failed := m.failures != before
	m.mu.Unlock()
	return !failed && (err == nil || errors.Is(err, ErrNotFound))
}

// background refreshes one due answer, one at a time, never twice at once.
func (m *MDBList) background(kind, id string) {
	m.mu.Lock()
	if m.bg == nil {
		m.bg = make(chan struct{}, 1)
	}
	_, busy := m.flights[kind+":"+id]
	m.mu.Unlock()
	if busy {
		return
	}
	select {
	case m.bg <- struct{}{}:
	default:
		return // another refresh runs; this one is due again on the next ask
	}
	go func() {
		defer func() { <-m.bg }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = m.load(ctx, kind, id)
	}()
}

// load fetches one title's ratings (single flight) and stores the answer:
// found for its due time, not found for 7 days, a failure never (the
// stored copy stays and is tried again in an hour).
func (m *MDBList) load(ctx context.Context, kind, id string) ([]byte, error) {
	key := kind + ":" + id
	m.mu.Lock()
	if m.flights == nil {
		m.flights = map[string]*ratingsFlight{}
	}
	if f, ok := m.flights[key]; ok {
		m.mu.Unlock()
		select {
		case <-f.done:
			return f.body, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &ratingsFlight{done: make(chan struct{})}
	m.flights[key] = f
	m.mu.Unlock()
	f.body, f.err = m.fetch(ctx, kind, id)
	now := m.now()
	switch {
	case f.err == nil:
		m.put(key, f.body, now, now.Add(dueAfter(f.body, now)))
	case errors.Is(f.err, ErrNotFound):
		if m.db != nil {
			// A title that had ratings keeps them; one that never had any
			// is "not found" for a week.
			_, _ = m.db.Exec(`INSERT INTO ratings(k,body,fetched,due) VALUES(?,NULL,?,?) ON CONFLICT(k) DO UPDATE SET due=excluded.due`, key, now.Unix(), now.Add(ratingsMiss).Unix())
		}
	default:
		if m.db != nil {
			_, _ = m.db.Exec(`UPDATE ratings SET due=? WHERE k=?`, now.Add(ratingsRetry).Unix(), key)
		}
		m.mu.Lock()
		m.failures++
		m.lastErr = "upstream_failed"
		m.mu.Unlock()
		var body []byte
		if m.db != nil && m.db.QueryRow(`SELECT body FROM ratings WHERE k=? AND body IS NOT NULL`, key).Scan(&body) == nil {
			f.body, f.err = body, nil
		}
	}
	m.mu.Lock()
	delete(m.flights, key)
	m.mu.Unlock()
	close(f.done)
	return f.body, f.err
}

func (m *MDBList) put(key string, body []byte, now, due time.Time) {
	if m.db == nil {
		return
	}
	_, _ = m.db.Exec(`INSERT INTO ratings(k,body,fetched,due) VALUES(?,?,?,?) ON CONFLICT(k) DO UPDATE SET body=excluded.body, fetched=excluded.fetched, due=excluded.due`, key, body, now.Unix(), due.Unix())
}

// keyParam is the query parameter MDBList reads its key from.
const keyParam = "apikey"

// upstream sources kept, with their scale.
var mdbSources = map[string]struct {
	name string
	max  float64
}{
	"imdb":       {"imdb", 10},
	"tomatoes":   {"rt_critics", 100},
	"popcorn":    {"rt_audience", 100},
	"metacritic": {"metacritic", 100},
	"letterboxd": {"letterboxd", 5},
	"trakt":      {"trakt", 100},
}

func (m *MDBList) fetch(ctx context.Context, kind, id string) ([]byte, error) {
	base := m.URL
	if base == "" {
		base = MDBListURL
	}
	media := map[string]string{"movie": "movie", "tv": "show"}[kind]
	var raw struct {
		IDs struct {
			IMDb string `json:"imdb"`
		} `json:"ids"`
		Released string `json:"released"`
		Ratings  []struct {
			Source string   `json:"source"`
			Value  *float64 `json:"value"`
			Votes  *int64   `json:"votes"`
		} `json:"ratings"`
	}
	params := url.Values{}
	params.Set(keyParam, strings.TrimSpace(m.Key))
	if err := getJSON(ctx, m.HTTP, base+"/tmdb/"+media+"/"+id+"?"+params.Encode(), m.UserAgent, nil, &raw); err != nil {
		return nil, err
	}
	r := Ratings{Kind: kind, ID: id, IMDbID: raw.IDs.IMDb, Scores: []Score{}, Via: "mdblist", Released: raw.Released}
	for _, s := range raw.Ratings {
		src, ok := mdbSources[s.Source]
		if !ok || s.Value == nil || *s.Value <= 0 || *s.Value > src.max {
			continue
		}
		sc := Score{Source: src.name, Value: *s.Value, Max: src.max}
		if s.Votes != nil && *s.Votes > 0 {
			sc.Votes = *s.Votes
		}
		r.Scores = append(r.Scores, sc)
	}
	if len(r.Scores) == 0 {
		return nil, ErrNotFound
	}
	return json.Marshal(r)
}
