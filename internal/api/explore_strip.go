package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// The Explore category strip, picked on the server for each member from the
// Browse sections already built (explore.PickStrip). It never waits for a
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

// stripRowsMax and stripRowLen bound the row titles a request may pass.
const (
	stripRowsMax = 40
	stripRowLen  = 80
)

// stripEntry is a member's one kept strip per tab and limit; sig is the
// day and a hash of the rows it was picked for.
type stripEntry struct {
	at    time.Time
	sig   string
	tiles []explore.CategoryTile
}

type stripTasteEntry struct {
	at    time.Time
	taste explore.StripTaste
}

var (
	stripAnswers sync.Map // %p|member|... -> stripEntry
	stripTastes  sync.Map // %p|member -> stripTasteEntry
	stripFetches sync.Map // %p|member -> true while a keyword pass runs
)

func (a *API) stripMemberKey(member int64) string { return fmt.Sprintf("%p|%d|", a, member) }

// forgetStrip drops a member's cached strips and taste.
func (a *API) forgetStrip(member int64) {
	prefix := a.stripMemberKey(member)
	for _, m := range []*sync.Map{&stripAnswers, &stripTastes} {
		m.Range(func(k, _ any) bool {
			if strings.HasPrefix(k.(string), prefix) {
				m.Delete(k)
			}
			return true
		})
	}
}

// stripRows are the row titles passed with row=; false when there are
// more than stripRowsMax or one is longer than stripRowLen.
func stripRows(r *http.Request) ([]string, bool) {
	raw := r.URL.Query()["row"]
	if len(raw) > stripRowsMax {
		return nil, false
	}
	var out []string
	for _, v := range raw {
		if len(v) > stripRowLen {
			return nil, false
		}
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out, true
}

// stripRowsHash is a short hash of the rows, normalised and sorted.
func stripRowsHash(rows []string) string {
	n := make([]string, len(rows))
	for i, v := range rows {
		n[i] = strings.ToLower(v)
	}
	sort.Strings(n)
	sum := sha256.Sum256([]byte(strings.Join(n, "|")))
	return hex.EncodeToString(sum[:])[:16]
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
func (a *API) exploreStrip(w http.ResponseWriter, r *http.Request) {
	sw, ok := exploreParams(w, r)
	if !ok {
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	ctx := r.Context()
	now := a.exploreNow()
	moment, _ := exploreMoment(r, now)
	member := a.person(r).UserID
	max := a.kidMax(r)
	rows, ok := stripRows(r)
	if !ok {
		fail(w, http.StatusBadRequest, "at most 40 row values of up to 80 characters")
		return
	}
	key := a.stripMemberKey(member) + sw + "|" + max
	sig := moment.Local.Format("2006-01-02") + "|" + stripRowsHash(rows)
	if v, ok := stripAnswers.Load(key); ok && v.(stripEntry).sig == sig && time.Since(v.(stripEntry).at) < stripTTL {
		a.exploreCaching(w, r, "private, no-cache")
		writeJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": v.(stripEntry).tiles, "partial": false})
		return
	}
	pg, page, kid, err := a.browsePage(ctx, r, sw)
	if err != nil {
		internal(w, err)
		return
	}
	cands := make([]explore.StripCand, 0, 128)
	for _, t := range page {
		cands = append(cands, explore.NewStripCand(explore.CategoryTile{BrowseTile: t, Section: "genres"}, pageGenreItems(pg, t.GenreName), now))
	}
	share := a.browseShare(sw, kid, max)
	complete := true
	for i, s := range explore.BrowseSections {
		res, built, _ := share.get(s.ID)
		if !built {
			complete = false
			missing := explore.BrowseSections[i:]
			bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			go supervisor.Guard("explore-strip-build", func() {
				defer cancel()
				for _, x := range missing {
					if _, have, _ := share.get(x.ID); !have {
						a.buildShared(bg, share, sw, kid, max, x.ID, false)
					}
				}
			})
			break
		}
		tiles, err := a.browseSectionTiles(ctx, r, sw, kid, pg, page, s.ID)
		if errors.Is(err, errBrowseBuilding) {
			complete = false
			break
		}
		if err != nil {
			internal(w, err)
			return
		}
		items := map[string][]explore.Item{}
		for _, c := range res {
			items[c.Cat.ID] = c.Items
		}
		for _, t := range tiles {
			cands = append(cands, explore.NewStripCand(t, items[t.Category], now))
		}
	}
	stats, recent, err := a.stripStats(ctx, member, now)
	if err != nil {
		internal(w, err)
		return
	}
	tiles := explore.PickStrip(explore.StripIn{Cands: cands, Taste: a.stripTaste(ctx, member, kid, now), Stats: stats, Recent: recent,
		Rows: rows, Local: moment.Local, Seed: fmt.Sprintf("%d|%s", member, moment.Local.Format("2006-01-02")), Kid: kid, Max: explore.StripMax})
	if complete {
		keepStrip(key, stripEntry{at: time.Now(), sig: sig, tiles: tiles})
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": tiles, "partial": !complete})
}

// pageGenreItems are the member's page titles of a genre.
func pageGenreItems(pg explore.Page, genre string) []explore.Item {
	var out []explore.Item
	seen := map[string]bool{}
	for _, row := range append(append([]explore.Row{}, pg.Head...), pg.Tail...) {
		for _, it := range row.Items {
			if seen[it.Key()] {
				continue
			}
			for _, g := range it.Genres {
				if g == genre {
					seen[it.Key()] = true
					out = append(out, it)
					break
				}
			}
		}
	}
	return out
}

// stripStats reads the member's strip counts of the last StripCountDays.
func (a *API) stripStats(ctx context.Context, member int64, now time.Time) (map[string]explore.StripStats, map[string]bool, error) {
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT category_id, day, shown, opened FROM explore_category_counts
		WHERE member_id = ? AND day >= ?`, member, explore.Day(now.AddDate(0, 0, -explore.StripCountDays)))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var days []explore.CategoryDay
	for rows.Next() {
		var d explore.CategoryDay
		var day string
		if err := rows.Scan(&d.Category, &day, &d.Shown, &d.Opened); err != nil {
			return nil, nil, err
		}
		if d.Day, err = time.Parse("2006-01-02", day); err == nil {
			days = append(days, d)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	stats, recent := explore.StripStatsFrom(days, now)
	return stats, recent, nil
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
func (a *API) stripTasteRefs(ctx context.Context, member int64, kid bool, now time.Time) ([]tasteRef, error) {
	db := a.s.Store.Read()
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
			when, _ := time.Parse(store.TimeLayout, at)
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

// stripKeywords is a title's cached genres and keywords.
type stripKeywords struct {
	Genres   []string `json:"g"`
	Keywords []string `json:"k"`
}

func stripKeywordKey(kind, id string) string { return "strip-kw|" + kind + "|" + id }

// stripTaste is the member's taste for the strip, from what is cached. The
// titles whose keywords are not cached yet are looked up in the
// background (never during the request), and the taste is rebuilt after.
func (a *API) stripTaste(ctx context.Context, member int64, kid bool, now time.Time) explore.StripTaste {
	key := a.stripMemberKey(member) + fmt.Sprint(kid)
	if v, ok := stripTastes.Load(key); ok && time.Since(v.(stripTasteEntry).at) < stripTasteTTL {
		return v.(stripTasteEntry).taste
	}
	refs, err := a.stripTasteRefs(ctx, member, kid, now)
	if err != nil {
		return explore.StripTaste{}
	}
	cache := store.LookupCache{Store: a.s.Store}
	titles := make([]explore.TasteTitle, 0, len(refs))
	var missing []tasteRef
	for _, ref := range refs {
		t := explore.TasteTitle{Genres: ref.genres, Weight: ref.weight}
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
	taste := explore.NewStripTaste(titles)
	stripTastes.Store(key, stripTasteEntry{at: time.Now(), taste: taste})
	if len(missing) > 0 {
		a.fetchStripKeywords(member, missing)
	}
	return taste
}

// fetchStripKeywords looks up the titles' genres and keywords in the
// background (one pass per member at a time, sharing the Browse fetch
// slots), caches them and drops the member's taste so it is rebuilt.
func (a *API) fetchStripKeywords(member int64, refs []tasteRef) {
	key := a.stripMemberKey(member)
	if _, busy := stripFetches.LoadOrStore(key, true); busy {
		return
	}
	go supervisor.Guard("explore-strip-keywords", func() {
		defer stripFetches.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		d, ok := metadata.DiscovererFor(ctx, a.s.Metadata)
		if !ok {
			return
		}
		cache := store.LookupCache{Store: a.s.Store}
		got := 0
		for _, ref := range refs {
			select {
			case browseFetch <- struct{}{}:
			case <-ctx.Done():
				return
			}
			det, err := d.Details(ctx, ref.kind, ref.tmdbID)
			<-browseFetch
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

// stripCategoryOK says id names a strip category: a Browse category or a
// genre tile's lower-case name.
func stripCategoryOK(id string) bool {
	if _, ok := explore.CategoryByID(id); ok {
		return true
	}
	if id == "" || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c == ' ' || c == '-' || c == '&') {
			return false
		}
	}
	return true
}

// exploreStripCounts answers POST /api/v1/explore/strip/counts {event,
// categories}: the strip showed these categories, or one was opened. The
// counts stay on this server and reorder only this member's strip.
func (a *API) exploreStripCounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Event      string   `json:"event"`
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "body must be {event, categories}")
		return
	}
	field := map[string]string{"shown": "shown", "open": "opened"}[body.Event]
	if field == "" || len(body.Categories) == 0 || len(body.Categories) > stripRowsMax || (field == "opened" && len(body.Categories) != 1) {
		fail(w, http.StatusBadRequest, "event must be shown (1 to 40 categories) or open (one category)")
		return
	}
	seen := map[string]bool{}
	for _, c := range body.Categories {
		if !stripCategoryOK(c) {
			fail(w, http.StatusBadRequest, "categories must be Explore strip categories")
			return
		}
		seen[c] = true
	}
	member, day := a.person(r).UserID, explore.Day(a.exploreNow())
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		for c := range seen {
			if _, err := tx.Exec(`INSERT INTO explore_category_counts (member_id, category_id, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, category_id, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, c, day); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exploreStripReset answers DELETE /api/v1/explore/strip/counts: Reset my
// recommendations clears what the strip learned about the member.
func (a *API) exploreStripReset(w http.ResponseWriter, r *http.Request) {
	member := a.person(r).UserID
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM explore_category_counts WHERE member_id = ?", member)
		return err
	})
	if err != nil {
		internal(w, err)
		return
	}
	a.forgetStrip(member)
	w.WriteHeader(http.StatusNoContent)
}
