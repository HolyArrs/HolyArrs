package discover

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The hero needs a title's card pick (logo and backdrop). Page answers
// normally only read stored picks and leave the rest to the paced filler,
// which on a fresh install takes minutes to reach the hero's titles, so the
// first visit had no hero until a reload. The hero's titles are fetched now.

const (
	// HeroArtMax is how many hero titles without a pick one answer fetches.
	HeroArtMax = 12
	// HeroArtWait is how long an answer waits for them; later ones are still
	// stored for the next answer.
	HeroArtWait = 4 * time.Second
)

// FillNow is Fill, after fetching side by side (at most max titles, waiting
// at most wait) the picks of items with none stored in the page's language
// or English.
func (c *CardArt) FillNow(r *http.Request, items []ItemView, max int, wait time.Duration) {
	if c.Store() != nil && c.Metadata != nil {
		lang := metadata.TextLanguage(r)
		var keys [][2]string
		for _, v := range items {
			if (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" {
				keys = append(keys, [2]string{v.Kind, v.TMDBID})
			}
		}
		got := c.Read(r.Context(), keys, lang)
		var todo []cardWant
		seen := map[string]bool{}
		for _, v := range items {
			k := CardKey(v.Kind, v.TMDBID, lang)
			if (v.Kind != "movie" && v.Kind != "show") || v.TMDBID == "" || seen[k] || len(todo) >= max {
				continue
			}
			if _, ok := got[k]; ok {
				continue
			}
			if _, ok := got[CardKey(v.Kind, v.TMDBID, "en")]; ok {
				continue
			}
			seen[k] = true
			todo = append(todo, cardWant{kind: v.Kind, id: v.TMDBID, lang: lang, fallback: v.Backdrop})
		}
		if len(todo) > 0 {
			c.fetchNow(context.WithoutCancel(r.Context()), todo, wait)
		}
	}
	c.Fill(r, items)
}

func (c *CardArt) fetchNow(ctx context.Context, todo []cardWant, wait time.Duration) {
	src, ok := metadata.CardArterFor(ctx, c.Metadata())
	if !ok {
		return
	}
	var wg sync.WaitGroup
	for _, w := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			im, err := src.CardImages(lctx, w.kind, w.id, w.lang)
			cancel()
			c.mu.Lock()
			c.fetched++
			c.mu.Unlock()
			if err != nil && !errors.Is(err, metadata.ErrNotFound) {
				return
			}
			_ = c.store(ctx, w, metadata.PickCardArt(im, w.lang, w.fallback))
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(wait):
	}
}

// HeroArt fills hero candidates' trailer keys and card picks, fetching the
// missing picks now (FillCardArtNow) when wired.
func (h *Handlers) HeroArt(r *http.Request, vs []ItemView) {
	h.FillTrailerKeys(r, vs)
	if h.FillCardArtNow != nil {
		h.FillCardArtNow(r, vs)
	} else {
		h.FillCardArt(r, vs)
	}
	// The hero's trailers are looked up now, not on the read-ahead's turn;
	// the hero shows its picture straight away and the trailer joins later.
	if h.TrailersNow != nil && h.KidMax(r) == "" {
		var titles [][2]string
		for _, v := range vs {
			if v.TrailerKey == "" && HeroArtReady(v) && len(titles) < 6 {
				titles = append(titles, [2]string{v.Kind, v.TMDBID})
			}
		}
		if len(titles) > 0 {
			h.TrailersNow(metadata.TrailerLanguage(r), titles)
		}
	}
}

// HeroReady are the candidates the hero may show, in order: art ready with
// a known trailer first, then (when fewer than half of max have one) art
// ready without one yet; with none, titles with a backdrop. At most max.
func HeroReady(vs []ItemView, max int) []ItemView {
	var ready, artOnly []ItemView
	for _, v := range vs {
		switch {
		case !HeroArtReady(v):
		case v.TrailerKey != "":
			ready = append(ready, v)
		default:
			artOnly = append(artOnly, v)
		}
	}
	if len(ready) < max/2 {
		ready = append(ready, artOnly...)
	}
	if len(ready) == 0 {
		// No logo in yet (a slow server on a fresh install): titles with a
		// backdrop still fill the hero, their name standing in for the logo.
		for _, v := range vs {
			if v.Backdrop != "" {
				ready = append(ready, v)
			}
		}
	}
	return ready[:min(len(ready), max)]
}
