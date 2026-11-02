package main

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

// openDiscog opens the discography store (every artist's full release
// groups, each album's chosen edition with tracks). Its walks run on the
// MusicBrainz queue, one call a step, inside the MusicBrainz budget and
// pace; it needs the speed system.
func openDiscog(c config, sp *speed, direct *providers.Client, upstream *budget.Budget, slim *mbread.Reader, o *api.Options) (*discog.Store, error) {
	if sp == nil {
		return nil, nil
	}
	var fromSlim discog.Slim
	if slim != nil {
		fromSlim = slim
		if sp.res != nil {
			sp.res.MBTitles = slim.GroupTitles
			sp.res.Homonyms = slim.HomonymIDs
			sp.res.TitleArtists = slim.TitleArtists
		}
	}
	d, err := discog.Open(filepath.Join(c.DataDir, "discog.sqlite"), discog.Options{
		Slim: fromSlim,
		Get:  direct.MusicBrainzGet,
		Take: func() bool { return upstream.Take("musicbrainz") },
		Stored: func(ctx context.Context, kind, mbid string) ([]byte, bool) {
			if sp.handler == nil {
				return nil, false
			}
			return sp.handler.StoredData(ctx, kind, mbid)
		}})
	if err != nil {
		return nil, err
	}
	sp.disc = d
	o.Discog = d
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		m["discography"] = map[string]any{"store": d.Stats(ctx), "prefetch": sp.discogLast()}
		return m
	}
	return d, nil
}

// Prefetch pacing: a pass every discogEvery queues at most discogPass
// walks; the queue runs them only when no one waits (P0 and P1 first).
var (
	discogEvery  = time.Hour
	discogPass   = 3000
	discogOpened = 30 * 24 * time.Hour
)

// discogSources are the prefetch's inputs, read from what the server
// already keeps (no upstream call).
type discogSources struct {
	kw      *kworb.Service
	tracker *refresh.Tracker
}

type discogPassStats struct {
	At            string         `json:"at"`
	Wanted        map[string]int `json:"wanted"`
	Fresh         int            `json:"fresh"`
	Queued        int            `json:"queued"`
	AlreadyQueued int            `json:"already_queued"`
	SkippedCap    int            `json:"skipped_over_cap"`
	Paused        bool           `json:"paused"`
}

var discogMu sync.Mutex

func (sp *speed) discogLast() discogPassStats {
	discogMu.Lock()
	defer discogMu.Unlock()
	return sp.discLast
}

// discogPrefetch keeps the full lists and album tracks ready for what the
// shared pages show: a pass five minutes after start, then hourly.
func (sp *speed) discogPrefetch(ctx context.Context, src discogSources) {
	t := time.NewTimer(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		sp.discogPassOnce(ctx, src)
		t.Reset(discogEvery)
	}
}

type discogWant struct {
	key string
	p   int
}

// discogWanted lines up the walks, most shown first: the rising row (P2);
// the shared lists' members and their related artists, the top 2,500 by
// listeners and the artists opened by any install in the last 30 days
// (P3); albums on the shared lists or opened in 30 days (P3); then stored
// answers the change feed marked or past their safety refresh (P4).
func (sp *speed) discogWanted(ctx context.Context, src discogSources) ([]discogWant, map[string]int) {
	var out []discogWant
	seen := map[string]bool{}
	counts := map[string]int{}
	add := func(list string, key string, p int) {
		counts[list]++
		if !seen[key] {
			seen[key] = true
			out = append(out, discogWant{key, p})
		}
	}
	if src.kw != nil {
		for _, id := range src.kw.RisingArtists() {
			add("rising", discog.ArtistKey(id), fetchq.P2Shared)
		}
	}
	if sp.fill != nil {
		for _, m := range sp.fill.Members() {
			switch m.Kind {
			case "artist":
				add("shared_artists", discog.ArtistKey(m.MBID), fetchq.P3Refresh)
			case "release-group":
				add("shared_albums", discog.AlbumKey(m.MBID), fetchq.P3Refresh)
			}
		}
	}
	if src.kw != nil {
		for _, id := range src.kw.TopArtists(2500) {
			add("top_listeners", discog.ArtistKey(id), fetchq.P3Refresh)
		}
		for _, id := range src.kw.OpenedArtists(discogOpened) {
			add("opened_artists", discog.ArtistKey(id), fetchq.P3Refresh)
		}
	}
	if src.tracker != nil {
		items, _ := src.tracker.Popular(ctx, "musicbrainz:", time.Now().Add(-discogOpened), 50000)
		for _, it := range items {
			var q struct{ Kind, ID string }
			if json.Unmarshal([]byte(it.Query), &q) == nil && q.Kind == "release-group" {
				add("opened_albums", discog.AlbumKey(q.ID), fetchq.P3Refresh)
			}
		}
	}
	artists, albums := sp.disc.Due(ctx, 1000)
	for _, id := range artists {
		add("due", discog.ArtistKey(id), fetchq.P4Deep)
	}
	for _, id := range albums {
		add("due", discog.AlbumKey(id), fetchq.P4Deep)
	}
	return out, counts
}

func (sp *speed) discogPassOnce(ctx context.Context, src discogSources) {
	if sp.disc == nil {
		return
	}
	st := discogPassStats{At: time.Now().UTC().Format(time.RFC3339)}
	if paused, until := sp.disc.Paused(); paused {
		st.Paused = true
		log.Printf("discog prefetch: skipped, musicbrainz paused until %s", until.UTC().Format(time.RFC3339))
		discogMu.Lock()
		sp.discLast = st
		discogMu.Unlock()
		return
	}
	wants, counts := sp.discogWanted(ctx, src)
	st.Wanted = counts
	for _, w := range wants {
		if ctx.Err() != nil {
			return
		}
		id := w.key[strings.LastIndex(w.key, ":")+1:]
		if !discog.ValidMBID(id) {
			continue
		}
		var need bool
		if strings.HasPrefix(w.key, discog.ArtistPrefix) {
			need = sp.disc.NeedsArtist(ctx, id)
		} else {
			need = sp.disc.NeedsAlbum(ctx, id)
		}
		if !need {
			st.Fresh++
			continue
		}
		if s, at := sp.mbq.State(ctx, w.key); s == "queued" || s == "running" || s == "dead" && time.Since(at) < 24*time.Hour {
			st.AlreadyQueued++
			continue
		}
		if st.Queued >= discogPass {
			st.SkippedCap++
			continue
		}
		sp.mbq.Enqueue(ctx, w.key, w.p, false)
		st.Queued++
	}
	log.Printf("discog prefetch: wanted %v; fresh %d, queued %d, already queued %d, skipped over cap %d", counts, st.Fresh, st.Queued, st.AlreadyQueued, st.SkippedCap)
	discogMu.Lock()
	sp.discLast = st
	discogMu.Unlock()
}
