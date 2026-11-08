package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/features"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

const (
	discoverMax = 20
)

type discoverEntry = discover.Entry

// discoverState holds the row cache and each person's request times.
type discoverState = discover.Cache

// loadedDetails is the details page already loaded for kind/id, nil if none.
func (a *API) loadedDetails(kind, id string) *metadata.Details {
	if e, ok := a.discover.Get("details|" + kind + "|" + id); ok {
		return e.Details
	}
	return nil
}

// kidLimited counts one TMDB search, lookup or details page of a Kid
// against the Explore minute limit; true when it answered 429.
func (a *API) kidLimited(w http.ResponseWriter, r *http.Request) bool {
	if a.kidMax(r) == "" || a.discover.Allow(a.person(r).UserID, time.Now()) {
		return false
	}
	w.Header().Set("Retry-After", "60")
	webserver.Fail(w, http.StatusTooManyRequests, "too many lookups; try again in a minute")
	return true
}

// exploreCaching marks an answer that depends on who asks: it varies by
// sign-in, and a Kid's is never reused without asking again. Others get
// cache ("" for none).
func (a *API) exploreCaching(w http.ResponseWriter, r *http.Request, cache string) {
	webserver.CacheHeaders(w, cache, a.kidMax(r) != "")
}

// discoverItemView is one discover item with what the Library knows of it.
type discoverItemView struct {
	metadata.DiscoverItem
	InLibrary bool      `json:"in_library"`
	TitleID   int64     `json:"title_id,omitempty"`
	Ask       *askBrief `json:"ask"`
	// Fresh is the card's one freshness strip (New Episode, Top 10...).
	Fresh *engine.Fresh `json:"fresh,omitempty"`
	// TrailerKey is the title's YouTube trailer when HolyArrs already knows
	// it (Explore only; never looked up for this answer).
	TrailerKey string `json:"trailer_key,omitempty"`
	// Streaming is the member's service the title is on (kept lists only).
	Streaming *streamingMark `json:"streaming,omitempty"`
	// The wide card's picture (card_art, card_logo, art_has_title), from
	// the card_art table only.
	metadata.CardArt
}

// kindsOn are the asked kinds whose part is on.
func (a *API) kindsOn(kind string) []string {
	var out []string
	for _, k := range []string{"movie", "show"} {
		if kind != "" && kind != k {
			continue
		}
		if p, _ := parts.ForTitleKind(k); a.partIsOn(p) {
			out = append(out, k)
		}
	}
	return out
}

// cachedRow reads one row of one kind, from the cache while fresh. A
// rate-limited TMDB is answered from a stale copy when there is one.
func (a *API) cachedRow(ctx context.Context, d metadata.Discoverer, row, kind, ref, genre string) ([]metadata.DiscoverItem, error) {
	key := row + "|" + kind + "|" + ref + "|" + genre
	e, ok := a.discover.Get(key)
	if ok && time.Since(e.At) < discover.TTL[row] {
		return e.Items, nil
	}
	items, err := d.Discover(ctx, row, kind, ref, genre)
	if err != nil {
		if ok && discover.StaleOK(err) {
			return e.Items, nil
		}
		return nil, err
	}
	a.discover.Put(key, discoverEntry{At: time.Now(), Items: items})
	return items, nil
}

// libraryJoin finds the Library title, the Never-add entry and the open ask
// for one item.
func (a *API) libraryJoin(ctx context.Context, it metadata.DiscoverItem, me int64) (discoverItemView, bool, error) {
	v := discoverItemView{DiscoverItem: it}
	if v.Genres == nil {
		v.Genres = []string{}
	}
	db := a.s.Store.Read()
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&v.TitleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	v.InLibrary = v.TitleID != 0
	if v.Fresh, err = a.cardFresh(ctx, it, v.TitleID); err != nil {
		return v, false, err
	}
	var never int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = ? AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&never); err != nil {
		return v, false, err
	}
	if it.Kind == "show" {
		v.Ask, err = a.openShowAsk(ctx, map[string]string{"tmdb": it.TMDBID}, me)
	} else {
		v.Ask, err = a.openAsk(ctx, it.TMDBID, me)
	}
	return v, never > 0, err
}

// dropNeverAdd leaves out the items on the Never-add list.
func (a *API) dropNeverAdd(ctx context.Context, items []metadata.DiscoverItem) ([]metadata.DiscoverItem, error) {
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT kind, json_extract(source_ids, '$.tmdb') FROM never_add WHERE kind IN ('movie', 'show') AND json_extract(source_ids, '$.tmdb') IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	never := map[[2]string]bool{}
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			return nil, err
		}
		never[[2]string{kind, id}] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]metadata.DiscoverItem, 0, len(items))
	for _, it := range items {
		if !never[[2]string{it.Kind, it.TMDBID}] {
			out = append(out, it)
		}
	}
	return out, nil
}

func (a *API) joinItems(ctx context.Context, items []metadata.DiscoverItem, me int64, n int) ([]discoverItemView, error) {
	out := []discoverItemView{}
	for _, it := range items {
		if len(out) == n {
			break
		}
		v, never, err := a.libraryJoin(ctx, it, me)
		if err != nil {
			return nil, err
		}
		if !never {
			out = append(out, v)
		}
	}
	return out, nil
}

func (a *API) discoverRow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	row, kind, idText, genre := q.Get("row"), q.Get("kind"), q.Get("id"), q.Get("g")
	if _, ok := discover.Titles[row]; !ok {
		webserver.Fail(w, http.StatusBadRequest, "row must be trending, popular, upcoming, similar, genre, services or best")
		return
	}
	if kind != "" && kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	var titleID int64
	if row == "similar" {
		n, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || n <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "similar needs id=<Library title id>")
			return
		}
		titleID = n
	}
	if row == "genre" && (genre == "" || len(genre) > 100) {
		webserver.Fail(w, http.StatusBadRequest, "genre needs g=<genre name>")
		return
	}
	services := ""
	if row == "services" {
		services, genre = q.Get("services"), strings.ToUpper(q.Get("region"))
		if !metadata.ServiceIDs(services) || !isCountry(genre) {
			webserver.Fail(w, http.StatusBadRequest, "services needs services=<ids joined by |> and region=<country code>")
			return
		}
	}
	me := a.person(r).UserID
	if !a.discover.Allow(me, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	kinds := a.kindsOn(kind)
	ref := services
	if row == "similar" {
		var t mediafiles.Title
		if err := database.Get(r.Context(), a.s.Store.Read(), "titles", titleID, &t); err != nil || t.State == "removed" || !a.titleAllowed(r, t) {
			webserver.Fail(w, http.StatusNotFound, "no such Title")
			return
		}
		ref = t.SourceIDs["tmdb"]
		kinds = a.kindsOn(t.Kind)
		if (t.Kind != "movie" && t.Kind != "show") || (kind != "" && kind != t.Kind) || ref == "" {
			kinds = nil
		}
	}
	var lists [][]metadata.DiscoverItem
	for _, k := range kinds {
		items, err := a.cachedRow(r.Context(), d, row, k, ref, genre)
		if err != nil {
			discover.Fail(w, err)
			return
		}
		lists = append(lists, items)
	}
	// Both kinds: take turns, so neither crowds the other out.
	var merged []metadata.DiscoverItem
	for i := 0; ; i++ {
		added := false
		for _, l := range lists {
			if i < len(l) {
				merged = append(merged, l[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	// Never-add items go first, so they do not use up a Kid's row. A Kid
	// gets only the items their limit allows; the ratings come from the
	// Library, the details cache or TMDB, and unknown ones stay hidden.
	merged, err := a.dropNeverAdd(r.Context(), merged)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	merged = a.kidItems(r.Context(), a.kidMax(r), merged, discoverMax)
	out, err := a.joinItems(withFreshRow(r.Context(), row), merged, me, discoverMax)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	a.exploreCaching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"row": row, "title": discover.Titles[row], "items": out})
}

// metadataDetails answers a movie or show page from TMDB, in the Library
// or not, with the Library and Requests joined.
func (a *API) metadataDetails(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	key := "details|" + kind + "|" + id
	e, cached := a.discover.Get(key)
	if !cached || e.Details == nil {
		// Details are kept on disk: a restart or a full memory cache does
		// not send the next view out to TMDB.
		if k, ok := loadKept[metadata.Details](r.Context(), a, key); ok {
			e, cached = discoverEntry{At: k.at, Details: &k.v}, true
			a.discover.Put(key, e)
		}
	}
	if cached && e.Details != nil && time.Since(e.At) >= discover.DetailsTTL(e) {
		// Stale but kept: served now, refreshed in the background.
		metadata.RefreshOnce(key, func(ctx context.Context) {
			if det, err := d.Details(ctx, kind, id); err == nil {
				a.discover.Put(key, discoverEntry{At: time.Now(), Details: &det})
				keepOnDisk(ctx, a, key, det)
			}
		})
	} else if !cached || time.Since(e.At) >= discover.DetailsTTL(e) {
		det, err := d.Details(r.Context(), kind, id)
		switch {
		case err == nil:
			e = discoverEntry{At: time.Now(), Details: &det}
			a.discover.Put(key, e)
			keepOnDisk(r.Context(), a, key, det)
		case !(cached && discover.StaleOK(err)):
			discover.Fail(w, err)
			return
		}
	}
	max := a.kidMax(r)
	if max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	me := a.person(r).UserID
	head, never, err := a.libraryJoin(r.Context(), e.Details.DiscoverItem, me)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	similar, err := a.joinItems(r.Context(), a.kidItems(r.Context(), max, e.Details.Similar, 0), me, len(e.Details.Similar))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	a.exploreCaching(w, r, "")
	det := *e.Details
	if det.People == nil {
		det.People = []metadata.DetailPerson{}
	}
	type collectionView struct {
		ID    string             `json:"id"`
		Name  string             `json:"name"`
		Parts []discoverItemView `json:"parts"`
	}
	var coll *collectionView
	if c := e.Details.Collection; c != nil {
		parts, err := a.joinItems(r.Context(), a.kidItems(r.Context(), max, c.Parts, 0), me, len(c.Parts))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if len(parts) > 1 {
			coll = &collectionView{ID: c.ID, Name: c.Name, Parts: parts}
		}
	}
	a.rowCardArt(r, similar)
	if coll != nil {
		a.rowCardArt(r, coll.Parts)
	}
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if !isCountry(region) {
		region = "US"
	}
	streaming := det.Streaming[region]
	if streaming == nil {
		streaming = []metadata.WatchService{}
	}
	quality := ""
	if head.TitleID != 0 {
		var res sql.NullInt64
		if err := a.s.Store.Read().QueryRowContext(r.Context(), `SELECT MAX(CAST(json_extract(f.quality, '$.resolution') AS INTEGER)) FROM files f
			JOIN versions v ON v.id = f.version_id WHERE v.title_id = ? AND f.role = 'main' AND f.state = 'present'`, head.TitleID).Scan(&res); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		quality = discover.QualityWord(res.Int64)
	}
	webserver.WriteJSON(w, http.StatusOK, struct {
		metadata.Details
		Quality    string                  `json:"quality,omitempty"`
		Streaming  []metadata.WatchService `json:"streaming"`
		Collection *collectionView         `json:"collection,omitempty"`
		Similar    []discoverItemView      `json:"similar"`
		InLibrary  bool                    `json:"in_library"`
		TitleID    int64                   `json:"title_id,omitempty"`
		Ask        *askBrief               `json:"ask"`
		NeverAdd   bool                    `json:"never_add"`
	}{Details: det, Quality: quality, Streaming: streaming, Collection: coll, Similar: similar, InLibrary: head.InLibrary, TitleID: head.TitleID, Ask: head.Ask, NeverAdd: never})
}

// rowCardArt gives a title page row its card pictures and logos from the
// card_art table (one read for the row, no outside call), and queues the
// titles without a pick for the paced filler.
func (a *API) rowCardArt(r *http.Request, items []discoverItemView) {
	a.fillCardArt(r, items)
	var missing []engine.Item
	for _, v := range items {
		if v.CardArt.Logo == "" && v.CardArt.Art == "" {
			missing = append(missing, engine.Item{Kind: v.Kind, TMDBID: v.TMDBID, Backdrop: v.Backdrop})
		}
	}
	if len(missing) > 0 {
		a.queueCardItems(context.WithoutCancel(r.Context()), missing)
	}
}

// moreFromMin is the fewest titles a More from row shows; fewer answer none.
const moreFromMin = 4

// metadataMoreFrom answers GET /api/v1/metadata/{kind}/{tmdb_id}/more-from:
// the other movies the title's director made, or the other shows its
// creator made, most popular first, with the Library and Requests joined.
// Items is empty when fewer than four are left. Cached 24 h per title.
func (a *API) metadataMoreFrom(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	l, ok2 := metadata.MadeByListerFor(r.Context(), a.s.Metadata)
	if !ok || !ok2 {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	key := "morefrom|" + kind + "|" + id
	e, cached := a.discover.Get(key)
	if !cached || time.Since(e.At) >= discover.TTL["details"] {
		dkey := "details|" + kind + "|" + id
		de, dcached := a.discover.Get(dkey)
		if !dcached || time.Since(de.At) >= discover.TTL["details"] {
			det, err := d.Details(r.Context(), kind, id)
			if err != nil {
				discover.Fail(w, err)
				return
			}
			de = discoverEntry{At: time.Now(), Details: &det}
			a.discover.Put(dkey, de)
		}
		role := "director"
		if kind == "show" {
			role = "creator"
		}
		var who metadata.DetailPerson
		for _, p := range de.Details.People {
			if p.Role == role && p.ID != "" {
				who = p
				break
			}
		}
		// The source title's rating is kept so the Kid gate below runs on cache hits too.
		e = discoverEntry{At: time.Now(), Details: &metadata.Details{Rating: de.Details.Rating, People: []metadata.DetailPerson{who}}, Items: []metadata.DiscoverItem{}}
		if who.ID != "" {
			items, err := l.MadeBy(r.Context(), kind, who.ID)
			switch {
			case err == nil:
				for _, it := range items {
					if it.TMDBID != id {
						e.Items = append(e.Items, it)
					}
				}
			case cached && errors.Is(err, metadata.ErrRateLimited):
				e, _ = a.discover.Get(key)
			default:
				discover.Fail(w, err)
				return
			}
		}
		a.discover.Put(key, e)
	}
	max := a.kidMax(r)
	if max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	items := a.kidItems(r.Context(), max, e.Items, 0)
	out := []discoverItemView{}
	if len(items) >= moreFromMin {
		var err error
		out, err = a.joinItems(r.Context(), items, a.person(r).UserID, len(items))
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		a.rowCardArt(r, out)
	}
	who := e.Details.People[0]
	a.exploreCaching(w, r, "")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"person": who.Name, "role": who.Role, "items": out})
}

// metadataSeason answers GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}:
// one season's episodes from TMDB (number, name, overview, runtime,
// still, air_day), for a show in the Library or not. Cached 24 h. A Kid
// only sees seasons of shows their limit allows.
func (a *API) metadataSeason(w http.ResponseWriter, r *http.Request) {
	id, num := r.PathValue("tmdb_id"), r.PathValue("number")
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	season, err := strconv.Atoi(num)
	if err != nil || season < 0 || season > 999 {
		webserver.Fail(w, http.StatusBadRequest, "number must be a season number")
		return
	}
	if len(a.kindsOn("show")) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	l, ok := metadata.SeasonListerFor(r.Context(), a.s.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	if max := a.kidMax(r); max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), "show", id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
			if !ok {
				webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
				return
			}
			key := "details|show|" + id
			e, cached := a.discover.Get(key)
			if !cached || time.Since(e.At) >= discover.TTL["details"] {
				det, err := d.Details(r.Context(), "show", id)
				if err != nil {
					discover.Fail(w, err)
					return
				}
				e = discoverEntry{At: time.Now(), Details: &det}
				a.discover.Put(key, e)
			}
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, "show", rating) {
			webserver.Fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	key := "season|" + id + "|" + strconv.Itoa(season)
	e, cached := a.discover.Get(key)
	if !cached || time.Since(e.At) >= discover.TTL["details"] {
		eps, err := l.SeasonEpisodes(r.Context(), id, season)
		switch {
		case err == nil:
			e = discoverEntry{At: time.Now(), Episodes: eps}
			a.discover.Put(key, e)
		case !(cached && errors.Is(err, metadata.ErrRateLimited)):
			discover.Fail(w, err)
			return
		}
	}
	a.exploreCaching(w, r, "")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tmdb_id": id, "season": season, "episodes": e.Episodes})
}

// watchServices answers GET /api/v1/watch-services?region=<ISO>: the
// streaming services TMDB knows there for movies and shows, best known first.
func (a *API) watchServices(w http.ResponseWriter, r *http.Request) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if !isCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "region must be a country code")
		return
	}
	key := "services-list|" + region
	if e, ok := a.discover.Get(key); ok && time.Since(e.At) < 24*time.Hour {
		a.exploreCaching(w, r, "private, max-age=3600")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"region": region, "services": e.Services})
		return
	}
	if !a.discover.Allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	l, ok := metadata.ServiceListerFor(r.Context(), a.s.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	// One list for both kinds: a service keeps its best place in either.
	at := map[int64]int{}
	out := []metadata.WatchService{}
	for _, k := range []string{"movie", "show"} {
		list, err := l.WatchServices(r.Context(), k, region)
		if err != nil {
			discover.Fail(w, err)
			return
		}
		for _, s := range list {
			if i, seen := at[s.ID]; seen {
				if s.Priority < out[i].Priority {
					out[i].Priority = s.Priority
				}
				continue
			}
			at[s.ID] = len(out)
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	a.discover.Put(key, discoverEntry{At: time.Now(), Services: out})
	a.exploreCaching(w, r, "private, max-age=3600")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"region": region, "services": out})
}

// reviewHandlers builds the metadata review routes on this package's
// Explore cache, Kid limits and part switches.
func (a *API) reviewHandlers() *metadata.ReviewHandlers {
	return &metadata.ReviewHandlers{
		Provider:      a.s.Metadata,
		Secrets:       a.s.Secrets,
		TraktBase:     a.s.TraktBase,
		TraktOn:       func() bool { return startup.FeatureOn(features.TraktLists) },
		PartOn:        func(kind string) bool { return len(a.kindsOn(kind)) > 0 },
		KidLimited:    a.kidLimited,
		KidMax:        a.kidMax,
		LibraryRating: a.libraryRating,
		Details:       a.cachedDetails,
		Cache:         (*reviewCache)(a),
		Caching:       func(w http.ResponseWriter, r *http.Request) { a.exploreCaching(w, r, "") },
		Fail:          discover.Fail,
		Actor:         actor,
	}
}

// cachedDetails is a title's details through the Explore cache.
func (a *API) cachedDetails(ctx context.Context, d metadata.Discoverer, kind, id string) (*metadata.Details, error) {
	key := "details|" + kind + "|" + id
	e, ok := a.discover.Get(key)
	if !ok || time.Since(e.At) >= discover.TTL["details"] {
		det, err := d.Details(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		e = discoverEntry{At: time.Now(), Details: &det}
		a.discover.Put(key, e)
	}
	return e.Details, nil
}

// reviewCache keeps review pages in the Explore cache.
type reviewCache API

func (c *reviewCache) GetReviews(key string) (*metadata.ReviewPage, time.Time, bool) {
	e, ok := c.discover.Get(key)
	if !ok || e.Reviews == nil {
		return nil, time.Time{}, false
	}
	return e.Reviews, e.At, true
}

func (c *reviewCache) PutReviews(key string, p *metadata.ReviewPage) {
	c.discover.Put(key, discoverEntry{At: time.Now(), Reviews: p})
}
