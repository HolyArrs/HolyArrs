package discover

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// newHero picks the hero: the best-scoring new titles of the rail, the
// services row and the hot and new rows that pass Explore's quality gate
// (a trailer, a logo and a backdrop), leaving out Explore's own hero.
func (hd *Handlers) newHero(r *http.Request, pg newPage, me int64) ([]ItemView, error) {
	skip := map[string]bool{}
	if v, ok := HeroShown.Load(me); ok {
		for _, it := range v.([]engine.Item) {
			skip[it.Key()] = true
		}
	}
	var cands []engine.Item
	seen := map[string]bool{}
	take := func(it engine.Item) {
		if !seen[it.Key()] && !skip[it.Key()] && pg.Scores[it.Key()] > 0 {
			seen[it.Key()] = true
			cands = append(cands, it)
		}
	}
	for _, d := range pg.Rail {
		for _, it := range d.Items {
			take(it.Item)
		}
	}
	for _, row := range pg.Rows {
		if row.Type == engine.TypeNPServices || row.Type == engine.TypeNPTopShows || row.Type == engine.TypeNPTopFilms {
			for _, it := range row.Items {
				take(it)
			}
		}
	}
	engine.SortByScore(cands, func(it engine.Item) float64 { return pg.Scores[it.Key()] })
	cands = cands[:min(len(cands), 24)]
	views, err := hd.ExploreViews(r.Context(), r, []engine.Row{{Items: cands}})
	if err != nil || len(views) != 1 {
		return []ItemView{}, err
	}
	var vs []ItemView
	var titles [][2]string
	for _, v := range views[0].Items {
		if !v.InLibrary {
			vs = append(vs, v)
			titles = append(titles, [2]string{v.Kind, v.TMDBID})
		}
	}
	// The trailer pass reads only Explore's pools: the titles this hero
	// picks from are noted for its next round (no lookup here).
	if hd.KidMax(r) == "" {
		hd.NoteTrailers(metadata.TrailerLanguage(r), titles)
	}
	hd.FillTrailerKeys(r, vs)
	hd.FillCardArt(r, vs)
	now := hd.Explore.Now().UTC()
	out := []ItemView{}
	for _, v := range vs {
		if len(out) == newHeroMax || v.TrailerKey == "" || !HeroArtReady(v) {
			continue
		}
		k := v.Kind + ":" + v.TMDBID
		if f := NewFresh(pg.Facts[k], now); f != nil {
			v.Fresh = f
		}
		if m, ok := pg.marks[k]; ok {
			v.Streaming = &m
		}
		out = append(out, v)
	}
	return out, nil
}

// newViews joins rows (Kid limit, never-add, Library), drops rows left
// under least and puts each card's newness strip and service on it.
func (hd *Handlers) newViews(r *http.Request, rows []engine.Row, pg newPage, least int) ([]ExploreRowView, error) {
	views, err := hd.ExploreViews(r.Context(), r, rows)
	if err != nil {
		return nil, err
	}
	now := hd.Explore.Now().UTC()
	out := []ExploreRowView{}
	for _, v := range views {
		if len(v.Items) < least {
			continue
		}
		hd.FillTrailerKeys(r, v.Items)
		out = append(out, v)
	}
	hd.FillRowsCardArt(r, out)
	hd.fillStreamingRows(r, out)
	for i := range out {
		for j := range out[i].Items {
			it := &out[i].Items[j]
			k := it.Kind + ":" + it.TMDBID
			if f := NewFresh(pg.Facts[k], now); f != nil {
				it.Fresh = f
			}
			if m, ok := pg.marks[k]; ok && (out[i].Type == engine.TypeNPServices || out[i].Type == engine.TypeNPTopShows || out[i].Type == engine.TypeNPTopFilms) {
				it.Streaming = &m
			}
		}
	}
	return out, nil
}

// newRailView is one rail title: its card plus the day's word for it.
type newRailView struct {
	ItemView
	What      string `json:"what"`
	Following bool   `json:"following"`
	Upcoming  bool   `json:"upcoming"`
}

type newRailDayView struct {
	Day   string        `json:"day"`
	Items []newRailView `json:"items"`
}

// newRailViews joins the rail with the Library and the Kid limit.
func (hd *Handlers) newRailViews(r *http.Request, rail []newRailDay) ([]newRailDayView, error) {
	out := []newRailDayView{}
	for _, d := range rail {
		row := engine.Row{ID: "np_rail_" + d.Day, Type: "np_rail", Items: make([]engine.Item, 0, len(d.Items))}
		extra := map[string]newRailItem{}
		for _, it := range d.Items {
			row.Items = append(row.Items, it.Item)
			if _, ok := extra[it.Key()]; !ok {
				extra[it.Key()] = it
			}
		}
		views, err := hd.ExploreViews(r.Context(), r, []engine.Row{row})
		if err != nil {
			return nil, err
		}
		if len(views) == 0 || len(views[0].Items) == 0 {
			continue
		}
		hd.rowsText(r, views)
		dv := newRailDayView{Day: d.Day, Items: []newRailView{}}
		for _, v := range views[0].Items {
			e := extra[v.Kind+":"+v.TMDBID]
			if e.Service != nil {
				m := *e.Service
				v.Streaming = &m
			}
			dv.Items = append(dv.Items, newRailView{ItemView: v, What: e.What, Following: e.Following, Upcoming: e.Upcoming})
		}
		out = append(out, dv)
	}
	return out, nil
}

// newParams reads ?type= and checks the member may see the page.
func (hd *Handlers) newParams(w http.ResponseWriter, r *http.Request) (string, bool) {
	sw, ok := ExploreSwitch(r.URL.Query().Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return "", false
	}
	if hd.KidLimited(w, r) {
		return "", false
	}
	if !hd.Cache.Allow(hd.UserID(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return "", false
	}
	return sw, true
}

// exploreNew answers GET /api/v1/explore/new?type=all|movies|shows&page=n:
// New & Popular, a page of its rows (5 first, then 4 a page); page 0 also
// carries the This week rail. more says another page follows, building
// that some lists are still on their way (ask again shortly), services_set
// whether the member picked their services.
func (hd *Handlers) exploreNew(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if r.URL.Query().Get("page") == "" {
		n, err = 0, nil
	}
	if err != nil || n < 0 || n > 20 {
		webserver.Fail(w, http.StatusBadRequest, "page must be a number from 0")
		return
	}
	sw, ok := hd.newParams(w, r)
	if !ok {
		return
	}
	me := hd.UserID(r)
	hd.WarmMusicNew() // the Music tab's lists build in the background (Music on)
	pg, building := hd.newRows(r.Context(), r, me, sw)
	// With lookups off the rows never build: say so once instead of "building" forever.
	if building && len(pg.Rows) == 0 && hd.Modes.Current() == metadata.ModeOff {
		metadata.WriteSourceOff(w)
		return
	}
	var hero []ItemView
	if n == 0 {
		if hero, err = hd.newHero(r, pg, me); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		keys := map[string]bool{}
		for _, v := range hero {
			keys[v.Kind+":"+v.TMDBID] = true
		}
		newHeroKeys.Store(pg.key, keys)
	}
	heroKeys := map[string]bool{}
	if v, ok := newHeroKeys.Load(pg.key); ok {
		heroKeys = v.(map[string]bool)
	}
	rows := WithoutHero(pg.Rows, heroKeys)
	from, to := 0, newFirstRows
	if n > 0 {
		from = newFirstRows + (n-1)*newPageRows
		to = from + newPageRows
	}
	from, to = min(from, len(rows)), min(to, len(rows))
	views, err := hd.newViews(r, rows[from:to], pg, engine.NewRowMin)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	body := map[string]any{"type": sw, "page": n, "more": to < len(rows), "building": building, "services_set": pg.picked, "rows": views}
	if n == 0 {
		body["hero"] = hero
		rail, err := hd.newRailViews(r, pg.Rail)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		body["rail"] = rail
	}
	hd.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, body)
}

// newPreviewMax and newPreviewPer shape Explore's New on your services
// row: the best-scoring new titles of the page's services row, a few per
// service.
const (
	newPreviewMax = 12
	newPreviewPer = 3
)

// exploreNewServices answers GET /api/v1/explore/new/services?type=: the
// best of "Fresh on your services" for Explore (12 titles, at most 3 per
// service, all new).
func (hd *Handlers) exploreNewServices(w http.ResponseWriter, r *http.Request) {
	sw, ok := hd.newParams(w, r)
	if !ok {
		return
	}
	pg, _ := hd.newRows(r.Context(), r, hd.UserID(r), sw)
	var items []engine.Item
	for _, row := range pg.Rows {
		if row.Type == engine.TypeNPServices {
			items = append(items, row.Items...)
		}
	}
	engine.SortByScore(items, func(it engine.Item) float64 { return pg.Scores[it.Key()] })
	items = engine.CapPer(items, func(it engine.Item) string { return strconv.Itoa(pg.marks[it.Key()].Service) }, newPreviewPer, newPreviewMax*2)
	row := ExploreRowView{ID: "np_services", Type: engine.TypeNPServices, Title: "New on your services", Items: []ItemView{}}
	views, err := hd.newViews(r, []engine.Row{{ID: "np_services", Type: engine.TypeNPServices, Title: "New on your services", Items: items}}, pg, 1)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if len(views) == 1 {
		row = views[0]
		row.Items = row.Items[:min(len(row.Items), newPreviewMax)]
	}
	hd.Caching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "row": row})
}

// newServiceRows names the chip rows exploreNewService answers for.
var newServiceRows = map[string]string{"services": engine.TypeNPServices, "hot_shows": engine.TypeNPTopShows, "hot_movies": engine.TypeNPTopFilms}

// exploreNewService answers GET /api/v1/explore/new/service?id=&type=&row=:
// one service's own list for a chip row (row services, the default: up to
// 20 new titles, Library titles last; hot_shows or hot_movies: that
// service's chart, new titles only). From the built page (kept 6 hours), so
// it never asks Streaming Availability again.
func (hd *Handlers) exploreNewService(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "id must be a service id")
		return
	}
	name := r.URL.Query().Get("row")
	if name == "" {
		name = "services"
	}
	typ, ok := newServiceRows[name]
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "row must be services, hot_shows or hot_movies")
		return
	}
	sw, ok := hd.newParams(w, r)
	if !ok {
		return
	}
	pg, building := hd.newRows(r.Context(), r, hd.UserID(r), sw)
	items := pg.perService[typ][id]
	row := ExploreRowView{ID: typ + "_" + strconv.Itoa(id), Type: typ, Items: []ItemView{}}
	views, err := hd.newViews(r, []engine.Row{{ID: row.ID, Type: typ, Items: items}}, pg, 1)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if len(views) == 1 {
		row = views[0]
	}
	var mark StreamingMark
	if n, ok := pg.svcNames[id]; ok {
		mark = StreamingMark{Service: id, Name: n}
	}
	// Library titles last (the Library join may know more than the page).
	sort.SliceStable(row.Items, func(i, j int) bool { return !row.Items[i].InLibrary && row.Items[j].InLibrary })
	for i := range row.Items {
		m := mark
		if typ != engine.TypeNPServices {
			m.Rank = i + 1
		}
		if m.Service != 0 {
			row.Items[i].Streaming = &m
		}
	}
	cache := "private, max-age=21600"
	if building {
		cache = "private, no-cache"
	}
	hd.Caching(w, r, cache)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "service": id, "building": building, "row": row})
}

// NewPage is a member's built New & Popular page (tests read its Rows,
// Rail, Facts and Scores).
type NewPage = newPage

// NewRows builds (or reads the kept) New & Popular page for tests.
func (hd *Handlers) NewRows(ctx context.Context, r *http.Request, me int64, sw string) (NewPage, bool) {
	return hd.newRows(ctx, r, me, sw)
}

// NewHero picks a page's hero for tests.
func (hd *Handlers) NewHero(r *http.Request, pg NewPage, me int64) ([]ItemView, error) {
	return hd.newHero(r, pg, me)
}

// ResetNewPages drops every kept New & Popular page (tests).
func ResetNewPages() {
	newPages.mu.Lock()
	newPages.pages = nil
	newPages.mu.Unlock()
}
