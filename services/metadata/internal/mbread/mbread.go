// Package mbread answers MusicBrainz lookups from the slim dump copy that
// internal/mbdump builds: root/CURRENT names the dump date in use and
// root/<date>/mb.sqlite is the copy, opened read-only.
//
// Answers have the shape the stored live MusicBrainz detail answers have
// ({"source":"musicbrainz","source_id","kind","title","summary",
// "source_data":{...}}, MusicBrainz's own field names under source_data),
// plus "source_tag" saying they came from the dump (srctag.MBDump) and
// which one. An item the copy lacks answers false, and callers fall back
// to their live MusicBrainz queue.
//
// A refresh swaps CURRENT to a new date. The reader notices within Check,
// opens the new copy beside the old one, sends new reads to it and closes
// the old copy only after its last read is done, so no read fails on a
// swap.
package mbread

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	_ "modernc.org/sqlite"
)

// Caps, so one huge artist or album stays a quick read.
const (
	maxGroups   = discog.DefaultMaxGroups
	maxReleases = discog.DefaultEditionPages * 100
	maxRecRels  = 100
)

// copyDB is one open copy. Reads hold mu shared; closing takes it whole,
// so a copy closes only after its last read.
type copyDB struct {
	date   string
	at     time.Time
	db     *sql.DB
	iso    bool                 // has iso_3166_1 (country codes)
	genres map[string][2]string // genre name: gid, comment
	mu     sync.RWMutex
	closed bool
}

// Reader reads the copy CURRENT points at.
type Reader struct {
	Root string
	// Check is how often CURRENT is read again (default 5 s).
	Check time.Duration

	cur     atomic.Pointer[copyDB]
	checkMu sync.Mutex
	checked atomic.Int64 // unix nanos of the last pointer read
	swaps   atomic.Int64
	retired sync.WaitGroup // old copies still closing
}

// Open makes a reader over root. A root with no copy yet is fine: every
// lookup answers false until CURRENT appears.
func Open(root string) (*Reader, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("mbread: empty root")
	}
	r := &Reader{Root: root, Check: 5 * time.Second}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Date answers the dump date in use ("" when none).
func (r *Reader) Date() string {
	if c := r.cur.Load(); c != nil {
		return c.date
	}
	return ""
}

// Swaps counts copies swapped in after the first.
func (r *Reader) Swaps() int64 { return r.swaps.Load() }

// Close closes the copy in use, after its reads and those of swapped-out
// copies are done.
func (r *Reader) Close() error {
	if c := r.cur.Swap(nil); c != nil {
		retire(c)
	}
	r.retired.Wait()
	return nil
}

func retire(c *copyDB) {
	c.mu.Lock()
	c.closed = true
	_ = c.db.Close()
	c.mu.Unlock()
}

// reload reads CURRENT and swaps in its copy when the date changed.
func (r *Reader) reload() error {
	r.checked.Store(time.Now().UnixNano())
	dir, err := mbdump.Current(r.Root)
	if err != nil || dir == "" {
		return err
	}
	date := filepath.Base(dir)
	if c := r.cur.Load(); c != nil && c.date == date {
		return nil
	}
	path := filepath.Join(dir, mbdump.DBName)
	if _, err := os.Stat(path); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(2 * runtime.NumCPU())
	db.SetMaxIdleConns(2 * runtime.NumCPU())
	var one int
	if err := db.QueryRow(`SELECT 1 FROM artist LIMIT 1`).Scan(&one); err != nil && !errors.Is(err, sql.ErrNoRows) {
		db.Close()
		return err
	}
	var iso int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='iso_3166_1'`).Scan(&iso)
	at, _ := time.Parse("20060102-150405", date)
	c := &copyDB{date: date, at: at.UTC(), db: db, iso: iso > 0, genres: map[string][2]string{}}
	if rows, err := db.Query(`SELECT name, gid, comment FROM genre`); err == nil {
		for rows.Next() {
			var n, g, cm sql.NullString
			if rows.Scan(&n, &g, &cm) == nil {
				c.genres[n.String] = [2]string{g.String, cm.String}
			}
		}
		rows.Close()
	}
	if old := r.cur.Swap(c); old != nil {
		r.swaps.Add(1)
		r.retired.Add(1)
		go func() {
			defer r.retired.Done()
			retire(old)
		}()
	}
	return nil
}

// acquire answers the copy in use, held for reading (release with
// c.mu.RUnlock), or nil when there is none.
func (r *Reader) acquire() *copyDB {
	check := r.Check
	if check <= 0 {
		check = 5 * time.Second
	}
	if time.Since(time.Unix(0, r.checked.Load())) >= check && r.checkMu.TryLock() {
		_ = r.reload() // a failed reload keeps the copy in use
		r.checkMu.Unlock()
	}
	for {
		c := r.cur.Load()
		if c == nil {
			return nil
		}
		c.mu.RLock()
		if !c.closed {
			return c
		}
		c.mu.RUnlock() // swapped out meanwhile: take the new one
	}
}

// Has reports whether the copy holds the item (kind artist,
// release-group, release or recording) without building its answer.
func (r *Reader) Has(ctx context.Context, kind, mbid string) bool {
	c := r.acquire()
	if c == nil {
		return false
	}
	defer c.mu.RUnlock()
	table, ok := map[string]string{"artist": "artist", "release-group": "release_group", "release": "release", "recording": "recording"}[kind]
	if !ok {
		return false
	}
	var id int64
	return c.db.QueryRowContext(ctx, `SELECT id FROM "`+table+`" WHERE gid=?`, strings.ToLower(mbid)).Scan(&id) == nil
}

// Tag is the source tag of the copy's answers.
func (c *copyDB) tag() srctag.Tag {
	return srctag.Tag{Source: srctag.MBDump, Ref: c.date, FetchedAt: c.at}
}

func tagJSON(t srctag.Tag) map[string]any {
	m := map[string]any{"source": t.Source, "ref": t.Ref}
	if !t.FetchedAt.IsZero() {
		m["fetched_at"] = t.FetchedAt.Format(time.RFC3339)
	}
	return m
}

// Stored answers the item's detail answer (kind artist, release-group,
// release or recording) in the stored live answer's shape, and the dump
// date. False when the copy lacks it or there is no copy.
func (r *Reader) Stored(ctx context.Context, kind, mbid string) ([]byte, string, bool) {
	c := r.acquire()
	if c == nil {
		return nil, "", false
	}
	defer c.mu.RUnlock()
	q := &query{ctx: ctx, c: c, credits: map[int64][]any{}, areas: map[int64]map[string]any{}}
	mbid = strings.ToLower(mbid)
	var src map[string]any
	var title, summary any
	var err error
	switch kind {
	case "artist":
		src, err = q.artist(mbid)
		if src != nil {
			title, summary = src["name"], src["disambiguation"]
		}
	case "release-group":
		src, err = q.releaseGroup(mbid)
	case "release":
		src, err = q.release(mbid, true)
	case "recording":
		src, err = q.recording(mbid)
	default:
		return nil, "", false
	}
	if err != nil || src == nil {
		return nil, "", false
	}
	if kind != "artist" {
		title, summary = src["title"], src["disambiguation"]
	}
	b, err := json.Marshal(map[string]any{"source": "musicbrainz", "source_id": mbid, "kind": kind, "title": title, "summary": summary,
		"source_data": src, "source_tag": tagJSON(c.tag())})
	if err != nil {
		return nil, "", false
	}
	return b, c.date, true
}

// StoredData is Stored without the date (the api.Server.StoredData shape).
func (r *Reader) StoredData(ctx context.Context, kind, mbid string) ([]byte, bool) {
	b, _, ok := r.Stored(ctx, kind, mbid)
	return b, ok
}

// Artist answers an artist's full release-group list (discog.Slim).
func (r *Reader) Artist(ctx context.Context, mbid string) (discog.Artist, bool) {
	c := r.acquire()
	if c == nil {
		return discog.Artist{}, false
	}
	defer c.mu.RUnlock()
	q := &query{ctx: ctx, c: c, credits: map[int64][]any{}, areas: map[int64]map[string]any{}}
	id, ok := q.id("artist", strings.ToLower(mbid))
	if !ok {
		return discog.Artist{}, false
	}
	raw, err := q.groups(id)
	if err != nil {
		return discog.Artist{}, false
	}
	b, _ := json.Marshal(raw)
	var groups []discog.Group
	if json.Unmarshal(b, &groups) != nil {
		return discog.Artist{}, false
	}
	now := time.Now()
	return discog.Artist{MBID: strings.ToLower(mbid), Groups: groups, Count: len(groups), Complete: len(groups) < maxGroups,
		Walked: now, Checked: now}, true
}

// Album answers an album's chosen edition with tracks and its editions
// (discog.Slim), chosen the way a walk chooses.
func (r *Reader) Album(ctx context.Context, mbid string) (discog.Album, bool) {
	c := r.acquire()
	if c == nil {
		return discog.Album{}, false
	}
	defer c.mu.RUnlock()
	q := &query{ctx: ctx, c: c, credits: map[int64][]any{}, areas: map[int64]map[string]any{}}
	mbid = strings.ToLower(mbid)
	gid, ok := q.id("release_group", mbid)
	if !ok {
		return discog.Album{}, false
	}
	rels, count, err := q.groupReleases(gid, true)
	if err != nil {
		return discog.Album{}, false
	}
	b, _ := json.Marshal(rels)
	home := ""
	if len(rels) > 0 {
		if cr, _ := rels[0]["artist-credit"].([]any); len(cr) > 0 {
			if a, ok := cr[0].(map[string]any)["artist"].(map[string]any); ok {
				home, _ = a["country"].(string)
			}
		}
	}
	a, ok := discog.FromReleases(mbid, b, count, home, func(id string) ([]byte, bool) {
		rel, err := q.release(id, true)
		if err != nil || rel == nil {
			return nil, false
		}
		out, err := json.Marshal(rel)
		return out, err == nil
	}, c.at)
	return a, ok
}

// GroupTitles answers the titles of an artist's release groups (nil when
// the artist is not in the copy); the Deezer matcher checks artist links
// with them (imgstore Resolver.MBTitles).
func (r *Reader) GroupTitles(ctx context.Context, mbid string) []string {
	a, ok := r.Artist(ctx, mbid)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a.Groups))
	for _, g := range a.Groups {
		out = append(out, g.Title)
	}
	return out
}

// DumpRows answers the dump date of the copy in use and the row count of
// each table (dump_meta); empty when there is no copy.
func (r *Reader) DumpRows(ctx context.Context) (string, map[string]int64) {
	c := r.acquire()
	if c == nil {
		return "", nil
	}
	defer c.mu.RUnlock()
	out := map[string]int64{}
	rows, err := c.db.QueryContext(ctx, `SELECT tbl, rows FROM dump_meta`)
	if err != nil {
		return c.date, out
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int64
		if rows.Scan(&t, &n) == nil {
			out[t] = n
		}
	}
	return c.date, out
}
