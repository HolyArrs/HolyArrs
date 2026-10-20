package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/ytdlp"
)

// Trailers in HolyArrs' own player: the title's YouTube trailer, resolved
// by yt-dlp to addresses on YouTube's video servers (an adaptive HLS
// manifest and the muxed 360p MP4). Only the resolved addresses are kept
// (in memory and in lookup_cache, until shortly before they expire); no
// video passes through or is stored by HolyArrs.

// EnvYTDLP names the yt-dlp program to use (package ytdlp has the order).
const EnvYTDLP = ytdlp.Env

const (
	trailerTimeout   = 8 * time.Second
	trailerPerMinute = 12               // resolves a person may start in a minute (cached answers are free)
	trailerKeep      = 4 * time.Hour    // when the address names no expiry
	trailerEarly     = 10 * time.Minute // dropped this long before it expires
	trailerMiss      = 10 * time.Minute // a title with no direct stream is not tried again before this
	trailerPool      = 4                // yt-dlp lookups at once
	trailerAhead     = 40               // ahead-of-time lookups per 10 minutes for the whole install
)

// trailerArgs ask yt-dlp for the formats only: one player API call per
// client (mweb has the muxed 360p MP4, format 18; visionos has the HLS
// manifest), no player config page, no format probing, no cache writes.
var trailerArgs = []string{"-J", "--no-playlist", "--no-warnings", "--no-cache-dir", "--no-check-formats", "--skip-download", "--ignore-no-formats-error",
	"--extractor-args", "youtube:player_client=mweb,visionos;player_skip=configs"}

// YTDLPTool finds yt-dlp; main gives it the bundled and updated copies.
var YTDLPTool = &ytdlp.Tool{}

// ytdlpRun is how yt-dlp is started; tests swap it for a fake.
var ytdlpRun = func() (bin string, pre []string, ok bool) {
	p := YTDLPTool.Find()
	return p, nil, p != ""
}

// TrailerStream is the answer of GET /trailer/{kind}/{tmdb_id}.
type TrailerStream struct {
	// Type is hls (URL is YouTube's adaptive HLS manifest, which starts low
	// and steps up) or mp4 (URL is the muxed 360p MP4, format 18).
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	// MP4URL is the muxed MP4 beside an HLS answer, for browsers without
	// native HLS: YouTube's video servers allow no cross-origin reads, so
	// a script player (hls.js) cannot fetch the manifest or its segments.
	MP4URL string `json:"mp4_url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// Audio is false for a picture-only stream (no sound to turn on).
	Audio bool `json:"audio"`
	// Key is the YouTube video, for the embed fallback.
	Key string `json:"key"`
}

type trailerEntry struct {
	s    TrailerStream
	miss time.Time // set: no direct stream, until this time
}

type trailerFlight struct {
	done chan struct{}
	e    trailerEntry
	err  error
}

type trailerState struct {
	mu      sync.Mutex
	cache   map[string]trailerEntry
	budget  map[int64][]time.Time
	flights map[string]*trailerFlight
	ahead   []time.Time
	pool    chan struct{}
	// failed counts yt-dlp lookups that failed in a row (a success resets).
	failed atomic.Int32
}

func (t *trailerState) get(key string, now time.Time) (trailerEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.cache[key]
	if !ok {
		return e, false
	}
	if (!e.miss.IsZero() && now.After(e.miss)) || (e.miss.IsZero() && now.After(e.s.ExpiresAt.Add(-trailerEarly))) {
		delete(t.cache, key)
		return e, false
	}
	return e, true
}

func (t *trailerState) put(key string, e trailerEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = map[string]trailerEntry{}
	}
	if len(t.cache) > 2000 {
		clear(t.cache)
	}
	t.cache[key] = e
}

// allow spends one of the person's resolves for this minute.
func (t *trailerState) allow(user int64, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.budget == nil {
		t.budget = map[int64][]time.Time{}
	}
	kept := t.budget[user][:0]
	for _, at := range t.budget[user] {
		if now.Sub(at) < time.Minute {
			kept = append(kept, at)
		}
	}
	if len(kept) >= trailerPerMinute {
		t.budget[user] = kept
		return false
	}
	t.budget[user] = append(kept, now)
	return true
}

// allowAhead spends one of the install's ahead-of-time lookups.
func (t *trailerState) allowAhead(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := t.ahead[:0]
	for _, at := range t.ahead {
		if now.Sub(at) < 10*time.Minute {
			kept = append(kept, at)
		}
	}
	t.ahead = kept
	if len(kept) >= trailerAhead {
		return false
	}
	t.ahead = append(t.ahead, now)
	return true
}

// TrailerFailures reports yt-dlp lookups that failed in a row, and whether
// there is a yt-dlp at all (for the health check).
func (a *API) TrailerFailures() (int, bool) {
	_, _, ok := ytdlpRun()
	return int(a.trailers.failed.Load()), ok
}

// trailerLanguage is the first language of Accept-Language ("de" from "de-DE,de;q=0.9").
func trailerLanguage(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("language")); len(l) == 2 {
		return strings.ToLower(l)
	}
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	first, _, _ = strings.Cut(strings.TrimSpace(first), ";")
	first, _, _ = strings.Cut(first, "-")
	if len(first) == 2 {
		return strings.ToLower(first)
	}
	return "en"
}

var (
	errNoTrailer = errors.New("no trailer")
	errNoStream  = errors.New("no direct stream")
)

func (a *API) trailerStream(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	// Kid accounts never autoplay trailers.
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, "trailers do not play for Kid accounts")
		return
	}
	if !a.uiSettings(r.Context()).AutoplayTrailers {
		fail(w, http.StatusNotFound, "trailers are turned off")
		return
	}
	lang := trailerLanguage(r)
	now := time.Now()
	ckey := kind + "|" + id + "|" + lang
	if _, ok := a.trailers.get(ckey, now); !ok && !a.trailerStored(r.Context(), ckey, now) && !a.trailers.inFlight(ckey) {
		if _, _, found := ytdlpRun(); !found {
			fail(w, http.StatusNotFound, "no direct stream")
			return
		}
		if !a.trailers.allow(a.person(r).UserID, now) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "too many trailers; try again in a minute")
			return
		}
	}
	s, err := a.resolveTitle(r.Context(), kind, id, lang)
	switch {
	case errors.Is(err, errNoTrailer):
		fail(w, http.StatusNotFound, "no trailer")
	case errors.Is(err, errNoStream):
		fail(w, http.StatusNotFound, "no direct stream")
	case err != nil:
		discoverFail(w, err)
	default:
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, s)
	}
}

func (t *trailerState) inFlight(ckey string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.flights[ckey] != nil
}

// trailerStored loads a kept address from SQLite into memory, so a
// restart does not redo lookups.
func (a *API) trailerStored(ctx context.Context, ckey string, now time.Time) bool {
	if a.s.Store == nil {
		return false
	}
	b, ok, err := store.LookupCache{Store: a.s.Store}.Get(ctx, "trailer|"+ckey, now)
	if err != nil || !ok {
		return false
	}
	var s TrailerStream
	if json.Unmarshal(b, &s) != nil || s.URL == "" || !now.Before(s.ExpiresAt.Add(-trailerEarly)) {
		return false
	}
	a.trailers.put(ckey, trailerEntry{s: s})
	return true
}

// resolveTitle answers one title's stream: memory, then SQLite, then one
// shared lookup (callers asking for the same title wait for the same one).
func (a *API) resolveTitle(ctx context.Context, kind, id, lang string) (TrailerStream, error) {
	ckey := kind + "|" + id + "|" + lang
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
	f := t.flights[ckey]
	if f == nil {
		f = &trailerFlight{done: make(chan struct{})}
		t.flights[ckey] = f
		// The lookup outlives one caller giving up: another may be waiting,
		// and the answer is kept for everyone.
		go func() {
			lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			f.e, f.err = a.lookupTrailer(lctx, kind, id, lang)
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
		return TrailerStream{}, f.err
	}
	if !f.e.miss.IsZero() {
		return TrailerStream{}, errNoStream
	}
	return f.e.s, nil
}

// trailerKey picks the YouTube key: the metadata server's pick first (it
// skips keys it found broken), else our own pick from TMDB's videos.
func (a *API) trailerKey(ctx context.Context, kind, id, lang string) (string, error) {
	if a.s.TrailerPick != nil {
		k, err := a.s.TrailerPick(ctx, kind, id, lang)
		if err == nil && k != "" {
			return k, nil
		}
		if errors.Is(err, metadata.ErrNotFound) {
			return "", errNoTrailer
		}
	}
	d, ok := metadata.DiscovererFor(ctx, a.s.Metadata)
	if !ok {
		return "", errNoStream
	}
	det, err := a.trailerDetails(ctx, d, kind, id)
	if err != nil {
		return "", err
	}
	v, ok := metadata.PickTrailer(det.Videos, lang)
	if !ok {
		return "", errNoTrailer
	}
	return v.Key, nil
}

func (a *API) lookupTrailer(ctx context.Context, kind, id, lang string) (trailerEntry, error) {
	ckey := kind + "|" + id + "|" + lang
	now := time.Now()
	key, err := a.trailerKey(ctx, kind, id, lang)
	if errors.Is(err, errNoTrailer) {
		a.trailers.put(ckey, trailerEntry{miss: now.Add(trailerMiss)})
		return trailerEntry{}, errNoTrailer
	}
	if err != nil {
		return trailerEntry{}, err
	}
	bin, pre, found := ytdlpRun()
	if !found {
		return trailerEntry{}, errNoStream
	}
	t := &a.trailers
	t.mu.Lock()
	if t.pool == nil {
		t.pool = make(chan struct{}, trailerPool)
	}
	pool := t.pool
	t.mu.Unlock()
	select {
	case pool <- struct{}{}:
	case <-ctx.Done():
		return trailerEntry{}, ctx.Err()
	}
	s, err := resolveTrailer(ctx, bin, pre, key, now)
	<-pool
	if err != nil {
		t.failed.Add(1)
		e := trailerEntry{miss: now.Add(trailerMiss)}
		a.trailers.put(ckey, e)
		return e, nil
	}
	t.failed.Store(0)
	e := trailerEntry{s: s}
	a.trailers.put(ckey, e)
	if a.s.Store != nil {
		if b, err := json.Marshal(s); err == nil {
			_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailer|"+ckey, b, now, s.ExpiresAt.Add(-trailerEarly))
		}
	}
	return e, nil
}

// prefetchTrailers looks trailers up ahead of time (Explore's hero and the
// first cards of its first rows), one after another so the pool stays
// free for people, within the install's ahead-of-time budget. Titles
// already known cost nothing.
func (a *API) prefetchTrailers(r *http.Request, titles [][2]string) {
	if len(titles) == 0 || a.kidMax(r) != "" || !a.uiSettings(r.Context()).AutoplayTrailers {
		return
	}
	if _, _, ok := ytdlpRun(); !ok {
		return
	}
	lang := trailerLanguage(r)
	ctx := context.WithoutCancel(r.Context())
	go func() {
		for _, t := range titles {
			ckey := t[0] + "|" + t[1] + "|" + lang
			now := time.Now()
			if _, ok := a.trailers.get(ckey, now); ok || a.trailerStored(ctx, ckey, now) || a.trailers.inFlight(ckey) {
				continue
			}
			if !a.trailers.allowAhead(now) {
				return
			}
			lctx, cancel := context.WithTimeout(ctx, 40*time.Second)
			_, _ = a.resolveTitle(lctx, t[0], t[1], lang)
			cancel()
		}
	}()
}

// trailerDetails reads the title's details through the details cache.
func (a *API) trailerDetails(ctx context.Context, d metadata.Discoverer, kind, id string) (metadata.Details, error) {
	key := "details|" + kind + "|" + id
	if e, ok := a.discover.get(key); ok && e.details != nil && time.Since(e.at) < discoverTTL["details"] {
		return *e.details, nil
	}
	det, err := d.Details(ctx, kind, id)
	if err != nil {
		return det, err
	}
	a.discover.put(key, discoverEntry{at: time.Now(), details: &det})
	return det, nil
}

// googlevideo says whether raw is an https address on YouTube's video servers.
func googlevideo(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.HasSuffix(u.Hostname(), ".googlevideo.com")
}

// trailerExpiry reads the address's expire value (a query value, or a
// path part /expire/<unix>/ in HLS manifests).
func trailerExpiry(raw string, now time.Time) time.Time {
	exp := now.Add(trailerKeep)
	u, err := url.Parse(raw)
	if err != nil {
		return exp
	}
	v := u.Query().Get("expire")
	if v == "" {
		if _, rest, ok := strings.Cut(u.Path, "/expire/"); ok {
			v, _, _ = strings.Cut(rest, "/")
		}
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > now.Unix() {
		exp = time.Unix(n, 0).UTC()
	}
	return exp
}

// resolveTrailer runs yt-dlp (at most trailerTimeout) for one YouTube key
// and returns the HLS manifest (with the muxed MP4 beside it) or, without
// one, the muxed MP4.
func resolveTrailer(ctx context.Context, bin string, pre []string, key string, now time.Time) (TrailerStream, error) {
	ctx, cancel := context.WithTimeout(ctx, trailerTimeout)
	defer cancel()
	args := append(append(append([]string{}, pre...), trailerArgs...), "--", "https://www.youtube.com/watch?v="+key)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil || len(out) > 16<<20 {
		return TrailerStream{}, errNoStream
	}
	var j struct {
		Formats []struct {
			FormatID    string `json:"format_id"`
			URL         string `json:"url"`
			ManifestURL string `json:"manifest_url"`
			Ext         string `json:"ext"`
			VCodec      string `json:"vcodec"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Protocol    string `json:"protocol"`
		} `json:"formats"`
	}
	if json.Unmarshal(out, &j) != nil {
		return TrailerStream{}, errNoStream
	}
	var hls, mp4 string
	var w, h int
	for _, f := range j.Formats {
		if hls == "" && strings.Contains(f.ManifestURL, "/hls_variant/") && googlevideo(f.ManifestURL) {
			hls = f.ManifestURL
		}
		if mp4 == "" && f.FormatID == "18" && f.Ext == "mp4" && strings.HasPrefix(f.VCodec, "avc1") && (f.Protocol == "" || f.Protocol == "https") && googlevideo(f.URL) {
			mp4, w, h = f.URL, f.Width, f.Height
		}
	}
	switch {
	case hls != "":
		s := TrailerStream{Type: "hls", URL: hls, ExpiresAt: trailerExpiry(hls, now), MP4URL: mp4, Width: w, Height: h, Key: key, Audio: true}
		if mp4 != "" {
			if e := trailerExpiry(mp4, now); e.Before(s.ExpiresAt) {
				s.ExpiresAt = e
			}
		}
		return s, nil
	case mp4 != "":
		return TrailerStream{Type: "mp4", URL: mp4, ExpiresAt: trailerExpiry(mp4, now), Width: w, Height: h, Key: key, Audio: true}, nil
	}
	return TrailerStream{}, errNoStream
}
