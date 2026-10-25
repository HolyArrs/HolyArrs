package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// musicTrendTTL is how long a music trending list is kept; the metadata
// server changes it a few times a day at most.
const musicTrendTTL = time.Hour

// musicTrendMiss is how long a missing list or a failed read is remembered.
const musicTrendMiss = 5 * time.Minute

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
	// artMiss is when a picture was last not found, by "kind|mbid".
	artMiss map[string]time.Time
	// artBusy are pictures being fetched now, so a second ask waits for
	// the first instead of fetching again.
	artBusy map[string]*artFetch
	// warmed is when a card picture was last queued for prefetch, by
	// "kind|mbid", so a page shown again does not queue it again.
	warmed map[string]time.Time
	// prefetching counts background picture fetches (tests wait on it).
	prefetching sync.WaitGroup
}

type artFetch struct {
	done       chan struct{}
	name, path string
	ok         bool
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
	// Like every music row: the listed artists' and albums' art is warmed.
	warm := make([]nameKey, 0, 2*len(out))
	for _, v := range out {
		warm = append(warm, nameKey{kind: kind, mbid: v.MBID})
		if v.ArtistMBID != "" {
			warm = append(warm, nameKey{kind: "artist", mbid: v.ArtistMBID})
		}
	}
	a.warmArtItems(warm)
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
	// Each kept list expires at its own time (an hour, give or take 15 min).
	if ok && ((e.err == nil && time.Since(e.at) < spreadAt(musicTrendTTL, e.at, 0.25)) || (e.err != nil && time.Since(e.at) < musicTrendMiss)) {
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
	if len(c.names) > musicNamesMax {
		c.names = map[string]metadata.MusicTrendItem{}
	}
	for _, it := range l.Items {
		c.names[kind+"|"+it.MBID] = it
	}
	if err == nil && len(l.Items) > 0 {
		c.prefetching.Add(1)
		go func() {
			defer c.prefetching.Done()
			a.prefetchMusicArt(kind, l.Items)
		}()
	}
	return l, err
}

// prefetchMusicArt fetches a fresh list's pictures into the artwork cache
// in the background, in the sizes the cards ask for (albums 250 and 500,
// artists the default), two at a time.
func (a *API) prefetchMusicArt(kind string, items []metadata.MusicTrendItem) {
	if !a.partIsOn(parts.Music) || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	sizes := []string{"250"}
	if kind == "album" {
		sizes = []string{"250", "500"}
	}
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	// One item's sizes in turn: sizes can share a picture file.
	for _, it := range items {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(mbid string) {
			defer func() { <-sem; wg.Done() }()
			for _, size := range sizes {
				a.musicTrendArtFile(ctx, kind, mbid, size)
			}
		}(it.MBID)
	}
	wg.Wait()
}

// musicTrendArt answers GET /api/v1/music-trending/art/{kind}/{mbid}: the
// picture of an artist or album from a cached trending list, through the
// artwork cache. Artists are found by their MusicBrainz id;
// albums the Cover Art Archive's release group front cover.
func (a *API) musicTrendArt(w http.ResponseWriter, r *http.Request) {
	if a.kidMax(r) != "" {
		fail(w, http.StatusNotFound, "no picture here")
		return
	}
	kind, mbid := r.PathValue("kind"), strings.ToLower(r.PathValue("mbid"))
	name, p, ok := a.musicTrendArtFile(r.Context(), kind, mbid, coverSize(r))
	if !ok {
		// A library artist's saved picture stands in when no source has one.
		if fb := r.URL.Query().Get("fallback"); kind == "artist" && strings.HasPrefix(fb, "/api/v1/artwork/") && !strings.Contains(fb, "//") && !strings.Contains(fb, "..") {
			http.Redirect(w, r, fb, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	if kind == "artist" {
		// Artist photos used to be cached as immutable; they now revalidate
		// (the ETag is the file name), so a sharper photo replaces an old one.
		serveArtworkCached(w, r, name, p, "private, max-age=600, must-revalidate")
		return
	}
	serveArtwork(w, r, name, p)
}

// albumCover asks the metadata server's cover chain for an album, kept a
// week when found (memory and disk) and minutes (musicArtMissTTL) when no source has one. found is false when
// the server could not answer, so the caller tries the Cover Art Archive.
func (a *API) albumCover(ctx context.Context, mbid string, it metadata.MusicTrendItem) (src string, found bool) {
	if a.s.MusicCover == nil {
		return "", false
	}
	c := &a.music
	c.mu.Lock()
	e, ok := c.covers[mbid]
	c.mu.Unlock()
	if !ok && a.s.Store != nil {
		if b, found, _ := (store.LookupCache{Store: a.s.Store}).Get(ctx, "musiccover|"+mbid, time.Now()); found {
			var d musicCoverDisk
			if json.Unmarshal(b, &d) == nil && d.Src != "" {
				e, ok = musicCover{src: d.Src, at: d.At}, true
				c.mu.Lock()
				a.keepCoverLocked(mbid, e)
				c.mu.Unlock()
			}
		}
	}
	if ok && ((e.src != "" && time.Since(e.at) < musicCoverKeep) || (e.src == "" && time.Since(e.at) < musicArtMissTTL)) {
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
	if src == "" && (it.Artist == "" || title == "") {
		// Without names the server only asks the Cover Art Archive; keep
		// no answer, so a later ask with names runs the whole chain.
		return "", true
	}
	now := time.Now()
	c.mu.Lock()
	a.keepCoverLocked(mbid, musicCover{src: src, at: now})
	c.mu.Unlock()
	if src != "" && a.s.Store != nil {
		if b, err := json.Marshal(musicCoverDisk{Src: src, At: now}); err == nil {
			_ = store.LookupCache{Store: a.s.Store}.Put(context.WithoutCancel(ctx), "musiccover|"+mbid, b, now, now.Add(musicCoverKeep))
		}
	}
	return src, true
}

type musicCoverDisk struct {
	Src string    `json:"src"`
	At  time.Time `json:"at"`
}

func (a *API) keepCoverLocked(mbid string, e musicCover) {
	c := &a.music
	if c.covers == nil {
		c.covers = map[string]musicCover{}
	}
	if _, has := c.covers[mbid]; !has {
		keepUnder(c.covers, 20000, func(m musicCover) time.Time { return m.at })
	}
	c.covers[mbid] = e
}

// musicCoverKeep is how long a found album cover is used before it is
// asked again; found covers are kept on disk too, so a restart keeps them.
const musicCoverKeep = 7 * 24 * time.Hour

// musicArtMissTTL is how long a picture no source has is not asked again:
// minutes, so a brand-new release's cover shows once a source has it.
const musicArtMissTTL = 10 * time.Minute

// musicTrendArtFile finds and keeps the picture of a named artist or
// album. An album no cover source has shows its artist's picture, so the
// card is never blank. Misses are remembered a while (each costs outside
// calls).
func (a *API) musicTrendArtFile(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	c := &a.music
	key := kind + "|" + mbid + "|" + size
	c.mu.Lock()
	if f := c.artBusy[key]; f != nil {
		c.mu.Unlock()
		select {
		case <-f.done:
			if f.ok {
				return f.name, f.path, true
			}
		case <-ctx.Done():
			return "", "", false
		}
		return a.fetchMusicArt(ctx, kind, mbid, size)
	}
	if c.artBusy == nil {
		c.artBusy = map[string]*artFetch{}
	}
	f := &artFetch{done: make(chan struct{})}
	c.artBusy[key] = f
	c.mu.Unlock()
	f.name, f.path, f.ok = a.fetchMusicArt(ctx, kind, mbid, size)
	c.mu.Lock()
	delete(c.artBusy, key)
	c.mu.Unlock()
	close(f.done)
	return f.name, f.path, f.ok
}

// serverPlaceholder says a metadata server picture answer is only its
// stand-in for a photo it does not have yet.
func serverPlaceholder(h http.Header) bool {
	return h.Get("X-Metadata-Placeholder") != ""
}

// serverArtistPhotoURL is the metadata server's sharp, scored photo of an
// artist: 500 for circles and cards, 1200 when the big size is asked. A miss
// answers a placeholder, which keep refuses, so the other sources run.
func (a *API) serverArtistPhotoURL(_ context.Context, mbid, size string) string {
	if a.s.ImageBase == "" || !metadata.ValidMBID(mbid) {
		return ""
	}
	px := "500"
	if size == "1200" {
		px = "1200"
	}
	return strings.TrimRight(a.s.ImageBase, "/") + "/artist/" + mbid + "/" + px + ".jpg"
}

// serverAlbumCoverURL is the metadata server's stored cover of an album at
// the size asked (250, 500 or 1200).
func (a *API) serverAlbumCoverURL(mbid, size string) string {
	if a.s.ImageBase == "" || !metadata.ValidMBID(mbid) {
		return ""
	}
	return strings.TrimRight(a.s.ImageBase, "/") + "/album/" + mbid + "/" + size + ".jpg"
}

func (a *API) fetchMusicArt(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	c := &a.music
	c.mu.Lock()
	it, listed := c.names[kind+"|"+mbid]
	// An artist page asks for its own artist by id, listed or not; any other picture is only for a listed id.
	known := listed || kind == "artist" && metadata.ValidMBID(mbid)
	missAt, missed := c.artMiss[kind+"|"+mbid]
	c.mu.Unlock()
	if !listed {
		// Any catalog album or artist answers at once, listed by a page yet
		// or not (no race with the page that lists it).
		if b := a.musicPre.catalog.Load(); b != nil {
			if ci, ok := b.Lookup(kind, mbid); ok {
				it, known = metadata.MusicTrendItem{MBID: mbid, Name: ci.Title, Title: ci.Title, Artist: ci.Artist}, true
			}
		}
	}
	lib := a.s.Library
	if !known || lib == nil || lib.Artwork == nil || (missed && time.Since(missAt) < musicArtMissTTL) {
		return "", "", false
	}
	keep := func(src string) bool {
		if src == "" {
			return false
		}
		name = library.ArtworkName(src)
		if path, ok = lib.Artwork.Path(name); ok {
			lib.Artwork.Recheck(src, serverPlaceholder)
			return true
		}
		if _, err := lib.Artwork.FetchUnless(ctx, src, serverPlaceholder); err != nil {
			return false
		}
		path, ok = lib.Artwork.Path(name)
		return ok
	}
	artist := func(n string) bool {
		if lib.ArtistImages == nil || n == "" {
			return false
		}
		src, _ := lib.ArtistImages.Image(ctx, n)
		return keep(src)
	}
	switch kind {
	case "artist":
		// The artist is known by its id, never by its name alone (two
		// artists can share a name): the chart's own picture, fanart.tv's
		// thumb by MusicBrainz id, the Deezer artist and Wikidata picture
		// its MusicBrainz links name, then fanart.tv's wide photo (the
		// circle crops it). No album cover, and no picture beats a wrong one.
		if keep(a.serverArtistPhotoURL(ctx, mbid, size)) {
			return name, path, true
		}
		if keep(it.Image) {
			return name, path, true
		}
		var art metadata.ServerArt
		if a.s.ExtraArt != nil {
			art, _ = a.s.ExtraArt(ctx, "artist", mbid)
			if keep(art.Thumb) {
				return name, path, true
			}
			for _, th := range art.Thumbs {
				if keep(th) {
					return name, path, true
				}
			}
		}
		if lib.ArtistImages != nil && metadata.ValidMBID(mbid) {
			if keep(lib.ArtistImages.PictureByIDs(ctx, a.artistIDs(ctx, mbid))) {
				return name, path, true
			}
		}
		for _, bg := range art.Backgrounds {
			if keep(bg) {
				return name, path, true
			}
		}
	case "album":
		// The metadata server's stored copy first (fast, kept at the edge);
		// an album it has not stored yet answers with a redirect, and every
		// other source runs before the name tile: the chart's cover link,
		// the server's cover chain, the Cover Art Archive, then the artist's
		// picture.
		if src := a.serverAlbumCoverURL(mbid, size); src != "" {
			n := library.ArtworkName(src)
			if p, found := lib.Artwork.Path(n); found {
				lib.Artwork.Recheck(src, serverPlaceholder)
				return n, p, true
			}
			if _, err := lib.Artwork.FetchStored(ctx, src, serverPlaceholder); err == nil {
				if p, found := lib.Artwork.Path(n); found {
					return n, p, true
				}
			}
		}
		base := lib.CoverArtBase
		if base == "" {
			base = library.CoverArtBase
		}
		caa := strings.TrimRight(base, "/") + "/release-group/" + url.PathEscape(mbid) + "/front-250"
		tried := map[string]bool{}
		try := func(src string) bool {
			if src == "" {
				return false
			}
			src = sizedCover(src, size)
			if tried[src] {
				return false
			}
			tried[src] = true
			return keep(src)
		}
		if try(it.Cover) {
			return name, path, true
		}
		// A top song's release without a MusicBrainz id has only its own
		// cover link and the artist's picture.
		isMBID := metadata.ValidMBID(mbid)
		if isMBID {
			if src, found := a.albumCover(ctx, mbid, it); found && try(src) {
				return name, path, true
			}
		}
		if (isMBID && try(caa)) || artist(it.Artist) {
			return name, path, true
		}
	}
	if ctx.Err() == nil {
		c.mu.Lock()
		if c.artMiss == nil || len(c.artMiss) > 20000 {
			c.artMiss = map[string]time.Time{}
		}
		c.artMiss[kind+"|"+mbid] = time.Now()
		c.mu.Unlock()
	}
	return "", "", false
}

// coverSize reads the ?size= a cover is asked at: 250 (the default), 500
// for sharp tiles, 1200 for heroes.
func coverSize(r *http.Request) string {
	switch s := r.URL.Query().Get("size"); s {
	case "500", "1200":
		return s
	}
	return "250"
}

// caaThumb matches a Cover Art Archive picture with or without its
// thumbnail suffix.
var caaThumb = regexp.MustCompile(`^(https?://(?:www\.)?coverartarchive\.org/release(?:-group)?/[^/]+/(?:front|back|\d+))(?:-(?:250|500|1200))?(?:\.jpg)?$`)

// Deezer CDN and iTunes covers carry their size in the path.
var (
	dzThumb = regexp.MustCompile(`^(https?://[a-z0-9-]+\.dzcdn\.net/images/(?:cover|artist)/[0-9a-f]+/)\d+x\d+(-.*)?$`)
	itThumb = regexp.MustCompile(`^(https?://[a-z0-9-]+\.mzstatic\.com/image/thumb/.+/)\d+x\d+(bb|cc)?(\.(?:jpg|png|webp))$`)
)

// sizedCover points a Cover Art Archive, Deezer or iTunes cover at the size
// asked for; each size is its own artwork cache file. Other sources stay as they are.
func sizedCover(src, size string) string {
	if m := caaThumb.FindStringSubmatch(src); m != nil {
		return m[1] + "-" + size
	}
	if m := dzThumb.FindStringSubmatch(src); m != nil {
		return m[1] + size + "x" + size + m[2]
	}
	if m := itThumb.FindStringSubmatch(src); m != nil {
		bb := m[2]
		if bb == "" {
			bb = "bb"
		}
		return m[1] + size + "x" + size + bb + m[3]
	}
	return src
}