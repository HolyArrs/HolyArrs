package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// StripBrowse is the Browse glue the strip needs, given by the program.
type StripBrowse struct {
	// Page is the member's Browse page, its genre tiles and the Kid flag.
	Page func(ctx context.Context, r *http.Request, sw string) (engine.Page, []engine.BrowseTile, bool, error)
	// Section is a shared section's results and whether it was built.
	Section func(sw string, kid bool, max, id string) ([]engine.CategoryResult, bool)
	// Build builds a shared section and returns when it is done.
	Build func(ctx context.Context, sw string, kid bool, max, id string)
	// Tiles are a section's tiles; Building is the error while it builds.
	Tiles    func(ctx context.Context, r *http.Request, sw string, kid bool, pg engine.Page, page []engine.BrowseTile, id string) ([]engine.CategoryTile, error)
	Building error
	// Fetch is the server-wide Browse fetch slots.
	Fetch chan struct{}
}

// StripRoutes registers the Explore strip routes.
func (h *Handlers) StripRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/explore/strip", h.exploreStrip)
	m.HandleFunc("POST /api/v1/explore/strip/counts", h.exploreStripCounts)
	m.HandleFunc("DELETE /api/v1/explore/strip/counts", h.exploreStripReset)
}

// The Explore category strip, picked on the server for each member from the
// Browse sections already built (engine.PickStrip). It never waits for a
// section: one not built yet starts building in the background and the
// answer says partial. Answers are cached per member, tab, limit, day and
// the rows below for stripTTL.

// stripTTL is how long a member's strip answer is kept.
const stripTTL = time.Hour

// stripTasteTTL is how long a member's taste is kept.
const stripTasteTTL = 6 * time.Hour

// stripKeywordTTL is how long a title's genres and keywords are kept.
const stripKeywordTTL = 30 * 24 * time.Hour

// stripKeywordFetches is the most titles one background pass looks up.
const stripKeywordFetches = 30

// stripEntry is a member's one kept strip per tab and limit; sig is the
// day and a hash of the rows it was picked for.
type stripEntry struct {
	at    time.Time
	sig   string
	tiles []engine.CategoryTile
}

type stripTasteEntry struct {
	at    time.Time
	taste engine.StripTaste
}

var (
	stripAnswers sync.Map // %p|member|... -> stripEntry
	stripTastes  sync.Map // %p|member -> stripTasteEntry
	stripFetches sync.Map // %p|member -> true while a keyword pass runs
)

func (h *Handlers) stripMemberKey(member int64) string { return fmt.Sprintf("%p|%d|", h, member) }

// forgetStrip drops a member's cached strips and taste.
func (h *Handlers) forgetStrip(member int64) {
	prefix := h.stripMemberKey(member)
	for _, m := range []*sync.Map{&stripAnswers, &stripTastes} {
		m.Range(func(k, _ any) bool {
			if strings.HasPrefix(k.(string), prefix) {
				m.Delete(k)
			}
			return true
		})
	}
}

// keepStrip stores a member's strip (replacing their earlier one for the
// tab and limit) and drops expired entries.
func keepStrip(key string, e stripEntry) {
	stripAnswers.Range(func(k, v any) bool {
		if time.Since(v.(stripEntry).at) >= stripTTL {
			stripAnswers.Delete(k)
		}
		return true
	})
	stripAnswers.Store(key, e)
}

// exploreStrip answers GET /api/v1/explore/strip?type=&row=: the member's
// category strip, in order.
func (h *Handlers) exploreStrip(w http.ResponseWriter, r *http.Request) {
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	ctx := r.Context()
	now := h.Explore.Now()
	moment, _ := ExploreMoment(r, now)
	member := h.UserID(r)
	max := h.KidMax(r)
	rows, ok := StripRows(r)
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "at most 40 row values of up to 80 characters")
		return
	}
	key := h.stripMemberKey(member) + sw + "|" + max
	// A hide changes the generation, so a kept strip is not reused after it.
	sig := moment.Local.Format("2006-01-02") + "|" + StripRowsHash(rows) + "|" + fmt.Sprint(hiddenGen(h.Store))
	if v, ok := stripAnswers.Load(key); ok && v.(stripEntry).sig == sig && time.Since(v.(stripEntry).at) < stripTTL {
		h.Caching(w, r, "private, no-cache")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": v.(stripEntry).tiles, "partial": false})
		return
	}
	pg, page, kid, err := h.Strip.Page(ctx, r, sw)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	cands := make([]engine.StripCand, 0, 128)
	for _, t := range page {
		cands = append(cands, engine.NewStripCand(engine.CategoryTile{BrowseTile: t, Section: "genres"}, PageGenreItems(pg, t.GenreName), now))
	}
	complete := true
	for i, s := range engine.BrowseSections {
		res, built := h.Strip.Section(sw, kid, max, s.ID)
		if !built {
			complete = false
			missing := engine.BrowseSections[i:]
			bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			go jobs.Guard("explore-strip-build", func() {
				defer cancel()
				for _, x := range missing {
					if _, have := h.Strip.Section(sw, kid, max, x.ID); !have {
						h.Strip.Build(bg, sw, kid, max, x.ID)
					}
				}
			})
			break
		}
		tiles, err := h.Strip.Tiles(ctx, r, sw, kid, pg, page, s.ID)
		if errors.Is(err, h.Strip.Building) {
			complete = false
			break
		}
		if err != nil {
			webserver.Internal(w, "discover", err)
			return
		}
		items := map[string][]engine.Item{}
		for _, c := range res {
			items[c.Cat.ID] = c.Items
		}
		for _, t := range tiles {
			cands = append(cands, engine.NewStripCand(t, items[t.Category], now))
		}
	}
	stats, recent, err := StripStats(ctx, h.Store.Read(), member, now)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	tiles := engine.PickStrip(engine.StripIn{Cands: cands, Taste: h.stripTaste(ctx, member, kid, now), Stats: stats, Recent: recent,
		Rows: rows, Local: moment.Local, Seed: fmt.Sprintf("%d|%s", member, moment.Local.Format("2006-01-02")), Kid: kid, Max: engine.StripMax})
	if complete {
		keepStrip(key, stripEntry{at: time.Now(), sig: sig, tiles: tiles})
	}
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": tiles, "partial": !complete})
}

// tasteRef is one title of a member's taste before its keywords are known.
type tasteRef struct {
	kind, tmdbID string
	genres       []string
	weight       float64
}

// stripTasteRefs are the member's taste titles: their own asks (2.0,
// fading over 120 days), their follows (1.0) and, for anyone but a Kid,
// the household Library (0.25, newest 200). A Kid's taste is their own.
func (h *Handlers) stripTasteRefs(ctx context.Context, member int64, kid bool, now time.Time) ([]tasteRef, error) {
	db := h.Store.Read()
	var out []tasteRef
	read := func(q string, scan func(*sql.Rows) (tasteRef, error), args ...any) error {
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scan(rows)
			if err != nil {
				return err
			}
			if t.tmdbID != "" && (t.kind == "movie" || t.kind == "show") {
				out = append(out, t)
			}
		}
		return rows.Err()
	}
	err := read(`SELECT a.kind, a.tmdb_id, p.asked_at FROM ask_people p JOIN asks a ON a.id = p.ask_id
		WHERE p.user_id = ? AND a.kind IN ('movie', 'show') AND a.tmdb_id != '' AND a.state != 'turned_down' ORDER BY p.asked_at DESC LIMIT 200`,
		func(rows *sql.Rows) (tasteRef, error) {
			var t tasteRef
			var at string
			err := rows.Scan(&t.kind, &t.tmdbID, &at)
			when, _ := time.Parse(database.TimeLayout, at)
			t.weight = 2 * math.Exp(-math.Max(0, now.Sub(when).Hours()/24)/120)
			return t, err
		}, member)
	if err != nil {
		return nil, err
	}
	libScan := func(w float64) func(*sql.Rows) (tasteRef, error) {
		return func(rows *sql.Rows) (tasteRef, error) {
			t := tasteRef{weight: w}
			var id sql.NullString
			var genres string
			err := rows.Scan(&t.kind, &id, &genres)
			t.tmdbID = id.String
			_ = json.Unmarshal([]byte(genres), &t.genres)
			return t, err
		}
	}
	if err := read(`SELECT t.kind, json_extract(t.source_ids, '$.tmdb'), t.genres FROM follows f JOIN titles t ON t.id = f.title_id
		WHERE f.user_id = ? AND t.kind IN ('movie', 'show')`, libScan(1), member); err != nil {
		return nil, err
	}
	if err := read(`SELECT kind, tmdb_id, '[]' FROM tmdb_follows WHERE user_id = ?
		UNION ALL SELECT 'movie', tmdb_id, '[]' FROM tmdb_movie_follows WHERE user_id = ?`, libScan(1), member, member); err != nil {
		return nil, err
	}
	if !kid {
		if err := read(`SELECT kind, json_extract(source_ids, '$.tmdb'), genres FROM titles
			WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
			ORDER BY created_at DESC, id DESC LIMIT 200`, libScan(0.25)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// StripTasteWeights are the weights of the member's taste titles (tests).
func (h *Handlers) StripTasteWeights(ctx context.Context, member int64, kid bool, now time.Time) ([]float64, error) {
	refs, err := h.stripTasteRefs(ctx, member, kid, now)
	out := make([]float64, len(refs))
	for i, r := range refs {
		out[i] = r.weight
	}
	return out, err
}

// stripKeywords is a title's cached genres and keywords.
type stripKeywords struct {
	Genres   []string `json:"g"`
	Keywords []string `json:"k"`
}

func stripKeywordKey(kind, id string) string { return "strip-kw|" + kind + "|" + id }

// stripTaste is the member's taste for the strip, from what is cached. The
// titles whose keywords are not cached yet are looked up in the
// background (never during the request), and the taste is rebuilt after.
func (h *Handlers) stripTaste(ctx context.Context, member int64, kid bool, now time.Time) engine.StripTaste {
	key := h.stripMemberKey(member) + fmt.Sprint(kid)
	if v, ok := stripTastes.Load(key); ok && time.Since(v.(stripTasteEntry).at) < stripTasteTTL {
		return v.(stripTasteEntry).taste
	}
	refs, err := h.stripTasteRefs(ctx, member, kid, now)
	if err != nil {
		return engine.StripTaste{}
	}
	cache := metadata.LookupCache{Store: h.Store}
	titles := make([]engine.TasteTitle, 0, len(refs))
	var missing []tasteRef
	for _, ref := range refs {
		t := engine.TasteTitle{Genres: ref.genres, Weight: ref.weight}
		if b, ok, _ := cache.Get(ctx, stripKeywordKey(ref.kind, ref.tmdbID), now); ok {
			var kw stripKeywords
			if json.Unmarshal(b, &kw) == nil {
				t.Keywords = kw.Keywords
				if len(t.Genres) == 0 {
					t.Genres = kw.Genres
				}
			}
		} else if len(missing) < stripKeywordFetches {
			missing = append(missing, ref)
		}
		titles = append(titles, t)
	}
	taste := engine.NewStripTaste(titles)
	stripTastes.Store(key, stripTasteEntry{at: time.Now(), taste: taste})
	if len(missing) > 0 {
		h.fetchStripKeywords(member, missing)
	}
	return taste
}

// fetchStripKeywords looks up the titles' genres and keywords in the
// background (one pass per member at a time, sharing the Browse fetch
// slots), caches them and drops the member's taste so it is rebuilt.
func (h *Handlers) fetchStripKeywords(member int64, refs []tasteRef) {
	key := h.stripMemberKey(member)
	if _, busy := stripFetches.LoadOrStore(key, true); busy {
		return
	}
	go jobs.Guard("explore-strip-keywords", func() {
		defer stripFetches.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		d, ok := metadata.DiscovererFor(ctx, h.Metadata)
		if !ok {
			return
		}
		cache := metadata.LookupCache{Store: h.Store}
		got := 0
		for _, ref := range refs {
			select {
			case h.Strip.Fetch <- struct{}{}:
			case <-ctx.Done():
				return
			}
			det, err := d.Details(ctx, ref.kind, ref.tmdbID)
			<-h.Strip.Fetch
			if err != nil {
				continue
			}
			b, _ := json.Marshal(stripKeywords{Genres: det.Genres, Keywords: det.Keywords})
			now := time.Now()
			if cache.Put(ctx, stripKeywordKey(ref.kind, ref.tmdbID), b, now, now.Add(stripKeywordTTL)) == nil {
				got++
			}
		}
		if got > 0 {
			prefix := key
			stripTastes.Range(func(k, _ any) bool {
				if strings.HasPrefix(k.(string), prefix) {
					stripTastes.Delete(k)
				}
				return true
			})
		}
	})
}

// StripDayCategoriesMax caps the categories one member counts in a day;
// real use stays far below it.
const StripDayCategoriesMax = 200

// exploreStripCounts answers POST /api/v1/explore/strip/counts {event,
// categories}: the strip showed these categories, or one was opened. The
// counts stay on this server and reorder only this member's strip.
func (h *Handlers) exploreStripCounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Event      string   `json:"event"`
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "body must be {event, categories}")
		return
	}
	field := map[string]string{"shown": "shown", "open": "opened"}[body.Event]
	if field == "" || len(body.Categories) == 0 || len(body.Categories) > StripRowsMax || (field == "opened" && len(body.Categories) != 1) {
		webserver.Fail(w, http.StatusBadRequest, "event must be shown (1 to 40 categories) or open (one category)")
		return
	}
	seen := map[string]bool{}
	for _, c := range body.Categories {
		if !accounts.ViewKeyOK(accounts.ViewCategory, c) {
			webserver.Fail(w, http.StatusBadRequest, "categories must be Explore strip categories")
			return
		}
		seen[c] = true
	}
	member, day := h.UserID(r), engine.Day(h.Explore.Now())
	err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		// A member's day holds at most StripDayCategoriesMax categories, so
		// made-up names cannot grow the database without end; past it only
		// categories already counted that day move.
		have, err := accounts.CategoryKeysOn(r.Context(), tx, member, day)
		if err != nil {
			return err
		}
		for c := range seen {
			if have >= StripDayCategoriesMax {
				if err := accounts.BumpCategoryCount(r.Context(), tx, member, c, day, field); err != nil {
					return err
				}
				continue
			}
			have++
			if err := accounts.AddCategoryCount(r.Context(), tx, member, c, day, field); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exploreStripReset answers DELETE /api/v1/explore/strip/counts: Reset my
// recommendations clears what the strip learned about the member.
func (h *Handlers) exploreStripReset(w http.ResponseWriter, r *http.Request) {
	member := h.UserID(r)
	err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		return accounts.ResetCategoryCounts(r.Context(), tx, member)
	})
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	h.forgetStrip(member)
	w.WriteHeader(http.StatusNoContent)
}
