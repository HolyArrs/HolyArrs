package mediafiles

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

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

// OwnedHandlers serve GET /api/v1/titles/owned. The part switch, the Kid
// limit and the age rating check come from the caller.
type OwnedHandlers struct {
	Store *database.Store
	// KindOn says whether a title kind's part is on (an album is an artist).
	KindOn func(kind string) bool
	// KidMax is the caller's age limit ("" none).
	KidMax func(*http.Request) string
	// RatingAllows says whether a rating fits under max for a kind.
	RatingAllows func(max, kind, rating string) bool
}

// Routes mounts GET /api/v1/titles/owned.
func (a *OwnedHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/owned", a.titlesOwned)
}

// titlesOwned answers GET /api/v1/titles/owned in one query, with the same
// part and Kid filters as /titles, an ETag and 304.
func (a *OwnedHandlers) titlesOwned(w http.ResponseWriter, r *http.Request) {
	on := func(kind string) bool {
		if kind == "album" {
			kind = "artist"
		}
		return a.KindOn(kind)
	}
	max := a.KidMax(r)
	// As titleAsks: without the right to approve, only the caller's own
	// asks mark a title requested (the body, so the ETag, is per person).
	who, signed := webserver.From(r.Context())
	all := !signed || who.Has(webserver.PermApprove)
	rows, err := a.Store.Read().QueryContext(r.Context(), ownedSQL, all, who.UserID)
	if err != nil {
		webserver.Internal(w, "api", err)
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
			webserver.Internal(w, "api", err)
			return
		}
		if !on(it.Kind) || !a.RatingAllows(max, strings.Replace(it.Kind, "album", "artist", 1), rating) {
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
		webserver.Internal(w, "api", err)
		return
	}
	w.Header().Set("Vary", "Cookie, Authorization")
	webserver.WriteTaggedJSON(w, r, map[string]any{"items": out})
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
