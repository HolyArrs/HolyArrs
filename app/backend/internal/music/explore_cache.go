package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// Explore → Music answers are built once and kept a minute per member, so
// switching tabs or coming back reads one cached answer. Only complete
// answers are kept (every genre and similar list known); an ask or a title
// added forgets them all. At start the genres of the Library's artists and
// the charts are read ahead, so the genre chips are there on the first visit.

const (
	musicAnswerTTL = time.Minute
	musicAnswerMax = 200
	// musicGenreWait is how long a request waits for genres being read:
	// none. The page answers with the genres known now; the rest are read
	// in the background and the kept (older) answer stands in meanwhile.
	musicGenreWait = 0
	// musicAnswerStale is how long a kept answer may stand in for one that
	// is still missing genres or similar lists.
	musicAnswerStale = 30 * time.Minute
	// musicWarmDelay lets the program finish starting before the read-ahead.
	musicWarmDelay = 5 * time.Second
	// musicV7WarmDelay is the wait before the v7 prewarm at start.
	musicV7WarmDelay = time.Second
)

type musicAnswer struct {
	at   time.Time
	body []byte
}

type musicAnswers struct {
	mu sync.Mutex
	m  map[string]musicAnswer
}

func musicAnswerKey(user int64, genre, region string, hour int) string {
	return strconv.FormatInt(user, 10) + "|" + genre + "|" + region + "|" + strconv.Itoa(hour)
}

// cachedMusicAnswer answers a kept answer younger than musicAnswerTTL.
func (a *API) cachedMusicAnswer(key string, now time.Time) ([]byte, bool) {
	c := &a.musicAns
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || now.Sub(e.at) >= musicAnswerTTL || now.Before(e.at) {
		return nil, false
	}
	return e.body, true
}

// staleMusicAnswer answers a kept answer younger than musicAnswerStale.
func (a *API) staleMusicAnswer(key string, now time.Time) ([]byte, bool) {
	c := &a.musicAns
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || now.Sub(e.at) >= musicAnswerStale || now.Before(e.at) {
		return nil, false
	}
	return e.body, true
}

func (a *API) keepMusicAnswer(key string, body []byte, now time.Time) {
	c := &a.musicAns
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= musicAnswerMax {
		c.m = map[string]musicAnswer{}
	}
	c.m[key] = musicAnswer{at: now, body: body}
}

// forgetMusicAnswers drops every kept answer (an ask, or a title added).
func (a *API) forgetMusicAnswers() {
	c := &a.musicAns
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// musicOnEvent forgets the kept answers when an ask or a title changes what they show.
func (a *API) musicOnEvent(ev domain.Event) {
	if !a.partIsOn(parts.Music) {
		return
	}
	switch ev.Kind {
	case domain.EventAskMade, domain.EventAskJoined, domain.EventTitleAdded:
		a.forgetMusicAnswers()
	}
	if ev.Kind == domain.EventTitleAdded || ev.Kind == domain.EventTitleRemoved {
		a.musicv2.cand.Forget("lib:")
		// A music title changes the household's rows: drop only the day
		// orders (chart candidates stay), once per burst of adds, so the
		// next visit rebuilds them instead of tomorrow.
		if ev.Kind == domain.EventTitleRemoved || ev.Data["album_types"] != nil {
			a.clearMusicDaysSoon()
		}
	}
}

// musicDayDebounce gathers a burst of adds (an import, a batch) into one
// clear of the day orders.
var musicDayDebounce = 3 * time.Second

func (a *API) clearMusicDaysSoon() {
	if !a.musicv2.clearing.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(musicDayDebounce, func() {
		a.musicv2.clearing.Store(false)
		a.musicv2.days.Clear()
		a.forgetMusicV7Snaps(-1)
	})
}

// writeMusicAnswer writes the answer, and keeps it when it is complete.
func (a *API) writeMusicAnswer(w http.ResponseWriter, key string, v any, complete bool, now time.Time) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// The answer carries its pictures' records (box, colour, version).
	b := a.withPictures(context.Background(), buf.Bytes())
	if complete {
		a.keepMusicAnswer(key, b, now)
	} else if old, ok := a.staleMusicAnswer(key, now); ok {
		// Still reading genres or similar lists: the last full answer stands in.
		b = old
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// genresWaiting answers the known genres of ids, waiting up to wait for the
// ones being read (the missing ones are queued); complete says every id
// has its genres.
func (a *API) genresWaiting(ids []string, wait time.Duration) (map[string][]string, bool) {
	want := 0
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			want++
		}
	}
	got := a.genresOf(ids)
	end := time.Now().Add(wait)
	for len(got) < want && time.Now().Before(end) && a.genresFilling() {
		time.Sleep(25 * time.Millisecond)
		got = a.genresOf(ids)
	}
	return got, len(got) >= want
}

func (a *API) genresFilling() bool {
	c := &a.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// MusicLoops are Music's background loops. Main adds them to the parts
// Control, so they run only while Music is on and stop when it is turned off.
func (a *API) MusicLoops() []jobs.Loop {
	return []jobs.Loop{{Name: "music_warm", Label: "Music read-ahead", Run: func(ctx context.Context) error {
		// Explore → Music v7 first: its layer 1 is read back from disk (or
		// fetched) within seconds of start, before the slower read-ahead.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(musicV7WarmDelay):
		}
		a.beginAhead(ctx)
		defer a.endAhead()
		a.prewarmMusicV7(ctx)
		a.warmMusic(ctx)
		for ctx.Err() == nil {
			a.prebuildMusic(ctx)
			a.prewarmMusicV7(ctx)
			// Jittered (±90 min on 3 h) and stretched by the server's switch.
			select {
			case <-ctx.Done():
			case <-time.After(musicPrebuildGap(a.musicSwitch(ctx, time.Now()))):
			}
		}
		return nil
	}}}
}

// warmMusic reads ahead the genres of the Library's artists and of the
// charts, once, shortly after Music starts.
func (a *API) warmMusic(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(musicWarmDelay):
	}
	if a.s.Store == nil {
		return
	}
	if _, ok := a.genreProvider(); !ok {
		return
	}
	titles, err := database.Select[mediafiles.Title](ctx, a.s.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		return
	}
	var ids []string
	for _, t := range titles {
		if id := strings.ToLower(t.SourceIDs["musicbrainz"]); id != "" {
			ids = append(ids, id)
		}
	}
	var art [][2]string
	if a.s.MusicTrending != nil {
		for _, kind := range []string{"artist", "album"} {
			l, err := a.cachedMusicTrend(ctx, kind, "global")
			if err != nil {
				continue
			}
			for _, it := range l.Items {
				art = append(art, [2]string{kind, strings.ToLower(it.MBID)})
				id := it.ArtistMBID
				if kind == "artist" {
					id = it.MBID
				}
				if id = strings.ToLower(id); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	a.genresOf(ids)
	// The charts' pictures are fetched ahead too, so the first Music visit
	// reads kept files instead of a picture search per card.
	for _, k := range art {
		if ctx.Err() != nil {
			return
		}
		_, _, _ = a.musicTrendArtFile(ctx, k[0], k[1], "500")
		select {
		case <-ctx.Done():
			return
		case <-time.After(musicArtWarmGap):
		}
	}
}

// musicArtWarmGap spaces the picture read-ahead.
const musicArtWarmGap = 250 * time.Millisecond
