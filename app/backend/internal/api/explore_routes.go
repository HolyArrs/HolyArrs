package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// exploreRowView is one row of the head or tail, with the Library joined.
type exploreRowView struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	// Subtitle is a small line under the title; TitleLayer says where the
	// title came from (fixed, plain or a template layer), for measuring.
	Subtitle   string             `json:"subtitle,omitempty"`
	TitleLayer string             `json:"title_layer,omitempty"`
	Wide       bool               `json:"wide,omitempty"`
	Items      []discoverItemView `json:"items"`
}

// exploreSwitch reads ?type=: all (default), movies or shows.
func exploreSwitch(v string) (string, bool) {
	switch v {
	case "", "all":
		return "all", true
	case "movies", "shows":
		return v, true
	}
	return "", false
}

// explorePageFor assembles the member's page: today's pool, else the
// newest older one, else cold-start rows from the list cache. It never
// waits for outside calls; a missing or stale pool is queued instead.
func (a *API) explorePageFor(ctx context.Context, r *http.Request, member int64, sw string) (engine.Page, bool, error) {
	now := a.exploreNow()
	day := engine.Day(now)
	moment, tz := exploreMoment(r, now)
	region := r.URL.Query().Get("region")
	key := strconv.FormatInt(member, 10) + "|" + sw + "|" + tz
	e := &a.explore
	pool, stale, err := a.readPool(ctx, member, sw, day, false)
	if err != nil {
		return engine.Page{}, false, err
	}
	building := false
	if pool == nil || stale || pool.Version != engine.PoolVersion {
		// A pool from an older build has other rows: build it again.
		building = true
		a.queuePool(poolKey{member, sw}, now, false)
	}
	if pool == nil {
		if pool, _, err = a.readPool(ctx, member, sw, day, true); err != nil {
			return engine.Page{}, false, err
		}
	}
	if pool != nil {
		e.mu.Lock()
		got, ok := e.pages[key]
		e.mu.Unlock()
		if ok && now.Sub(got.at) < explorePageTTL && got.built.Equal(pool.Built) {
			pg, err := a.titleExplore(ctx, member, got.page, moment)
			return pg, building, err
		}
	}
	if pool == nil {
		m, err := a.loadMember(ctx, member)
		if err != nil && err != sql.ErrNoRows {
			return engine.Page{}, false, err
		}
		// The cold head is built from the Library, the asks and whatever
		// lists are cached (memory or SQLite), with no outside call.
		in := engine.Input{Member: member, Kid: m.kid, Switch: sw, Now: now, Services: m.services, Region: m.region}
		if err == nil {
			if full, ierr := a.exploreInput(ctx, m, sw, now); ierr == nil {
				in = full
			}
		}
		if m.kid {
			in.Allow = a.kidAllow(ctx, m.maxRating)
		}
		cold, err := engine.Build(ctx, exploreSource{a: a, cacheOnly: true}, in)
		if err != nil {
			return engine.Page{}, false, err
		}
		pool = &cold
	}
	pg := a.poolPage(*pool, member, sw, region, moment)
	e.mu.Lock()
	if e.pages == nil || len(e.pages) >= 1000 {
		e.pages = map[string]explorePage{}
	}
	if !building || pool.Date == day {
		e.pages[key] = explorePage{at: now, built: pool.Built, page: pg}
	}
	e.mu.Unlock()
	pg, err = a.titleExplore(ctx, member, pg, moment)
	return pg, building, err
}

// poolPage assembles the member's page from a pool for a moment, with no
// request: Trending on streaming as kept (never fetched here; warmed at
// start), so the rows under it skip its titles.
func (a *API) poolPage(p engine.Pool, member int64, sw, region string, m engine.Moment) engine.Page {
	return engine.AssembleTabAt(p, a.keptServiceTop(strings.ToUpper(region), sw), member, m)
}

// exploreViews joins rows with the Library; a Kid's rows are filtered
// again here (the response-stage Kid limit), and rows that fall under
// their minimum go.
func (a *API) exploreViews(ctx context.Context, r *http.Request, rows []engine.Row) ([]exploreRowView, error) {
	if max := a.kidMax(r); max != "" {
		var all []metadata.DiscoverItem
		for _, row := range rows {
			for _, it := range row.Items {
				all = append(all, metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind})
			}
		}
		ok := map[string]bool{}
		for _, it := range a.kidItems(ctx, max, all, 0) {
			ok[it.Kind+":"+it.TMDBID] = true
		}
		rows = engine.Page{Head: rows}.Filter(func(it engine.Item) bool { return ok[it.Key()] }).Head
	}
	me := a.person(r).UserID
	out := []exploreRowView{}
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
		items, err := a.dropNeverAdd(ctx, items)
		if err != nil {
			return nil, err
		}
		joined, err := a.joinItems(withFreshRow(ctx, row.Type), items, me, engine.RowMax)
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
		out = append(out, exploreRowView{ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, TitleLayer: row.Layer, Wide: row.Wide, Items: joined})
	}
	return out, nil
}

// exploreCount adds to the member's measuring counts for row types.
func (a *API) exploreCount(ctx context.Context, member int64, field string, types []string) {
	if len(types) == 0 {
		return
	}
	day := engine.Day(a.exploreNow())
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		return accounts.AddRowCounts(ctx, tx, member, day, field, types)
	})
}

func rowTypes(rows []exploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Type)
	}
	return out
}

// exploreParams reads the type switch shared by the head and the tail.
func exploreParams(w http.ResponseWriter, r *http.Request) (sw string, ok bool) {
	sw, ok = exploreSwitch(r.URL.Query().Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
	}
	return sw, ok
}

// exploreHead answers GET /api/v1/explore/head: the head rows in one answer, from the stored pool. building says a pool is
// on its way (the page may ask again shortly).
func (a *API) exploreHead(w http.ResponseWriter, r *http.Request) {
	st := webserver.NewServerTiming()
	sw, ok := exploreParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	services, region := q.Get("services"), strings.ToUpper(q.Get("region"))
	if services != "" && (!metadata.ServiceIDs(services) || !isCountry(region)) {
		webserver.Fail(w, http.StatusBadRequest, "services needs services=<ids joined by |> and region=<country code>")
		return
	}
	if region != "" && !isCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "region must be a country code")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	me := a.person(r).UserID
	if q.Has("services") {
		var old, oldRegion string
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT services, region FROM explore_prefs WHERE member_id = ?", me).Scan(&old, &oldRegion)
		if old != services || oldRegion != region {
			err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
				_, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (?, ?, ?)
					ON CONFLICT (member_id) DO UPDATE SET services = excluded.services, region = excluded.region`, me, services, region)
				return err
			})
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			a.markMemberStale(r.Context(), me, a.exploreNow())
		}
	}
	pg, building, err := a.explorePageFor(r.Context(), r, me, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	st.Mark("page")
	// The hero is up to 6 titles not in the Library from Trending, with Ask
	// as its action.
	head := []exploreRowView{}
	for _, row := range pg.Head {
		if row.Type == engine.TypeTopServices {
			// The page fills this row itself (GET /explore/service-top).
			head = append(head, exploreRowView{ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, TitleLayer: row.Layer, Items: []discoverItemView{}})
			continue
		}
		views, err := a.exploreViews(r.Context(), r, []engine.Row{row})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		head = append(head, views...)
	}
	hero, err := a.exploreHero(r, pg, sw == "all", me)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	st.Mark("views")
	a.exploreCount(r.Context(), me, "shown", rowTypes(head))
	a.exploreTitleCount(r.Context(), me, "shown", rowLayers(head))
	st.Mark("counts")
	for i := range head {
		a.fillTrailerKeys(r, head[i].Items)
	}
	st.Mark("trailers")
	a.fillRowsCardArt(r, head)
	a.fillStreaming(r, head)
	st.Mark("cardart")
	a.exploreCaching(w, r, "private, no-cache")
	st.Write(w)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "building": building, "seeds": pg.Seeds, "tail_cap": pg.Cap,
		"tail_rows": len(pg.Tail), "hero": hero, "head": head, "taste": pg.Taste})
}

// exploreTail answers GET /api/v1/explore/tail?page=n: the next 4 rows of
// the generated tail; more says another page follows.
func (a *API) exploreTail(w http.ResponseWriter, r *http.Request) {
	sw, ok := exploreParams(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 0 || n > 100 {
		webserver.Fail(w, http.StatusBadRequest, "page must be a number from 0")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	me := a.person(r).UserID
	pg, _, err := a.explorePageFor(r.Context(), r, me, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	slice, more := pg.TailSlice(n)
	rows, err := a.exploreViews(r.Context(), r, slice)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	a.exploreCount(r.Context(), me, "shown", rowTypes(rows))
	a.exploreTitleCount(r.Context(), me, "shown", rowLayers(rows))
	for i := range rows {
		a.fillTrailerKeys(r, rows[i].Items)
	}
	a.fillRowsCardArt(r, rows)
	a.fillStreaming(r, rows)
	a.exploreCaching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "page": n, "more": more, "rows": rows})
}

// exploreViewKeyOK is the check accounts calls before it counts a view:
// the row types, title layers and strip categories Explore knows.
func exploreViewKeyOK(kind, key string) bool {
	switch kind {
	case accounts.ViewRow:
		return engine.MinItems(key) != 0 || key == "hero"
	case accounts.ViewLayer:
		return validTitleLayer(key) || key == engine.LayerFixed
	case accounts.ViewCategory:
		return stripCategoryOK(key)
	}
	return false
}

// exploreCounts answers POST /api/v1/explore/counts {row_type, event}:
// a card opened or an ask started from a row. Counts only measure; they
// never change what Explore shows.
func (a *API) exploreCounts(w http.ResponseWriter, r *http.Request) {
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
	a.exploreCount(r.Context(), a.person(r).UserID, field, []string{body.RowType})
	a.exploreTitleCount(r.Context(), a.person(r).UserID, field, []string{body.TitleLayer})
	w.WriteHeader(http.StatusNoContent)
}

// heroCandidates are the titles the hero picks from (before its quality
// gate), best first; All takes 2 x HeroShuffleTop more (the tabs' share).
func heroCandidates(pg engine.Page, all bool) []engine.Item {
	n := engine.HeroPool
	if all {
		n += 2 * engine.HeroShuffleTop
	}
	return engine.HeroRank(engine.HeroCandidates(pg.Head, pg.Leans, n), pg.Taste)
}

// exploreTrailerTitles are the movies and shows a page shows first, each
// once: the hero's candidates, then the first 4 cards of the first 3 rows
// that have cards (the trailer pass hands it the hero's candidates only).
func exploreTrailerTitles(hero []engine.Item, head []engine.Row) [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	add := func(v engine.Item) {
		if (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" && !seen[v.Kind+v.TMDBID] {
			seen[v.Kind+v.TMDBID] = true
			out = append(out, [2]string{v.Kind, v.TMDBID})
		}
	}
	for _, v := range hero {
		add(v)
	}
	rows := 0
	for _, row := range head {
		if len(row.Items) == 0 || rows == 3 {
			continue
		}
		rows++
		for i, v := range row.Items {
			if i == 4 {
				break
			}
			add(v)
		}
	}
	return out
}

// exploreHero picks the hero (engine.PickHero) from a pool of candidates
// that pass the quality gate: a known trailer, a logo and a backdrop. When
// trailers are off or too few are known yet, art-only titles top it up.
func (a *API) exploreHero(r *http.Request, pg engine.Page, all bool, me int64) ([]discoverItemView, error) {
	cands := heroCandidates(pg, all)
	views, err := a.exploreViews(r.Context(), r, []engine.Row{{Items: cands}})
	if err != nil || len(views) != 1 {
		return []discoverItemView{}, err
	}
	byKey := map[string]discoverItemView{}
	for _, v := range views[0].Items {
		if !v.InLibrary {
			byKey[v.Kind+":"+v.TMDBID] = v
		}
	}
	vs := make([]discoverItemView, 0, len(byKey))
	for _, it := range cands {
		if v, ok := byKey[it.Key()]; ok {
			vs = append(vs, v)
		}
	}
	a.fillTrailerKeys(r, vs)
	a.fillCardArt(r, vs)
	items := map[string]engine.Item{}
	for _, it := range cands {
		items[it.Key()] = it
	}
	var ready, artOnly []engine.Item
	for _, v := range vs {
		it := items[v.Kind+":"+v.TMDBID]
		switch {
		case !heroArtReady(v):
		case v.TrailerKey != "":
			ready = append(ready, it)
		default:
			artOnly = append(artOnly, it)
		}
	}
	if len(ready) < engine.HeroCount/2 {
		ready = append(ready, artOnly...)
	}
	day := engine.Day(a.exploreNow())
	picked := engine.PickHero(ready, all, me, day)
	heroShown.Store(me, picked) // New & Popular's hero leaves these out
	out := make([]discoverItemView, 0, len(picked))
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

// heroArtReady says the hero card has a logo and a backdrop.
func heroArtReady(v discoverItemView) bool {
	return v.CardArt.Logo != "" && (v.Backdrop != "" || v.CardArt.Art != "")
}
