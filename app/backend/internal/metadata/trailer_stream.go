package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

// Trailers in HolyArrs' own player: the title's YouTube trailer, looked up
// by HolyArrs's own reader (metadata/youtube) to addresses on YouTube's
// video servers (an adaptive HLS manifest and an H.264 MP4). Only the
// addresses are kept (in memory and in lookup_cache, until shortly before
// they expire), and each title's YouTube key for 30 days. Video is never
// stored; it passes through HolyArrs only on the pass-through relay
// (trailer_relay.go). Trailers are looked up only by the trailer pass, on
// a fixed schedule (trailer_pass.go); a page open reads what it kept.

const (
	trailerTimeout        = 8 * time.Second
	trailerPerMinute      = 12                 // music video resolves a person may start in a minute (cached answers are free)
	trailerKeep           = 4 * time.Hour      // when the address names no expiry
	trailerLongMiss       = 2 * time.Hour      // ...when it has no trailer, or none plays here for a lasting reason
	trailerUnplayableMiss = time.Hour          // ...when one is blocked here or a premiere not on yet
	trailerPool           = 4                  // reader lookups at once
	trailerBadKeep        = 7 * 24 * time.Hour // a video YouTube will not play here is not tried again before this
	trailerMore           = 4                  // other trailers of the title tried in one lookup (up to 5 in all, 4 when the first is marked bad)
)

const (
	// TrailerEarly is how long before it expires a kept address is dropped.
	TrailerEarly = 10 * time.Minute
	// TrailerMiss is how long a title with no direct stream is not tried again.
	TrailerMiss = 10 * time.Minute
)

var (
	// ErrNoTrailer says a title has no trailer at all.
	ErrNoTrailer = errors.New("no trailer")
	// ErrNoStream says a title or video has no direct stream.
	ErrNoStream = errors.New("no direct stream")
)

// trailerReader looks videos up on YouTube; tests point it at a fake.
var trailerReader = &youtube.Reader{}

var trailerLangRe = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

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

// TrailerEntry is what is kept for a title or a music video: a stream, or
// a miss until a time.
type TrailerEntry struct {
	Stream TrailerStream
	// Miss, when set, means no direct stream until this time.
	Miss time.Time
	// Reason is why a miss has no stream (the reader's reason) and Key a
	// video the YouTube player may still play ("" when none).
	Reason youtube.Reason
	Key    string
	// None: the title has no trailer at all.
	None bool
}

// TrailerMissed is a miss as an error: it is ErrNoStream, and says why.
type TrailerMissed struct {
	Reason youtube.Reason
	Key    string
}

func (e *TrailerMissed) Error() string        { return "no direct stream" }
func (e *TrailerMissed) Is(target error) bool { return target == ErrNoStream }

// MissErr is the entry's miss as an error.
func (e TrailerEntry) MissErr() error {
	if e.None {
		return ErrNoTrailer
	}
	return &TrailerMissed{Reason: e.Reason, Key: e.Key}
}

// Trailers keeps resolved trailer and music video addresses, each title's
// YouTube key and the reader's health, serves the trailer routes and runs
// the trailer pass.
type Trailers struct {
	Store *database.Store
	// Version is HolyArrs' version; bad marks are kept per version.
	Version string
	// Provider is the title source in use, read at call time.
	Provider func() Provider
	// Pick reads the metadata server's trailer key, read at call time (nil
	// when there is none).
	Pick func() func(ctx context.Context, kind, id, lang string) (string, error)
	// Explore reads a title's details through the shared cache.
	Explore  ExploreCache
	Settings UISettings
	Ages     AgeLimits
	People   People
	// KindsOn lists the kinds (movie, show) whose parts are on, of kind
	// ("" for both).
	KindsOn func(kind string) []string
	Wants   TrailerWants
	Videos  ArtistVideos

	mu      sync.Mutex
	cache   map[string]TrailerEntry
	budget  map[int64][]time.Time
	flights map[string]*trailerFlight
	pool    chan struct{}
	// failed counts lookups that failed in a row at YouTube's door (a
	// success resets; a video YouTube will not play here does not count);
	// the trailer pass stops on one.
	failed atomic.Int32
	// For the health check, kept across restarts (trailerUpkeepKey): the
	// different titles and music videos that failed since the last good
	// lookup (upkeep, by Read's upkeep key), the last of those failures'
	// reason and the last lookup that found addresses. saved is when they
	// were last written.
	upkeep     []string
	lastReason youtube.Reason
	lastGood   time.Time
	saved      time.Time
	loaded     sync.Once
	saving     sync.Mutex
	// bad keeps videos YouTube will not play here (by key) until a time.
	bad map[string]trailerBadEntry

	keys  trailerKeyCache
	relay trailerRelay
	asked trailerAsked
}

type trailerFlight struct {
	done chan struct{}
	e    TrailerEntry
	err  error
}

// UseYouTube points the trailer reader at base, a stand-in YouTube for
// the browser tests (main checks the address first). Call it before the
// trailer pass starts.
func UseYouTube(base string) {
	trailerReader = &youtube.Reader{Base: base}
}

// UseReader swaps the trailer reader (for tests) and answers the one it
// replaced.
func UseReader(r *youtube.Reader) *youtube.Reader {
	old := trailerReader
	trailerReader = r
	return old
}

// Reader is the trailer reader in use.
func Reader() *youtube.Reader { return trailerReader }

// lookupPool is the shared pool of reader lookups.
func (t *Trailers) lookupPool() chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pool == nil {
		t.pool = make(chan struct{}, trailerPool)
	}
	return t.pool
}

// Get answers a kept entry from memory; an expired one is dropped.
func (t *Trailers) Get(key string, now time.Time) (TrailerEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.cache[key]
	if !ok {
		return e, false
	}
	if (!e.Miss.IsZero() && now.After(e.Miss)) || (e.Miss.IsZero() && now.After(e.Stream.ExpiresAt.Add(-TrailerEarly))) {
		delete(t.cache, key)
		return e, false
	}
	return e, true
}

// Put keeps an entry in memory.
func (t *Trailers) Put(key string, e TrailerEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = map[string]TrailerEntry{}
	}
	if len(t.cache) > 2000 {
		clear(t.cache)
	}
	t.cache[key] = e
}

// Allow spends one of the person's resolves for this minute.
func (t *Trailers) Allow(user int64, now time.Time) bool {
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

// Failed is how many lookups failed in a row at YouTube's door.
func (t *Trailers) Failed() int32 { return t.failed.Load() }

// InFlight says whether a lookup of ckey is under way.
func (t *Trailers) InFlight(ckey string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.flights[ckey] != nil
}

// TrailerLanguage is the two-letter language of ?language= ("de" or
// "de-DE"; anything else is ignored) or else the first language of
// Accept-Language ("de" from "de-DE,de;q=0.9"), so only [a-z]{2} reaches
// the cache keys.
func TrailerLanguage(r *http.Request) string {
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

// Kept answers a title's or video's kept entry (memory, then SQLite); no
// lookup.
func (t *Trailers) Kept(ctx context.Context, ckey string, now time.Time) (TrailerEntry, bool) {
	e, ok := t.Get(ckey, now)
	if !ok && t.Stored(ctx, ckey, now) {
		e, ok = t.Get(ckey, now)
	}
	return e, ok
}

// Stored loads a kept address from SQLite into memory, so a restart does
// not redo lookups.
func (t *Trailers) Stored(ctx context.Context, ckey string, now time.Time) bool {
	if t.Store == nil || t.Store.Read() == nil {
		return false
	}
	b, ok, err := LookupCache{Store: t.Store}.Get(ctx, "trailer|"+ckey, now)
	if err != nil || !ok {
		return false
	}
	var s TrailerStream
	// A kept row is checked again as if new: both addresses on YouTube's
	// video servers and not yet (nearly) expired, by the row and by the
	// address's own expire value.
	if json.Unmarshal(b, &s) != nil || !Googlevideo(s.URL) || (s.MP4URL != "" && !Googlevideo(s.MP4URL)) ||
		!now.Before(s.ExpiresAt.Add(-TrailerEarly)) || urlExpired(s.URL, now) || (s.MP4URL != "" && urlExpired(s.MP4URL, now)) {
		return false
	}
	t.Put(ckey, TrailerEntry{Stream: s})
	return true
}

// KeepStream keeps a found stream in memory and SQLite.
func (t *Trailers) KeepStream(ctx context.Context, ckey string, s TrailerStream, now time.Time) TrailerEntry {
	e := TrailerEntry{Stream: s}
	t.Put(ckey, e)
	if t.Store != nil {
		if b, err := json.Marshal(s); err == nil {
			_ = LookupCache{Store: t.Store}.Put(ctx, "trailer|"+ckey, b, now, s.ExpiresAt.Add(-TrailerEarly))
		}
	}
	return e
}

// ResolveTitle answers one title's stream: memory, then SQLite, then one
// shared lookup (LookupShared).
func (t *Trailers) ResolveTitle(ctx context.Context, kind, id, lang string) (TrailerStream, error) {
	if e, ok := t.Kept(ctx, kind+"|"+id+"|"+lang, time.Now()); ok {
		if !e.Miss.IsZero() {
			return TrailerStream{}, e.MissErr()
		}
		return e.Stream, nil
	}
	return t.LookupShared(ctx, kind, id, lang)
}

// LookupShared looks the title up even when an address is kept (the
// trailer pass renews addresses before they expire); callers asking for
// the same title wait for the same lookup.
func (t *Trailers) LookupShared(ctx context.Context, kind, id, lang string) (TrailerStream, error) {
	e, err, done := t.Shared(ctx, kind+"|"+id+"|"+lang, func(lctx context.Context) (TrailerEntry, error) {
		return t.lookupTrailer(lctx, kind, id, lang)
	})
	if !done {
		return TrailerStream{}, ctx.Err()
	}
	if err != nil {
		return TrailerStream{}, err
	}
	if !e.Miss.IsZero() {
		return TrailerStream{}, e.MissErr()
	}
	return e.Stream, nil
}

// Shared runs look once for everyone asking for ckey at the same time;
// done is false when ctx ended first. The lookup outlives one caller
// giving up: another may be waiting, and the answer is kept for everyone.
func (t *Trailers) Shared(ctx context.Context, ckey string, look func(ctx context.Context) (TrailerEntry, error)) (e TrailerEntry, err error, done bool) {
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
			f.e, f.err = look(lctx)
			t.mu.Lock()
			delete(t.flights, ckey)
			t.mu.Unlock()
			close(f.done)
		}()
	}
	t.mu.Unlock()
	select {
	case <-ctx.Done():
		return TrailerEntry{}, ctx.Err(), false
	case <-f.done:
	}
	return f.e, f.err, true
}

func (t *Trailers) lookupTrailer(ctx context.Context, kind, id, lang string) (TrailerEntry, error) {
	ckey := kind + "|" + id + "|" + lang
	now := time.Now()
	key, err := t.Key(ctx, kind, id, lang)
	if errors.Is(err, ErrNoTrailer) {
		if e := t.missOrKept(ctx, ckey, TrailerEntry{Miss: now.Add(trailerLongMiss), None: true}, now); e.Miss.IsZero() {
			return e, nil
		}
		return TrailerEntry{}, ErrNoTrailer
	}
	if err != nil {
		return TrailerEntry{}, err
	}
	// The picked key first, unless YouTube would not play it here lately;
	// when a video will not play (age, removed, blocked), the title's next
	// trailers are tried, at most trailerMore of them.
	miss := TrailerEntry{Miss: now.Add(TrailerMiss)}
	var cands []string
	if why, bad := t.Bad(ctx, key, now); bad {
		miss.Reason = why
	} else {
		cands = append(cands, key)
	}
	listed, more, sawUnplayable := false, 0, false
	for i := 0; ; i++ {
		if i >= len(cands) && !listed {
			listed = true
			cands = append(cands, t.moreKeys(ctx, kind, id, lang, key, now)...)
		}
		if i >= len(cands) {
			break
		}
		k := cands[i]
		if k != key {
			if more == trailerMore {
				// Nothing played: this untried one may still play in YouTube's player.
				miss.Key = k
				break
			}
			more++
		}
		s, err := t.Read(ctx, k, upkeepTitle(ckey), now, false)
		if err == nil {
			// A next trailer that plays is not kept as the title's key: the
			// picked one is asked again once its bad mark is over.
			return t.KeepStream(ctx, ckey, s, now), nil
		}
		if ctx.Err() != nil && youtube.ReasonOf(err) == "" {
			return TrailerEntry{}, ctx.Err()
		}
		miss.Reason = TrailerReasonOf(err)
		if !miss.Reason.VideoReason() {
			// YouTube's door failed: another trailer would fail the same
			// way, and YouTube's player may still play this one.
			miss.Key = k
			break
		}
		// Unplayable may not last (a premiere not on yet, say): only the
		// normal miss, so it is asked again after TrailerMiss.
		// A removed video found on a lookup Health does not count (a first
		// lookup while trailers play fine) is not marked either: if the
		// reader itself broke that way, the next lookups must reach it.
		switch {
		case miss.Reason == youtube.Unplayable:
			sawUnplayable = true
		case upkeepReason(miss.Reason) && !t.countsVideo(isRenewal(ctx), now):
		default:
			t.MarkBad(ctx, k, miss.Reason, now)
		}
	}
	// No trailer plays here for a lasting reason: the pass asks again after
	// trailerLongMiss. YouTube's door failing, or a premiere not on yet, is
	// asked again on the next pass.
	if miss.Reason.VideoReason() && miss.Reason != youtube.Unplayable && !sawUnplayable {
		miss.Miss = now.Add(trailerLongMiss)
	} else if miss.Reason.VideoReason() {
		// Blocked here, or a premiere not on yet: asked again within the
		// hour, not every pass (each ask tries all of the title's trailers).
		miss.Miss = now.Add(trailerUnplayableMiss)
	}
	return t.missOrKept(ctx, ckey, miss, now), nil
}

// missOrKept keeps a lookup's miss, unless an address from an earlier
// lookup is still good: a renewal that fails keeps it until it expires.
func (t *Trailers) missOrKept(ctx context.Context, ckey string, miss TrailerEntry, now time.Time) TrailerEntry {
	if e, ok := t.Kept(ctx, ckey, now); ok && e.Miss.IsZero() {
		return e
	}
	t.Put(ckey, miss)
	return miss
}

// TrailerReasonOf is the reader's reason for err, except that YouTube's
// "try again later" (it refuses this server for a while, often an hour)
// is its door refusing, not the video.
func TrailerReasonOf(err error) youtube.Reason {
	r := youtube.ReasonOf(err)
	var e *youtube.Error
	if r.VideoReason() && errors.As(err, &e) && strings.Contains(strings.ToLower(e.Detail), "try again later") {
		return youtube.Refused
	}
	return r
}

// MissedFor turns a reader error with a reason into a *TrailerMissed;
// other errors pass as they are.
func MissedFor(err error) error {
	if err == nil || youtube.ReasonOf(err) == "" {
		return err
	}
	return TrailerEntry{Reason: TrailerReasonOf(err)}.MissErr()
}

// Read looks one video up in the shared pool and counts the outcome for
// the health check under upkeep (the title's "kind|id", or the video key
// for a music video). withPeak also reads the "most replayed" point (the
// watch page, about a second); a trailer needs only the addresses.
func (t *Trailers) Read(ctx context.Context, key, upkeep string, now time.Time, withPeak bool) (TrailerStream, error) {
	t.loadUpkeep(ctx)
	pool := t.lookupPool()
	select {
	case pool <- struct{}{}:
	case <-ctx.Done():
		return TrailerStream{}, ctx.Err()
	}
	s, err := t.resolve(ctx, key, now, withPeak)
	<-pool
	if t.note(upkeep, err, time.Now(), isRenewal(ctx)) {
		t.saveUpkeep(ctx)
	}
	return s, err
}

type renewalKey struct{}

// Renewing marks a lookup of a title that has a kept address (the pass's
// renewals): it played here before.
func Renewing(ctx context.Context) context.Context {
	return context.WithValue(ctx, renewalKey{}, true)
}

func isRenewal(ctx context.Context) bool {
	v, _ := ctx.Value(renewalKey{}).(bool)
	return v
}

// upkeepTitle is a title's key for the health count, without its
// language ("movie|603" from "movie|603|de"): one title failing in three
// languages is one title, not three.
func upkeepTitle(ckey string) string {
	if p := strings.SplitN(ckey, "|", 3); len(p) == 3 {
		return p[0] + "|" + p[1]
	}
	return ckey
}

// details reads the title's details through the shared cache.
func (t *Trailers) details(ctx context.Context, kind, id string) (Details, error) {
	return t.Explore.Details(ctx, kind, id)
}

// Googlevideo says whether raw is an https address on YouTube's video servers.
func Googlevideo(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.HasSuffix(u.Hostname(), ".googlevideo.com")
}

// urlExpired says whether the address carries an expire value that is
// past or nearly so (an address without one is judged by its row).
func urlExpired(raw string, now time.Time) bool {
	if e := trailerExpiry(raw, time.Unix(0, 0)); e.Unix() > int64(trailerKeep/time.Second) {
		return !now.Before(e.Add(-TrailerEarly))
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

// resolve looks one YouTube key up with the reader (at most
// trailerTimeout) and returns the HLS manifest (with an MP4 beside it) or,
// without one, the MP4. A failure is the reader's *youtube.Error.
func (t *Trailers) resolve(ctx context.Context, key string, now time.Time, withPeak bool) (TrailerStream, error) {
	return ResolveTrailer(ctx, key, now, withPeak)
}

// ResolveTrailer is one reader lookup of key, outside the pool and the
// health count.
func ResolveTrailer(ctx context.Context, key string, now time.Time, withPeak bool) (TrailerStream, error) {
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
	if strings.Contains(v.HLS, "/hls_variant/") && Googlevideo(v.HLS) {
		hls = v.HLS
	}
	var mp4 string
	var w, h, rank int
	audio := false
	for _, f := range v.Formats {
		if !f.AVC || f.Height > 1080 || !Googlevideo(f.URL) {
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
