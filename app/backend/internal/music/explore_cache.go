package music

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// Explore → Music pages come from the day's plan (explore_page.go).
// At start the genres of the Library's artists and
// the charts are read ahead, so the genre chips are there on the first visit.

const (
	// musicWarmDelay lets the program finish starting before the read-ahead.
	musicWarmDelay = 5 * time.Second
	// musicPlanWarmDelay is the wait before the page-plan prewarm at start.
	musicPlanWarmDelay = time.Second
)

// musicOnEvent drops cached rows when a title changes what they show.
func (h *Handlers) musicOnEvent(ev domain.Event) {
	if !h.PartIsOn(parts.Music) {
		return
	}
	if ev.Kind == domain.EventTitleAdded || ev.Kind == domain.EventTitleRemoved {
		h.exploreCache.cand.Forget("lib:")
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
	if !h.exploreCache.clearing.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(musicDayDebounce, func() {
		h.exploreCache.clearing.Store(false)
		h.exploreCache.days.Clear()
		h.forgetMusicSnaps(-1)
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
		case <-time.After(musicPlanWarmDelay):
		}
		h.beginAhead(ctx)
		defer h.endAhead()
		h.prewarmMusicPlan(ctx)
		h.warmMusic(ctx)
		for ctx.Err() == nil {
			h.prebuildMusic(ctx)
			h.prewarmMusicPlan(ctx)
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
