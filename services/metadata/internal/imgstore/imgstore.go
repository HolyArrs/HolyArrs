// Package imgstore is the music picture store (picture plan 2026-10-26):
// album covers, artist photos, wide backgrounds and logos, each found
// through its kind's chain of sources, cleaned (upright, sRGB, no
// metadata), stored as WebP at fixed slot sizes (logos as PNG) and
// recorded with where it came from, its size and its main colour.
//
// Slots: covers and photos 500 at first fetch and the big size (1200, or
// the source's size when smaller) made ahead in the background; wide
// backgrounds 960 and 1920; logos as they come. Installs make 250 from 500.
package imgstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	_ "modernc.org/sqlite"
)

// Kinds stored.
const (
	KindArtist = "artist" // an artist's photo
	KindAlbum  = "album"  // an album's cover
	KindWide   = "wide"   // an artist's wide background
	KindLogo   = "logo"   // an artist's logo
)

// Slot files.
const (
	SlotCard     = "500"
	SlotBig      = "1200"
	SlotWideRow  = "960"
	SlotWideHero = "1920"
	SlotLogo     = "png"
)

// KnownKind reports a stored kind.
func KnownKind(kind string) bool {
	switch kind {
	case KindArtist, KindAlbum, KindWide, KindLogo, KindTitleLogo, KindTitleBanner, KindTitleWide, KindPerson:
		return true
	}
	return false
}

var (
	// ErrNoPicture: no source has a picture (a real "not found").
	ErrNoPicture = errors.New("imgstore: no picture")
	// ErrRetry: a source could not answer (busy, down, a timeout) or the
	// stored answer the chains need is not there yet; ask again later.
	// Never stored as missing.
	ErrRetry = errors.New("imgstore: ask again later")
	// ErrMusicFull: the music data is over its cap, so no new music
	// picture is written; it is a retry, never stored as missing.
	ErrMusicFull = fmt.Errorf("%w: music data over the cap", ErrRetry)
)

// errBlockedURL is a redirect to an address that is not a picture host.
var errBlockedURL = errors.New("imgstore: not a picture host")

// pictureDomains are the hosts pictures are downloaded from (each with its
// subdomains).
var pictureDomains = []string{"mzstatic.com", "dzcdn.net", "scdn.co", "spotifycdn.com", "fanart.tv",
	"coverartarchive.org", "archive.org", "ytimg.com", "wikimedia.org"}

// PictureURL reports an https address on a picture host.
func PictureURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range pictureDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidMBID reports a lower-case MusicBrainz ID.
func ValidMBID(s string) bool { return mbidRe.MatchString(s) }

// Record is one stored picture.
type Record struct {
	Kind, ID string
	// Provider and SourceURL are where the picture came from; BigURL is
	// the same picture at its big size ("" when the source has none).
	Provider, SourceURL, BigURL, ETag string
	// License and Author are recorded for Wikimedia pictures.
	License, Author string
	// Version is a hash of the first slot's bytes (picture addresses carry
	// it, so a changed picture gets a new address).
	Version       string
	Width, Height int
	Color         string
	// Sizes are the slot files made ("500", "1200"; "960", "1920"; "png";
	// "banner", "1600").
	Sizes   []string
	Bytes   int64
	Fetched time.Time
	Recheck time.Time
	// Upgrade is the source queued to replace an overflow picture on its
	// recheck ("applemusic"; "" for none).
	Upgrade string
}

// Has reports a made slot.
func (r Record) Has(slot string) bool { return slices.Contains(r.Sizes, slot) }

// Info is a picture's record as text answers carry it: installs draw the
// right box and colour before the picture arrives.
type Info struct {
	Version string `json:"v"`
	Width   int    `json:"w,omitempty"`
	Height  int    `json:"h,omitempty"`
	Color   string `json:"color,omitempty"`
	// Source and FetchedAt say where the picture came from and when it
	// was fetched (RFC 3339, UTC).
	Source    string `json:"src,omitempty"`
	FetchedAt string `json:"fetched_at,omitempty"`
}

// Info answers the record's public part.
func (r Record) Info() Info {
	i := Info{Version: r.Version, Width: r.Width, Height: r.Height, Color: r.Color, Source: r.Provider}
	if !r.Fetched.IsZero() {
		i.FetchedAt = r.Fetched.UTC().Format(time.RFC3339)
	}
	return i
}

// Options configure the store.
type Options struct {
	Dir string
	// Chain lists a picture's sources in order (Resolver.Chain).
	Chain func(kind, mbid string) []Step
	// Known reports whether the stored answer the chains read is there.
	Known     func(ctx context.Context, kind, mbid string) bool
	HTTP      *http.Client
	UserAgent string
	// WantBig queues a picture's big size (made in the background); nil
	// makes it at once.
	WantBig func(kind, mbid string)
	// MinFree is the free-space share under which cleanup starts (0.15);
	// MaxBytes caps the store (0: no cap). Least-viewed pictures go first.
	MinFree  float64
	MaxBytes int64
	// Disk reports used and total bytes of the volume (refresh.DiskUsage).
	Disk func(path string) (used, total uint64, err error)
	// AllowURL says whether a picture address (or a redirect) may be
	// downloaded (PictureURL in serve); nil allows every address.
	AllowURL func(u *url.URL) bool
	// MusicCap is checked every 10 minutes (and by CheckMusicCap); while
	// it answers an error no new music picture is written. nil: no guard.
	MusicCap func(ctx context.Context) error
	// AlbumsOf answers an artist's album ids (release groups); an artist
	// photo byte for byte one of their covers is refused. nil: no check.
	AlbumsOf func(ctx context.Context, artist string) []string
	Now      func() time.Time
	Logf     func(string, ...any)
}

// Stats are the store numbers for /v1/stats.
type Stats struct {
	Pictures  int64            `json:"pictures"`
	Bytes     int64            `json:"bytes"`
	Providers map[string]int64 `json:"providers,omitempty"`
	Fetched   int64            `json:"fetched"`
	Unchanged int64            `json:"unchanged_304"`
	Missing   int64            `json:"no_picture"`
	Retry     int64            `json:"retry_later"`
	Rejected  int64            `json:"rejected"`
	Bigs      int64            `json:"big_made"`
	Evicted   int64            `json:"evicted"`
	Failed    int64            `json:"failed"`
}

// Store keeps pictures on disk with one SQLite row each.
type Store struct {
	o  Options
	db *sql.DB

	mu    sync.Mutex
	views map[string]int64
	st    Stats

	recMu sync.RWMutex
	recs  map[string]*Record // nil value: known not stored

	flightMu sync.Mutex
	flights  map[string]*flight

	musicFull atomic.Bool
}

// CheckMusicCap runs the MusicCap guard and stops (or starts again) new
// music picture writes. It answers true while over the cap.
func (s *Store) CheckMusicCap(ctx context.Context) bool {
	if s.o.MusicCap == nil {
		return false
	}
	err := s.o.MusicCap(ctx)
	full := err != nil
	if s.musicFull.Swap(full) != full {
		if full {
			s.o.Logf("imgstore: new music pictures stopped: %v", err)
		} else {
			s.o.Logf("imgstore: music data back under the cap, new music pictures allowed")
		}
	}
	return full
}

type flight struct {
	done chan struct{}
	err  error
}

// slots caps the CPU-heavy work (decoding, resizing, encoding) at one at
// a time: the container has two cores and pages keep the other one.
var slots = make(chan struct{}, 1)

// SlotCount is how many picture-work slots there are.
func SlotCount() int { return cap(slots) }

// Slot takes one of the two picture-work slots; the answer gives it back.
func Slot(ctx context.Context) (func(), error) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Open opens (or creates) the store in o.Dir.
func Open(o Options) (*Store, error) {
	if o.MinFree <= 0 {
		o.MinFree = 0.15
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if allow := o.AllowURL; allow != nil {
		// Redirects are held to the same hosts.
		c := *o.HTTP
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("imgstore: too many redirects")
			}
			if !allow(req.URL) {
				return errBlockedURL
			}
			return nil
		}
		o.HTTP = &c
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(o.Dir, "images.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{o: o, db: db, views: map[string]int64{}, recs: map[string]*Record{}, flights: map[string]*flight{}}, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000; PRAGMA cache_size=-32768; PRAGMA mmap_size=268435456;
CREATE TABLE IF NOT EXISTS images(
 kind TEXT NOT NULL, mbid TEXT NOT NULL,
 source TEXT NOT NULL, source_url TEXT NOT NULL, source_etag TEXT NOT NULL DEFAULT '',
 version TEXT NOT NULL, bytes INTEGER NOT NULL,
 fetched INTEGER NOT NULL, checked INTEGER NOT NULL,
 views INTEGER NOT NULL DEFAULT 0, last_served INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(kind, mbid));
CREATE INDEX IF NOT EXISTS images_cold ON images(views, last_served);
CREATE TABLE IF NOT EXISTS misses(kind TEXT NOT NULL, mbid TEXT NOT NULL, until INTEGER NOT NULL, PRIMARY KEY(kind, mbid));
CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS source_ids(kind TEXT NOT NULL, mbid TEXT NOT NULL, source TEXT NOT NULL, store TEXT NOT NULL, ext TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(kind, mbid, source));
CREATE INDEX IF NOT EXISTS source_ids_ext ON source_ids(source, ext);`); err != nil {
		return err
	}
	for _, col := range []string{"big_url TEXT NOT NULL DEFAULT ''", "license TEXT NOT NULL DEFAULT ''", "author TEXT NOT NULL DEFAULT ''",
		"width INTEGER NOT NULL DEFAULT 0", "height INTEGER NOT NULL DEFAULT 0", "color TEXT NOT NULL DEFAULT ''",
		"sizes TEXT NOT NULL DEFAULT ''", "recheck INTEGER NOT NULL DEFAULT 0", "upgrade TEXT NOT NULL DEFAULT ''"} {
		if _, err := db.Exec("ALTER TABLE images ADD COLUMN " + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	// found_by is how a source match was found (a Via name; "" before
	// the exact-id order classified it).
	if _, err := db.Exec("ALTER TABLE source_ids ADD COLUMN found_by TEXT NOT NULL DEFAULT ''"); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	// Source matches carry when they were fetched and through which
	// address; older rows take their match time as the fetch time.
	for _, col := range []string{"fetched_at INTEGER NOT NULL DEFAULT 0", "via_ip TEXT NOT NULL DEFAULT ''"} {
		if _, err := db.Exec("ALTER TABLE source_ids ADD COLUMN " + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE source_ids SET fetched_at = at WHERE fetched_at = 0`); err != nil {
		return err
	}
	// The older picking rules' columns and table are gone.
	for _, col := range []string{"picked", "cand_key", "scores"} {
		if _, err := db.Exec("ALTER TABLE images DROP COLUMN " + col); err != nil && !strings.Contains(err.Error(), "no such column") {
			return err
		}
	}
	// Photos were once checked for blur at the card size, which refused real
	// portraits: their misses go, and photos a later source gave are looked
	// at again (once, at low priority).
	var done string
	if db.QueryRow(`SELECT v FROM kv WHERE k='photo_gate'`).Scan(&done) != nil {
		if _, err := db.Exec(`DELETE FROM misses WHERE kind IN ('artist','person');
UPDATE images SET recheck=0 WHERE kind='artist' AND source IN ('spotify','deezer') AND sizes != '';
INSERT INTO kv(k,v) VALUES('photo_gate','2')`); err != nil {
			return err
		}
	}
	_, err := db.Exec(`DROP TABLE IF EXISTS art;
DROP INDEX IF EXISTS images_recheck;
CREATE INDEX IF NOT EXISTS images_due ON images(recheck);
CREATE INDEX IF NOT EXISTS images_source ON images(source);`)
	return err
}

// Close flushes view counts and closes the store.
func (s *Store) Close() error {
	s.FlushViews(context.Background())
	return s.db.Close()
}

func key(kind, id string) string { return kind + "|" + id }

// Dir is the folder of one picture's files, in at most 256 folders per
// kind: a MusicBrainz ID's first two characters, another ID's last two
// (a one-character ID padded with a 0). Callers check IDs first (ValidID);
// an empty one still gets a folder of its own, never a panic.
func (s *Store) Dir(kind, id string) string {
	return filepath.Join(s.o.Dir, kind, shard(id), id)
}

func shard(id string) string {
	switch {
	case ValidMBID(id):
		return id[:2]
	case id == "":
		return "00"
	case len(id) == 1:
		return "0" + id
	}
	return id[len(id)-2:]
}

// SlotPath is the file of one slot.
func (s *Store) SlotPath(kind, id, slot string) string {
	if slot == SlotLogo {
		return filepath.Join(s.Dir(kind, id), "logo.png")
	}
	if slot == SlotBanner {
		return filepath.Join(s.Dir(kind, id), "banner.webp")
	}
	return filepath.Join(s.Dir(kind, id), slot+".webp")
}

const recordCols = `kind, mbid, source, source_url, big_url, source_etag, license, author, version, width, height, color, sizes, bytes, fetched, recheck, upgrade`

func scanRecord(sc interface{ Scan(...any) error }) (Record, error) {
	var r Record
	var sizes string
	var fetched, recheck int64
	err := sc.Scan(&r.Kind, &r.ID, &r.Provider, &r.SourceURL, &r.BigURL, &r.ETag, &r.License, &r.Author, &r.Version, &r.Width, &r.Height, &r.Color, &sizes, &r.Bytes, &fetched, &recheck, &r.Upgrade)
	if sizes != "" {
		r.Sizes = strings.Split(sizes, ",")
	}
	r.Fetched, r.Recheck = time.Unix(fetched, 0), time.Unix(recheck, 0)
	return r, err
}

// Record answers a stored picture's record (held in memory once read).
func (s *Store) Record(ctx context.Context, kind, id string) (Record, bool) {
	k := key(kind, id)
	s.recMu.RLock()
	r, cached := s.recs[k]
	s.recMu.RUnlock()
	if cached {
		if r == nil {
			return Record{}, false
		}
		return *r, true
	}
	rec, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordCols+` FROM images WHERE kind=? AND mbid=?`, kind, id))
	s.recMu.Lock()
	if len(s.recs) > 400000 {
		s.recs = map[string]*Record{}
	}
	switch {
	case err == nil:
		s.recs[k] = &rec
	case errors.Is(err, sql.ErrNoRows):
		s.recs[k] = nil
	}
	s.recMu.Unlock()
	return rec, err == nil
}

func (s *Store) forget(kind, id string) {
	s.recMu.Lock()
	delete(s.recs, key(kind, id))
	s.recMu.Unlock()
}

// Version answers a stored picture's version ("" when not stored).
func (s *Store) Version(ctx context.Context, kind, id string) string {
	r, _ := s.Record(ctx, kind, id)
	return r.Version
}

// Served answers the file that serves a slot of a stored picture, and
// counts a view. final is false when a smaller slot stands in for a big one
// not made yet.
func (s *Store) Served(ctx context.Context, kind, id, want string) (path string, rec Record, final, ok bool) {
	if !ValidID(kind, id) {
		return "", rec, false, false
	}
	rec, ok = s.Record(ctx, kind, id)
	if !ok {
		return "", rec, false, false
	}
	final = true
	slot := want
	switch kind {
	case KindWide:
		slot = SlotWideHero
		if want == "500" || want == SlotWideRow {
			slot = SlotWideRow
		}
	case KindLogo, KindTitleLogo:
		slot = SlotLogo
	case KindTitleBanner:
		slot = SlotBanner
	case KindTitleWide:
		slot = SlotTitleWide
	default:
		if want != SlotBig {
			slot = SlotCard
		} else if !rec.Has(SlotBig) {
			slot, final = SlotCard, rec.BigURL == ""
		}
	}
	path = s.SlotPath(kind, id, slot)
	if _, err := os.Stat(path); err != nil {
		return "", rec, false, false
	}
	s.mu.Lock()
	s.views[key(kind, id)]++
	s.mu.Unlock()
	return path, rec, final, true
}

// FlushViews writes the counted views (batched, not one write per request).
func (s *Store) FlushViews(ctx context.Context) {
	s.mu.Lock()
	views := s.views
	s.views = map[string]int64{}
	s.mu.Unlock()
	now := s.o.Now().Unix()
	for k, n := range views {
		kind, id, _ := strings.Cut(k, "|")
		_, _ = s.db.ExecContext(ctx, `UPDATE images SET views=views+?, last_served=? WHERE kind=? AND mbid=?`, n, now, kind, id)
	}
}

func (s *Store) count(f func(*Stats)) {
	s.mu.Lock()
	f(&s.st)
	s.mu.Unlock()
}

// missed reports a recent real "not found" (re-checked after MissRecheck).
// Missed reports a picture no source had, within its re-check wait.
func (s *Store) Missed(ctx context.Context, kind, id string) bool { return s.missed(ctx, kind, id) }

func (s *Store) missed(ctx context.Context, kind, id string) bool {
	var until int64
	return s.db.QueryRowContext(ctx, `SELECT until FROM misses WHERE kind=? AND mbid=?`, kind, id).Scan(&until) == nil && s.o.Now().Unix() < until
}

// Fetch stores (or re-checks) one picture by walking its chain. Several
// asks for one picture at once share one fetch. ErrNoPicture is a real
// "not found" (kept MissRecheck); ErrRetry means ask again later.
func (s *Store) Fetch(ctx context.Context, kind, id string) error {
	if !KnownKind(kind) || !ValidID(kind, id) {
		return ErrNoPicture
	}
	k := key(kind, id)
	s.flightMu.Lock()
	if f, ok := s.flights[k]; ok {
		s.flightMu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	s.flights[k] = f
	s.flightMu.Unlock()
	f.err = s.fetch(ctx, kind, id)
	s.flightMu.Lock()
	delete(s.flights, k)
	s.flightMu.Unlock()
	close(f.done)
	return f.err
}

func (s *Store) fetch(ctx context.Context, kind, id string) error {
	old, stored := s.Record(ctx, kind, id)
	if !stored && s.missed(ctx, kind, id) {
		return ErrNoPicture
	}
	known := s.o.Known == nil || s.o.Known(ctx, kind, id)
	transient := false
	// connErr is a source that could not be reached at all: the retry then
	// carries it, so the queue waits without counting a failure.
	var connErr error
	retry := func() error {
		s.count(func(st *Stats) { st.Retry++ })
		if connErr != nil {
			return errors.Join(ErrRetry, connErr)
		}
		return ErrRetry
	}
	// skipped is the first source passed over because it is paused or
	// rate-limited (a picture not stored yet only): what a later source
	// gives is tagged for an upgrade from it, looked at again soon.
	skipped := ""
	for _, st := range s.o.Chain(kind, id) {
		t0 := time.Now()
		cands, err := st.Get(ctx)
		since(ctx, PhaseChain, t0)
		var se *SourceError
		if err != nil && !stored && errors.As(err, &se) && se.Paused {
			if skipped == "" {
				skipped = se.Source
			}
			continue
		}
		if skipped != "" {
			for i := range cands {
				if cands[i].Upgrade == "" {
					cands[i].Upgrade, cands[i].Recheck = skipped, UpgradeRecheck
				}
			}
		}
		if err != nil {
			transient = true
			if upstream.ConnError(err) {
				connErr = err
			}
			s.o.Logf("imgstore: %s %s: %v", kind, id, err)
			continue
		}
		for _, c := range cands {
			if stored && transient {
				// A source before this one could not answer: keep the
				// stored picture rather than swap it for a later source.
				return retry()
			}
			done, err := s.tryCandidate(ctx, kind, id, c, old, stored, known && !transient)
			if done {
				return err
			}
			if err != nil {
				transient = true
				if upstream.ConnError(err) {
					connErr = err
				}
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// A skipped source never answered, so nothing found is no miss yet;
	// the retry says which source is paused.
	if skipped != "" && !transient && known {
		s.count(func(st *Stats) { st.Retry++ })
		return errors.Join(ErrRetry, fmt.Errorf("%w: %s", ErrSourcePaused, skipped))
	}
	if transient || !known {
		return retry()
	}
	if stored {
		return s.keepOrDrop(ctx, old)
	}
	s.count(func(st *Stats) { st.Missing++ })
	_, _ = s.db.ExecContext(ctx, `INSERT INTO misses(kind,mbid,until) VALUES(?,?,?) ON CONFLICT(kind,mbid) DO UPDATE SET until=excluded.until`, kind, id, s.o.Now().Add(MissRecheck).Unix())
	return ErrNoPicture
}

// tryCandidate downloads, checks and stores one candidate. done reports
// the chain ends here (stored, or unchanged since the last check); err
// with done false is a failure worth asking again for (busy, down).
func (s *Store) tryCandidate(ctx context.Context, kind, id string, c Candidate, old Record, stored, sure bool) (done bool, err error) {
	etag := ""
	if stored && old.SourceURL == c.URL {
		etag = old.ETag
	}
	t0 := time.Now()
	raw, newTag, status, err := s.download(ctx, c.URL, etag)
	since(ctx, PhaseDownload, t0)
	switch {
	case err != nil:
		s.count(func(st *Stats) { st.Failed++ })
		return false, err
	case status == http.StatusNotModified:
		s.recheck(ctx, kind, id, c, sure)
		s.count(func(st *Stats) { st.Unchanged++ })
		return true, nil
	case status != 200:
		return false, nil
	}
	t0 = time.Now()
	release, err := Slot(ctx)
	since(ctx, PhaseSlotWait, t0)
	if err != nil {
		return true, err
	}
	t0 = time.Now()
	img, err := Decode(raw)
	if err != nil {
		release()
		s.count(func(st *Stats) { st.Rejected++ })
		return false, nil
	}
	if g := CheckGate(kind, img); !g.OK {
		release()
		s.count(func(st *Stats) { st.Rejected++ })
		return false, nil
	}
	err = s.write(ctx, kind, id, c, newTag, img, sure)
	release()
	since(ctx, PhaseWork, t0)
	if errors.Is(err, ErrNotAPhoto) {
		s.count(func(st *Stats) { st.Rejected++ })
		return false, nil
	}
	if err != nil {
		return true, err
	}
	s.storeBig(ctx, kind, id, c.Big)
	return true, nil
}

type bigLaterKey struct{}

// BigLater marks a fetch someone waits on: its big size is queued for
// later rather than made before the fetch answers.
func BigLater(ctx context.Context) context.Context {
	return context.WithValue(ctx, bigLaterKey{}, true)
}

func bigLater(ctx context.Context) bool { v, _ := ctx.Value(bigLaterKey{}).(bool); return v }

// IsBigLater reports whether ctx was marked with BigLater.
func IsBigLater(ctx context.Context) bool { return bigLater(ctx) }

// storeBig makes a new cover's or photo's big size (1200) at store time
// when its source has a bigger address, so it never waits for a later
// pass; a fetch marked BigLater, or a failed try, queues it (WantBig).
func (s *Store) storeBig(ctx context.Context, kind, id, bigURL string) {
	if bigURL == "" || kind != KindAlbum && kind != KindArtist && kind != KindPerson {
		return
	}
	if !bigLater(ctx) || s.o.WantBig == nil {
		t0 := time.Now()
		err := s.MakeBig(ctx, kind, id)
		since(ctx, PhaseBig, t0)
		if err == nil {
			return
		}
		s.o.Logf("imgstore: big %s %s: %v", kind, id, err)
	}
	if s.o.WantBig != nil {
		s.o.WantBig(kind, id)
	}
}

// keepOrDrop handles a re-check where no source offered a picture: the
// stored one stays unless its own address is gone (404 or 410).
func (s *Store) keepOrDrop(ctx context.Context, old Record) error {
	_, _, status, err := s.download(ctx, old.SourceURL, old.ETag)
	if err == nil && (status == http.StatusNotFound || status == http.StatusGone) {
		s.drop(ctx, old.Kind, old.ID)
		_, _ = s.db.ExecContext(ctx, `INSERT INTO misses(kind,mbid,until) VALUES(?,?,?) ON CONFLICT(kind,mbid) DO UPDATE SET until=excluded.until`, old.Kind, old.ID, s.o.Now().Add(MissRecheck).Unix())
		return ErrNoPicture
	}
	s.recheck(ctx, old.Kind, old.ID, Candidate{Provider: old.Provider, URL: old.SourceURL}, true)
	return nil
}

func recheckFor(c Candidate, sure bool) time.Duration {
	if !sure {
		return UnknownRecheck
	}
	if c.Recheck > 0 {
		return c.Recheck
	}
	return DefaultRecheck
}

func (s *Store) recheck(ctx context.Context, kind, id string, c Candidate, sure bool) {
	now := s.o.Now()
	_, _ = s.db.ExecContext(ctx, `UPDATE images SET checked=?, recheck=? WHERE kind=? AND mbid=?`, now.Unix(), now.Add(recheckFor(c, sure)).Unix(), kind, id)
	s.forget(kind, id)
}

// download fetches a picture (If-None-Match with etag). A network failure,
// a timeout, 429 or 5xx is an error (ask again later); any other status is
// answered for the caller to read.
func (s *Store) download(ctx context.Context, u, etag string) (raw []byte, newTag string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", 0, nil
	}
	if s.o.AllowURL != nil && !s.o.AllowURL(req.URL) {
		// Not a picture host: never fetched, no picture from it.
		return nil, "", http.StatusForbidden, nil
	}
	if s.o.UserAgent != "" {
		req.Header.Set("User-Agent", s.o.UserAgent)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := s.o.HTTP.Do(req)
	if errors.Is(err, errBlockedURL) || errors.Is(err, upstream.ErrBlockedAddress) {
		return nil, "", http.StatusForbidden, nil
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) || ctx.Err() == nil {
			return nil, "", 0, fmt.Errorf("imgstore: download: %w", redactURL(err))
		}
		return nil, "", 0, ctx.Err()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, "", resp.StatusCode, fmt.Errorf("imgstore: picture host answered %d", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, "", resp.StatusCode, nil
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, MaxSourceBytes+1))
	if err != nil {
		return nil, "", 0, fmt.Errorf("imgstore: download: %w", redactURL(err))
	}
	return raw, resp.Header.Get("ETag"), 200, nil
}

// redactURL keeps a request error without its address (a key may be in it).
func redactURL(err error) error {
	var ue interface{ Unwrap() error }
	if errors.As(err, &ue) && strings.Contains(err.Error(), "://") {
		if inner := ue.Unwrap(); inner != nil {
			return inner
		}
	}
	return err
}

// write makes a picture's first slots from a decoded, checked candidate
// and records it. Covers and photos get 500 now; their big size comes from
// the source's big address in the background (or now, from these pixels,
// when the source has no bigger address but sent more than 500).
func (s *Store) write(ctx context.Context, kind, id string, c Candidate, etag string, img *image.NRGBA, sure bool) error {
	if s.musicFull.Load() && slices.Contains(MusicKinds, kind) {
		return ErrMusicFull
	}
	type out struct {
		slot string
		data []byte
		w, h int
	}
	var outs []out
	add := func(slot string, m image.Image, png bool) error {
		var b []byte
		var err error
		if png {
			b, err = EncodePNG(m)
		} else {
			b, err = EncodeWebP(m)
		}
		if err == nil {
			outs = append(outs, out{slot, b, m.Bounds().Dx(), m.Bounds().Dy()})
		}
		return err
	}
	var err error
	bigURL := c.Big
	switch kind {
	case KindWide:
		if err = add(SlotWideRow, Resize(img, 960), false); err == nil {
			err = add(SlotWideHero, Resize(img, 1920), false)
		}
	case KindLogo, KindTitleLogo:
		err = add(SlotLogo, img, true)
	case KindTitleBanner:
		err = add(SlotBanner, Resize(img, 1000), false)
	case KindTitleWide:
		err = add(SlotTitleWide, Resize(img, 1600), false)
	default:
		err = add(SlotCard, Resize(img, 500), false)
		// The big size from these pixels when the source has no bigger
		// address but sent more than 500, or already sent 1200 or more.
		if n := maxInt(img.Rect.Dx(), img.Rect.Dy()); err == nil && (bigURL == "" && n > 500 || n >= 1200) {
			err = add(SlotBig, Resize(img, 1200), false)
		}
	}
	if err != nil {
		return err
	}
	if kind == KindArtist {
		if issue := s.artistCardIssue(ctx, id, outs[0].data); issue != "" {
			s.o.Logf("imgstore: artist %s: %s picture from %s refused", id, issue, c.Provider)
			return ErrNotAPhoto
		}
	}
	dir := s.Dir(kind, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{}
	var total int64
	sizes := make([]string, 0, len(outs))
	for _, o := range outs {
		p := s.SlotPath(kind, id, o.slot)
		if err := writeFile(p, o.data); err != nil {
			return err
		}
		keep[filepath.Base(p)] = true
		total += int64(len(o.data))
		sizes = append(sizes, o.slot)
	}
	// Files of an earlier picture (or the older JPEG layout) go.
	if es, err := os.ReadDir(dir); err == nil {
		for _, e := range es {
			if !keep[e.Name()] {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	sum := sha256.Sum256(outs[0].data)
	last := outs[len(outs)-1]
	now := s.o.Now()
	_, err = s.db.ExecContext(ctx, `INSERT INTO images(kind,mbid,source,source_url,big_url,source_etag,license,author,version,width,height,color,sizes,bytes,fetched,checked,recheck,upgrade) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(kind,mbid) DO UPDATE SET source=excluded.source, source_url=excluded.source_url, big_url=excluded.big_url, source_etag=excluded.source_etag, license=excluded.license, author=excluded.author, version=excluded.version, width=excluded.width, height=excluded.height, color=excluded.color, sizes=excluded.sizes, bytes=excluded.bytes, fetched=excluded.fetched, checked=excluded.checked, recheck=excluded.recheck, upgrade=excluded.upgrade`,
		kind, id, c.Provider, c.URL, bigURL, etag, c.License, c.Author, hex.EncodeToString(sum[:])[:10], last.w, last.h, MainColor(img), strings.Join(sizes, ","), total, now.Unix(), now.Unix(), now.Add(recheckFor(c, sure)).Unix(), c.Upgrade)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM misses WHERE kind=? AND mbid=?`, kind, id)
	s.forget(kind, id)
	s.count(func(st *Stats) { st.Fetched++ })
	return nil
}

func writeFile(p string, b []byte) error {
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// MakeBig makes a cover's or photo's big slot from its source's big
// address (1200, or the source's size when smaller; never larger than the
// source). A source without one is recorded as having none.
func (s *Store) MakeBig(ctx context.Context, kind, id string) error {
	if !ValidID(kind, id) {
		return nil
	}
	rec, ok := s.Record(ctx, kind, id)
	if !ok || rec.Has(SlotBig) || rec.BigURL == "" || (kind != KindAlbum && kind != KindArtist && kind != KindPerson) {
		return nil
	}
	raw, _, status, err := s.download(ctx, rec.BigURL, "")
	if err != nil {
		return ErrRetry
	}
	if status != 200 {
		_, err := s.db.ExecContext(ctx, `UPDATE images SET big_url='' WHERE kind=? AND mbid=? AND version=?`, kind, id, rec.Version)
		s.forget(kind, id)
		return err
	}
	release, err := Slot(ctx)
	if err != nil {
		return err
	}
	defer release()
	img, err := Decode(raw)
	if err != nil || maxInt(img.Rect.Dx(), img.Rect.Dy()) <= 500 {
		_, err := s.db.ExecContext(ctx, `UPDATE images SET big_url='' WHERE kind=? AND mbid=? AND version=?`, kind, id, rec.Version)
		s.forget(kind, id)
		return err
	}
	big := Resize(img, 1200)
	b, err := EncodeWebP(big)
	if err != nil {
		return err
	}
	if err := writeFile(s.SlotPath(kind, id, SlotBig), b); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE images SET sizes=?, width=?, height=?, bytes=bytes+? WHERE kind=? AND mbid=? AND version=?`,
		strings.Join(append(rec.Sizes, SlotBig), ","), big.Bounds().Dx(), big.Bounds().Dy(), len(b), kind, id, rec.Version)
	s.forget(kind, id)
	s.count(func(st *Stats) { st.Bigs++ })
	return err
}

// NeedsBig reports a cover or photo whose big slot is still to be made.
func (s *Store) NeedsBig(ctx context.Context, kind, id string) bool {
	r, ok := s.Record(ctx, kind, id)
	return ok && !r.Has(SlotBig) && r.BigURL != ""
}

func (s *Store) drop(ctx context.Context, kind, id string) {
	_ = os.RemoveAll(s.Dir(kind, id))
	_, _ = s.db.ExecContext(ctx, `DELETE FROM images WHERE kind=? AND mbid=?`, kind, id)
	s.forget(kind, id)
}

// Key names one picture.
type Key struct{ Kind, ID string }

// Due lists up to limit stored pictures whose re-check time has come, most
// viewed first.
func (s *Store) Due(ctx context.Context, limit int) []Key {
	return s.keys(ctx, `SELECT kind, mbid FROM images WHERE recheck < ? ORDER BY views DESC LIMIT ?`, s.o.Now().Unix(), limit)
}

// BigsDue lists up to limit covers and photos whose big slot is to be
// made: artist photos first, then the rest, most viewed first.
func (s *Store) BigsDue(ctx context.Context, limit int) []Key {
	return s.keys(ctx, `SELECT kind, mbid FROM images WHERE sizes = '500' AND big_url != '' ORDER BY kind = ? DESC, views DESC LIMIT ?`, KindArtist, limit)
}

// BigsLeft counts covers and photos whose big slot is still to be made.
func (s *Store) BigsLeft(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	rows, err := s.db.QueryContext(ctx, `SELECT kind, COUNT(*) FROM images WHERE sizes = '500' AND big_url != '' GROUP BY kind`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int64
		if rows.Scan(&k, &n) == nil {
			out[k] = n
		}
	}
	return out
}

func (s *Store) keys(ctx context.Context, q string, args ...any) []Key {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		var k Key
		if rows.Scan(&k.Kind, &k.ID) == nil {
			out = append(out, k)
		}
	}
	return out
}

// Purge deletes every picture a provider gave and answers them, so each can
// be filled again from the next source.
func (s *Store) Purge(ctx context.Context, provider string) ([]Key, error) {
	keys := s.keys(ctx, `SELECT kind, mbid FROM images WHERE source=?`, provider)
	for _, k := range keys {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.drop(ctx, k.Kind, k.ID)
	}
	return keys, nil
}

// Refill rebuilds pictures from their stored source addresses, no keys
// needed (after lost files, or to redo older ones). With missing only, a
// picture whose files are there is left alone. progress is told after each.
func (s *Store) Refill(ctx context.Context, missing bool, progress func(k Key, err error)) error {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordCols+` FROM images ORDER BY views DESC`)
	if err != nil {
		return err
	}
	var all []Record
	for rows.Next() {
		if r, err := scanRecord(rows); err == nil {
			all = append(all, r)
		}
	}
	rows.Close()
	for _, r := range all {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if missing && s.filesThere(r) {
			continue
		}
		err := s.refillOne(ctx, r)
		if progress != nil {
			progress(Key{r.Kind, r.ID}, err)
		}
	}
	return nil
}

func (s *Store) filesThere(r Record) bool {
	for _, slot := range r.Sizes {
		if _, err := os.Stat(s.SlotPath(r.Kind, r.ID, slot)); err != nil {
			return false
		}
	}
	return true
}

func (s *Store) refillOne(ctx context.Context, r Record) error {
	c := Candidate{Provider: r.Provider, URL: r.SourceURL, Big: r.BigURL, License: r.License, Author: r.Author}
	raw, tag, status, err := s.download(ctx, c.URL, "")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("imgstore: source answered %d", status)
	}
	release, err := Slot(ctx)
	if err != nil {
		return err
	}
	defer release()
	img, err := Decode(raw)
	if err != nil {
		return err
	}
	return s.write(ctx, r.Kind, r.ID, c, tag, img, true)
}

// Stats reads the store numbers.
func (s *Store) Stats(ctx context.Context) Stats {
	s.mu.Lock()
	st := s.st
	s.mu.Unlock()
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(bytes),0) FROM images`).Scan(&st.Pictures, &st.Bytes)
	st.Providers = map[string]int64{}
	if rows, err := s.db.QueryContext(ctx, `SELECT source, COUNT(*) FROM images GROUP BY source`); err == nil {
		for rows.Next() {
			var p string
			var n int64
			if rows.Scan(&p, &n) == nil {
				st.Providers[p] = n
			}
		}
		rows.Close()
	}
	return st
}

// Cleanup removes least-viewed music pictures while free space is under
// MinFree or the music pictures are over MaxBytes; they are fetched again
// when asked for. Movie, TV and person pictures are never removed.
func (s *Store) Cleanup(ctx context.Context) int {
	s.FlushViews(ctx)
	over := func() bool {
		if s.o.MaxBytes > 0 {
			total, _ := s.MusicBytes(ctx)
			if total > s.o.MaxBytes {
				return true
			}
		}
		if s.o.Disk == nil {
			return false
		}
		used, total, err := s.o.Disk(s.o.Dir)
		return err == nil && total > 0 && 1-float64(used)/float64(total) < s.o.MinFree
	}
	removed := 0
	for ctx.Err() == nil && over() {
		batch := s.keys(ctx, `SELECT kind, mbid FROM images WHERE kind IN (?,?,?,?) ORDER BY views, last_served, fetched LIMIT 50`,
			MusicKinds[0], MusicKinds[1], MusicKinds[2], MusicKinds[3])
		if len(batch) == 0 {
			break
		}
		for _, k := range batch {
			s.drop(ctx, k.Kind, k.ID)
			removed++
		}
	}
	if removed > 0 {
		s.count(func(st *Stats) { st.Evicted += int64(removed) })
		s.o.Logf("imgstore: over the store cap or under %.0f%% free, removed %d least-viewed pictures", s.o.MinFree*100, removed)
	}
	return removed
}

// Run flushes views every minute and checks space every 10 minutes.
func (s *Store) Run(ctx context.Context) {
	s.CheckMusicCap(ctx)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.FlushViews(ctx)
			if n++; n%10 == 0 {
				s.CheckMusicCap(ctx)
				s.Cleanup(ctx)
			}
		}
	}
}

// Checked answers when a stored picture was last fetched or re-checked
// (zero when not stored).
func (s *Store) Checked(ctx context.Context, kind, id string) time.Time {
	var at int64
	if s.db.QueryRowContext(ctx, `SELECT checked FROM images WHERE kind=? AND mbid=?`, kind, id).Scan(&at) != nil {
		return time.Time{}
	}
	return time.Unix(at, 0)
}

// SaveQuota keeps YouTube's daily count (Limits persist hook).
func (s *Store) SaveQuota(day string, units int) {
	_, _ = s.db.Exec(`INSERT INTO kv(k,v) VALUES('youtube_quota',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, day+"="+strconv.Itoa(units))
}

// LoadQuota reads the kept YouTube count.
func (s *Store) LoadQuota() (string, int) {
	var v string
	_ = s.db.QueryRow(`SELECT v FROM kv WHERE k='youtube_quota'`).Scan(&v)
	day, n, _ := strings.Cut(v, "=")
	units, _ := strconv.Atoi(n)
	return day, units
}

// Fresh reports a stored picture whose re-check is not due yet: a fetch
// for it would only ask its sources again.
func (s *Store) Fresh(ctx context.Context, kind, id string) bool {
	r, ok := s.Record(ctx, kind, id)
	return ok && r.Version != "" && s.o.Now().Before(r.Recheck)
}

// HasMiss reports a picture no source had at its last ask (its week over
// or not): it is asked again only by the weekly pass (MissesDue).
func (s *Store) HasMiss(ctx context.Context, kind, id string) bool {
	var n int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM misses WHERE kind=? AND mbid=?`, kind, id).Scan(&n) == nil
}

// MissesDue lists up to limit misses whose week is over, oldest first,
// for the weekly re-ask of the whole source chain.
func (s *Store) MissesDue(ctx context.Context, limit int) []Key {
	return s.keys(ctx, `SELECT kind, mbid FROM misses WHERE until < ? ORDER BY until LIMIT ?`, s.o.Now().Unix(), limit)
}
