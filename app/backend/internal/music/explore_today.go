package music

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// risingCache keeps the day's rising artists row for an hour, as the
// metadata server does; a miss is kept a few minutes so a server that is
// down is not asked on every page view.
type risingCache struct {
	mu  sync.Mutex
	row *MusicRow
	at  time.Time
}

const (
	risingKeep = time.Hour
	risingMiss = 5 * time.Minute
)

// todayRow is the "Rising today" row (round artist cards, the day's gain
// on each), or nil when the server has none or did not answer.
func (h *Handlers) todayRow(ctx context.Context) *MusicRow {
	if h.MusicRising == nil {
		return nil
	}
	c := &h.rising
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if !c.at.IsZero() && (now.Sub(c.at) < risingMiss || (c.row != nil && now.Sub(c.at) < risingKeep)) {
		return c.row
	}
	c.at = now
	c.row = nil
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	l, err := h.MusicRising(ctx)
	if err != nil || len(l.Items) == 0 {
		return nil
	}
	row := MusicRow{ID: "rising_today", Type: "rising_today", Title: "Rising today", Reason: "Gaining the most listeners today", Shape: "round", Items: []MusicCard{}}
	var names []nameKey
	for i, it := range l.Items {
		if i == 20 {
			break
		}
		card := MusicCard{Key: "rising_today:" + it.MBID, Kind: "artist", MBID: it.MBID, Name: it.Name, Artist: it.Name, ArtistMBID: it.MBID,
			Image: artPath("artist", it.MBID), Rank: i + 1, Genres: []string{}}
		if it.Gain > 0 {
			card.Note = "+" + shortCount(it.Gain) + " listeners today"
		}
		row.Items = append(row.Items, card)
		names = append(names, nameKey{kind: "artist", mbid: it.MBID, name: it.Name})
	}
	h.rememberMusicNames(names)
	c.row = &row
	return c.row
}

// risingToday answers GET /api/v1/explore/music/rising: the row, or 404
// when there is none today.
func (h *Handlers) risingToday(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) || h.KidMax(r) != "" {
		webserver.Fail(w, http.StatusNotFound, "no rising artists")
		return
	}
	row := h.todayRow(context.WithoutCancel(r.Context()))
	if row == nil {
		webserver.Fail(w, http.StatusNotFound, "no rising artists")
		return
	}
	h.ExploreCaching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, row)
}

// shortCount writes 1234 as 1.2K and 2500000 as 2.5M.
func shortCount(n int64) string {
	f := func(v float64, unit string) string {
		s := strconv.FormatFloat(v, 'f', 1, 64)
		if len(s) > 2 && s[len(s)-2:] == ".0" || v >= 100 {
			s = strconv.FormatFloat(v, 'f', 0, 64)
		}
		return s + unit
	}
	switch {
	case n >= 1_000_000_000:
		return f(float64(n)/1e9, "B")
	case n >= 1_000_000:
		return f(float64(n)/1e6, "M")
	case n >= 1_000:
		return f(float64(n)/1e3, "K")
	}
	return strconv.FormatInt(n, 10)
}
