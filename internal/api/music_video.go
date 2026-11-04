package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
	"github.com/CoderLuii/holyarrs/internal/youtube"
)

// The artist page hero plays one of the artist's official music videos,
// muted and looping, through the trailer relay (pass-through HLS). The
// metadata server lists the videos by YouTube id (with the most replayed
// point when it knows it); the YouTube reader resolves the stream like a
// trailer and reads the "most replayed" graph. No videos or an older
// server: 404, and the hero keeps its photo. Kid accounts never get one (the page gate), nor does
// anyone with "Autoplay trailers" off; the relay's caps apply as for trailers.

// ArtistVideoView answers GET /api/v1/music/artists/{mbid}?part=video.
type ArtistVideoView struct {
	Key string `json:"key"`
	// ProxyURL is the relayed HLS stream the hero plays.
	ProxyURL string `json:"proxy_url"`
	// Start is where to start, in seconds: the server's most replayed
	// point, else YouTube's heatmap peak, else 30% of the length.
	Start     float64   `json:"start"`
	Duration  float64   `json:"duration,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// A non-empty list is kept on disk and re-checked daily in the background
// (the page never waits on that check); an empty list is re-asked after a
// few minutes; a pending or failed answer is not kept (cache audit
// 2026-10-23).
const (
	artistVideosFresh = 24 * time.Hour
	artistVideosEmpty = 5 * time.Minute
	artistVideosDisk  = 30 * 24 * time.Hour
	artistVideosMax   = 2000
)

type artistVideoList struct {
	at   time.Time
	list []metadata.ArtistVideo
}

type artistVideoCache struct {
	mu   sync.Mutex
	m    map[string]artistVideoList
	busy map[string]bool
	now  func() time.Time
}

func (c *artistVideoCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *artistVideoCache) putLocked(k string, e artistVideoList) {
	if c.m == nil {
		c.m = map[string]artistVideoList{}
	}
	if _, has := c.m[k]; !has {
		keepUnder(c.m, artistVideosMax, func(v artistVideoList) time.Time { return v.at })
	}
	c.m[k] = e
}

type storedVideos struct {
	At     time.Time              `json:"at"`
	Videos []metadata.ArtistVideo `json:"videos"`
}

func videoDiskKey(mbid string) string { return "artistvideos|" + mbid }

// keepVideos stores a list in memory, and on disk when it is not empty.
func (a *API) keepVideos(ctx context.Context, mbid string, e artistVideoList) {
	c := &a.artistVideos
	c.mu.Lock()
	c.putLocked(mbid, e)
	c.mu.Unlock()
	if a.s.Store == nil {
		return
	}
	lc := store.LookupCache{Store: a.s.Store}
	if len(e.list) == 0 {
		_ = lc.Put(context.WithoutCancel(ctx), videoDiskKey(mbid), []byte("{}"), e.at, e.at)
		return
	}
	if b, err := json.Marshal(storedVideos{At: e.at, Videos: e.list}); err == nil {
		_ = lc.Put(context.WithoutCancel(ctx), videoDiskKey(mbid), b, e.at, e.at.Add(artistVideosDisk))
	}
}

// keptVideos reads the artist's videos from memory, then disk; ok is false
// when neither has an answer.
func (a *API) keptVideos(ctx context.Context, mbid string) (artistVideoList, bool) {
	c := &a.artistVideos
	c.mu.Lock()
	e, ok := c.m[mbid]
	c.mu.Unlock()
	if !ok && a.s.Store != nil {
		if b, found, _ := (store.LookupCache{Store: a.s.Store}).Get(ctx, videoDiskKey(mbid), time.Now()); found {
			var sv storedVideos
			if json.Unmarshal(b, &sv) == nil && len(sv.Videos) > 0 {
				e, ok = artistVideoList{at: sv.At, list: sv.Videos}, true
				c.mu.Lock()
				c.putLocked(mbid, e)
				c.mu.Unlock()
			}
		}
	}
	return e, ok
}

// videosOf reads the artist's videos: memory, then disk, then the server.
func (a *API) videosOf(ctx context.Context, mbid string) []metadata.ArtistVideo {
	c := &a.artistVideos
	now := c.clock()
	e, ok := a.keptVideos(ctx, mbid)
	switch {
	case ok && len(e.list) > 0:
		if now.Sub(e.at) >= artistVideosFresh {
			a.recheckVideos(mbid)
		}
		return e.list
	case ok && now.Sub(e.at) < artistVideosEmpty:
		return nil
	}
	l, err := a.s.ArtistVideos(ctx, mbid)
	if err != nil {
		// An artist the server does not know is kept as an empty list, so
		// asking again within a few minutes costs no lookup.
		if errors.Is(err, metadata.ErrNotFound) {
			a.keepVideos(ctx, mbid, artistVideoList{at: now})
		}
		return nil
	}
	a.keepVideos(ctx, mbid, artistVideoList{at: now, list: l})
	return l
}

// recheckVideos asks the server again in the background; a failed or empty
// answer keeps the list it has.
func (a *API) recheckVideos(mbid string) {
	c := &a.artistVideos
	c.mu.Lock()
	if c.busy == nil {
		c.busy = map[string]bool{}
	}
	if c.busy[mbid] {
		c.mu.Unlock()
		return
	}
	c.busy[mbid] = true
	c.mu.Unlock()
	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.busy, mbid)
			c.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		l, err := a.s.ArtistVideos(ctx, mbid)
		if err == nil && len(l) > 0 {
			a.keepVideos(ctx, mbid, artistVideoList{at: c.clock(), list: l})
		}
	}()
}

// dropVideo removes a video that would not play from the kept list.
func (a *API) dropVideo(ctx context.Context, mbid, id string) []metadata.ArtistVideo {
	c := &a.artistVideos
	c.mu.Lock()
	e := c.m[mbid]
	c.mu.Unlock()
	kept := make([]metadata.ArtistVideo, 0, len(e.list))
	for _, v := range e.list {
		if v.ID != id {
			kept = append(kept, v)
		}
	}
	a.keepVideos(ctx, mbid, artistVideoList{at: e.at, list: kept})
	return kept
}

// videoStart picks the start point: the server's most replayed point,
// else the heatmap peak, else 30% of the length.
func videoStart(v metadata.ArtistVideo, s TrailerStream) float64 {
	dur := v.Duration
	if dur <= 0 {
		dur = s.Duration
	}
	switch {
	case v.MostReplayed > 0:
		return v.MostReplayed
	case s.Peak > 0 && (dur <= 0 || s.Peak < dur):
		return s.Peak
	case dur > 0:
		return dur * 0.3
	}
	return 0
}

func (a *API) artistVideo(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	// ?id= is a card in the Music videos row: a click, not autoplay.
	if id := r.URL.Query().Get("id"); id != "" {
		a.pickedVideo(w, r, mbid, id)
		return
	}
	if !a.uiSettings(r.Context()).AutoplayTrailers {
		fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	if a.s.ArtistVideos == nil {
		fail(w, http.StatusNotFound, "no video")
		return
	}
	vids := a.videosOf(metadata.MemberLookup(r.Context()), mbid)
	if len(vids) == 0 {
		fail(w, http.StatusNotFound, "no video")
		return
	}
	// Noted on every view, so the pass keeps a video ready (and renewed).
	a.noteArtistVideos(r.Context(), []string{mbid})
	// ?skip=id,id names videos already played: the next one is another of the
	// artist's videos, and none left answers 404 so the page repeats its last.
	if skip := r.URL.Query().Get("skip"); skip != "" {
		vids = withoutVideos(vids, strings.Split(skip, ","))
		if len(vids) == 0 {
			fail(w, http.StatusNotFound, "no more videos")
			return
		}
	}
	// Only a stream already looked up answers: the visitor never waits on a
	// lookup, and none starts here. None ready yet answers 404 at once (the
	// hero keeps its photo); the trailer pass makes one ready.
	v, s, ready := a.readyVideo(r.Context(), vids)
	if !ready {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Retry-After", "5")
		fail(w, http.StatusNotFound, "no video ready yet")
		return
	}
	view, ok := a.videoView(r, v, s)
	if !ok {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, view)
}

// pickedVideo answers ?id=: that one of the artist's videos, relayed. Label
// videos refuse YouTube's embedded player (error 150), so the row plays
// them through the relay. One that will not play answers 404 and stays in
// the list (the row's embed may still play it).
func (a *API) pickedVideo(w http.ResponseWriter, r *http.Request, mbid, id string) {
	var v metadata.ArtistVideo
	if a.s.ArtistVideos != nil {
		for _, k := range a.videosOf(metadata.MemberLookup(r.Context()), mbid) {
			if k.ID == id && metadata.ValidYouTubeID(k.ID) {
				v = k
				break
			}
		}
	}
	if v.ID == "" {
		fail(w, http.StatusNotFound, "no such video")
		return
	}
	s, known := a.videoKnown(r.Context(), v.ID)
	if !known {
		// Looked up behind the answer; the row's embed plays meanwhile.
		a.resolveVideoBehind(r, mbid, nil, v.ID)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Retry-After", "5")
		fail(w, http.StatusNotFound, "no video ready yet")
		return
	}
	if s.Type != "hls" {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	view, ok := a.videoView(r, v, s)
	if !ok {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, view)
}

// videoView relays the stream for this person and describes it.
func (a *API) videoView(r *http.Request, v metadata.ArtistVideo, s TrailerStream) (ArtistVideoView, bool) {
	id := a.trailerRelay.register(a.person(r).UserID, s.URL, s.ExpiresAt)
	if id == "" {
		return ArtistVideoView{}, false
	}
	dur := v.Duration
	if dur <= 0 {
		dur = s.Duration
	}
	return ArtistVideoView{Key: v.ID, ProxyURL: urlbase.From(r.Context()) + "/api/v1/trailer/stream/" + id + "/manifest.m3u8",
		Start: videoStart(v, s), Duration: dur, ExpiresAt: s.ExpiresAt}, true
}

// videoKnown reads a video's stream from memory or SQLite only; known
// with a zero stream means it has no direct stream (for now).
func (a *API) videoKnown(ctx context.Context, id string) (TrailerStream, bool) {
	ckey := "musicvideo|" + id
	now := time.Now()
	e, ok := a.trailers.get(ckey, now)
	if !ok && a.trailerStored(ctx, ckey, now) {
		e, ok = a.trailers.get(ckey, now)
	}
	if !ok || !e.miss.IsZero() || e.s.Type != "hls" {
		return TrailerStream{}, ok
	}
	return e.s, true
}

// readyVideo is the first of the videos whose stream is already known.
func (a *API) readyVideo(ctx context.Context, vids []metadata.ArtistVideo) (metadata.ArtistVideo, TrailerStream, bool) {
	for _, v := range vids {
		if s, _ := a.videoKnown(ctx, v.ID); s.URL != "" {
			return v, s, true
		}
	}
	return metadata.ArtistVideo{}, TrailerStream{}, false
}

// resolveVideoBehind looks one video's stream up after the answer: id, else
// the first of vids not known yet. It counts against the person's video
// budget; one YouTube will not play here is dropped from the artist's list.
func (a *API) resolveVideoBehind(r *http.Request, mbid string, vids []metadata.ArtistVideo, id string) {
	ctx := metadata.MemberLookup(context.WithoutCancel(r.Context()))
	if id == "" {
		for _, v := range vids {
			if _, known := a.videoKnown(ctx, v.ID); !known {
				id = v.ID
				break
			}
		}
	}
	if id == "" || a.trailers.inFlight("musicvideo|"+id) || !a.trailers.allow(a.person(r).UserID, time.Now()) {
		return
	}
	go func() {
		lctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if _, err := a.resolveMusicVideo(lctx, id); videoWontPlay(err) && lctx.Err() == nil && vids != nil {
			a.dropVideo(ctx, mbid, id)
		}
	}()
}

// videoWontPlay says whether a music video's lookup failed for that video
// (removed, age, blocked): it is dropped. A failure at YouTube's door
// (a bot check, say) is no reason to drop the artist's videos.
func videoWontPlay(err error) bool {
	m := (*trailerMissed)(nil)
	return err != nil && errors.As(err, &m) && m.reason.VideoReason()
}

// resolveMusicVideo resolves one YouTube video like a trailer: memory,
// then SQLite, then one shared reader lookup in the trailer pool.
func (a *API) resolveMusicVideo(ctx context.Context, key string) (TrailerStream, error) {
	ckey := "musicvideo|" + key
	now := time.Now()
	e, ok := a.trailers.get(ckey, now)
	if !ok && a.trailerStored(ctx, ckey, now) {
		e, ok = a.trailers.get(ckey, now)
	}
	if ok {
		if !e.miss.IsZero() {
			return TrailerStream{}, e.missErr()
		}
		return e.s, nil
	}
	t := &a.trailers
	t.mu.Lock()
	if t.flights == nil {
		t.flights = map[string]*trailerFlight{}
	}
	f := t.flights[ckey]
	if f == nil {
		f = &trailerFlight{done: make(chan struct{})}
		t.flights[ckey] = f
		go func() {
			lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			f.e, f.err = a.lookupMusicVideo(lctx, key, ckey)
			t.mu.Lock()
			delete(t.flights, ckey)
			t.mu.Unlock()
			close(f.done)
		}()
	}
	t.mu.Unlock()
	select {
	case <-ctx.Done():
		return TrailerStream{}, ctx.Err()
	case <-f.done:
	}
	if f.err != nil {
		return TrailerStream{}, errNoStream
	}
	if !f.e.miss.IsZero() {
		return TrailerStream{}, f.e.missErr()
	}
	return f.e.s, nil
}

func (a *API) lookupMusicVideo(ctx context.Context, key, ckey string) (trailerEntry, error) {
	now := time.Now()
	s, err := a.readTrailer(ctx, key, key, now, true)
	if err != nil {
		if youtube.ReasonOf(err) == "" {
			return trailerEntry{}, err
		}
		e := trailerEntry{miss: now.Add(trailerMiss), reason: trailerReason(err)}
		a.trailers.put(ckey, e)
		return e, nil
	}
	return a.keepMusicVideo(ctx, ckey, s, now), nil
}

// keepMusicVideo keeps a music video's stream in memory and SQLite.
func (a *API) keepMusicVideo(ctx context.Context, ckey string, s TrailerStream, now time.Time) trailerEntry {
	e := trailerEntry{s: s}
	a.trailers.put(ckey, e)
	if a.s.Store != nil {
		if b, err := json.Marshal(s); err == nil {
			_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailer|"+ckey, b, now, s.ExpiresAt.Add(-trailerEarly))
		}
	}
	return e
}

// missedFor turns a reader error with a reason into the miss callers
// test with videoWontPlay; other errors pass as they are.
func missedFor(err error) error {
	if err == nil || youtube.ReasonOf(err) == "" {
		return err
	}
	return trailerEntry{reason: trailerReason(err)}.missErr()
}

// withoutVideos drops the listed video ids (at most 50 are honoured).
func withoutVideos(vids []metadata.ArtistVideo, skip []string) []metadata.ArtistVideo {
	if len(skip) > 50 {
		skip = skip[:50]
	}
	drop := make(map[string]bool, len(skip))
	for _, id := range skip {
		if id = strings.TrimSpace(id); id != "" {
			drop[id] = true
		}
	}
	out := make([]metadata.ArtistVideo, 0, len(vids))
	for _, v := range vids {
		if !drop[v.ID] {
			out = append(out, v)
		}
	}
	return out
}
