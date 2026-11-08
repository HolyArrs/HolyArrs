package metadata

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

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

// trailerBadEntry is a video YouTube will not play here, until a time.
type trailerBadEntry struct {
	until  time.Time
	reason youtube.Reason
}

// Key picks the YouTube key: the metadata server's pick first (it skips
// keys it found broken), else our own pick from TMDB's videos.
func (t *Trailers) Key(ctx context.Context, kind, id, lang string) (string, error) {
	if k := t.KnownKey(ctx, kind, id, lang); k != "" {
		return k, nil
	}
	k, err := t.pickKey(ctx, kind, id, lang)
	if err == nil && k != "" {
		t.KeepKey(ctx, kind, id, lang, k)
	}
	return k, err
}

// KnownKey answers the kept key, "" when none is kept (no lookup).
func (t *Trailers) KnownKey(ctx context.Context, kind, id, lang string) string {
	ck := kind + "|" + id + "|" + lang
	now := time.Now()
	c := &t.keys
	c.mu.Lock()
	e, ok := c.m[ck]
	c.mu.Unlock()
	if ok && ((e.key != "" && now.Sub(e.at) < trailerKeyKeep) || (e.key == "" && now.Sub(e.at) < trailerKeyMiss)) {
		return e.key
	}
	e = trailerKeyEntry{at: now}
	if t.Store != nil {
		if b, found, err := (LookupCache{Store: t.Store}).Get(ctx, "trailerkey|"+ck, now); err == nil && found {
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

// KeepKey keeps a title's YouTube key for trailerKeyKeep (memory and lookup_cache).
func (t *Trailers) KeepKey(ctx context.Context, kind, id, lang, key string) {
	ck := kind + "|" + id + "|" + lang
	now := time.Now()
	t.keys.put(ck, trailerKeyEntry{key: key, at: now})
	if t.Store != nil {
		_ = LookupCache{Store: t.Store}.Put(ctx, "trailerkey|"+ck, []byte(key), now, now.Add(trailerKeyKeep))
	}
}

func (t *Trailers) pickKey(ctx context.Context, kind, id, lang string) (string, error) {
	if pick := t.Pick(); pick != nil {
		k, err := pick(ctx, kind, id, lang)
		if err == nil && k != "" {
			return k, nil
		}
		if errors.Is(err, ErrNotFound) {
			return "", ErrNoTrailer
		}
	}
	if _, ok := DiscovererFor(ctx, t.Provider()); !ok {
		return "", ErrNoStream
	}
	det, err := t.details(ctx, kind, id)
	if errors.Is(err, ErrNotFound) {
		// TMDB has no such title: kept as "no trailer", so a background
		// pass does not stop on it every time.
		return "", ErrNoTrailer
	}
	if err != nil {
		return "", err
	}
	v, ok := PickTrailer(det.Videos, lang)
	if !ok {
		return "", ErrNoTrailer
	}
	return v.Key, nil
}

// moreKeys are the title's other trailers from TMDB's videos, in
// PickTrailer's order, without first or ones marked bad: up to trailerMore
// to try and one more for YouTube's player.
func (t *Trailers) moreKeys(ctx context.Context, kind, id, lang, first string, now time.Time) []string {
	if _, ok := DiscovererFor(ctx, t.Provider()); !ok {
		return nil
	}
	det, err := t.details(ctx, kind, id)
	if err != nil {
		return nil
	}
	var left []DetailVideo
	for _, v := range det.Videos {
		if _, bad := t.Bad(ctx, v.Key, now); v.Key != first && !bad {
			left = append(left, v)
		}
	}
	var out []string
	for len(out) <= trailerMore {
		v, ok := PickTrailer(left, lang)
		if !ok {
			break
		}
		out = append(out, v.Key)
		left = slices.DeleteFunc(left, func(x DetailVideo) bool { return x.Key == v.Key })
	}
	return out
}

// Bad says whether YouTube would not play the video here lately, and why
// (memory, then lookup_cache).
func (t *Trailers) Bad(ctx context.Context, key string, now time.Time) (youtube.Reason, bool) {
	t.mu.Lock()
	e, ok := t.bad[key]
	t.mu.Unlock()
	if ok && now.Before(e.until) {
		return e.reason, true
	}
	if t.Store == nil || t.Store.Read() == nil {
		return "", false
	}
	b, found, err := LookupCache{Store: t.Store}.Get(ctx, t.badKey(key), now)
	if err != nil || !found {
		return "", false
	}
	return youtube.Reason(b), true
}

// badKey is where a bad mark is kept, per HolyArrs version: an update
// starts with none, so trailers a broken reader marked play again once the
// update fixed it (old marks just expire).
func (t *Trailers) badKey(key string) string {
	return "trailerbad|" + t.Version + "|" + key
}

// MarkBad keeps that YouTube will not play the video here (the reason is
// the row's body) for trailerBadKeep.
func (t *Trailers) MarkBad(ctx context.Context, key string, reason youtube.Reason, now time.Time) {
	t.mu.Lock()
	if t.bad == nil || len(t.bad) > 5000 {
		t.bad = map[string]trailerBadEntry{}
	}
	t.bad[key] = trailerBadEntry{until: now.Add(trailerBadKeep), reason: reason}
	t.mu.Unlock()
	if t.Store != nil {
		_ = LookupCache{Store: t.Store}.Put(ctx, t.badKey(key), []byte(reason), now, now.Add(trailerBadKeep))
	}
}

// VideoMissReason says why the video reader could not play a video, or ""
// when err is not one of its known reasons.
func VideoMissReason(err error) youtube.Reason { return youtube.ReasonOf(err) }

// VideoReader is the built-in video reader, for tests that point it at a
// fake server.
type VideoReader = youtube.Reader

// Reasons the video reader gives for a video it cannot play.
const (
	VideoAgeRestricted = youtube.AgeRestricted
	VideoBotCheck      = youtube.BotCheck
	VideoNetwork       = youtube.Network
	VideoUnavailable   = youtube.Unavailable
)
