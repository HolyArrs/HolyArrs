package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
)

// musicTrendTTL is how long a music trending list is kept; the metadata
// server changes it a few times a day at most.
const musicTrendTTL = time.Hour

const musicTrendMax = 300

type musicTrendEntry struct {
	at   time.Time
	list metadata.MusicTrend
	err  error
}

type musicTrendCache struct {
	mu    sync.Mutex
	lists map[string]musicTrendEntry
	// names are the artists and albums of the cached lists by MusicBrainz
	// id, so only those pictures are fetched.
	names map[string]metadata.MusicTrendItem
	// covers are album cover URLs by release group ("" when none has one).
	covers map[string]musicCover
}

type musicCover struct {
	src string
	at  time.Time
}

// musicTrendView is one card of the row.
type musicTrendView struct {
	Rank       int    `json:"rank"`
	MBID       string `json:"mbid"`
	Name       string `json:"name"`
	Artist     string `json:"artist_name,omitempty"`
	ArtistMBID string `json:"artist_mbid,omitempty"`
	// Listens is this week's listen count, when the source has one.
	Listens int64 `json:"listen_count,omitempty"`
}

// musicTrending answers GET /api/v1/music-trending?kind=artist|album
// &region=global|<ISO>: the metadata server's weekly music trending list
// (ListenBrainz worldwide, Deezer charts per country).
func (a *API) musicTrending(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, region := q.Get("kind"), strings.ToUpper(q.Get("region"))
	if region == "" || region == "GLOBAL" {
		region = "global"
	}
	if (kind != "artist" && kind != "album") || (region != "global" && !isCountry(region)) {
		fail(w, http.StatusBadRequest, "kind must be artist or album, region global or a country code")
		return
	}
	// A Kid gets no list: it can hold explicit releases (like /explore/music).
	if a.kidMax(r) != "" {
		writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "items": []any{}})
		return
	}
	if p, _ := parts.ForTitleKind("artist"); a.s.MusicTrending == nil || !a.partIsOn(p) {
		fail(w, http.StatusNotFound, "no music trending list here")
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	list, err := a.cachedMusicTrend(r.Context(), kind, region)
	if errors.Is(err, metadata.ErrNotFound) {
		fail(w, http.StatusNotFound, "no music trending list for that region")
		return
	}
	if err != nil {
		fail(w, http.StatusBadGateway, "the metadata server did not answer; try again shortly")
		return
	}
	out := make([]musicTrendView, 0, len(list.Items))
	for _, it := range list.Items {
		v := musicTrendView{Rank: it.Rank, MBID: it.MBID, Name: it.Name, Listens: it.Listens}
		if kind == "album" {
			v.Name, v.Artist, v.ArtistMBID = it.Title, it.Artist, it.ArtistMBID
		}
		if v.MBID != "" && v.Name != "" {
			out = append(out, v)
		}
	}
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "region": list.Region, "source": list.Source, "regions": list.Regions, "items": out})
}

// cachedMusicTrend keeps lists an hour, and missing lists or failures five
// minutes; a failure is answered from an older good copy when there is one.
func (a *API) cachedMusicTrend(ctx context.Context, kind, region string) (metadata.MusicTrend, error) {
	key := kind + "|" + region
	c := &a.music
	c.mu.Lock()
	e, ok := c.lists[key]
	c.mu.Unlock()
	if ok && ((e.err == nil && time.Since(e.at) < musicTrendTTL) || (e.err != nil && time.Since(e.at) < top10Miss)) {
		return e.list, e.err
	}
	l, err := a.s.MusicTrending(ctx, kind, region)
	if err != nil && ok && e.err == nil && !errors.Is(err, metadata.ErrNotFound) {
		return e.list, nil
	}
	if err != nil && ctx.Err() != nil {
		return l, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lists == nil {
		c.lists, c.names = map[string]musicTrendEntry{}, map[string]metadata.MusicTrendItem{}
	}
	if _, has := c.lists[key]; !has && len(c.lists) >= musicTrendMax {
		oldest := ""
		for k, v := range c.lists {
			if oldest == "" || v.at.Before(c.lists[oldest].at) {
				oldest = k
			}
		}
		delete(c.lists, oldest)
	}
	c.lists[key] = musicTrendEntry{at: time.Now(), list: l, err: err}
	if len(c.names) > 20000 {
		c.names = map[string]metadata.MusicTrendItem{}
	}
	for _, it := range l.Items {
		c.names[kind+"|"+it.MBID] = it
	}
	return l, err
}

// musicTrendArt answers GET /api/v1/music-trending/art/{kind}/{mbid}: the
// picture of an artist or album from a cached trending list, through the
// artwork cache. Artists use the artist-picture chain's Deezer search;
// albums the Cover Art Archive's release group front cover.
func (a *API) musicTrendArt(w http.ResponseWriter, r *http.Request) {
	if a.kidMax(r) != "" {
		fail(w, http.StatusNotFound, "no picture here")
		return
	}
	kind, mbid := r.PathValue("kind"), strings.ToLower(r.PathValue("mbid"))
	c := &a.music
	c.mu.Lock()
	it, ok := c.names[kind+"|"+mbid]
	c.mu.Unlock()
	lib := a.s.Library
	if !ok || lib == nil || lib.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	src := ""
	switch kind {
	case "artist":
		if lib.ArtistImages != nil {
			src, _ = lib.ArtistImages.Image(r.Context(), it.Name)
		}
	case "album":
		var found bool
		if src, found = a.albumCover(r.Context(), mbid, it); !found {
			base := lib.CoverArtBase
			if base == "" {
				base = library.CoverArtBase
			}
			src = strings.TrimRight(base, "/") + "/release-group/" + url.PathEscape(mbid) + "/front-250"
		}
	}
	if src == "" {
		http.NotFound(w, r)
		return
	}
	name := library.ArtworkName(src)
	p, ok := lib.Artwork.Path(name)
	if !ok {
		if _, err := lib.Artwork.Fetch(r.Context(), src); err != nil {
			http.Error(w, "picture unavailable", http.StatusNotFound)
			return
		}
		if p, ok = lib.Artwork.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	serveArtwork(w, r, name, p)
}

// albumCover asks the metadata server's cover chain for an album, kept a
// month when found and a day when no source has one. found is false when
// the server could not answer, so the caller tries the Cover Art Archive.
func (a *API) albumCover(ctx context.Context, mbid string, it metadata.MusicTrendItem) (src string, found bool) {
	if a.s.MusicCover == nil {
		return "", false
	}
	c := &a.music
	c.mu.Lock()
	e, ok := c.covers[mbid]
	c.mu.Unlock()
	if ok && ((e.src != "" && time.Since(e.at) < 30*24*time.Hour) || (e.src == "" && time.Since(e.at) < 24*time.Hour)) {
		return e.src, true
	}
	title := it.Title
	if title == "" {
		title = it.Name
	}
	src, err := a.s.MusicCover(ctx, mbid, it.Artist, title)
	if err != nil && !errors.Is(err, metadata.ErrNotFound) {
		return "", false
	}
	c.mu.Lock()
	if c.covers == nil || len(c.covers) > 20000 {
		c.covers = map[string]musicCover{}
	}
	c.covers[mbid] = musicCover{src: src, at: time.Now()}
	c.mu.Unlock()
	return src, true
}
