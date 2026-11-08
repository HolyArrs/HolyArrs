package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/asks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// writeTaggedJSON writes v with an ETag (a hash of the body) and, unless
// the handler set its own, "private, no-cache", so the browser keeps the
// answer and revalidates it; a matching If-None-Match gets 304.
func writeTaggedJSON(w http.ResponseWriter, r *http.Request, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		internal(w, err)
		return
	}
	sum := sha256.Sum256(buf.Bytes())
	tag := `"` + hex.EncodeToString(sum[:12]) + `"`
	h := w.Header()
	h.Set("ETag", tag)
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-cache")
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		for _, t := range strings.Split(inm, ",") {
			if t = strings.TrimSpace(t); t == tag || t == "*" || strings.TrimPrefix(t, "W/") == tag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// OwnedItem is one entry of GET /api/v1/titles/owned: a Library title (or
// one album of a Library artist) with its outside id and one state word.
type OwnedItem struct {
	Kind    string   `json:"kind"` // movie, show, artist or album
	TMDB    string   `json:"tmdb,omitempty"`
	MBID    string   `json:"mbid,omitempty"`
	ID      int64    `json:"id"`                // the Library title id (an album's artist)
	State   string   `json:"state"`             // owned, partial, downloading, requested, wanted
	Percent *float64 `json:"percent,omitempty"` // 0-100, while downloading
	Genres  []string `json:"genres,omitempty"`  // movies and shows
}

// ownedSQL reads every Library title and album with its slot counts, its
// active downloads' progress and open asks, in one statement.
const ownedSQL = `WITH st AS (
	SELECT i.title_id, i.part_id, s.state, d.progress FROM slots s JOIN items i ON i.id = s.item_id
	LEFT JOIN downloads d ON d.id = s.active_download_id AND s.state IN ('downloading', 'importing', 'stalled')),
agg AS (SELECT title_id, part_id,
	SUM(state IN ('in_library', 'better_wanted')) AS have,
	SUM(state NOT IN ('in_library', 'better_wanted', 'not_out_yet', 'removed', 'rejected')) AS lack,
	SUM(state IN ('downloading', 'importing', 'stalled')) AS dl,
	AVG(progress) AS pct FROM st GROUP BY title_id, part_id),
tagg AS (SELECT title_id, SUM(have) AS have, SUM(lack) AS lack, SUM(dl) AS dl, AVG(pct) AS pct FROM agg GROUP BY title_id),
ask AS (SELECT DISTINCT a.title_id FROM asks a WHERE a.title_id IS NOT NULL AND a.state IN ('asked', 'approved')
	AND (? OR EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)))
SELECT t.kind, COALESCE(json_extract(t.source_ids, '$.tmdb'), ''), COALESCE(json_extract(t.source_ids, '$.musicbrainz'), ''), t.id, COALESCE(t.rating, ''),
	COALESCE(g.have, 0), COALESCE(g.lack, 0), COALESCE(g.dl, 0), g.pct, ask.title_id IS NOT NULL, COALESCE(t.genres, '[]')
FROM titles t LEFT JOIN tagg g ON g.title_id = t.id LEFT JOIN ask ON ask.title_id = t.id WHERE t.state != 'removed'
UNION ALL
SELECT 'album', '', COALESCE(json_extract(p.source_ids, '$.musicbrainz'), ''), t.id, '',
	COALESCE(g.have, 0), COALESCE(g.lack, 0), COALESCE(g.dl, 0), g.pct, 0, '[]'
FROM parts p JOIN titles t ON t.id = p.title_id AND t.kind = 'artist' AND t.state != 'removed'
LEFT JOIN agg g ON g.part_id = p.id WHERE p.kind = 'album' AND p.included = 1`

// titlesOwned answers GET /api/v1/titles/owned in one query, with the same
// part and Kid filters as /titles, an ETag and 304.
func (a *API) titlesOwned(w http.ResponseWriter, r *http.Request) {
	on := func(kind string) bool {
		if kind == "album" {
			kind = "artist"
		}
		p, ok := parts.ForTitleKind(kind)
		return !ok || a.partIsOn(p) || a.s.Parts.SafeMode
	}
	max := a.kidMax(r)
	// As titleAsks: without the right to approve, only the caller's own
	// asks mark a title requested (the body, so the ETag, is per person).
	who, signed := webserver.From(r.Context())
	all := !signed || who.Has(webserver.PermApprove)
	rows, err := a.s.Store.Read().QueryContext(r.Context(), ownedSQL, all, who.UserID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []OwnedItem{}
	for rows.Next() {
		var it OwnedItem
		var mbid, rating string
		var have, lack, dl int64
		var pct sql.NullFloat64
		var asked bool
		var genres string
		if err := rows.Scan(&it.Kind, &it.TMDB, &mbid, &it.ID, &rating, &have, &lack, &dl, &pct, &asked, &genres); err != nil {
			internal(w, err)
			return
		}
		if !on(it.Kind) || !metadata.RatingAllows(max, strings.Replace(it.Kind, "album", "artist", 1), rating) {
			continue
		}
		if it.Kind == "artist" || it.Kind == "album" {
			it.TMDB, it.MBID = "", mbid
			if mbid == "" {
				continue
			}
		} else if it.TMDB == "" {
			continue
		} else {
			_ = json.Unmarshal([]byte(genres), &it.Genres)
			if it.Genres == nil {
				it.Genres = []string{}
			}
		}
		switch {
		case dl > 0:
			it.State = "downloading"
			if pct.Valid {
				p := math.Round(min(max64(pct.Float64, 0), 1) * 100)
				it.Percent = &p
			}
		case have > 0 && lack > 0:
			it.State = "partial"
		case have > 0:
			it.State = "owned"
		case asked:
			it.State = "requested"
		default:
			it.State = "wanted"
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Vary", "Cookie, Authorization")
	writeTaggedJSON(w, r, map[string]any{"items": out})
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// titleBatch holds the movie Slots (with their Downloads and Files) and the
// asks of every Title, read in a few queries, so listing all Titles does
// not query them once per Title.
type titleBatch struct {
	slots     map[int64][]mediafiles.Slot // by title id (movies only)
	downloads map[int64]domain.Download
	files     map[int64]mediafiles.File
	labels    map[int64]mediafiles.Version
	asks      map[int64][]asks.View
}

type titleBatchKey struct{}

func batchOf(ctx context.Context) *titleBatch {
	b, _ := ctx.Value(titleBatchKey{}).(*titleBatch)
	return b
}

// withTitleBatch reads the batch for all Titles and keeps it on ctx for titleView.
func (a *API) withTitleBatch(ctx context.Context) (context.Context, error) {
	db := a.s.Store.Read()
	b := &titleBatch{slots: map[int64][]mediafiles.Slot{}, downloads: map[int64]domain.Download{}, files: map[int64]mediafiles.File{},
		labels: map[int64]mediafiles.Version{}, asks: map[int64][]asks.View{}}
	itemTitle := map[int64]int64{}
	rows, err := db.QueryContext(ctx, `SELECT i.id, i.title_id FROM items i JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie'`)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var item, title int64
		if err := rows.Scan(&item, &title); err != nil {
			rows.Close()
			return ctx, err
		}
		itemTitle[item] = title
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	movieSlots := "item_id IN (SELECT i.id FROM items i JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie')"
	ss, err := database.Select[mediafiles.Slot](ctx, db, "slots", movieSlots+" ORDER BY id")
	if err != nil {
		return ctx, err
	}
	for _, s := range ss {
		b.slots[itemTitle[s.ItemID]] = append(b.slots[itemTitle[s.ItemID]], s)
	}
	ds, err := database.Select[domain.Download](ctx, db, "downloads", "id IN (SELECT active_download_id FROM slots WHERE "+movieSlots+")")
	if err != nil {
		return ctx, err
	}
	for _, d := range ds {
		b.downloads[d.ID] = d
	}
	fs, err := database.Select[mediafiles.File](ctx, db, "files", "id IN (SELECT file_id FROM slots WHERE "+movieSlots+")")
	if err != nil {
		return ctx, err
	}
	for _, f := range fs {
		b.files[f.ID] = f
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id IN (SELECT id FROM titles WHERE kind = 'movie')")
	if err != nil {
		return ctx, err
	}
	for _, v := range vs {
		b.labels[v.ID] = v
	}
	// Asks: the same rules as titleAsks, in one read.
	who, ok := webserver.From(ctx)
	var views []asks.View
	if !ok || who.Has(webserver.PermApprove) {
		views, err = a.s.Asks.Views(ctx, "a.title_id IS NOT NULL")
	} else {
		views, err = a.s.Asks.Views(ctx, "a.title_id IS NOT NULL AND EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)", who.UserID)
		for i := range views {
			views[i].Mine = true
		}
		hideDeciderNames(views)
	}
	if err != nil {
		return ctx, err
	}
	for _, v := range views {
		if v.TitleID != nil {
			b.asks[*v.TitleID] = append(b.asks[*v.TitleID], v)
		}
	}
	return context.WithValue(ctx, titleBatchKey{}, b), nil
}

// asksFor answers a Title's asks from the batch (never nil, as Views).
func (b *titleBatch) asksFor(id int64) []asks.View {
	if v := b.asks[id]; v != nil {
		return v
	}
	return []asks.View{}
}
