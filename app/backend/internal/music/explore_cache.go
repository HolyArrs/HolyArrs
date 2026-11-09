package music

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// Explore → Music answers are built once and kept a minute per member, so
// switching tabs or coming back reads one cached answer. Only complete
// answers are kept (every genre and similar list known); an ask or a title
// added forgets them all. At start the genres of the Library's artists and
// the charts are read ahead, so the genre chips are there on the first visit.

const (
	musicAnswerMax = 200
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

// forgetMusicAnswers drops every kept answer (an ask, or a title added).
func (h *Handlers) forgetMusicAnswers() {
	c := &h.musicAns
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// musicOnEvent forgets the kept answers when an ask or a title changes what they show.
func (h *Handlers) musicOnEvent(ev domain.Event) {
	if !h.PartIsOn(parts.Music) {
		return
	}
	switch ev.Kind {
	case domain.EventAskMade, domain.EventAskJoined, domain.EventTitleAdded:
		h.forgetMusicAnswers()
	}
	if ev.Kind == domain.EventTitleAdded || ev.Kind == domain.EventTitleRemoved {
		h.musicv2.cand.Forget("lib:")
		// A music title changes the household's rows: drop only the day
		// orders (chart candidates stay), once per burst of adds, so the
		// next visit rebuilds them instead of tomorrow.
		if ev.Kind == domain.EventTitleRemoved || ev.Data["album_types"] != nil {
			h.clearMusicDaysSoon()
		}
	}
}

// musicDayDebounce gathers a burst of adds (an import, a batch) into one
// clear of the day orders.
var musicDayDebounce = 3 * time.Second

func (h *Handlers) clearMusicDaysSoon() {
	if !h.musicv2.clearing.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(musicDayDebounce, func() {
		h.musicv2.clearing.Store(false)
		h.musicv2.days.Clear()
		h.forgetMusicV7Snaps(-1)
	})
}

// MusicLoops are Music's background loops. Main adds them to the parts
// Control, so they run only while Music is on and stop when it is turned off.
func (h *Handlers) MusicLoops() []jobs.Loop {
	return []jobs.Loop{{Name: "music_warm", Label: "Music read-ahead", Run: func(ctx context.Context) error {
		// Explore → Music v7 first: its layer 1 is read back from disk (or
		// fetched) within seconds of start, before the slower read-ahead.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(musicV7WarmDelay):
		}
		h.beginAhead(ctx)
		defer h.endAhead()
		h.prewarmMusicV7(ctx)
		h.warmMusic(ctx)
		for ctx.Err() == nil {
			h.prebuildMusic(ctx)
			h.prewarmMusicV7(ctx)
			// Jittered (±90 min on 3 h) and stretched by the server's switch.
			select {
			case <-ctx.Done():
			case <-time.After(musicPrebuildGap(h.musicSwitch(ctx, time.Now()))):
			}
		}
		return nil
	}}}
}

// warmMusic reads ahead the genres of the Library's artists and of the
// charts, once, shortly after Music starts.
func (h *Handlers) warmMusic(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(musicWarmDelay):
	}
	if h.Store == nil {
		return
	}
	if _, ok := h.genreProvider(); !ok {
		return
	}
	titles, err := database.Select[mediafiles.Title](ctx, h.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
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
	if h.MusicTrending != nil {
		for _, kind := range []string{"artist", "album"} {
			l, err := h.cachedMusicTrend(ctx, kind, "global")
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
	h.genresOf(ids)
	// The charts' pictures are fetched ahead too, so the first Music visit
	// reads kept files instead of a picture search per card.
	for _, k := range art {
		if ctx.Err() != nil {
			return
		}
		_, _, _ = h.musicTrendArtFile(ctx, k[0], k[1], "500")
		select {
		case <-ctx.Done():
			return
		case <-time.After(musicArtWarmGap):
		}
	}
}

// musicArtWarmGap spaces the picture read-ahead.
const musicArtWarmGap = 250 * time.Millisecond
