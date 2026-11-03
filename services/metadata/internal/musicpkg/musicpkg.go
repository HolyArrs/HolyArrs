// Package musicpkg keeps one complete package per covered artist: the
// stored MusicBrainz artist answer, every release group sorted into kinds
// (album, ep, single, compilation, live, soundtrack...), the chosen
// edition's full track list of each picked release group, and the picture
// records. Packages are built on a schedule from what the server already
// stores (never from an outside call) and kept in SQLite with a version
// that moves only when a package's content changes, so app servers can
// pull every changed package with a cursor.
//
// No install is ever recorded: a sync read carries no identity, and an
// ask to cover an artist keeps only its MusicBrainz id and the time.
package musicpkg

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	_ "modernc.org/sqlite"
)

// SchemaVersion is the package shape's version.
const SchemaVersion = 1

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidMBID reports a lower-case MusicBrainz id.
func ValidMBID(id string) bool { return mbidRe.MatchString(id) }

// Sources read the stored parts a package is made of (no outside call).
type Sources struct {
	Artist   func(ctx context.Context, mbid string) ([]byte, bool)
	Discog   func(ctx context.Context, mbid string) (discog.Artist, bool)
	Album    func(ctx context.Context, rg string) (discog.Album, bool)
	Pictures func(ctx context.Context, kind, mbid string) map[string]imgstore.Info
}

// Group is one release group in a package.
type Group struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Kind             string         `json:"kind"`
	PrimaryType      string         `json:"primary_type,omitempty"`
	SecondaryTypes   []string       `json:"secondary_types"`
	FirstReleaseDate string         `json:"first_release_date,omitempty"`
	Disambiguation   string         `json:"disambiguation,omitempty"`
	Cover            *imgstore.Info `json:"cover,omitempty"`
	// Tracklist: "stored" (in Albums), "pending" (picked, not walked
	// yet) or "not_picked" (outside this artist's track list depth).
	Tracklist string `json:"tracklist"`
}

// Tracklist is the chosen edition and its tracks for one release group.
type Tracklist struct {
	Edition      *discog.Edition `json:"edition"`
	Tracks       []discog.Track  `json:"tracks"`
	TrackCount   int             `json:"track_count"`
	DiscCount    int             `json:"disc_count"`
	LengthMS     int64           `json:"length_ms,omitempty"`
	EditionCount int             `json:"edition_count"`
	FetchedAt    time.Time       `json:"fetched_at"`
}

// Package is one artist's complete package.
type Package struct {
	SchemaVersion         int                      `json:"schema_version"`
	MBID                  string                   `json:"mbid"`
	Version               int64                    `json:"version"`
	Hash                  string                   `json:"hash"`
	BuiltAt               time.Time                `json:"built_at"`
	Rank                  int                      `json:"rank"`
	Complete              bool                     `json:"complete"`
	Missing               []string                 `json:"missing"`
	Artist                json.RawMessage          `json:"artist"`
	Pictures              map[string]imgstore.Info `json:"pictures"`
	ReleaseGroupCount     int                      `json:"release_group_count"`
	ReleaseGroupsComplete bool                     `json:"release_groups_complete"`
	Kinds                 map[string]int           `json:"kinds"`
	ReleaseGroups         []Group                  `json:"release_groups"`
	Albums                map[string]Tracklist     `json:"albums"`
}

// Kind sorts a release group: a secondary type wins over the primary one
// (a live album is "live", not "album").
func Kind(primary string, secondary []string) string {
	order := []struct{ sec, kind string }{
		{"compilation", "compilation"}, {"soundtrack", "soundtrack"}, {"live", "live"},
		{"remix", "remix"}, {"dj-mix", "dj-mix"}, {"mixtape/street", "mixtape"},
		{"demo", "demo"}, {"spokenword", "spoken"}, {"interview", "spoken"}, {"audiobook", "spoken"},
		{"audio drama", "spoken"}, {"field recording", "other"},
	}
	for _, o := range order {
		for _, s := range secondary {
			if strings.EqualFold(s, o.sec) {
				return o.kind
			}
		}
	}
	switch p := strings.ToLower(primary); p {
	case "album", "ep", "single", "broadcast":
		return p
	}
	return "other"
}

// kindRank orders kinds for track list picking (lower first).
var kindRank = map[string]int{"album": 0, "ep": 1, "single": 2, "live": 3, "compilation": 4, "soundtrack": 5}

// Depth is how many release groups get a track list, and of which kinds.
type Depth struct {
	Max   int
	Kinds map[string]bool // nil: every kind
}

// Pick answers the release groups whose track lists a package carries:
// albums first, then EPs, singles and the rest, newest first in a kind,
// at most d.Max.
func Pick(groups []discog.Group, d Depth) []string {
	type g struct {
		id, kind, date string
	}
	list := make([]g, 0, len(groups))
	for _, x := range groups {
		k := Kind(x.PrimaryType, x.SecondaryTypes)
		if d.Kinds != nil && !d.Kinds[k] {
			continue
		}
		list = append(list, g{strings.ToLower(x.ID), k, x.FirstReleaseDate})
	}
	rank := func(k string) int {
		if r, ok := kindRank[k]; ok {
			return r
		}
		return 9
	}
	sort.SliceStable(list, func(i, j int) bool {
		if ri, rj := rank(list[i].kind), rank(list[j].kind); ri != rj {
			return ri < rj
		}
		if list[i].date != list[j].date {
			return list[i].date > list[j].date
		}
		return list[i].id < list[j].id
	})
	if d.Max > 0 && len(list) > d.Max {
		list = list[:d.Max]
	}
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.id
	}
	return out
}

// Build makes an artist's package from the stored parts. ok is false when
// not even the artist answer is stored. The Version is left 0 (the store
// sets it).
func Build(ctx context.Context, src Sources, mbid string, rank int, d Depth, now time.Time) (Package, bool) {
	mbid = strings.ToLower(mbid)
	p := Package{SchemaVersion: SchemaVersion, MBID: mbid, Rank: rank, BuiltAt: now.UTC(), Missing: []string{},
		Pictures: map[string]imgstore.Info{}, Kinds: map[string]int{}, ReleaseGroups: []Group{}, Albums: map[string]Tracklist{}}
	body, ok := src.Artist(ctx, mbid)
	if !ok {
		return p, false
	}
	p.Artist = json.RawMessage(body)
	if src.Pictures != nil {
		for k, v := range src.Pictures(ctx, "artist", mbid) {
			p.Pictures[k] = v
		}
	}
	if _, ok := p.Pictures["photo"]; !ok {
		p.Missing = append(p.Missing, "photo")
	}
	a, ok := src.Discog(ctx, mbid)
	if !ok {
		p.Missing = append(p.Missing, "release_groups")
		p.Hash = hash(p)
		return p, true
	}
	p.ReleaseGroupCount, p.ReleaseGroupsComplete = a.Count, a.Complete
	if !a.Complete {
		p.Missing = append(p.Missing, "release_groups_partial")
	}
	picked := map[string]bool{}
	for _, id := range Pick(a.Groups, d) {
		picked[id] = true
	}
	pending := 0
	for _, g := range a.Groups {
		id := strings.ToLower(g.ID)
		k := Kind(g.PrimaryType, g.SecondaryTypes)
		p.Kinds[k]++
		x := Group{ID: id, Title: g.Title, Kind: k, PrimaryType: g.PrimaryType, SecondaryTypes: g.SecondaryTypes,
			FirstReleaseDate: g.FirstReleaseDate, Disambiguation: g.Disambiguation, Tracklist: "not_picked"}
		if x.SecondaryTypes == nil {
			x.SecondaryTypes = []string{}
		}
		if src.Pictures != nil {
			if c, ok := src.Pictures(ctx, "release-group", id)["cover"]; ok {
				x.Cover = &c
			}
		}
		if picked[id] {
			if al, ok := src.Album(ctx, id); ok {
				x.Tracklist = "stored"
				p.Albums[id] = Tracklist{Edition: al.Edition, Tracks: al.Tracks, TrackCount: al.TrackCount, DiscCount: al.DiscCount,
					LengthMS: al.LengthMS, EditionCount: al.EditionCount, FetchedAt: al.FetchedAt.UTC()}
			} else {
				x.Tracklist = "pending"
				pending++
			}
		}
		p.ReleaseGroups = append(p.ReleaseGroups, x)
	}
	if pending > 0 {
		p.Missing = append(p.Missing, "tracklists")
	}
	p.Complete = a.Complete && pending == 0
	p.Hash = hash(p)
	return p, true
}

// hash covers the content only (not the build time, version, rank or
// fetch times).
func hash(p Package) string {
	p.Version, p.Hash, p.BuiltAt, p.Rank = 0, "", time.Time{}, 0
	albums := make(map[string]Tracklist, len(p.Albums))
	for id, t := range p.Albums {
		t.FetchedAt = time.Time{}
		albums[id] = t
	}
	p.Albums = albums
	// A picture fetched again unchanged keeps its version: when it was
	// fetched is not content.
	pics := make(map[string]imgstore.Info, len(p.Pictures))
	for k, i := range p.Pictures {
		i.FetchedAt = ""
		pics[k] = i
	}
	p.Pictures = pics
	groups := make([]Group, len(p.ReleaseGroups))
	for n, g := range p.ReleaseGroups {
		if g.Cover != nil {
			c := *g.Cover
			c.FetchedAt = ""
			g.Cover = &c
		}
		groups[n] = g
	}
	p.ReleaseGroups = groups
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

// Store keeps the packages and the asks for uncovered artists.
type Store struct{ db *sql.DB }

// Open opens (or makes) the store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS packages(mbid TEXT PRIMARY KEY, version INTEGER NOT NULL, hash TEXT NOT NULL, built INTEGER NOT NULL, complete INTEGER NOT NULL, rank INTEGER NOT NULL, body BLOB NOT NULL) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS packages_version ON packages(version);
CREATE TABLE IF NOT EXISTS wanted(mbid TEXT PRIMARY KEY, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS seq(id INTEGER PRIMARY KEY CHECK (id = 1), n INTEGER NOT NULL);
INSERT OR IGNORE INTO seq(id, n) VALUES (1, 0);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// Put stores a package. A package whose hash matches the stored one keeps
// its version (only the build time moves); changed reports a new version.
func (s *Store) Put(ctx context.Context, p Package) (changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var old string
	var ver int64
	err = tx.QueryRowContext(ctx, `SELECT hash, version FROM packages WHERE mbid = ?`, p.MBID).Scan(&old, &ver)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == nil && old == p.Hash {
		if _, err := tx.ExecContext(ctx, `UPDATE packages SET built = ?, rank = ? WHERE mbid = ?`, p.BuiltAt.Unix(), p.Rank, p.MBID); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	if err := tx.QueryRowContext(ctx, `UPDATE seq SET n = n + 1 WHERE id = 1 RETURNING n`).Scan(&ver); err != nil {
		return false, err
	}
	p.Version = ver
	body, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO packages(mbid, version, hash, built, complete, rank, body) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(mbid) DO UPDATE SET version = excluded.version, hash = excluded.hash, built = excluded.built, complete = excluded.complete, rank = excluded.rank, body = excluded.body`,
		p.MBID, ver, p.Hash, p.BuiltAt.Unix(), b2i(p.Complete), p.Rank, body); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wanted WHERE mbid = ?`, p.MBID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Get answers one stored package's JSON.
func (s *Store) Get(ctx context.Context, mbid string) ([]byte, bool) {
	var body []byte
	if s.db.QueryRowContext(ctx, `SELECT body FROM packages WHERE mbid = ?`, strings.ToLower(mbid)).Scan(&body) != nil {
		return nil, false
	}
	return body, true
}

// Meta is a stored package's state.
type Meta struct {
	Built    time.Time
	Complete bool
}

// Meta answers when a package was built and whether it was complete.
func (s *Store) Meta(ctx context.Context, mbid string) (Meta, bool) {
	var built int64
	var complete int
	if s.db.QueryRowContext(ctx, `SELECT built, complete FROM packages WHERE mbid = ?`, mbid).Scan(&built, &complete) != nil {
		return Meta{}, false
	}
	return Meta{Built: time.Unix(built, 0), Complete: complete == 1}, true
}

// IndexEntry is one package's line in an index page.
type IndexEntry struct {
	MBID     string `json:"mbid"`
	Version  int64  `json:"version"`
	Hash     string `json:"hash"`
	Complete bool   `json:"complete"`
}

// Page answers packages changed after the cursor (version > since), oldest
// change first, at most limit and about maxBytes (always at least one).
// next is the cursor for the next page; more says whether one exists.
func (s *Store) Page(ctx context.Context, since int64, limit, maxBytes int) (bodies []json.RawMessage, next int64, more bool, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, body FROM packages WHERE version > ? ORDER BY version LIMIT ?`, since, limit+1)
	if err != nil {
		return nil, since, false, err
	}
	defer rows.Close()
	next, size := since, 0
	for rows.Next() {
		var v int64
		var b []byte
		if err := rows.Scan(&v, &b); err != nil {
			return nil, since, false, err
		}
		if len(bodies) >= limit || (len(bodies) > 0 && size+len(b) > maxBytes) {
			more = true
			break
		}
		bodies = append(bodies, b)
		size += len(b)
		next = v
	}
	return bodies, next, more, rows.Err()
}

// Index answers the index lines changed after since (no bodies).
func (s *Store) Index(ctx context.Context, since int64, limit int) (out []IndexEntry, next int64, more bool, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid, version, hash, complete FROM packages WHERE version > ? ORDER BY version LIMIT ?`, since, limit+1)
	if err != nil {
		return nil, since, false, err
	}
	defer rows.Close()
	next = since
	out = []IndexEntry{}
	for rows.Next() {
		var e IndexEntry
		var c int
		if err := rows.Scan(&e.MBID, &e.Version, &e.Hash, &c); err != nil {
			return nil, since, false, err
		}
		if len(out) >= limit {
			more = true
			break
		}
		e.Complete = c == 1
		out = append(out, e)
		next = e.Version
	}
	return out, next, more, rows.Err()
}

// Floor is the newest stored version at or below since (0 when none): the
// same page as since, so equivalent cursors can share one cached answer.
func (s *Store) Floor(ctx context.Context, since int64) int64 {
	var n int64
	if s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM packages WHERE version <= ?`, since).Scan(&n) != nil {
		return since
	}
	return n
}

// Latest is the newest version (the head cursor).
func (s *Store) Latest(ctx context.Context) int64 {
	var n int64
	_ = s.db.QueryRowContext(ctx, `SELECT n FROM seq WHERE id = 1`).Scan(&n)
	return n
}

// MaxWanted caps the asks kept; the oldest go first.
const MaxWanted = 20000

// WantedFor is how long an ask is kept without a package.
const WantedFor = 30 * 24 * time.Hour

// Want records an ask to cover an artist (the id and time only).
func (s *Store) Want(ctx context.Context, mbid string, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO wanted(mbid, at) VALUES (?, ?) ON CONFLICT(mbid) DO NOTHING`, mbid, now.Unix()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM wanted WHERE mbid IN (SELECT mbid FROM wanted ORDER BY at DESC LIMIT -1 OFFSET ?)`, MaxWanted)
	return err
}

// Wanted answers the kept asks, oldest first, dropping expired ones.
func (s *Store) Wanted(ctx context.Context, now time.Time) []string {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM wanted WHERE at < ?`, now.Add(-WantedFor).Unix())
	rows, err := s.db.QueryContext(ctx, `SELECT mbid FROM wanted ORDER BY at, mbid`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// Drop forgets an ask (the artist does not exist).
func (s *Store) Drop(ctx context.Context, mbid string) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM wanted WHERE mbid = ?`, mbid)
}

// Counts are the store's numbers.
type Counts struct {
	Packages int64 `json:"packages"`
	Complete int64 `json:"complete"`
	Wanted   int64 `json:"wanted"`
	Latest   int64 `json:"latest_version"`
}

// Counts answers the store's numbers.
func (s *Store) Counts(ctx context.Context) Counts {
	var c Counts
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(complete), 0) FROM packages`).Scan(&c.Packages, &c.Complete)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wanted`).Scan(&c.Wanted)
	c.Latest = s.Latest(ctx)
	return c
}
