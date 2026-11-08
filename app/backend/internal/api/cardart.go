package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/explore"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Wide title cards show their title on the art (docs/design-review/apple/
// spec.md, card system). The picture is chosen on the server once per
// title and language, kept 7 days in card_art, and handed out with the
// Explore and Home answers. Page loads only read the table; the image
// lists are fetched by the Explore pool job, one paced call per title.

const (
	cardArtTTL = 7 * 24 * time.Hour
	// cardArtGap spaces the image list calls of the pool job.
	cardArtGap = 400 * time.Millisecond
	// cardArtLangTTL is how long a language asked by a page keeps being filled.
	cardArtLangTTL = 30 * 24 * time.Hour
)

type cardWant struct {
	kind, id, lang, fallback string
}

type cardArtState struct {
	mu      sync.Mutex
	langs   map[string]time.Time
	want    map[string]cardWant
	order   []string
	running bool
	// gap overrides cardArtGap (tests).
	gap time.Duration
	// fetched counts image list calls (tests).
	fetched int
}

func cardKey(kind, id, lang string) string { return kind + ":" + id + ":" + lang }

// cardArtRow is one stored pick.
type cardArtRow struct {
	metadata.CardArt
	at time.Time
}

// readCardArt returns the stored picks for the titles in lang and English.
func (a *API) readCardArt(ctx context.Context, keys [][2]string, lang string) map[string]cardArtRow {
	out := map[string]cardArtRow{}
	if a.s.Store == nil || len(keys) == 0 {
		return out
	}
	for start := 0; start < len(keys); start += 200 {
		end := min(start+200, len(keys))
		var b strings.Builder
		args := []any{lang, "en"}
		b.WriteString("SELECT kind, tmdb_id, lang, art, logo, has_title, fetched_at FROM card_art WHERE lang IN (?, ?) AND (kind, tmdb_id) IN (")
		for i, k := range keys[start:end] {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("(?, ?)")
			args = append(args, k[0], k[1])
		}
		b.WriteString(")")
		rows, err := a.s.Store.Read().QueryContext(ctx, b.String(), args...)
		if err != nil {
			return out
		}
		for rows.Next() {
			var kind, id, l, at string
			var r cardArtRow
			if rows.Scan(&kind, &id, &l, &r.Art, &r.Logo, &r.HasTitle, &at) != nil {
				continue
			}
			r.at, _ = time.Parse(database.TimeLayout, at)
			out[cardKey(kind, id, l)] = r
		}
		_ = rows.Close()
	}
	return out
}

// fillCardArt sets card_art, card_logo and art_has_title from the stored
// picks (the asked language first, else English). It never goes outside;
// titles without a pick are left to the pool job, and the page falls back
// to the backdrop and the name.
func (a *API) fillCardArt(r *http.Request, items []discoverItemView) {
	lang := metadata.TrailerLanguage(r)
	a.noteCardLang(lang)
	var keys [][2]string
	for _, v := range items {
		if (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" {
			keys = append(keys, [2]string{v.Kind, v.TMDBID})
		}
	}
	got := a.readCardArt(r.Context(), keys, lang)
	for i := range items {
		v := &items[i]
		row, ok := got[cardKey(v.Kind, v.TMDBID, lang)]
		if !ok {
			row, ok = got[cardKey(v.Kind, v.TMDBID, "en")]
		}
		if ok {
			v.CardArt = row.CardArt
			if v.CardArt.HasTitle {
				// Never hand out a backdrop with the title printed on it: the card
				// keeps the title's own backdrop and the logo floats over it.
				v.CardArt.Art, v.CardArt.HasTitle = "", false
			}
		}
	}
}

func (a *API) fillRowsCardArt(r *http.Request, rows []exploreRowView) {
	var all []discoverItemView
	for _, row := range rows {
		all = append(all, row.Items...)
	}
	a.fillCardArt(r, all)
	n := 0
	for i := range rows {
		copy(rows[i].Items, all[n:n+len(rows[i].Items)])
		n += len(rows[i].Items)
	}
}

func (a *API) noteCardLang(lang string) {
	c := &a.explore.cards
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.langs == nil {
		c.langs = map[string]time.Time{}
	}
	c.langs[lang] = time.Now()
}

// cardLangs are English and every language a page asked for lately.
func (a *API) cardLangs() []string {
	c := &a.explore.cards
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{"en"}
	for l, at := range c.langs {
		if l != "en" && time.Since(at) < cardArtLangTTL {
			out = append(out, l)
		}
	}
	return out
}

// libraryCardArt is Home's card pictures for Library titles, by
// "kind:tmdb id". Titles without a pick are queued for the paced filler
// (no outside call on the request).
func (a *API) libraryCardArt(r *http.Request, titles []mediafiles.Title) map[string]metadata.CardArt {
	lang := metadata.TrailerLanguage(r)
	a.noteCardLang(lang)
	var keys [][2]string
	var items []explore.Item
	for _, t := range titles {
		if id := t.SourceIDs["tmdb"]; id != "" && (t.Kind == "movie" || t.Kind == "show") {
			keys = append(keys, [2]string{t.Kind, id})
			items = append(items, explore.Item{Kind: t.Kind, TMDBID: id, Backdrop: t.Backdrop})
		}
	}
	got := a.readCardArt(r.Context(), keys, lang)
	out := map[string]metadata.CardArt{}
	for _, k := range keys {
		row, ok := got[cardKey(k[0], k[1], lang)]
		if !ok {
			row, ok = got[cardKey(k[0], k[1], "en")]
		}
		if ok {
			out[k[0]+":"+k[1]] = row.CardArt
		}
	}
	if len(out) < len(keys) {
		a.queueCardItems(context.WithoutCancel(r.Context()), items)
	}
	return out
}

// queueCardArt queues the pool's movies and shows that have no fresh pick
// and starts the paced filler (the pool job's background work).
func (a *API) queueCardArt(ctx context.Context, pool *explore.Pool) {
	if pool == nil {
		return
	}
	var items []explore.Item
	for _, row := range pool.Rows {
		items = append(items, row.Items...)
	}
	a.queueCardItems(ctx, items)
}

func (a *API) queueCardItems(ctx context.Context, all []explore.Item) {
	if a.s.Store == nil {
		return
	}
	seen := map[string]bool{}
	var items []explore.Item
	for _, it := range all {
		if (it.Kind == "movie" || it.Kind == "show") && it.TMDBID != "" && !seen[it.Key()] {
			seen[it.Key()] = true
			items = append(items, it)
		}
	}
	keys := make([][2]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, [2]string{it.Kind, it.TMDBID})
	}
	langs := a.cardLangs()
	c := &a.explore.cards
	for _, lang := range langs {
		got := a.readCardArt(ctx, keys, lang)
		c.mu.Lock()
		if c.want == nil {
			c.want = map[string]cardWant{}
		}
		for _, it := range items {
			k := cardKey(it.Kind, it.TMDBID, lang)
			// A titled backdrop stored with a logo is an old pick (the name showed
			// twice); pick it again as a textless backdrop plus the logo.
			if row, ok := got[k]; ok && time.Since(row.at) < cardArtTTL && !(row.HasTitle && row.Logo != "") {
				continue
			}
			if _, ok := c.want[k]; !ok {
				c.want[k] = cardWant{kind: it.Kind, id: it.TMDBID, lang: lang, fallback: it.Backdrop}
				c.order = append(c.order, k)
			}
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	start := !c.running && len(c.order) > 0
	if start {
		c.running = true
	}
	c.mu.Unlock()
	if start {
		go jobs.Guard("explore-card-art", func() { a.drainCardArt(context.WithoutCancel(ctx)) })
	}
}

// drainCardArt fetches one image list per queued title, paced, and stores
// the pick. A busy or missing source stops the run; the next pool build
// queues what is left.
func (a *API) drainCardArt(ctx context.Context) {
	c := &a.explore.cards
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	gap := c.gap
	if gap <= 0 {
		gap = cardArtGap
	}
	for {
		c.mu.Lock()
		if len(c.order) == 0 {
			c.mu.Unlock()
			return
		}
		k := c.order[0]
		c.order = c.order[1:]
		w, ok := c.want[k]
		delete(c.want, k)
		c.mu.Unlock()
		if !ok {
			continue
		}
		src, ok := metadata.CardArterFor(ctx, a.s.Metadata)
		if !ok {
			a.dropCardWants()
			return
		}
		lctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		im, err := src.CardImages(lctx, w.kind, w.id, w.lang)
		cancel()
		c.mu.Lock()
		c.fetched++
		c.mu.Unlock()
		switch {
		case errors.Is(err, metadata.ErrNotFound):
			im = metadata.CardImages{}
		case err != nil:
			if errors.Is(err, metadata.ErrRateLimited) || errors.Is(err, metadata.ErrNoAccess) {
				a.dropCardWants()
				return
			}
			log.Printf("explore: card art %s: %v", k, err)
			continue
		}
		pick := metadata.PickCardArt(im, w.lang, w.fallback)
		_ = a.storeCardArt(ctx, w, pick)
		time.Sleep(gap)
	}
}

func (a *API) dropCardWants() {
	c := &a.explore.cards
	c.mu.Lock()
	c.want, c.order = nil, nil
	c.mu.Unlock()
}

func (a *API) storeCardArt(ctx context.Context, w cardWant, p metadata.CardArt) error {
	return a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO card_art (kind, tmdb_id, lang, art, logo, has_title, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (kind, tmdb_id, lang) DO UPDATE SET art = excluded.art, logo = excluded.logo, has_title = excluded.has_title, fetched_at = excluded.fetched_at`,
			w.kind, w.id, w.lang, p.Art, p.Logo, p.HasTitle, database.FormatTime(time.Now()))
		return err
	})
}
