// Package discog keeps every artist's full list of release groups and
// each album's chosen edition with its track list, so an artist or album
// page answers from storage in one call and never waits on a MusicBrainz
// walk.
//
// MusicBrainz caps an artist lookup at 25 release groups and a browse
// page at 100, so the full list is a walk of browse pages. Each walk runs
// as a fetch-queue job one MusicBrainz call a step (fetchq.ErrMore between
// steps), inside the queue's rate and priorities: a page someone waits on
// goes first, and one job key per artist or album means one walk at a
// time. A walk is made again only when the replication feed marks the
// artist, one of its release groups or the album's edition changed, or
// after a slow safety time; a cheap first-page check covers new albums
// in between.
package discog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

// Defaults.
const (
	// DefaultMaxGroups caps one artist's stored list (20 browse pages);
	// a longer list is stored as far as this and marked not complete.
	DefaultMaxGroups = 2000
	// DefaultEditionPages caps an album's edition browse (100 a page).
	DefaultEditionPages = 3
	// DefaultRewalk is the safety refresh: a full walk again after this.
	DefaultRewalk = 30 * 24 * time.Hour
	// DefaultCheck is how often a stored artist's first page is read to
	// see whether a release group was added (one call).
	DefaultCheck = 7 * 24 * time.Hour
	browsePage   = 100
)

// Options configure the store.
type Options struct {
	// Get makes one paced MusicBrainz call (providers.Client.MusicBrainzGet).
	Get func(ctx context.Context, path string, v url.Values, out any) error
	// Take spends one call of the MusicBrainz budget; false when spent
	// (the step waits and is tried again). nil: no budget.
	Take func() bool
	// Stored reads a stored MusicBrainz detail answer (kind "artist"); a
	// lean artist answer that already holds every release group is taken
	// over without a call, and the artist's country picks an edition.
	Stored func(ctx context.Context, kind, mbid string) ([]byte, bool)
	// Slim answers artists and albums from the slim MusicBrainz dump copy
	// (internal/mbread). When it has one, that answer is served and no walk
	// runs; the queue walks live MusicBrainz only for what it lacks. nil:
	// walks only.
	Slim         Slim
	MaxGroups    int
	EditionPages int
	Rewalk       time.Duration
	Check        time.Duration
	// Pause is how long background walks (P2 and below) stop after
	// PauseAfter network failures or 503s in a row (default 15 min after
	// 3): musicbrainz.org shares one address with the Cover Art Archive
	// and ListenBrainz, and a throttled address must not be pressed.
	// Walks someone waits on (P0, P1) still run, at the shared pace.
	Pause      time.Duration
	PauseAfter int
	Now        func() time.Time
	Logf       func(string, ...any)
}

// Slim answers an artist's list or an album from a local MusicBrainz
// copy (false when the copy lacks it).
type Slim interface {
	Artist(ctx context.Context, mbid string) (Artist, bool)
	Album(ctx context.Context, mbid string) (Album, bool)
}

// Group is one release group of an artist's list, in MusicBrainz's own
// field names (the shape of the artist answer's source_data
// "release-groups").
type Group struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	PrimaryType      string   `json:"primary-type"`
	SecondaryTypes   []string `json:"secondary-types"`
	FirstReleaseDate string   `json:"first-release-date"`
	Disambiguation   string   `json:"disambiguation"`
}

// Artist is an artist's stored list.
type Artist struct {
	MBID   string
	Groups []Group
	// Count is MusicBrainz's release-group count; Complete is true when
	// every one of them is in Groups.
	Count    int
	Complete bool
	Walked   time.Time
	Checked  time.Time
	Stale    bool
}

// Edition is one release (edition) of an album, without its tracks.
type Edition struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status,omitempty"`
	Date           string   `json:"date,omitempty"`
	Country        string   `json:"country,omitempty"`
	Formats        []string `json:"formats"`
	TrackCount     int      `json:"track_count"`
	DiscCount      int      `json:"disc_count"`
	Barcode        string   `json:"barcode,omitempty"`
	Disambiguation string   `json:"disambiguation,omitempty"`
	Packaging      string   `json:"packaging,omitempty"`
	// Countries are the release's country and release-event countries
	// (XW: worldwide).
	Countries []string `json:"countries,omitempty"`
}

// Track is one track of the chosen edition. Explicit is left out:
// MusicBrainz does not say.
type Track struct {
	Disc         int    `json:"disc"`
	Position     int    `json:"position"`
	Number       string `json:"number"`
	Title        string `json:"title"`
	LengthMS     int    `json:"length_ms,omitempty"`
	Recording    string `json:"recording"`
	ArtistCredit string `json:"artist_credit,omitempty"`
	Explicit     *bool  `json:"explicit,omitempty"`
}

// Credit is one credited artist.
type Credit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Album is an album's stored answer: the release group, its chosen
// edition with tracks, and every edition (up to the browse cap) without.
type Album struct {
	MBID             string    `json:"mbid"`
	Title            string    `json:"title"`
	PrimaryType      string    `json:"primary_type,omitempty"`
	SecondaryTypes   []string  `json:"secondary_types"`
	FirstReleaseDate string    `json:"first_release_date,omitempty"`
	ArtistCredit     string    `json:"artist_credit,omitempty"`
	Artists          []Credit  `json:"artists"`
	Edition          *Edition  `json:"edition"`
	Tracks           []Track   `json:"tracks"`
	TrackCount       int       `json:"track_count"`
	DiscCount        int       `json:"disc_count"`
	LengthMS         int64     `json:"length_ms,omitempty"`
	Editions         []Edition `json:"editions"`
	EditionCount     int       `json:"edition_count"`
	EditionsComplete bool      `json:"editions_complete"`
	FetchedAt        time.Time `json:"fetched_at"`
	stale            bool
}

// Stats are the walk numbers for /v1/stats.
type Stats struct {
	Artists        int64  `json:"artists"`
	ArtistsFull    int64  `json:"artists_complete"`
	Albums         int64  `json:"albums"`
	Stale          int64  `json:"stale"`
	Calls          int64  `json:"calls"`
	Adopted        int64  `json:"adopted"`
	Checked        int64  `json:"checked_unchanged"`
	ArtistWalks    int64  `json:"artist_walks"`
	AlbumWalks     int64  `json:"album_walks"`
	BudgetWaits    int64  `json:"budget_waits"`
	MarkedFromFeed int64  `json:"marked_from_feed"`
	Pauses         int64  `json:"pauses"`
	PausedUntil    string `json:"paused_until,omitempty"`
}

// Store is the discography store in its own SQLite file.
type Store struct {
	db *sql.DB
	o  Options
	mu sync.Mutex
	st Stats
	// fails counts network failures and 503s in a row; pausedUntil stops
	// background walks.
	fails       int
	pausedUntil time.Time
}

// Paused reports whether background walks are stopped (and until when).
func (s *Store) Paused() (bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.o.Now().Before(s.pausedUntil), s.pausedUntil
}

// note counts a call's outcome: a network failure or 503/429 in a row
// PauseAfter times stops background walks for Pause.
func (s *Store) note(err error) {
	var fe *model.FetchError
	bad := errors.As(err, &fe) && (fe.Code == "upstream_unavailable" || fe.Status == 503 || fe.Status == 429)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !bad {
		s.fails = 0
		return
	}
	s.fails++
	if s.fails >= s.o.PauseAfter && !s.o.Now().Before(s.pausedUntil) {
		s.pausedUntil = s.o.Now().Add(s.o.Pause)
		s.st.Pauses++
		s.o.Logf("discog: musicbrainz failed %d times in a row (%s); background walks stop until %s", s.fails, fe.Code, s.pausedUntil.UTC().Format(time.RFC3339))
	}
}

// Open opens (or creates) the store at path.
func Open(path string, o Options) (*Store, error) {
	if o.MaxGroups <= 0 {
		o.MaxGroups = DefaultMaxGroups
	}
	if o.EditionPages <= 0 {
		o.EditionPages = DefaultEditionPages
	}
	if o.Rewalk <= 0 {
		o.Rewalk = DefaultRewalk
	}
	if o.Check <= 0 {
		o.Check = DefaultCheck
	}
	if o.Pause <= 0 {
		o.Pause = 15 * time.Minute
	}
	if o.PauseAfter <= 0 {
		o.PauseAfter = 3
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS artists(mbid TEXT PRIMARY KEY, body BLOB NOT NULL DEFAULT '', count INTEGER NOT NULL DEFAULT 0, complete INTEGER NOT NULL DEFAULT 0, walked INTEGER NOT NULL DEFAULT 0, checked INTEGER NOT NULL DEFAULT 0, stale INTEGER NOT NULL DEFAULT 0, walk BLOB NOT NULL DEFAULT '') WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS albums(mbid TEXT PRIMARY KEY, body BLOB NOT NULL DEFAULT '', walked INTEGER NOT NULL DEFAULT 0, stale INTEGER NOT NULL DEFAULT 0, walk BLOB NOT NULL DEFAULT '') WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS refs(ref TEXT NOT NULL, owner TEXT NOT NULL, PRIMARY KEY(ref, owner)) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS refs_owner ON refs(owner);
CREATE TABLE IF NOT EXISTS counters(name TEXT PRIMARY KEY, n INTEGER NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	for _, t := range []string{"artists", "albums"} {
		if err := srctag.Stamp(db, srctag.Spec{Table: t, Key: []string{"mbid"}, Body: "body", Src: "'musicbrainz'", Back: "'" + srctag.Untagged + "'", Known: "walked"}); err != nil {
			db.Close()
			return nil, err
		}
	}
	// Rows stored before tagging with no known source are walked again, so
	// they take a real source from the walk.
	for _, t := range []string{"artists", "albums"} {
		_, _ = db.Exec(`UPDATE ` + t + ` SET stale=1 WHERE src='` + srctag.Untagged + `' AND walked>0 AND stale=0`)
	}
	s := &Store{db: db, o: o}
	rows, err := db.Query(`SELECT name, n FROM counters`)
	if err == nil {
		for rows.Next() {
			var name string
			var n int64
			if rows.Scan(&name, &n) == nil {
				*s.counter(name) = n
			}
		}
		rows.Close()
	}
	return s, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) counter(name string) *int64 {
	switch name {
	case "calls":
		return &s.st.Calls
	case "adopted":
		return &s.st.Adopted
	case "checked":
		return &s.st.Checked
	case "artist_walks":
		return &s.st.ArtistWalks
	case "album_walks":
		return &s.st.AlbumWalks
	case "budget_waits":
		return &s.st.BudgetWaits
	case "marked":
		return &s.st.MarkedFromFeed
	}
	var none int64
	return &none
}

func (s *Store) add(name string, n int64) {
	s.mu.Lock()
	*s.counter(name) += n
	s.mu.Unlock()
	_, _ = s.db.Exec(`INSERT INTO counters(name,n) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET n=n+excluded.n`, name, n)
}

// Job keys on the MusicBrainz fetch queue.
const (
	ArtistPrefix = "disc:artist:"
	AlbumPrefix  = "disc:album:"
)

// ArtistKey and AlbumKey are the queue keys of an artist's and an
// album's walk.
func ArtistKey(mbid string) string { return ArtistPrefix + strings.ToLower(mbid) }
func AlbumKey(mbid string) string  { return AlbumPrefix + strings.ToLower(mbid) }

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidMBID reports a lowercase MusicBrainz id.
func ValidMBID(id string) bool { return mbidRe.MatchString(id) }

// Artist answers an artist's stored list (false when none is stored).
func (s *Store) Artist(ctx context.Context, mbid string) (Artist, bool) {
	if s.o.Slim != nil {
		if a, ok := s.o.Slim.Artist(ctx, mbid); ok {
			return a, true
		}
	}
	return s.storedArtist(ctx, mbid)
}

func (s *Store) storedArtist(ctx context.Context, mbid string) (Artist, bool) {
	a := Artist{MBID: mbid}
	var body []byte
	var walked, checked int64
	var complete, stale int
	if s.db.QueryRowContext(ctx, `SELECT body, count, complete, walked, checked, stale FROM artists WHERE mbid=? AND walked>0`, mbid).Scan(&body, &a.Count, &complete, &walked, &checked, &stale) != nil {
		return Artist{}, false
	}
	if json.Unmarshal(body, &a.Groups) != nil {
		return Artist{}, false
	}
	a.Complete, a.Stale = complete == 1, stale == 1
	a.Walked, a.Checked = time.Unix(walked, 0), time.Unix(checked, 0)
	return a, true
}

// Album answers an album's stored answer (false when none is stored).
func (s *Store) Album(ctx context.Context, mbid string) (Album, bool) {
	if s.o.Slim != nil {
		if a, ok := s.o.Slim.Album(ctx, mbid); ok {
			return a, true
		}
	}
	return s.storedAlbum(ctx, mbid)
}

func (s *Store) storedAlbum(ctx context.Context, mbid string) (Album, bool) {
	var body []byte
	var stale int
	if s.db.QueryRowContext(ctx, `SELECT body, stale FROM albums WHERE mbid=? AND walked>0`, mbid).Scan(&body, &stale) != nil {
		return Album{}, false
	}
	var a Album
	if json.Unmarshal(body, &a) != nil {
		return Album{}, false
	}
	a.stale = stale == 1
	return a, true
}

// ArtistDue reports a stored list the server should walk or check again:
// marked changed, past the safety refresh, or its first page unchecked
// for Check.
func (s *Store) ArtistDue(a Artist) bool {
	now := s.o.Now()
	return a.Stale || now.Sub(a.Walked) >= s.o.Rewalk || now.Sub(a.Checked) >= s.o.Check
}

// AlbumDue reports a stored album marked changed or past the safety refresh.
func (s *Store) AlbumDue(a Album) bool {
	return a.stale || s.o.Now().Sub(a.FetchedAt) >= s.o.Rewalk
}

// Run is the queue worker for "disc:artist:" and "disc:album:" jobs: one
// MusicBrainz call a run, fetchq.ErrMore while the walk goes on, nil when
// the answer is stored (or was fresh already).
func (s *Store) Run(ctx context.Context, key string, p int) error {
	if paused, _ := s.Paused(); paused && p >= fetchq.P2Shared {
		// Paused after connection failures: wait without counting.
		return fetchq.ErrConnection
	}
	switch {
	case strings.HasPrefix(key, ArtistPrefix):
		id := strings.TrimPrefix(key, ArtistPrefix)
		if !ValidMBID(id) {
			return fetchq.ErrNotFound
		}
		if s.o.Slim != nil {
			if _, ok := s.o.Slim.Artist(ctx, id); ok {
				return nil // the slim copy answers it: no walk
			}
		}
		return s.stepArtist(ctx, id)
	case strings.HasPrefix(key, AlbumPrefix):
		id := strings.TrimPrefix(key, AlbumPrefix)
		if !ValidMBID(id) {
			return fetchq.ErrNotFound
		}
		if s.o.Slim != nil {
			if _, ok := s.o.Slim.Album(ctx, id); ok {
				return nil
			}
		}
		return s.stepAlbum(ctx, id)
	}
	return fetchq.ErrNotFound
}

// call makes one budgeted MusicBrainz call and maps its failure to the
// queue's errors.
func (s *Store) call(ctx context.Context, path string, v url.Values, out any) error {
	if s.o.Take != nil && !s.o.Take() {
		s.add("budget_waits", 1)
		return fetchq.ErrThrottled // the hour's budget is spent: slow down, uncounted
	}
	s.add("calls", 1)
	err := s.o.Get(ctx, path, v, out)
	s.note(err)
	var fe *model.FetchError
	if errors.As(err, &fe) {
		switch {
		case fe.Status == 404 || fe.Status == 400:
			return fetchq.ErrNotFound
		case fe.Status == 503 || fe.Status == 429:
			return fetchq.ErrThrottled
		}
	}
	return err
}

// artistWalk is a walk in progress.
type artistWalk struct {
	Offset int     `json:"offset"`
	Count  int     `json:"count"`
	Groups []Group `json:"groups"`
}

type groupPage struct {
	Count  int     `json:"release-group-count"`
	Offset int     `json:"release-group-offset"`
	Groups []Group `json:"release-groups"`
}

func (s *Store) stepArtist(ctx context.Context, mbid string) error {
	var walkRaw []byte
	_ = s.db.QueryRowContext(ctx, `SELECT walk FROM artists WHERE mbid=?`, mbid).Scan(&walkRaw)
	var w artistWalk
	walking := len(walkRaw) > 0 && json.Unmarshal(walkRaw, &w) == nil
	old, stored := s.storedArtist(ctx, mbid)
	if !walking {
		if stored && !s.ArtistDue(old) {
			return nil
		}
		if !stored && s.adopt(ctx, mbid) {
			return nil
		}
		w = artistWalk{}
	}
	var page groupPage
	if err := s.call(ctx, "/release-group", url.Values{"artist": {mbid}, "limit": {strconv.Itoa(browsePage)}, "offset": {strconv.Itoa(w.Offset)}}, &page); err != nil {
		return err
	}
	for i := range page.Groups {
		page.Groups[i] = clean(page.Groups[i])
	}
	if w.Offset == 0 && stored && !old.Stale && s.o.Now().Sub(old.Walked) < s.o.Rewalk && page.Count == old.Count && subset(page.Groups, old.Groups) {
		// The first page shows nothing new: the stored list stands.
		_, err := s.db.ExecContext(ctx, `UPDATE artists SET checked=?, walk='' WHERE mbid=?`, s.o.Now().Unix(), mbid)
		s.add("checked", 1)
		return err
	}
	w.Groups = append(w.Groups, page.Groups...)
	w.Offset += len(page.Groups)
	w.Count = page.Count
	if len(page.Groups) > 0 && w.Offset < page.Count && w.Offset < s.o.MaxGroups {
		b, _ := json.Marshal(w)
		_, err := s.db.ExecContext(ctx, `INSERT INTO artists(mbid, walk) VALUES(?,?) ON CONFLICT(mbid) DO UPDATE SET walk=excluded.walk`, mbid, b)
		if err != nil {
			return err
		}
		return fetchq.ErrMore
	}
	s.add("artist_walks", 1)
	return s.putArtist(ctx, mbid, w.Groups, w.Count)
}

// adopt takes over a stored lean artist answer that already holds every
// release group (one browse of up to 100 made with the artist), so most
// artists cost no call here.
func (s *Store) adopt(ctx context.Context, mbid string) bool {
	if s.o.Stored == nil {
		return false
	}
	b, ok := s.o.Stored(ctx, "artist", mbid)
	if !ok {
		return false
	}
	var d struct {
		Source struct {
			Groups []Group `json:"release-groups"`
			Count  *int    `json:"release-group-count"`
		} `json:"source_data"`
	}
	if json.Unmarshal(b, &d) != nil || d.Source.Count == nil || len(d.Source.Groups) < *d.Source.Count {
		return false
	}
	for i := range d.Source.Groups {
		d.Source.Groups[i] = clean(d.Source.Groups[i])
	}
	if s.putArtist(ctx, mbid, d.Source.Groups, *d.Source.Count) != nil {
		return false
	}
	s.add("adopted", 1)
	return true
}

func clean(g Group) Group {
	g.ID = strings.ToLower(g.ID)
	if g.SecondaryTypes == nil {
		g.SecondaryTypes = []string{}
	}
	return g
}

func subset(page, all []Group) bool {
	known := make(map[string]bool, len(all))
	for _, g := range all {
		known[g.ID] = true
	}
	for _, g := range page {
		if !known[g.ID] {
			return false
		}
	}
	return true
}

// putArtist stores a finished list (no repeats, oldest first) and the
// refs the change feed marks it by: the artist and each release group.
func (s *Store) putArtist(ctx context.Context, mbid string, groups []Group, count int) error {
	seen := map[string]bool{}
	list := make([]Group, 0, len(groups))
	for _, g := range groups {
		if ValidMBID(g.ID) && !seen[g.ID] {
			seen[g.ID] = true
			list = append(list, g)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.FirstReleaseDate != b.FirstReleaseDate {
			if a.FirstReleaseDate == "" || b.FirstReleaseDate == "" {
				return b.FirstReleaseDate == ""
			}
			return a.FirstReleaseDate < b.FirstReleaseDate
		}
		return a.Title < b.Title
	})
	body, err := json.Marshal(list)
	if err != nil {
		return err
	}
	complete := 0
	if len(list) >= count {
		complete = 1
	}
	now := s.o.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO artists(mbid, body, count, complete, walked, checked, stale, walk) VALUES(?,?,?,?,?,?,0,'')
ON CONFLICT(mbid) DO UPDATE SET body=excluded.body, count=excluded.count, complete=excluded.complete, walked=excluded.walked, checked=excluded.checked, stale=0, walk=''`, mbid, body, count, complete, now, now); err != nil {
		return err
	}
	owner := "a:" + mbid
	if err := setRefs(ctx, tx, owner, append([]string{mbid}, keysOf(list)...)); err != nil {
		return err
	}
	return tx.Commit()
}

func keysOf(list []Group) []string {
	out := make([]string, len(list))
	for i, g := range list {
		out[i] = g.ID
	}
	return out
}

func setRefs(ctx context.Context, tx *sql.Tx, owner string, refs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM refs WHERE owner=?`, owner); err != nil {
		return err
	}
	for _, r := range refs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO refs(ref, owner) VALUES(?,?)`, r, owner); err != nil {
			return err
		}
	}
	return nil
}

// MarkChanged marks the stored answers the changed MusicBrainz ids touch
// (the replication feed's ids): an artist's list when the artist or one
// of its release groups changed, an album when its release group or its
// chosen edition did. It answers how many it marked.
func (s *Store) MarkChanged(ctx context.Context, ids []string) int {
	total := 0
	for start := 0; start < len(ids); start += 400 {
		part := ids[start:min(len(ids), start+400)]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = strings.ToLower(id)
		}
		in := "(?" + strings.Repeat(",?", len(part)-1) + ")"
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT owner FROM refs WHERE ref IN `+in, args...)
		if err != nil {
			continue
		}
		var owners []string
		for rows.Next() {
			var o string
			if rows.Scan(&o) == nil {
				owners = append(owners, o)
			}
		}
		rows.Close()
		for _, o := range owners {
			table, id := "artists", strings.TrimPrefix(o, "a:")
			if strings.HasPrefix(o, "g:") {
				table, id = "albums", strings.TrimPrefix(o, "g:")
			}
			res, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET stale=1 WHERE mbid=? AND stale=0`, id)
			if err == nil {
				n, _ := res.RowsAffected()
				total += int(n)
			}
		}
	}
	if total > 0 {
		s.add("marked", int64(total))
	}
	return total
}

// albumWalk is an album walk in progress: editions read so far, then the
// chosen edition whose tracks are read last.
type albumWalk struct {
	Offset   int       `json:"offset"`
	Count    int       `json:"count"`
	Pages    int       `json:"pages"`
	Editions []Edition `json:"editions"`
	Album    Album     `json:"album"`
	Chosen   string    `json:"chosen"`
}

type mbCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

type mbRelease struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Date           string `json:"date"`
	Country        string `json:"country"`
	Barcode        string `json:"barcode"`
	Disambiguation string `json:"disambiguation"`
	Packaging      string `json:"packaging"`
	Events         []struct {
		Area struct {
			Codes []string `json:"iso-3166-1-codes"`
		} `json:"area"`
	} `json:"release-events"`
	Media []struct {
		Position int    `json:"position"`
		Format   string `json:"format"`
		Count    int    `json:"track-count"`
		Tracks   []struct {
			Position  int        `json:"position"`
			Number    string     `json:"number"`
			Title     string     `json:"title"`
			Length    *int       `json:"length"`
			Credit    []mbCredit `json:"artist-credit"`
			Recording struct {
				ID     string     `json:"id"`
				Length *int       `json:"length"`
				Credit []mbCredit `json:"artist-credit"`
			} `json:"recording"`
		} `json:"tracks"`
	} `json:"media"`
	Credit []mbCredit `json:"artist-credit"`
	Group  *struct {
		Title          string   `json:"title"`
		PrimaryType    string   `json:"primary-type"`
		SecondaryTypes []string `json:"secondary-types"`
		Date           string   `json:"first-release-date"`
	} `json:"release-group"`
}

func creditText(c []mbCredit) string {
	var b strings.Builder
	for _, x := range c {
		b.WriteString(x.Name)
		b.WriteString(x.JoinPhrase)
	}
	return b.String()
}

func edition(r mbRelease) Edition {
	e := Edition{ID: strings.ToLower(r.ID), Title: r.Title, Status: r.Status, Date: r.Date, Country: r.Country, Formats: []string{}, Barcode: strings.TrimSpace(r.Barcode), Disambiguation: r.Disambiguation, Packaging: r.Packaging, DiscCount: len(r.Media)}
	for _, m := range r.Media {
		e.TrackCount += m.Count
		f := m.Format
		if f == "" {
			f = "Unknown"
		}
		e.Formats = append(e.Formats, f)
	}
	seen := map[string]bool{}
	for _, c := range append([]string{r.Country}, eventCodes(r)...) {
		if c != "" && !seen[c] {
			seen[c] = true
			e.Countries = append(e.Countries, c)
		}
	}
	return e
}

func eventCodes(r mbRelease) []string {
	var out []string
	for _, ev := range r.Events {
		out = append(out, ev.Area.Codes...)
	}
	return out
}

func (s *Store) stepAlbum(ctx context.Context, mbid string) error {
	var walkRaw []byte
	_ = s.db.QueryRowContext(ctx, `SELECT walk FROM albums WHERE mbid=?`, mbid).Scan(&walkRaw)
	var w albumWalk
	walking := len(walkRaw) > 0 && json.Unmarshal(walkRaw, &w) == nil
	if !walking {
		if old, ok := s.storedAlbum(ctx, mbid); ok && !s.AlbumDue(old) {
			return nil
		}
		w = albumWalk{Album: Album{MBID: mbid, SecondaryTypes: []string{}, Artists: []Credit{}}}
	}
	if w.Chosen == "" {
		var page struct {
			Count    int         `json:"release-count"`
			Releases []mbRelease `json:"releases"`
		}
		if err := s.call(ctx, "/release", url.Values{"release-group": {mbid}, "inc": {"media+artist-credits+release-groups"}, "limit": {strconv.Itoa(browsePage)}, "offset": {strconv.Itoa(w.Offset)}}, &page); err != nil {
			return err
		}
		for _, r := range page.Releases {
			if len(w.Album.Artists) == 0 && len(r.Credit) > 0 {
				w.Album.ArtistCredit = creditText(r.Credit)
				for _, c := range r.Credit {
					w.Album.Artists = append(w.Album.Artists, Credit{ID: strings.ToLower(c.Artist.ID), Name: c.Artist.Name})
				}
			}
			if w.Album.Title == "" && r.Group != nil {
				w.Album.Title, w.Album.PrimaryType, w.Album.FirstReleaseDate = r.Group.Title, r.Group.PrimaryType, r.Group.Date
				if r.Group.SecondaryTypes != nil {
					w.Album.SecondaryTypes = r.Group.SecondaryTypes
				}
			}
			w.Editions = append(w.Editions, edition(r))
		}
		w.Offset += len(page.Releases)
		w.Count = page.Count
		w.Pages++
		if len(page.Releases) > 0 && w.Offset < page.Count && w.Pages < s.o.EditionPages {
			return s.saveAlbumWalk(ctx, mbid, w)
		}
		home := ""
		if len(w.Album.Artists) > 0 {
			home = s.country(ctx, w.Album.Artists[0].ID)
		}
		w.Chosen = Choose(w.Editions, home)
		if w.Chosen != "" {
			return s.saveAlbumWalk(ctx, mbid, w)
		}
	} else {
		var rel mbRelease
		if err := s.call(ctx, "/release/"+w.Chosen, url.Values{"inc": {"recordings+artist-credits"}}, &rel); err != nil {
			return err
		}
		w.Album.Tracks = tracks(rel)
	}
	a := assemble(w, s.o.Now().UTC())
	s.add("album_walks", 1)
	return s.putAlbum(ctx, a, w.Chosen)
}

// assemble makes the album answer of a finished walk.
func assemble(w albumWalk, now time.Time) Album {
	a := w.Album
	a.Editions = w.Editions
	if a.Editions == nil {
		a.Editions = []Edition{}
	}
	if a.Tracks == nil {
		a.Tracks = []Track{}
	}
	a.EditionCount = w.Count
	a.EditionsComplete = len(a.Editions) >= w.Count
	for i := range a.Editions {
		if a.Editions[i].ID == w.Chosen {
			e := a.Editions[i]
			a.Edition = &e
		}
	}
	discs := map[int]bool{}
	for _, t := range a.Tracks {
		discs[t.Disc] = true
		a.LengthMS += int64(t.LengthMS)
	}
	a.TrackCount, a.DiscCount = len(a.Tracks), len(discs)
	a.FetchedAt = now
	return a
}

// FromReleases makes an album answer the way a walk does, from
// MusicBrainz-shaped JSON: releases is the release group's release list
// (each with media, artist credits and the release group, as the release
// browse answers it) and full answers one release with its recordings.
// home is the artist's country (picks the edition). False when releases
// does not parse.
func FromReleases(mbid string, releases []byte, count int, home string, full func(id string) ([]byte, bool), at time.Time) (Album, bool) {
	var rels []mbRelease
	if json.Unmarshal(releases, &rels) != nil {
		return Album{}, false
	}
	w := albumWalk{Album: Album{MBID: mbid, SecondaryTypes: []string{}, Artists: []Credit{}}, Count: count}
	for _, r := range rels {
		if len(w.Album.Artists) == 0 && len(r.Credit) > 0 {
			w.Album.ArtistCredit = creditText(r.Credit)
			for _, c := range r.Credit {
				w.Album.Artists = append(w.Album.Artists, Credit{ID: strings.ToLower(c.Artist.ID), Name: c.Artist.Name})
			}
		}
		if w.Album.Title == "" && r.Group != nil {
			w.Album.Title, w.Album.PrimaryType, w.Album.FirstReleaseDate = r.Group.Title, r.Group.PrimaryType, r.Group.Date
			if r.Group.SecondaryTypes != nil {
				w.Album.SecondaryTypes = r.Group.SecondaryTypes
			}
		}
		w.Editions = append(w.Editions, edition(r))
	}
	w.Chosen = Choose(w.Editions, home)
	if w.Chosen != "" && full != nil {
		if b, ok := full(w.Chosen); ok {
			var rel mbRelease
			if json.Unmarshal(b, &rel) == nil {
				w.Album.Tracks = tracks(rel)
			}
		}
	}
	return assemble(w, at), true
}

func (s *Store) saveAlbumWalk(ctx context.Context, mbid string, w albumWalk) error {
	b, _ := json.Marshal(w)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO albums(mbid, walk) VALUES(?,?) ON CONFLICT(mbid) DO UPDATE SET walk=excluded.walk`, mbid, b); err != nil {
		return err
	}
	return fetchq.ErrMore
}

func tracks(rel mbRelease) []Track {
	out := []Track{}
	for _, m := range rel.Media {
		for _, t := range m.Tracks {
			tr := Track{Disc: m.Position, Position: t.Position, Number: t.Number, Title: t.Title, Recording: strings.ToLower(t.Recording.ID)}
			switch {
			case t.Length != nil:
				tr.LengthMS = *t.Length
			case t.Recording.Length != nil:
				tr.LengthMS = *t.Recording.Length
			}
			credit := t.Credit
			if len(credit) == 0 {
				credit = t.Recording.Credit
			}
			if c := creditText(credit); c != creditText(rel.Credit) {
				tr.ArtistCredit = c
			}
			out = append(out, tr)
		}
	}
	return out
}

func (s *Store) putAlbum(ctx context.Context, a Album, chosen string) error {
	body, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO albums(mbid, body, walked, stale, walk) VALUES(?,?,?,0,'')
ON CONFLICT(mbid) DO UPDATE SET body=excluded.body, walked=excluded.walked, stale=0, walk=''`, a.MBID, body, s.o.Now().Unix()); err != nil {
		return err
	}
	refs := []string{a.MBID}
	if chosen != "" {
		refs = append(refs, chosen)
	}
	if err := setRefs(ctx, tx, "g:"+a.MBID, refs); err != nil {
		return err
	}
	return tx.Commit()
}

// country is the artist's country from its stored answer ("" when unknown).
func (s *Store) country(ctx context.Context, mbid string) string {
	if s.o.Stored == nil || mbid == "" {
		return ""
	}
	b, ok := s.o.Stored(ctx, "artist", mbid)
	if !ok {
		return ""
	}
	var d struct {
		Source struct {
			Country string `json:"country"`
		} `json:"source_data"`
	}
	_ = json.Unmarshal(b, &d)
	return d.Source.Country
}

// Choose picks the edition whose tracks an album shows: of the official
// editions (any when none is), those with the most common track count
// (the smaller on a tie); of those, released in the artist's home country
// or worldwide when any is; of those, digital or CD only when any is;
// then the earliest, then the lowest id. "" when there is no edition.
func Choose(eds []Edition, home string) string {
	pool := filter(eds, func(e Edition) bool { return e.Status == "Official" })
	if len(pool) == 0 {
		pool = eds
	}
	if len(pool) == 0 {
		return ""
	}
	counts := map[int]int{}
	for _, e := range pool {
		counts[e.TrackCount]++
	}
	mode, best := 0, -1
	for n, c := range counts {
		if c > best || c == best && n < mode {
			mode, best = n, c
		}
	}
	pool = filter(pool, func(e Edition) bool { return e.TrackCount == mode })
	pool = prefer(pool, func(e Edition) bool {
		for _, c := range e.Countries {
			if c == "XW" || home != "" && c == home {
				return true
			}
		}
		return false
	})
	pool = prefer(pool, func(e Edition) bool {
		if len(e.Formats) == 0 {
			return false
		}
		for _, f := range e.Formats {
			if f != "Digital Media" && f != "CD" {
				return false
			}
		}
		return true
	})
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if a.Date != b.Date {
			if a.Date == "" || b.Date == "" {
				return b.Date == ""
			}
			return a.Date < b.Date
		}
		return a.ID < b.ID
	})
	return pool[0].ID
}

func filter(eds []Edition, keep func(Edition) bool) []Edition {
	var out []Edition
	for _, e := range eds {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

// prefer keeps the editions that match when any does.
func prefer(eds []Edition, match func(Edition) bool) []Edition {
	if m := filter(eds, match); len(m) > 0 {
		return m
	}
	return eds
}

// Due lists stored artists and albums the change feed marked or that are
// past the safety refresh, oldest first, at most limit of each.
func (s *Store) Due(ctx context.Context, limit int) (artists, albums []string) {
	cut := s.o.Now().Add(-s.o.Rewalk).Unix()
	for _, q := range []struct {
		sql string
		out *[]string
	}{
		{`SELECT mbid FROM artists WHERE walked>0 AND (stale=1 OR walked<?) ORDER BY walked LIMIT ?`, &artists},
		{`SELECT mbid FROM albums WHERE walked>0 AND (stale=1 OR walked<?) ORDER BY walked LIMIT ?`, &albums},
	} {
		rows, err := s.db.QueryContext(ctx, q.sql, cut, limit)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				*q.out = append(*q.out, id)
			}
		}
		rows.Close()
	}
	return artists, albums
}

// NeedsArtist and NeedsAlbum report whether a prefetch should queue the
// walk: nothing stored, or the stored answer is due.
// An answer from the slim copy is never due (the dump refresh renews it).
// A copy with a cheap check (mbread Has) is asked that first, so the
// hourly prefetch does not build every answer just to see it exists.
func (s *Store) NeedsArtist(ctx context.Context, mbid string) bool {
	if h, ok := s.o.Slim.(interface {
		Has(ctx context.Context, kind, mbid string) bool
	}); ok && h.Has(ctx, "artist", mbid) {
		return false
	}
	if s.o.Slim != nil {
		if _, ok := s.o.Slim.Artist(ctx, mbid); ok {
			return false
		}
	}
	a, ok := s.storedArtist(ctx, mbid)
	return !ok || s.ArtistDue(a)
}

func (s *Store) NeedsAlbum(ctx context.Context, mbid string) bool {
	if h, ok := s.o.Slim.(interface {
		Has(ctx context.Context, kind, mbid string) bool
	}); ok && h.Has(ctx, "release-group", mbid) {
		return false
	}
	if s.o.Slim != nil {
		if _, ok := s.o.Slim.Album(ctx, mbid); ok {
			return false
		}
	}
	a, ok := s.storedAlbum(ctx, mbid)
	return !ok || s.AlbumDue(a)
}

// Stats reads the walk numbers.
func (s *Store) Stats(ctx context.Context) Stats {
	s.mu.Lock()
	st := s.st
	if s.o.Now().Before(s.pausedUntil) {
		st.PausedUntil = s.pausedUntil.UTC().Format(time.RFC3339)
	}
	s.mu.Unlock()
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(complete),0), COALESCE(SUM(stale),0) FROM artists WHERE walked>0`).Scan(&st.Artists, &st.ArtistsFull, &st.Stale)
	var albumStale int64
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(stale),0) FROM albums WHERE walked>0`).Scan(&st.Albums, &albumStale)
	st.Stale += albumStale
	return st
}
