package music

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The New & Popular → Music lists are prebuilt by the Music read-ahead
// loop and kept in the lookup cache, so a visit (even right after a
// restart) answers at once from the last build. A build older than its
// TTL is served while a new one is made in the background.

const musicNewDiskKey = "musicnew|v1"

// musicPrebuildEvery is how often the read-ahead rebuilds Music's pages.
var musicPrebuildEvery = 3 * time.Hour

type musicNewDisk struct {
	At       time.Time                          `json:"at"`
	Releases []metadata.NewRelease              `json:"releases"`
	Fresh    metadata.MusicTrend                `json:"fresh"`
	Albums   metadata.MusicTrend                `json:"albums"`
	Artists  metadata.MusicTrend                `json:"artists"`
	Server   map[string][]metadata.MusicNewItem `json:"server"`
}

// loadMusicNew reads the last kept build once, when none is in memory.
func (h *Handlers) loadMusicNew() {
	if h.Store == nil {
		return
	}
	musicNew.mu.Lock()
	defer musicNew.mu.Unlock()
	if musicNew.data != nil {
		return
	}
	b, ok, err := metadata.LookupCache{Store: h.Store}.Get(context.Background(), musicNewDiskKey, time.Now())
	if err != nil || !ok {
		return
	}
	var v musicNewDisk
	if json.Unmarshal(b, &v) != nil || v.At.IsZero() {
		return
	}
	musicNew.data = &musicNewData{at: v.At, releases: v.Releases, fresh: v.Fresh, albums: v.Albums, artists: v.Artists, server: v.Server}
}

// refreshMusicNew builds the lists now, swaps them in and keeps them.
// It returns false when another build is already running.
func (h *Handlers) refreshMusicNew(ctx context.Context) bool {
	musicNew.mu.Lock()
	if musicNew.building {
		musicNew.mu.Unlock()
		return false
	}
	musicNew.building = true
	musicNew.mu.Unlock()
	now := time.Now().UTC()
	nd := h.buildMusicNew(ctx, now)
	musicNew.mu.Lock()
	musicNew.data, musicNew.building = nd, false
	musicNew.mu.Unlock()
	if h.Store != nil {
		b, err := json.Marshal(musicNewDisk{At: nd.at, Releases: nd.releases, Fresh: nd.fresh, Albums: nd.albums, Artists: nd.artists, Server: nd.server})
		if err == nil {
			// Kept long past the TTL: a stale build still answers at once.
			_ = metadata.LookupCache{Store: h.Store}.Put(context.WithoutCancel(ctx), musicNewDiskKey, b, now, now.Add(30*24*time.Hour))
		}
	}
	return true
}

// prebuildMusic builds New & Popular → Music, Explore Music's charts and
// every genre page, then fetches ahead the details and covers of what they
// show (music_ahead.go). One thing at a time, so upstream limits are kept.
func (h *Handlers) prebuildMusic(ctx context.Context) {
	h.loadMusicNew()
	h.refreshMusicNew(ctx)
	if ctx.Err() != nil {
		return
	}
	now := time.Now().UTC()
	region := "global"
	var art [][2]string
	add := func(kind, id string) {
		if id = strings.ToLower(id); metadata.ValidMBID(id) {
			art = append(art, [2]string{kind, id})
		}
	}
	for _, k := range []string{"albums", "songs"} {
		for _, it := range h.musicChartOf(ctx, region, k, now) {
			add("album", it.MBID)
			add("artist", it.ArtistMBID)
		}
	}
	if lib, err := h.musicLibrary(ctx, now); err == nil {
		if acts, err := h.musicActsOf(ctx, 0); err == nil {
			for _, g := range h.musicGenreTiles(ctx, region, now) {
				if ctx.Err() != nil {
					return
				}
				_, _, rows := h.buildMusicGenre(ctx, 0, region, g, lib, acts, now)
				for _, r := range rows {
					for _, c := range r.Items {
						if c.Kind == "album" {
							add("album", c.MBID)
						}
						add("artist", c.ArtistMBID)
					}
				}
			}
		}
	}
	musicNew.mu.Lock()
	d := musicNew.data
	musicNew.mu.Unlock()
	if d != nil {
		for _, r := range d.releases {
			add("album", r.MBID)
		}
		for _, l := range []metadata.MusicTrend{d.fresh, d.albums} {
			for _, it := range l.Items {
				add("album", it.MBID)
			}
		}
		for _, it := range d.artists.Items {
			add("artist", it.MBID)
		}
		for _, l := range d.server {
			for _, it := range l {
				add("album", it.MBID)
			}
		}
	}
	h.fetchAhead(ctx, aheadOf(art))
}
