package music

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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
// 23 October 2026).
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
	// failed is when asking the metadata server about an artist last
	// failed (not "no such artist"): the pass waits artistFailWait.
	failed map[string]time.Time
	now    func() time.Time
}

// noteFailed remembers that asking about mbid failed at now. Entries older
// than artistFailWait go first, and the map stays under artistVideosMax.
func (c *artistVideoCache) noteFailed(mbid string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed == nil {
		c.failed = map[string]time.Time{}
	}
	if _, ok := c.failed[mbid]; !ok && len(c.failed) >= artistVideosMax {
		for k, at := range c.failed {
			if now.Sub(at) >= artistFailWait {
				delete(c.failed, k)
			}
		}
		if len(c.failed) >= artistVideosMax {
			return
		}
	}
	c.failed[mbid] = now
}

// forgetFailed drops mbid's failure once the server answered about it.
func (c *artistVideoCache) forgetFailed(mbid string) {
	c.mu.Lock()
	delete(c.failed, mbid)
	c.mu.Unlock()
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
		webserver.KeepUnder(c.m, artistVideosMax, func(v artistVideoList) time.Time { return v.at })
	}
	c.m[k] = e
}

type storedVideos struct {
	At     time.Time              `json:"at"`
	Videos []metadata.ArtistVideo `json:"videos"`
}

func videoDiskKey(mbid string) string { return "artistvideos|" + mbid }

// keepVideos stores a list in memory, and on disk when it is not empty.
func (h *Handlers) keepVideos(ctx context.Context, mbid string, e artistVideoList) {
	c := &h.artistVideos
	c.mu.Lock()
	c.putLocked(mbid, e)
	c.mu.Unlock()
	if h.Store == nil {
		return
	}
	lc := metadata.LookupCache{Store: h.Store}
	if len(e.list) == 0 {
		// Kept until the pass would ask again (a restart does not re-ask
		// every artist with no videos at once).
		if b, err := json.Marshal(storedVideos{At: e.at, Videos: []metadata.ArtistVideo{}}); err == nil {
			_ = lc.Put(context.WithoutCancel(ctx), videoDiskKey(mbid), b, e.at, e.at.Add(artistEmptyRecheck))
		}
		return
	}
	if b, err := json.Marshal(storedVideos{At: e.at, Videos: e.list}); err == nil {
		_ = lc.Put(context.WithoutCancel(ctx), videoDiskKey(mbid), b, e.at, e.at.Add(artistVideosDisk))
	}
}

// keptVideos reads the artist's videos from memory, then disk; ok is false
// when neither has an answer.
func (h *Handlers) keptVideos(ctx context.Context, mbid string) (artistVideoList, bool) {
	c := &h.artistVideos
	c.mu.Lock()
	e, ok := c.m[mbid]
	c.mu.Unlock()
	if !ok && h.Store != nil {
		if b, found, _ := (metadata.LookupCache{Store: h.Store}).Get(ctx, videoDiskKey(mbid), time.Now()); found {
			var sv storedVideos
			if json.Unmarshal(b, &sv) == nil && !sv.At.IsZero() {
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
func (h *Handlers) videosOf(ctx context.Context, mbid string) []metadata.ArtistVideo {
	c := &h.artistVideos
	now := c.clock()
	e, ok := h.keptVideos(ctx, mbid)
	switch {
	case ok && len(e.list) > 0:
		if now.Sub(e.at) >= artistVideosFresh {
			h.recheckVideos(mbid)
		}
		return e.list
	case ok && now.Sub(e.at) < artistVideosEmpty:
		return nil
	}
	l, err := h.ArtistVideos(ctx, mbid)
	if err != nil {
		// An artist the server does not know is kept as an empty list, so
		// asking again within a few minutes costs no lookup. Any other
		// failure (the server down) is remembered for the pass.
		switch {
		case errors.Is(err, metadata.ErrNotFound):
			c.forgetFailed(mbid)
			h.keepVideos(ctx, mbid, artistVideoList{at: now})
		case ctx.Err() == nil:
			// Not the caller giving up (its own cancel or deadline).
			c.noteFailed(mbid, now)
		}
		return nil
	}
	c.forgetFailed(mbid)
	h.keepVideos(ctx, mbid, artistVideoList{at: now, list: l})
	return l
}

// recheckVideos asks the server again in the background; a failed or empty
// answer keeps the list it has.
func (h *Handlers) recheckVideos(mbid string) {
	c := &h.artistVideos
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
		l, err := h.ArtistVideos(ctx, mbid)
		if err == nil && len(l) > 0 {
			h.keepVideos(ctx, mbid, artistVideoList{at: c.clock(), list: l})
		}
	}()
}

// dropVideo removes a video that would not play from the kept list.
func (h *Handlers) dropVideo(ctx context.Context, mbid, id string) []metadata.ArtistVideo {
	c := &h.artistVideos
	c.mu.Lock()
	e := c.m[mbid]
	c.mu.Unlock()
	kept := make([]metadata.ArtistVideo, 0, len(e.list))
	for _, v := range e.list {
		if v.ID != id {
			kept = append(kept, v)
		}
	}
	h.keepVideos(ctx, mbid, artistVideoList{at: e.at, list: kept})
	return kept
}

// videoStart picks the start point: the server's most replayed point,
// else the heatmap peak, else 30% of the length.
func videoStart(v metadata.ArtistVideo, s metadata.TrailerStream) float64 {
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

func (h *Handlers) artistVideo(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	// ?id= is a card in the Music videos row: pointing at it reads what is
	// kept; a click (&play=1) may look that one video up.
	if id := r.URL.Query().Get("id"); id != "" {
		h.pickedVideo(w, r, mbid, id, r.URL.Query().Get("play") == "1")
		return
	}
	if !health.AutoplayOn(r.Context(), h.Store) {
		webserver.Fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	if h.ArtistVideos == nil {
		webserver.Fail(w, http.StatusNotFound, "no video")
		return
	}
	vids := h.videosOf(metadata.MemberLookup(r.Context()), mbid)
	if len(vids) == 0 {
		webserver.Fail(w, http.StatusNotFound, "no video")
		return
	}
	// Noted on every view, so the pass keeps a video ready (and renewed).
	h.noteArtistVideos(r.Context(), []string{mbid})
	// ?skip=id,id names videos already played: the next one is another of the
	// artist's videos, and none left answers 404 so the page repeats its last.
	if skip := r.URL.Query().Get("skip"); skip != "" {
		vids = withoutVideos(vids, strings.Split(skip, ","))
		if len(vids) == 0 {
			webserver.Fail(w, http.StatusNotFound, "no more videos")
			return
		}
	}
	// Only a stream already looked up answers: the visitor never waits on a
	// lookup, and none starts here. None ready yet answers 404 at once (the
	// hero keeps its photo); the trailer pass makes one ready.
	v, s, ready := h.readyVideo(r.Context(), vids)
	if !ready {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusNotFound, "no video ready yet")
		return
	}
	view, ok := h.videoView(r, v, s)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	webserver.WriteJSON(w, http.StatusOK, view)
}

// pickedVideo answers ?id=: that one of the artist's videos, relayed. Label
// videos refuse YouTube's embedded player (error 150), so the row plays
// them through the relay. Only a click (play) looks a video not kept yet
// up, behind the answer: pointing at a card or focusing it reads what is
// kept. One that will not play answers 404 and stays in the list (the
// row's embed may still play it).
func (h *Handlers) pickedVideo(w http.ResponseWriter, r *http.Request, mbid, id string, play bool) {
	var v metadata.ArtistVideo
	if h.ArtistVideos != nil {
		for _, k := range h.videosOf(metadata.MemberLookup(r.Context()), mbid) {
			if k.ID == id && metadata.ValidYouTubeID(k.ID) {
				v = k
				break
			}
		}
	}
	if v.ID == "" {
		webserver.Fail(w, http.StatusNotFound, "no such video")
		return
	}
	s, known := h.videoKnown(r.Context(), v.ID)
	if !known {
		// A click: looked up behind the answer; the row's embed plays meanwhile.
		if play {
			h.resolveVideoBehind(r, mbid, nil, v.ID)
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusNotFound, "no video ready yet")
		return
	}
	if s.Type != "hls" {
		webserver.Fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	view, ok := h.videoView(r, v, s)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	webserver.WriteJSON(w, http.StatusOK, view)
}

// videoView relays the stream for this person and describes it.
func (h *Handlers) videoView(r *http.Request, v metadata.ArtistVideo, s metadata.TrailerStream) (ArtistVideoView, bool) {
	id := h.Trailers.Relay(h.Person(r).UserID, s.URL, s.ExpiresAt)
	if id == "" {
		return ArtistVideoView{}, false
	}
	dur := v.Duration
	if dur <= 0 {
		dur = s.Duration
	}
	return ArtistVideoView{Key: v.ID, ProxyURL: webserver.URLBaseFrom(r.Context()) + "/api/v1/trailer/stream/" + id + "/manifest.m3u8",
		Start: videoStart(v, s), Duration: dur, ExpiresAt: s.ExpiresAt}, true
}

// videoKnown reads a video's stream from memory or SQLite only; known
// with a zero stream means it has no direct stream.
func (h *Handlers) videoKnown(ctx context.Context, id string) (metadata.TrailerStream, bool) {
	ckey := "musicvideo|" + id
	now := time.Now()
	e, ok := h.Trailers.Get(ckey, now)
	if !ok && h.Trailers.Stored(ctx, ckey, now) {
		e, ok = h.Trailers.Get(ckey, now)
	}
	if !ok || !e.Miss.IsZero() || e.Stream.Type != "hls" {
		return metadata.TrailerStream{}, ok
	}
	return e.Stream, true
}

// readyVideo is the first of the videos whose stream is already known.
func (h *Handlers) readyVideo(ctx context.Context, vids []metadata.ArtistVideo) (metadata.ArtistVideo, metadata.TrailerStream, bool) {
	for _, v := range vids {
		if s, _ := h.videoKnown(ctx, v.ID); s.URL != "" {
			return v, s, true
		}
	}
	return metadata.ArtistVideo{}, metadata.TrailerStream{}, false
}

// resolveVideoBehind looks one video's stream up after the answer: id, else
// the first of vids not known yet. It counts against the person's video
// budget; one YouTube will not play here is dropped from the artist's list.
func (h *Handlers) resolveVideoBehind(r *http.Request, mbid string, vids []metadata.ArtistVideo, id string) {
	ctx := metadata.MemberLookup(context.WithoutCancel(r.Context()))
	if id == "" {
		for _, v := range vids {
			if _, known := h.videoKnown(ctx, v.ID); !known {
				id = v.ID
				break
			}
		}
	}
	if id == "" || h.Trailers.InFlight("musicvideo|"+id) || !h.Trailers.Allow(h.Person(r).UserID, time.Now()) {
		return
	}
	go func() {
		lctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if _, err := h.resolveMusicVideo(lctx, id); videoWontPlay(err) && lctx.Err() == nil && vids != nil {
			h.dropVideo(ctx, mbid, id)
		}
	}()
}

// videoWontPlay says whether a music video's lookup failed for that video
// (removed, age, blocked): it is dropped. A failure at YouTube's door
// (a bot check, say) is no reason to drop the artist's videos.
func videoWontPlay(err error) bool {
	m := (*metadata.TrailerMissed)(nil)
	return err != nil && errors.As(err, &m) && m.Reason.VideoReason()
}

// resolveMusicVideo resolves one YouTube video like a trailer: memory,
// then SQLite, then one shared reader lookup in the trailer pool.
func (h *Handlers) resolveMusicVideo(ctx context.Context, key string) (metadata.TrailerStream, error) {
	ckey := "musicvideo|" + key
	now := time.Now()
	e, ok := h.Trailers.Get(ckey, now)
	if !ok && h.Trailers.Stored(ctx, ckey, now) {
		e, ok = h.Trailers.Get(ckey, now)
	}
	if ok {
		if !e.Miss.IsZero() {
			return metadata.TrailerStream{}, e.MissErr()
		}
		return e.Stream, nil
	}
	f, err, done := h.Trailers.Shared(ctx, ckey, func(lctx context.Context) (metadata.TrailerEntry, error) {
		return h.lookupMusicVideo(lctx, key, ckey)
	})
	if !done {
		return metadata.TrailerStream{}, ctx.Err()
	}
	if err != nil {
		return metadata.TrailerStream{}, metadata.ErrNoStream
	}
	if !f.Miss.IsZero() {
		return metadata.TrailerStream{}, f.MissErr()
	}
	return f.Stream, nil
}

func (h *Handlers) lookupMusicVideo(ctx context.Context, key, ckey string) (metadata.TrailerEntry, error) {
	now := time.Now()
	s, err := h.Trailers.Read(ctx, key, key, now, true)
	if err != nil {
		if metadata.VideoMissReason(err) == "" {
			return metadata.TrailerEntry{}, err
		}
		e := metadata.TrailerEntry{Miss: now.Add(metadata.TrailerMiss), Reason: metadata.TrailerReasonOf(err)}
		h.Trailers.Put(ckey, e)
		return e, nil
	}
	return h.Trailers.KeepStream(ctx, ckey, s, now), nil
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
