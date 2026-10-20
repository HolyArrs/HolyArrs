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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Explore → Music answers are built once and kept a minute per member, so
// switching tabs or coming back reads one cached answer. Only complete
// answers are kept (every genre and similar list known); an ask or a title
// added forgets them all. At start the genres of the Library's artists and
// the charts are read ahead, so the genre chips are there on the first visit.

const (
	musicAnswerTTL = time.Minute
	musicAnswerMax = 200
	// musicGenreWait is how long a request waits for genres being read
	// (mostly from the lookup cache, which answers at once).
	musicGenreWait = 1500 * time.Millisecond
	// musicWarmDelay lets the program finish starting before the read-ahead.
	musicWarmDelay = 5 * time.Second
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
	switch ev.Kind {
	case domain.EventAskMade, domain.EventAskJoined, domain.EventTitleAdded:
		a.forgetMusicAnswers()
	}
}

// writeMusicAnswer writes the answer, and keeps it when it is complete.
func (a *API) writeMusicAnswer(w http.ResponseWriter, key string, v any, complete bool, now time.Time) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		internal(w, err)
		return
	}
	if complete {
		a.keepMusicAnswer(key, buf.Bytes(), now)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// genresWaiting answers the known genres of ids, waiting up to wait for the
// ones being read; complete says every id has its genres.
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

// warmMusic reads ahead the genres of the Library's artists and of the
// charts, once, shortly after start.
func (a *API) warmMusic(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(musicWarmDelay):
	}
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		return
	}
	if _, ok := a.genreProvider(); !ok {
		return
	}
	titles, err := store.Select[domain.Title](ctx, a.s.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		return
	}
	var ids []string
	for _, t := range titles {
		if id := strings.ToLower(t.SourceIDs["musicbrainz"]); id != "" {
			ids = append(ids, id)
		}
	}
	if a.s.MusicTrending != nil {
		for _, kind := range []string{"artist", "album"} {
			l, err := a.cachedMusicTrend(ctx, kind, "global")
			if err != nil {
				continue
			}
			for _, it := range l.Items {
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
}
