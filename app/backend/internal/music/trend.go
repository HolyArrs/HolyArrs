package music

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
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
	// artMiss is when a picture was last not found and how many times in a
	// row, by "kind|mbid" (artMissWait spaces the retries).
	artMiss map[string]artMissAt
	// artQueued are pictures the route asked to be fetched behind its
	// answer, by "kind|mbid|size|v".
	artQueued map[string]bool
	// coreMiss is when an album core last answered 404, by "album|artist".
	coreMiss map[string]time.Time
	// artBusy are pictures being fetched now, so a second ask waits for
	// the first instead of fetching again.
	artBusy map[string]*artFetch
	// warmed is when a card picture was last queued for prefetch, by
	// "kind|mbid", so a page shown again does not queue it again.
	warmed map[string]time.Time
	// prefetching counts background picture fetches (tests wait on it).
	prefetching sync.WaitGroup
}

type artMissAt struct {
	at time.Time
	n  int
}

type artFetch struct {
	done chan struct{}
	file artFile
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
func (h *Handlers) musicTrending(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, region := q.Get("kind"), strings.ToUpper(q.Get("region"))
	if region == "" || region == "GLOBAL" {
		region = "global"
	}
	if (kind != "artist" && kind != "album") || (region != "global" && !isCountry(region)) {
		webserver.Fail(w, http.StatusBadRequest, "kind must be artist or album, region global or a country code")
		return
	}
	// A Kid gets no list: it can hold explicit releases (like /explore/music).
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"kind": kind, "items": []any{}})
		return
	}
	if p, _ := parts.ForTitleKind("artist"); h.MusicTrending == nil || !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "no music trending list here")
		return
	}
	if !h.Limit(h.Person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	list, err := h.cachedMusicTrend(r.Context(), kind, region)
	if errors.Is(err, metadata.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no music trending list for that region")
		return
	}
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, "the metadata server did not answer; try again shortly")
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
	h.warmArtItems(warm)
	h.ExploreCaching(w, r, "private, max-age=300")
	paths := make([]string, 0, len(warm))
	for _, k := range warm {
		paths = append(paths, musicArtPrefix+k.kind+"/"+k.mbid)
	}
	h.WriteMusicJSON(w, r, map[string]any{"kind": kind, "region": list.Region, "source": list.Source, "regions": list.Regions, "items": out}, paths...)
}

// cachedMusicTrend keeps lists an hour, and missing lists or failures five
// minutes; a failure is answered from an older good copy when there is one.
func (h *Handlers) cachedMusicTrend(ctx context.Context, kind, region string) (metadata.MusicTrend, error) {
	key := kind + "|" + region
	c := &h.music
	c.mu.Lock()
	e, ok := c.lists[key]
	c.mu.Unlock()
	// Each kept list expires at its own time (an hour, give or take 15 min).
	if ok && ((e.err == nil && time.Since(e.at) < spreadAt(musicTrendTTL, e.at, 0.25)) || (e.err != nil && time.Since(e.at) < musicTrendMiss)) {
		return e.list, e.err
	}
	l, err := h.MusicTrending(ctx, kind, region)
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
			h.prefetchMusicArt(kind, l.Items)
		}()
	}
	return l, err
}

// prefetchMusicArt fetches a fresh list's pictures into the artwork cache
// in the background, at the card size (500), two at a time, after their
// records (one batch call), so the versioned copies the cards ask for are
// the ones stored.
func (h *Handlers) prefetchMusicArt(kind string, items []metadata.MusicTrendItem) {
	if !h.PartIsOn(parts.Music) || h.lib() == nil || h.lib().Artwork == nil {
		return
	}
	// Nobody waits on a prefetch: marked ahead, never urgent at the server.
	ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), 15*time.Minute)
	defer cancel()
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.MBID)
	}
	if kind == "album" {
		h.PictureRecords(ctx, nil, ids, 5*time.Second)
	} else {
		h.PictureRecords(ctx, ids, nil, 5*time.Second)
	}
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, it := range items {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(mbid string) {
			defer func() { <-sem; wg.Done() }()
			h.musicTrendArtFile(ctx, kind, mbid, "500")
		}(it.MBID)
	}
	wg.Wait()
}

// musicTrendArt answers GET /api/v1/music-trending/art/{kind}/{mbid}
// [?size=][&pv=]: an album cover, artist photo, wide background or logo,
// through the artwork cache, only from the metadata server's store. size
// is the slot (250, 500 or 1200 for covers and photos; 960 or 1920 for
// wide); pv is the stored version from the answer's picture records, which
// makes the answer keep a year. 250 is made here from the 500.
func (h *Handlers) musicTrendArt(w http.ResponseWriter, r *http.Request) {
	if h.KidMax(r) != "" {
		NoPicture(w)
		return
	}
	kind, mbid := r.PathValue("kind"), strings.ToLower(r.PathValue("mbid"))
	q := r.URL.Query()
	if q.Get("fresh") != "" {
		// An open page asks again for a picture the server was still
		// storing: the remembered miss does not hold it back.
		h.music.mu.Lock()
		delete(h.music.artMiss, kind+"|"+mbid)
		h.music.mu.Unlock()
	}
	v := q.Get("pv")
	if !metadata.ValidPictureVersion(v) {
		v = ""
	}
	// Only what is on disk answers: a picture not stored yet is fetched
	// behind the answer, never while the visitor waits.
	f := h.fetchMusicArt(r.Context(), kind, mbid, ArtSize(kind, q.Get("size")), v, false)
	queued := false
	if !f.ok || !f.exact {
		queued = h.queueMusicArt(kind, mbid, q.Get("size"), v)
	}
	if f.ok {
		h.tplMarkArt(kind, mbid)
	}
	if !f.ok {
		// A library artist's saved picture stands in when no source has one.
		if fb := q.Get("fallback"); kind == "artist" && strings.HasPrefix(fb, "/api/v1/artwork/") && !strings.Contains(fb, "//") && !strings.Contains(fb, "..") {
			http.Redirect(w, r, fb, http.StatusFound)
			return
		}
		// No picture anywhere: an empty answer. The page's image falls back to its
		// placeholder the same way, but the browser does not log it as a failed load.
		if queued {
			// Being fetched now: worth asking again soon.
			w.Header().Set("Cache-Control", "private, max-age=20")
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		NoPicture(w)
		return
	}
	switch {
	case !f.exact:
		// A smaller size stands in until the asked one is made: asked again soon.
		serveArtworkCached(w, r, f.name, f.path, "private, max-age=60")
	case v != "" && f.v == v:
		serveArtworkCached(w, r, f.name, f.path, "private, max-age=31536000, immutable")
	case kind == "artist":
		// An unversioned photo revalidates (the ETag is the file name), so a
		// sharper photo replaces an old one.
		serveArtworkCached(w, r, f.name, f.path, "private, max-age=600, must-revalidate")
	case f.small:
		serveArtworkCached(w, r, f.name, f.path, "private, max-age=86400")
	default:
		serveArtwork(w, r, f.name, f.path)
	}
}

// musicArtMissTTL is how long a picture no source has is first not asked
// again: minutes, so a brand-new release's cover shows once a source has it.
const musicArtMissTTL = 10 * time.Minute

// artMissSteps space the retries of a picture missed again and again: 10
// minutes, then 30, 2 hours, and 6 hours from then on.
var artMissSteps = []time.Duration{musicArtMissTTL, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}

// artMissWait is how long a picture missed n times in a row is not asked.
func artMissWait(n int) time.Duration {
	return artMissSteps[min(max(n, 1), len(artMissSteps))-1]
}

const (
	// artQueueMax caps the pictures waiting to be fetched behind answers.
	artQueueMax = 2000
	// artQueueWorkers fetch them, a few at a time.
	artQueueWorkers = 4
	// artQueueItem bounds one queued fetch.
	artQueueItem = 40 * time.Second
)

// artQueueSem holds the queued fetches running now.
var artQueueSem = make(chan struct{}, artQueueWorkers)

// queueMusicArt fetches a picture behind the route's answer (bounded: a
// few at a time, at most artQueueMax waiting); false when it is already
// queued, remembered missing, unknown or the queue is full.
func (h *Handlers) queueMusicArt(kind, mbid, size, v string) bool {
	if h.lib() == nil || h.lib().Artwork == nil || !metadata.ValidMBID(mbid) {
		return false
	}
	size = ArtSize(kind, size)
	key := kind + "|" + mbid + "|" + size + "|" + v
	c := &h.music
	c.mu.Lock()
	if m, ok := c.artMiss[kind+"|"+mbid]; ok && time.Since(m.at) < artMissWait(m.n) {
		c.mu.Unlock()
		return false
	}
	if c.artQueued == nil {
		c.artQueued = map[string]bool{}
	}
	if c.artQueued[key] || len(c.artQueued) >= artQueueMax {
		c.mu.Unlock()
		return c.artQueued[key]
	}
	c.artQueued[key] = true
	c.mu.Unlock()
	c.prefetching.Add(1)
	go func() {
		defer c.prefetching.Done()
		defer func() {
			c.mu.Lock()
			delete(c.artQueued, key)
			c.mu.Unlock()
		}()
		artQueueSem <- struct{}{}
		defer func() { <-artQueueSem }()
		ctx, cancel := context.WithTimeout(context.Background(), artQueueItem)
		defer cancel()
		h.musicArt(ctx, kind, mbid, size, v)
	}()
	return true
}

// artFile is one picture the route can serve: exact is false when a
// smaller size stands in for the one asked (not made yet), small when it is
// the copy made here, v the version it was fetched at.
type artFile struct {
	name, path, v    string
	ok, exact, small bool
}

// musicTrendArtFile finds and keeps the picture of an artist or album at a
// size (the record's version when known). Misses are remembered a while.
func (h *Handlers) musicTrendArtFile(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	f := h.musicArt(ctx, kind, mbid, size, "")
	return f.name, f.path, f.ok
}

// musicArt is musicTrendArtFile at a version; one fetch per picture at a
// time (a second ask waits for the first).
func (h *Handlers) musicArt(ctx context.Context, kind, mbid, size, v string) artFile {
	size = ArtSize(kind, size)
	c := &h.music
	key := kind + "|" + mbid + "|" + size + "|" + v
	c.mu.Lock()
	if f := c.artBusy[key]; f != nil {
		c.mu.Unlock()
		select {
		case <-f.done:
			if f.file.ok {
				return f.file
			}
		case <-ctx.Done():
			return artFile{}
		}
		return h.fetchMusicArt(ctx, kind, mbid, size, v, true)
	}
	if c.artBusy == nil {
		c.artBusy = map[string]*artFetch{}
	}
	f := &artFetch{done: make(chan struct{})}
	c.artBusy[key] = f
	c.mu.Unlock()
	f.file = h.fetchMusicArt(ctx, kind, mbid, size, v, true)
	c.mu.Lock()
	delete(c.artBusy, key)
	c.mu.Unlock()
	close(f.done)
	return f.file
}

// heroArtistImage is an artist's photo for a wide Spotlight card: the
// 1200 px copy (the 500 one goes soft once cropped to 16:9).
func heroArtistImage(mbid string) string {
	return "/api/v1/music-trending/art/artist/" + mbid + "?size=1200"
}

// musicArtSrc is the metadata server's address of a stored music picture
// (WebP covers and photos at 500 or 1200, wide at 960 or 1920, PNG logos)
// at version v ("" when not known: the address then answers a copy kept a
// day). There is no stored 250: it is made here from the 500.
func (h *Handlers) musicArtSrc(kind, mbid, size, v string) string {
	if h.imageBase() == "" || !metadata.ValidMBID(mbid) {
		return ""
	}
	var file string
	switch kind {
	case "album", "artist":
		file = "500.webp"
		if size == "1200" {
			file = "1200.webp"
		}
	case "wide":
		file = ArtSize("wide", size) + ".webp"
	case "logo":
		file = "1200.png"
	default:
		return ""
	}
	u := strings.TrimRight(h.imageBase(), "/") + "/" + kind + "/" + mbid + "/" + file
	if v != "" {
		u += "?v=" + v
	}
	return u
}

// artTemporary reports a picture fetch error that says nothing about the
// picture: a rate limit or server trouble (a 429 also slows the preload
// down), or a network failure. A redirect (not stored yet), a 404 or a
// stand-in is a real miss.
func (h *Handlers) artTemporary(err error) bool {
	if err == nil || errors.Is(err, library.ErrArtworkMoved) || errors.Is(err, library.ErrArtworkRejected) || errors.Is(err, library.ErrArtworkURL) {
		return false
	}
	var se *library.ArtworkStatusError
	if errors.As(err, &se) {
		if se.Code == 429 {
			h.musicPre.slowDown(time.Now())
		}
		return se.Temporary()
	}
	return true
}

// fetchMusicArt finds a picture on disk, and with fetch asks the metadata
// server for one not stored here yet.
func (h *Handlers) fetchMusicArt(ctx context.Context, kind, mbid, size, v string, fetch bool) artFile {
	c := &h.music
	c.mu.Lock()
	_, listed := c.names[kind+"|"+mbid]
	// An artist page asks for its own artist by id, listed or not; any other picture is only for a listed id.
	known := listed || (kind == "artist" || kind == "wide" || kind == "logo") && metadata.ValidMBID(mbid)
	miss, missed := c.artMiss[kind+"|"+mbid]
	c.mu.Unlock()
	if !listed {
		// Any catalog album or artist answers at once, listed by a page yet
		// or not (no race with the page that lists it).
		if b := h.musicPre.catalog.Load(); b != nil {
			if _, ok := b.Lookup(kind, mbid); ok {
				known = true
			}
		}
	}
	if !known && h.Packages != nil {
		// Every artist and album a kept package lists is known.
		known = kind == "album" && h.Packages.HasAlbum(mbid) || kind != "album" && h.Packages.Covers(mbid)
	}
	lib := h.lib()
	if !known || lib == nil || lib.Artwork == nil || (fetch && missed && time.Since(miss.at) < artMissWait(miss.n)) {
		return artFile{}
	}
	if v == "" {
		v = h.PictureVersion(kind, mbid)
	}
	// temporary: the server's answer said nothing about the picture (rate
	// limit, trouble, network): never remembered as missing.
	temporary := false
	stored := func(src string) (string, string, bool) {
		if src == "" {
			return "", "", false
		}
		n := library.ArtworkName(src)
		if p, ok := lib.Artwork.Path(n); ok {
			lib.Artwork.Recheck(src, ServerStandIn)
			return n, p, true
		}
		if !fetch {
			return "", "", false
		}
		// A failed fetch may only have lost the race with a warm of the same
		// picture (the file is then stored): it counts when the file is there.
		_, err := lib.Artwork.FetchStored(ctx, src, ServerStandIn)
		if p, ok := lib.Artwork.Path(n); ok {
			return n, p, true
		}
		temporary = temporary || h.artTemporary(err)
		return "", "", false
	}
	// Pictures come only from our metadata server: never
	// straight from an outside source. One the server has not stored yet is
	// a miss (the card shows its placeholder); the server queues it, and the
	// picture is asked again after musicArtMissTTL or on the next preload.
	want := size
	if size == "250" {
		want = "500"
	}
	if n, p, ok := stored(h.musicArtSrc(kind, mbid, want, v)); ok {
		if size != "250" {
			return artFile{name: n, path: p, v: v, ok: true, exact: true}
		}
		if sn, sp, err := lib.Artwork.Smaller(n, 250); err == nil {
			return artFile{name: sn, path: sp, v: v, ok: true, exact: true, small: sn != n}
		}
		return artFile{name: n, path: p, v: v, ok: true}
	}
	if small := StandIn[size]; small != "" && kind != "logo" {
		if n, p, ok := stored(h.musicArtSrc(kind, mbid, small, v)); ok {
			return artFile{name: n, path: p, v: v, ok: true}
		}
	}
	if fetch && ctx.Err() == nil && !temporary {
		c.mu.Lock()
		if c.artMiss == nil || len(c.artMiss) > 20000 {
			c.artMiss = map[string]artMissAt{}
		}
		k := kind + "|" + mbid
		c.artMiss[k] = artMissAt{at: time.Now(), n: c.artMiss[k].n + 1}
		c.mu.Unlock()
	}
	return artFile{}
}
