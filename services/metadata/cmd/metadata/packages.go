package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicpkg"
)

// Package pass pacing and depth.
var (
	pkgEvery   = time.Hour
	pkgFirst   = 10 * time.Minute
	pkgQueue   = 3000           // walks queued a pass at most
	pkgRebuild = 24 * time.Hour // a complete package is rebuilt daily
	pkgTop     = 1000           // the most popular get full depth and P3
	pkgFull    = musicpkg.Depth{Max: 400}
	pkgLight   = musicpkg.Depth{Max: 60, Kinds: map[string]bool{"album": true, "ep": true}}
)

// pkgCoverage is one covered artist with where it came from.
type pkgCoverage struct {
	mbid   string
	source string
}

// pkgPass is the last pass's numbers (also logged).
type pkgPass struct {
	At       string         `json:"at"`
	Covered  int            `json:"covered"`
	Sources  map[string]int `json:"sources"`
	Built    int            `json:"built"`
	Changed  int            `json:"changed"`
	Fresh    int            `json:"fresh"`
	Complete int            `json:"complete"`
	Partial  int            `json:"partial"`
	NoArtist int            `json:"no_artist_yet"`
	Queued   map[string]int `json:"queued"`
	OverCap  int            `json:"skipped_over_cap"`
	Dropped  int            `json:"wanted_dropped"`
	Paused   bool           `json:"paused,omitempty"`
	TookMS   int64          `json:"took_ms"`
}

type packager struct {
	sp    *speed
	store *musicpkg.Store
	kw    *kworb.Service
	// src reads the stored parts (set by openPackages; tests replace it).
	src  musicpkg.Sources
	mu   sync.Mutex
	last pkgPass
}

// openPackages opens the package store and serves it (sync, one package,
// asks). Nothing without the speed system (it needs the queue and stores).
func openPackages(c config, sp *speed, o *api.Options) (*packager, error) {
	if sp == nil || sp.disc == nil {
		return nil, nil
	}
	st, err := musicpkg.Open(filepath.Join(c.DataDir, "packages.sqlite"))
	if err != nil {
		return nil, err
	}
	pk := &packager{sp: sp, store: st, src: musicpkg.Sources{
		Artist: func(ctx context.Context, id string) ([]byte, bool) {
			if sp.handler == nil {
				return nil, false
			}
			return sp.handler.StoredData(ctx, "artist", id)
		},
		Discog:   sp.disc.Artist,
		Album:    sp.disc.Album,
		Pictures: sp.pictures,
	}}
	o.Packages = st
	o.WantPackage = pk.want
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["packages"] = map[string]any{"store": st.Counts(ctx), "pass": pk.lastPass()}
		return m
	}
	return pk, nil
}

func (pk *packager) Close() error { return pk.store.Close() }

func (pk *packager) lastPass() pkgPass {
	pk.mu.Lock()
	defer pk.mu.Unlock()
	return pk.last
}

// pkgWantPerPass caps the asks a pass handles (oldest first).
var pkgWantPerPass = 200

// want records an ask (the id only) and starts its walks below the shared
// lists (P3), skipping walks already queued, running or recently dead.
func (pk *packager) want(ctx context.Context, mbid string) error {
	mbid = strings.ToLower(mbid)
	if !musicpkg.ValidMBID(mbid) {
		return errors.New("invalid mbid")
	}
	if err := pk.store.Want(ctx, mbid, time.Now()); err != nil {
		return err
	}
	for _, key := range []string{api.QueueKey("artist", mbid), discog.ArtistKey(mbid)} {
		if s, at := pk.sp.mbq.State(ctx, key); s == "queued" || s == "running" || s == "dead" && time.Since(at) < 24*time.Hour {
			continue
		}
		pk.sp.mbq.Enqueue(ctx, key, fetchq.P3Refresh, false)
	}
	return nil
}

// run makes a pass pkgFirst after start, then every pkgEvery.
func (pk *packager) run(ctx context.Context) {
	t := time.NewTimer(pkgFirst)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		pk.once(ctx)
		t.Reset(pkgEvery)
	}
}

// ranked reports an artist among the top listeners.
func (pk *packager) ranked(mbid string) bool {
	if pk.kw == nil {
		return false
	}
	for _, id := range pk.kw.TopArtists(pkgTop) {
		if id == mbid {
			return true
		}
	}
	return false
}

// coverage lines up the covered artists, most wanted first: the rising
// row, artists an app server asked for, every artist on a shared list
// (charts per region, genre tops, trending, new and coming, the list
// source's lists) and their similar artists one hop out, most streamed
// first, then the top artists by listeners. Read from stored data only.
func (pk *packager) coverage(ctx context.Context) []pkgCoverage {
	var out []pkgCoverage
	seen := map[string]bool{}
	add := func(id, source string) {
		if musicpkg.ValidMBID(id) && !seen[id] {
			seen[id] = true
			out = append(out, pkgCoverage{id, source})
		}
	}
	rank := map[string]int{}
	var top []string
	if pk.kw != nil {
		for _, id := range pk.kw.RisingArtists() {
			add(id, "rising")
		}
		top = pk.kw.TopArtists(3000)
		for i, id := range top {
			rank[id] = i
		}
	}
	for i, id := range pk.store.Wanted(ctx, time.Now()) {
		if i >= pkgWantPerPass {
			break
		}
		add(id, "wanted")
	}
	if pk.sp.fill != nil {
		var listed []string
		for _, m := range pk.sp.fill.Members() {
			if m.Kind == "artist" {
				listed = append(listed, m.MBID)
			}
		}
		sort.Slice(listed, func(i, j int) bool {
			ri, oki := rank[listed[i]]
			rj, okj := rank[listed[j]]
			if oki != okj {
				return oki
			}
			if oki && ri != rj {
				return ri < rj
			}
			return listed[i] < listed[j]
		})
		for _, id := range listed {
			add(id, "lists_and_similar")
		}
	}
	for i, id := range top {
		if i >= 2500 {
			break
		}
		add(id, "top_listeners")
	}
	return out
}

// once makes one pass: queue what each covered artist's package still
// lacks, then build its package from what is stored.
func (pk *packager) once(ctx context.Context) {
	started := time.Now()
	st := pkgPass{At: started.UTC().Format(time.RFC3339), Sources: map[string]int{}, Queued: map[string]int{}}
	sp := pk.sp
	paused, _ := sp.disc.Paused()
	st.Paused = paused
	queued := 0
	enqueue := func(kind, key string, p int) {
		if paused {
			return
		}
		if s, at := sp.mbq.State(ctx, key); s == "queued" || s == "running" || s == "dead" && time.Since(at) < 24*time.Hour {
			return
		}
		if queued >= pkgQueue {
			st.OverCap++
			return
		}
		sp.mbq.Enqueue(ctx, key, p, false)
		queued++
		st.Queued[kind]++
	}
	src := pk.src
	// First, the tracklists of the albums the shared lists show.
	if sp.fill != nil {
		for _, m := range sp.fill.Members() {
			if m.Kind == "release-group" && sp.disc.NeedsAlbum(ctx, m.MBID) {
				enqueue("listed_tracklists", discog.AlbumKey(m.MBID), fetchq.P2Shared)
			}
		}
	}
	cover := pk.coverage(ctx)
	st.Covered = len(cover)
	for i, c := range cover {
		if ctx.Err() != nil {
			return
		}
		st.Sources[c.source]++
		p, depth := fetchq.P4Deep, pkgLight
		if c.source == "lists_and_similar" {
			// What users see: ahead of the top-listener long tail.
			p = fetchq.P2Shared
		}
		// Asks get full depth only when the artist ranks in the top set;
		// otherwise light depth at idle priority.
		if i < pkgTop && c.source != "wanted" || c.source == "wanted" && pk.ranked(c.mbid) {
			p, depth = fetchq.P3Refresh, pkgFull
		}
		if c.source == "wanted" {
			if s, _ := sp.mbq.State(ctx, api.QueueKey("artist", c.mbid)); s == "dead" {
				pk.store.Drop(ctx, c.mbid)
				st.Dropped++
				continue
			}
		}
		// A complete package built today costs nothing (the discography
		// prefetch refreshes what the change feed marks).
		if m, ok := pk.store.Meta(ctx, c.mbid); ok && m.Complete && time.Since(m.Built) < pkgRebuild {
			st.Fresh++
			st.Complete++
			continue
		}
		if _, ok := src.Artist(ctx, c.mbid); !ok {
			enqueue("artist", api.QueueKey("artist", c.mbid), p)
		}
		if sp.disc.NeedsArtist(ctx, c.mbid) {
			enqueue("release_groups", discog.ArtistKey(c.mbid), p)
		}
		if a, ok := sp.disc.Artist(ctx, c.mbid); ok {
			for _, rg := range musicpkg.Pick(a.Groups, depth) {
				if sp.disc.NeedsAlbum(ctx, rg) {
					enqueue("tracklists", discog.AlbumKey(rg), p)
				}
			}
		}
		pkg, ok := musicpkg.Build(ctx, src, c.mbid, i+1, depth, time.Now())
		if !ok {
			st.NoArtist++
			continue
		}
		st.Built++
		if pkg.Complete {
			st.Complete++
		} else {
			st.Partial++
		}
		changed, err := pk.store.Put(ctx, pkg)
		if err != nil {
			log.Printf("packages: store %s: %v", c.mbid, err)
			continue
		}
		if changed {
			st.Changed++
		}
	}
	st.TookMS = time.Since(started).Milliseconds()
	counts := pk.store.Counts(ctx)
	log.Printf("packages: covered %d %v; built %d (changed %d), fresh %d; complete %d, partial %d, no artist answer yet %d; queued %v, over cap %d, asks dropped %d; stored %d (%d complete), asks %d, version %d; %d ms",
		st.Covered, st.Sources, st.Built, st.Changed, st.Fresh, st.Complete, st.Partial, st.NoArtist, st.Queued, st.OverCap, st.Dropped,
		counts.Packages, counts.Complete, counts.Wanted, counts.Latest, st.TookMS)
	pk.mu.Lock()
	pk.last = st
	pk.mu.Unlock()
}
