// Package refresh keeps the change-driven refresh state of stored answers:
// when each was last verified, whether an upstream change marked it stale,
// and when it was last asked for. Feeds (the TMDB change lists, MusicBrainz
// replication packets) mark rows stale; the server answers from the store
// and refreshes stale rows in the background.
package refresh

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Item is one tracked stored answer.
type Item struct {
	Key      string
	Query    string // JSON of the model.Query, used to refetch
	Ref      string // upstream identity, e.g. "tmdb:movie:603", "musicbrainz:<mbid>"
	Verified time.Time
	Stale    bool
}

// Counters kept across restarts.
const (
	CountChanges = "changes_applied"
	CountSaved   = "upstream_calls_saved"
	CountMarked  = "rows_marked_stale"
)

// Tracker is the refresh state in its own SQLite file.
type Tracker struct {
	db  *sql.DB
	Now func() time.Time
	mu  sync.Mutex
}

func Open(path string) (*Tracker, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS items (key TEXT PRIMARY KEY, query TEXT NOT NULL, ref TEXT NOT NULL, verified INTEGER NOT NULL, access INTEGER NOT NULL, stale INTEGER NOT NULL DEFAULT 0, stale_at INTEGER NOT NULL DEFAULT 0) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS items_ref ON items(ref);
CREATE INDEX IF NOT EXISTS items_stale ON items(stale, access);
CREATE INDEX IF NOT EXISTS items_verified ON items(verified);
CREATE INDEX IF NOT EXISTS items_access ON items(access);
CREATE TABLE IF NOT EXISTS kv (name TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS counters (name TEXT PRIMARY KEY, n INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lags (at INTEGER NOT NULL, seconds INTEGER NOT NULL);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Tracker{db: db, Now: time.Now}, nil
}

func (t *Tracker) Close() error { return t.db.Close() }

func (t *Tracker) now() time.Time { return t.Now() }

// Record notes a fresh fetch of key: verified now, not stale. When the row
// was stale, the time from the change being seen to being applied is kept.
func (t *Tracker) Record(ctx context.Context, key, query, ref string) error {
	now := t.now().Unix()
	var staleAt int64
	var stale int
	err := t.db.QueryRowContext(ctx, `SELECT stale, stale_at FROM items WHERE key=?`, key).Scan(&stale, &staleAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := t.db.ExecContext(ctx, `INSERT INTO items(key,query,ref,verified,access,stale,stale_at) VALUES(?,?,?,?,?,0,0)
ON CONFLICT(key) DO UPDATE SET query=excluded.query, ref=excluded.ref, verified=excluded.verified, stale=0, stale_at=0`, key, query, ref, now, now); err != nil {
		return err
	}
	if stale == 1 && staleAt > 0 {
		t.Add(ctx, CountChanges, 1)
		_, _ = t.db.ExecContext(ctx, `INSERT INTO lags(at,seconds) VALUES(?,?)`, now, max(0, now-staleAt))
		_, _ = t.db.ExecContext(ctx, `DELETE FROM lags WHERE rowid NOT IN (SELECT rowid FROM lags ORDER BY at DESC LIMIT 1000)`)
	}
	return nil
}

// State reports a key's last verification and stale flag.
func (t *Tracker) State(ctx context.Context, key string) (verified time.Time, stale, found bool) {
	var v int64
	var s int
	if t.db.QueryRowContext(ctx, `SELECT verified, stale FROM items WHERE key=?`, key).Scan(&v, &s) != nil {
		return time.Time{}, false, false
	}
	return time.Unix(v, 0), s == 1, true
}

// Touch notes a request for key (at most once a minute per row).
func (t *Tracker) Touch(ctx context.Context, key string) {
	now := t.now().Unix()
	_, _ = t.db.ExecContext(ctx, `UPDATE items SET access=? WHERE key=? AND access<?`, now, key, now-60)
}

// MarkRefs marks every row of the given refs stale and returns how many rows.
func (t *Tracker) MarkRefs(ctx context.Context, refs []string) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	now := t.now().Unix()
	total := 0
	for start := 0; start < len(refs); start += 400 {
		part := refs[start:min(len(refs), start+400)]
		args := make([]any, 0, len(part)+1)
		args = append(args, now)
		for _, r := range part {
			args = append(args, r)
		}
		res, err := t.db.ExecContext(ctx, `UPDATE items SET stale=1, stale_at=CASE WHEN stale=1 THEN stale_at ELSE ? END WHERE ref IN (?`+strings.Repeat(",?", len(part)-1)+`)`, args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	t.Add(ctx, CountMarked, int64(total))
	return total, nil
}

// MarkPrefixStale marks every row whose ref starts with prefix and that was
// verified before the given time stale (recovery after a long outage).
func (t *Tracker) MarkPrefixStale(ctx context.Context, prefix string, before time.Time) (int, error) {
	res, err := t.db.ExecContext(ctx, `UPDATE items SET stale=1, stale_at=CASE WHEN stale=1 THEN stale_at ELSE ? END WHERE ref LIKE ? AND verified<?`, t.now().Unix(), prefix+"%", before.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	t.Add(ctx, CountMarked, n)
	return int(n), nil
}

// Pending lists stale rows asked for since accessedSince, most recent first.
func (t *Tracker) Pending(ctx context.Context, accessedSince time.Time, limit int) ([]Item, error) {
	return t.list(ctx, `SELECT key,query,ref,verified,stale FROM items WHERE stale=1 AND access>=? ORDER BY access DESC LIMIT ?`, accessedSince.Unix(), limit)
}

// Backstop lists rows not verified since before, oldest first, that are
// not already stale.
func (t *Tracker) Backstop(ctx context.Context, before time.Time, limit int) ([]Item, error) {
	return t.list(ctx, `SELECT key,query,ref,verified,stale FROM items WHERE verified<? AND stale=0 ORDER BY verified LIMIT ?`, before.Unix(), limit)
}

// Unrequested lists rows not asked for since before (eviction candidates).
func (t *Tracker) Unrequested(ctx context.Context, before time.Time, limit int) ([]Item, error) {
	return t.list(ctx, `SELECT key,query,ref,verified,stale FROM items WHERE access<? ORDER BY access LIMIT ?`, before.Unix(), limit)
}

// Popular lists rows whose ref starts with prefix, most recently asked first.
func (t *Tracker) Popular(ctx context.Context, prefix string, since time.Time, limit int) ([]Item, error) {
	return t.list(ctx, `SELECT key,query,ref,verified,stale FROM items WHERE ref LIKE ? AND access>=? ORDER BY access DESC LIMIT ?`, prefix+"%", since.Unix(), limit)
}

// MarkKeys marks the given keys stale.
func (t *Tracker) MarkKeys(ctx context.Context, keys []string) {
	now := t.now().Unix()
	for _, k := range keys {
		_, _ = t.db.ExecContext(ctx, `UPDATE items SET stale=1, stale_at=CASE WHEN stale=1 THEN stale_at ELSE ? END WHERE key=?`, now, k)
	}
}

// Forget drops tracked rows.
func (t *Tracker) Forget(ctx context.Context, keys []string) {
	for _, k := range keys {
		_, _ = t.db.ExecContext(ctx, `DELETE FROM items WHERE key=?`, k)
	}
}

func (t *Tracker) list(ctx context.Context, q string, args ...any) ([]Item, error) {
	rows, err := t.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		var v int64
		var s int
		if err := rows.Scan(&it.Key, &it.Query, &it.Ref, &v, &s); err != nil {
			return nil, err
		}
		it.Verified, it.Stale = time.Unix(v, 0), s == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

// Get and Set keep small named values (feed cursors).
func (t *Tracker) Get(ctx context.Context, name string) string {
	var v string
	_ = t.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE name=?`, name).Scan(&v)
	return v
}

func (t *Tracker) Set(ctx context.Context, name, value string) error {
	_, err := t.db.ExecContext(ctx, `INSERT INTO kv(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value`, name, value)
	return err
}

// Add increments a persistent counter.
func (t *Tracker) Add(ctx context.Context, name string, n int64) {
	if n == 0 {
		return
	}
	_, _ = t.db.ExecContext(ctx, `INSERT INTO counters(name,n) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET n=n+excluded.n`, name, n)
}

// Stats are the refresh numbers in /v1/stats.
type Stats struct {
	StoreItems            int64             `json:"store_items"`
	StoreBytes            int64             `json:"store_bytes"`
	Stale                 int64             `json:"stale"`
	ChangesApplied        int64             `json:"changes_applied"`
	RowsMarkedStale       int64             `json:"rows_marked_stale"`
	UpstreamCallsSaved    int64             `json:"upstream_calls_saved"`
	MedianChangeToApplied int64             `json:"median_change_to_applied_seconds"`
	Feeds                 map[string]string `json:"feeds"`
	Disk                  *Disk             `json:"disk,omitempty"`
	Warnings              []string          `json:"warnings"`
}

// Disk is the store volume usage.
type Disk struct {
	UsedBytes  uint64  `json:"used_bytes"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedRatio  float64 `json:"used_ratio"`
}

// Snapshot collects the stats; feeds are the per-feed states.
func (t *Tracker) Snapshot(ctx context.Context) Stats {
	st := Stats{Feeds: map[string]string{}, Warnings: []string{}}
	_ = t.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(stale),0) FROM items`).Scan(&st.StoreItems, &st.Stale)
	rows, err := t.db.QueryContext(ctx, `SELECT name, n FROM counters`)
	if err == nil {
		for rows.Next() {
			var name string
			var n int64
			if rows.Scan(&name, &n) == nil {
				switch name {
				case CountChanges:
					st.ChangesApplied = n
				case CountSaved:
					st.UpstreamCallsSaved = n
				case CountMarked:
					st.RowsMarkedStale = n
				}
			}
		}
		rows.Close()
	}
	var lags []int64
	if rows, err := t.db.QueryContext(ctx, `SELECT seconds FROM lags`); err == nil {
		for rows.Next() {
			var s int64
			if rows.Scan(&s) == nil {
				lags = append(lags, s)
			}
		}
		rows.Close()
	}
	st.MedianChangeToApplied = median(lags)
	if rows, err := t.db.QueryContext(ctx, `SELECT name, value FROM kv WHERE name LIKE 'feed:%'`); err == nil {
		for rows.Next() {
			var n, v string
			if rows.Scan(&n, &v) == nil {
				st.Feeds[strings.TrimPrefix(n, "feed:")] = v
			}
		}
		rows.Close()
	}
	return st
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}
