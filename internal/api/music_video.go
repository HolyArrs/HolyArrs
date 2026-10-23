package api

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

// The artist page hero plays one of the artist's official music videos,
// muted and looping, through the trailer relay (pass-through HLS). The
// metadata server lists the videos by YouTube id (with the most replayed
// point when it knows it); yt-dlp resolves the stream like a trailer and
// reads the heatmap. No videos, an older server or no yt-dlp: 404, and the
// hero keeps its photo. Kid accounts never get one (the page gate), nor does
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

const artistVideosKeep = time.Hour

type artistVideoList struct {
	at   time.Time
	list []metadata.ArtistVideo
}

type artistVideoCache struct {
	mu sync.Mutex
	m  map[string]artistVideoList
}

// videosOf reads the artist's videos, kept an hour (none: kept too).
func (a *API) videosOf(ctx context.Context, mbid string) []metadata.ArtistVideo {
	c := &a.artistVideos
	now := time.Now()
	c.mu.Lock()
	e, ok := c.m[mbid]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < artistVideosKeep {
		return e.list
	}
	l, err := a.s.ArtistVideos(ctx, mbid)
	if err != nil && ctx.Err() != nil {
		return nil
	}
	c.mu.Lock()
	if c.m == nil || len(c.m) > 2000 {
		c.m = map[string]artistVideoList{}
	}
	c.m[mbid] = artistVideoList{at: now, list: l}
	c.mu.Unlock()
	return l
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
	if !a.uiSettings(r.Context()).AutoplayTrailers {
		fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	if a.s.ArtistVideos == nil {
		fail(w, http.StatusNotFound, "no video")
		return
	}
	if _, _, found := ytdlpRun(); !found {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	vids := a.videosOf(metadata.MemberLookup(r.Context()), mbid)
	if len(vids) == 0 {
		fail(w, http.StatusNotFound, "no video")
		return
	}
	v := vids[rand.IntN(len(vids))]
	ckey := "musicvideo|" + v.ID
	now := time.Now()
	if _, ok := a.trailers.get(ckey, now); !ok && !a.trailerStored(r.Context(), ckey, now) && !a.trailers.inFlight(ckey) {
		if !a.trailers.allow(a.person(r).UserID, now) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "too many videos; try again in a minute")
			return
		}
	}
	s, err := a.resolveMusicVideo(r.Context(), v.ID)
	if err != nil || s.Type != "hls" {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	id := a.trailerRelay.register(a.person(r).UserID, s.URL, s.ExpiresAt)
	if id == "" {
		fail(w, http.StatusNotFound, "no direct stream")
		return
	}
	dur := v.Duration
	if dur <= 0 {
		dur = s.Duration
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, ArtistVideoView{Key: v.ID, ProxyURL: urlbase.From(r.Context()) + "/api/v1/trailer/stream/" + id + "/manifest.m3u8",
		Start: videoStart(v, s), Duration: dur, ExpiresAt: s.ExpiresAt})
}

// resolveMusicVideo resolves one YouTube video like a trailer: memory,
// then SQLite, then one shared yt-dlp lookup in the trailer pool.
func (a *API) resolveMusicVideo(ctx context.Context, key string) (TrailerStream, error) {
	ckey := "musicvideo|" + key
	now := time.Now()
	e, ok := a.trailers.get(ckey, now)
	if !ok && a.trailerStored(ctx, ckey, now) {
		e, ok = a.trailers.get(ckey, now)
	}
	if ok {
		if !e.miss.IsZero() {
			return TrailerStream{}, errNoStream
		}
		return e.s, nil
	}
	t := &a.trailers
	t.mu.Lock()
	if t.flights == nil {
		t.flights = map[string]*trailerFlight{}
	}
	if t.pool == nil {
		t.pool = make(chan struct{}, trailerPool)
	}
	pool := t.pool
	f := t.flights[ckey]
	if f == nil {
		f = &trailerFlight{done: make(chan struct{})}
		t.flights[ckey] = f
		go func() {
			lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			f.e, f.err = a.lookupMusicVideo(lctx, pool, key, ckey)
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
	if f.err != nil || !f.e.miss.IsZero() {
		return TrailerStream{}, errNoStream
	}
	return f.e.s, nil
}

func (a *API) lookupMusicVideo(ctx context.Context, pool chan struct{}, key, ckey string) (trailerEntry, error) {
	bin, pre, found := ytdlpRun()
	if !found {
		return trailerEntry{}, errNoStream
	}
	select {
	case pool <- struct{}{}:
	case <-ctx.Done():
		return trailerEntry{}, ctx.Err()
	}
	now := time.Now()
	s, err := resolveTrailer(ctx, bin, pre, key, now)
	<-pool
	if err != nil {
		a.trailers.failed.Add(1)
		e := trailerEntry{miss: now.Add(trailerMiss)}
		a.trailers.put(ckey, e)
		return e, nil
	}
	a.trailers.failed.Store(0)
	e := trailerEntry{s: s}
	a.trailers.put(ckey, e)
	if a.s.Store != nil {
		if b, err := json.Marshal(s); err == nil {
			_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailer|"+ckey, b, now, s.ExpiresAt.Add(-trailerEarly))
		}
	}
	return e, nil
}
