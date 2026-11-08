// PackageSync keeps a local copy of the metadata server's artist
// packages and their pictures, refreshed on a fixed schedule. Music pages
// answer from this copy instead of waiting on the server. The copy sends no
// credentials and nothing about the install. Only an artist that a page asked
// for and no package covers is named to the server, so it gets built.

package music

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	// DefaultEvery is how often the copy runs.
	DefaultEvery = 30 * time.Minute
	// LowMemoryEvery is the slower schedule in low-memory mode: fewer runs, each
	// catching up on more packages.
	LowMemoryEvery = 3 * time.Hour

	// pageLimit is the most packages the server returns in one page.
	pageLimit = 200
	// pageGap spaces pages out so the server is not hammered.
	pageGap = time.Second
	// bigRank is the lowest-ranked artist that still gets the 1200 photo.
	bigRank = 200
	// picWorkers is how many pictures download at once.
	picWorkers = 4
	// wantAgain is how long an artist is left alone after it was asked for.
	wantAgain = 6 * time.Hour
	// wantQueue is how many asks can wait for the sender.
	wantQueue = 256
	// cacheMax is how many decoded packages stay in memory.
	cacheMax = 64
	// maxPackageBytes caps a package after unzipping, so a bad body cannot
	// fill memory.
	maxPackageBytes = 64 << 20
)

// picGap spaces one worker's pictures out. Tests set it to zero.
var picGap = 50 * time.Millisecond

var _ metadata.LocalMusic = (*PackageSync)(nil)

// PackageSync is the local package store and the job that fills it.
type PackageSync struct {
	Store    *database.Store
	Source   Source
	Pictures Pictures
	// Every is the schedule. Zero means the default for the current mode.
	Every time.Duration
	// OnSynced runs after each copy that changed packages.
	OnSynced func(ctx context.Context)

	mu sync.RWMutex
	// artists is every covered artist, in memory because each artist check
	// reads it. Albums stay in music_package_albums and are read by key, which
	// keeps about 110 MB out of memory.
	artists map[string]artistEntry
	albums  int
	loaded  bool
	// synced is true once a whole copy reached the server's latest. Only then
	// is a missing artist really not covered.
	synced bool
	stats  Stats

	cacheMu sync.Mutex
	cache   map[string]*metadata.MusicPackage
	order   []string

	wantMu sync.Mutex
	wanted map[string]time.Time
	wants  chan string
}

// Source is the metadata server's package list.
type Source interface {
	MusicPackages(ctx context.Context, since int64, limit int) (metadata.PackagePage, error)
	WantMusicPackage(ctx context.Context, mbid string) error
}

// Pictures stores one picture in the local picture store and reports whether
// it worked. Kind is artist or album, size is 500 or 1200, v is the version.
type Pictures func(ctx context.Context, kind, mbid, size, v string) bool

// Stats describes the local copy.
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

type artistEntry struct {
	rank int
	pics metadata.ArtistPictures
}

type picJob struct{ kind, mbid, size, v string }

// Load reads the artist index and picture records from disk.
func (s *PackageSync) Load(ctx context.Context) error {
	arts := map[string]artistEntry{}
	db := s.Store.Read()
	index, err := readPackageIndex(ctx, db)
	if err != nil {
		return err
	}
	for _, r := range index {
		e := artistEntry{rank: r.rank}
		_ = json.Unmarshal([]byte(r.pictures), &e.pics)
		arts[r.mbid] = e
	}
	albs := s.albumCount(ctx)
	cursor, at := readPackageSync(ctx, db)
	size := readPackageBytes(ctx, db)
	s.mu.Lock()
	s.artists, s.albums, s.loaded = arts, albs, true
	// An earlier copy (cursor past 0) answers coverage until this one ends.
	s.synced = s.synced || cursor > 0
	s.stats.Packages, s.stats.Albums, s.stats.Bytes, s.stats.Cursor = len(arts), albs, size, cursor
	if t, err := time.Parse(database.TimeLayout, at); err == nil {
		s.stats.LastSync = t
	}
	s.mu.Unlock()
	return nil
}

// Stats returns the state of the copy.
func (s *PackageSync) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Run loads the index, copies at once, then copies on the schedule until ctx ends.
func (s *PackageSync) Run(ctx context.Context) {
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
// stores the pictures of the changed packages. It returns how many changed.
func (s *PackageSync) SyncOnce(ctx context.Context) (int, error) {
	start := time.Now()
	s.mu.RLock()
	loaded := s.loaded
	s.mu.RUnlock()
	if !loaded {
		if err := s.Load(ctx); err != nil {
			return 0, err
		}
	}
	cursor := readPackageCursor(ctx, s.Store.Read())
	// Keep only the picture jobs until the copy ends, not the decoded packages:
	// a first copy would otherwise hold every package at once.
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

// Package reads one artist's package from disk. Decoded packages stay in a
// small cache.
func (s *PackageSync) Package(ctx context.Context, mbid string) (*metadata.MusicPackage, bool) {
	mbid = strings.ToLower(mbid)
	s.cacheMu.Lock()
	if p, ok := s.cache[mbid]; ok {
		s.cacheMu.Unlock()
		return p, true
	}
	s.cacheMu.Unlock()
	body, err := readPackageBody(ctx, s.Store.Read(), mbid)
	if err != nil {
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

// Covers reports whether a package covers the artist.
func (s *PackageSync) Covers(mbid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.artists[strings.ToLower(mbid)]
	return ok
}

// HasAlbum reports whether a package lists the release group.
func (s *PackageSync) HasAlbum(rg string) bool {
	_, _, ok := s.album(context.Background(), rg)
	return ok
}

// ArtistPictures returns a covered artist's picture records.
func (s *PackageSync) ArtistPictures(mbid string) (metadata.ArtistPictures, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.artists[strings.ToLower(mbid)]
	return e.pics, ok
}

// AlbumCover returns a listed album's cover record. The record is nil when
// none is stored.
func (s *PackageSync) AlbumCover(rg string) (*metadata.PictureRecord, bool) {
	_, cover, ok := s.album(context.Background(), rg)
	return cover, ok
}

// LocalArtist returns the artist from its package. It implements metadata.LocalMusic.
func (s *PackageSync) LocalArtist(ctx context.Context, mbid string) (metadata.Artist, bool) {
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
	// A package with no release groups built yet is no answer. The page uses
	// the live path and the server is asked to finish the package.
	if len(a.Albums) == 0 && (!p.ReleaseGroupsComplete || slices.Contains(p.Missing, "release_groups")) {
		s.want(mbid)
		return metadata.Artist{}, false
	}
	return a, true
}

// LocalPressings returns the pressings of a release group from its package.
// It implements metadata.LocalMusic.
func (s *PackageSync) LocalPressings(ctx context.Context, rg string) ([]metadata.Pressing, bool, error) {
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

// LocalAlbumArtist returns the artist that owns the release group. It
// implements metadata.LocalMusic.
func (s *PackageSync) LocalAlbumArtist(ctx context.Context, rg string) (string, bool) {
	artist, _, ok := s.album(ctx, rg)
	return artist, ok
}

// Uncovered asks the server to build the artist's package when no package
// covers it. It implements metadata.LocalMusic.
func (s *PackageSync) Uncovered(mbid string) {
	mbid = strings.ToLower(mbid)
	if !metadata.ValidMBID(mbid) || s.Covers(mbid) {
		return
	}
	s.want(mbid)
}

// schedule returns Every, else the default for the current mode.
func (s *PackageSync) schedule() time.Duration {
	switch {
	case s.Every > 0:
		return s.Every
	case startup.LowMemoryOn():
		return LowMemoryEvery
	}
	return DefaultEvery
}

// finish records the result of a copy run in the stats.
func (s *PackageSync) finish(start time.Time, cursor int64, changed int, whole bool) {
	size := readPackageBytes(context.Background(), s.Store.Read())
	albs := s.albumCount(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.synced = s.synced || whole
	s.albums = albs
	s.stats.Packages, s.stats.Albums, s.stats.Bytes, s.stats.Cursor = len(s.artists), albs, size, cursor
	s.stats.LastSync, s.stats.Took, s.stats.Changed = time.Now().UTC(), time.Since(start), changed
}

// storePage writes one page's valid packages and the cursor in one write.
// It returns how many packages it stored and their picture jobs.
func (s *PackageSync) storePage(ctx context.Context, page metadata.PackagePage) (int, []picJob, error) {
	type row struct {
		p    *metadata.MusicPackage
		body []byte
		pics []byte
	}
	var rows []row
	// One writer serves the whole page.
	z, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
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
	now := database.FormatTime(time.Now())
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			var albums []packageAlbum
			for _, g := range r.p.ReleaseGroups {
				cover := ""
				if g.Cover != nil {
					b, _ := json.Marshal(g.Cover)
					cover = string(b)
				}
				albums = append(albums, packageAlbum{rg: strings.ToLower(g.ID), cover: cover})
			}
			p := packageRow{mbid: r.p.MBID, version: r.p.Version, hash: r.p.Hash, rank: r.p.Rank, complete: r.p.Complete, body: r.body, pictures: string(r.pics)}
			if err := putPackageTx(tx, p, albums, now); err != nil {
				return err
			}
		}
		err := putPackageCursorTx(tx, page.Next, now)
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

// artistPictures picks the artist picture records out of a package.
func artistPictures(p *metadata.MusicPackage) metadata.ArtistPictures {
	return metadata.ArtistPictures{Photo: p.Pictures["photo"], Wide: p.Pictures["wide"], Logo: p.Pictures["logo"]}
}

// pack gzips raw with z, which is reset for each package.
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

// unpack unzips a stored package body.
func unpack(body []byte) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	return io.ReadAll(io.LimitReader(z, maxPackageBytes))
}

// picJobs lists the pictures to store for a package: the 500 photo, the 1200
// photo for top-ranked artists, and the 500 album covers.
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

// storePictures stores the pictures through Pictures, a few at a time.
func (s *PackageSync) storePictures(ctx context.Context, jobs []picJob) {
	if s.Pictures == nil || len(jobs) == 0 {
		return
	}
	ch := make(chan picJob)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, missed := 0, 0
	// Low-memory mode runs fewer workers.
	for range min(startup.LowMemoryPick(picWorkers, 2), len(jobs)) {
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

// album reads one listed album by key. A read error counts as not listed, so
// callers fall back to the live path.
func (s *PackageSync) album(ctx context.Context, rg string) (artist string, cover *metadata.PictureRecord, ok bool) {
	artist, raw, err := readPackageAlbum(ctx, s.Store.Read(), strings.ToLower(rg))
	if err != nil {
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

// albumCount returns how many albums the packages list.
func (s *PackageSync) albumCount(ctx context.Context) int {
	return countPackageAlbums(ctx, s.Store.Read())
}

// want asks the server to build or finish the artist's package. It does
// nothing until the copy is whole, and asks at most once per wantAgain.
func (s *PackageSync) want(mbid string) {
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

// startWants starts the one worker that sends the asks to the server.
func (s *PackageSync) startWants(ctx context.Context) {
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
					// Forget the ask so a later page asks again, and back off.
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
