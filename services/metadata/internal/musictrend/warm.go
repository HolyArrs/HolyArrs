package musictrend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Warmer keeps the most popular artists' MusicBrainz answers (the artist
// and its release groups) in the cache, so their pages open from the cache
// on a first visit. It works through the list slowly, leaving most of the
// MusicBrainz budget for real requests. Answers the replication feed marks
// changed are stale, so the next pass fetches them again.
type Warmer struct {
	// ListenBrainzURL is the sitewide stats root (ListenBrainzURL).
	ListenBrainzURL string
	UserAgent       string
	HTTP            *http.Client
	// Charts gives the artists on the current chart lists (optional).
	Charts func() []string
	// Warm fetches one artist unless a fresh answer is kept; fetched says
	// whether it called MusicBrainz.
	Warm func(ctx context.Context, mbid string) (fetched bool, err error)
	// Max is how many artists are kept warm (default 2000).
	Max int
	// Pace is the wait after each fetch (default 4 s, about a quarter of
	// the one-request-a-second MusicBrainz rate).
	Pace time.Duration
	// Every is the time between passes (default 24 h).
	Every time.Duration
	// Start is the wait before the first pass (default 2 min).
	Start time.Duration
	// Log reports each pass (default log.Printf).
	Log func(format string, args ...any)
}

func (w *Warmer) defaults() {
	if w.Max <= 0 {
		w.Max = 2000
	}
	if w.Pace <= 0 {
		w.Pace = 4 * time.Second
	}
	if w.Every <= 0 {
		w.Every = 24 * time.Hour
	}
	if w.Start < 0 {
		w.Start = 0
	} else if w.Start == 0 {
		w.Start = 2 * time.Minute
	}
	if w.HTTP == nil {
		w.HTTP = &http.Client{Timeout: time.Minute}
	}
	if w.Log == nil {
		w.Log = log.Printf
	}
}

// Seeds lists the artists to keep warm: the chart artists first (up to
// half of Max), then
// ListenBrainz's most listened artists this month, this year and of all
// time, without repeats, up to Max.
func (w *Warmer) Seeds(ctx context.Context) []string {
	w.defaults()
	var out []string
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.ToLower(id)
		if len(out) < w.Max && validMBID(id) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if w.Charts != nil {
		// The charts of every country could fill the list alone: they get half.
		for _, id := range w.Charts() {
			if len(out) >= w.Max/2 {
				break
			}
			add(id)
		}
	}
	for _, r := range []string{"month", "year", "all_time"} {
		if len(out) >= w.Max || ctx.Err() != nil {
			break
		}
		list, err := w.top(ctx, r)
		if err != nil {
			w.Log("musicwarm: listenbrainz %s: %v", r, err)
			continue
		}
		for _, e := range list {
			add(e.MBID)
		}
	}
	return out
}

func (w *Warmer) top(ctx context.Context, rng string) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.ListenBrainzURL+"/artists?range="+rng+"&count=1000", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", w.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := w.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("answered %s", resp.Status)
	}
	list, _, err := ParseLBArtists(io.LimitReader(resp.Body, 16<<20))
	return list, err
}

// Pass warms every seed once and reports how many it fetched and how many
// failed. A failure waits a little longer before the next artist.
func (w *Warmer) Pass(ctx context.Context) (seeds, fetched, failed int) {
	w.defaults()
	ids := w.Seeds(ctx)
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		got, err := w.Warm(ctx, id)
		if got {
			fetched++
		}
		wait := time.Duration(0)
		if err != nil {
			failed++
			wait = 4 * w.Pace
		} else if got {
			wait = w.Pace
		}
		if wait > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
		}
	}
	return len(ids), fetched, failed
}

// Run waits Start, then runs a pass every Every until ctx ends.
func (w *Warmer) Run(ctx context.Context) {
	w.defaults()
	wait := w.Start
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		began := time.Now()
		n, got, bad := w.Pass(ctx)
		if ctx.Err() != nil {
			return
		}
		w.Log("musicwarm: %d artists, %d fetched, %d failed in %s", n, got, bad, time.Since(began).Round(time.Second))
		wait = w.Every
	}
}

// ChartArtists lists the artist ids on every chart list kept now: the
// artist lists, then the artists of the album lists.
func (s *Service) ChartArtists() []string {
	var artists, albums []string
	for k, l := range *s.lists.Load() {
		var body struct {
			Items []struct {
				MBID       string `json:"mbid"`
				ArtistMBID string `json:"artist_mbid"`
			} `json:"items"`
		}
		if json.Unmarshal(l.body, &body) != nil {
			continue
		}
		for _, it := range body.Items {
			switch {
			case k.Kind == "artist" && validMBID(it.MBID):
				artists = append(artists, it.MBID)
			case k.Kind == "album" && validMBID(it.ArtistMBID):
				albums = append(albums, it.ArtistMBID)
			}
		}
	}
	return append(artists, albums...)
}
