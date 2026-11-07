// Package musicsync copies the metadata server's artist packages to this
// install's disk on a fixed schedule, with their pictures, so no music page
// ever waits on the server: an artist or album a package covers answers
// from local rows. The copy sends no credential and nothing about the
// install; only an artist a page asked for that no package covers is named
// to the server (package-want), so it gets covered.
package musicsync

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/lowmem"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Source is the metadata server's package list.
type Source interface {
	MusicPackages(ctx context.Context, since int64, limit int) (metadata.PackagePage, error)
	WantMusicPackage(ctx context.Context, mbid string) error
}

// Pictures stores one picture (kind artist or album, size 500 or 1200, at
// version v) in the local picture store; false when it could not.
type Pictures func(ctx context.Context, kind, mbid, size, v string) bool

const (
	// DefaultEvery is the fixed schedule of the copy.
	DefaultEvery = 30 * time.Minute
	// pageLimit is the packages asked per page (the server's most).
	pageLimit = 200
	// pageGap is the wait between two pages.
	pageGap = time.Second
	// bigRank: artists ranked up to this get their 1200 photo too.
	bigRank = 200
	// picWorkers bound the picture downloads at once.
	picWorkers = 4
	// wantAgain is how long one artist is not wanted again.
	wantAgain = 6 * time.Hour
	// wantQueue bounds the wants waiting.
	wantQueue = 256
	// cacheMax is how many decoded packages are kept in memory.
	cacheMax = 64
)

// picGap is the wait between one worker's pictures (tests shorten it).
var picGap = 50 * time.Millisecond

type artistEntry struct {
	rank int
	pics metadata.ArtistPictures
}

// Stats describe the local copy.
type Stats struct {
	Packages  int           `json:"packages"`
	Albums    int           `json:"albums"`
	Bytes     int64         `json:"bytes"`
	Cursor    int64         `json:"cursor"`
	LastSync  time.Time     `json:"last_sync"`
	Took      time.Duration `json:"took"`
	Changed   int           `json:"changed"`
	Pictures  int           `json:"pictures"`
	PicMissed int           `json:"pic_missed"`
}

// Sync is the local package store and its copy job.
type Sync struct {
	Store    *store.Store
	Source   Source
	Pictures Pictures
	// Every is the schedule (DefaultEvery when 0).
	Every time.Duration
	// OnSynced runs after each copy that changed packages (Explore prewarm).
	OnSynced func(ctx context.Context)

	mu sync.RWMutex
	// artists is the covered artists, kept in memory (about 20k, read on
	// every artist check). Albums are not: their half a million rows stay in
	// music_package_albums and are read by key (album), which keeps about
	// 110 MB out of memory.
	artists map[string]artistEntry
	albums  int // how many albums are listed (for Stats)
	loaded  bool
	// synced is true once a whole copy reached the server's latest: only
	// then is an artist missing here really not covered.
	synced bool
	stats  Stats

	cacheMu sync.Mutex
	cache   map[string]*metadata.MusicPackage
	order   []string

	wantMu sync.Mutex
	wanted map[string]time.Time
	wants  chan string
}

// Load reads the artist index and picture records from disk.
func (s *Sync) Load(ctx context.Context) error {
	arts := map[string]artistEntry{}
	db := s.Store.Read()
	rows, err := db.QueryContext(ctx, "SELECT mbid, rank, pictures FROM music_packages")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, pics string
		var rank int
		if err := rows.Scan(&id, &rank, &pics); err != nil {
			rows.Close()
			return err
		}
		e := artistEntry{rank: rank}
		_ = json.Unmarshal([]byte(pics), &e.pics)
		arts[id] = e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	albs := s.albumCount(ctx)
	var cursor int64
	var at string
	_ = db.QueryRowContext(ctx, "SELECT cursor, synced_at FROM music_package_sync WHERE id = 1").Scan(&cursor, &at)
	var size int64
	_ = db.QueryRowContext(ctx, "SELECT coalesce(sum(length(body)), 0) FROM music_packages").Scan(&size)
	s.mu.Lock()
	s.artists, s.albums, s.loaded = arts, albs, true
	// A copy made before (cursor past 0) answers coverage until this one ends.
	s.synced = s.synced || cursor > 0
	s.stats.Packages, s.stats.Albums, s.stats.Bytes, s.stats.Cursor = len(arts), albs, size, cursor
	if t, err := time.Parse(store.TimeLayout, at); err == nil {
		s.stats.LastSync = t
	}
	s.mu.Unlock()
	return nil
}

// Stats answers the copy's state.
func (s *Sync) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// LowMemoryEvery is the copy's schedule in low-memory mode (#131): fewer
// copies, each catching up on more packages.
const LowMemoryEvery = 3 * time.Hour

// schedule is Every, else DefaultEvery (LowMemoryEvery in low-memory mode).
func (s *Sync) schedule() time.Duration {
	switch {
	case s.Every > 0:
		return s.Every
	case lowmem.On():
		return LowMemoryEvery
	}
	return DefaultEvery
}

// Run loads the index, copies at once and then on the schedule, until ctx ends.
func (s *Sync) Run(ctx context.Context) {
	_ = s.Load(ctx)
	s.startWants(ctx)
	every := s.schedule()
	for {
		if changed, err := s.SyncOnce(ctx); err == nil && changed > 0 && s.OnSynced != nil {
			s.OnSynced(ctx)
		}
		t := time.NewTimer(every)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// SyncOnce copies every package changed since the stored cursor, then
// stores the changed packages' pictures. It answers how many changed.
func (s *Sync) SyncOnce(ctx context.Context) (int, error) {
	start := time.Now()
	s.mu.RLock()
	loaded := s.loaded
	s.mu.RUnlock()
	if !loaded {
		if err := s.Load(ctx); err != nil {
			return 0, err
		}
	}
	var cursor int64
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT cursor FROM music_package_sync WHERE id = 1").Scan(&cursor)
	// Only the picture jobs of changed packages are kept until the copy ends,
	// not the packages (a first copy held every decoded package at once).
	var jobs []picJob
	changed := 0
	for {
		page, err := s.Source.MusicPackages(ctx, cursor, pageLimit)
		if err != nil {
			s.finish(start, cursor, changed, false)
			s.storePictures(ctx, jobs)
			return changed, err
		}
		n, got, err := s.storePage(ctx, page)
		if err != nil {
			s.finish(start, cursor, changed, false)
			return changed, err
		}
		changed += n
		jobs = append(jobs, got...)
		cursor = page.Next
		if !page.More || len(page.Packages) == 0 {
			break
		}
		t := time.NewTimer(pageGap)
		select {
		case <-ctx.Done():
			t.Stop()
			return changed, ctx.Err()
		case <-t.C:
		}
	}
	s.finish(start, cursor, changed, true)
	s.storePictures(ctx, jobs)
	return changed, nil
}

func (s *Sync) finish(start time.Time, cursor int64, changed int, whole bool) {
	var size int64
	_ = s.Store.Read().QueryRow("SELECT coalesce(sum(length(body)), 0) FROM music_packages").Scan(&size)
	albs := s.albumCount(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.synced = s.synced || whole
	s.albums = albs
	s.stats.Packages, s.stats.Albums, s.stats.Bytes, s.stats.Cursor = len(s.artists), albs, size, cursor
	s.stats.LastSync, s.stats.Took, s.stats.Changed = time.Now().UTC(), time.Since(start), changed
}

// storePage writes one page's valid packages and the cursor in one write,
// and answers how many it stored and their picture jobs.
func (s *Sync) storePage(ctx context.Context, page metadata.PackagePage) (int, []picJob, error) {
	type row struct {
		p    *metadata.MusicPackage
		body []byte
		pics []byte
	}
	var rows []row
	z, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed) // one writer for the page
	for _, raw := range page.Packages {
		p := &metadata.MusicPackage{}
		if json.Unmarshal(raw, p) != nil || !p.Valid() {
			continue
		}
		p.MBID = strings.ToLower(p.MBID)
		body, err := pack(z, raw)
		if err != nil {
			return 0, nil, err
		}
		pics, _ := json.Marshal(artistPictures(p))
		rows = append(rows, row{p, body, pics})
	}
	now := store.FormatTime(time.Now())
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.Exec(`INSERT INTO music_packages (mbid, version, hash, rank, complete, body, pictures, stored_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (mbid) DO UPDATE SET version = excluded.version, hash = excluded.hash, rank = excluded.rank, complete = excluded.complete,
				body = excluded.body, pictures = excluded.pictures, stored_at = excluded.stored_at`,
				r.p.MBID, r.p.Version, r.p.Hash, r.p.Rank, r.p.Complete, r.body, string(r.pics), now); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM music_package_albums WHERE artist = ?", r.p.MBID); err != nil {
				return err
			}
			for _, g := range r.p.ReleaseGroups {
				cover := ""
				if g.Cover != nil {
					b, _ := json.Marshal(g.Cover)
					cover = string(b)
				}
				if _, err := tx.Exec(`INSERT INTO music_package_albums (rg, artist, cover) VALUES (?, ?, ?)
					ON CONFLICT (rg) DO UPDATE SET artist = excluded.artist, cover = excluded.cover`, strings.ToLower(g.ID), r.p.MBID, cover); err != nil {
					return err
				}
			}
		}
		_, err := tx.Exec(`INSERT INTO music_package_sync (id, cursor, synced_at) VALUES (1, ?, ?)
			ON CONFLICT (id) DO UPDATE SET cursor = excluded.cursor, synced_at = excluded.synced_at`, page.Next, now)
		return err
	})
	if err != nil {
		return 0, nil, err
	}
	var jobs []picJob
	s.mu.Lock()
	if s.artists == nil {
		s.artists = map[string]artistEntry{}
	}
	for _, r := range rows {
		s.artists[r.p.MBID] = artistEntry{rank: r.p.Rank, pics: artistPictures(r.p)}
		jobs = append(jobs, picJobs(r.p)...)
	}
	s.mu.Unlock()
	s.cacheMu.Lock()
	for _, r := range rows {
		delete(s.cache, r.p.MBID)
	}
	s.cacheMu.Unlock()
	return len(rows), jobs, nil
}

func artistPictures(p *metadata.MusicPackage) metadata.ArtistPictures {
	return metadata.ArtistPictures{Photo: p.Pictures["photo"], Wide: p.Pictures["wide"], Logo: p.Pictures["logo"]}
}

// pack gzips raw with z (reset for each package).
func pack(z *gzip.Writer, raw []byte) ([]byte, error) {
	var b bytes.Buffer
	z.Reset(&b)
	if _, err := z.Write(raw); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func unpack(body []byte) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	return io.ReadAll(io.LimitReader(z, 64<<20))
}

type picJob struct{ kind, mbid, size, v string }

// picJobs are a package's artist photos (500, and 1200 for the top ranked)
// and album covers (500) to store.
func picJobs(p *metadata.MusicPackage) []picJob {
	var jobs []picJob
	if ph := p.Pictures["photo"]; ph != nil && metadata.ValidPictureVersion(ph.V) {
		jobs = append(jobs, picJob{"artist", p.MBID, "500", ph.V})
		if p.Rank > 0 && p.Rank <= bigRank {
			jobs = append(jobs, picJob{"artist", p.MBID, "1200", ph.V})
		}
	}
	for _, g := range p.ReleaseGroups {
		if g.Cover != nil && metadata.ValidPictureVersion(g.Cover.V) {
			jobs = append(jobs, picJob{"album", strings.ToLower(g.ID), "500", g.Cover.V})
		}
	}
	return jobs
}

// storePictures stores the changed packages' pictures through Pictures, a
// few at a time.
func (s *Sync) storePictures(ctx context.Context, jobs []picJob) {
	if s.Pictures == nil || len(jobs) == 0 {
		return
	}
	ch := make(chan picJob)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, missed := 0, 0
	for range min(lowmem.Pick(picWorkers, 2), len(jobs)) { // low-memory mode: fewer at once
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				got := s.Pictures(ctx, j.kind, j.mbid, j.size, j.v)
				mu.Lock()
				if got {
					ok++
				} else {
					missed++
				}
				mu.Unlock()
				time.Sleep(picGap)
			}
		}()
	}
feed:
	for _, j := range jobs {
		select {
		case ch <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	s.mu.Lock()
	s.stats.Pictures, s.stats.PicMissed = ok, missed
	s.mu.Unlock()
}

// Package reads one artist's package from disk (kept decoded a while).
func (s *Sync) Package(ctx context.Context, mbid string) (*metadata.MusicPackage, bool) {
	mbid = strings.ToLower(mbid)
	s.cacheMu.Lock()
	if p, ok := s.cache[mbid]; ok {
		s.cacheMu.Unlock()
		return p, true
	}
	s.cacheMu.Unlock()
	var body []byte
	if s.Store.Read().QueryRowContext(ctx, "SELECT body FROM music_packages WHERE mbid = ?", mbid).Scan(&body) != nil {
		return nil, false
	}
	raw, err := unpack(body)
	if err != nil {
		return nil, false
	}
	p := &metadata.MusicPackage{}
	if json.Unmarshal(raw, p) != nil || !p.Valid() {
		return nil, false
	}
	s.cacheMu.Lock()
	if s.cache == nil {
		s.cache = map[string]*metadata.MusicPackage{}
	}
	if _, ok := s.cache[mbid]; !ok {
		s.order = append(s.order, mbid)
		if len(s.order) > cacheMax {
			delete(s.cache, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.cache[mbid] = p
	s.cacheMu.Unlock()
	return p, true
}

// Covers says a package covers the artist.
func (s *Sync) Covers(mbid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.artists[strings.ToLower(mbid)]
	return ok
}

// album reads one listed album (its artist and cover) by key. A read error
// counts as not listed, like a miss, so callers fall back to the live path.
func (s *Sync) album(ctx context.Context, rg string) (artist string, cover *metadata.PictureRecord, ok bool) {
	var raw string
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT artist, cover FROM music_package_albums WHERE rg = ?", strings.ToLower(rg)).Scan(&artist, &raw); err != nil {
		return "", nil, false
	}
	if raw != "" {
		var c metadata.PictureRecord
		if json.Unmarshal([]byte(raw), &c) == nil {
			cover = &c
		}
	}
	return artist, cover, true
}

// albumCount is how many albums the packages list.
func (s *Sync) albumCount(ctx context.Context) int {
	var n int
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM music_package_albums").Scan(&n)
	return n
}

// HasAlbum says a package lists the release group.
func (s *Sync) HasAlbum(rg string) bool {
	_, _, ok := s.album(context.Background(), rg)
	return ok
}

// ArtistPictures answers a covered artist's picture records.
func (s *Sync) ArtistPictures(mbid string) (metadata.ArtistPictures, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.artists[strings.ToLower(mbid)]
	return e.pics, ok
}

// AlbumCover answers a listed album's cover record (nil when none stored).
func (s *Sync) AlbumCover(rg string) (*metadata.PictureRecord, bool) {
	_, cover, ok := s.album(context.Background(), rg)
	return cover, ok
}

// LocalArtist answers metadata.LocalMusic.
func (s *Sync) LocalArtist(ctx context.Context, mbid string) (metadata.Artist, bool) {
	if !s.Covers(mbid) {
		return metadata.Artist{}, false
	}
	p, ok := s.Package(ctx, mbid)
	if !ok {
		return metadata.Artist{}, false
	}
	a, err := p.PackageArtist()
	if err != nil {
		return metadata.Artist{}, false
	}
	// A package whose release groups are not built yet (none listed and
	// not marked whole) is no answer: the page asks the live path and the
	// server is asked to finish the package.
	if len(a.Albums) == 0 && (!p.ReleaseGroupsComplete || slices.Contains(p.Missing, "release_groups")) {
		s.want(mbid)
		return metadata.Artist{}, false
	}
	return a, true
}

// LocalPressings answers metadata.LocalMusic.
func (s *Sync) LocalPressings(ctx context.Context, rg string) ([]metadata.Pressing, bool, error) {
	artist, _, ok := s.album(ctx, rg)
	if !ok {
		return nil, false, nil
	}
	p, ok := s.Package(ctx, artist)
	if !ok {
		return nil, false, nil
	}
	return p.Pressings(rg)
}

// LocalAlbumArtist answers metadata.LocalMusic.
func (s *Sync) LocalAlbumArtist(ctx context.Context, rg string) (string, bool) {
	artist, _, ok := s.album(ctx, rg)
	return artist, ok
}

// Uncovered answers metadata.LocalMusic: once the copy is whole, an artist
// no package covers is wanted at the server (at most every wantAgain).
func (s *Sync) Uncovered(mbid string) {
	mbid = strings.ToLower(mbid)
	if !metadata.ValidMBID(mbid) || s.Covers(mbid) {
		return
	}
	s.want(mbid)
}

// want asks the server (once the copy is whole, at most every wantAgain)
// to build or finish the artist's package.
func (s *Sync) want(mbid string) {
	mbid = strings.ToLower(mbid)
	s.mu.RLock()
	synced := s.synced
	s.mu.RUnlock()
	if !synced {
		return
	}
	s.wantMu.Lock()
	defer s.wantMu.Unlock()
	if s.wanted == nil {
		s.wanted = map[string]time.Time{}
	}
	if at, ok := s.wanted[mbid]; ok && time.Since(at) < wantAgain {
		return
	}
	if s.wants == nil {
		return
	}
	select {
	case s.wants <- mbid:
		s.wanted[mbid] = time.Now()
	default:
	}
}

// startWants starts the one worker that sends package-wants.
func (s *Sync) startWants(ctx context.Context) {
	s.wantMu.Lock()
	if s.wants != nil {
		s.wantMu.Unlock()
		return
	}
	s.wants = make(chan string, wantQueue)
	ch := s.wants
	s.wantMu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-ch:
				err := s.Source.WantMusicPackage(ctx, id)
				if errors.Is(err, metadata.ErrServiceBusy) {
					// The server is busy: forget the ask so a later page asks again.
					s.wantMu.Lock()
					delete(s.wanted, id)
					s.wantMu.Unlock()
					t := time.NewTimer(time.Minute)
					select {
					case <-ctx.Done():
						t.Stop()
						return
					case <-t.C:
					}
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()
}

var _ metadata.LocalMusic = (*Sync)(nil)
