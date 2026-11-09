package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// ExploreRowView is one row of the head or tail, with the Library joined.
type ExploreRowView struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	// Subtitle is a small line under the title; TitleLayer says where the
	// title came from (fixed, plain or a template layer), for measuring.
	Subtitle   string     `json:"subtitle,omitempty"`
	TitleLayer string     `json:"title_layer,omitempty"`
	Wide       bool       `json:"wide,omitempty"`
	Items      []ItemView `json:"items"`
}

// explorePageFor assembles the member's page: today's pool, else the
// newest older one, else cold-start rows from the list cache. It never
// waits for outside calls; a missing or stale pool is queued instead.
func (h *Handlers) ExplorePageFor(ctx context.Context, r *http.Request, member int64, sw string) (engine.Page, bool, error) {
	now := h.Explore.Now()
	day := engine.Day(now)
	moment, tz := ExploreMoment(r, now)
	region := r.URL.Query().Get("region")
	key := strconv.FormatInt(member, 10) + "|" + sw + "|" + tz
	e := h.Explore
	pool, stale, err := h.Explore.ReadPool(ctx, member, sw, day, false)
	if err != nil {
		return engine.Page{}, false, err
	}
	building := false
	if pool == nil || stale || pool.Version != engine.PoolVersion {
		// A pool from an older build has other rows: build it again.
		building = true
		h.Explore.QueuePool(PoolKey{Member: member, Switch: sw}, now, false)
	}
	if pool == nil {
		if pool, _, err = h.Explore.ReadPool(ctx, member, sw, day, true); err != nil {
			return engine.Page{}, false, err
		}
	}
	if pool != nil {
		got, ok := e.Page(key)
		if ok && now.Sub(got.At) < ExplorePageTTL && got.Built.Equal(pool.Built) {
			pg, err := TitleExplore(ctx, h.Store, member, got.Page, moment)
			return pg, building, err
		}
	}
	if pool == nil {
		m, err := h.Explore.LoadMember(ctx, member)
		if err != nil && err != sql.ErrNoRows {
			return engine.Page{}, false, err
		}
		// The cold head is built from the Library, the asks and whatever
		// lists are cached (memory or SQLite), with no outside call.
		in := engine.Input{Member: member, Kid: m.Kid, Switch: sw, Now: now, Services: m.Services, Region: m.Region}
		if err == nil {
			if full, ierr := h.Explore.Input(ctx, m, sw, now); ierr == nil {
				in = full
			}
		}
		if m.Kid {
			in.Allow = h.Explore.KidAllow(ctx, m.MaxRating)
		}
		cold, err := engine.Build(ctx, ExploreSource{E: h.Explore, CacheOnly: true}, in)
		if err != nil {
			return engine.Page{}, false, err
		}
		pool = &cold
	}
	pg := h.PoolPage(*pool, member, sw, region, moment)
	if !building || pool.Date == day {
		e.KeepPage(key, ExplorePage{At: now, Built: pool.Built, Page: pg})
	}
	pg, err = TitleExplore(ctx, h.Store, member, pg, moment)
	return pg, building, err
}

// poolPage assembles the member's page from a pool for a moment, with no
// request: Trending on streaming as kept (never fetched here; warmed at
// start), so the rows under it skip its titles.
func (h *Handlers) PoolPage(p engine.Pool, member int64, sw, region string, m engine.Moment) engine.Page {
	return engine.AssembleTabAt(p, h.ServiceTopCache.Kept(strings.ToUpper(region), sw, TopServicesMax), member, m)
}

// exploreViews joins rows with the Library; a Kid's rows are filtered
// again here (the response-stage Kid limit), and rows that fall under
// their minimum go.
func (h *Handlers) ExploreViews(ctx context.Context, r *http.Request, rows []engine.Row) ([]ExploreRowView, error) {
	if max := h.KidMax(r); max != "" {
		var all []metadata.DiscoverItem
		for _, row := range rows {
			for _, it := range row.Items {
				all = append(all, metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind})
			}
		}
		ok := map[string]bool{}
		for _, it := range h.KidItems(ctx, max, all, 0) {
			ok[it.Kind+":"+it.TMDBID] = true
		}
		rows = engine.Page{Head: rows}.Filter(func(it engine.Item) bool { return ok[it.Key()] }).Head
	}
	me := h.UserID(r)
	out := []ExploreRowView{}
	for _, row := range rows {
		items := make([]metadata.DiscoverItem, 0, len(row.Items))
		for _, it := range row.Items {
			g := it.Genres
			if g == nil {
				g = []string{}
			}
			items = append(items, metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind, Name: it.Name, Year: it.Year, Poster: it.Poster,
				Backdrop: it.Backdrop, Overview: it.Overview, Genres: g, Released: it.Released})
		}
		items, err := h.DropNeverAdd(ctx, items)
		if err != nil {
			return nil, err
		}
		joined, err := h.JoinItems(WithFreshRow(ctx, row.Type), items, me, engine.RowMax)
		if err != nil {
			return nil, err
		}
		if len(joined) < engine.MinItems(row.Type) {
			continue
		}
		if row.Counted > 0 && len(joined) != row.Counted {
			// A {count} title must state the count the row shows.
			row.Title, row.Subtitle, row.Layer = row.Plain, "", engine.LayerPlain
		}
		out = append(out, ExploreRowView{ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, TitleLayer: row.Layer, Wide: row.Wide, Items: joined})
	}
	return out, nil
}

// exploreCount adds to the member's measuring counts for row types.
func (h *Handlers) exploreCount(ctx context.Context, member int64, field string, types []string) {
	if len(types) == 0 {
		return
	}
	day := engine.Day(h.Explore.Now())
	_ = h.Store.Write(ctx, func(tx *sql.Tx) error {
		return accounts.AddRowCounts(ctx, tx, member, day, field, types)
	})
}

func rowTypes(rows []ExploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Type)
	}
	return out
}

// exploreHead answers GET /api/v1/explore/head: the head rows in one answer, from the stored pool. building says a pool is
// on its way (the page may ask again shortly).
func (h *Handlers) exploreHead(w http.ResponseWriter, r *http.Request) {
	st := webserver.NewServerTiming()
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	services, region := q.Get("services"), strings.ToUpper(q.Get("region"))
	if services != "" && (!metadata.ServiceIDs(services) || !IsCountry(region)) {
		webserver.Fail(w, http.StatusBadRequest, "services needs services=<ids joined by |> and region=<country code>")
		return
	}
	if region != "" && !IsCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "region must be a country code")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	me := h.UserID(r)
	if q.Has("services") {
		var old, oldRegion string
		_ = h.Store.Read().QueryRowContext(r.Context(), "SELECT services, region FROM explore_prefs WHERE member_id = ?", me).Scan(&old, &oldRegion)
		if old != services || oldRegion != region {
			err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
				_, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (?, ?, ?)
					ON CONFLICT (member_id) DO UPDATE SET services = excluded.services, region = excluded.region`, me, services, region)
				return err
			})
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			h.Explore.MarkMemberStale(r.Context(), me, h.Explore.Now())
		}
	}
	pg, building, err := h.ExplorePageFor(r.Context(), r, me, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	st.Mark("page")
	// The hero is up to 6 titles not in the Library from Trending, with Ask
	// as its action.
	head := []ExploreRowView{}
	for _, row := range pg.Head {
		if row.Type == engine.TypeTopServices {
			// The page fills this row itself (GET /explore/service-top).
			head = append(head, ExploreRowView{ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, TitleLayer: row.Layer, Items: []ItemView{}})
			continue
		}
		views, err := h.ExploreViews(r.Context(), r, []engine.Row{row})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		head = append(head, views...)
	}
	hero, err := h.ExploreHero(r, pg, sw == "all", me)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	st.Mark("views")
	h.exploreCount(r.Context(), me, "shown", rowTypes(head))
	ExploreTitleCount(r.Context(), h.Store, h.Explore.Now(), me, "shown", RowLayers(head))
	st.Mark("counts")
	for i := range head {
		h.FillTrailerKeys(r, head[i].Items)
	}
	st.Mark("trailers")
	h.FillRowsCardArt(r, head)
	h.fillStreamingRows(r, head)
	st.Mark("cardart")
	h.Caching(w, r, "private, no-cache")
	st.Write(w)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "building": building, "seeds": pg.Seeds, "tail_cap": pg.Cap,
		"tail_rows": len(pg.Tail), "hero": hero, "head": head, "taste": pg.Taste})
}

// exploreTail answers GET /api/v1/explore/tail?page=n: the next 4 rows of
// the generated tail; more says another page follows.
func (h *Handlers) exploreTail(w http.ResponseWriter, r *http.Request) {
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 0 || n > 100 {
		webserver.Fail(w, http.StatusBadRequest, "page must be a number from 0")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	me := h.UserID(r)
	pg, _, err := h.ExplorePageFor(r.Context(), r, me, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	slice, more := pg.TailSlice(n)
	rows, err := h.ExploreViews(r.Context(), r, slice)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.exploreCount(r.Context(), me, "shown", rowTypes(rows))
	ExploreTitleCount(r.Context(), h.Store, h.Explore.Now(), me, "shown", RowLayers(rows))
	for i := range rows {
		h.FillTrailerKeys(r, rows[i].Items)
	}
	h.FillRowsCardArt(r, rows)
	h.fillStreamingRows(r, rows)
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "page": n, "more": more, "rows": rows})
}

// exploreCounts answers POST /api/v1/explore/counts {row_type, event}:
// a card opened or an ask started from a row. Counts only measure; they
// never change what Explore shows.
func (h *Handlers) exploreCounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RowType string `json:"row_type"`
		Event   string `json:"event"`
		// TitleLayer is the row's title_layer, when the page knows it.
		TitleLayer string `json:"title_layer"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "body must be {row_type, event}")
		return
	}
	field := map[string]string{"open": "opens", "ask": "asks"}[body.Event]
	if field == "" || !accounts.ViewKeyOK(accounts.ViewRow, body.RowType) {
		webserver.Fail(w, http.StatusBadRequest, "event must be open or ask, row_type an Explore row type")
		return
	}
	if body.TitleLayer != "" && !accounts.ViewKeyOK(accounts.ViewLayer, body.TitleLayer) {
		webserver.Fail(w, http.StatusBadRequest, "title_layer must be an Explore title layer")
		return
	}
	h.exploreCount(r.Context(), h.UserID(r), field, []string{body.RowType})
	ExploreTitleCount(r.Context(), h.Store, h.Explore.Now(), h.UserID(r), field, []string{body.TitleLayer})
	w.WriteHeader(http.StatusNoContent)
}

// exploreHero picks the hero (engine.PickHero) from a pool of candidates
// that pass the quality gate: a known trailer, a logo and a backdrop. When
// trailers are off or too few are known yet, art-only titles top it up.
func (h *Handlers) ExploreHero(r *http.Request, pg engine.Page, all bool, me int64) ([]ItemView, error) {
	cands := HeroCandidates(pg, all)
	views, err := h.ExploreViews(r.Context(), r, []engine.Row{{Items: cands}})
	if err != nil || len(views) != 1 {
		return []ItemView{}, err
	}
	byKey := map[string]ItemView{}
	for _, v := range views[0].Items {
		if !v.InLibrary {
			byKey[v.Kind+":"+v.TMDBID] = v
		}
	}
	vs := make([]ItemView, 0, len(byKey))
	for _, it := range cands {
		if v, ok := byKey[it.Key()]; ok {
			vs = append(vs, v)
		}
	}
	h.FillTrailerKeys(r, vs)
	h.FillCardArt(r, vs)
	items := map[string]engine.Item{}
	for _, it := range cands {
		items[it.Key()] = it
	}
	var ready, artOnly []engine.Item
	for _, v := range vs {
		it := items[v.Kind+":"+v.TMDBID]
		switch {
		case !HeroArtReady(v):
		case v.TrailerKey != "":
			ready = append(ready, it)
		default:
			artOnly = append(artOnly, it)
		}
	}
	if len(ready) < engine.HeroCount/2 {
		ready = append(ready, artOnly...)
	}
	day := engine.Day(h.Explore.Now())
	picked := engine.PickHero(ready, all, me, day)
	HeroShown.Store(me, picked) // New & Popular's hero leaves these out
	out := make([]ItemView, 0, len(picked))
	for _, it := range picked {
		for _, v := range vs {
			if v.Kind+":"+v.TMDBID == it.Key() {
				out = append(out, v)
				break
			}
		}
	}
	return out, nil
}

// HeroArtReady says the hero card has a logo and a backdrop.
func HeroArtReady(v ItemView) bool {
	return v.CardArt.Logo != "" && (v.Backdrop != "" || v.CardArt.Art != "")
}

// HeroShown is each member's latest Explore hero (member id -> []engine.Item),
// so New & Popular's hero can leave those titles out.
var HeroShown = NewTTLMap(24*time.Hour, 10000)

// RowLayers are the rows' title layers, in order.
func RowLayers(rows []ExploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TitleLayer)
	}
	return out
}

// FillRowsCardArt fills the wide card art of every row's cards at once.
func (h *Handlers) FillRowsCardArt(r *http.Request, rows []ExploreRowView) {
	var all []ItemView
	for _, row := range rows {
		all = append(all, row.Items...)
	}
	h.FillCardArt(r, all)
	n := 0
	for i := range rows {
		copy(rows[i].Items, all[n:n+len(rows[i].Items)])
		n += len(rows[i].Items)
	}
}

// fillStreamingRows marks the rows' cards on the member's services.
func (h *Handlers) fillStreamingRows(r *http.Request, rows []ExploreRowView) {
	items := make([][]ItemView, len(rows))
	for i := range rows {
		items[i] = rows[i].Items
	}
	h.FillStreaming(r, items...)
}
