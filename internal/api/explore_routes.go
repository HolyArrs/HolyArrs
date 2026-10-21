package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
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
func (a *API) explorePageFor(ctx context.Context, r *http.Request, member int64, sw, topKind, topRegion string) (explore.Page, map[string]any, bool, error) {
	now := a.exploreNow()
	day := explore.Day(now)
	moment, tz := exploreMoment(r, now)
	// Trending on streaming, as kept (never fetched here; warmed at
	// start): the rows under it skip its titles.
	svcTop := a.keptServiceTop(strings.ToUpper(r.URL.Query().Get("region")), sw)
	key := strconv.FormatInt(member, 10) + "|" + sw + "|" + topKind + "|" + topRegion + "|" + tz
	e := &a.explore
	pool, stale, err := a.readPool(ctx, member, sw, day, false)
	if err != nil {
		return explore.Page{}, nil, false, err
	}
	building := false
	if pool == nil || stale || pool.Version != explore.PoolVersion {
		// A pool from an older build has other rows: build it again.
		building = true
		a.queuePool(poolKey{member, sw}, now, false)
	}
	if pool == nil {
		if pool, _, err = a.readPool(ctx, member, sw, day, true); err != nil {
			return explore.Page{}, nil, false, err
		}
	}
	if pool != nil {
		e.mu.Lock()
		got, ok := e.pages[key]
		e.mu.Unlock()
		if ok && now.Sub(got.at) < explorePageTTL && got.built.Equal(pool.Built) {
			pg, err := a.titleExplore(ctx, member, got.page, moment, topRegion)
			return pg, got.top10, building, err
		}
	}
	if pool == nil {
		m, err := a.loadMember(ctx, member)
		if err != nil && err != sql.ErrNoRows {
			return explore.Page{}, nil, false, err
		}
		// The cold head is built from the Library, the asks and whatever
		// lists are cached (memory or SQLite), with no outside call.
		in := explore.Input{Member: member, Kid: m.kid, Switch: sw, Now: now, Services: m.services, Region: m.region}
		if err == nil {
			if full, ierr := a.exploreInput(ctx, m, sw, now); ierr == nil {
				in = full
			}
		}
		if m.kid {
			in.Allow = a.kidAllow(ctx, m.maxRating)
		}
		cold, err := explore.Build(ctx, exploreSource{a: a, cacheOnly: true}, in)
		if err != nil {
			return explore.Page{}, nil, false, err
		}
		pool = &cold
	}
	// Top 10 per kind the tab shows (All: both, as two rows). top10 is the
	// asked kind's list, with every kind's list under by_kind.
	var top10 map[string]any
	tops := map[string][]explore.Item{}
	if a.s.Top10 != nil && topKind != "" {
		lang := ""
		if topRegion == "global" {
			lang = "en"
		}
		byKind := map[string]any{}
		for _, kind := range explore.Kinds(sw) {
			status, _, body := a.top10Answer(ctx, r, kind, topRegion, lang)
			if status != http.StatusOK {
				continue
			}
			byKind[kind] = body
			for _, v := range body["items"].([]top10View) {
				if v.TMDBID != "" {
					tops[kind] = append(tops[kind], explore.Item{TMDBID: v.TMDBID, Kind: v.Kind, Name: v.Name, Poster: v.Poster, Backdrop: v.Backdrop, Owned: v.InLibrary})
				}
			}
		}
		if body, ok := byKind[topKind].(map[string]any); ok {
			top10 = map[string]any{}
			for k, v := range body {
				top10[k] = v
			}
			top10["by_kind"] = byKind
		}
	}
	pg := explore.AssembleTabAt(*pool, tops, svcTop, member, moment)
	e.mu.Lock()
	if e.pages == nil || len(e.pages) >= 1000 {
		e.pages = map[string]explorePage{}
	}
	if !building || pool.Date == day {
		e.pages[key] = explorePage{at: now, built: pool.Built, page: pg, top10: top10}
	}
	e.mu.Unlock()
	pg, err = a.titleExplore(ctx, member, pg, moment, topRegion)
	return pg, top10, building, err
}

// exploreViews joins rows with the Library; a Kid's rows are filtered
// again here (the response-stage Kid limit), and rows that fall under
// their minimum go.
func (a *API) exploreViews(ctx context.Context, r *http.Request, rows []explore.Row) ([]exploreRowView, error) {
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
		rows = explore.Page{Head: rows}.Filter(func(it explore.Item) bool { return ok[it.Key()] }).Head
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
		joined, err := a.joinItems(withFreshRow(ctx, row.Type), items, me, explore.RowMax)
		if err != nil {
			return nil, err
		}
		if len(joined) < explore.MinItems(row.Type) {
			continue
		}
		if row.Counted > 0 && len(joined) != row.Counted {
			// A {count} title must state the count the row shows.
			row.Title, row.Subtitle, row.Layer = row.Plain, "", explore.LayerPlain
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
	day := explore.Day(a.exploreNow())
	_ = a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, t := range types {
			if _, err := tx.Exec(`INSERT INTO explore_counts (member_id, row_type, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, row_type, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, t, day); err != nil {
				return err
			}
		}
		return nil
	})
}

func rowTypes(rows []exploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Type)
	}
	return out
}

// exploreParams reads the type switch and the Top 10 choice shared by the
// head and the tail.
func exploreParams(w http.ResponseWriter, r *http.Request) (sw, topKind, topRegion string, ok bool) {
	q := r.URL.Query()
	sw, ok = exploreSwitch(q.Get("type"))
	if !ok {
		fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	topKind = q.Get("top10_kind")
	switch {
	case sw == "movies":
		topKind = "movie"
	case sw == "shows":
		topKind = "show"
	case topKind == "":
		topKind = "movie"
	}
	if topKind != "movie" && topKind != "show" {
		fail(w, http.StatusBadRequest, "top10_kind must be movie or show")
		return "", "", "", false
	}
	topRegion = strings.ToUpper(q.Get("top10_region"))
	if topRegion == "" || topRegion == "GLOBAL" {
		topRegion = "global"
	} else if !isCountry(topRegion) {
		fail(w, http.StatusBadRequest, "top10_region must be global or a country code")
		return "", "", "", false
	}
	return sw, topKind, topRegion, true
}

// exploreHead answers GET /api/v1/explore/head: the Top 10 list and the
// head rows in one answer, from the stored pool. building says a pool is
// on its way (the page may ask again shortly).
func (a *API) exploreHead(w http.ResponseWriter, r *http.Request) {
	st := newServerTiming()
	sw, topKind, topRegion, ok := exploreParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	services, region := q.Get("services"), strings.ToUpper(q.Get("region"))
	if services != "" && (!metadata.ServiceIDs(services) || !isCountry(region)) {
		fail(w, http.StatusBadRequest, "services needs services=<ids joined by |> and region=<country code>")
		return
	}
	if region != "" && !isCountry(region) {
		fail(w, http.StatusBadRequest, "region must be a country code")
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
				internal(w, err)
				return
			}
			a.markMemberStale(r.Context(), me, a.exploreNow())
		}
	}
	pg, top10, building, err := a.explorePageFor(r.Context(), r, me, sw, topKind, topRegion)
	if err != nil {
		internal(w, err)
		return
	}
	st.mark("page")
	r = r.WithContext(withTop10Ranks(r.Context(), top10Ranks(top10)))
	// Top 10 stays in its slot as an empty marker (the page draws it from
	// top10); the hero is up to 6 titles not in the Library from Top 10 and
	// Trending, with Ask as its action.
	head := []exploreRowView{}
	for _, row := range pg.Head {
		if row.Type == explore.TypeTopServices {
			// The page fills this row itself (GET /explore/service-top).
			head = append(head, exploreRowView{ID: row.ID, Type: row.Type, Title: row.Title, Subtitle: row.Subtitle, TitleLayer: row.Layer, Items: []discoverItemView{}})
			continue
		}
		views, err := a.exploreViews(r.Context(), r, []explore.Row{row})
		if err != nil {
			internal(w, err)
			return
		}
		head = append(head, views...)
	}
	hero, err := a.exploreHero(r, pg, sw == "all", me)
	if err != nil {
		internal(w, err)
		return
	}
	st.mark("views")
	a.exploreCount(r.Context(), me, "shown", rowTypes(head))
	a.exploreTitleCount(r.Context(), me, "shown", rowLayers(head))
	st.mark("counts")
	a.prefetchTrailers(r, exploreTrailerTitles(hero, head))
	for i := range head {
		a.fillTrailerKeys(r, head[i].Items)
	}
	st.mark("trailers")
	a.fillRowsCardArt(r, head)
	a.fillStreaming(r, head)
	st.mark("cardart")
	a.exploreCaching(w, r, "private, no-cache")
	st.write(w)
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "building": building, "seeds": pg.Seeds, "tail_cap": pg.Cap,
		"tail_rows": len(pg.Tail), "top10": top10, "hero": hero, "head": head, "taste": pg.Taste})
}

// exploreTail answers GET /api/v1/explore/tail?page=n: the next 4 rows of
// the generated tail; more says another page follows.
func (a *API) exploreTail(w http.ResponseWriter, r *http.Request) {
	sw, topKind, topRegion, ok := exploreParams(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 0 || n > 100 {
		fail(w, http.StatusBadRequest, "page must be a number from 0")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	me := a.person(r).UserID
	pg, top10, _, err := a.explorePageFor(r.Context(), r, me, sw, topKind, topRegion)
	if err != nil {
		internal(w, err)
		return
	}
	r = r.WithContext(withTop10Ranks(r.Context(), top10Ranks(top10)))
	slice, more := pg.TailSlice(n)
	rows, err := a.exploreViews(r.Context(), r, slice)
	if err != nil {
		internal(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "page": n, "more": more, "rows": rows})
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
		fail(w, http.StatusBadRequest, "body must be {row_type, event}")
		return
	}
	field := map[string]string{"open": "opens", "ask": "asks"}[body.Event]
	if field == "" || (explore.MinItems(body.RowType) == 0 && body.RowType != "hero") {
		fail(w, http.StatusBadRequest, "event must be open or ask, row_type an Explore row type")
		return
	}
	if body.TitleLayer != "" && !validTitleLayer(body.TitleLayer) && body.TitleLayer != explore.LayerFixed {
		fail(w, http.StatusBadRequest, "title_layer must be an Explore title layer")
		return
	}
	a.exploreCount(r.Context(), a.person(r).UserID, field, []string{body.RowType})
	a.exploreTitleCount(r.Context(), a.person(r).UserID, field, []string{body.TitleLayer})
	w.WriteHeader(http.StatusNoContent)
}


// exploreTrailerTitles are the titles whose trailers are looked up ahead of
// time: the hero first, then the first 4 cards of the first 3 rows.
func exploreTrailerTitles(hero []discoverItemView, head []exploreRowView) [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	add := func(v discoverItemView) {
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

// exploreHero picks the hero (explore.PickHero) from a pool of candidates
// that pass the quality gate: a known trailer, a logo and a backdrop. When
// trailers are off or too few are known yet, art-only titles top it up.
func (a *API) exploreHero(r *http.Request, pg explore.Page, all bool, me int64) ([]discoverItemView, error) {
	n := explore.HeroPool
	if all {
		n += 2 * explore.HeroShuffleTop
	}
	cands := explore.HeroRank(explore.HeroCandidates(pg.Head, pg.Leans, n), pg.Taste)
	views, err := a.exploreViews(r.Context(), r, []explore.Row{{Items: cands}})
	if err != nil || len(views) != 1 {
		return []discoverItemView{}, err
	}
	byKey := map[string]discoverItemView{}
	var titles [][2]string
	for _, v := range views[0].Items {
		if !v.InLibrary {
			byKey[v.Kind+":"+v.TMDBID] = v
			titles = append(titles, [2]string{v.Kind, v.TMDBID})
		}
	}
	a.prefetchTrailers(r, titles)
	vs := make([]discoverItemView, 0, len(byKey))
	for _, it := range cands {
		if v, ok := byKey[it.Key()]; ok {
			vs = append(vs, v)
		}
	}
	a.fillTrailerKeys(r, vs)
	a.fillCardArt(r, vs)
	items := map[string]explore.Item{}
	for _, it := range cands {
		items[it.Key()] = it
	}
	var ready, artOnly []explore.Item
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
	if len(ready) < explore.HeroCount/2 {
		ready = append(ready, artOnly...)
	}
	day := explore.Day(a.exploreNow())
	picked := explore.PickHero(ready, all, me, day)
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
