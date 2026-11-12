package discover

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

const discoverMax = 20

// LoadedDetails is the details page already loaded for kind/id, nil if none.
func (h *Handlers) LoadedDetails(kind, id string) *metadata.Details {
	if e, ok := h.Cache.Get("details|" + kind + "|" + id); ok {
		return e.Details
	}
	return nil
}

// KidLimited counts one TMDB search, lookup or details page of a Kid
// against the Explore minute limit; true when it answered 429.
func (h *Handlers) KidLimited(w http.ResponseWriter, r *http.Request) bool {
	if h.KidMax(r) == "" || h.Cache.Allow(h.UserID(r), time.Now()) {
		return false
	}
	w.Header().Set("Retry-After", "60")
	webserver.Fail(w, http.StatusTooManyRequests, "too many lookups; try again in a minute")
	return true
}

// Caching marks an answer that depends on who asks: it varies by
// sign-in, and a Kid's is never reused without asking again. Others get
// cache ("" for none).
func (h *Handlers) Caching(w http.ResponseWriter, r *http.Request, cache string) {
	webserver.CacheHeaders(w, cache, h.KidMax(r) != "")
}

// KindsOn are the asked kinds whose part is on.
func (h *Handlers) KindsOn(kind string) []string {
	var out []string
	for _, k := range []string{"movie", "show"} {
		if kind != "" && kind != k {
			continue
		}
		if p, _ := parts.ForTitleKind(k); h.PartOn(p) {
			out = append(out, k)
		}
	}
	return out
}

// CachedRow reads one row of one kind, from the cache while fresh. A
// rate-limited TMDB is answered from a stale copy when there is one.
func (h *Handlers) CachedRow(ctx context.Context, d metadata.Discoverer, row, kind, ref, genre string) ([]metadata.DiscoverItem, error) {
	key := row + "|" + kind + "|" + ref + "|" + genre
	e, ok := h.Cache.Get(key)
	if ok && time.Since(e.At) < TTL[row] {
		return e.Items, nil
	}
	items, err := d.Discover(ctx, row, kind, ref, genre)
	if err != nil {
		if ok && StaleOK(err) {
			return e.Items, nil
		}
		return nil, err
	}
	h.Cache.Put(key, Entry{At: time.Now(), Items: items})
	return items, nil
}

// LibraryJoin finds the Library title, the Never-add entry and the open ask
// for one item.
func (h *Handlers) LibraryJoin(ctx context.Context, it metadata.DiscoverItem, me int64) (ItemView, bool, error) {
	v := ItemView{DiscoverItem: it}
	if v.Genres == nil {
		v.Genres = []string{}
	}
	db := h.Store.Read()
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&v.TitleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	v.InLibrary = v.TitleID != 0
	if v.Fresh, err = h.CardFresh(ctx, it, v.TitleID); err != nil {
		return v, false, err
	}
	var never int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = ? AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&never); err != nil {
		return v, false, err
	}
	if it.Kind == "show" {
		v.Ask, err = h.OpenShowAsk(ctx, map[string]string{"tmdb": it.TMDBID}, me)
	} else {
		v.Ask, err = h.OpenAsk(ctx, it.TMDBID, me)
	}
	return v, never > 0, err
}

// DropNeverAdd leaves out the items on the Never-add list, and the ones
// the person on ctx has hidden from browsing.
func (h *Handlers) DropNeverAdd(ctx context.Context, items []metadata.DiscoverItem) ([]metadata.DiscoverItem, error) {
	items, err := h.dropHidden(ctx, items)
	if err != nil {
		return nil, err
	}
	rows, err := h.Store.Read().QueryContext(ctx, "SELECT kind, json_extract(source_ids, '$.tmdb') FROM never_add WHERE kind IN ('movie', 'show') AND json_extract(source_ids, '$.tmdb') IS NOT NULL")
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

// JoinItems joins up to n items with the Library, leaving out Never-add
// items and the ones the person on ctx has hidden.
func (h *Handlers) JoinItems(ctx context.Context, items []metadata.DiscoverItem, me int64, n int) ([]ItemView, error) {
	out := []ItemView{}
	items, err := h.dropHidden(ctx, items)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if len(out) == n {
			break
		}
		v, never, err := h.LibraryJoin(ctx, it, me)
		if err != nil {
			return nil, err
		}
		if !never {
			out = append(out, v)
		}
	}
	return out, nil
}

func (h *Handlers) discoverRow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	row, kind, idText, genre := q.Get("row"), q.Get("kind"), q.Get("id"), q.Get("g")
	if _, ok := Titles[row]; !ok {
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
		if !metadata.ServiceIDs(services) || !IsCountry(genre) {
			webserver.Fail(w, http.StatusBadRequest, "services needs services=<ids joined by |> and region=<country code>")
			return
		}
	}
	me := h.UserID(r)
	if !h.Cache.Allow(me, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), h.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	kinds := h.KindsOn(kind)
	ref := services
	if row == "similar" {
		var t mediafiles.Title
		if err := database.Get(r.Context(), h.Store.Read(), "titles", titleID, &t); err != nil || t.State == "removed" || !h.TitleAllowed(r, t) {
			webserver.Fail(w, http.StatusNotFound, "no such Title")
			return
		}
		ref = t.SourceIDs["tmdb"]
		kinds = h.KindsOn(t.Kind)
		if (t.Kind != "movie" && t.Kind != "show") || (kind != "" && kind != t.Kind) || ref == "" {
			kinds = nil
		}
	}
	var lists [][]metadata.DiscoverItem
	for _, k := range kinds {
		items, err := h.CachedRow(r.Context(), d, row, k, ref, genre)
		if err != nil {
			Fail(w, err)
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
	merged, err := h.DropNeverAdd(r.Context(), merged)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	merged = h.KidItems(r.Context(), h.KidMax(r), merged, discoverMax)
	out, err := h.JoinItems(h.WithFreshRow(r.Context(), row), merged, me, discoverMax)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	h.Caching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"row": row, "title": Titles[row], "items": out})
}

// metadataDetails answers a movie or show page from TMDB, in the Library
// or not, with the Library and Requests joined.
func (h *Handlers) metadataDetails(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(h.KindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), h.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	key := "details|" + kind + "|" + id
	e, cached := h.Cache.Get(key)
	if !cached || e.Details == nil {
		// Details are kept on disk: a restart or a full memory cache does
		// not send the next view out to TMDB.
		if k, ok := metadata.LoadKept[metadata.Details](r.Context(), h.Store, key); ok {
			e, cached = Entry{At: k.At, Details: &k.V}, true
			h.Cache.Put(key, e)
		}
	}
	// Details kept before titles carried their texts are read again now
	// for a person who reads another language than English.
	lang := metadata.TextLanguage(r)
	text, textKnown := keptText(r.Context(), h.Metadata, kind, id, lang)
	if cached && e.Details != nil && time.Since(e.At) >= DetailsTTL(e) && textKnown {
		// Stale but kept: served now, refreshed in the background.
		metadata.RefreshOnce(key, func(ctx context.Context) {
			if det, err := d.Details(ctx, kind, id); err == nil {
				h.Cache.Put(key, Entry{At: time.Now(), Details: &det})
				metadata.KeepOnDisk(ctx, h.Store, key, det)
			}
		})
	} else if !cached || !textKnown || time.Since(e.At) >= DetailsTTL(e) {
		det, err := d.Details(r.Context(), kind, id)
		switch {
		case err == nil:
			e = Entry{At: time.Now(), Details: &det}
			h.Cache.Put(key, e)
			metadata.KeepOnDisk(r.Context(), h.Store, key, det)
			text, _ = keptText(r.Context(), h.Metadata, kind, id, lang)
		// A kept page asked again only for its texts stays, in English.
		case !(cached && (e.Details != nil || StaleOK(err))):
			Fail(w, err)
			return
		}
	}
	max := h.KidMax(r)
	if max != "" {
		rating, inLibrary := h.LibraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
			return
		}
	}
	me := h.UserID(r)
	head, never, err := h.LibraryJoin(r.Context(), e.Details.DiscoverItem, me)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	similar, err := h.JoinItems(r.Context(), h.KidItems(r.Context(), max, e.Details.Similar, 0), me, len(e.Details.Similar))
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	h.Caching(w, r, "")
	det := *e.Details
	if det.People == nil {
		det.People = []metadata.DetailPerson{}
	}
	detailsText(&det, text)
	type collectionView struct {
		ID    string     `json:"id"`
		Name  string     `json:"name"`
		Parts []ItemView `json:"parts"`
	}
	var coll *collectionView
	if c := e.Details.Collection; c != nil {
		parts, err := h.JoinItems(r.Context(), h.KidItems(r.Context(), max, c.Parts, 0), me, len(c.Parts))
		if err != nil {
			webserver.Internal(w, "discover", err)
			return
		}
		if len(parts) > 1 {
			coll = &collectionView{ID: c.ID, Name: c.Name, Parts: parts}
		}
	}
	h.rowCardArt(r, similar)
	if coll != nil {
		h.rowCardArt(r, coll.Parts)
	}
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if !IsCountry(region) {
		region = "US"
	}
	streaming := det.Streaming[region]
	if streaming == nil {
		streaming = []metadata.WatchService{}
	}
	quality := ""
	if head.TitleID != 0 {
		var res sql.NullInt64
		if err := h.Store.Read().QueryRowContext(r.Context(), `SELECT MAX(CAST(json_extract(f.quality, '$.resolution') AS INTEGER)) FROM files f
			JOIN versions v ON v.id = f.version_id WHERE v.title_id = ? AND f.role = 'main' AND f.state = 'present'`, head.TitleID).Scan(&res); err != nil {
			webserver.Internal(w, "discover", err)
			return
		}
		quality = QualityWord(res.Int64)
	}
	hidden, hides, err := h.titleHides(r, e.Details.DiscoverItem)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, struct {
		metadata.Details
		// Hidden: the viewer's hides cover this title; Hides are the hides
		// that cover it, for owners and admins (to show it again).
		Hidden     bool                    `json:"hidden,omitempty"`
		Hides      []Hide                  `json:"hides,omitempty"`
		Quality    string                  `json:"quality,omitempty"`
		Streaming  []metadata.WatchService `json:"streaming"`
		Collection *collectionView         `json:"collection,omitempty"`
		Similar    []ItemView              `json:"similar"`
		InLibrary  bool                    `json:"in_library"`
		TitleID    int64                   `json:"title_id,omitempty"`
		Ask        *requests.AskBrief      `json:"ask"`
		NeverAdd   bool                    `json:"never_add"`
	}{Details: det, Hidden: hidden, Hides: hides, Quality: quality, Streaming: streaming, Collection: coll, Similar: similar, InLibrary: head.InLibrary, TitleID: head.TitleID, Ask: head.Ask, NeverAdd: never})
}

// rowCardArt gives a title page row its card pictures and logos from the
// card_art table (one read for the row, no outside call), and queues the
// titles without a pick for the paced filler.
func (h *Handlers) rowCardArt(r *http.Request, items []ItemView) {
	h.FillCardArt(r, items)
	var missing []engine.Item
	for _, v := range items {
		if v.CardArt.Logo == "" && v.CardArt.Art == "" {
			missing = append(missing, engine.Item{Kind: v.Kind, TMDBID: v.TMDBID, Backdrop: v.Backdrop})
		}
	}
	if len(missing) > 0 {
		h.QueueCardItems(context.WithoutCancel(r.Context()), missing)
	}
}

// moreFromMin is the fewest titles a More from row shows; fewer answer none.
const moreFromMin = 4

// metadataMoreFrom answers GET /api/v1/metadata/{kind}/{tmdb_id}/more-from:
// the other movies the title's director made, or the other shows its
// creator made, most popular first, with the Library and Requests joined.
// Items is empty when fewer than four are left. Cached 24 h per title.
func (h *Handlers) metadataMoreFrom(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(h.KindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), h.Metadata)
	l, ok2 := metadata.MadeByListerFor(r.Context(), h.Metadata)
	if !ok || !ok2 {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	key := "morefrom|" + kind + "|" + id
	e, cached := h.Cache.Get(key)
	if !cached || time.Since(e.At) >= TTL["details"] {
		dkey := "details|" + kind + "|" + id
		de, dcached := h.Cache.Get(dkey)
		if !dcached || time.Since(de.At) >= TTL["details"] {
			det, err := d.Details(r.Context(), kind, id)
			if err != nil {
				Fail(w, err)
				return
			}
			de = Entry{At: time.Now(), Details: &det}
			h.Cache.Put(dkey, de)
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
		e = Entry{At: time.Now(), Details: &metadata.Details{Rating: de.Details.Rating, People: []metadata.DetailPerson{who}}, Items: []metadata.DiscoverItem{}}
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
				e, _ = h.Cache.Get(key)
			default:
				Fail(w, err)
				return
			}
		}
		h.Cache.Put(key, e)
	}
	max := h.KidMax(r)
	if max != "" {
		rating, inLibrary := h.LibraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
			return
		}
	}
	items := h.KidItems(r.Context(), max, e.Items, 0)
	out := []ItemView{}
	if len(items) >= moreFromMin {
		var err error
		out, err = h.JoinItems(r.Context(), items, h.UserID(r), len(items))
		if err != nil {
			webserver.Internal(w, "discover", err)
			return
		}
		h.rowCardArt(r, out)
	}
	who := e.Details.People[0]
	h.Caching(w, r, "")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"person": who.Name, "role": who.Role, "items": out})
}

// metadataSeason answers GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}:
// one season's episodes from TMDB (number, name, overview, runtime,
// still, air_day), for a show in the Library or not. Cached 24 h. A Kid
// only sees seasons of shows their limit allows.
func (h *Handlers) metadataSeason(w http.ResponseWriter, r *http.Request) {
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
	if len(h.KindsOn("show")) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	l, ok := metadata.SeasonListerFor(r.Context(), h.Metadata)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	if max := h.KidMax(r); max != "" {
		rating, inLibrary := h.LibraryRating(r.Context(), "show", id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), h.Metadata)
			if !ok {
				webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
				return
			}
			key := "details|show|" + id
			e, cached := h.Cache.Get(key)
			if !cached || time.Since(e.At) >= TTL["details"] {
				det, err := d.Details(r.Context(), "show", id)
				if err != nil {
					Fail(w, err)
					return
				}
				e = Entry{At: time.Now(), Details: &det}
				h.Cache.Put(key, e)
			}
			rating = e.Details.Rating
		}
		if !metadata.RatingAllows(max, "show", rating) {
			webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
			return
		}
	}
	key := "season|" + id + "|" + strconv.Itoa(season)
	e, cached := h.Cache.Get(key)
	if !cached || time.Since(e.At) >= TTL["details"] {
		eps, err := l.SeasonEpisodes(r.Context(), id, season)
		switch {
		case err == nil:
			e = Entry{At: time.Now(), Episodes: eps}
			h.Cache.Put(key, e)
		case !(cached && errors.Is(err, metadata.ErrRateLimited)):
			Fail(w, err)
			return
		}
	}
	eps := e.Episodes
	// Episode names and synopses in the page's language, where TMDB has
	// them; a failed ask keeps English.
	if st, ok := metadata.SeasonTexterFor(r.Context(), h.Metadata); ok {
		if texts, err := st.SeasonText(r.Context(), id, season, metadata.TextLanguage(r)); err == nil {
			eps = seasonText(eps, texts)
		}
	}
	h.Caching(w, r, "")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tmdb_id": id, "season": season, "episodes": eps})
}

// watchServices answers GET /api/v1/watch-services?region=<ISO>: the
// streaming services TMDB knows there for movies and shows, best known first.
func (h *Handlers) watchServices(w http.ResponseWriter, r *http.Request) {
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if !IsCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "region must be a country code")
		return
	}
	key := "services-list|" + region
	if e, ok := h.Cache.Get(key); ok && time.Since(e.At) < 24*time.Hour {
		h.Caching(w, r, "private, max-age=3600")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"region": region, "services": e.Services})
		return
	}
	if !h.Cache.Allow(h.UserID(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	l, ok := metadata.ServiceListerFor(r.Context(), h.Metadata)
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
			Fail(w, err)
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
	h.Cache.Put(key, Entry{At: time.Now(), Services: out})
	h.Caching(w, r, "private, max-age=3600")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"region": region, "services": out})
}

// reviewHandlers builds the metadata review routes on this package's
// Explore cache, Kid limits and part switches.
func (h *Handlers) reviewHandlers() *metadata.ReviewHandlers {
	return &metadata.ReviewHandlers{
		Provider:      h.Metadata,
		Secrets:       h.Secrets,
		TraktBase:     h.TraktBase,
		TraktOn:       func() bool { return startup.FeatureOn(startup.TraktLists) },
		PartOn:        func(kind string) bool { return len(h.KindsOn(kind)) > 0 },
		KidLimited:    h.KidLimited,
		KidMax:        h.KidMax,
		LibraryRating: h.LibraryRating,
		Details:       h.CachedDetails,
		Cache:         (*reviewCache)(h),
		Caching:       func(w http.ResponseWriter, r *http.Request) { h.Caching(w, r, "") },
		Fail:          Fail,
		Actor:         h.Actor,
	}
}

// CachedDetails is a title's details through the Explore cache.
func (h *Handlers) CachedDetails(ctx context.Context, d metadata.Discoverer, kind, id string) (*metadata.Details, error) {
	key := "details|" + kind + "|" + id
	e, ok := h.Cache.Get(key)
	if !ok || time.Since(e.At) >= TTL["details"] {
		det, err := d.Details(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		e = Entry{At: time.Now(), Details: &det}
		h.Cache.Put(key, e)
	}
	return e.Details, nil
}

// reviewCache keeps review pages in the Explore cache.
type reviewCache Handlers

func (c *reviewCache) GetReviews(key string) (*metadata.ReviewPage, time.Time, bool) {
	e, ok := c.Cache.Get(key)
	if !ok || e.Reviews == nil {
		return nil, time.Time{}, false
	}
	return e.Reviews, e.At, true
}

func (c *reviewCache) PutReviews(key string, p *metadata.ReviewPage) {
	c.Cache.Put(key, Entry{At: time.Now(), Reviews: p})
}
