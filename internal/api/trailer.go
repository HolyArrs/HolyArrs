package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
	"github.com/CoderLuii/holyarrs/internal/youtube"
)

// Trailers in HolyArrs' own player: the title's YouTube trailer, looked up
// by HolyArrs's own reader (internal/youtube) to addresses on YouTube's
// video servers (an adaptive HLS manifest and an H.264 MP4). Only the
// addresses are kept (in memory and in lookup_cache, until shortly before
// they expire), and each title's YouTube key for 30 days. Video is never
// stored; it passes through HolyArrs only on the pass-through relay
// (trailer_proxy.go). Trailers are looked up only by the trailer pass, on
// a fixed schedule (trailer_pass.go); a page open reads what it kept.

const (
	trailerTimeout   = 8 * time.Second
	trailerPerMinute = 12                 // music video resolves a person may start in a minute (cached answers are free)
	trailerKeep      = 4 * time.Hour      // when the address names no expiry
	trailerEarly     = 10 * time.Minute   // dropped this long before it expires
	trailerMiss      = 10 * time.Minute   // a title with no direct stream is not tried again before this
	trailerLongMiss  = 2 * time.Hour      // ...when it has no trailer, or none plays here for a lasting reason
	trailerPool      = 4                  // reader lookups at once
	trailerAhead     = 40                 // ahead-of-time music video lookups per 10 minutes for the whole install
	trailerBadKeep   = 7 * 24 * time.Hour // a video YouTube will not play here is not tried again before this
	trailerMore      = 4                  // other trailers of the title tried in one lookup (5 in all)
)

// trailerReader looks videos up on YouTube; tests point it at a fake.
var trailerReader = &youtube.Reader{}

// TrailerStream is the answer of GET /trailer/{kind}/{tmdb_id}.
type TrailerStream struct {
	// Type is hls (URL is YouTube's adaptive HLS manifest with sound, which
	// starts low and steps up) or mp4 (URL is an H.264 MP4: up to 1080p
	// muxed when offered, else picture only at most 720p).
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	// MP4URL is the MP4 beside an HLS answer, for browsers without native
	// HLS: YouTube's video servers allow no cross-origin reads, so a script
	// player (hls.js) cannot fetch the manifest or its segments.
	MP4URL string `json:"mp4_url,omitempty"`
	// MP4Audio says whether MP4URL has sound.
	MP4Audio bool `json:"mp4_audio,omitempty"`
	Width    int  `json:"width,omitempty"`
	Height   int  `json:"height,omitempty"`
	// Audio is false for a picture-only stream (no sound to turn on).
	Audio bool `json:"audio"`
	// Key is the YouTube video, for the embed fallback.
	Key string `json:"key"`
	// ProxyURL is the same HLS stream relayed by HolyArrs (pass-through),
	// for sound in browsers without native HLS and for viewers whose direct
	// playback is refused. Given per person; never stored.
	ProxyURL string `json:"proxy_url,omitempty"`
	// Duration is the video's length and Peak its most replayed point
	// ("most replayed", the opening skipped), in seconds; 0 when unknown.
	Duration float64 `json:"duration,omitempty"`
	Peak     float64 `json:"peak,omitempty"`
}

type trailerEntry struct {
	s    TrailerStream
	miss time.Time // set: no direct stream, until this time
	// reason is why a miss has no stream (the reader's reason) and key a
	// video the YouTube player may still play ("" when none).
	reason youtube.Reason
	key    string
	// none: the title has no trailer at all.
	none bool
}

// trailerMissed is a miss as an error: it is errNoStream, and says why.
type trailerMissed struct {
	reason youtube.Reason
	key    string
}

func (e *trailerMissed) Error() string        { return "no direct stream" }
func (e *trailerMissed) Is(target error) bool { return target == errNoStream }

func (e trailerEntry) missErr() error {
	if e.none {
		return errNoTrailer
	}
	return &trailerMissed{reason: e.reason, key: e.key}
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
	// failed counts lookups that failed in a row at YouTube's door (a
	// success resets; a video YouTube will not play here does not count);
	// the trailer pass stops on one.
	failed atomic.Int32
	// For the health check, kept across restarts (trailerUpkeepKey): the
	// different titles and music videos that failed since the last good
	// lookup (upkeep, by readTrailer's upkeep key), the
	// last of those failures' reason and the last lookup that found
	// addresses. saved is when they were last written.
	upkeep     []string
	lastReason youtube.Reason
	lastGood   time.Time
	saved      time.Time
	loaded     sync.Once
	saving     sync.Mutex
	// bad keeps videos YouTube will not play here (by key) until a time.
	bad map[string]trailerBadEntry
}

// lookupPool is the shared pool of reader lookups.
func (t *trailerState) lookupPool() chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pool == nil {
		t.pool = make(chan struct{}, trailerPool)
	}
	return t.pool
}

const (
	trailerUpkeepKey   = "trailerhealth|reader" // the health check's state in lookup_cache
	trailerUpkeepKeep  = 90 * 24 * time.Hour    // ...kept this long after its last write
	trailerUpkeepVideo = 50                     // different failing titles and videos remembered
)

// trailerUpkeep is the health check's state as kept in lookup_cache.
type trailerUpkeep struct {
	Videos   []string       `json:"videos,omitempty"`
	Reason   youtube.Reason `json:"reason,omitempty"`
	LastGood time.Time      `json:"last_good,omitzero"`
}

// upkeepReason says whether a failure for r counts toward the health
// check: every reason but one that is plainly about that video alone (age,
// sign-in, a content warning, a bad key). When YouTube changes, videos that
// played before come back without an address, refused, behind a bot check,
// unplayable or unavailable, across many videos at once.
func upkeepReason(r youtube.Reason) bool {
	switch r {
	case youtube.AgeRestricted, youtube.SignInNeeded, youtube.ContentCheck, youtube.BadID:
		return false
	}
	return true
}

// note counts a lookup's outcome under key (a title, or a music video): a
// success resets both counts, a door failure adds one to the failures in a
// row, and a failure for an upkeep reason adds key to the different ones
// that failed, so a title whose trailers all fail counts once. It
// says whether the health state should be written now: on a change, or
// at most once a minute for a success that changed nothing but the time.
func (t *trailerState) note(key string, err error, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.failed.Store(0)
		t.lastGood = now
		if len(t.upkeep) > 0 {
			t.upkeep = nil
			return true
		}
		return now.Sub(t.saved) >= time.Minute
	}
	r := trailerReason(err)
	if r == "" {
		r = youtube.Network
	}
	if !r.VideoReason() {
		t.failed.Add(1)
	}
	if !upkeepReason(r) {
		return false
	}
	changed := r != t.lastReason
	t.lastReason = r
	if !slices.Contains(t.upkeep, key) && len(t.upkeep) < trailerUpkeepVideo {
		t.upkeep = append(t.upkeep, key)
		changed = true
	}
	return changed
}

// loadUpkeep reads the health state kept before a restart, once, before
// anything counts.
func (a *API) loadUpkeep(ctx context.Context) {
	t := &a.trailers
	t.loaded.Do(func() {
		if a.s.Store == nil || a.s.Store.Read() == nil {
			return
		}
		b, ok, err := store.LookupCache{Store: a.s.Store}.Get(context.WithoutCancel(ctx), trailerUpkeepKey, time.Now())
		var k trailerUpkeep
		if err != nil || !ok || json.Unmarshal(b, &k) != nil {
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		t.upkeep = k.Videos[:min(len(k.Videos), trailerUpkeepVideo)]
		t.lastReason, t.lastGood = k.Reason, k.LastGood
	})
}

// saveUpkeep writes the health state as it is now (one write at a time, so
// an older state never lands after a newer one).
func (a *API) saveUpkeep(ctx context.Context) {
	if a.s.Store == nil {
		return
	}
	t := &a.trailers
	t.saving.Lock()
	defer t.saving.Unlock()
	now := time.Now()
	t.mu.Lock()
	k := trailerUpkeep{Videos: slices.Clone(t.upkeep), Reason: t.lastReason, LastGood: t.lastGood}
	t.saved = now
	t.mu.Unlock()
	if b, err := json.Marshal(k); err == nil {
		_ = store.LookupCache{Store: a.s.Store}.Put(context.WithoutCancel(ctx), trailerUpkeepKey, b, now, now.Add(trailerUpkeepKeep))
	}
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

// TrailerReaderState is the YouTube reader's state for the health check.
type TrailerReaderState struct {
	Failed int // lookups that failed in a row at YouTube's door
	// Videos is how many different titles and music videos failed since the
	// last good lookup, for any reason but one about that video alone
	// (upkeepReason); a title counts once, however many trailers it tried.
	Videos     int
	LastGood   time.Time      // the last lookup that found addresses (zero: none yet)
	LastReason youtube.Reason // the reason of the last failure Videos counts ("" when none)
}

// TrailersOn says whether trailers are looked up at all: every YouTube
// lookup (the trailer pass, music videos) waits on trailer autoplay, so the
// health check is off while it is off.
func (a *API) TrailersOn(ctx context.Context) bool {
	return a.s.Store == nil || a.uiSettings(ctx).AutoplayTrailers
}

// TrailerReader reports the reader's state (for the health check), with
// what was kept before a restart.
func (a *API) TrailerReader() TrailerReaderState {
	a.loadUpkeep(context.Background())
	t := &a.trailers
	t.mu.Lock()
	defer t.mu.Unlock()
	return TrailerReaderState{Failed: int(t.failed.Load()), Videos: len(t.upkeep), LastGood: t.lastGood, LastReason: t.lastReason}
}

var trailerLangRe = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// trailerLanguage is the two-letter language of ?language= ("de" or
// "de-DE"; anything else is ignored) or else the first language of
// Accept-Language ("de" from "de-DE,de;q=0.9"), so only [a-z]{2} reaches
// the cache keys.
func trailerLanguage(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("language")); trailerLangRe.MatchString(l) {
		return l[:2]
	}
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	first, _, _ = strings.Cut(strings.TrimSpace(first), ";")
	first, _, _ = strings.Cut(first, "-")
	if first = strings.ToLower(first); trailerLangRe.MatchString(first) {
		return first
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
	// Only what the trailer pass kept (memory, then SQLite): a page open
	// never starts a lookup.
	lang := trailerLanguage(r)
	e, ok := a.keptTrailer(r.Context(), kind+"|"+id+"|"+lang, time.Now())
	w.Header().Set("Cache-Control", "private, no-store")
	switch {
	case !ok:
		// Not looked up yet: noted for the next trailer pass, and the key
		// learned in the background (no YouTube call); the kept key is for
		// YouTube's player meanwhile.
		a.noteTrailerTitles(lang, [][2]string{{kind, id}})
		a.warmTitles(lang, [][2]string{{kind, id}})
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no direct stream", "key": a.knownTrailerKey(r.Context(), kind, id, lang)})
	case e.none:
		fail(w, http.StatusNotFound, "no trailer")
	case !e.miss.IsZero():
		// A miss says why (the reader's reason) and which video the YouTube
		// player may still play ("" when every one tried will not play).
		body := map[string]string{"error": "no direct stream"}
		if e.reason != "" {
			body["reason"], body["key"] = string(e.reason), e.key
		}
		writeJSON(w, http.StatusNotFound, body)
	default:
		s := e.s
		if s.Type == "hls" {
			if id := a.trailerRelay.register(a.person(r).UserID, s.URL, s.ExpiresAt); id != "" {
				s.ProxyURL = urlbase.From(r.Context()) + "/api/v1/trailer/stream/" + id + "/manifest.m3u8"
			}
		}
		writeJSON(w, http.StatusOK, s)
	}
}

// keptTrailer answers a title's kept entry (memory, then SQLite); no lookup.
func (a *API) keptTrailer(ctx context.Context, ckey string, now time.Time) (trailerEntry, bool) {
	e, ok := a.trailers.get(ckey, now)
	if !ok && a.trailerStored(ctx, ckey, now) {
		e, ok = a.trailers.get(ckey, now)
	}
	return e, ok
}

func (t *trailerState) inFlight(ckey string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.flights[ckey] != nil
}

// trailerStored loads a kept address from SQLite into memory, so a
// restart does not redo lookups.
func (a *API) trailerStored(ctx context.Context, ckey string, now time.Time) bool {
	if a.s.Store == nil || a.s.Store.Read() == nil {
		return false
	}
	b, ok, err := store.LookupCache{Store: a.s.Store}.Get(ctx, "trailer|"+ckey, now)
	if err != nil || !ok {
		return false
	}
	var s TrailerStream
	// A kept row is checked again as if new: both addresses on YouTube's
	// video servers and not yet (nearly) expired, by the row and by the
	// address's own expire value.
	if json.Unmarshal(b, &s) != nil || !googlevideo(s.URL) || (s.MP4URL != "" && !googlevideo(s.MP4URL)) ||
		!now.Before(s.ExpiresAt.Add(-trailerEarly)) || urlExpired(s.URL, now) || (s.MP4URL != "" && urlExpired(s.MP4URL, now)) {
		return false
	}
	a.trailers.put(ckey, trailerEntry{s: s})
	return true
}

// resolveTitle answers one title's stream: memory, then SQLite, then one
// shared lookup (lookupShared).
func (a *API) resolveTitle(ctx context.Context, kind, id, lang string) (TrailerStream, error) {
	if e, ok := a.keptTrailer(ctx, kind+"|"+id+"|"+lang, time.Now()); ok {
		if !e.miss.IsZero() {
			return TrailerStream{}, e.missErr()
		}
		return e.s, nil
	}
	return a.lookupShared(ctx, kind, id, lang)
}

// lookupShared looks the title up even when an address is kept (the
// trailer pass renews addresses before they expire); callers asking for
// the same title wait for the same lookup.
func (a *API) lookupShared(ctx context.Context, kind, id, lang string) (TrailerStream, error) {
	ckey := kind + "|" + id + "|" + lang
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
		return TrailerStream{}, f.e.missErr()
	}
	return f.e.s, nil
}

// trailerKey picks the YouTube key: the metadata server's pick first (it
// skips keys it found broken), else our own pick from TMDB's videos.
func (a *API) trailerKey(ctx context.Context, kind, id, lang string) (string, error) {
	if k := a.knownTrailerKey(ctx, kind, id, lang); k != "" {
		return k, nil
	}
	k, err := a.pickTrailerKey(ctx, kind, id, lang)
	if err == nil && k != "" {
		a.keepTrailerKey(ctx, kind, id, lang, k)
	}
	return k, err
}

const (
	trailerKeyKeep = 30 * 24 * time.Hour // a title's YouTube key is kept this long
	trailerKeyMiss = 10 * time.Minute    // an unknown key is not read from SQLite again before this
)

type trailerKeyEntry struct {
	key string
	at  time.Time
}

// trailerKeyCache keeps titles' YouTube keys in memory before lookup_cache.
type trailerKeyCache struct {
	mu sync.Mutex
	m  map[string]trailerKeyEntry
}

// knownTrailerKey answers the kept key, "" when none is kept (no lookup).
func (a *API) knownTrailerKey(ctx context.Context, kind, id, lang string) string {
	ck := kind + "|" + id + "|" + lang
	now := time.Now()
	c := &a.trailerKeys
	c.mu.Lock()
	e, ok := c.m[ck]
	c.mu.Unlock()
	if ok && ((e.key != "" && now.Sub(e.at) < trailerKeyKeep) || (e.key == "" && now.Sub(e.at) < trailerKeyMiss)) {
		return e.key
	}
	e = trailerKeyEntry{at: now}
	if a.s.Store != nil {
		if b, found, err := (store.LookupCache{Store: a.s.Store}).Get(ctx, "trailerkey|"+ck, now); err == nil && found {
			e.key = string(b)
		}
	}
	c.put(ck, e)
	return e.key
}

func (c *trailerKeyCache) put(ck string, e trailerKeyEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 20000 {
		c.m = map[string]trailerKeyEntry{}
	}
	c.m[ck] = e
}

func (a *API) keepTrailerKey(ctx context.Context, kind, id, lang, key string) {
	ck := kind + "|" + id + "|" + lang
	now := time.Now()
	a.trailerKeys.put(ck, trailerKeyEntry{key: key, at: now})
	if a.s.Store != nil {
		_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailerkey|"+ck, []byte(key), now, now.Add(trailerKeyKeep))
	}
}

// fillTrailerKeys sets trailer_key on Explore cards whose key is kept.
func (a *API) fillTrailerKeys(r *http.Request, items []discoverItemView) {
	lang := trailerLanguage(r)
	// Every listed title is warmed ahead (details, extras, trailer key), so
	// opening or hovering it never makes the first outside call.
	warm := make([][2]string, 0, len(items))
	for _, v := range items {
		if v.Kind == "movie" || v.Kind == "show" {
			warm = append(warm, [2]string{v.Kind, v.TMDBID})
		}
	}
	a.warmTitles(lang, warm)
	if a.kidMax(r) != "" {
		return
	}
	for i := range items {
		if v := &items[i]; (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" {
			v.TrailerKey = a.knownTrailerKey(r.Context(), v.Kind, v.TMDBID, lang)
		}
	}
}

func (a *API) pickTrailerKey(ctx context.Context, kind, id, lang string) (string, error) {
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
	if errors.Is(err, metadata.ErrNotFound) {
		// TMDB has no such title: kept as "no trailer", so a background
		// pass does not stop on it every time.
		return "", errNoTrailer
	}
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
		if e := a.missOrKept(ctx, ckey, trailerEntry{miss: now.Add(trailerLongMiss), none: true}, now); e.miss.IsZero() {
			return e, nil
		}
		return trailerEntry{}, errNoTrailer
	}
	if err != nil {
		return trailerEntry{}, err
	}
	// The picked key first, unless YouTube would not play it here lately;
	// when a video will not play (age, removed, blocked), the title's next
	// trailers are tried, at most trailerMore of them.
	miss := trailerEntry{miss: now.Add(trailerMiss)}
	var cands []string
	if why, bad := a.trailerBad(ctx, key, now); bad {
		miss.reason = why
	} else {
		cands = append(cands, key)
	}
	listed, more, sawUnplayable := false, 0, false
	for i := 0; ; i++ {
		if i >= len(cands) && !listed {
			listed = true
			cands = append(cands, a.moreTrailerKeys(ctx, kind, id, lang, key, now)...)
		}
		if i >= len(cands) {
			break
		}
		k := cands[i]
		if k != key {
			if more == trailerMore {
				// Nothing played: this untried one may still play in YouTube's player.
				miss.key = k
				break
			}
			more++
		}
		s, err := a.readTrailer(ctx, k, ckey, now, false)
		if err == nil {
			// A next trailer that plays is not kept as the title's key: the
			// picked one is asked again once its bad mark is over.
			e := trailerEntry{s: s}
			a.trailers.put(ckey, e)
			if a.s.Store != nil {
				if b, err := json.Marshal(s); err == nil {
					_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailer|"+ckey, b, now, s.ExpiresAt.Add(-trailerEarly))
				}
			}
			return e, nil
		}
		if ctx.Err() != nil && youtube.ReasonOf(err) == "" {
			return trailerEntry{}, ctx.Err()
		}
		miss.reason = trailerReason(err)
		if !miss.reason.VideoReason() {
			// YouTube's door failed: another trailer would fail the same
			// way, and YouTube's player may still play this one.
			miss.key = k
			break
		}
		// Unplayable may not last (a premiere not on yet, say): only the
		// normal miss, so it is asked again after trailerMiss.
		if miss.reason != youtube.Unplayable {
			a.markTrailerBad(ctx, k, miss.reason, now)
		} else {
			sawUnplayable = true
		}
	}
	// No trailer plays here for a lasting reason: the pass asks again after
	// trailerLongMiss. YouTube's door failing, or a premiere not on yet, is
	// asked again on the next pass.
	if miss.reason.VideoReason() && miss.reason != youtube.Unplayable && !sawUnplayable {
		miss.miss = now.Add(trailerLongMiss)
	}
	return a.missOrKept(ctx, ckey, miss, now), nil
}

// missOrKept keeps a lookup's miss, unless an address from an earlier
// lookup is still good: a renewal that fails keeps it until it expires.
func (a *API) missOrKept(ctx context.Context, ckey string, miss trailerEntry, now time.Time) trailerEntry {
	if e, ok := a.keptTrailer(ctx, ckey, now); ok && e.miss.IsZero() {
		return e
	}
	a.trailers.put(ckey, miss)
	return miss
}

// trailerReason is the reader's reason for err, except that YouTube's
// "try again later" (it refuses this server for a while, often an hour)
// is its door refusing, not the video.
func trailerReason(err error) youtube.Reason {
	r := youtube.ReasonOf(err)
	var e *youtube.Error
	if r.VideoReason() && errors.As(err, &e) && strings.Contains(strings.ToLower(e.Detail), "try again later") {
		return youtube.Refused
	}
	return r
}

// readTrailer looks one video up in the shared pool and counts the outcome
// for the health check under upkeep (the title's "kind|id|lang", or the
// video key for a music video). withPeak also reads the "most replayed" point (the
// watch page, about a second); a trailer needs only the addresses.
func (a *API) readTrailer(ctx context.Context, key, upkeep string, now time.Time, withPeak bool) (TrailerStream, error) {
	a.loadUpkeep(ctx)
	pool := a.trailers.lookupPool()
	select {
	case pool <- struct{}{}:
	case <-ctx.Done():
		return TrailerStream{}, ctx.Err()
	}
	s, err := resolveTrailer(ctx, key, now, withPeak)
	<-pool
	if a.trailers.note(upkeep, err, time.Now()) {
		a.saveUpkeep(ctx)
	}
	return s, err
}

// trailerBadEntry is a video YouTube will not play here, until a time.
type trailerBadEntry struct {
	until  time.Time
	reason youtube.Reason
}

// trailerBad says whether YouTube would not play the video here lately,
// and why (memory, then lookup_cache).
func (a *API) trailerBad(ctx context.Context, key string, now time.Time) (youtube.Reason, bool) {
	t := &a.trailers
	t.mu.Lock()
	e, ok := t.bad[key]
	t.mu.Unlock()
	if ok && now.Before(e.until) {
		return e.reason, true
	}
	if a.s.Store == nil || a.s.Store.Read() == nil {
		return "", false
	}
	b, found, err := store.LookupCache{Store: a.s.Store}.Get(ctx, "trailerbad|"+key, now)
	if err != nil || !found {
		return "", false
	}
	return youtube.Reason(b), true
}

// markTrailerBad keeps that YouTube will not play the video here (the
// reason is the row's body) for trailerBadKeep.
func (a *API) markTrailerBad(ctx context.Context, key string, reason youtube.Reason, now time.Time) {
	t := &a.trailers
	t.mu.Lock()
	if t.bad == nil || len(t.bad) > 5000 {
		t.bad = map[string]trailerBadEntry{}
	}
	t.bad[key] = trailerBadEntry{until: now.Add(trailerBadKeep), reason: reason}
	t.mu.Unlock()
	if a.s.Store != nil {
		_ = store.LookupCache{Store: a.s.Store}.Put(ctx, "trailerbad|"+key, []byte(reason), now, now.Add(trailerBadKeep))
	}
}

// moreTrailerKeys are the title's other trailers from TMDB's videos, in
// PickTrailer's order, without first or ones marked bad: up to trailerMore
// to try and one more for YouTube's player.
func (a *API) moreTrailerKeys(ctx context.Context, kind, id, lang, first string, now time.Time) []string {
	d, ok := metadata.DiscovererFor(ctx, a.s.Metadata)
	if !ok {
		return nil
	}
	det, err := a.trailerDetails(ctx, d, kind, id)
	if err != nil {
		return nil
	}
	var left []metadata.DetailVideo
	for _, v := range det.Videos {
		if _, bad := a.trailerBad(ctx, v.Key, now); v.Key != first && !bad {
			left = append(left, v)
		}
	}
	var out []string
	for len(out) <= trailerMore {
		v, ok := metadata.PickTrailer(left, lang)
		if !ok {
			break
		}
		out = append(out, v.Key)
		left = slices.DeleteFunc(left, func(x metadata.DetailVideo) bool { return x.Key == v.Key })
	}
	return out
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

// urlExpired says whether the address carries an expire value that is
// past or nearly so (an address without one is judged by its row).
func urlExpired(raw string, now time.Time) bool {
	if e := trailerExpiry(raw, time.Unix(0, 0)); e.Unix() > int64(trailerKeep/time.Second) {
		return !now.Before(e.Add(-trailerEarly))
	}
	return false
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

// resolveTrailer looks one YouTube key up with the reader (at most
// trailerTimeout) and returns the HLS manifest (with an MP4 beside it) or,
// without one, the MP4. A failure is the reader's *youtube.Error.
func resolveTrailer(ctx context.Context, key string, now time.Time, withPeak bool) (TrailerStream, error) {
	ctx, cancel := context.WithTimeout(ctx, trailerTimeout)
	defer cancel()
	look := trailerReader.Addresses
	if withPeak {
		look = trailerReader.Look
	}
	v, err := look(ctx, key)
	if err != nil {
		return TrailerStream{}, err
	}
	hls := ""
	if strings.Contains(v.HLS, "/hls_variant/") && googlevideo(v.HLS) {
		hls = v.HLS
	}
	var mp4 string
	var w, h, rank int
	audio := false
	for _, f := range v.Formats {
		if !f.AVC || f.Height > 1080 || !googlevideo(f.URL) {
			continue
		}
		// The muxed MP4 (sound) first, else the tallest picture-only one.
		// Up to 1080p with sound (rare on YouTube; HLS is the usual 1080p
		// path), picture-only stays at 720p.
		if !f.Audio && f.Height > 720 {
			continue
		}
		r := f.Height
		if f.Audio {
			r += 10000
		}
		if r > rank {
			mp4, w, h, rank, audio = f.URL, f.Width, f.Height, r, f.Audio
		}
	}
	dur := max(v.Duration, 0)
	// The most replayed point (the reader skips the opening, which every
	// play starts on).
	peak := 0.0
	if v.PeakFound && v.Peak >= 0 && (dur <= 0 || v.Peak < dur) {
		peak = v.Peak
	}
	switch {
	case hls != "":
		s := TrailerStream{Type: "hls", URL: hls, ExpiresAt: trailerExpiry(hls, now), MP4URL: mp4, MP4Audio: audio, Width: w, Height: h, Key: key, Audio: true, Duration: dur, Peak: peak}
		if mp4 != "" {
			if e := trailerExpiry(mp4, now); e.Before(s.ExpiresAt) {
				s.ExpiresAt = e
			}
		}
		return s, nil
	case mp4 != "":
		return TrailerStream{Type: "mp4", URL: mp4, ExpiresAt: trailerExpiry(mp4, now), Width: w, Height: h, Key: key, Audio: audio, Duration: dur, Peak: peak}, nil
	}
	return TrailerStream{}, &youtube.Error{Reason: youtube.NoAddress, Detail: "no H.264 or HLS address on YouTube's video servers"}
}
